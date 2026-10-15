package intro_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/narratoai/narrato-go/internal/intro"
)

func TestImagesEditsURL(t *testing.T) {
	if got := intro.ImagesEditsURL("https://api.openai.com"); got != "https://api.openai.com/v1/images/edits" {
		t.Fatalf("got %s", got)
	}
	if got := intro.ImagesEditsURL("https://api.openai.com/v1/"); got != "https://api.openai.com/v1/images/edits" {
		t.Fatalf("got %s", got)
	}
}

func TestGenerateFourView(t *testing.T) {
	png := []byte("png-bytes")
	var gotAuth, gotModel, gotPrompt, gotSize, gotFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" || r.Method != http.MethodPost {
			t.Errorf("path %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "model":
				gotModel = string(b)
			case "prompt":
				gotPrompt = string(b)
			case "size":
				gotSize = string(b)
			case "image[]":
				gotFile = part.FileName()
				if string(b) != "ref" {
					t.Fatalf("file body %q", b)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(png)}},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "role.png")
	if err := os.WriteFile(src, []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out", "fourview.png")
	got, err := intro.GenerateFourView(context.Background(), intro.FourViewOptions{
		ImagePath: src,
		OutPath:   out,
		BaseURL:   srv.URL + "/v1",
		APIKey:    "sk-test",
		HTTP:      srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != out || gotAuth != "Bearer sk-test" || gotModel != intro.ImageModel || gotPrompt != intro.FourViewPrompt || gotSize != intro.ImageSize || gotFile != "role.png" {
		t.Fatalf("got=%s auth=%s model=%s size=%s file=%s prompt=%q", got, gotAuth, gotModel, gotSize, gotFile, gotPrompt)
	}
	body, err := os.ReadFile(out)
	if err != nil || string(body) != string(png) {
		t.Fatalf("saved %q err=%v", body, err)
	}
}

func TestGenerateFourViewURL(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/img" {
			_, _ = w.Write([]byte("from-url"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"url": srv.URL + "/img"}},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "a.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "fourview.png")
	if _, err := intro.GenerateFourView(context.Background(), intro.FourViewOptions{
		ImagePath: src,
		OutPath:   out,
		BaseURL:   srv.URL + "/v1",
		APIKey:    "sk",
		HTTP:      srv.Client(),
	}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(out)
	if err != nil || string(body) != "from-url" {
		t.Fatalf("body %q err=%v", body, err)
	}
}

func TestGenerateFourViewAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "a.png")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := intro.GenerateFourView(context.Background(), intro.FourViewOptions{
		ImagePath: src,
		BaseURL:   srv.URL,
		APIKey:    "sk",
		HTTP:      srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "复刻 API Key") {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerateIntro(t *testing.T) {
	var polls int
	var mu sync.Mutex
	var gotPrompt, gotModel, gotRatio, gotImage string
	var gotDuration float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/videos":
			if r.Header.Get("Authorization") != "Bearer vw_test" {
				t.Errorf("auth %s", r.Header.Get("Authorization"))
			}
			var body struct {
				Prompt   string   `json:"prompt"`
				Model    string   `json:"model"`
				Duration float64  `json:"duration"`
				Ratio    string   `json:"ratio"`
				Images   []string `json:"images"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			gotPrompt, gotModel, gotRatio, gotDuration = body.Prompt, body.Model, body.Ratio, body.Duration
			if len(body.Images) == 1 {
				gotImage = body.Images[0]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job1", "status": "queued", "warnings": []string{"已按默认生成"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/videos/job1":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			status := "in_progress"
			if n >= 2 {
				status = "completed"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job1", "status": status, "progress": 40})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/videos/job1/content":
			if r.URL.Query().Get("compat") != "1" {
				t.Errorf("compat=%s", r.URL.RawQuery)
			}
			if r.Header.Get("Authorization") != "Bearer vw_test" {
				t.Errorf("download auth")
			}
			_, _ = w.Write([]byte("mp4"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "role.png")
	if err := os.WriteFile(src, []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "clip", "intro.mp4")
	var msgs []string
	got, err := intro.GenerateIntro(context.Background(), intro.IntroOptions{
		ImagePath: src,
		Prompt:    intro.DefaultIntroPrompt,
		OutPath:   out,
		APIKey:    "vw_test",
		BaseURL:   srv.URL,
		HTTP:      srv.Client(),
		PollEvery: 5 * time.Millisecond,
		OnProgress: func(percent int, msg string) {
			msgs = append(msgs, msg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != out || gotPrompt != intro.DefaultIntroPrompt || gotModel != intro.VideoModel || gotDuration != intro.VideoDuration || gotRatio != intro.VideoRatio {
		t.Fatalf("prompt=%q model=%s dur=%v ratio=%s", gotPrompt, gotModel, gotDuration, gotRatio)
	}
	if !strings.HasPrefix(gotImage, "data:image/png;base64,") {
		t.Fatalf("image %s", gotImage)
	}
	body, err := os.ReadFile(out)
	if err != nil || string(body) != "mp4" {
		t.Fatalf("file %q err=%v", body, err)
	}
	joined := strings.Join(msgs, "|")
	if !strings.Contains(joined, "已按默认生成") || !strings.Contains(joined, "介绍片已保存") {
		t.Fatalf("progress %v", msgs)
	}
}

func TestGenerateIntroFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "job2", "status": "queued"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "job2", "status": "failed", "error": map[string]string{"message": "上游失败"},
		})
	}))
	defer srv.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "a.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := intro.GenerateIntro(context.Background(), intro.IntroOptions{
		ImagePath: src,
		Prompt:    "片头",
		APIKey:    "vw",
		BaseURL:   srv.URL,
		HTTP:      srv.Client(),
		PollEvery: time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "上游失败") {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerateIntroQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"quota_exceeded","message":"余额不足"}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "a.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := intro.GenerateIntro(context.Background(), intro.IntroOptions{
		ImagePath: src,
		Prompt:    "片头",
		APIKey:    "vw",
		BaseURL:   srv.URL,
		HTTP:      srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "余额不足") {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerateFourViewNoImageAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"No available compatible accounts","type":"api_error"}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "a.png")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := intro.GenerateFourView(context.Background(), intro.FourViewOptions{
		ImagePath: src,
		BaseURL:   srv.URL + "/v1",
		APIKey:    "sk",
		HTTP:      srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "没有可用的生图账号") {
		t.Fatalf("err=%v", err)
	}
}

func TestMissingKeys(t *testing.T) {
	if _, err := intro.GenerateFourView(context.Background(), intro.FourViewOptions{ImagePath: "a.png"}); err == nil || !strings.Contains(err.Error(), "复刻 API Key") {
		t.Fatal(err)
	}
	if _, err := intro.GenerateIntro(context.Background(), intro.IntroOptions{ImagePath: "a.png", Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "视频 API Key") {
		t.Fatal(err)
	}
}

func TestPromptTemplates(t *testing.T) {
	tpls := intro.PromptTemplates()
	if len(tpls) != 5 {
		t.Fatalf("len=%d", len(tpls))
	}
	seen := map[string]bool{}
	for _, tpl := range tpls {
		if tpl.Name == "" || !strings.Contains(tpl.Text, "15秒") || !strings.Contains(tpl.Text, "台词") {
			t.Fatalf("template %#v", tpl.Name)
		}
		if seen[tpl.Text] {
			t.Fatalf("duplicate text %s", tpl.Name)
		}
		seen[tpl.Text] = true
	}
	if intro.DefaultIntroPrompt != tpls[0].Text {
		t.Fatal("default prompt should be the first template")
	}
	if intro.PromptByName("没有这套") != tpls[0].Text || intro.PromptByName("冷面复仇") != tpls[4].Text {
		t.Fatal("lookup")
	}
}

func TestOutputDir(t *testing.T) {
	got := intro.OutputDir(`E:\clips\role.png`, time.Date(2026, 10, 15, 10, 46, 0, 0, time.UTC))
	if !strings.HasSuffix(filepath.ToSlash(got), "clips/intro_20261015_104600") {
		t.Fatalf("got %s", got)
	}
}
