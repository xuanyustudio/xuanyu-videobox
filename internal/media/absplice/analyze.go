package absplice

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
)

// KeyFrameInfo is one key frame (usually IDR in closed-GOP streams).
type KeyFrameInfo struct {
	Index    int
	TimeSec  float64
	PictType string
}

// VideoAnalysis summarises H.264 frame / GOP structure via ffprobe.
type VideoAnalysis struct {
	Path         string
	Codec        string
	Width        int
	Height       int
	FPS          float64
	DurationSec  float64
	FrameCount   int
	IFrames      int
	PFrames      int
	BFrames      int
	KeyFrames    int // ffprobe key_frame=1；封闭 GOP 下通常等于 IDR 数
	GOPCount     int
	GOPMinFrames int
	GOPMaxFrames int
	GOPAvgFrames float64
	KeyFramesAt  []KeyFrameInfo // 前若干关键帧时间戳
}

// AnalyzeVideo probes stream metadata and per-frame pict_type / key_frame.
// Long videos may take a while because every frame is scanned.
func AnalyzeVideo(ctx context.Context, bins media.FFmpegBins, videoPath string) (*VideoAnalysis, error) {
	if strings.TrimSpace(videoPath) == "" {
		return nil, fmt.Errorf("video path required")
	}
	ffprobe := bins.FFprobe
	if ffprobe == "" {
		var err error
		ffprobe, err = media.ResolveFFprobe("", "")
		if err != nil {
			return nil, fmt.Errorf("ffprobe required: %w", err)
		}
	}

	metaOut, err := media.Run(ctx, ffprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,avg_frame_rate,nb_frames,duration",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1",
		videoPath,
	)
	if err != nil {
		return nil, fmt.Errorf("ffprobe stream: %w\n%s", err, metaOut)
	}

	a := &VideoAnalysis{Path: videoPath}
	for _, line := range strings.Split(metaOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		switch k {
		case "codec_name":
			a.Codec = v
		case "width":
			a.Width, _ = strconv.Atoi(v)
		case "height":
			a.Height, _ = strconv.Atoi(v)
		case "avg_frame_rate":
			a.FPS = parseFrameRate(v)
		case "nb_frames":
			if n, e := strconv.Atoi(v); e == nil && n > 0 {
				a.FrameCount = n
			}
		case "duration":
			if d, e := strconv.ParseFloat(v, 64); e == nil && d > 0 {
				// prefer stream duration; format duration overwrites only if stream empty
				if a.DurationSec == 0 {
					a.DurationSec = d
				} else if d > a.DurationSec {
					// keep stream if already set; format often same
					_ = d
				}
			}
		}
	}
	// format duration may appear twice; re-parse preferring larger sensible value already set
	if a.DurationSec == 0 {
		for _, line := range strings.Split(metaOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "duration=") {
				if d, e := strconv.ParseFloat(strings.TrimPrefix(line, "duration="), 64); e == nil {
					a.DurationSec = d
				}
			}
		}
	}

	framesOut, err := media.Run(ctx, ffprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_frames",
		"-show_entries", "frame=pict_type,key_frame,pts_time",
		"-of", "csv=p=0",
		videoPath,
	)
	if err != nil {
		return nil, fmt.Errorf("ffprobe frames: %w\n%s", err, framesOut)
	}

	var (
		idx          int
		keyIndexes   []int
		keyInfos     []KeyFrameInfo
		maxKeyList   = 40
	)
	for _, line := range strings.Split(framesOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// csv order follows show_entries: pict_type,key_frame,pts_time
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		pict := parts[0]
		key := parts[1]
		pts := 0.0
		if len(parts) >= 3 {
			pts, _ = strconv.ParseFloat(parts[2], 64)
		}
		// Some ffprobe builds reorder; detect if first field is 0/1
		if pict == "0" || pict == "1" {
			key = parts[0]
			if len(parts) >= 3 {
				pts, _ = strconv.ParseFloat(parts[1], 64)
				pict = parts[2]
			} else if len(parts) == 2 {
				pict = parts[1]
			}
		}
		switch pict {
		case "I":
			a.IFrames++
		case "P":
			a.PFrames++
		case "B":
			a.BFrames++
		}
		if key == "1" {
			a.KeyFrames++
			keyIndexes = append(keyIndexes, idx)
			if len(keyInfos) < maxKeyList {
				keyInfos = append(keyInfos, KeyFrameInfo{Index: idx, TimeSec: pts, PictType: pict})
			}
		}
		idx++
	}
	a.FrameCount = idx
	a.KeyFramesAt = keyInfos

	if len(keyIndexes) == 0 {
		a.GOPCount = 0
	} else {
		a.GOPCount = len(keyIndexes)
		a.GOPMinFrames = math.MaxInt32
		sum := 0
		for i, start := range keyIndexes {
			end := a.FrameCount
			if i+1 < len(keyIndexes) {
				end = keyIndexes[i+1]
			}
			g := end - start
			if g < 0 {
				g = 0
			}
			sum += g
			if g < a.GOPMinFrames {
				a.GOPMinFrames = g
			}
			if g > a.GOPMaxFrames {
				a.GOPMaxFrames = g
			}
		}
		if a.GOPMinFrames == math.MaxInt32 {
			a.GOPMinFrames = 0
		}
		a.GOPAvgFrames = float64(sum) / float64(len(keyIndexes))
	}

	if a.FPS <= 0 && a.DurationSec > 0 && a.FrameCount > 0 {
		a.FPS = float64(a.FrameCount) / a.DurationSec
	}
	return a, nil
}

func parseFrameRate(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0/0" || s == "N/A" {
		return 0
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		num, e1 := strconv.ParseFloat(parts[0], 64)
		den, e2 := strconv.ParseFloat(parts[1], 64)
		if e1 == nil && e2 == nil && den != 0 {
			return num / den
		}
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// FormatReport returns a human-readable multi-line report for dialogs.
func (a *VideoAnalysis) FormatReport(spliceDurSec float64, outFPS int) string {
	if a == nil {
		return "(无数据)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "文件：%s\n", filepath.Base(a.Path))
	fmt.Fprintf(&b, "路径：%s\n\n", a.Path)

	fmt.Fprintf(&b, "【基本信息】\n")
	fmt.Fprintf(&b, "  编码：%s\n", emptyDash(a.Codec))
	fmt.Fprintf(&b, "  分辨率：%d × %d\n", a.Width, a.Height)
	fmt.Fprintf(&b, "  帧率：%.3f fps\n", a.FPS)
	fmt.Fprintf(&b, "  时长：%.3f 秒\n", a.DurationSec)
	fmt.Fprintf(&b, "  总帧数：%d\n\n", a.FrameCount)

	fmt.Fprintf(&b, "【帧类型】\n")
	fmt.Fprintf(&b, "  I 帧：%d\n", a.IFrames)
	fmt.Fprintf(&b, "  P 帧：%d\n", a.PFrames)
	fmt.Fprintf(&b, "  B 帧：%d\n", a.BFrames)
	fmt.Fprintf(&b, "  关键帧(key_frame，封闭 GOP ≈ IDR)：%d\n\n", a.KeyFrames)

	fmt.Fprintf(&b, "【GOP】\n")
	fmt.Fprintf(&b, "  GOP 数量：%d\n", a.GOPCount)
	if a.GOPCount > 0 {
		fmt.Fprintf(&b, "  GOP 长度：最小 %d / 平均 %.1f / 最大 %d（帧）\n", a.GOPMinFrames, a.GOPAvgFrames, a.GOPMaxFrames)
		if a.FPS > 0 {
			fmt.Fprintf(&b, "  约合秒数：最小 %.2fs / 平均 %.2fs / 最大 %.2fs\n",
				float64(a.GOPMinFrames)/a.FPS, a.GOPAvgFrames/a.FPS, float64(a.GOPMaxFrames)/a.FPS)
		}
	}
	b.WriteByte('\n')

	if len(a.KeyFramesAt) > 0 {
		fmt.Fprintf(&b, "【关键帧时间戳】（最多列出 %d 个）\n", len(a.KeyFramesAt))
		for _, k := range a.KeyFramesAt {
			fmt.Fprintf(&b, "  #%d  t=%.3fs  type=%s\n", k.Index, k.TimeSec, emptyDash(k.PictType))
		}
		if a.KeyFrames > len(a.KeyFramesAt) {
			fmt.Fprintf(&b, "  … 其余 %d 个省略\n", a.KeyFrames-len(a.KeyFramesAt))
		}
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "【AB 拼接建议】\n")
	fps := float64(outFPS)
	if fps <= 0 {
		fps = a.FPS
	}
	if fps <= 0 {
		fps = 30
	}
	if spliceDurSec <= 0 {
		spliceDurSec = 1
	}
	needFrames := int(math.Ceil(spliceDurSec * fps))
	// UI GOP → keyint = GOP*2
	suggestGOP := int(math.Ceil(float64(needFrames) / 2.0))
	if suggestGOP < 30 {
		suggestGOP = 30
	}
	fmt.Fprintf(&b, "  若拼接持续 %.1fs @ %.0ffps，约需覆盖 %d 帧，且 B 段最好只有 1 个 IDR。\n", spliceDurSec, fps, needFrames)
	fmt.Fprintf(&b, "  建议界面「GOP 长度」≥ %d（内部 keyint=GOP×2≥%d）。\n", suggestGOP, suggestGOP*2)
	fmt.Fprintf(&b, "  更稳妥可直接填 %d 或 999。\n", needFrames)

	return b.String()
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
