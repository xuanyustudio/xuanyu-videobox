package tts

import (
	"fmt"
	"strings"

	"github.com/narratoai/narrato-go/internal/voiceclone"
)

// SynthRequest holds everything needed to build a synthesizer.
type SynthRequest struct {
	Engine             string
	Params             VoiceParams
	MiniMaxBaseURL     string
	MiniMaxAPIKey      string
	MiniMaxModel       string
	KokoroURL          string
	OpenVoiceURL        string
	OpenVoiceReference  string
	IndexTTS2URL        string
	IndexTTS2Reference  string
}

// NewSynthesizer builds an Edge or MiniMax synthesizer from stored settings.
// MiniMax never falls back to a silent Fake voice.
func NewSynthesizer(engine, minimaxBaseURL, minimaxAPIKey, minimaxModel string, params VoiceParams) (Synthesizer, error) {
	return NewSynthesizerFromRequest(SynthRequest{
		Engine:         engine,
		Params:         params,
		MiniMaxBaseURL: minimaxBaseURL,
		MiniMaxAPIKey:  minimaxAPIKey,
		MiniMaxModel:   minimaxModel,
	})
}

// NewSynthesizerFromRequest builds Edge, MiniMax, Kokoro, OpenVoice or IndexTTS-2 synthesizers.
// Unknown or empty engines still fall back to Edge (not to Fake).
func NewSynthesizerFromRequest(req SynthRequest) (Synthesizer, error) {
	params := ClampVoiceParams(req.Params)
	switch NormalizeEngine(req.Engine) {
	case EngineMiniMax:
		if strings.TrimSpace(req.MiniMaxAPIKey) == "" {
			return nil, fmt.Errorf("请先在「设置」里填写 MiniMax API Key")
		}
		return MiniMax{
			Client: &voiceclone.Client{
				BaseURL: req.MiniMaxBaseURL,
				APIKey:  req.MiniMaxAPIKey,
				Model:   req.MiniMaxModel,
			},
			Params: params,
		}, nil
	case EngineKokoro:
		return Kokoro{BaseURL: req.KokoroURL, Params: params}, nil
	case EngineOpenVoice:
		return OpenVoice{BaseURL: req.OpenVoiceURL, Reference: req.OpenVoiceReference, Params: params}, nil
	case EngineIndexTTS2:
		return IndexTTS2{BaseURL: req.IndexTTS2URL, Reference: req.IndexTTS2Reference, Params: params}, nil
	default:
		return Edge{Params: params}, nil
	}
}
