package original

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/narratoai/narrato-go/internal/media"
)

// SubtitleStyle controls ASS force_style when burning an external SRT.
type SubtitleStyle struct {
	FontSize     int
	FontName     string
	PrimaryColor string // #RRGGBB
	OutlineColor string
	Outline      int
	Shadow       int
	MarginV      int
}

// DefaultSubtitleStyle matches LocalMiniDrama defaults.
func DefaultSubtitleStyle() SubtitleStyle {
	return SubtitleStyle{
		FontSize:     22,
		FontName:     "Arial",
		PrimaryColor: "#FFFFFF",
		OutlineColor: "#000000",
		Outline:      2,
		Shadow:       1,
		MarginV:      36,
	}
}

type flyingObj struct {
	name, char, color string
	size              int
}

var flyingObjects = []flyingObj{
	{"蒲公英", "○", "white", 220},
	{"小鸟", "◆", "sienna", 200},
	{"蝴蝶", "★", "hotpink", 210},
	{"树叶", "♠", "limegreen", 200},
	{"樱花", "●", "lightpink", 220},
	{"星星", "★", "gold", 200},
	{"雪花", "☆", "lavender", 210},
	{"气泡", "○", "lightskyblue", 240},
	{"萤火虫", "●", "yellow", 180},
	{"花瓣", "♥", "deeppink", 200},
}

// PreviewSecondsDefault is used when PreviewSeconds is unset but Preview is requested.
const PreviewSecondsDefault = 5.0

// Run applies selected transforms via ffmpeg and writes OutputPath (full export).
func Run(ctx context.Context, p Params) (string, error) {
	p.PreviewSeconds = 0
	return run(ctx, p)
}

// Preview applies the same filter chain but only the first N seconds, then returns the path.
func Preview(ctx context.Context, p Params) (string, error) {
	if p.PreviewSeconds <= 0 {
		p.PreviewSeconds = PreviewSecondsDefault
	}
	// Skip BGM on preview for speed; visual/audio filters still apply.
	p.BGMPath = ""
	if p.OutputPath == "" {
		dir := filepath.Join(os.TempDir(), "narrato_original_preview")
		_ = os.MkdirAll(dir, 0o755)
		p.OutputPath = filepath.Join(dir, fmt.Sprintf("preview_%d.mp4", time.Now().UnixNano()))
	}
	return run(ctx, p)
}

func run(ctx context.Context, p Params) (string, error) {
	if err := ValidateParams(p); err != nil {
		return "", err
	}
	if _, err := os.Stat(p.VideoPath); err != nil {
		return "", fmt.Errorf("视频不存在: %w", err)
	}

	settings, notice := NormalizeSettings(p.Settings)
	p.Settings = settings

	ffmpeg := p.Bins.FFmpeg
	if ffmpeg == "" {
		var err error
		ffmpeg, err = media.ResolveFFmpeg("", "")
		if err != nil {
			return "", err
		}
		p.Bins.FFmpeg = ffmpeg
	}
	if p.Bins.FFprobe == "" {
		if fp, err := media.ResolveFFprobe("", ""); err == nil {
			p.Bins.FFprobe = fp
		}
	}

	out := p.OutputPath
	if out == "" {
		dir := filepath.Join(filepath.Dir(p.VideoPath), "original_"+time.Now().Format("20060102_150405"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		base := strings.TrimSuffix(filepath.Base(p.VideoPath), filepath.Ext(p.VideoPath))
		out = filepath.Join(dir, base+"_original.mp4")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}

	progress := p.OnProgress
	if progress == nil {
		progress = func(int, string) {}
	}
	if notice != "" {
		progress(5, notice)
	}
	progress(10, "正在分析视频…")

	dur, _ := media.ProbeDurationSec(ctx, p.Bins, p.VideoPath)
	chain := BuildFilters(FilterInput{
		Settings:        p.Settings,
		SubtitleSRT:     p.SubtitleSRT,
		Style:           p.Style,
		DurationSec:     dur,
		WatermarkText:   p.WatermarkText,
		WatermarkCorner: p.WatermarkCorner,
	})

	hasBGM := strings.TrimSpace(p.BGMPath) != ""
	if hasBGM {
		if _, err := os.Stat(p.BGMPath); err != nil {
			hasBGM = false
		}
	}

	tempPath := out
	if hasBGM {
		tempPath = filepath.Join(filepath.Dir(out), "_pre_bgm.mp4")
	}

	progress(15, "正在处理: "+chain.Summary+"…")

	args := []string{"-i", p.VideoPath}
	if p.PreviewSeconds > 0 {
		args = append(args, "-t", fmt.Sprintf("%g", p.PreviewSeconds))
	}
	if len(chain.Video) > 0 {
		args = append(args, "-vf", chain.VFJoin)
		args = append(args, "-c:v", "libx264", "-preset", "fast", "-crf", "23")
	} else {
		args = append(args, "-c:v", "copy")
	}
	if len(chain.Audio) > 0 {
		args = append(args, "-af", chain.AFJoin)
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	} else {
		args = append(args, "-c:a", "copy")
	}
	args = append(args, "-y", tempPath)

	if _, err := media.Run(ctx, p.Bins.FFmpeg, args...); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("ffmpeg 处理失败: %w", err)
	}
	if _, err := os.Stat(tempPath); err != nil {
		return "", fmt.Errorf("处理后视频文件不存在")
	}

	if hasBGM {
		progress(85, "正在混入背景音乐…")
		if err := mixBGM(ctx, p.Bins, tempPath, p.BGMPath, p.BGMVolume, out); err != nil {
			_ = os.Remove(tempPath)
			return "", err
		}
		_ = os.Remove(tempPath)
	}

	progress(100, "完成")
	return out, nil
}

// DescribeFilters builds the learning panel text for current settings.
func DescribeFilters(p Params) FilterChain {
	dur := 0.0
	return BuildFilters(FilterInput{
		Settings:        p.Settings,
		SubtitleSRT:     p.SubtitleSRT,
		Style:           p.Style,
		DurationSec:     dur,
		WatermarkText:   p.WatermarkText,
		WatermarkCorner: p.WatermarkCorner,
	})
}

func normalizeBGMVolume(v float64) float64 {
	if v != v { // NaN
		return 0.25
	}
	if v > 1 {
		if v > 100 {
			v = 100
		}
		if v < 0 {
			v = 0
		}
		return v / 100
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mixBGM(ctx context.Context, bins media.FFmpegBins, videoPath, bgmPath string, volume float64, outputPath string) error {
	dur, _ := media.ProbeDurationSec(ctx, bins, videoPath)
	vol := normalizeBGMVolume(volume)
	trimDur := dur
	if trimDur <= 0 {
		trimDur = 86400
	}
	fc := fmt.Sprintf(
		"[1:a]aloop=loop=-1:size=2147483647,atrim=duration=%g,asetpts=N/SR/TB,volume=%g[bg];[0:a][bg]amix=inputs=2:duration=first:normalize=0[aout]",
		trimDur, vol,
	)
	args := []string{
		"-i", videoPath, "-i", bgmPath,
		"-filter_complex", fc,
		"-map", "0:v:0", "-map", "[aout]",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
	}
	if dur > 0 {
		args = append(args, "-t", fmt.Sprintf("%g", dur))
	}
	args = append(args, "-y", outputPath)
	if _, err := media.Run(ctx, bins.FFmpeg, args...); err != nil {
		return fmt.Errorf("混入背景音乐失败: %w", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("混入背景音乐后文件不存在")
	}
	return nil
}
