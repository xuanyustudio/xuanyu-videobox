package tts

import (
	"context"
	"math"
)

// Synthesizer turns text into speech audio at outPath and returns estimated duration in seconds.
type Synthesizer interface {
	Synthesize(ctx context.Context, text, voice, outPath string) (durationSec float64, err error)
}

// EstimateDurationSec approximates narration length from rune count (matches Fake and clip planning).
func EstimateDurationSec(text string) float64 {
	sec := float64(len([]rune(text))) * 0.28
	return math.Max(1, sec)
}
