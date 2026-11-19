package reverse

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/narratoai/narrato-go/internal/media"
)

// Job is one reverse-engineering run.
type Job struct {
	VideoPath string
	OutDir    string
	Duration  float64
	Shots     []Shot
	Frames    []Frame
}

type Frame struct {
	Path      string  `json:"path"`
	Index     int     `json:"index"`
	Timestamp float64 `json:"timestamp"`
	Shot      int     `json:"shot"`
}

func NewOutDir(videoPath string) string {
	dir := filepath.Dir(videoPath)
	stamp := time.Now().Format("20060102_150405")
	base := filepath.Base(videoPath)
	stem := base
	if ext := filepath.Ext(base); ext != "" {
		stem = base[:len(base)-len(ext)]
	}
	return filepath.Join(dir, "reverse_"+stem+"_"+stamp)
}

func Prepare(ctx context.Context, bins media.FFmpegBins, videoPath, outDir string, progress func(int, string)) (*Job, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	framesDir := filepath.Join(outDir, "frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return nil, err
	}

	progress(5, "正在探测片长…")
	dur, err := media.ProbeDurationSec(ctx, bins, videoPath)
	if err != nil || dur <= 0 {
		progress(-1, "未能读取片长，将按已抽出的帧继续")
		dur = 0
	}

	progress(8, "正在检测镜头切点…")
	shots, detErr := detectShots(ctx, bins, videoPath, outDir, dur, progress)
	if detErr != nil {
		progress(-1, "镜头检测失败，改为按时长补帧: "+detErr.Error())
		if dur <= 0 {
			dur = 60
		}
		shots = fallbackShots(dur)
	}
	if dur <= 0 && len(shots) > 0 {
		dur = shots[len(shots)-1].End
	}

	points := planSampleTimes(shots, maxAnalysisFrames)
	progress(15, fmt.Sprintf("检测到 %d 个镜头，准备抽出 %d 帧", len(shots), len(points)))

	paths, kept, err := extractAt(ctx, bins, videoPath, framesDir, points, progress)
	if err != nil {
		return nil, err
	}

	frames := make([]Frame, 0, len(paths))
	for i, p := range paths {
		ts, shot := 0.0, 0
		if i < len(kept) {
			ts = kept[i].Timestamp
			shot = kept[i].Shot
		}
		frames = append(frames, Frame{Path: filepath.Base(p), Index: i + 1, Timestamp: ts, Shot: shot})
	}
	job := &Job{VideoPath: videoPath, OutDir: outDir, Duration: dur, Shots: shots, Frames: frames}
	man, _ := json.MarshalIndent(map[string]any{
		"video":    videoPath,
		"duration": dur,
		"shots":    shots,
		"frames":   frames,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "frames.json"), man, 0o644); err != nil {
		return nil, err
	}
	jobMD := fmt.Sprintf("# 逆向任务\n\n- 原片：`%s`\n- 时长：%.2f 秒\n- 镜头：共 %d 个，已按画面变化自动切开，起止时间见 `frames.json` 的 shots\n- 关键帧：`frames/`，共 %d 张（每个镜头至少 1 张，较长镜头会补首尾帧，间隔约 %.0f 秒），所属镜头见 frames.json\n- 请按 skill「视频逆向」写出 CAST.md、SHOTS.md、REMAKE.md、REPORT.md。\n- 写 SHOTS.md 时请按 shots 的起止时间分镜，不要把两次抽样之间的整段都当成一个镜头；关键帧是抽样，不是全部画面。\n- 分析由程序分批进行：每批观察若干关键帧，把笔记追加到 NOTES.md；全部观察完成后再抽出音轨、转写台词到 TRANSCRIPT.md，并根据 NOTES.md 和转写写出上述四个文件。\n",
		videoPath, dur, len(shots), len(frames), maxGapSec)
	if err := os.WriteFile(filepath.Join(outDir, "JOB.md"), []byte(jobMD), 0o644); err != nil {
		return nil, err
	}
	progress(40, fmt.Sprintf("已按 %d 个镜头抽出 %d 帧，时长 %.1f 秒", len(shots), len(frames), dur))
	return job, nil
}
