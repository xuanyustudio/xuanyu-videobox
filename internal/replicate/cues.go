package replicate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/script"
)

// Cue is one subtitle line on the source timeline.
type Cue struct {
	Start float64
	End   float64
	Text  string
}

// ParseSRT reads a standard SRT body. Malformed blocks are skipped.
// Cue text is kept as written; speaker labels are stripped later for prompts.
func ParseSRT(raw string) []Cue {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []Cue
	for _, block := range strings.Split(raw, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 {
			continue
		}
		timeLine := lines[0]
		textLines := lines[1:]
		if !strings.Contains(timeLine, "-->") && len(lines) >= 3 {
			timeLine = lines[1]
			textLines = lines[2:]
		}
		start, end, ok := parseSRTRange(timeLine)
		if !ok {
			continue
		}
		text := strings.TrimSpace(strings.Join(textLines, "\n"))
		if text == "" {
			continue
		}
		out = append(out, Cue{Start: start, End: end, Text: text})
	}
	return out
}

func parseSRTRange(line string) (float64, float64, bool) {
	parts := strings.Split(line, "-->")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, ok1 := parseSRTClock(parts[0])
	end, ok2 := parseSRTClock(parts[1])
	if !ok1 || !ok2 || end < start {
		return 0, 0, false
	}
	return start, end, true
}

func parseSRTClock(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	chunk := strings.Split(s, ":")
	if len(chunk) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(chunk[0])
	m, err2 := strconv.Atoi(chunk[1])
	sec, err3 := strconv.ParseFloat(chunk[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return float64(h*3600+m*60) + sec, true
}

// CuesForSpan returns cues that overlap span, in start-time order, text uncut.
func CuesForSpan(cues []Cue, span Span) []Cue {
	var out []Cue
	for _, c := range cues {
		if c.Start < span.End && c.End > span.Start {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Start < out[j].Start
	})
	return out
}

// FormatCueLines renders prompt lines. Speaker prefixes are removed here only.
func FormatCueLines(cues []Cue) string {
	var lines []string
	for _, c := range cues {
		text := script.StripSpeakerLabels(c.Text)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%.1f–%.1f秒：%s", c.Start, c.End, text))
	}
	return strings.Join(lines, "\n")
}
