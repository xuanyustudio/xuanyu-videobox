package media

import (
	"math"

	"github.com/narratoai/narrato-go/internal/script"
)

// ResolveClipDuration computes clip length from TTS seconds (matches Python resolve_script_clip_duration_sec).
func ResolveClipDuration(ttsSec float64, isOpeningHook bool) float64 {
	tts := math.Max(0, ttsSec)
	if isOpeningHook {
		return math.Max(tts, script.OpeningHookMin)
	}
	padded := tts + script.TailPad
	return math.Max(padded, script.GeneralClipMin)
}
