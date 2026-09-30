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

func (c EncodeConfig) x264ArgsBase() []string {
	gop := c.GOPLength
	if gop <= 0 {
		gop = 30
	}
	// bf=0：无 B 帧，frame_num/POC 重写更简单可靠。
	return []string{
		"-c:v", "libx264",
		"-preset", c.Preset,
		"-crf", fmt.Sprintf("%d", c.CRF),
		"-pix_fmt", "yuv420p",
		"-r", fmt.Sprintf("%d", c.FPS),
		"-s", fmt.Sprintf("%dx%d", c.Width, c.Height),
		"-bf", "0",
		"-g", fmt.Sprintf("%d", gop*2),
		"-keyint_min", fmt.Sprintf("%d", gop*2),
		"-sc_threshold", "0",
		"-x264-params", fmt.Sprintf(
			"keyint=%d:min-keyint=%d:scenecut=0:intra-refresh=0:no-scenecut=1:open-gop=0:aud=1:bframes=0:ref=3",
			gop*2, gop*2,
		),
		"-forced-idr", "0",
		"-color_range", "1",
		"-colorspace", "1",
		"-color_trc", "1",
		"-color_primaries", "1",
	}
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
	if err := EnsureDir(outH264); err != nil {
		return err
	}
	base := cfg.x264ArgsBase()
	args := []string{"-y", "-loop", "1", "-framerate", fmt.Sprintf("%d", cfg.FPS), "-t", fmt.Sprintf("%.6f", 1.0/float64(cfg.FPS)), "-i", imagePath, "-an"}
	args = append(args, base...)
	args = append(args,
		"-g", "1", "-keyint_min", "1",
		"-x264-params", "keyint=1:min-keyint=1:scenecut=0:open-gop=0:aud=1:bframes=0:ref=3",
		"-forced-idr", "1",
		"-frames:v", "1",
		"-bsf:v", "h264_mp4toannexb",
		"-f", "h264",
		outH264,
	)
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
	if err := EnsureDir(outH264); err != nil {
		return err
	}
	base := cfg.x264ArgsBase()
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", startTime),
		"-t", fmt.Sprintf("%.3f", duration),
		"-i", videoPath,
		"-an",
	}
	args = append(args, base...)
	args = append(args,
		"-forced-idr", "0",
		"-bsf:v", "h264_mp4toannexb",
		"-f", "h264",
		outH264,
	)
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
	if err := EnsureDir(outAudio); err != nil {
		return err
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
