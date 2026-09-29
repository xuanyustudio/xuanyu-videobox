package tts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/tts"
)

func TestFakeSynthesizeWritesFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "a.wav")
	f := tts.Fake{}
	dur, err := f.Synthesize(context.Background(), "你好世界", "zh-CN-XiaoxiaoNeural", out)
	if err != nil || dur < 1 {
		t.Fatalf("%v %v", dur, err)
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatal("expected non-empty file")
	}
}

func TestEstimateDurationSec(t *testing.T) {
	if got := tts.EstimateDurationSec(""); got != 1 {
		t.Fatalf("empty text: got %v want 1", got)
	}
	if got := tts.EstimateDurationSec("你好世界"); got < 1 {
		t.Fatalf("got %v", got)
	}
}
