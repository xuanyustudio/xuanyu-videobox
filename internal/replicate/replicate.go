// Package replicate implements 提示词提取: sample keyframes from a reference video,
// ask a vision model (default Qwen3-VL via ModelScope) to write SORA2-style
// replication prompts, and save prompts + keyframes beside the source video.
// Ported from the Python AI-video-Replicate project (qwen3vl.py / app.py).
package replicate

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/media"
)

// Output file names inside the result directory.
const (
	AnalysisFile = "sora2_analysis.md"
	PromptENFile = "prompt_en.txt"
	PromptZHFile = "prompt_zh.txt"
)

// Params drives one replication-prompt job.
type Params struct {
	VideoPath string
	// OutputDir defaults to <video dir>/replicate_<timestamp>.
	OutputDir string
	Frames    int
	Template  Template

	BaseURL string
	APIKey  string
	Model   string

	// SegmentSec is the analysis window length. Only 10 or 15 are kept; anything else is 15.
	SegmentSec int
	ASR        asr.Options
	// Transcribe writes an SRT to outPath. Nil uses asr.RequireReady then asr.Transcribe.
	Transcribe func(mediaPath, outPath string) error

	Bins       media.FFmpegBins
	HTTP       *http.Client
	OnProgress func(percent int, msg string)
}

// SegmentResult is one analyzed window. PromptEN and PromptZH include the
// appearance lock when the video was split into more than one segment.
type SegmentResult struct {
	Index      int // 1-based
	StartSec   float64
	EndSec     float64
	Frames     []string
	Analysis   string
	PromptEN   string
	PromptZH   string
	FrameStart int // index into Result.Frames
}

// Result is what the UI shows / what was written to disk.
// PromptEN, PromptZH, Analysis and Prompt describe the combined output.
type Result struct {
	OutputDir       string
	Frames          []string
	DurationSec     float64
	Analysis        string // combined model reply
	PromptEN        string
	PromptZH        string
	Prompt          string // EN if present, else ZH, else full reply
	AnalysisPath    string
	SubtitlePath    string
	CharacterLockZH string
	CharacterLockEN string
	CharacterRef    string
	Segments        []SegmentResult
}

// Validate checks required fields before running.
func Validate(p Params) error {
	if strings.TrimSpace(p.VideoPath) == "" {
		return fmt.Errorf("请先选择视频")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return fmt.Errorf("请先在「设置」中填写视觉模型 API Key")
	}
	return nil
}

// ClampFrames normalizes the requested frame count (0 → default, capped at MaxFrames).
func ClampFrames(n int) int {
	if n <= 0 {
		return DefaultFrames
	}
	if n > MaxFrames {
		return MaxFrames
	}
	return n
}

// Run extracts subtitles and keyframes, calls the vision model and saves the outputs.
// Videos longer than one segment are appearance-locked, then analyzed per segment.
func Run(ctx context.Context, p Params) (*Result, error) {
	if err := Validate(p); err != nil {
		return nil, err
	}
	if _, err := os.Stat(p.VideoPath); err != nil {
		return nil, fmt.Errorf("视频不存在: %w", err)
	}
	progress := p.OnProgress
	if progress == nil {
		progress = func(int, string) {}
	}

	outDir := p.OutputDir
	if outDir == "" {
		outDir = filepath.Join(filepath.Dir(p.VideoPath), "replicate_"+time.Now().Format("20060102_150405"))
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	segSec := NormalizeSegmentSec(p.SegmentSec)
	n := ClampFrames(p.Frames)
	dur, err := media.ProbeDurationSec(ctx, p.Bins, p.VideoPath)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		dur = 0
	}
	spans := PlanSegments(dur, segSec)
	progress(8, "正在准备分段…")

	srtPath := filepath.Join(outDir, "subtitles.srt")
	progress(12, "正在提取字幕…")
	transcribe := p.Transcribe
	if transcribe == nil {
		if err := asr.RequireReady(p.ASR); err != nil {
			return nil, fmt.Errorf("字幕提取失败: %w", err)
		}
		asrOpt := p.ASR
		transcribe = func(mediaPath, outPath string) error {
			return asr.Transcribe(mediaPath, outPath, asrOpt)
		}
	}
	if err := transcribe(p.VideoPath, srtPath); err != nil {
		return nil, fmt.Errorf("字幕提取失败: %w", err)
	}
	srtBytes, err := os.ReadFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("字幕提取失败: %w", err)
	}
	progress(20, "字幕已提取")
	cues := ParseSRT(string(srtBytes))

	multi := len(spans) > 1
	type extracted struct {
		span   Span
		dir    string
		frames []string
	}
	parts := make([]extracted, 0, len(spans))
	for i, span := range spans {
		dir := outDir
		if multi {
			dir = filepath.Join(outDir, fmt.Sprintf("segment_%02d", i+1))
			progress(20+i*20/len(spans), fmt.Sprintf("正在提取第 %d 段关键帧…", i+1))
		} else {
			progress(30, fmt.Sprintf("正在提取 %d 张关键帧…", n))
		}
		frames, err := ExtractSpanFrames(ctx, p.Bins, p.VideoPath, span, n, dir, nil)
		if err != nil {
			return nil, err
		}
		if len(frames) == 0 {
			return nil, fmt.Errorf("第 %d 段无法提取视频帧，请确保已安装 ffmpeg", i+1)
		}
		parts = append(parts, extracted{span: span, dir: dir, frames: frames})
	}

	client := &Client{BaseURL: p.BaseURL, APIKey: p.APIKey, Model: p.Model, HTTP: p.HTTP}
	var lockZH, lockEN, refPath string
	if multi {
		refPath = filepath.Join(outDir, "character_ref.jpg")
		refBytes, err := os.ReadFile(parts[0].frames[0])
		if err != nil {
			return nil, fmt.Errorf("锁定形象失败: %w", err)
		}
		if err := os.WriteFile(refPath, refBytes, 0o644); err != nil {
			return nil, fmt.Errorf("锁定形象失败: %w", err)
		}
		lockFrames := make([]string, 0, len(parts))
		for _, part := range parts {
			if len(lockFrames) >= 8 {
				break
			}
			lockFrames = append(lockFrames, part.frames[0])
		}
		progress(50, "正在锁定形象…")
		lockSys, lockUser := LockPrompts()
		lockReply, err := client.Analyze(ctx, lockSys, lockUser, lockFrames)
		if err != nil {
			return nil, fmt.Errorf("锁定形象失败: %w", err)
		}
		lockZH, lockEN = ParseLock(lockReply)
		if strings.TrimSpace(lockZH) == "" || strings.TrimSpace(lockEN) == "" {
			return nil, fmt.Errorf("锁定形象失败: 未解析出中英文")
		}
	}

	drafts := make([]SegmentDraft, 0, len(parts))
	raws := make([]string, 0, len(parts))
	for i, part := range parts {
		if multi {
			progress(55+(i*35)/len(parts), fmt.Sprintf("正在分析第 %d 段…", i+1))
		} else {
			progress(60, fmt.Sprintf("已提取 %d 帧，正在分析视频并生成 SORA2 提示词…", len(part.frames)))
		}
		user := BuildSegmentUserPrompt(p.Template, len(part.frames), part.span, segSec, CuesForSpan(cues, part.span), multi)
		if multi {
			user += "\n\n【锁定形象】\n" + strings.TrimSpace(lockZH)
			if en := strings.TrimSpace(lockEN); en != "" {
				user += "\n\n[Locked appearance]\n" + en
			}
		}
		images := part.frames
		if multi {
			images = append([]string{refPath}, part.frames...)
		}
		reply, err := client.Analyze(ctx, SystemPrompt(p.Template), user, images)
		if err != nil {
			return nil, fmt.Errorf("第 %d 段分析失败: %w", i+1, err)
		}
		en, zh := ParsePrompts(reply)
		drafts = append(drafts, SegmentDraft{
			Index:    i + 1,
			Start:    part.span.Start,
			End:      part.span.End,
			PromptEN: en,
			PromptZH: zh,
			Analysis: reply,
		})
		raws = append(raws, reply)
	}

	assembled := AssemblePrompts(drafts, lockZH, lockEN)
	progress(90, "正在保存结果…")

	var allFrames []string
	segs := make([]SegmentResult, 0, len(parts))
	frameStart := 0
	for i, part := range parts {
		allFrames = append(allFrames, part.frames...)
		segs = append(segs, SegmentResult{
			Index:      i + 1,
			StartSec:   part.span.Start,
			EndSec:     part.span.End,
			Frames:     part.frames,
			Analysis:   raws[i],
			PromptEN:   assembled.SegmentEN[i],
			PromptZH:   assembled.SegmentZH[i],
			FrameStart: frameStart,
		})
		frameStart += len(part.frames)
		if multi {
			if err := writeSegmentFiles(part.dir, assembled.SegmentEN[i], assembled.SegmentZH[i], raws[i]); err != nil {
				return nil, err
			}
		}
	}

	res := &Result{
		OutputDir:       outDir,
		Frames:          allFrames,
		DurationSec:     dur,
		Analysis:        assembled.Analysis,
		PromptEN:        assembled.PromptEN,
		PromptZH:        assembled.PromptZH,
		Prompt:          PickPrompt(assembled.PromptEN, assembled.PromptZH, assembled.Analysis),
		SubtitlePath:    srtPath,
		CharacterLockZH: lockZH,
		CharacterLockEN: lockEN,
		CharacterRef:    refPath,
		Segments:        segs,
	}
	if err := save(res); err != nil {
		return nil, err
	}
	progress(100, "SORA2 提示词生成完成")
	return res, nil
}

func writeSegmentFiles(dir, en, zh, analysis string) error {
	if err := os.WriteFile(filepath.Join(dir, PromptENFile), []byte(en), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, PromptZHFile), []byte(zh), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "analysis.md"), []byte(analysis), 0o644)
}

func save(res *Result) error {
	res.AnalysisPath = filepath.Join(res.OutputDir, AnalysisFile)
	if err := os.WriteFile(res.AnalysisPath, []byte(res.Analysis), 0o644); err != nil {
		return err
	}
	if res.PromptEN != "" {
		if err := os.WriteFile(filepath.Join(res.OutputDir, PromptENFile), []byte(res.PromptEN), 0o644); err != nil {
			return err
		}
	}
	if res.PromptZH != "" {
		if err := os.WriteFile(filepath.Join(res.OutputDir, PromptZHFile), []byte(res.PromptZH), 0o644); err != nil {
			return err
		}
	}
	return nil
}

var (
	// Same patterns as app.py extract_prompt_from_video (re.DOTALL).
	reEN = regexp.MustCompile("(?s)## SORA2 Prompt \\(English\\)\\s*```\\s*(.*?)\\s*```")
	reZH = regexp.MustCompile("(?s)## SORA2 提示词 \\(中文\\)\\s*```\\s*(.*?)\\s*```")
	// Fallbacks for the SORA2视频提示词专家 output format.
	reENAlt = regexp.MustCompile("(?s)#### English Version\\s*```\\s*(.*?)\\s*```")
	reZHAlt = regexp.MustCompile("(?s)#### 中文版本\\s*```\\s*(.*?)\\s*```")
)

// ParsePrompts extracts the fenced English and Chinese SORA2 prompts from a reply.
func ParsePrompts(text string) (en, zh string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	en = firstMatch(text, reEN, reENAlt)
	zh = firstMatch(text, reZH, reZHAlt)
	return en, zh
}

// PickPrompt mirrors app.py: English first, then Chinese. If neither block is found
// the whole reply is returned so the user still has something to edit.
func PickPrompt(en, zh, full string) string {
	if en != "" {
		return en
	}
	if zh != "" {
		return zh
	}
	return strings.TrimSpace(full)
}

func firstMatch(text string, res ...*regexp.Regexp) string {
	for _, re := range res {
		if m := re.FindStringSubmatch(text); m != nil {
			if s := strings.TrimSpace(m[1]); s != "" {
				return s
			}
		}
	}
	return ""
}
