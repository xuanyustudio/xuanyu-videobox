package hookopen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
)

type videoInfo struct {
	Duration float64
	Width    int
	Height   int
	FPS      string
	HasAudio bool
}

func resolveBins(cfg *config.Config) media.FFmpegBins {
	beside := ""
	if exe, err := os.Executable(); err == nil {
		beside = filepath.Dir(exe)
	}
	var path, probe string
	if cfg != nil {
		path = cfg.FFmpeg.Path
		probe = cfg.FFmpeg.FFprobePath
	}
	ff, _ := media.ResolveFFmpeg(path, beside)
	fp, _ := media.ResolveFFprobe(probe, beside)
	return media.FFmpegBins{FFmpeg: ff, FFprobe: fp}
}

func probeVideo(ctx context.Context, bins media.FFmpegBins, path string) (videoInfo, error) {
	var info videoInfo
	dur, err := media.ProbeDurationSec(ctx, bins, path)
	if err != nil {
		return info, err
	}
	info.Duration = dur
	info.FPS = "30"
	info.Width, info.Height = 1080, 1920
	if bins.FFprobe == "" {
		return info, nil
	}
	out, err := media.Run(ctx, bins.FFprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate",
		"-of", "csv=p=0",
		path,
	)
	if err == nil {
		parts := strings.Split(strings.TrimSpace(strings.Split(out, "\n")[0]), ",")
		if len(parts) >= 2 {
			if w, e := strconv.Atoi(strings.TrimSpace(parts[0])); e == nil && w > 0 {
				info.Width = even(w)
			}
			if h, e := strconv.Atoi(strings.TrimSpace(parts[1])); e == nil && h > 0 {
				info.Height = even(h)
			}
		}
		if len(parts) >= 3 {
			if fps := normalizeFPS(parts[2]); fps != "" {
				info.FPS = fps
			}
		}
	}
	aout, aerr := media.Run(ctx, bins.FFprobe,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		path,
	)
	if aerr == nil && strings.Contains(strings.ToLower(aout), "audio") {
		info.HasAudio = true
	}
	return info, nil
}

func even(n int) int {
	if n%2 != 0 {
		return n - 1
	}
	return n
}

func normalizeFPS(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0/0" {
		return ""
	}
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		if len(parts) == 2 {
			a, e1 := strconv.ParseFloat(parts[0], 64)
			b, e2 := strconv.ParseFloat(parts[1], 64)
			if e1 == nil && e2 == nil && b != 0 {
				v := a / b
				if v >= 23.5 && v <= 24.5 {
					return "24"
				}
				if v >= 24.5 && v <= 25.5 {
					return "25"
				}
				if v >= 29.5 && v <= 30.5 {
					return "30"
				}
				return strconv.FormatFloat(v, 'f', 3, 64)
			}
		}
	}
	return raw
}

func extractWAV(ctx context.Context, bins media.FFmpegBins, videoPath, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("未找到 ffmpeg")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	log, err := media.Run(ctx, bins.FFmpeg,
		"-y", "-i", videoPath,
		"-vn", "-ac", "1", "-ar", "16000",
		"-c:a", "pcm_s16le",
		outPath,
	)
	if err != nil {
		low := strings.ToLower(log)
		if strings.Contains(low, "matches no streams") || strings.Contains(low, "does not contain any stream") {
			return fmt.Errorf("原片没有音轨")
		}
		return fmt.Errorf("抽出音轨失败: %s", strings.TrimSpace(log))
	}
	return nil
}

func extractFrame(ctx context.Context, bins media.FFmpegBins, videoPath string, at float64, outPath string) error {
	if at < 0 {
		at = 0
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	_, err := media.Run(ctx, bins.FFmpeg,
		"-y",
		"-ss", fmt.Sprintf("%.3f", at),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "3",
		outPath,
	)
	return err
}

func cutPreview(ctx context.Context, bins media.FFmpegBins, videoPath string, start, end float64, outPath string) error {
	dur := end - start
	if dur <= 0 {
		return fmt.Errorf("片段时长无效")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	_, err := media.Run(ctx, bins.FFmpeg,
		"-y",
		"-ss", fmt.Sprintf("%.3f", start),
		"-i", videoPath,
		"-t", fmt.Sprintf("%.3f", dur),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-pix_fmt", "yuv420p", "-profile:v", "high",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		"-avoid_negative_ts", "make_zero",
		outPath,
	)
	if err != nil {
		// retry without audio
		_, err = media.Run(ctx, bins.FFmpeg,
			"-y",
			"-ss", fmt.Sprintf("%.3f", start),
			"-i", videoPath,
			"-t", fmt.Sprintf("%.3f", dur),
			"-an",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
			"-pix_fmt", "yuv420p", "-profile:v", "high",
			"-movflags", "+faststart",
			outPath,
		)
	}
	return err
}

func detectScenes(ctx context.Context, bins media.FFmpegBins, videoPath string, duration float64) []float64 {
	if bins.FFmpeg == "" {
		return nil
	}
	// downscale for speed; print scene scores
	log, _ := media.Run(ctx, bins.FFmpeg,
		"-hide_banner",
		"-i", videoPath,
		"-an",
		"-vf", "scale=320:-2,select='gt(scene,0.28)',showinfo",
		"-vsync", "vfr",
		"-f", "null",
		"-",
	)
	var times []float64
	for _, line := range strings.Split(log, "\n") {
		if !strings.Contains(line, "pts_time:") {
			continue
		}
		i := strings.Index(line, "pts_time:")
		rest := line[i+len("pts_time:"):]
		field := strings.FieldsFunc(rest, func(r rune) bool {
			return r == ' ' || r == ':'
		})
		if len(field) == 0 {
			continue
		}
		t, err := strconv.ParseFloat(strings.TrimSpace(field[0]), 64)
		if err != nil || t <= 0 || (duration > 0 && t >= duration) {
			continue
		}
		if len(times) > 0 && t-times[len(times)-1] < 1.2 {
			continue
		}
		times = append(times, t)
		if len(times) >= 16 {
			break
		}
	}
	return times
}

func workDirFor(videoPath string) string {
	dir := filepath.Dir(videoPath)
	stem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	if stem == "" || stem == "." {
		stem = "video"
	}
	return filepath.Join(dir, stem+"_爆点开头")
}

func defaultOutputPath(videoPath string) string {
	dir := filepath.Dir(videoPath)
	stem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	if stem == "" || stem == "." {
		stem = "video"
	}
	return filepath.Join(dir, stem+"_爆点开头.mp4")
}

func openPath(path string) error {
	return media.OpenFile(path)
}

func ffmpegExists(bins media.FFmpegBins) error {
	if strings.TrimSpace(bins.FFmpeg) == "" {
		return fmt.Errorf("未找到 ffmpeg，请在「设置」里填写路径")
	}
	if _, err := exec.LookPath(bins.FFmpeg); err != nil {
		if _, err2 := os.Stat(bins.FFmpeg); err2 != nil {
			return fmt.Errorf("未找到 ffmpeg，请在「设置」里填写路径")
		}
	}
	return nil
}
