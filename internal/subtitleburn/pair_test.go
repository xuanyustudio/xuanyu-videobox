package subtitleburn_test

import (
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestPair(t *testing.T) {
	video := filepath.Join("videos", "A.MP4")
	srt := filepath.Join("subs", "a.srt")
	items, unused := subtitleburn.Pair([]string{video}, []string{srt})
	if len(items) != 1 || items[0].Video != video || items[0].Subtitle != srt || len(unused) != 0 {
		t.Fatalf("case pair: items=%v unused=%v", items, unused)
	}

	ass := filepath.Join("subs", "a.ass")
	items, unused = subtitleburn.Pair([]string{video}, []string{ass, srt})
	if items[0].Subtitle != srt || len(unused) != 1 || unused[0] != ass {
		t.Fatalf("srt wins: items=%v unused=%v", items, unused)
	}

	first := filepath.Join("one", "a.srt")
	second := filepath.Join("two", "a.srt")
	items, unused = subtitleburn.Pair([]string{video}, []string{first, second})
	if items[0].Subtitle != first || len(unused) != 1 || unused[0] != second {
		t.Fatalf("earlier wins: items=%v unused=%v", items, unused)
	}

	same := filepath.Join("otherdir", "A.mov")
	items, unused = subtitleburn.Pair([]string{video, same}, []string{srt})
	if items[0].Subtitle != srt || items[1].Subtitle != "" || len(unused) != 0 {
		t.Fatalf("same stem: items=%v unused=%v", items, unused)
	}

	other := filepath.Join("videos", "B.mkv")
	items, unused = subtitleburn.Pair([]string{video, other}, []string{srt})
	if items[0].Subtitle != srt || items[1].Subtitle != "" || len(unused) != 0 {
		t.Fatalf("unmatched video: items=%v unused=%v", items, unused)
	}

	orphan := filepath.Join("subs", "c.vtt")
	items, unused = subtitleburn.Pair([]string{video}, []string{srt, orphan})
	if items[0].Subtitle != srt || len(unused) != 1 || unused[0] != orphan {
		t.Fatalf("orphan: items=%v unused=%v", items, unused)
	}

	items, unused = subtitleburn.Pair([]string{video, other}, nil)
	if items[0].Subtitle != "" || items[1].Subtitle != "" || len(unused) != 0 {
		t.Fatalf("no subs: items=%v unused=%v", items, unused)
	}
}
