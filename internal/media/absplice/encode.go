package absplice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/narratoai/narrato-go/internal/media"
)

type EncodeConfig struct {
	FFmpegBins media.FFmpegBins
	Width      int
	Height     int
	FPS        int
	CRF        int
	Preset     string
	GOPLength  int
}

func DefaultEncodeConfig(bins media.FFmpegBins) EncodeConfig {
	return EncodeConfig{
		FFmpegBins: bins,
		Width:      1920,
		Height:     1080,
		FPS:        30,
		CRF:        23,
		Preset:     "medium",
		GOPLength:  30,
	}
}

func EncodeIFrame(ctx context.Context, cfg EncodeConfig, imagePath string, outH264 string) error {
	if cfg.FFmpegBins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	args := []string{
		"-y",
		"-loop", "1",
		"-t", "0.04",
		"-i", imagePath,
		"-an",
		"-c:v", "libx264",
		"-preset", cfg.Preset,
		"-crf", fmt.Sprintf("%d", cfg.CRF),
		"-pix_fmt", "yuv420p",
		"-r", fmt.Sprintf("%d", cfg.FPS),
		"-s", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
		"-g", "1",
		"-keyint_min", "1",
		"-sc_threshold", "0",
		"-x264-params", "keyint=1:min-keyint=1:scenecut=0:no-cabac=0",
		"-frames:v", "1",
		"-bsf:v", "h264_mp4toannexb",
		outH264,
	}
	out, err := media.Run(ctx, cfg.FFmpegBins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("encode I-frame failed: %w, output: %s", err, out)
	}
	return nil
}

func EncodePBFrames(ctx context.Context, cfg EncodeConfig, videoPath string, startTime float64, duration float64, outH264 string) error {
	if cfg.FFmpegBins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	gop := cfg.GOPLength
	if gop <= 0 {
		gop = 30
	}
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", startTime),
		"-t", fmt.Sprintf("%.3f", duration),
		"-i", videoPath,
		"-an",
		"-c:v", "libx264",
		"-preset", cfg.Preset,
		"-crf", fmt.Sprintf("%d", cfg.CRF),
		"-pix_fmt", "yuv420p",
		"-r", fmt.Sprintf("%d", cfg.FPS),
		"-s", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
		"-g", fmt.Sprintf("%d", gop*2),
		"-keyint_min", fmt.Sprintf("%d", gop*2),
		"-sc_threshold", "0",
		"-x264-params", fmt.Sprintf("keyint=%d:min-keyint=%d:scenecut=0:intra-refresh=0:no-scenecut=1", gop*2, gop*2),
		"-forced-idr", "0",
		"-bsf:v", "h264_mp4toannexb",
		outH264,
	}
	out, err := media.Run(ctx, cfg.FFmpegBins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("encode P/B frames failed: %w, output: %s", err, out)
	}
	return nil
}

func ExtractAudio(ctx context.Context, bins media.FFmpegBins, videoPath string, startTime float64, duration float64, outAudio string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", startTime),
		"-t", fmt.Sprintf("%.3f", duration),
		"-i", videoPath,
		"-vn",
		"-c:a", "aac",
		"-ar", "44100",
		"-ac", "2",
		"-b:a", "192k",
		outAudio,
	}
	out, err := media.Run(ctx, bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("extract audio failed: %w, output: %s", err, out)
	}
	return nil
}

func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	return nil
}
