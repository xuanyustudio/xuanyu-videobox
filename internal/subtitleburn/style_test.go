package subtitleburn_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestForceStyle(t *testing.T) {
	got := subtitleburn.ForceStyle(subtitleburn.DefaultStyle())
	want := "FontSize=15,FontName=Arial,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=2,Shadow=1,MarginV=36,Alignment=2"
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

func TestBurnStyleDefaultKeepsPosition(t *testing.T) {
	got := subtitleburn.BurnStyle(subtitleburn.DefaultStyle(), "bottom", false, 1080, 1920, 1080, 1920, media.SubtitleMask{Y: 79, Height: 16})
	if got.Alignment != 2 || got.MarginV != 36 {
		t.Fatalf("default position changed: %+v", got)
	}
	if !strings.Contains(subtitleburn.ForceStyle(got), "MarginV=36") {
		t.Fatal("force style lost the default margin")
	}
}

func TestBurnStyleOnMaskSitsInBand(t *testing.T) {
	mask := media.SubtitleMask{Y: 79, Height: 16}
	got := subtitleburn.BurnStyle(subtitleburn.DefaultStyle(), "top", true, 1080, 1920, 1080, 1920, mask)
	region := media.OutputMaskRect(1080, 1920, 1080, 1920, mask)
	textCenter := 1920 - got.MarginV - got.FontSize/2
	if got.Alignment == 8 {
		textCenter = got.MarginV + got.FontSize/2
	}
	if textCenter < region.Y || textCenter > region.Y+region.H {
		t.Fatalf("text center %d outside mask %d-%d style=%+v", textCenter, region.Y, region.Y+region.H, got)
	}
	if got.MarginV == 36 && got.Alignment == 8 {
		t.Fatal("on-mask should leave the plain top/bottom placement")
	}
	forced := subtitleburn.ForceStyle(got)
	if !strings.Contains(forced, "MarginV=") {
		t.Fatalf("force style: %s", forced)
	}
}
