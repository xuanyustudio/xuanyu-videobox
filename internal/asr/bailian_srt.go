package asr

import (
	"fmt"
	"strings"
)

// bailianResultToSRT converts DashScope Fun-ASR JSON to SRT (sentence-level).
func bailianResultToSRT(result map[string]any) (string, error) {
	type cue struct {
		start, end float64 // milliseconds
		text       string
	}
	var cues []cue

	for _, sentence := range iterBailianSentences(result) {
		text := strings.TrimSpace(fmt.Sprint(sentence["text"]))
		if text == "" || text == "<nil>" {
			continue
		}
		start := asFloat(sentence["begin_time"])
		end := asFloat(sentence["end_time"])
		if end <= start {
			end = start + 500
		}
		prefix := speakerPrefix(sentence["speaker_id"])
		cues = append(cues, cue{start, end, prefix + text})
	}
	if len(cues) == 0 {
		return "", fmt.Errorf("百炼转写结果为空")
	}
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, formatSRTTimeMS(c.start), formatSRTTimeMS(c.end), c.text)
	}
	return b.String(), nil
}

func iterBailianSentences(result map[string]any) []map[string]any {
	var out []map[string]any
	if transcripts, ok := result["transcripts"].([]any); ok {
		for _, t := range transcripts {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if sents, ok := tm["sentences"].([]any); ok {
				for _, s := range sents {
					if sm, ok := s.(map[string]any); ok {
						out = append(out, sm)
					}
				}
			}
		}
	}
	if len(out) == 0 {
		if sents, ok := result["sentences"].([]any); ok {
			for _, s := range sents {
				if sm, ok := s.(map[string]any); ok {
					out = append(out, sm)
				}
			}
		}
	}
	return out
}

func speakerPrefix(id any) string {
	if id == nil {
		return ""
	}
	switch v := id.(type) {
	case float64:
		return fmt.Sprintf("说话人%d: ", int(v)+1)
	case string:
		if v == "" {
			return ""
		}
		return "说话人" + v + ": "
	default:
		s := strings.TrimSpace(fmt.Sprint(id))
		if s == "" || s == "<nil>" {
			return ""
		}
		return "说话人" + s + ": "
	}
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func formatSRTTimeMS(ms float64) string {
	if ms < 0 {
		ms = 0
	}
	total := int(ms + 0.5)
	h := total / 3600000
	total %= 3600000
	m := total / 60000
	total %= 60000
	s := total / 1000
	milli := total % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, milli)
}
