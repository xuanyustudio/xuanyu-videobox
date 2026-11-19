package hookopen

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

type scoredWindow struct {
	Start float64
	End   float64
	Quote string
	Score float64
	Why   string
}

var hookKeywords = []string{
	"竟然", "没想到", "原来", "背叛", "离婚", "分手", "跪下", "耳光", "渣男", "绿茶",
	"怀孕", "出轨", "真相", "秘密", "凭什么", "滚出去", "不可能", "对不起", "为什么",
	"反转", "揭穿", "撕毁", "协议", "继承", "不是你的", "白月光", "小三", "逼宫",
	"你是谁", "救我", "死", "杀", "恨你", "爱你", "股份", "公司", "家产", "孩子",
}

func buildWindows(cues []Cue, peaks []float64, scenes []float64, duration float64) []scoredWindow {
	if duration <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []scoredWindow
	add := func(w scoredWindow) {
		w.Start, w.End = clampWindow(w.Start, w.End, duration)
		if w.End-w.Start < MinHookSec-0.05 {
			return
		}
		key := formatRange(round1(w.Start), round1(w.End))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if w.Quote == "" {
			w.Quote = quoteInRange(cues, w.Start, w.End)
		}
		w.Score += scoreWindow(w, cues, duration)
		out = append(out, w)
	}

	for i := 0; i < len(cues); i++ {
		end := cues[i].End
		j := i
		for j+1 < len(cues) && cues[j+1].End-cues[i].Start <= MaxHookSec {
			j++
			end = cues[j].End
		}
		// shrink from the right to stay within max, keep at least min
		for j > i && end-cues[i].Start > MaxHookSec {
			j--
			end = cues[j].End
		}
		start := cues[i].Start
		if end-start < MinHookSec {
			end = math.Min(duration, start+PrefHookSec)
		}
		if end-start > MaxHookSec {
			end = start + MaxHookSec
		}
		add(scoredWindow{Start: start, End: end, Why: "台词段落"})
	}

	centers := append([]float64{}, peaks...)
	centers = append(centers, scenes...)
	for _, mid := range centers {
		start := mid - 1.5
		end := start + PrefHookSec
		start, end = snapToCues(start, end, cues, duration)
		add(scoredWindow{Start: start, End: end, Why: "高能点"})
	}

	// drop heavy overlaps, keep higher score
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	var kept []scoredWindow
	for _, w := range out {
		overlap := false
		for _, k := range kept {
			if overlapRatio(w.Start, w.End, k.Start, k.End) >= 0.55 {
				overlap = true
				break
			}
		}
		if !overlap {
			kept = append(kept, w)
		}
		if len(kept) >= 12 {
			break
		}
	}
	return kept
}

func scoreWindow(w scoredWindow, cues []Cue, duration float64) float64 {
	s := 0.0
	dur := w.End - w.Start
	// prefer 4-7s
	if dur >= 4 && dur <= 7 {
		s += 1.2
	} else if dur >= MinHookSec && dur <= MaxHookSec {
		s += 0.4
	}
	text := w.Quote
	if text == "" {
		text = quoteInRange(cues, w.Start, w.End)
	}
	hits := 0
	for _, kw := range hookKeywords {
		if strings.Contains(text, kw) {
			hits++
		}
	}
	s += math.Min(2.5, float64(hits)*0.7)
	// mid-late body often has the twist; avoid cold open and ending credits
	if duration > 0 {
		mid := (w.Start + w.End) / 2 / duration
		if mid < 0.06 || mid > 0.92 {
			s -= 1.5
		} else if mid >= 0.2 && mid <= 0.85 {
			s += 0.6
		}
	}
	if utf8.RuneCountInString(text) >= 8 {
		s += 0.3
	}
	return s
}

func snapToCues(start, end float64, cues []Cue, duration float64) (float64, float64) {
	start, end = clampWindow(start, end, duration)
	if len(cues) == 0 {
		return start, end
	}
	// snap start to nearest cue start within 0.6s
	bestS, bestSD := start, 0.6
	bestE, bestED := end, 0.6
	for _, c := range cues {
		if d := abs(c.Start - start); d < bestSD {
			bestS, bestSD = c.Start, d
		}
		if d := abs(c.End - end); d < bestED {
			bestE, bestED = c.End, d
		}
	}
	if bestE-bestS < MinHookSec {
		return clampWindow(start, end, duration)
	}
	if bestE-bestS > MaxHookSec {
		bestE = bestS + MaxHookSec
	}
	return clampWindow(bestS, bestE, duration)
}

func clampWindow(start, end, duration float64) (float64, float64) {
	if start < 0 {
		start = 0
	}
	if duration > 0 && end > duration {
		end = duration
	}
	if end-start < MinHookSec && duration >= MinHookSec {
		end = start + MinHookSec
		if duration > 0 && end > duration {
			end = duration
			start = math.Max(0, end-MinHookSec)
		}
	}
	if end-start > MaxHookSec {
		end = start + MaxHookSec
	}
	if end < start {
		end = start
	}
	return start, end
}

func overlapRatio(a0, a1, b0, b1 float64) float64 {
	inter := math.Min(a1, b1) - math.Max(a0, b0)
	if inter <= 0 {
		return 0
	}
	union := math.Max(a1, b1) - math.Min(a0, b0)
	if union <= 0 {
		return 0
	}
	return inter / union
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
