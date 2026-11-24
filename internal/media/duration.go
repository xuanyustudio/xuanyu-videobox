package media

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ProbeDurationSec returns media duration in seconds via ffprobe.
func ProbeDurationSec(ctx context.Context, bins FFmpegBins, path string) (float64, error) {
	ffprobe := bins.FFprobe
	if ffprobe == "" {
		var err error
		ffprobe, err = ResolveFFprobe("", "")
		if err != nil {
			return 0, err
		}
	}
	out, err := Run(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", strings.TrimSpace(out), err)
	}
	return d, nil
}

// VideoSize is a frame width and height in pixels.
type VideoSize struct {
	Width  int
	Height int
}

// ProbeVideoSize returns the first video stream's width and height.
func ProbeVideoSize(ctx context.Context, bins FFmpegBins, path string) (VideoSize, error) {
	ffprobe := bins.FFprobe
	if ffprobe == "" {
		var err error
		ffprobe, err = ResolveFFprobe("", "")
		if err != nil {
			return VideoSize{}, err
		}
	}
	out, err := Run(ctx, ffprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0:s=x",
		path,
	)
	if err != nil {
		return VideoSize{}, fmt.Errorf("ffprobe size: %w", err)
	}
	line := strings.TrimSpace(out)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	parts := strings.Split(line, "x")
	if len(parts) != 2 {
		return VideoSize{}, fmt.Errorf("parse size %q", line)
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return VideoSize{}, fmt.Errorf("parse size %q", line)
	}
	return VideoSize{Width: w, Height: h}, nil
}

// HasVideoStream reports whether path has at least one video stream.
func HasVideoStream(ctx context.Context, bins FFmpegBins, path string) (bool, error) {
	ffprobe := bins.FFprobe
	if ffprobe == "" {
		var err error
		ffprobe, err = ResolveFFprobe("", "")
		if err != nil {
			return false, err
		}
	}
	out, err := Run(ctx, ffprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		path,
	)
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(out)), "video"), nil
}

