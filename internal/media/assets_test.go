package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSidecarSubtitle(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip01.mp4")
	srt := filepath.Join(dir, "clip01.srt")
	if err := os.WriteFile(video, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srt, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := FindSidecarSubtitle(video)
	if got != srt {
		t.Fatalf("got %q want %q", got, srt)
	}
}

func TestListVideosInDirAndMatch(t *testing.T) {
	dir := t.TempDir()
	v1 := filepath.Join(dir, "a.mp4")
	v2 := filepath.Join(dir, "b.mov")
	_ = os.WriteFile(v1, []byte("x"), 0o644)
	_ = os.WriteFile(v2, []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "a.srt"), []byte("s"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("t"), 0o644)

	videos, err := ListVideosInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 2 {
		t.Fatalf("videos=%d", len(videos))
	}
	subs := MatchSidecarSubtitles(videos)
	if len(subs) != 1 {
		t.Fatalf("subs=%v", subs)
	}
}

func TestAppendUniquePaths(t *testing.T) {
	got := AppendUniquePaths([]string{`E:\a.mp4`}, `E:\a.mp4`, `E:\b.mp4`)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
