package script

import "testing"

func testSpans() []SourceSegment {
	// Matches the 二次恋爱 first two sources: 174.720s + 167.744s.
	return BuildSourceTimeline(
		[]string{`E:\v\1.mp4`, `E:\v\2.mp4`, `E:\v\3.mp4`},
		[]int{174720, 167744, 226709},
	)
}

func TestBuildSourceTimelineOffsets(t *testing.T) {
	spans := testSpans()
	if len(spans) != 3 || spans[0].StartMs != 0 || spans[0].EndMs != 174720 {
		t.Fatalf("video 1: %+v", spans[0])
	}
	if spans[1].StartMs != 174720 || spans[1].EndMs != 174720+167744 {
		t.Fatalf("video 2: %+v", spans[1])
	}
	if spans[1].VideoID != 2 || spans[2].VideoID != 3 {
		t.Fatalf("ids %d %d", spans[1].VideoID, spans[2].VideoID)
	}
}

func TestMapGlobalTimeIntoSecondVideo(t *testing.T) {
	spans := testSpans()
	seg, local, ok := MapGlobalTime(spans, 178000) // 00:02:58
	if !ok {
		t.Fatal("expected map")
	}
	if seg.VideoID != 2 || local != 178000-174720 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestResolveItemSourceUnsetContinuesPastFirstVideo(t *testing.T) {
	spans := testSpans()
	seg, local, err := ResolveItemSource(0, 178000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 2 || local != 3280 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestResolveItemSourceHonorsLocalVideoID(t *testing.T) {
	spans := testSpans()
	// 00:01:00 on video 2 must stay local to 2.mp4, not global (which would be 1.mp4).
	seg, local, err := ResolveItemSource(2, 60000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 2 || local != 60000 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestResolveItemSourceOverflowRemapsGlobally(t *testing.T) {
	spans := testSpans()
	// Last 二次恋爱 row: video_id=1, timestamp 00:05:23, which overflows 1.mp4
	// but lands in 2.mp4 on the concat timeline.
	seg, local, err := ResolveItemSource(1, 323000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 2 || local != 323000-174720 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestNormalizeItemSourceWritesLocalTimestamp(t *testing.T) {
	spans := testSpans()
	item := Item{ID: 12, VideoID: 0, Timestamp: "00:02:58,000-00:03:15,000"}
	if err := NormalizeItemSource(&item, spans); err != nil {
		t.Fatal(err)
	}
	if item.VideoID != 2 {
		t.Fatalf("video_id %d", item.VideoID)
	}
	if item.Timestamp != "00:00:03,280-00:00:20,280" {
		t.Fatalf("timestamp %s", item.Timestamp)
	}
	if item.VideoName != spans[1].Name {
		t.Fatalf("name %s", item.VideoName)
	}
}

func TestResolveItemSourcePastAllVideos(t *testing.T) {
	spans := testSpans()
	_, _, err := ResolveItemSource(0, TotalDurationMs(spans)+1000, spans)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExplicitLaterEpisodeNotRemappedToFirst(t *testing.T) {
	// Episode 1 is long enough that a 2-minute clock value sits inside it.
	// The script names episode 2, whose own file is only 90s. Selection must
	// stay on episode 2 instead of playing episode 1 for that range.
	spans := BuildSourceTimeline(
		[]string{`E:\v\1.mp4`, `E:\v\2.mp4`},
		[]int{600000, 90000},
	)
	seg, local, err := resolveItemSource(2, "", 120000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 2 || local != 120000 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestVideoNameUnsticksDefaultEpisodeOne(t *testing.T) {
	spans := testSpans()
	seg, local, err := resolveItemSource(1, spans[1].Name, 60000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 2 || local != 60000 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestVideoNameDoesNotOverrideLaterEpisode(t *testing.T) {
	spans := testSpans()
	seg, local, err := resolveItemSource(3, `1.mp4`, 30000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 3 || local != 30000 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestGlobalTimestampOnMatchingLaterEpisode(t *testing.T) {
	spans := testSpans()
	seg, local, err := resolveItemSource(2, "", 323000, spans)
	if err != nil {
		t.Fatal(err)
	}
	if seg.VideoID != 2 || local != 323000-174720 {
		t.Fatalf("got video %d local %d", seg.VideoID, local)
	}
}

func TestLocalizeItemFollowsScriptEpisodeOrder(t *testing.T) {
	spans := BuildSourceTimeline(
		[]string{"1.mp4", "2.mp4", "3.mp4"},
		[]int{600000, 120000, 120000},
	)
	items := []Item{
		{VideoID: 2, Timestamp: "00:00:05,000-00:00:12,000"},
		{VideoIndex: 3, Timestamp: "00:00:01,000-00:00:04,000"},
		{VideoID: 1, SourceVideo: "2.mp4", Timestamp: "00:00:20,000-00:00:28,000"},
		{VideoID: 1, Timestamp: "00:11:00,000-00:11:08,000"},
	}
	want := []int{2, 3, 2, 2}
	for i, item := range items {
		seg, ts, err := LocalizeItem(item, spans)
		if err != nil {
			t.Fatalf("item %d: %v", i+1, err)
		}
		if seg.VideoID != want[i] {
			t.Fatalf("item %d video %d want %d (%s)", i+1, seg.VideoID, want[i], ts)
		}
		if _, _, err := ParseTimestampRange(ts); err != nil {
			t.Fatalf("item %d local timestamp %s: %v", i+1, ts, err)
		}
	}
}
