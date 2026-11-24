package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitNarrationRespectsLimitAndPunctuation(t *testing.T) {
	short := SplitNarration("你好，世界。", 480)
	if len(short) != 1 || short[0] != "你好，世界。" {
		t.Fatalf("short: %#v", short)
	}
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("这是第")
		b.WriteString(strings.Repeat("一段话", 8))
		b.WriteString("。")
	}
	parts := SplitNarration(b.String(), 80)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	for i, p := range parts {
		if len([]rune(p)) > 80 {
			t.Fatalf("chunk %d too long (%d): %q", i, len([]rune(p)), p)
		}
	}
	joined := strings.Join(parts, "")
	if !strings.Contains(joined, "这是第") {
		t.Fatal("lost text")
	}
}

func TestConcatWAVFilesAndDuration(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.wav")
	b := filepath.Join(dir, "b.wav")
	out := filepath.Join(dir, "out.wav")
	// 24000 Hz, 2400 samples = 0.1s each
	sa := make([]int16, 2400)
	sb := make([]int16, 2400)
	for i := range sa {
		sa[i] = 100
		sb[i] = 200
	}
	if err := WritePCM16WAV(a, 24000, sa); err != nil {
		t.Fatal(err)
	}
	if err := WritePCM16WAV(b, 24000, sb); err != nil {
		t.Fatal(err)
	}
	if err := ConcatWAVFiles([]string{a, b}, out); err != nil {
		t.Fatal(err)
	}
	dur, err := WAVDurationSec(out)
	if err != nil {
		t.Fatal(err)
	}
	if dur < 0.19 || dur > 0.21 {
		t.Fatalf("duration %v", dur)
	}
}

func TestKokoroClientChunksAndConcat(t *testing.T) {
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/voices":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "zf_001", "name": "女声 001 (温柔)"}})
		case "/api/tts":
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Text  string  `json:"text"`
				Voice string  `json:"voice"`
				Speed float64 `json:"speed"`
			}
			_ = json.Unmarshal(body, &req)
			if req.Voice != "zf_001" {
				t.Errorf("voice=%s", req.Voice)
			}
			if req.Speed != 1.2 {
				t.Errorf("speed=%v", req.Speed)
			}
			if len([]rune(req.Text)) > KokoroMaxRunes {
				http.Error(w, `{"error":"文本不能超过500字"}`, 400)
				return
			}
			texts = append(texts, req.Text)
			samples := make([]int16, 240)
			tmp, err := os.CreateTemp("", "k-*.wav")
			if err != nil {
				t.Error(err)
				return
			}
			tmp.Close()
			defer os.Remove(tmp.Name())
			if err := WritePCM16WAV(tmp.Name(), 24000, samples); err != nil {
				t.Error(err)
				return
			}
			raw, _ := os.ReadFile(tmp.Name())
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	list, err := ListKokoroVoices(context.Background(), srv.URL)
	if err != nil || len(list) != 1 || list[0].ID != "zf_001" {
		t.Fatalf("list: %v %#v", err, list)
	}

	var long strings.Builder
	for i := 0; i < 30; i++ {
		long.WriteString("你好世界，这是一段用来测试拆分的解说词。")
	}
	if len([]rune(long.String())) <= KokoroMaxRunes {
		t.Fatal("test text should exceed limit")
	}
	out := filepath.Join(t.TempDir(), "kokoro.wav")
	k := Kokoro{BaseURL: srv.URL, Params: VoiceParams{Speed: 1.2}}
	dur, err := k.Synthesize(context.Background(), long.String(), "zf_001", out)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) < 2 {
		t.Fatalf("expected chunked requests, got %d", len(texts))
	}
	if dur <= 0 {
		t.Fatalf("duration %v", dur)
	}
	if st, err := os.Stat(out); err != nil || st.Size() < 44 {
		t.Fatalf("out file: %v %#v", err, st)
	}
}

func TestKokoroUnreachableMessage(t *testing.T) {
	k := Kokoro{BaseURL: "http://127.0.0.1:1", Params: VoiceParams{Speed: 1}}
	_, err := k.Synthesize(context.Background(), "你好", "zf_001", filepath.Join(t.TempDir(), "x.wav"))
	if err == nil || !strings.Contains(err.Error(), "启动Kokoro.bat") {
		t.Fatalf("want bat hint, got %v", err)
	}
}

func TestOpenVoiceCloneRequiresReference(t *testing.T) {
	o := OpenVoice{BaseURL: "http://127.0.0.1:3212"}
	_, err := o.Synthesize(context.Background(), "你好", "clone", filepath.Join(t.TempDir(), "x.wav"))
	if err == nil || !strings.Contains(err.Error(), "参考音频") {
		t.Fatalf("want reference error, got %v", err)
	}
}

func TestOpenVoiceClientPostsReference(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		samples := make([]int16, 120)
		tmp := filepath.Join(t.TempDir(), "ov.wav")
		if err := WritePCM16WAV(tmp, 22050, samples); err != nil {
			t.Error(err)
			return
		}
		raw, _ := os.ReadFile(tmp)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "out.wav")
	o := OpenVoice{BaseURL: srv.URL, Reference: `E:\voices\ref.wav`, Params: VoiceParams{Speed: 0.9}}
	if _, err := o.Synthesize(context.Background(), "你好", "克隆音色", out); err != nil {
		t.Fatal(err)
	}
	if got["voice"] != OpenVoiceCloneID || got["reference"] != `E:\voices\ref.wav` {
		t.Fatalf("payload %#v", got)
	}
	if got["speed"].(float64) != 0.9 {
		t.Fatalf("speed %#v", got["speed"])
	}
}

func TestNormalizeEngineLocal(t *testing.T) {
	if NormalizeEngine("Kokoro") != EngineKokoro {
		t.Fatal("kokoro")
	}
	if NormalizeEngine("openvoice") != EngineOpenVoice {
		t.Fatal("openvoice")
	}
	if NormalizeEngine("IndexTTS-2") != EngineIndexTTS2 && NormalizeEngine("indextts2") != EngineIndexTTS2 {
		t.Fatal("indextts2")
	}
	if NormalizeEngine("indextts2") != EngineIndexTTS2 {
		t.Fatal("indextts2 id")
	}
	if NormalizeEngine("minimax") != EngineMiniMax {
		t.Fatal("minimax")
	}
	if NormalizeEngine("") != EngineEdge {
		t.Fatal("edge default")
	}
}

func TestIndexTTS2CloneRequiresReference(t *testing.T) {
	x := IndexTTS2{BaseURL: "http://127.0.0.1:3213"}
	_, err := x.Synthesize(context.Background(), "你好", "clone", filepath.Join(t.TempDir(), "x.wav"))
	if err == nil || !strings.Contains(err.Error(), "参考音频") {
		t.Fatalf("want reference error, got %v", err)
	}
}

func TestIndexTTS2ExampleUsesNoReference(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/voices":
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"id": IndexTTS2ExampleID, "label": IndexTTS2ExampleLabel},
			})
		case "/api/tts":
			_ = json.NewDecoder(r.Body).Decode(&got)
			tmp := filepath.Join(t.TempDir(), "ix.wav")
			if err := WritePCM16WAV(tmp, 22050, make([]int16, 120)); err != nil {
				t.Error(err)
				return
			}
			raw, _ := os.ReadFile(tmp)
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	list, err := ListIndexTTS2Voices(context.Background(), srv.URL)
	if err != nil || len(list) < 2 {
		t.Fatalf("list: %v %#v", err, list)
	}
	if list[0].ID != IndexTTS2VoiceID("voice_01") || list[0].Label != "官方示例 01" {
		t.Fatalf("first official example: %#v", list[0])
	}
	if list[len(list)-1].ID != IndexTTS2CloneID {
		t.Fatalf("clone should be last: %#v", list[len(list)-1])
	}
	out := filepath.Join(t.TempDir(), "out.wav")
	x := IndexTTS2{BaseURL: srv.URL, Params: VoiceParams{Speed: 1.1}}
	if _, err := x.Synthesize(context.Background(), "你好", IndexTTS2ExampleLabel, out); err != nil {
		t.Fatal(err)
	}
	if got["voice"] != IndexTTS2VoiceID("voice_01") {
		t.Fatalf("payload %#v", got)
	}
	if ref, ok := got["reference"].(string); ok && ref != "" {
		if !strings.Contains(ref, "voice_01") && !strings.Contains(ref, "examples") {
			t.Fatalf("example should not send a custom reference: %#v", got)
		}
	}
	if got["speed"].(float64) != 1.1 {
		t.Fatalf("speed %#v", got["speed"])
	}
}

func TestBuiltinIndexTTS2Voices(t *testing.T) {
	list := BuiltinIndexTTS2Voices()
	if len(list) != 12 {
		t.Fatalf("want 11 official examples + clone, got %d", len(list))
	}
	if list[0].ID != "indextts2_voice_01" || list[0].Label != "官方示例 01" {
		t.Fatalf("first %#v", list[0])
	}
	if list[9].ID != "indextts2_voice_11" {
		t.Fatalf("voice_11 at index 9: %#v", list[9])
	}
	if list[11].ID != IndexTTS2CloneID || list[11].Label != IndexTTS2CloneLabel {
		t.Fatalf("clone %#v", list[11])
	}
	if IndexTTS2IDForLabel("官方示例 07") != "indextts2_voice_07" {
		t.Fatal("label 07")
	}
	if IndexTTS2IDForLabel(IndexTTS2ExampleLabel) != "indextts2_voice_01" {
		t.Fatal("legacy example label")
	}
	if !IsIndexTTS2ID("官方示例 12") || !IsIndexTTS2ID("indextts2_voice_03") {
		t.Fatal("IsIndexTTS2ID")
	}
}

func TestIndexTTS2PresetSendsExamplePath(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "voice_07.wav")
	if err := WritePCM16WAV(wav, 22050, make([]int16, 80)); err != nil {
		t.Fatal(err)
	}
	old := IndexTTS2ExamplesDir
	IndexTTS2ExamplesDir = dir
	defer func() { IndexTTS2ExamplesDir = old }()

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		tmp := filepath.Join(t.TempDir(), "ix.wav")
		if err := WritePCM16WAV(tmp, 22050, make([]int16, 120)); err != nil {
			t.Error(err)
			return
		}
		raw, _ := os.ReadFile(tmp)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "out.wav")
	x := IndexTTS2{BaseURL: srv.URL, Reference: `E:\voices\user-ref.wav`, Params: VoiceParams{Speed: 1}}
	if _, err := x.Synthesize(context.Background(), "你好", "官方示例 07", out); err != nil {
		t.Fatal(err)
	}
	if got["voice"] != "indextts2_voice_07" {
		t.Fatalf("voice %#v", got)
	}
	if got["reference"] != wav {
		t.Fatalf("should use official example, not user clone ref: %#v", got)
	}
}

func TestIndexTTS2ClientPostsReference(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		tmp := filepath.Join(t.TempDir(), "ix.wav")
		if err := WritePCM16WAV(tmp, 22050, make([]int16, 120)); err != nil {
			t.Error(err)
			return
		}
		raw, _ := os.ReadFile(tmp)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()
	out := filepath.Join(t.TempDir(), "out.wav")
	x := IndexTTS2{BaseURL: srv.URL, Reference: `E:\voices\ref.wav`, Params: VoiceParams{Speed: 0.9}}
	if _, err := x.Synthesize(context.Background(), "你好", "克隆音色", out); err != nil {
		t.Fatal(err)
	}
	if got["voice"] != IndexTTS2CloneID || got["reference"] != `E:\voices\ref.wav` {
		t.Fatalf("payload %#v", got)
	}
}

func TestIndexTTS2UnreachableMessage(t *testing.T) {
	x := IndexTTS2{BaseURL: "http://127.0.0.1:1", Params: VoiceParams{Speed: 1}}
	_, err := x.Synthesize(context.Background(), "你好", IndexTTS2ExampleID, filepath.Join(t.TempDir(), "x.wav"))
	if err == nil || !strings.Contains(err.Error(), "启动IndexTTS2.bat") {
		t.Fatalf("want bat hint, got %v", err)
	}
}
