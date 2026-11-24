package media_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
)

func TestResolveSubtitleMaskDefaultsPortrait(t *testing.T) {
	r := media.ResolveSubtitleMask(1080, 1920, media.SubtitleMask{Enabled: true})
	if !r.Portrait {
		t.Fatal("expected portrait")
	}
	if r.Y < 1400 || r.H < 200 {
		t.Fatalf("region should sit near the bottom: %+v", r)
	}
	if r.X+r.W > 1080 || r.Y+r.H > 1920 {
		t.Fatalf("region exceeds frame: %+v", r)
	}
}

func TestSubtitleMaskFilterContainsBlurAndBox(t *testing.T) {
	f := media.SubtitleMaskFilter(1920, 1080, media.SubtitleMask{Enabled: true})
	if f == "" {
		t.Fatal("empty filter")
	}
	for _, needle := range []string{"crop=", "boxblur=", "overlay=", "drawbox="} {
		if !strings.Contains(f, needle) {
			t.Fatalf("missing %s in %s", needle, f)
		}
	}
}

func TestSubtitleMaskDisabled(t *testing.T) {
	if media.SubtitleMaskFilter(1920, 1080, media.SubtitleMask{}) != "" {
		t.Fatal("disabled mask should yield empty filter")
	}
}

func TestSubtitleMaskCustomPercents(t *testing.T) {
	r := media.ResolveSubtitleMask(1000, 1000, media.SubtitleMask{
		Enabled: true, X: 10, Y: 80, Width: 80, Height: 15, Blur: 10, Opacity: 50,
	})
	if r.Y < 700 || r.H < 100 {
		t.Fatalf("unexpected region: %+v", r)
	}
}
