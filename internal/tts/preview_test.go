package tts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/tts"
)

func TestPreviewTextUsesFirstNarration(t *testing.T) {
	items := []script.Item{
		{OST: 1, Narration: "原片对白不应用于试听"},
		{OST: 0, Narration: "  这是第一句解说。  "},
		{OST: 2, Narration: "第二句"},
	}
	got := tts.PreviewText(items, 80)
	if got != "这是第一句解说。" {
		t.Fatalf("got %q", got)
	}
}

func TestPreviewTextFallbackAndTruncate(t *testing.T) {
	if got := tts.PreviewText(nil, 80); got != tts.DefaultPreviewText {
		t.Fatalf("empty: %q", got)
	}
	long := stringsRepeat("测", 100)
	got := tts.PreviewText([]script.Item{{OST: 0, Narration: long}}, 80)
	if len([]rune(got)) != 80 {
		t.Fatalf("len=%d", len([]rune(got)))
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]rune, 0, n)
	r := []rune(s)[0]
	for i := 0; i < n; i++ {
		out = append(out, r)
	}
	return string(out)
}

func TestDirectPreviewPathOfficial(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "voice_05.wav")
	payload := append([]byte("RIFF"), make([]byte, 60)...)
	if err := os.WriteFile(wav, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	old := tts.IndexTTS2ExamplesDir
	tts.IndexTTS2ExamplesDir = dir
	defer func() { tts.IndexTTS2ExamplesDir = old }()

	path, ok, err := tts.DirectPreviewPath(tts.EngineIndexTTS2, "官方示例 05", "")
	if err != nil || !ok || path != wav {
		t.Fatalf("official: path=%q ok=%v err=%v", path, ok, err)
	}
	if _, ok, err := tts.DirectPreviewPath(tts.EngineEdge, "zh-CN-XiaoxiaoNeural", ""); err != nil || ok {
		t.Fatalf("edge should synthesize, ok=%v err=%v", ok, err)
	}
}

func TestDirectPreviewPathClone(t *testing.T) {
	_, ok, err := tts.DirectPreviewPath(tts.EngineIndexTTS2, tts.IndexTTS2CloneID, "")
	if ok || err == nil || !strings.Contains(err.Error(), "参考音频") {
		t.Fatalf("clone without ref: ok=%v err=%v", ok, err)
	}
	dir := t.TempDir()
	ref := filepath.Join(dir, "mine.mp3")
	if err := os.WriteFile(ref, []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok, err := tts.DirectPreviewPath(tts.EngineIndexTTS2, "克隆音色", ref)
	if err != nil || !ok || path != ref {
		t.Fatalf("clone with ref: path=%q ok=%v err=%v", path, ok, err)
	}
}
