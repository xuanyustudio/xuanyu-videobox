package replicate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/narratoai/narrato-go/internal/media"
)

const (
	// DefaultFrames matches Python MAX_FRAMES.
	DefaultFrames = 8
	// MaxFrames caps sampling to keep request bodies reasonable.
	MaxFrames = 16
	// frameQuality mirrors Python FRAME_QUALITY=85 → ffmpeg -q:v int((100-85)/10)+1 = 2.
	frameQuality = 85
	// fallbackDurationSec is used when ffprobe cannot read the duration (Python: 60s).
	fallbackDurationSec = 60.0
)

// FrameTimestamps returns n evenly spaced sample points: interval = duration/(n+1),
// frame i at interval*(i+1) (e.g. 10s / 8 frames → 1.1s, 2.2s … 8.9s).
func FrameTimestamps(durationSec float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	if durationSec <= 0 {
		durationSec = fallbackDurationSec
	}
	interval := durationSec / float64(n+1)
	out := make([]float64, n)
	for i := range out {
		out[i] = interval * float64(i+1)
	}
	return out
}

// ExtractFrames samples n JPEG keyframes into outDir as frame_000.jpg … and returns
// their paths plus the probed duration (0 if unknown). Frames that fail are skipped.
func ExtractFrames(ctx context.Context, bins media.FFmpegBins, videoPath string, n int, outDir string, progress func(int, string)) ([]string, float64, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	if bins.FFmpeg == "" {
		bin, err := media.ResolveFFmpeg("", "")
		if err != nil {
			return nil, 0, fmt.Errorf("未找到 ffmpeg: %w", err)
		}
		bins.FFmpeg = bin
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, 0, err
	}
	dur, _ := media.ProbeDurationSec(ctx, bins, videoPath)
	q := fmt.Sprintf("%d", (100-frameQuality)/10+1)

	var frames []string
	stamps := FrameTimestamps(dur, n)
	for i, ts := range stamps {
		if err := ctx.Err(); err != nil {
			return nil, dur, err
		}
		out := filepath.Join(outDir, fmt.Sprintf("frame_%03d.jpg", i))
		_, err := media.Run(ctx, bins.FFmpeg,
			"-y",
			"-ss", fmt.Sprintf("%.3f", ts),
			"-i", videoPath,
			"-vframes", "1",
			"-q:v", q,
			out,
		)
		if err != nil {
			progress(10+i*30/len(stamps), fmt.Sprintf("帧 %d 提取失败: %v", i+1, err))
			continue
		}
		if st, err := os.Stat(out); err == nil && st.Size() > 0 {
			frames = append(frames, out)
			progress(10+(i+1)*30/len(stamps), fmt.Sprintf("提取帧 %d/%d @ %.1fs", i+1, len(stamps), ts))
		}
	}
	if len(frames) == 0 {
		return nil, dur, fmt.Errorf("无法提取视频帧，请确保已安装 ffmpeg")
	}
	return frames, dur, nil
}
