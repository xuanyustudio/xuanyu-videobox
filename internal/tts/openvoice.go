package tts

import (
	"context"
	"fmt"
	"strings"
)

const (
	OpenVoiceBaseID     = "zh_base"
	OpenVoiceCloneID    = "clone"
	OpenVoiceBaseLabel  = "基础中文"
	OpenVoiceCloneLabel = "克隆音色"
)

// OpenVoice talks to the local OpenVoice HTTP API (default http://127.0.0.1:3212).
type OpenVoice struct {
	BaseURL   string
	Reference string
	Params    VoiceParams
}

func BuiltinOpenVoiceVoices() []VoiceOption {
	return []VoiceOption{
		{ID: OpenVoiceBaseID, Label: OpenVoiceBaseLabel},
		{ID: OpenVoiceCloneID, Label: OpenVoiceCloneLabel},
	}
}

func IsOpenVoiceID(id string) bool {
	id = strings.TrimSpace(id)
	switch id {
	case OpenVoiceBaseID, OpenVoiceCloneID, OpenVoiceBaseLabel, OpenVoiceCloneLabel, "ZH", "zh":
		return true
	default:
		return false
	}
}

func OpenVoiceLabelForID(id string) string {
	switch strings.TrimSpace(id) {
	case OpenVoiceCloneID, OpenVoiceCloneLabel:
		return OpenVoiceCloneLabel
	case OpenVoiceBaseID, OpenVoiceBaseLabel, "ZH", "zh", "":
		return OpenVoiceBaseLabel
	default:
		return id
	}
}

func OpenVoiceIDForLabel(label string) string {
	switch strings.TrimSpace(label) {
	case OpenVoiceCloneID, OpenVoiceCloneLabel:
		return OpenVoiceCloneID
	case OpenVoiceBaseID, OpenVoiceBaseLabel, "ZH", "zh", "":
		return OpenVoiceBaseID
	default:
		return label
	}
}

func ListOpenVoiceVoices(ctx context.Context, baseURL string) ([]VoiceOption, error) {
	baseURL = NormalizeLocalURL(baseURL, DefaultOpenVoiceURL)
	list, err := fetchLocalVoices(ctx, baseURL)
	if err != nil {
		return BuiltinOpenVoiceVoices(), localDownError("OpenVoice", OpenVoiceStartBat, err)
	}
	if len(list) == 0 {
		return BuiltinOpenVoiceVoices(), nil
	}
	return list, nil
}

func (o OpenVoice) Synthesize(ctx context.Context, text, voice, outPath string) (float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("openvoice: 文本为空")
	}
	id := OpenVoiceIDForLabel(voice)
	ref := strings.TrimSpace(o.Reference)
	if id == OpenVoiceCloneID && ref == "" {
		return 0, fmt.Errorf("克隆音色需要一段参考音频，请先在合成页或设置里选择 wav/mp3")
	}
	base := NormalizeLocalURL(o.BaseURL, DefaultOpenVoiceURL)
	speed := ClampVoiceParams(o.Params).Speed
	return synthesizeChunkedWAV(ctx, base, "OpenVoice", OpenVoiceStartBat, OpenVoiceMaxRunes, text, func(chunk string) any {
		payload := map[string]any{
			"text":     chunk,
			"language": "ZH",
			"speed":    speed,
			"voice":    id,
		}
		if id == OpenVoiceCloneID && ref != "" {
			payload["reference"] = ref
		}
		return payload
	}, outPath)
}
