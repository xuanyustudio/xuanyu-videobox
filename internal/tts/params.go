package tts

import (
	"fmt"
	"math"
	"strings"
)

const (
	EngineEdge      = "edge"
	EngineMiniMax   = "minimax"
	EngineKokoro    = "kokoro"
	EngineOpenVoice = "openvoice"
	EngineIndexTTS2 = "indextts2"

	DefaultSpeed  = 1.0
	DefaultVolume = 1.0
	DefaultPitch  = 0.0
	MinSpeed      = 0.5
	MaxSpeed      = 2.0
	MinVolume     = 0.1
	MaxVolume     = 2.0
	MinPitch      = -12.0
	MaxPitch      = 12.0
)

// VoiceParams are synthesis-time rate / volume / pitch settings.
type VoiceParams struct {
	Speed  float64
	Volume float64
	Pitch  float64
}

// NormalizeEngine maps a stored engine name to a known engine id.
func NormalizeEngine(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case EngineMiniMax:
		return EngineMiniMax
	case EngineKokoro, "kokoro-tts":
		return EngineKokoro
	case EngineOpenVoice, "open-voice":
		return EngineOpenVoice
	case EngineIndexTTS2, "index-tts2", "index_tts2", "indextts-2":
		return EngineIndexTTS2
	default:
		return EngineEdge
	}
}

// ClampVoiceParams returns a copy with values limited to supported ranges.
// Zero speed/volume (unset) fall back to defaults; pitch 0 is a valid value.
func ClampVoiceParams(p VoiceParams) VoiceParams {
	if p.Speed == 0 {
		p.Speed = DefaultSpeed
	}
	if p.Volume == 0 {
		p.Volume = DefaultVolume
	}
	p.Speed = clampFloat(p.Speed, MinSpeed, MaxSpeed)
	p.Volume = clampFloat(p.Volume, MinVolume, MaxVolume)
	p.Pitch = clampFloat(p.Pitch, MinPitch, MaxPitch)
	return p
}

// EstimateDurationSecAtSpeed scales EstimateDurationSec by speaking rate.
func EstimateDurationSecAtSpeed(text string, speed float64) float64 {
	if speed <= 0 {
		speed = DefaultSpeed
	}
	speed = clampFloat(speed, MinSpeed, MaxSpeed)
	return math.Max(1, EstimateDurationSec(text)/speed)
}

// EdgeRatePercent is the rate string Edge TTS expects, e.g. "+20%" or "-10%".
func EdgeRatePercent(speed float64) string {
	speed = ClampVoiceParams(VoiceParams{Speed: speed, Volume: 1}).Speed
	return formatEdgePercent((speed - 1) * 100)
}

// EdgeVolumePercent is the volume string Edge TTS expects.
func EdgeVolumePercent(volume float64) string {
	volume = ClampVoiceParams(VoiceParams{Speed: 1, Volume: volume}).Volume
	return formatEdgePercent((volume - 1) * 100)
}

// EdgePitchHz is the pitch string Edge TTS expects, e.g. "+4Hz".
func EdgePitchHz(pitch float64) string {
	pitch = ClampVoiceParams(VoiceParams{Speed: 1, Volume: 1, Pitch: pitch}).Pitch
	n := int(math.Round(pitch))
	if n == 0 {
		return "+0Hz"
	}
	return fmt.Sprintf("%+dHz", n)
}

// MiniMaxSpeed returns the clamped speed for the T2A API.
func MiniMaxSpeed(speed float64) float64 {
	return ClampVoiceParams(VoiceParams{Speed: speed, Volume: 1}).Speed
}

// MiniMaxVolume returns the clamped volume for the T2A API.
func MiniMaxVolume(volume float64) float64 {
	return ClampVoiceParams(VoiceParams{Speed: 1, Volume: volume}).Volume
}

// MiniMaxPitch returns the clamped integer pitch for the T2A API.
func MiniMaxPitch(pitch float64) int {
	p := ClampVoiceParams(VoiceParams{Speed: 1, Volume: 1, Pitch: pitch}).Pitch
	return int(math.Round(p))
}

func formatEdgePercent(pct float64) string {
	n := int(math.Round(pct))
	if n == 0 {
		return "+0%"
	}
	return fmt.Sprintf("%+d%%", n)
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
