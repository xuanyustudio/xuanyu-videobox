//go:build edge_live

package tts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/tts"
)

func TestEdgeSynthesizeLive(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "live.mp3")
	e := tts.Edge{}
	dur, err := e.Synthesize(context.Background(), "你好", "zh-CN-XiaoxiaoNeural", out)
	if err != nil {
		t.Fatal(err)
	}
	if dur < 1 {
		t.Fatalf("duration %v", dur)
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatal("expected non-empty audio file")
	}
}
