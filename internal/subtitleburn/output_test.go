package subtitleburn_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

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
