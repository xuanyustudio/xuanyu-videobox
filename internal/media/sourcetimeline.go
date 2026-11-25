package media

import (
	"context"
	"fmt"
	"math"
	"path/filepath"

	"github.com/narratoai/narrato-go/internal/script"
)

// ProbeSourceTimeline reads each video's duration and builds the concatenated
// project timeline used to map script timestamps onto the right file.
func ProbeSourceTimeline(ctx context.Context, bins FFmpegBins, paths []string) ([]script.SourceSegment, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("没有可用的片源")
	}
	durs := make([]int, len(paths))
	for i, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sec, err := ProbeDurationSec(ctx, bins, p)
		if err != nil {
			return nil, fmt.Errorf("无法读取「%s」的时长: %w", filepath.Base(p), err)
		}
		if sec < 0 {
			sec = 0
		}
		durs[i] = int(math.Round(sec * 1000))
	}
	return script.BuildSourceTimeline(paths, durs), nil
}
