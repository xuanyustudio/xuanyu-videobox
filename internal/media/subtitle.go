package media

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/narratoai/narrato-go/internal/script"
)

const defaultMaxCharsPerCue = 12

// SubtitleOptions controls programmatic script SRT generation.
type SubtitleOptions struct {
	HideOnOriginal bool // default true in callers
	MaxCharsPerCue int  // default 12
}

var editedClockRangeRE = regexp.MustCompile(
	`^(?P<start>\d{2}:\d{2}:\d{2})-(?P<end>\d{2}:\d{2}:\d{2})$`,
)

// BuildScriptSRT builds an SRT file body from script narration rows (OST 0 and 2).
func BuildScriptSRT(items []script.Item, opts SubtitleOptions) (string, error) {
	maxChars := opts.MaxCharsPerCue
	if maxChars <= 0 {
		maxChars = defaultMaxCharsPerCue
	}

	var blocks []string
	cueIndex := 0
	currentSec := 0.0

	for _, item := range items {
		if opts.HideOnOriginal && item.OST == 1 {
			_, _, next, ok := resolveItemWindow(item, currentSec)
			if ok {
				currentSec = next
			}
			continue
		}
		if item.OST != 0 && item.OST != 2 {
			_, _, next, ok := resolveItemWindow(item, currentSec)
			if ok {
				currentSec = next
			}
			continue
		}

		startSec, endSec, nextSec, ok := resolveItemWindow(item, currentSec)
		if !ok {
			continue
		}
		currentSec = nextSec

		chunks := splitNarration(item.Narration, maxChars)
		if len(chunks) == 0 {
			continue
		}

		segmentDur := endSec - startSec
		if segmentDur <= 0 {
			continue
		}

		chunkDur := segmentDur / float64(len(chunks))
		for i, chunk := range chunks {
			chunkStart := startSec + chunkDur*float64(i)
			chunkEnd := endSec
			if i < len(chunks)-1 {
				chunkEnd = startSec + chunkDur*float64(i+1)
			}
			cueIndex++
			blocks = append(blocks, formatSRTBlock(cueIndex, chunkStart, chunkEnd, chunk))
		}
	}

	if len(blocks) == 0 {
		return "", nil
	}
	return strings.Join(blocks, "\n\n") + "\n", nil
}

func resolveItemWindow(item script.Item, currentSec float64) (startSec, endSec, nextSec float64, ok bool) {
	if startMs, endMs, parsed := parseEditedTimeRange(item.EditedTimeRange); parsed {
		start := float64(startMs) / 1000.0
		end := float64(endMs) / 1000.0
		if end > start {
			return start, end, end, true
		}
	}

	dur := itemDurationSec(item)
	if dur <= 0 {
		return 0, 0, currentSec, false
	}
	start := currentSec
	end := currentSec + dur
	return start, end, end, true
}

func itemDurationSec(item script.Item) float64 {
	if item.Duration != nil && *item.Duration > 0 {
		return *item.Duration
	}
	return script.EstimateItemDurationSec(item)
}

func parseEditedTimeRange(value string) (startMs, endMs int, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	if startMs, endMs, err := script.ParseTimestampRange(value); err == nil && endMs > startMs {
		return startMs, endMs, true
	}
	match := editedClockRangeRE.FindStringSubmatch(value)
	if match == nil {
		return 0, 0, false
	}
	startMs, err := clockToMs(match[1])
	if err != nil {
		return 0, 0, false
	}
	endMs, err = clockToMs(match[2])
	if err != nil || endMs <= startMs {
		return 0, 0, false
	}
	return startMs, endMs, true
}

func clockToMs(clock string) (int, error) {
	parts := strings.Split(clock, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid clock: %s", clock)
	}
	hh, err := atoi(parts[0])
	if err != nil {
		return 0, err
	}
	mm, err := atoi(parts[1])
	if err != nil {
		return 0, err
	}
	ss, err := atoi(parts[2])
	if err != nil {
		return 0, err
	}
	return ((hh*60+mm)*60+ss)*1000, nil
}

func atoi(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid number: %s", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func splitNarration(text string, maxChars int) []string {
	text = cleanSubtitleText(text)
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	if maxChars <= 0 {
		maxChars = defaultMaxCharsPerCue
	}

	var chunks []string
	for i := 0; i < len(runes); i += maxChars {
		end := i + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[i:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func cleanSubtitleText(text string) string {
	text = script.StripSpeakerLabels(text)
	text = strings.Join(strings.Fields(text), " ")
	return stripPunctuation(text)
}

func stripPunctuation(text string) string {
	var b strings.Builder
	for _, r := range text {
		if unicode.IsPunct(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func formatSRTBlock(index int, startSec, endSec float64, text string) string {
	return fmt.Sprintf(
		"%d\n%s --> %s\n%s",
		index,
		formatSRTTime(startSec),
		formatSRTTime(endSec),
		text,
	)
}

func formatSRTTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	msTotal := int64(seconds*1000 + 0.5)
	ms := msTotal % 1000
	totalSec := msTotal / 1000
	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	secs := totalSec % 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, ms)
}
