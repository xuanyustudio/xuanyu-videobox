package subtitleburn

import (
	"context"
	"fmt"
	"path/filepath"
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
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", SubtitlesFilter(videoPath, subtitlePath, style),
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "copy",
		"-movflags", "+faststart",
		outPath,
	}
	out, err := media.RunInDir(ctx, filepath.Dir(videoPath), bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}
