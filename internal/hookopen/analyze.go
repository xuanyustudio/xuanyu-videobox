package hookopen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/narratoai/narrato-go/internal/config"
)

// Analyze studies a video and returns 3 hook opening candidates.
func Analyze(ctx context.Context, cfg *config.Config, videoPath string, progress Progress) (*Result, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	videoPath = strings.TrimSpace(videoPath)
	if videoPath == "" {
		return nil, fmt.Errorf("请先选择一条视频")
	}
	if _, err := os.Stat(videoPath); err != nil {
		return nil, fmt.Errorf("找不到视频: %w", err)
	}
	bins := resolveBins(cfg)
	if err := ffmpegExists(bins); err != nil {
		return nil, err
	}
	progress(5, "正在读取视频信息…")
	info, err := probeVideo(ctx, bins, videoPath)
	if err != nil {
		return nil, fmt.Errorf("读取视频失败: %w", err)
	}
	if info.Duration < MinHookSec+1 {
		return nil, fmt.Errorf("视频太短，至少需要约 %.0f 秒才能做爆点开头", MinHookSec+1)
	}
	workDir := workDirFor(videoPath)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}

	progress(12, "正在抽出音轨并转写台词…")
	cues, transcript, err := transcribeVideo(ctx, cfg, bins, videoPath, workDir, info.HasAudio, progress)
	if err != nil {
		progress(-1, "台词转写未完成，将主要根据画面和音量分析: "+err.Error())
		cues = nil
		transcript = "（台词转写失败，仅根据画面和音量）"
	} else if len(cues) > 0 {
		progress(-1, fmt.Sprintf("已转写 %d 句台词", len(cues)))
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	progress(45, "正在寻找高能段落…")
	var peaks []float64
	wavPath := filepath.Join(workDir, "audio.wav")
	if _, err := os.Stat(wavPath); err == nil {
		if series, e := rmsSeries(wavPath, 0.5, 0.25); e == nil {
			peaks = peakTimes(series, 8)
		}
	}
	scenes := detectScenes(ctx, bins, videoPath, info.Duration)
	windows := buildWindows(cues, peaks, scenes, info.Duration)
	if len(windows) == 0 {
		// last resort: evenly sampled mid-body windows
		step := info.Duration / 5
		for i := 1; i <= 3; i++ {
			start := step * float64(i)
			end := start + PrefHookSec
			start, end = clampWindow(start, end, info.Duration)
			windows = append(windows, scoredWindow{Start: start, End: end, Why: "均匀抽样"})
		}
	}

	progress(60, "正在抽出候选画面…")
	thumbDir := filepath.Join(workDir, "thumbs")
	_ = os.MkdirAll(thumbDir, 0o755)
	thumbs := make([]string, len(windows))
	for i, w := range windows {
		if i >= 8 {
			break
		}
		mid := (w.Start + w.End) / 2
		p := filepath.Join(thumbDir, fmt.Sprintf("w%02d.jpg", i+1))
		if err := extractFrame(ctx, bins, videoPath, mid, p); err == nil {
			thumbs[i] = p
		}
	}

	progress(70, "正在判断最抓眼球的片段…")
	cands, usedModel, err := chooseCandidates(ctx, cfg, workDir, transcript, cues, windows, thumbs, info.Duration, progress)
	if err != nil {
		return nil, err
	}

	progress(88, "正在生成预览…")
	for i := range cands {
		prev := filepath.Join(workDir, fmt.Sprintf("preview_%d.mp4", i+1))
		if err := cutPreview(ctx, bins, videoPath, cands[i].StartSec, cands[i].EndSec, prev); err == nil {
			cands[i].PreviewPath = prev
		}
		thumb := filepath.Join(workDir, fmt.Sprintf("thumb_%d.jpg", i+1))
		mid := (cands[i].StartSec + cands[i].EndSec) / 2
		if err := extractFrame(ctx, bins, videoPath, mid, thumb); err == nil {
			cands[i].ThumbPath = thumb
		}
		if strings.TrimSpace(cands[i].Quote) == "" {
			cands[i].Quote = quoteInRange(cues, cands[i].StartSec, cands[i].EndSec)
		}
	}

	progress(100, "分析完成，请选择一个片头")
	return &Result{
		VideoPath:  videoPath,
		WorkDir:    workDir,
		Duration:   info.Duration,
		Transcript: transcript,
		Candidates: cands,
		UsedModel:  usedModel,
	}, nil
}

func chooseCandidates(ctx context.Context, cfg *config.Config, workDir, transcript string, cues []Cue, windows []scoredWindow, thumbs []string, duration float64, progress Progress) ([]Candidate, bool, error) {
	cands, err := pickWithModel(ctx, cfg, workDir, transcript, windows, thumbs, duration)
	if err == nil && len(cands) > 0 {
		return cands, true, nil
	}
	if err != nil {
		progress(-1, "模型挑选失败，改用台词和音量排序: "+err.Error())
	}
	fallback := heuristicCandidates(windows, cues)
	if len(fallback) == 0 {
		if err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("没有找到合适的片头片段")
	}
	return fallback, false, nil
}

func heuristicCandidates(windows []scoredWindow, cues []Cue) []Candidate {
	sort.Slice(windows, func(i, j int) bool { return windows[i].Score > windows[j].Score })
	types := []string{"强冲突", "强反转", "强悬念"}
	var out []Candidate
	for i, w := range windows {
		if i >= 3 {
			break
		}
		q := w.Quote
		if q == "" {
			q = quoteInRange(cues, w.Start, w.End)
		}
		reason := "按台词冲击力和音量变化选出的高能段落"
		if w.Why != "" {
			reason = w.Why + "，综合台词和音量排序"
		}
		out = append(out, Candidate{
			Rank:        i + 1,
			HookType:    types[i%len(types)],
			Reason:      reason,
			StartSec:    w.Start,
			EndSec:      w.End,
			Quote:       q,
			Recommended: i == 0,
		})
	}
	return out
}
