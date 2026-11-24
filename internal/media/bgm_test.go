package media_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
)

func TestListBGMLibrary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "theme.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := media.ListBGMLibrary(dir)
	if len(got) != 1 || got[0].Title != "theme" {
		t.Fatalf("got %#v", got)
	}
}

func TestIsAudioPath(t *testing.T) {
	if !media.IsAudioPath("a.WAV") || media.IsAudioPath("a.mp4") {
		t.Fatal("ext check")
	}
}
