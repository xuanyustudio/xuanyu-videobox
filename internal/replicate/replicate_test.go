package replicate_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/replicate"
)

const sampleReply = "## SORA2 Prompt (English)\n```\n### Style\nWarm light.\n```\n\n## SORA2 提示词 (中文)\n```\n### 风格\n暖光。\n```"

func TestFrameTimestamps(t *testing.T) {
	got := replicate.FrameTimestamps(9, 8)
	if len(got) != 8 {
		t.Fatalf("len=%d", len(got))
	}
	for i, v := range got {
		if math.Abs(v-float64(i+1)) > 1e-9 {
			t.Fatalf("frame %d at %v", i, v)
		}
	}
	// Unknown duration falls back to 60s like the Python version.
	if got := replicate.FrameTimestamps(0, 2); math.Abs(got[0]-20) > 1e-9 || math.Abs(got[1]-40) > 1e-9 {
		t.Fatalf("fallback %v", got)
	}
}

func TestClampFrames(t *testing.T) {
	if replicate.ClampFrames(0) != replicate.DefaultFrames || replicate.ClampFrames(99) != replicate.MaxFrames || replicate.ClampFrames(12) != 12 {
		t.Fatal("clamp")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	p := replicate.BuildUserPrompt(replicate.TemplateStandard, 8, 12.34)
	if !strings.HasPrefix(p, "这是从一个视频中提取的 8 帧关键画面（按时间顺序）。") {
		t.Fatalf("prefix: %q", p)
	}
	if !strings.HasSuffix(p, "视频实际时长: 12.3秒，请根据此时长分配各场景时间。") {
		t.Fatalf("suffix: %q", p)
	}
	if strings.Contains(replicate.BuildUserPrompt(replicate.TemplateStandard, 8, 0), "视频实际时长") {
		t.Fatal("no duration hint expected when duration unknown")
	}
	sd := replicate.BuildUserPrompt(replicate.TemplateSeedance2, 8, 12.34)
	if !strings.Contains(sd, "恰好 15 秒") || !strings.Contains(sd, "源视频实际时长: 12.3秒") {
		t.Fatalf("seedance user prompt: %q", sd)
	}
	if !strings.Contains(replicate.BuildUserPrompt(replicate.TemplateSeedance2, 8, 0), "源视频时长未知") {
		t.Fatal("seedance unknown duration hint")
	}
}

func TestSystemPrompts(t *testing.T) {
	std := replicate.SystemPrompt(replicate.TemplateStandard)
	if !strings.HasPrefix(std, "你是 SORA2 视频复刻提示词专家。") || !strings.Contains(std, "## SORA2 Prompt (English)") {
		t.Fatalf("standard prompt: %.80q", std)
	}
	if !strings.HasPrefix(replicate.SystemPrompt(replicate.TemplateReplicateExpert), "# 复刻 SORA2 视频提示词专家模板") {
		t.Fatal("replicate expert template not embedded")
	}
	if !strings.HasPrefix(replicate.SystemPrompt(replicate.TemplateSora2Expert), "# SORA2 视频提示词生成专家") {
		t.Fatal("sora2 expert template not embedded")
	}
	if !strings.HasPrefix(replicate.SystemPrompt(replicate.TemplateSeedance2), "你是 Seedance 2.0") {
		t.Fatal("seedance2 template not embedded")
	}
	if strings.Contains(replicate.SystemPrompt(replicate.TemplateSora2Expert), "\r") {
		t.Fatal("CRLF should be normalized")
	}
	for _, o := range replicate.TemplateOptions() {
		if replicate.TemplateByLabel(o.Label) != o.ID {
			t.Fatalf("label roundtrip %q", o.Label)
		}
	}
}

func TestParsePrompts(t *testing.T) {
	en, zh := replicate.ParsePrompts(sampleReply)
	if en != "### Style\nWarm light." || zh != "### 风格\n暖光。" {
		t.Fatalf("en=%q zh=%q", en, zh)
	}
	alt := "#### English Version\n```\nhello\n```\n#### 中文版本\n```\n你好\n```"
	if en, zh := replicate.ParsePrompts(alt); en != "hello" || zh != "你好" {
		t.Fatalf("alt en=%q zh=%q", en, zh)
	}
	if got := replicate.PickPrompt("", "中文", "full"); got != "中文" {
		t.Fatal(got)
	}
	if got := replicate.PickPrompt("", "", " full "); got != "full" {
		t.Fatal(got)
	}
}

type captured struct {
	Auth string
	Path string
	Body struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
}

func fakeServer(t *testing.T, got *captured, status int, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Auth = r.Header.Get("Authorization")
		got.Path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &got.Body); err != nil {
			t.Errorf("bad request json: %v", err)
		}
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(reply))
			return
		}
		out, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": reply}}},
		})
		_, _ = w.Write(out)
	}))
}

func writeFakeFrames(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	var out []string
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, fmt.Sprintf("frame_%03d.jpg", i))
		if err := os.WriteFile(p, []byte{0xFF, 0xD8, 0xFF, byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func TestClientAnalyzeRequestShape(t *testing.T) {
	var got captured
	srv := fakeServer(t, &got, http.StatusOK, sampleReply)
	defer srv.Close()

	frames := writeFakeFrames(t, 3)
	c := &replicate.Client{BaseURL: srv.URL + "/v1", APIKey: "ms-key", Model: "Qwen/Qwen3-VL-8B-Instruct"}
	text, err := c.Analyze(context.Background(), "SYS", "USER", frames)
	if err != nil {
		t.Fatal(err)
	}
	if text != sampleReply {
		t.Fatalf("text=%q", text)
	}
	if got.Auth != "Bearer ms-key" || got.Path != "/v1/chat/completions" || got.Body.Model != "Qwen/Qwen3-VL-8B-Instruct" || got.Body.Stream {
		t.Fatalf("request: %+v", got)
	}
	if len(got.Body.Messages) != 2 || got.Body.Messages[0].Role != "system" || got.Body.Messages[1].Role != "user" {
		t.Fatalf("messages: %+v", got.Body.Messages)
	}
	var sys string
	_ = json.Unmarshal(got.Body.Messages[0].Content, &sys)
	if sys != "SYS" {
		t.Fatalf("system=%q", sys)
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(got.Body.Messages[1].Content, &parts); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 4 || parts[0].Type != "text" || parts[0].Text != "USER" {
		t.Fatalf("parts: %+v", parts)
	}
	for _, p := range parts[1:] {
		if p.Type != "image_url" || !strings.HasPrefix(p.ImageURL.URL, "data:image/jpeg;base64,/9j/") {
			t.Fatalf("image part: %+v", p)
		}
	}
}

func TestClientAnalyzeErrors(t *testing.T) {
	c := &replicate.Client{BaseURL: "http://127.0.0.1:1"}
	if _, err := c.Analyze(context.Background(), "", "u", nil); err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("missing key err=%v", err)
	}
	var got captured
	srv := fakeServer(t, &got, http.StatusUnauthorized, `{"error":"bad token"}`)
	defer srv.Close()
	c = &replicate.Client{BaseURL: srv.URL, APIKey: "x"}
	if _, err := c.Analyze(context.Background(), "", "u", nil); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("401 err=%v", err)
	}
	// Without a system prompt only the user message is sent.
	if len(got.Body.Messages) != 1 || got.Body.Messages[0].Role != "user" {
		t.Fatalf("messages: %+v", got.Body.Messages)
	}
}

func TestValidate(t *testing.T) {
	if err := replicate.Validate(replicate.Params{}); err == nil {
		t.Fatal("expected error for missing video")
	}
	if err := replicate.Validate(replicate.Params{VideoPath: "a.mp4"}); err == nil {
		t.Fatal("expected error for missing key")
	}
}

// TestRunEndToEnd uses real ffmpeg (skipped if absent) and a fake vision API.
func TestRunEndToEnd(t *testing.T) {
	dir, video, bins := synthesizeLavfi(t, 3)

	srv, reqs := recordVision(t, []string{sampleReply})
	defer srv.Close()

	var last int
	var msgs []string
	res, err := replicate.Run(context.Background(), replicate.Params{
		VideoPath: video,
		OutputDir: filepath.Join(dir, "out"),
		Frames:    4,
		BaseURL:   srv.URL,
		APIKey:    "k",
		Bins:      bins,
		Transcribe: func(_, outPath string) error {
			return os.WriteFile(outPath, []byte("1\n00:00:00,000 --> 00:00:01,000\n你好\n\n"), 0o644)
		},
		OnProgress: func(p int, msg string) {
			last = p
			msgs = append(msgs, msg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 4 || last != 100 {
		t.Fatalf("frames=%d last=%d", len(res.Frames), last)
	}
	if res.DurationSec < 2.9 || res.DurationSec > 3.1 {
		t.Fatalf("duration=%v", res.DurationSec)
	}
	if res.Prompt != "### Style\nWarm light." {
		t.Fatalf("prompt=%q", res.Prompt)
	}
	for _, name := range []string{"frame_000.jpg", "frame_003.jpg", replicate.AnalysisFile, replicate.PromptENFile, replicate.PromptZHFile} {
		if _, err := os.Stat(filepath.Join(dir, "out", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "character_ref.jpg")); !os.IsNotExist(err) {
		t.Fatalf("character_ref.jpg should not exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "segment_01")); !os.IsNotExist(err) {
		t.Fatalf("segment_01 should not exist: %v", err)
	}
	zh, err := os.ReadFile(filepath.Join(dir, "out", replicate.PromptZHFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(zh), "【锁定形象】") {
		t.Fatalf("single-segment prompt has lock header: %q", zh)
	}
	if !strings.Contains(strings.Join(msgs, "\n"), "字幕") {
		t.Fatalf("progress messages: %q", msgs)
	}
	gotReqs := *reqs
	if len(gotReqs) != 1 {
		t.Fatalf("vision calls=%d", len(gotReqs))
	}
	got := gotReqs[0]
	var parts []json.RawMessage
	_ = json.Unmarshal(got.Body.Messages[1].Content, &parts)
	if len(parts) != 5 {
		t.Fatalf("expected 1 text + 4 images, got %d", len(parts))
	}
	var textPart struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(parts[0], &textPart); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textPart.Text, "你好") {
		t.Fatalf("user text: %q", textPart.Text)
	}
	var sys string
	_ = json.Unmarshal(got.Body.Messages[0].Content, &sys)
	if sys != replicate.SystemPrompt(replicate.TemplateStandard) {
		t.Fatal("standard system prompt expected by default")
	}
}

func TestRunUsesSubtitleFailure(t *testing.T) {
	dir, video, bins := synthesizeLavfi(t, 3)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := replicate.Run(context.Background(), replicate.Params{
		VideoPath: video,
		OutputDir: filepath.Join(dir, "out"),
		Frames:    2,
		BaseURL:   srv.URL,
		APIKey:    "k",
		Bins:      bins,
		Transcribe: func(_, _ string) error {
			return fmt.Errorf("连不上")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "字幕提取失败") {
		t.Fatalf("err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("vision calls=%d", calls)
	}
}

func TestRunLongVideoSegments(t *testing.T) {
	dir, video, bins := synthesizeLavfi(t, 20)
	lockReply := "## 锁定形象 (中文)\n```\n黑发。\n```\n\n## 锁定形象 (English)\n```\nBlack hair.\n```\n"
	srv, reqs := recordVision(t, []string{lockReply, sampleReply, sampleReply})
	defer srv.Close()

	var msgs []string
	res, err := replicate.Run(context.Background(), replicate.Params{
		VideoPath:  video,
		OutputDir:  filepath.Join(dir, "out"),
		Frames:     2,
		SegmentSec: 15,
		BaseURL:    srv.URL,
		APIKey:     "k",
		Bins:       bins,
		Transcribe: func(_, outPath string) error {
			body := "1\n00:00:01,000 --> 00:00:02,000\n早句\n\n2\n00:00:14,500 --> 00:00:15,500\n跨句\n\n"
			return os.WriteFile(outPath, []byte(body), 0o644)
		},
		OnProgress: func(_ int, msg string) {
			msgs = append(msgs, msg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := *reqs
	if len(got) != 3 {
		t.Fatalf("vision calls=%d", len(got))
	}
	lockSys, lockUser := replicate.LockPrompts()
	text, images := userVision(t, got[0])
	var sys string
	if err := json.Unmarshal(got[0].Body.Messages[0].Content, &sys); err != nil {
		t.Fatal(err)
	}
	if sys != lockSys || text != lockUser {
		t.Fatalf("lock sys=%q user=%q", sys, text)
	}
	if len(images) != 2 {
		t.Fatalf("lock images=%d, want one per segment", len(images))
	}
	if strings.Contains(text, "早句") || strings.Contains(text, "跨句") {
		t.Fatal("lock request included segment subtitles")
	}

	ref, err := os.ReadFile(filepath.Join(dir, "out", "character_ref.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Segments) != 2 || res.Segments[1].StartSec != 15 {
		t.Fatalf("segments=%+v", res.Segments)
	}
	for i, req := range got[1:] {
		text, images := userVision(t, req)
		if !strings.Contains(text, "跨句") || !strings.Contains(text, "黑发。") {
			t.Fatalf("analysis %d text=%q", i, text)
		}
		hasEarly := strings.Contains(text, "早句")
		if i == 0 && !hasEarly {
			t.Fatal("first analysis missing 早句")
		}
		if i == 1 && hasEarly {
			t.Fatal("second analysis includes 早句")
		}
		if len(images) != len(res.Segments[i].Frames)+1 {
			t.Fatalf("analysis %d images=%d frames=%d", i, len(images), len(res.Segments[i].Frames))
		}
		if !bytes.Equal(decodeDataJPEG(t, images[0]), ref) {
			t.Fatalf("analysis %d first image is not character_ref.jpg", i)
		}
		var segSys string
		if err := json.Unmarshal(req.Body.Messages[0].Content, &segSys); err != nil {
			t.Fatal(err)
		}
		if segSys != replicate.SystemPrompt(replicate.TemplateStandard) {
			t.Fatalf("analysis %d system prompt", i)
		}
	}
	for _, name := range []string{"segment_01", "segment_02"} {
		b, err := os.ReadFile(filepath.Join(dir, "out", name, replicate.PromptZHFile))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(b), "【锁定形象】\n黑发。") {
			t.Fatalf("%s prompt_zh=%q", name, b)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "out", name, "analysis.md"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "【锁定形象】") {
			t.Fatalf("%s analysis.md includes lock: %q", name, raw)
		}
	}
	rootZH, err := os.ReadFile(filepath.Join(dir, "out", replicate.PromptZHFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(rootZH), "【锁定形象】") != 1 || !strings.Contains(string(rootZH), "第 2 段") {
		t.Fatalf("root zh=%q", rootZH)
	}
	srt, err := os.ReadFile(filepath.Join(dir, "out", "subtitles.srt"))
	if err != nil || !strings.Contains(string(srt), "跨句") {
		t.Fatalf("srt err=%v body=%q", err, srt)
	}
	analysis, err := os.ReadFile(filepath.Join(dir, "out", replicate.AnalysisFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(analysis), "【锁定形象】") {
		t.Fatalf("analysis=%q", analysis)
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "字幕") || !strings.Contains(joined, "锁定形象") || !strings.Contains(joined, "第 2 段") {
		t.Fatalf("progress messages: %q", msgs)
	}
}

func synthesizeLavfi(t *testing.T, seconds int) (dir, video string, bins media.FFmpegBins) {
	t.Helper()
	ffmpeg, err := media.ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	ffprobe, _ := media.ResolveFFprobe("", "")
	dir = t.TempDir()
	video = filepath.Join(dir, "in.mp4")
	cmd := exec.Command(ffmpeg, "-y", "-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=160x120:rate=10:duration=%d", seconds), "-pix_fmt", "yuv420p", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize test video: %v %s", err, out)
	}
	return dir, video, media.FFmpegBins{FFmpeg: ffmpeg, FFprobe: ffprobe}
}

func recordVision(t *testing.T, replies []string) (*httptest.Server, *[]captured) {
	t.Helper()
	var mu sync.Mutex
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c captured
		c.Auth = r.Header.Get("Authorization")
		c.Path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &c.Body); err != nil {
			t.Errorf("bad request json: %v", err)
		}
		mu.Lock()
		got = append(got, c)
		n := len(got)
		mu.Unlock()
		reply := sampleReply
		if n-1 < len(replies) {
			reply = replies[n-1]
		}
		w.WriteHeader(http.StatusOK)
		out, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": reply}}},
		})
		_, _ = w.Write(out)
	}))
	return srv, &got
}

func userVision(t *testing.T, c captured) (text string, images []string) {
	t.Helper()
	var raw json.RawMessage
	for _, m := range c.Body.Messages {
		if m.Role == "user" {
			raw = m.Content
			break
		}
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		t.Fatalf("user content: %v", err)
	}
	for _, p := range parts {
		switch p.Type {
		case "text":
			text = p.Text
		case "image_url":
			images = append(images, p.ImageURL.URL)
		}
	}
	return text, images
}

func decodeDataJPEG(t *testing.T, dataURL string) []byte {
	t.Helper()
	const pfx = "data:image/jpeg;base64,"
	if !strings.HasPrefix(dataURL, pfx) {
		t.Fatalf("image url: %.48q", dataURL)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, pfx))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
