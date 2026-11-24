package media_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
)

func TestTargetSize(t *testing.T) {
	w, h := media.TargetSize(1080, 1920, "vertical", "1080p")
	if w != 1080 || h != 1920 {
		t.Fatalf("vertical 1080p: %dx%d", w, h)
	}
	w, h = media.TargetSize(1920, 1080, "horizontal", "720p")
	if w != 1280 || h != 720 {
		t.Fatalf("horizontal 720p: %dx%d", w, h)
	}
	w, h = media.TargetSize(1080, 1920, "source", "1080p")
	if w != 1080 || h != 1920 {
		t.Fatalf("source portrait 1080p: %dx%d", w, h)
	}
	w, h = media.TargetSize(1920, 1080, "source", "1080p")
	if w != 1920 || h != 1080 {
		t.Fatalf("source landscape 1080p: %dx%d", w, h)
	}
	w, h = media.TargetSize(100, 100, "vertical", "480p")
	if w%2 != 0 || h%2 != 0 {
		t.Fatalf("odd size: %dx%d", w, h)
	}
}

func TestScaleCropFilter(t *testing.T) {
	f := media.ScaleCropFilter(1080, 1920)
	if !strings.Contains(f, "scale=1080:1920") || !strings.Contains(f, "crop=1080:1920") {
		t.Fatalf("filter: %s", f)
	}
}

func TestParseResolutionHeight(t *testing.T) {
	if media.ParseResolutionHeight("4K") != 2160 {
		t.Fatal("4K")
	}
	if media.ParseResolutionHeight("") != 1080 {
		t.Fatal("default")
	}
}
