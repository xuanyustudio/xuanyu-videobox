package absplice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/narratoai/narrato-go/internal/media"
)

func TestAnalyzeVideo_SampleOut(t *testing.T) {
	root := findRepoRoot(t)
	video := filepath.Join(root, "dist", "abtest2", "out.mp4")
	if _, err := os.Stat(video); err != nil {
		t.Skip("sample out.mp4 not present")
	}
	ffprobe, err := media.ResolveFFprobe(filepath.Join(root, "dist", "ffprobe.exe"), filepath.Join(root, "dist"))
	if err != nil {
		t.Skip("ffprobe not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := AnalyzeVideo(ctx, media.FFmpegBins{FFprobe: ffprobe}, video)
	if err != nil {
		t.Fatal(err)
	}
	if info.FrameCount != 25 {
		t.Fatalf("frame count: got %d want 25", info.FrameCount)
	}
	if info.KeyFrames != 1 {
		t.Fatalf("key frames: got %d want 1", info.KeyFrames)
	}
	if info.GOPCount != 1 {
		t.Fatalf("gop count: got %d want 1", info.GOPCount)
	}
	if info.IFrames != 1 || info.PFrames != 24 {
		t.Fatalf("I/P = %d/%d, want 1/24", info.IFrames, info.PFrames)
	}
	report := info.FormatReport(1.0, 25)
	if report == "" {
		t.Fatal("empty report")
	}
	t.Log("\n" + report)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found")
	return ""
}
