package subtitleburn_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestForceStyle(t *testing.T) {
	got := subtitleburn.ForceStyle(subtitleburn.DefaultStyle())
	want := "FontSize=22,FontName=Arial,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=2,Shadow=1,MarginV=36"
	if got != want {
		t.Fatalf("default\n got %s\nwant %s", got, want)
	}

	style := subtitleburn.DefaultStyle()
	style.MarginV = 0
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, "MarginV=0") {
		t.Fatalf("margin 0: %s", got)
	}

	style = subtitleburn.DefaultStyle()
	style.PrimaryColor = "blue"
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, "PrimaryColour=&H00FFFFFF") {
		t.Fatalf("bad color: %s", got)
	}

	style = subtitleburn.DefaultStyle()
	style.PrimaryColor = "#FF0000"
	style.OutlineColor = "#00FF00"
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, "PrimaryColour=&H000000FF") || !strings.Contains(got, "OutlineColour=&H0000FF00") {
		t.Fatalf("bgr: %s", got)
	}

	style = subtitleburn.DefaultStyle()
	style.FontName = "O'Hara"
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, `FontName=O\'Hara`) {
		t.Fatalf("quote: %s", got)
	}
}
