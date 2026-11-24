package asr

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLocalFunASRPreset(t *testing.T) {
	backend, apiURL := LocalFunASRPreset()
	if backend != BackendLocal {
		t.Fatalf("backend=%q", backend)
	}
	if apiURL != DefaultAPIURL {
		t.Fatalf("apiURL=%q", apiURL)
	}
	if DefaultAPIURL != "http://127.0.0.1:7860" {
		t.Fatalf("default url=%q", DefaultAPIURL)
	}
	if NormalizeBackend(backend) != BackendLocal {
		t.Fatal("preset backend should stay on the local FunASR path")
	}
	if BackendLabel(BackendBailian) != "阿里百炼 Fun-ASR" {
		t.Fatalf("bailian label changed: %q", BackendLabel(BackendBailian))
	}
	if BackendLabel(BackendFireRed) != "本地 FireRedASR" {
		t.Fatalf("firered label changed: %q", BackendLabel(BackendFireRed))
	}
	if BackendLabel(BackendLocal) != LabelLocalFunASR || !strings.Contains(LabelLocalFunASR, "SenseVoiceSmall") {
		t.Fatalf("local label=%q", BackendLabel(BackendLocal))
	}
}

func TestEnsureLocalASRURLs(t *testing.T) {
	if EnsureFunASRURL("") != DefaultAPIURL || EnsureFunASRURL("  ") != DefaultAPIURL {
		t.Fatalf("empty funasr url=%q", EnsureFunASRURL(""))
	}
	if EnsureFunASRURL(" http://10.0.0.8:7860 ") != "http://10.0.0.8:7860" {
		t.Fatal("custom funasr url was overwritten")
	}
	if EnsureFireRedURL("") != DefaultFireRedAPIURL {
		t.Fatal("empty firered url")
	}
	if EnsureFireRedURL("http://127.0.0.1:9000") != "http://127.0.0.1:9000" {
		t.Fatal("custom firered url was overwritten")
	}
}

func TestFormatLocalProbeStatus(t *testing.T) {
	ok := FormatLocalProbeStatus(nil)
	if !strings.Contains(ok, "已连接") || !strings.Contains(ok, "SenseVoiceSmall") {
		t.Fatalf("connected status=%q", ok)
	}
	refused := friendlyDialError(fmt.Errorf("dial tcp 127.0.0.1:7860: connect: connection refused"), DefaultAPIURL)
	status := FormatLocalProbeStatus(refused)
	if !strings.HasPrefix(status, "未启动") {
		t.Fatalf("status=%q", status)
	}
	if !strings.Contains(status, FunASRPackDownloadURL) {
		t.Fatalf("missing download hint: %q", status)
	}
	other := FormatLocalProbeStatus(fmt.Errorf("HTTP 500"))
	if !strings.HasPrefix(other, "未连接") || !strings.Contains(other, "HTTP 500") {
		t.Fatalf("other=%q", other)
	}
}

func TestProbeSuccessAndRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := (&Client{APIURL: srv.URL}).Probe(); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	err = (&Client{APIURL: "http://" + addr}).Probe()
	if err == nil {
		t.Fatal("expected connection refused")
	}
	status := FormatLocalProbeStatus(err)
	if !strings.HasPrefix(status, "未启动") || !strings.Contains(status, FunASRPackDownloadURL) {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if !strings.Contains(err.Error(), FunASRPackDownloadURL) || !strings.Contains(err.Error(), "无法连接") {
		t.Fatalf("err=%v", err)
	}
}

func TestTranscribeFilePostsSenseVoiceModel(t *testing.T) {
	var mu sync.Mutex
	var model, format, endpoint string
	var asrHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/asr" {
			mu.Lock()
			asrHits++
			mu.Unlock()
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/v1/audio/transcriptions" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		model = r.FormValue("model")
		format = r.FormValue("response_format")
		endpoint = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"字幕","segments":[{"start":0,"end":1.2,"text":"字幕"}]}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	media := filepath.Join(dir, "clip.wav")
	if err := os.WriteFile(media, []byte("RIFF....WAVEfmt"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "subtitles", "clip.srt")
	if err := (&Client{APIURL: srv.URL}).TranscribeFile(media, out); err != nil {
		t.Fatal(err)
	}
	if asrHits == 0 {
		t.Fatal("expected /asr to be tried first")
	}
	if model != SenseVoiceModel || SenseVoiceModel != "sensevoice" {
		t.Fatalf("model=%q const=%q", model, SenseVoiceModel)
	}
	if format != "verbose_json" || endpoint != "/v1/audio/transcriptions" {
		t.Fatalf("format=%q endpoint=%q", format, endpoint)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "字幕") {
		t.Fatalf("srt=%q", data)
	}
}

func TestTranscribeFilePrefersPackASR(t *testing.T) {
	var mu sync.Mutex
	var endpoint, filename, enableSpk, hotword string
	var openaiHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/audio/transcriptions" {
			mu.Lock()
			openaiHits++
			mu.Unlock()
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/asr" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		endpoint = r.URL.Path
		enableSpk = r.FormValue("enable_spk")
		hotword = r.FormValue("hotword")
		if f, hdr, err := r.FormFile("file"); err == nil {
			filename = hdr.Filename
			_ = f.Close()
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"你好","segments":[{"start":0,"end":1.0,"text":"你好"}]}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	media := filepath.Join(dir, "clip.wav")
	if err := os.WriteFile(media, []byte("RIFF....WAVEfmt"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.srt")
	c := &Client{APIURL: srv.URL, EnableSpk: true, Hotword: "二次恋爱"}
	if err := c.TranscribeFile(media, out); err != nil {
		t.Fatal(err)
	}
	if openaiHits != 0 {
		t.Fatalf("openai hits=%d, pack /asr should succeed first", openaiHits)
	}
	if endpoint != "/asr" || filename != "clip.wav" || enableSpk != "true" || hotword != "二次恋爱" {
		t.Fatalf("endpoint=%q file=%q spk=%q hw=%q", endpoint, filename, enableSpk, hotword)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "你好") {
		t.Fatalf("srt=%q", data)
	}
}

func TestTranscribeFileExtractsVideoAudio(t *testing.T) {
	var mu sync.Mutex
	var gotName, gotBody, endpoint string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/asr" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(f)
		_ = f.Close()
		mu.Lock()
		gotName = hdr.Filename
		gotBody = string(body)
		endpoint = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"抽出","segments":[{"start":0,"end":1.0,"text":"抽出"}]}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	media := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(media, []byte("not-a-real-media"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.srt")
	c := &Client{
		APIURL: srv.URL,
		extractAudio: func(src, dst string) error {
			if src != media {
				t.Fatalf("extract src=%q", src)
			}
			if !strings.HasSuffix(strings.ToLower(dst), ".wav") {
				t.Fatalf("extract dst=%q", dst)
			}
			return os.WriteFile(dst, []byte("FAKEWAV"), 0o644)
		},
	}
	if err := c.TranscribeFile(media, out); err != nil {
		t.Fatal(err)
	}
	if endpoint != "/asr" || gotBody != "FAKEWAV" {
		t.Fatalf("endpoint=%q body=%q name=%q", endpoint, gotBody, gotName)
	}
	if !strings.HasSuffix(strings.ToLower(gotName), ".wav") {
		t.Fatalf("uploaded name should be wav, got %q", gotName)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "抽出") {
		t.Fatalf("srt=%q", data)
	}
}
