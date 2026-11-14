package subtitleburn_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestSubtitlesFilter(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	srt := filepath.Join(dir, "clip.srt")
	got := subtitleburn.SubtitlesFilter(video, srt, subtitleburn.DefaultStyle())
	want := "subtitles='clip.srt':force_style='FontSize=15,FontName=Arial,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=2,Shadow=1,MarginV=36'"
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestBurnRequiresFFmpeg(t *testing.T) {
	err := subtitleburn.Burn(context.Background(), media.FFmpegBins{}, "a.mp4", "a.srt", "out.mp4", subtitleburn.DefaultStyle())
	if err == nil || !strings.Contains(err.Error(), "ffmpeg binary required") {
		t.Fatal(err)
	}
}
