package script

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var srtCueTimeRE = regexp.MustCompile(`(?i)(\d{2}:\d{2}:\d{2}[,.]\d{1,3})\s*-->\s*(\d{2}:\d{2}:\d{2}[,.]\d{1,3})`)

// ShiftSubtitleTimes adds offsetMs to every SRT/VTT cue timestamp in text.
func ShiftSubtitleTimes(text string, offsetMs int) string {
	if offsetMs == 0 || strings.TrimSpace(text) == "" {
		return text
	}
	return srtCueTimeRE.ReplaceAllStringFunc(text, func(cue string) string {
		m := srtCueTimeRE.FindStringSubmatch(cue)
		if len(m) != 3 {
			return cue
		}
		start, err1 := timestampToMs(m[1])
		end, err2 := timestampToMs(m[2])
		if err1 != nil || err2 != nil {
			return cue
		}
		sep := " --> "
		if strings.Contains(cue, "-->") {
			// keep a single space form; SRT commonly uses " --> "
			left := strings.Index(cue, "-->")
			if left >= 0 {
				// preserve original arrow spacing as " --> "
			}
		}
		return FormatTimestampMs(start+offsetMs) + sep + FormatTimestampMs(end+offsetMs)
	})
}

// ComposeSubtitleText joins each video's subtitles onto the concatenated
// global timeline and labels the video breaks so the script LLM can keep
// time and source in sync.
func ComposeSubtitleText(subtitlePaths []string, spans []SourceSegment) (string, error) {
	n := len(spans)
	if len(subtitlePaths) < n {
		n = len(subtitlePaths)
	}
	var parts []string
	for i := 0; i < n; i++ {
		seg := spans[i]
		path := strings.TrimSpace(subtitlePaths[i])
		body := ""
		if path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("读取字幕 %s: %w", path, err)
			}
			body = StripSpeakerLabels(string(data))
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		shifted := ShiftSubtitleTimes(body, seg.StartMs)
		header := fmt.Sprintf(
			"【视频 %d/%d】文件：%s\n本段在全局时间轴上的范围：%s–%s\n（以下字幕时间已换算为全局时间，请不要把每段视频重新从 00:00:00 起算）",
			seg.VideoID, len(spans), seg.Name,
			FormatTimestampMs(seg.StartMs), FormatTimestampMs(seg.EndMs),
		)
		parts = append(parts, header+"\n\n"+shifted)
	}
	return strings.Join(parts, "\n\n"), nil
}
