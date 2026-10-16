package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMP3Path(t *testing.T) {
	got := MP3Path(filepath.Join("D:", "clips", "开场.mp4"))
	want := filepath.Join("D:", "clips", "开场.mp3")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	same := MP3Path(filepath.Join("D:", "clips", "song.mp3"))
	if same == filepath.Join("D:", "clips", "song.mp3") || !strings.HasSuffix(same, "_audio.mp3") {
		t.Fatalf("mp3 input should not overwrite itself: %q", same)
	}
}

func TestExtractMP3(t *testing.T) {
	ff, err := ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	ctx := context.Background()
	if _, err := Run(ctx, ff,
		"-y",
		"-f", "lavfi", "-i", "color=c=black:s=160x120:d=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-shortest",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac",
		video,
	); err != nil {
		t.Fatal(err)
	}
	out, err := ExtractMP3(ctx, FFmpegBins{FFmpeg: ff}, video)
	if err != nil {
		t.Fatal(err)
	}
	if out != filepath.Join(dir, "clip.mp3") {
		t.Fatalf("out=%q", out)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() < 128 {
		t.Fatalf("mp3 too small: %d", st.Size())
	}
}

func TestExtractMP3NoAudio(t *testing.T) {
	ff, err := ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "silent.mp4")
	ctx := context.Background()
	if _, err := Run(ctx, ff,
		"-y",
		"-f", "lavfi", "-i", "color=c=black:s=160x120:d=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-an",
		video,
	); err != nil {
		t.Fatal(err)
	}
	_, err = ExtractMP3(ctx, FFmpegBins{FFmpeg: ff}, video)
	if err == nil || !strings.Contains(err.Error(), "没有音轨") {
		t.Fatalf("err=%v", err)
	}
}
