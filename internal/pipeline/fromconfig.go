package pipeline

import (
	"strings"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/sonilo"
	"github.com/narratoai/narrato-go/internal/subtitleburn"
	"github.com/narratoai/narrato-go/internal/tts"
)

// NewSynthesizerFromConfig builds the Edge or MiniMax synthesizer for a render.
func NewSynthesizerFromConfig(cfg *config.Config) (tts.Synthesizer, error) {
	if cfg == nil {
		return tts.Edge{}, nil
	}
	return tts.NewSynthesizerFromRequest(tts.SynthRequest{
		Engine:             cfg.TTS.Engine,
		Params:             tts.VoiceParams{Speed: cfg.TTS.Speed, Volume: cfg.TTS.Volume, Pitch: cfg.TTS.Pitch},
		MiniMaxBaseURL:     cfg.MiniMax.BaseURL,
		MiniMaxAPIKey:      cfg.MiniMax.APIKey,
		MiniMaxModel:       cfg.MiniMax.Model,
		KokoroURL:          cfg.TTS.KokoroURL,
		OpenVoiceURL:        cfg.TTS.OpenVoiceURL,
		OpenVoiceReference:  cfg.TTS.OpenVoiceReference,
		IndexTTS2URL:        cfg.TTS.IndexTTS2URL,
		IndexTTS2Reference:  cfg.TTS.IndexTTS2Reference,
	})
}

// ApplyConfig fills render options from persisted settings. Voice, items, paths and TTS stay with the caller.
func ApplyConfig(p *Params, cfg *config.Config) {
	if p == nil || cfg == nil {
		return
	}
	hide := cfg.UI.SubtitleHideOnOriginal
	p.HideOnOriginal = &hide
	p.OriginalVolume = cfg.UI.OriginalVolume
	p.VoiceVolume = cfg.UI.VoiceVolume
	p.BGMVolume = cfg.UI.BGMVolume
	p.SFXVolume = cfg.UI.SFXVolume
	p.BGMPath = strings.TrimSpace(cfg.UI.BGMPath)
	source := strings.ToLower(strings.TrimSpace(cfg.UI.BGMSource))
	switch source {
	case "off", "none", "":
		p.BGMPath = ""
		p.SoniloBGM = false
	case "sonilo":
		p.BGMPath = ""
		p.SoniloBGM = true
	default:
		p.SoniloBGM = false
	}
	p.SoniloSFX = cfg.UI.SFXEnabled
	p.SoniloBGMPrompt = cfg.Sonilo.BGMPrompt
	p.SoniloSFXPrompt = cfg.UI.SFXPrompt
	if p.SoniloBGM || p.SoniloSFX {
		if strings.TrimSpace(cfg.Sonilo.APIKey) != "" {
			p.Sonilo = &sonilo.Client{
				BaseURL: cfg.Sonilo.BaseURL,
				APIKey:  cfg.Sonilo.APIKey,
			}
		}
	}
	burn := cfg.UI.SubtitleBurnEnabled
	p.BurnSubtitles = &burn
	p.SubtitlePosition = cfg.UI.SubtitlePosition
	p.SubtitleOnMask = cfg.UI.SubtitleOnMask
	style := subtitleburn.DefaultStyle()
	if font := strings.TrimSpace(cfg.UI.SubtitleFont); font != "" {
		style.FontName = font
	}
	if cfg.UI.SubtitleFontSize > 0 {
		style.FontSize = cfg.UI.SubtitleFontSize
	}
	if color := strings.TrimSpace(cfg.UI.SubtitleColor); color != "" {
		style.PrimaryColor = color
	}
	p.SubtitleStyle = style
	p.Mask = media.SubtitleMask{
		Enabled: cfg.UI.SubtitleMaskEnabled,
		X:       cfg.UI.SubtitleMaskX,
		Y:       cfg.UI.SubtitleMaskY,
		Width:   cfg.UI.SubtitleMaskWidth,
		Height:  cfg.UI.SubtitleMaskHeight,
		Blur:    cfg.UI.SubtitleMaskBlur,
		Opacity: cfg.UI.SubtitleMaskOpacity,
	}
	p.Aspect = cfg.UI.AspectRatio
	p.Resolution = cfg.UI.Resolution
	if p.Voice == "" {
		p.Voice = cfg.TTS.Voice
	}
	p.Bins = media.FFmpegBins{FFmpeg: cfg.FFmpeg.Path, FFprobe: cfg.FFmpeg.FFprobePath}
}
