package replicate

// Span is one analysis window on the source timeline, in seconds.
type Span struct {
	Start float64
	End   float64
}

// NormalizeSegmentSec accepts only 10 or 15. Anything else is 15.
func NormalizeSegmentSec(n int) int {
	if n == 10 || n == 15 {
		return n
	}
	return 15
}

// PlanSegments splits durationSec into windows of segmentSec.
// A non-positive duration becomes one 0–60s span (the existing frame fallback).
// A remainder shorter than 1 second is absorbed into the previous window.
func PlanSegments(durationSec float64, segmentSec int) []Span {
	s := float64(NormalizeSegmentSec(segmentSec))
	if durationSec <= 0 {
		return []Span{{Start: 0, End: fallbackDurationSec}}
	}
	if durationSec <= s {
		return []Span{{Start: 0, End: durationSec}}
	}
	var out []Span
	start := 0.0
	for start+s < durationSec-1e-9 {
		end := start + s
		if durationSec-end < 1 {
			out = append(out, Span{Start: start, End: durationSec})
			return out
		}
		out = append(out, Span{Start: start, End: end})
		start = end
	}
	if start < durationSec-1e-9 {
		out = append(out, Span{Start: start, End: durationSec})
	}
	return out
}
