package tts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/narratoai/narrato-go/internal/voiceclone"
)

// MiniMax synthesizes speech via the MiniMax T2A API.
// It never falls back to a silent placeholder; errors are returned to the caller.
type MiniMax struct {
	Client *voiceclone.Client
	Params VoiceParams
}

func (m MiniMax) Synthesize(ctx context.Context, text, voice, outPath string) (float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("minimax tts: empty text")
	}
	if m.Client == nil {
		return 0, fmt.Errorf("请先在「设置」里填写 MiniMax API Key")
	}
	voice = strings.TrimSpace(voice)
	if voice == "" {
		return 0, fmt.Errorf("请选择 MiniMax 音色")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}

	p := ClampVoiceParams(m.Params)
	res, err := m.Client.SynthesizeWith(ctx, voice, text, voiceclone.SynthesizeOptions{
		Speed: MiniMaxSpeed(p.Speed),
		Vol:   MiniMaxVolume(p.Volume),
		Pitch: MiniMaxPitch(p.Pitch),
	})
	if err != nil {
		return 0, err
	}
	if len(res.Audio) < 2048 {
		return 0, fmt.Errorf("minimax tts: 输出过小 (%d bytes)，合成可能失败", len(res.Audio))
	}
	if err := os.WriteFile(outPath, res.Audio, 0o644); err != nil {
		return 0, err
	}
	dur := EstimateDurationSecAtSpeed(text, p.Speed)
	if res.LengthMS > 0 {
		actual := float64(res.LengthMS) / 1000
		if actual > dur {
			dur = actual
		}
	}
	return dur, nil
}
