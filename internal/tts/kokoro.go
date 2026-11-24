package tts

import (
	"context"
	"fmt"
	"strings"
)

// Kokoro talks to the local Kokoro HTTP API (default http://127.0.0.1:3211).
type Kokoro struct {
	BaseURL string
	Params  VoiceParams
}

// BuiltinKokoroVoices matches E:\kokoro-tts\server.py VOICE_LIST.
func BuiltinKokoroVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "zf_001", Label: "女声 001 (温柔)"},
		{ID: "zf_002", Label: "女声 002"},
		{ID: "zf_003", Label: "女声 003"},
		{ID: "zf_004", Label: "女声 004"},
		{ID: "zf_005", Label: "女声 005"},
		{ID: "zf_006", Label: "女声 006"},
		{ID: "zf_007", Label: "女声 007"},
		{ID: "zf_008", Label: "女声 008"},
		{ID: "zm_009", Label: "男声 009 (沉稳)"},
		{ID: "zm_010", Label: "男声 010"},
		{ID: "zm_011", Label: "男声 011"},
		{ID: "zm_012", Label: "男声 012"},
		{ID: "zm_013", Label: "男声 013"},
		{ID: "zm_014", Label: "男声 014"},
		{ID: "zm_015", Label: "男声 015"},
		{ID: "zm_016", Label: "男声 016"},
	}
}

func IsKokoroVoice(id string) bool {
	id = strings.TrimSpace(id)
	for _, v := range BuiltinKokoroVoices() {
		if strings.EqualFold(v.ID, id) || v.Label == id {
			return true
		}
	}
	return strings.HasPrefix(strings.ToLower(id), "zf_") || strings.HasPrefix(strings.ToLower(id), "zm_")
}

func KokoroLabelForID(id string) string {
	id = strings.TrimSpace(id)
	for _, v := range BuiltinKokoroVoices() {
		if strings.EqualFold(v.ID, id) {
			return v.Label
		}
	}
	if id == "" {
		return BuiltinKokoroVoices()[0].Label
	}
	return id
}

func KokoroIDForLabel(label string) string {
	label = strings.TrimSpace(label)
	for _, v := range BuiltinKokoroVoices() {
		if v.Label == label || strings.EqualFold(v.ID, label) {
			return v.ID
		}
	}
	return label
}

func ListKokoroVoices(ctx context.Context, baseURL string) ([]VoiceOption, error) {
	baseURL = NormalizeLocalURL(baseURL, DefaultKokoroURL)
	list, err := fetchLocalVoices(ctx, baseURL)
	if err != nil {
		return BuiltinKokoroVoices(), localDownError("Kokoro", KokoroStartBat, err)
	}
	if len(list) == 0 {
		return BuiltinKokoroVoices(), nil
	}
	return list, nil
}

func (k Kokoro) Synthesize(ctx context.Context, text, voice, outPath string) (float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("kokoro: 文本为空")
	}
	voice = strings.TrimSpace(voice)
	if voice == "" || IsEdgeVoice(voice) || IsOpenVoiceID(voice) || IsIndexTTS2ID(voice) {
		voice = "zf_001"
	}
	voice = KokoroIDForLabel(voice)
	base := NormalizeLocalURL(k.BaseURL, DefaultKokoroURL)
	speed := ClampVoiceParams(k.Params).Speed
	return synthesizeChunkedWAV(ctx, base, "Kokoro", KokoroStartBat, KokoroMaxRunes, text, func(chunk string) any {
		return map[string]any{"text": chunk, "voice": voice, "speed": speed}
	}, outPath)
}
