// Package replicate implements 爆款复刻: sample keyframes from a reference video,
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

	Bins       media.FFmpegBins
	HTTP       *http.Client
	OnProgress func(percent int, msg string)
}

// Result is what the UI shows / what was written to disk.
type Result struct {
	OutputDir    string
	Frames       []string
	DurationSec  float64
	Analysis     string // full model reply
	PromptEN     string
	PromptZH     string
	Prompt       string // EN if present, else ZH, else full reply
	AnalysisPath string
}

// Validate checks required fields before running.
func Validate(p Params) error {
	if strings.TrimSpace(p.VideoPath) == "" {
		return fmt.Errorf("请先选择视频")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return fmt.Errorf("请先在「设置」中填写爆款复刻 API Key")
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

// Run extracts keyframes, calls the vision model and saves the outputs.
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
	n := ClampFrames(p.Frames)

	outDir := p.OutputDir
	if outDir == "" {
		outDir = filepath.Join(filepath.Dir(p.VideoPath), "replicate_"+time.Now().Format("20060102_150405"))
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	progress(5, fmt.Sprintf("正在提取 %d 张关键帧…", n))
	frames, dur, err := ExtractFrames(ctx, p.Bins, p.VideoPath, n, outDir, progress)
	if err != nil {
		return nil, err
	}

	progress(45, fmt.Sprintf("已提取 %d 帧，正在分析视频并生成 SORA2 提示词…", len(frames)))
	client := &Client{BaseURL: p.BaseURL, APIKey: p.APIKey, Model: p.Model, HTTP: p.HTTP}
	text, err := client.Analyze(ctx, SystemPrompt(p.Template), BuildUserPrompt(p.Template, len(frames), dur), frames)
	if err != nil {
		return nil, err
	}

	progress(90, "正在保存结果…")
	res := &Result{
		OutputDir:   outDir,
		Frames:      frames,
		DurationSec: dur,
		Analysis:    text,
	}
	res.PromptEN, res.PromptZH = ParsePrompts(text)
	res.Prompt = PickPrompt(res.PromptEN, res.PromptZH, text)
	if err := save(res); err != nil {
		return nil, err
	}
	progress(100, "SORA2 提示词生成完成")
	return res, nil
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
