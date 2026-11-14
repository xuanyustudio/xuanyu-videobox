package subtitleburn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
)

// SubtitlesFilter is the ffmpeg -vf value for one burned subtitle.
func SubtitlesFilter(videoPath, subtitlePath string, style Style) string {
	escaped := media.EscapeSubtitlesFilterPath(videoPath, subtitlePath)
	return fmt.Sprintf("subtitles='%s':force_style='%s'", escaped, ForceStyle(style))
}

// Burn writes outPath with the subtitle burned in using style.
func Burn(ctx context.Context, bins media.FFmpegBins, videoPath, subtitlePath, outPath string, style Style) error {
	return burn(ctx, bins, videoPath, subtitlePath, outPath, style, nil)
}

// BurnProgress is Burn plus a callback of 0–100 for the current file.
func BurnProgress(ctx context.Context, bins media.FFmpegBins, videoPath, subtitlePath, outPath string, style Style, onProgress func(percent int)) error {
	return burn(ctx, bins, videoPath, subtitlePath, outPath, style, onProgress)
}

func burn(ctx context.Context, bins media.FFmpegBins, videoPath, subtitlePath, outPath string, style Style, onProgress func(percent int)) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	if _, err := os.Stat(bins.FFmpeg); err != nil {
		return fmt.Errorf("找不到 ffmpeg：%s。设置里如果写了 .\\ffmpeg.exe，请改成完整路径，或把 ffmpeg.exe 放在 narrato.exe 旁边", bins.FFmpeg)
	}
	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", SubtitlesFilter(videoPath, subtitlePath, style),
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "copy",
		"-movflags", "+faststart",
	}
	duration := 0.0
	if onProgress != nil && strings.TrimSpace(bins.FFprobe) != "" {
		if d, err := media.ProbeDurationSec(ctx, bins, videoPath); err == nil && d > 0 {
			duration = d
		}
		args = append(args, "-progress", "pipe:1", "-nostats")
	}
	args = append(args, outPath)
	if onProgress == nil {
		out, err := media.RunInDir(ctx, filepath.Dir(videoPath), bins.FFmpeg, args...)
		if err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
		}
		return nil
	}
	stderr, err := media.RunInDirLines(ctx, filepath.Dir(videoPath), bins.FFmpeg, func(line string) {
		if pct, ok := progressPercent(line, duration); ok {
			onProgress(pct)
		}
	}, args...)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	onProgress(100)
	return nil
}

func progressPercent(line string, duration float64) (int, bool) {
	line = strings.TrimSpace(line)
	if line == "progress=end" {
		return 100, true
	}
	const prefix = "out_time="
	if duration <= 0 || !strings.HasPrefix(line, prefix) {
		return 0, false
	}
	sec, ok := parseClock(strings.TrimPrefix(line, prefix))
	if !ok {
		return 0, false
	}
	pct := int(sec / duration * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 99 {
		pct = 99
	}
	return pct, true
}

func parseClock(v string) (float64, bool) {
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, errH := strconv.ParseFloat(parts[0], 64)
	m, errM := strconv.ParseFloat(parts[1], 64)
	s, errS := strconv.ParseFloat(parts[2], 64)
	if errH != nil || errM != nil || errS != nil {
		return 0, false
	}
	return h*3600 + m*60 + s, true
}
