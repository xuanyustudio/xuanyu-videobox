package media

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAbsCommand(t *testing.T) {
	got := absCommand(`.\ffmpeg.exe`)
	if !filepath.IsAbs(got) || !strings.HasSuffix(strings.ToLower(got), "ffmpeg.exe") {
		t.Fatalf("relative: %s", got)
	}
	abs := filepath.Join(t.TempDir(), "ffmpeg.exe")
	if absCommand(abs) != abs {
		t.Fatalf("absolute: %s", absCommand(abs))
	}
	if absCommand("") != "" {
		t.Fatal("empty")
	}
}
