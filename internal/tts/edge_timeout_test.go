package tts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEdgeSynthesizeHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := filepath.Join(t.TempDir(), "cancel.mp3")
	start := time.Now()
	_, err := (Edge{}).Synthesize(ctx, "你好", "zh-CN-XiaoxiaoNeural", out)
	if err == nil {
		t.Fatal("expected error on canceled context")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("canceled synthesize took too long: %s", time.Since(start))
	}
	msg := err.Error()
	if !strings.Contains(msg, "edge tts") {
		t.Fatalf("error should mention edge tts: %s", msg)
	}
}

func TestEdgeSynthesizeHonorsShortDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(3 * time.Millisecond)
	out := filepath.Join(t.TempDir(), "deadline.mp3")
	start := time.Now()
	_, err := (Edge{}).Synthesize(ctx, "你好", "zh-CN-XiaoxiaoNeural", out)
	if err == nil {
		t.Fatal("expected error on expired deadline")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("deadline synthesize took too long: %s", time.Since(start))
	}
	_ = os.Remove(out)
}
