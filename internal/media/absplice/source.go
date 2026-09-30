package absplice

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
)

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".bmp": true,
	".webp": true, ".tif": true, ".tiff": true, ".gif": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".mkv": true, ".avi": true,
	".m4v": true, ".webm": true, ".ts": true, ".flv": true, ".wmv": true,
}

// IsImagePath reports whether path looks like a still image.
func IsImagePath(path string) bool {
	return imageExts[strings.ToLower(filepath.Ext(path))]
}

// IsVideoPath reports whether path looks like a video container.
func IsVideoPath(path string) bool {
	return videoExts[strings.ToLower(filepath.Ext(path))]
}

// SuggestIDRCount returns how many A-IDRs to inject.
// manual > 0 wins; otherwise ~1 per second of splice (clamped 1..60).
// Images always resolve to 1 at call sites that pass forceOne.
func SuggestIDRCount(spliceDurSec float64, manual int, perSec float64) int {
	if manual > 0 {
		if manual > 60 {
			return 60
		}
		return manual
	}
	if perSec <= 0 {
		perSec = 1
	}
	if spliceDurSec <= 0 {
		return 1
	}
	n := int(math.Ceil(spliceDurSec * perSec))
	if n < 1 {
		n = 1
	}
	if n > 60 {
		n = 60
	}
	return n
}

// EvenSampleTimes returns N timestamps in [0, duration).
func EvenSampleTimes(durationSec float64, n int) []float64 {
	if n < 1 {
		n = 1
	}
	if durationSec < 0 {
		durationSec = 0
	}
	out := make([]float64, n)
	if n == 1 {
		out[0] = 0
		if durationSec > 0.04 {
			out[0] = durationSec * 0.5
		}
		return out
	}
	span := durationSec
	if span < 0.001 {
		span = 0.001
	}
	for i := 0; i < n; i++ {
		// centers of N equal buckets
		out[i] = (float64(i) + 0.5) * span / float64(n)
		if out[i] >= durationSec && durationSec > 0 {
			out[i] = math.Max(0, durationSec-0.04)
		}
	}
	return out
}

// SegmentWindows splits [start, start+totalDur) into n contiguous windows.
func SegmentWindows(start, totalDur float64, n int) [][2]float64 {
	if n < 1 {
		n = 1
	}
	if totalDur <= 0 {
		totalDur = 0.04
	}
	out := make([][2]float64, n)
	base := totalDur / float64(n)
	acc := start
	for i := 0; i < n; i++ {
		d := base
		if i == n-1 {
			d = (start + totalDur) - acc
		}
		if d < 0.04 {
			d = 0.04
		}
		out[i] = [2]float64{acc, d}
		acc += d
	}
	return out
}

// ProbeMediaDurationSec returns duration; images return 0.
func ProbeMediaDurationSec(ctx context.Context, bins media.FFmpegBins, path string) (float64, error) {
	if IsImagePath(path) {
		return 0, nil
	}
	return media.ProbeDurationSec(ctx, bins, path)
}

// EncodeIDRFromMedia encodes one IDR from an image, or one frame of a video at atSec.
func EncodeIDRFromMedia(ctx context.Context, cfg EncodeConfig, mediaPath string, atSec float64, outH264 string) error {
	if cfg.FFmpegBins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	if err := EnsureDir(outH264); err != nil {
		return err
	}
	if IsImagePath(mediaPath) {
		return EncodeIFrame(ctx, cfg, mediaPath, outH264)
	}
	base := cfg.x264ArgsBase()
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", atSec),
		"-i", mediaPath,
		"-an",
		"-frames:v", "1",
	}
	args = append(args, base...)
	args = append(args,
		"-g", "1", "-keyint_min", "1",
		"-x264-params", "keyint=1:min-keyint=1:scenecut=0:open-gop=0:aud=1:bframes=0:ref=3",
		"-forced-idr", "1",
		"-bsf:v", "h264_mp4toannexb",
		"-f", "h264",
		outH264,
	)
	out, err := media.Run(ctx, cfg.FFmpegBins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("encode IDR from media at %.3fs failed: %w, output: %s", atSec, err, out)
	}
	return nil
}

// EnsureSegmentGOP bumps cfg.GOPLength so keyint (=GOP*2) covers segDur at fps.
func EnsureSegmentGOP(cfg *EncodeConfig, segDurSec float64) {
	if cfg == nil {
		return
	}
	fps := cfg.FPS
	if fps <= 0 {
		fps = 30
	}
	needFrames := int(math.Ceil(segDurSec*float64(fps))) + 2
	// keyint = GOPLength*2
	needGOP := (needFrames + 1) / 2
	if needGOP < 30 {
		needGOP = 30
	}
	if cfg.GOPLength < needGOP {
		cfg.GOPLength = needGOP
	}
}
