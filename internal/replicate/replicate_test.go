package replicate_test

import (
	"context"
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
	p := replicate.BuildUserPrompt(8, 12.34)
	if !strings.HasPrefix(p, "这是从一个视频中提取的 8 帧关键画面（按时间顺序）。") {
		t.Fatalf("prefix: %q", p)
	}
	if !strings.HasSuffix(p, "视频实际时长: 12.3秒，请根据此时长分配各场景时间。") {
		t.Fatalf("suffix: %q", p)
	}
	if strings.Contains(replicate.BuildUserPrompt(8, 0), "视频实际时长") {
		t.Fatal("no duration hint expected when duration unknown")
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
	ffmpeg, err := media.ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	ffprobe, _ := media.ResolveFFprobe("", "")
	dir := t.TempDir()
	video := filepath.Join(dir, "in.mp4")
	cmd := exec.Command(ffmpeg, "-y", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=3", "-pix_fmt", "yuv420p", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize test video: %v %s", err, out)
	}

	var got captured
	srv := fakeServer(t, &got, http.StatusOK, sampleReply)
	defer srv.Close()

	var last int
	res, err := replicate.Run(context.Background(), replicate.Params{
		VideoPath: video,
		OutputDir: filepath.Join(dir, "out"),
		Frames:    4,
		BaseURL:   srv.URL,
		APIKey:    "k",
		Bins:      media.FFmpegBins{FFmpeg: ffmpeg, FFprobe: ffprobe},
		OnProgress: func(p int, _ string) {
			last = p
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
	var parts []json.RawMessage
	_ = json.Unmarshal(got.Body.Messages[1].Content, &parts)
	if len(parts) != 5 {
		t.Fatalf("expected 1 text + 4 images, got %d", len(parts))
	}
	var sys string
	_ = json.Unmarshal(got.Body.Messages[0].Content, &sys)
	if sys != replicate.SystemPrompt(replicate.TemplateStandard) {
		t.Fatal("standard system prompt expected by default")
	}
}
