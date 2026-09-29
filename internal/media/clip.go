package media

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/script"
)

// FFmpegBins holds resolved ffmpeg and ffprobe binary paths.
type FFmpegBins struct {
	FFmpeg  string
	FFprobe string
}

// ClipSegment cuts one segment from videoPath according to script OST rules.
//
// Audio strategy (phase 1):
//   - OST 0: silent video (-an); length from ResolveClipDuration(ttsDur, isHook).
//   - OST 1: keep source audio; clip script timestamp, extending end if window < min.
//   - OST 2: keep source audio; length from ResolveClipDuration like OST 0.
func ClipSegment(ctx context.Context, bins FFmpegBins, videoPath, timestamp string, ost int, ttsDur float64, isHook bool, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}

	startMs, endMs, err := script.ParseTimestampRange(timestamp)
	if err != nil {
		return fmt.Errorf("parse timestamp: %w", err)
	}

	removeAudio := false
	switch ost {
	case 0:
		removeAudio = true
		dur := ResolveClipDuration(ttsDur, isHook)
		endMs = startMs + durationToMs(dur)
	case 1:
		minSec := script.GeneralClipMin
		if isHook {
			minSec = script.OpeningHookMin
		}
		endMs = extendEndMs(startMs, endMs, minSec)
	case 2:
		dur := ResolveClipDuration(ttsDur, isHook)
		endMs = startMs + durationToMs(dur)
	default:
		return fmt.Errorf("unsupported OST %d", ost)
	}

	startFF := msToFFmpegTime(startMs)
	endFF := msToFFmpegTime(endMs)
	duration, err := ffmpegDurationBetween(startFF, endFF)
	if err != nil {
		return err
	}

	args := []string{"-y", "-ss", startFF, "-i", videoPath, "-t", duration}
	args = append(args, defaultVideoEncodeArgs()...)
	if removeAudio {
		args = append(args, "-an")
	} else {
		args = append(args, defaultAudioEncodeArgs()...)
	}
	args = append(args, "-avoid_negative_ts", "make_zero", "-movflags", "+faststart", outPath)

	_, err = Run(ctx, bins.FFmpeg, args...)
	return err
}

// PreviewClip extracts the script timestamp range with stream copy for quick playback.
func PreviewClip(ctx context.Context, bins FFmpegBins, videoPath, timestamp, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	startMs, endMs, err := script.ParseTimestampRange(timestamp)
	if err != nil {
		return fmt.Errorf("时间范围无效（应为 00:00:00,000-00:00:02,000）: %w", err)
	}
	startFF := msToFFmpegTime(startMs)
	endFF := msToFFmpegTime(endMs)
	duration, err := ffmpegDurationBetween(startFF, endFF)
	if err != nil {
		return err
	}
	args := []string{
		"-y", "-ss", startFF, "-i", videoPath, "-t", duration,
		"-c", "copy", "-avoid_negative_ts", "make_zero", "-movflags", "+faststart",
		outPath,
	}
	_, err = Run(ctx, bins.FFmpeg, args...)
	return err
}

func extendEndMs(startMs, endMs int, minSec float64) int {
	minMs := int(math.Round(minSec * 1000))
	if minMs <= 0 {
		return endMs
	}
	current := endMs - startMs
	if current > minMs {
		return endMs
	}
	return startMs + minMs
}

func durationToMs(sec float64) int {
	return int(math.Round(sec * 1000))
}

func msToFFmpegTime(ms int) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	remMs := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, remMs)
}

func ffmpegDurationBetween(startFF, endFF string) (string, error) {
	startSec, err := ffmpegTimeToSeconds(startFF)
	if err != nil {
		return "", fmt.Errorf("start time: %w", err)
	}
	endSec, err := ffmpegTimeToSeconds(endFF)
	if err != nil {
		return "", fmt.Errorf("end time: %w", err)
	}
	dur := endSec - startSec
	if dur <= 0 {
		return "", fmt.Errorf("invalid clip range: %s -> %s", startFF, endFF)
	}
	return trimFloatDuration(dur), nil
}

func ffmpegTimeToSeconds(timeValue string) (float64, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(timeValue, ",", "."))
	parts := strings.Split(normalized, ":")
	switch len(parts) {
	case 3:
		h, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, err
		}
		s, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}
		return float64(h*3600+m*60) + s, nil
	case 2:
		m, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		s, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		return float64(m*60) + s, nil
	default:
		return strconv.ParseFloat(normalized, 64)
	}
}

func trimFloatDuration(sec float64) string {
	s := strconv.FormatFloat(sec, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

func defaultVideoEncodeArgs() []string {
	return []string{"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23", "-pix_fmt", "yuv420p"}
}

func defaultAudioEncodeArgs() []string {
	return []string{"-c:a", "aac", "-ar", "44100", "-ac", "2"}
}
