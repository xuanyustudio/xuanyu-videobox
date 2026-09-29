package script

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	GeneralClipMin       = 2.0
	TailPad              = 0.35
	OpeningHookMin       = 5.001
	NarrationSecPerChar  = 0.28
)

var (
	scriptRangeRE = regexp.MustCompile(
		`^(?P<start>\d{2}:\d{2}:\d{2}[,.]\d{3})-(?P<end>\d{2}:\d{2}:\d{2}[,.]\d{3})$`,
	)
	timestampRE = regexp.MustCompile(`^\d{2}:\d{2}:\d{2},\d{3}$`)
	speakerLabelRE = regexp.MustCompile(
		`(?:说话人\s*\d+|(?i)Speaker\s*\d+)\s*[:：]?\s*`,
	)
)

// ParseTimestampRange parses HH:MM:SS,mmm-HH:MM:SS,mmm into start/end milliseconds.
func ParseTimestampRange(timestampRange string) (startMs, endMs int, err error) {
	value := strings.TrimSpace(strings.ReplaceAll(timestampRange, ".", ","))
	match := scriptRangeRE.FindStringSubmatch(value)
	if match == nil {
		return 0, 0, errInvalidTimestampRange
	}
	startMs, err = timestampToMs(match[1])
	if err != nil {
		return 0, 0, err
	}
	endMs, err = timestampToMs(match[2])
	if err != nil {
		return 0, 0, err
	}
	return startMs, endMs, nil
}

var errInvalidTimestampRange = strconv.ErrSyntax

func timestampToMs(timestamp string) (int, error) {
	value := strings.TrimSpace(strings.ReplaceAll(timestamp, ".", ","))
	if !timestampRE.MatchString(value) {
		return 0, errInvalidTimestampRange
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, errInvalidTimestampRange
	}
	hh, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	ssParts := strings.Split(parts[2], ",")
	if len(ssParts) != 2 {
		return 0, errInvalidTimestampRange
	}
	ss, err := strconv.Atoi(ssParts[0])
	if err != nil {
		return 0, err
	}
	ms, err := strconv.Atoi(ssParts[1])
	if err != nil {
		return 0, err
	}
	return ((hh*60+mm)*60+ss)*1000 + ms, nil
}

// TimestampDurationSec returns the span of a script timestamp range in seconds.
func TimestampDurationSec(timestampRange string) (float64, error) {
	startMs, endMs, err := ParseTimestampRange(timestampRange)
	if err != nil {
		return 0, err
	}
	return math.Max(0, float64(endMs-startMs)/1000.0), nil
}

// FormatDurationClock formats seconds as MM:SS or HH:MM:SS when hours are non-zero.
func FormatDurationClock(seconds float64) string {
	sec := int(math.Round(seconds))
	if sec < 0 {
		sec = 0
	}
	hours := sec / 3600
	rem := sec % 3600
	minutes := rem / 60
	secs := rem % 60
	if hours > 0 {
		return sprintfClock3(hours, minutes, secs)
	}
	return sprintfClock2(minutes, secs)
}

func sprintfClock2(minutes, secs int) string {
	return pad2(minutes) + ":" + pad2(secs)
}

func sprintfClock3(hours, minutes, secs int) string {
	return pad2(hours) + ":" + pad2(minutes) + ":" + pad2(secs)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// StripSpeakerLabels removes FunASR speaker prefixes from subtitle text.
func StripSpeakerLabels(text string) string {
	return strings.TrimSpace(speakerLabelRE.ReplaceAllString(text, ""))
}

func isMarkedOpeningHook(item Item) bool {
	return item.HookSourceID != nil
}

func positiveDuration(value *float64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	if *value > 0 {
		return *value, true
	}
	return 0, false
}

func estimateNarrationSeconds(item Item) float64 {
	text := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, item.Narration)
	ttsEst := math.Max(0, float64(len([]rune(text)))*NarrationSecPerChar)
	if isMarkedOpeningHook(item) {
		return math.Max(ttsEst, OpeningHookMin)
	}
	return math.Max(ttsEst+TailPad, GeneralClipMin)
}

// EstimateItemDurationSec estimates how long one script row lasts in the output.
func EstimateItemDurationSec(item Item) float64 {
	if d, ok := positiveDuration(item.Duration); ok {
		return d
	}

	ost := item.OST
	timestamp := strings.TrimSpace(item.Timestamp)
	if ost == 1 {
		if timestamp != "" {
			if dur, err := TimestampDurationSec(timestamp); err == nil {
				return math.Max(dur, GeneralClipMin)
			}
			return GeneralClipMin
		}
		return GeneralClipMin
	}

	narration := strings.TrimSpace(item.Narration)
	if narration != "" {
		return estimateNarrationSeconds(item)
	}
	if timestamp != "" {
		if dur, err := TimestampDurationSec(timestamp); err == nil {
			return math.Max(dur, GeneralClipMin)
		}
	}
	return GeneralClipMin
}

// EstimateOutputDurationSec sums per-item estimates for the whole script.
func EstimateOutputDurationSec(items []Item) float64 {
	if len(items) == 0 {
		return 0
	}
	var total float64
	for _, item := range items {
		total += EstimateItemDurationSec(item)
	}
	return total
}
