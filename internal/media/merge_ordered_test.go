package media

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildMergeFilterSilentClipUsesExtraInput(t *testing.T) {
	got := buildMergeFilter([]clipGeometry{
		{Width: 320, Height: 240, FPS: "30", HasAudio: true, Duration: 2},
		{Width: 160, Height: 120, FPS: "25", HasAudio: false, Duration: 3.5},
	}, mergeTarget{Width: 320, Height: 240, FPS: "30"})
	for _, want := range []string{
		"scale=320:240",
		"[0:a]",
		"[2:a]",
		"apad=whole_dur=3.5",
		"concat=n=2:v=1:a=1[vout][aout]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "[1:a]") {
		t.Fatalf("silent clip should not use its own audio input: %s", got)
	}
}

func TestTargetFromClipsUsesLargest(t *testing.T) {
	target := targetFromClips([]clipGeometry{
		{Width: 1919, Height: 1080, FPS: "25"},
		{Width: 1280, Height: 720, FPS: "30"},
	})
	if target.Width != 1920 || target.Height != 1080 || target.FPS != "25" {
		t.Fatalf("%+v", target)
	}
}

func TestNormalizeFPS(t *testing.T) {
	if got := normalizeFPS("30/1"); got != "30" {
		t.Fatalf("30/1: %s", got)
	}
	if got := normalizeFPS("30000/1001"); got != "29.97" {
		t.Fatalf("ntsc: %s", got)
	}
	if got := normalizeFPS("0/0"); got != "30" {
		t.Fatalf("zero: %s", got)
	}
	if got := normalizeFPS("240"); got != "30" {
		t.Fatalf("too fast: %s", got)
	}
}

func TestMergeProgressPercent(t *testing.T) {
	pct, ok := mergeProgressPercent("out_time=00:00:01.000000", 4)
	if !ok || pct != 25 {
		t.Fatalf("quarter: %d %v", pct, ok)
	}
	pct, ok = mergeProgressPercent("progress=end", 4)
	if !ok || pct != 100 {
		t.Fatalf("end: %d %v", pct, ok)
	}
	if _, ok = mergeProgressPercent("out_time=N/A", 4); ok {
		t.Fatal("N/A should be ignored")
	}
}

func TestMergeOrderedVideosDifferentSizes(t *testing.T) {
	bins := mergeTestBins(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "small.mp4")
	second := filepath.Join(dir, "wide.mp4")
	generateColorClip(t, bins, first, "black", "160x120", 2, true)
	generateColorClip(t, bins, second, "red", "320x240", 3, false)

	out := filepath.Join(dir, "merged.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := MergeOrderedVideos(ctx, bins, []string{first, second}, out, nil); err != nil {
		t.Fatalf("MergeOrderedVideos: %v", err)
	}
	w, h := probeVideoSize(t, bins, out)
	if w != 320 || h != 240 {
		t.Fatalf("size = %dx%d", w, h)
	}
	d, err := ProbeDurationSec(ctx, bins, out)
	if err != nil {
		t.Fatalf("duration: %v", err)
	}
	if d < 4.5 || d > 5.8 {
		t.Fatalf("duration = %.3f", d)
	}
}

func mergeTestBins(t *testing.T) FFmpegBins {
	t.Helper()
	ff, err := ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	fp, err := ResolveFFprobe("", "")
	if err != nil {
		t.Skip(err)
	}
	return FFmpegBins{FFmpeg: ff, FFprobe: fp}
}

func generateColorClip(t *testing.T, bins FFmpegBins, path, color, size string, seconds int, withAudio bool) {
	t.Helper()
	args := []string{
		"-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=%s:s=%s:d=%d", color, size, seconds),
	}
	if withAudio {
		args = append(args,
			"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
			"-map", "0:v", "-map", "1:a",
			"-shortest",
		)
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-t", strconv.Itoa(seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		path,
	)
	if _, err := Run(context.Background(), bins.FFmpeg, args...); err != nil {
		t.Fatalf("generate %s: %v", path, err)
	}
}

func probeVideoSize(t *testing.T, bins FFmpegBins, path string) (int, int) {
	t.Helper()
	out, err := Run(context.Background(), bins.FFprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		path,
	)
	if err != nil {
		t.Fatalf("probe size: %v", err)
	}
	parts := strings.Split(strings.TrimSpace(out), ",")
	if len(parts) < 2 {
		t.Fatalf("size csv: %q", out)
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil {
		t.Fatalf("parse size %q", out)
	}
	return w, h
}
