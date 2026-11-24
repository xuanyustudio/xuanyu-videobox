package tts

import "strings"

const (
	// KokoroMaxRunes is just under the Kokoro server's 500-character limit.
	KokoroMaxRunes = 480
	// OpenVoiceMaxRunes keeps MeloTTS requests to a comfortable length.
	OpenVoiceMaxRunes = 400
)

var narrationSeps = map[rune]bool{
	'。': true, '！': true, '？': true, '；': true, '!': true, '?': true, ';': true,
	'\n': true, '，': true, ',': true, '、': true, ' ': true, '\t': true, '：': true, ':': true,
}

// SplitNarration splits text into chunks of at most maxRunes, preferring
// punctuation boundaries so a long script item can be synthesized in pieces.
func SplitNarration(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = KokoroMaxRunes
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	var out []string
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			part := strings.TrimSpace(string(runes[start:]))
			if part != "" {
				out = append(out, part)
			}
			break
		}
		cut := -1
		for i := end - 1; i > start; i-- {
			if narrationSeps[runes[i]] {
				cut = i + 1
				break
			}
		}
		if cut <= start {
			cut = end
		}
		part := strings.TrimSpace(string(runes[start:cut]))
		if part != "" {
			out = append(out, part)
		}
		start = cut
	}
	return out
}
