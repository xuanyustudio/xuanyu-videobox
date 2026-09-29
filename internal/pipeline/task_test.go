package pipeline_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/pipeline"
	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/tts"
)

func ffmpegBinsOrSkip(t *testing.T) media.FFmpegBins {
	t.Helper()
	ff, err := media.ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	fp, _ := media.ResolveFFprobe("", "")
	return media.FFmpegBins{FFmpeg: ff, FFprobe: fp}
}

func generateTestSource8s(t *testing.T, bins media.FFmpegBins, outPath string) {
	t.Helper()
	ctx := context.Background()
	_, err := media.Run(ctx, bins.FFmpeg,
		"-y",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=8",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-t", "8",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23",
		"-c:a", "aac",
		"-shortest",
		outPath,
	)
	if err != nil {
		t.Fatalf("generate test source: %v", err)
	}
}

func TestRunPipelineSmoke(t *testing.T) {
	bins := ffmpegBinsOrSkip(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	generateTestSource8s(t, bins, src)

	workDir := filepath.Join(dir, "work")
	outPath := filepath.Join(dir, "output.mp4")

	items := []script.Item{
		{
			ID:        1,
			VideoID:   1,
			Timestamp: "00:00:00,000-00:00:05,000",
			Narration: "测试解说一二三四五六",
			OST:       0,
		},
		{
			ID:        2,
			VideoID:   1,
			Timestamp: "00:00:02,000-00:00:05,000",
			Narration: "播放原片",
			OST:       1,
		},
	}

	ctx := context.Background()
	got, err := pipeline.Run(ctx, pipeline.Params{
		Items:      items,
		VideoPaths: []string{src},
		WorkDir:    workDir,
		OutputPath: outPath,
		TTS:        tts.Fake{},
		Bins:       bins,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != outPath {
		t.Fatalf("output path: got %q want %q", got, outPath)
	}
	st, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if st.Size() <= 1000 {
		t.Fatalf("output too small: %d bytes", st.Size())
	}
}
