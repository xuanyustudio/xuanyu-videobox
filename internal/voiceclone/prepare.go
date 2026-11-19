package voiceclone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/narratoai/narrato-go/internal/media"
)

const (
	MinDurationSec = 10.0
	MaxDurationSec = 5 * 60.0
	MaxFileBytes   = 20 << 20
)

// PreparedAudio is a MiniMax-ready reference clip next to the source file.
type PreparedAudio struct {
	Path      string
	Duration  float64
	Bytes     int64
	Trimmed   bool
	Converted bool
	OutputDir string
}

var cloneExts = map[string]bool{
	".mp3": true,
	".m4a": true,
	".wav": true,
}

// OutputDir is the folder next to the source file for one clone session.
func OutputDir(src string, now time.Time) string {
	return filepath.Join(filepath.Dir(src), "voiceclone_"+now.Format("20060102_150405"))
}

// PrepareAudio validates (and if needed converts or trims) a reference file
// into an mp3/m4a/wav clip that MiniMax will accept.
func PrepareAudio(ctx context.Context, bins media.FFmpegBins, src, outDir string) (*PreparedAudio, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("请先选择参考音或视频")
	}
	if _, err := os.Stat(src); err != nil {
		return nil, fmt.Errorf("找不到参考文件: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	dur, err := media.ProbeDurationSec(ctx, bins, src)
	if err != nil {
		return nil, fmt.Errorf("无法读取时长，请确认已配置 ffmpeg/ffprobe: %w", err)
	}
	if dur+0.05 < MinDurationSec {
		return nil, fmt.Errorf("参考音至少需要 %.0f 秒，当前约 %.1f 秒", MinDurationSec, dur)
	}

	ext := strings.ToLower(filepath.Ext(src))
	st, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	out := &PreparedAudio{Duration: dur, OutputDir: outDir}
	needTrim := dur > MaxDurationSec+0.05
	needConvert := !cloneExts[ext] || st.Size() > MaxFileBytes || needTrim

	if !needConvert {
		out.Path = src
		out.Bytes = st.Size()
		if out.Bytes > MaxFileBytes {
			return nil, fmt.Errorf("参考音不能超过 20 MB")
		}
		return out, nil
	}
	if strings.TrimSpace(bins.FFmpeg) == "" {
		return nil, fmt.Errorf("需要把参考音转成 mp3，请先在「设置」里填写 ffmpeg 路径")
	}

	dst := filepath.Join(outDir, "source.mp3")
	maxSec := 0.0
	if needTrim {
		maxSec = MaxDurationSec
		out.Trimmed = true
		out.Duration = MaxDurationSec
	}
	if err := convertMP3(ctx, bins.FFmpeg, src, dst, maxSec); err != nil {
		return nil, fmt.Errorf("转换参考音失败: %w", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("转换后的参考音仍超过 20 MB，请换一段更短或码率更低的音频")
	}
	if !needTrim {
		if d, err := media.ProbeDurationSec(ctx, bins, dst); err == nil {
			out.Duration = d
		}
	}
	out.Path = dst
	out.Bytes = info.Size()
	out.Converted = true
	return out, nil
}

func convertMP3(ctx context.Context, ffmpeg, src, dst string, maxSec float64) error {
	args := []string{"-y", "-i", src, "-vn", "-ac", "1", "-ar", "32000", "-b:a", "128k"}
	if maxSec > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", maxSec))
	}
	args = append(args, dst)
	_, err := media.Run(ctx, ffmpeg, args...)
	return err
}
