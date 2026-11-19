package hookopen

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	srtIndexRe = regexp.MustCompile(`^\d+$`)
	srtTimeRe  = regexp.MustCompile(`(?i)^\s*(\d{1,2}:\d{2}:\d{2}[,.]\d{1,3})\s*-->\s*(\d{1,2}:\d{2}:\d{2}[,.]\d{1,3})`)
)

func parseSRT(s string) []Cue {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	blocks := strings.Split(s, "\n\n")
	var out []Cue
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		i := 0
		if srtIndexRe.MatchString(strings.TrimSpace(lines[0])) {
			i = 1
		}
		if i >= len(lines) {
			continue
		}
		m := srtTimeRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		start, err1 := parseStamp(m[1])
		end, err2 := parseStamp(m[2])
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		text := strings.TrimSpace(strings.Join(lines[i+1:], " "))
		text = strings.ReplaceAll(text, "\n", " ")
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}
		out = append(out, Cue{Start: start, End: end, Text: text})
	}
	return out
}

func parseStamp(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, errBadStamp
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, errBadStamp
	}
	return float64(h)*3600 + float64(m)*60 + sec, nil
}

type stampError string

func (e stampError) Error() string { return string(e) }

const errBadStamp stampError = "bad timestamp"

func formatClock(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := sec - float64(h*3600+m*60)
	return fmt.Sprintf("%02d:%02d:%06.3f", h, m, s)
}

func formatRange(start, end float64) string {
	return FormatRange(start, end)
}

// FormatRange formats a time window for the UI.
func FormatRange(start, end float64) string {
	return fmt.Sprintf("%.2f–%.2f 秒", start, end)
}

func formatTranscript(cues []Cue) string {
	if len(cues) == 0 {
		return "（没有识别到台词）"
	}
	var b strings.Builder
	for _, c := range cues {
		fmt.Fprintf(&b, "- %.2f–%.2f  %s\n", c.Start, c.End, c.Text)
	}
	return b.String()
}

func quoteInRange(cues []Cue, start, end float64) string {
	var parts []string
	for _, c := range cues {
		if c.End < start-0.05 || c.Start > end+0.05 {
			continue
		}
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, " ")
}
