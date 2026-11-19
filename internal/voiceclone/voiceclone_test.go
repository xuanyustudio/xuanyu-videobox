package voiceclone

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAndNewVoiceID(t *testing.T) {
	if err := ValidateVoiceID("abc"); err == nil {
		t.Fatal("short id should fail")
	}
	if err := ValidateVoiceID("1abcdefg"); err == nil {
		t.Fatal("id starting with digit should fail")
	}
	if err := ValidateVoiceID("MiniMax-"); err == nil {
		t.Fatal("id ending with hyphen should fail")
	}
	if err := ValidateVoiceID("MiniMax001"); err != nil {
		t.Fatal(err)
	}
	id := NewVoiceID()
	if err := ValidateVoiceID(id); err != nil {
		t.Fatalf("generated %q: %v", id, err)
	}
	if !strings.HasPrefix(id, "Xuanyu") {
		t.Fatalf("prefix: %s", id)
	}
	if NormalizeVoiceID("  MiniMax001 ") != "MiniMax001" {
		t.Fatal("trim")
	}
	got := NormalizeVoiceID("")
	if err := ValidateVoiceID(got); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := NormalizeBaseURL(""); got != DefaultBaseURL {
		t.Fatalf("empty=%q", got)
	}
	if got := NormalizeBaseURL(" https://api.minimax.io/ "); got != "https://api.minimax.io" {
		t.Fatalf("got %q", got)
	}
}

func TestCloneAndSynthesize(t *testing.T) {
	audio := bytes.Repeat([]byte("mp3"), 32)
	var uploadedPurpose string
	var uploadedName string
	var cloneBody map[string]any
	var t2aBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer testdevkey" {
			t.Errorf("auth=%q", got)
		}
		switch r.URL.Path {
		case "/v1/files/upload":
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Error(err)
				http.Error(w, err.Error(), 400)
				return
			}
			uploadedPurpose = r.FormValue("purpose")
			_, hdr, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				http.Error(w, err.Error(), 400)
				return
			}
			uploadedName = hdr.Filename
			_ = json.NewEncoder(w).Encode(map[string]any{
				"file":      map[string]any{"file_id": 123456789},
				"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
			})
		case "/v1/voice_clone":
			if err := json.NewDecoder(r.Body).Decode(&cloneBody); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"demo_audio": "http://example.invalid/demo.mp3",
				"base_resp":  map[string]any{"status_code": 0, "status_msg": "success"},
			})
		case "/v1/t2a_v2":
			if err := json.NewDecoder(r.Body).Decode(&t2aBody); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"audio": hex.EncodeToString(audio), "status": 2},
				"extra_info": map[string]any{"audio_length": 1200, "usage_characters": 8, "audio_format": "mp3"},
				"base_resp":  map[string]any{"status_code": 0, "status_msg": "success"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp3")
	if err := os.WriteFile(src, audio, 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Client{BaseURL: srv.URL, APIKey: "testdevkey", Model: "speech-2.8-hd", HTTP: srv.Client()}
	ctx := context.Background()
	fileID, err := c.UploadVoiceClone(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if fileID != "123456789" || uploadedPurpose != "voice_clone" || uploadedName != "clip.mp3" {
		t.Fatalf("upload file_id=%s purpose=%s name=%s", fileID, uploadedPurpose, uploadedName)
	}
	cloned, err := c.CloneVoice(ctx, fileID, "MiniMax001", CloneOptions{Text: "你好，这是试听。", NeedNoiseReduction: true})
	if err != nil {
		t.Fatal(err)
	}
	if cloned.VoiceID != "MiniMax001" || cloned.DemoAudio == "" {
		t.Fatalf("%#v", cloned)
	}
	if cloneBody["voice_id"] != "MiniMax001" || cloneBody["text"] != "你好，这是试听。" {
		t.Fatalf("clone body %#v", cloneBody)
	}
	if _, ok := cloneBody["file_id"].(float64); !ok && fmtNum(cloneBody["file_id"]) != "123456789" {
		t.Fatalf("file_id not numeric: %#v", cloneBody["file_id"])
	}
	syn, err := c.Synthesize(ctx, cloned.VoiceID, "请用这个音色说话。")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(syn.Audio, audio) || syn.Format != "mp3" || syn.LengthMS != 1200 {
		t.Fatalf("%#v", syn)
	}
	vs, _ := t2aBody["voice_setting"].(map[string]any)
	if vs == nil || vs["voice_id"] != "MiniMax001" {
		t.Fatalf("t2a body %#v", t2aBody)
	}
}

func fmtNum(v any) string {
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func TestCloneAuthError(t *testing.T) {
	c := &Client{}
	if _, err := c.UploadVoiceClone(context.Background(), "x.mp3"); err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("err=%v", err)
	}
}

func TestMapStatus(t *testing.T) {
	err := baseResp{StatusCode: 2038, StatusMsg: "no permission"}.err()
	if err == nil || !strings.Contains(err.Error(), "认证") {
		t.Fatalf("err=%v", err)
	}
	if err := (baseResp{StatusCode: 0}).err(); err != nil {
		t.Fatal(err)
	}
}

func TestMustJSONNumber(t *testing.T) {
	n, ok := mustJSONNumber("123456789").(json.Number)
	if !ok || n.String() != "123456789" {
		t.Fatalf("got %#v", mustJSONNumber("123456789"))
	}
	if mustJSONNumber("abc") != "abc" {
		t.Fatal("string id")
	}
}

func TestDownloadDemo(t *testing.T) {
	payload := []byte("ID3demo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "demo.mp3")
	if err := DownloadDemo(context.Background(), srv.Client(), srv.URL+"/demo.mp3", dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}
	_ = io.Discard
}

func TestListVoices(t *testing.T) {
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/get_voice" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		gotType, _ = body["voice_type"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"system_voice": []any{
				map[string]any{
					"voice_id":     "Chinese (Mandarin)_News_Anchor",
					"voice_name":   "新闻女声",
					"description":  []string{"专业播音腔"},
					"created_time": "1970-01-01",
				},
			},
			"voice_cloning": []any{
				map[string]any{
					"voice_id":     "test12345",
					"description":  []any{},
					"created_time": "2025-08-20",
				},
			},
			"voice_generation": []any{
				map[string]any{
					"voice_id":     "ttv-voice-abc",
					"description":  "文生音色",
					"created_time": "2025-08-21",
				},
			},
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "testdevkey", HTTP: srv.Client()}
	voices, err := c.ListVoices(context.Background(), VoiceTypeAll)
	if err != nil {
		t.Fatal(err)
	}
	if gotType != VoiceTypeAll {
		t.Fatalf("voice_type=%q", gotType)
	}
	if len(voices) != 3 {
		t.Fatalf("len=%d %#v", len(voices), voices)
	}
	if voices[0].Kind != VoiceTypeCloning || voices[0].ID != "test12345" || voices[0].CreatedTime != "2025-08-20" {
		t.Fatalf("cloning %#v", voices[0])
	}
	if voices[1].Kind != VoiceTypeGeneration || voices[1].Description != "文生音色" {
		t.Fatalf("generation %#v", voices[1])
	}
	if voices[2].Kind != VoiceTypeSystem || voices[2].Name != "新闻女声" {
		t.Fatalf("system %#v", voices[2])
	}
	if !strings.Contains(voices[2].Display(), "新闻女声") || !strings.Contains(voices[0].Display(), "复刻") {
		t.Fatalf("display %q %q", voices[0].Display(), voices[2].Display())
	}
	cloned := FilterVoices(voices, VoiceTypeCloning)
	if len(cloned) != 1 || cloned[0].ID != "test12345" {
		t.Fatalf("filter %#v", cloned)
	}
}

func TestListVoicesRequiresKey(t *testing.T) {
	c := &Client{}
	if _, err := c.ListVoices(context.Background(), VoiceTypeAll); err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("err=%v", err)
	}
}
