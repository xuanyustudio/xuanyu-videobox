package media_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
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

func generateTestSource(t *testing.T, bins media.FFmpegBins, outPath string) {
	t.Helper()
	ctx := context.Background()
	_, err := media.Run(ctx, bins.FFmpeg,
		"-y",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=5",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-t", "5",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23",
		"-c:a", "aac",
		"-shortest",
		outPath,
	)
	if err != nil {
		t.Fatalf("generate test source: %v", err)
	}
}

func probeDurationSec(t *testing.T, bins media.FFmpegBins, path string) float64 {
	t.Helper()
	if bins.FFprobe == "" {
		t.Skip("ffprobe not available")
	}
	out, err := media.Run(context.Background(), bins.FFprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	if err != nil {
		t.Fatalf("ffprobe duration: %v", err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil {
		t.Fatalf("parse duration %q: %v", out, err)
	}
	return d
}

func TestPipelineClipConcat(t *testing.T) {
	bins := ffmpegBinsOrSkip(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "testsrc.mp4")
	generateTestSource(t, bins, src)

	ctx := context.Background()
	clip1 := filepath.Join(dir, "clip1.mp4")
	if err := media.ClipSegment(ctx, bins, src, "00:00:00,000-00:00:05,000", 0, 1.2, false, clip1); err != nil {
		t.Fatalf("ClipSegment OST0: %v", err)
	}
	if _, err := os.Stat(clip1); err != nil {
		t.Fatalf("clip1 missing: %v", err)
	}
	d1 := probeDurationSec(t, bins, clip1)
	if d1 < 1.9 {
		t.Fatalf("clip1 duration too short: %.3f", d1)
	}

	clip2 := filepath.Join(dir, "clip2.mp4")
	if err := media.ClipSegment(ctx, bins, src, "00:00:01,000-00:00:03,000", 0, 1.0, false, clip2); err != nil {
		t.Fatalf("ClipSegment OST0 #2: %v", err)
	}

	merged := filepath.Join(dir, "merged.mp4")
	if err := media.ConcatVideos(ctx, bins, []string{clip1, clip2}, merged); err != nil {
		t.Fatalf("ConcatVideos: %v", err)
	}
	if _, err := os.Stat(merged); err != nil {
		t.Fatalf("merged missing: %v", err)
	}
}

func TestMaskEnableExpr(t *testing.T) {
	expr := media.MaskEnableExpr([][2]float64{{5, 13}})
	if !strings.Contains(expr, "not(") || !strings.Contains(expr, "between") {
		t.Fatalf("unexpected expr: %q", expr)
	}
	if media.MaskEnableExpr(nil) != "" {
		t.Fatal("empty ranges should yield empty expr")
	}
}
