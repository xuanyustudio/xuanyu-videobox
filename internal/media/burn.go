package media

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// BurnSubtitles muxes video with an SRT burned in via the subtitles filter.
func BurnSubtitles(ctx context.Context, bins FFmpegBins, videoPath, srtPath, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	filterPath := escapeSubtitlesFilterPath(videoPath, srtPath)
	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", "subtitles=" + filterPath,
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "copy",
		"-movflags", "+faststart",
		outPath,
	}
	workDir := filepath.Dir(videoPath)
	out, err := RunInDir(ctx, workDir, bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// EscapeSubtitlesFilterPath normalizes paths for ffmpeg's subtitles filter (Windows-safe).
func EscapeSubtitlesFilterPath(videoPath, srtPath string) string {
	return escapeSubtitlesFilterPath(videoPath, srtPath)
}

// escapeSubtitlesFilterPath normalizes paths for ffmpeg's subtitles filter (Windows-safe).
func escapeSubtitlesFilterPath(videoPath, srtPath string) string {
	if rel, err := filepath.Rel(filepath.Dir(videoPath), srtPath); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		p := filepath.ToSlash(rel)
		p = strings.ReplaceAll(p, ":", "\\:")
		p = strings.ReplaceAll(p, "'", "\\'")
		return p
	}
	p := filepath.ToSlash(srtPath)
	p = strings.ReplaceAll(p, ":", "\\:")
	p = strings.ReplaceAll(p, "'", "\\'")
	return p
}
