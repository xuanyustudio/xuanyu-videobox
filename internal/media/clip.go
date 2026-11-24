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

// Minimum leftover media required after the clip start; shorter than this
// is treated as past the end of the source (ffmpeg can still write a
// header-only mp4 with no video).
const minRemainingClipMs = 50

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

	startMs, endMs, err := PlanClipRange(timestamp, ost, ttsDur, isHook)
	if err != nil {
		return err
	}
	if srcDur, perr := ProbeDurationSec(ctx, bins, videoPath); perr == nil && srcDur > 0 {
		startMs, endMs, err = FitClipRangeToSource(startMs, endMs, durationToMs(srcDur))
		if err != nil {
			return err
		}
	}

	startFF := msToFFmpegTime(startMs)
	endFF := msToFFmpegTime(endMs)
	duration, err := ffmpegDurationBetween(startFF, endFF)
	if err != nil {
		return err
	}

	args := []string{"-y", "-ss", startFF, "-i", videoPath, "-t", duration, "-map", "0:v:0"}
	args = append(args, defaultVideoEncodeArgs()...)
	if ost == 0 {
		args = append(args, "-an")
	} else {
		args = append(args, "-map", "0:a?")
		args = append(args, defaultAudioEncodeArgs()...)
	}
	args = append(args, "-dn", "-avoid_negative_ts", "make_zero", "-movflags", "+faststart", outPath)

	if _, err = Run(ctx, bins.FFmpeg, args...); err != nil {
		return err
	}
	if ok, herr := HasVideoStream(ctx, bins, outPath); herr != nil || !ok {
		return fmt.Errorf("裁剪结果没有画面（时间 %s–%s），请检查该段时间是否超出片源时长", FormatScriptClock(startMs), FormatScriptClock(endMs))
	}
	return nil
}

// PlanClipRange returns the start/end (milliseconds) ClipSegment would request
// before clamping to the source duration.
func PlanClipRange(timestamp string, ost int, ttsDur float64, isHook bool) (startMs, endMs int, err error) {
	startMs, endMs, err = script.ParseTimestampRange(timestamp)
	if err != nil {
		return 0, 0, fmt.Errorf("parse timestamp: %w", err)
	}

	switch ost {
	case 0:
		endMs = startMs + durationToMs(ResolveClipDuration(ttsDur, isHook))
	case 1:
		minSec := script.GeneralClipMin
		if isHook {
			minSec = script.OpeningHookMin
		}
		endMs = extendEndMs(startMs, endMs, minSec)
	case 2:
		endMs = startMs + durationToMs(ResolveClipDuration(ttsDur, isHook))
	default:
		return 0, 0, fmt.Errorf("unsupported OST %d", ost)
	}
	if endMs <= startMs {
		return startMs, endMs, fmt.Errorf("invalid clip range: %s -> %s", msToFFmpegTime(startMs), msToFFmpegTime(endMs))
	}
	return startMs, endMs, nil
}

// FitClipRangeToSource clamps end to the source duration. If the start is at
// or past the end of the source (or leftover media is under 50ms), it returns
// a Chinese error the UI can show directly.
func FitClipRangeToSource(startMs, endMs, sourceMs int) (int, int, error) {
	if sourceMs <= 0 {
		return startMs, endMs, fmt.Errorf("无法读取片源时长")
	}
	if startMs >= sourceMs || sourceMs-startMs < minRemainingClipMs {
		return startMs, endMs, fmt.Errorf("时间范围从 %s 开始，但片源只有 %s，已经超出片尾", FormatScriptClock(startMs), FormatScriptClock(sourceMs))
	}
	if endMs > sourceMs {
		endMs = sourceMs
	}
	if endMs-startMs < minRemainingClipMs {
		return startMs, endMs, fmt.Errorf("时间范围从 %s 开始，但片源只有 %s，已经超出片尾", FormatScriptClock(startMs), FormatScriptClock(sourceMs))
	}
	return startMs, endMs, nil
}

// FormatScriptClock formats milliseconds as 00:02:54,720 (script-style clock).
func FormatScriptClock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	rem := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, rem)
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
	if srcDur, perr := ProbeDurationSec(ctx, bins, videoPath); perr == nil && srcDur > 0 {
		startMs, endMs, err = FitClipRangeToSource(startMs, endMs, durationToMs(srcDur))
		if err != nil {
			return err
		}
	}
	startFF := msToFFmpegTime(startMs)
	endFF := msToFFmpegTime(endMs)
	duration, err := ffmpegDurationBetween(startFF, endFF)
	if err != nil {
		return err
	}
	args := []string{
		"-y", "-ss", startFF, "-i", videoPath, "-t", duration,
		"-map", "0:v:0", "-map", "0:a?", "-dn",
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
