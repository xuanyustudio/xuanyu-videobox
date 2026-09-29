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
