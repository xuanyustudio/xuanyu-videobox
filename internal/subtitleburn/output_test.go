package subtitleburn_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestPrepareOutput(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	got, err := subtitleburn.PrepareOutput(video, "_sub.mp4")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "srt", "clip_sub.mp4")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "srt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(got, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = subtitleburn.PrepareOutput(video, "_sub.mp4")
	if err != nil || got != filepath.Join(dir, "srt", "clip_sub_2.mp4") {
		t.Fatalf("second: got %s err %v", got, err)
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "clip_sub.mp4")
	got, err := subtitleburn.UniquePath(base)
	if err != nil || got != base {
		t.Fatalf("missing: got %q err %v", got, err)
	}
	if err := os.WriteFile(base, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = subtitleburn.UniquePath(base)
	if err != nil || got != filepath.Join(dir, "clip_sub_2.mp4") {
		t.Fatalf("second: got %q err %v", got, err)
	}
	if err := os.WriteFile(got, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = subtitleburn.UniquePath(base)
	if err != nil || got != filepath.Join(dir, "clip_sub_3.mp4") {
		t.Fatalf("third: got %q err %v", got, err)
	}
}
