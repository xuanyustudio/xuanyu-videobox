package tts_test

import (
	"testing"

	"github.com/narratoai/narrato-go/internal/tts"
)

func TestNormalizeEngine(t *testing.T) {
	if got := tts.NormalizeEngine(""); got != tts.EngineEdge {
		t.Fatalf("empty: %q", got)
	}
	if got := tts.NormalizeEngine("MiniMax"); got != tts.EngineMiniMax {
		t.Fatalf("minimax: %q", got)
	}
}

func TestEdgeParamStrings(t *testing.T) {
	if got := tts.EdgeRatePercent(1); got != "+0%" {
		t.Fatalf("rate 1: %q", got)
	}
	if got := tts.EdgeRatePercent(1.2); got != "+20%" {
		t.Fatalf("rate 1.2: %q", got)
	}
	if got := tts.EdgeRatePercent(0.8); got != "-20%" {
		t.Fatalf("rate 0.8: %q", got)
	}
	if got := tts.EdgeVolumePercent(1.5); got != "+50%" {
		t.Fatalf("vol 1.5: %q", got)
	}
	if got := tts.EdgePitchHz(0); got != "+0Hz" {
		t.Fatalf("pitch 0: %q", got)
	}
	if got := tts.EdgePitchHz(-4); got != "-4Hz" {
		t.Fatalf("pitch -4: %q", got)
	}
}

func TestMiniMaxParamClamps(t *testing.T) {
	if tts.MiniMaxSpeed(0) != 1 {
		t.Fatal("unset speed should default to 1")
	}
	if tts.MiniMaxSpeed(3) != 2 {
		t.Fatal("speed should clamp to 2")
	}
	if tts.MiniMaxPitch(20) != 12 {
		t.Fatal("pitch should clamp to 12")
	}
	if tts.MiniMaxVolume(0) != 1 {
		t.Fatal("unset volume should default to 1")
	}
}

func TestEstimateDurationSecAtSpeed(t *testing.T) {
	base := tts.EstimateDurationSec("你好世界你好世界")
	fast := tts.EstimateDurationSecAtSpeed("你好世界你好世界", 2)
	if fast >= base {
		t.Fatalf("faster speech should be shorter: base=%v fast=%v", base, fast)
	}
}
