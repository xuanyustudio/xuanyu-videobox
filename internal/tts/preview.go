package tts

import (
	"strings"

	"github.com/narratoai/narrato-go/internal/script"
)

// DefaultPreviewText is used when the script has no narration line to preview.
const DefaultPreviewText = "你好，这是这段音色的试听。"

const defaultPreviewMaxRunes = 80

// PreviewText picks the first narration line that needs TTS (OST 0 or 2).
// If none is available, it returns DefaultPreviewText. Text is truncated
// to maxRunes (default 80).
func PreviewText(items []script.Item, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultPreviewMaxRunes
	}
	for _, item := range items {
		if item.OST != 0 && item.OST != 2 {
			continue
		}
		text := strings.TrimSpace(item.Narration)
		if text == "" {
			continue
		}
		return TruncateRunes(text, maxRunes)
	}
	return DefaultPreviewText
}

// TruncateRunes shortens s to at most n runes.
func TruncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
