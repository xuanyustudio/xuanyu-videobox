package script

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SourceSegment is one source video on the project's concatenated timeline.
// Videos are laid end-to-end in list order: item timestamps that continue
// past the first file belong to later files at a local offset.
type SourceSegment struct {
	Index      int    // 0-based index in VideoPaths
	VideoID    int    // 1-based id stored on script items
	Path       string
	Name       string
	DurationMs int
	StartMs    int // inclusive global start
	EndMs      int // exclusive global end
}

// BuildSourceTimeline lays paths end-to-end using durationsMs (same length and order).
func BuildSourceTimeline(paths []string, durationsMs []int) []SourceSegment {
	n := len(paths)
	if len(durationsMs) < n {
		n = len(durationsMs)
	}
	out := make([]SourceSegment, 0, n)
	cursor := 0
	for i := 0; i < n; i++ {
		d := durationsMs[i]
		if d < 0 {
			d = 0
		}
		p := strings.TrimSpace(paths[i])
		out = append(out, SourceSegment{
			Index:      i,
			VideoID:    i + 1,
			Path:       p,
			Name:       filepath.Base(p),
			DurationMs: d,
			StartMs:    cursor,
			EndMs:      cursor + d,
		})
		cursor += d
	}
	return out
}

// TotalDurationMs is the concatenated length of all source videos.
func TotalDurationMs(spans []SourceSegment) int {
	if len(spans) == 0 {
		return 0
	}
	return spans[len(spans)-1].EndMs
}

// MapGlobalTime finds the source video containing a global timeline position.
func MapGlobalTime(spans []SourceSegment, globalMs int) (SourceSegment, int, bool) {
	if len(spans) == 0 || globalMs < 0 {
		return SourceSegment{}, 0, false
	}
	for _, seg := range spans {
		if globalMs >= seg.StartMs && globalMs < seg.EndMs {
			return seg, globalMs - seg.StartMs, true
		}
	}
	return SourceSegment{}, 0, false
}

// ResolveItemSource decides which file a script row should be cut from.
//
// video_id is 1-based. If it is set and startMs fits inside that file, the
// timestamp is treated as local to that file (how the editor saves a row).
// Otherwise startMs is treated as a time on the concatenated global timeline,
// so rows that the LLM left on video_id=0 but continued past the first file
// land on the correct later video.
func ResolveItemSource(videoID, startMs int, spans []SourceSegment) (SourceSegment, int, error) {
	if len(spans) == 0 {
		return SourceSegment{}, 0, fmt.Errorf("没有可用的片源")
	}
	if videoID >= 1 && videoID <= len(spans) {
		seg := spans[videoID-1]
		if startMs >= 0 && startMs < seg.DurationMs {
			return seg, startMs, nil
		}
	}
	if seg, local, ok := MapGlobalTime(spans, startMs); ok {
		return seg, local, nil
	}
	total := TotalDurationMs(spans)
	return SourceSegment{}, 0, fmt.Errorf("时间 %s 已超出全部素材合计时长 %s", FormatTimestampMs(startMs), FormatTimestampMs(total))
}

// NormalizeItemSource writes 1-based video_id and a local timestamp onto item
// when the stored time is on the concatenated global timeline.
func NormalizeItemSource(item *Item, spans []SourceSegment) error {
	if item == nil {
		return fmt.Errorf("nil item")
	}
	startMs, endMs, err := ParseTimestampRange(item.Timestamp)
	if err != nil {
		return err
	}
	seg, localStart, err := ResolveItemSource(item.VideoID, startMs, spans)
	if err != nil {
		return err
	}
	dur := endMs - startMs
	if dur < 0 {
		dur = 0
	}
	localEnd := localStart + dur
	if localEnd > seg.DurationMs {
		localEnd = seg.DurationMs
	}
	if localEnd <= localStart {
		// Keep a short window if the start is valid but the end clamped away.
		localEnd = localStart + 1
		if localEnd > seg.DurationMs {
			return fmt.Errorf("时间 %s 已超出片源「%s」时长 %s", FormatTimestampMs(startMs), seg.Name, FormatTimestampMs(seg.DurationMs))
		}
	}
	item.VideoID = seg.VideoID
	item.VideoName = seg.Name
	item.Timestamp = FormatTimestampRange(localStart, localEnd)
	return nil
}

// NormalizeItemsSource remaps every item in place. It returns a combined error
// if any row could not be placed on a source video.
func NormalizeItemsSource(items []Item, spans []SourceSegment) error {
	var problems []string
	for i := range items {
		if err := NormalizeItemSource(&items[i], spans); err != nil {
			problems = append(problems, fmt.Sprintf("第 %d 条: %s", i+1, err.Error()))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}

// FormatTimestampMs formats milliseconds as 00:02:54,720.
func FormatTimestampMs(ms int) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	rem := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, rem)
}

// FormatTimestampRange formats a start-end window in script timestamp style.
func FormatTimestampRange(startMs, endMs int) string {
	return FormatTimestampMs(startMs) + "-" + FormatTimestampMs(endMs)
}

// LocalizeItemTimes maps a stored timestamp onto the correct source video
// and returns a timestamp that is local to that file.
func LocalizeItemTimes(videoID int, timestamp string, spans []SourceSegment) (SourceSegment, string, error) {
	startMs, endMs, err := ParseTimestampRange(timestamp)
	if err != nil {
		return SourceSegment{}, "", err
	}
	seg, localStart, err := ResolveItemSource(videoID, startMs, spans)
	if err != nil {
		return SourceSegment{}, "", err
	}
	dur := endMs - startMs
	if dur < 0 {
		dur = 0
	}
	return seg, FormatTimestampRange(localStart, localStart+dur), nil
}
