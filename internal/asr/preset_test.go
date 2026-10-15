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
}

func TestTranscribeFilePostsSenseVoiceModel(t *testing.T) {
	var mu sync.Mutex
	var model, format, endpoint string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
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
	media := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(media, []byte("not-a-real-media"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "subtitles", "clip.srt")
	if err := (&Client{APIURL: srv.URL}).TranscribeFile(media, out); err != nil {
		t.Fatal(err)
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
