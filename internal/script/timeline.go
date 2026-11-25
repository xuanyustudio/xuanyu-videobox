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
	Index      int // 0-based index in VideoPaths
	VideoID    int // 1-based id stored on script items
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
// video_id is 1-based. When it names a file and startMs fits inside that
// file, the timestamp is local to that file (how the editor saves a row).
// A timestamp past episode 1, with video_id unset or left at the default 1,
// is a time on the concatenated global timeline. An explicit later episode
// is never replaced by episode 1 just because the clock value also falls
// inside the first file.
func ResolveItemSource(videoID, startMs int, spans []SourceSegment) (SourceSegment, int, error) {
	return resolveItemSource(videoID, "", startMs, spans)
}

func resolveItemSource(videoID int, videoName string, startMs int, spans []SourceSegment) (SourceSegment, int, error) {
	if len(spans) == 0 {
		return SourceSegment{}, 0, fmt.Errorf("没有可用的片源")
	}
	if named := matchVideoName(videoName, spans); named > 0 && (videoID <= 1 || named == videoID) {
		// A file name can correct the default video_id=1. It must not
		// pull an explicit later episode back to the first file.
		videoID = named
	}
	if videoID >= 1 && videoID <= len(spans) {
		seg := spans[videoID-1]
		if startMs >= 0 && startMs < seg.DurationMs {
			return seg, startMs, nil
		}
		if mapped, local, ok := MapGlobalTime(spans, startMs); ok && mapped.VideoID == seg.VideoID {
			return mapped, local, nil
		}
		if videoID == 1 {
			if mapped, local, ok := MapGlobalTime(spans, startMs); ok {
				return mapped, local, nil
			}
		}
		// Keep the script's episode. The clip step reports a range past
		// that file instead of silently playing episode 1.
		return seg, startMs, nil
	}
	if seg, local, ok := MapGlobalTime(spans, startMs); ok {
		return seg, local, nil
	}
	total := TotalDurationMs(spans)
	return SourceSegment{}, 0, fmt.Errorf("时间 %s 已超出全部素材合计时长 %s", FormatTimestampMs(startMs), FormatTimestampMs(total))
}

// matchVideoName returns a 1-based video id when name is a source file name.
func matchVideoName(name string, spans []SourceSegment) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0
	}
	base := strings.ToLower(filepath.Base(name))
	for _, seg := range spans {
		if strings.ToLower(seg.Name) == base || strings.ToLower(filepath.Base(seg.Path)) == base {
			return seg.VideoID
		}
	}
	return 0
}

func itemSourceHint(item *Item) (int, string) {
	if item == nil {
		return 0, ""
	}
	id := item.VideoID
	if id <= 0 {
		id = item.VideoIndex
	}
	name := strings.TrimSpace(item.VideoName)
	if name == "" {
		name = strings.TrimSpace(item.SourceVideo)
	}
	return id, name
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
	id, name := itemSourceHint(item)
	seg, localStart, err := resolveItemSource(id, name, startMs, spans)
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
	return localizeTimes(videoID, "", timestamp, spans)
}

// LocalizeItem maps one script row onto its source file. video_id,
// video_index, video_name and source_video are all consulted, in that order,
// so a commentary row is not stuck on episode 1 when the script names
// another episode.
func LocalizeItem(item Item, spans []SourceSegment) (SourceSegment, string, error) {
	id, name := itemSourceHint(&item)
	return localizeTimes(id, name, item.Timestamp, spans)
}

func localizeTimes(videoID int, videoName, timestamp string, spans []SourceSegment) (SourceSegment, string, error) {
	startMs, endMs, err := ParseTimestampRange(timestamp)
	if err != nil {
		return SourceSegment{}, "", err
	}
	seg, localStart, err := resolveItemSource(videoID, videoName, startMs, spans)
	if err != nil {
		return SourceSegment{}, "", err
	}
	dur := endMs - startMs
	if dur < 0 {
		dur = 0
	}
	return seg, FormatTimestampRange(localStart, localStart+dur), nil
}
