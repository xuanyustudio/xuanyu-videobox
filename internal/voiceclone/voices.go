package voiceclone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	VoiceTypeAll        = "all"
	VoiceTypeSystem     = "system"
	VoiceTypeCloning    = "voice_cloning"
	VoiceTypeGeneration = "voice_generation"
)

// VoiceInfo is one entry from the MiniMax get_voice API.
type VoiceInfo struct {
	ID          string
	Name        string
	Kind        string
	Description string
	CreatedTime string
}

// KindLabel is a short Chinese label for the voice category.
func (v VoiceInfo) KindLabel() string {
	switch v.Kind {
	case VoiceTypeCloning:
		return "复刻"
	case VoiceTypeGeneration:
		return "文生"
	default:
		return "系统"
	}
}

// Display is the one-line text shown in the voice list.
func (v VoiceInfo) Display() string {
	label := v.KindLabel() + " · "
	if v.Name != "" && v.Name != v.ID {
		label += v.Name + "  "
	}
	label += v.ID
	if v.CreatedTime != "" {
		label += "  " + v.CreatedTime
	}
	return label
}

// ListVoices queries voice IDs available to the current MiniMax account.
// voiceType should be all, system, voice_cloning, or voice_generation.
func (c *Client) ListVoices(ctx context.Context, voiceType string) ([]VoiceInfo, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("请先在「设置」里填写 MiniMax API Key")
	}
	voiceType = strings.TrimSpace(voiceType)
	if voiceType == "" {
		voiceType = VoiceTypeAll
	}
	switch voiceType {
	case VoiceTypeAll, VoiceTypeSystem, VoiceTypeCloning, VoiceTypeGeneration:
	default:
		return nil, fmt.Errorf("不支持的音色类型: %s", voiceType)
	}
	raw, err := c.postJSON(ctx, "/v1/get_voice", map[string]any{"voice_type": voiceType})
	if err != nil {
		return nil, fmt.Errorf("查询音色失败: %w", err)
	}
	var parsed struct {
		SystemVoice     []rawVoice `json:"system_voice"`
		VoiceCloning    []rawVoice `json:"voice_cloning"`
		VoiceGeneration []rawVoice `json:"voice_generation"`
		BaseResp        baseResp   `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("查询音色响应无法解析: %w", err)
	}
	if err := parsed.BaseResp.err(); err != nil {
		return nil, err
	}
	out := make([]VoiceInfo, 0, len(parsed.VoiceCloning)+len(parsed.VoiceGeneration)+len(parsed.SystemVoice))
	out = append(out, collectVoices(parsed.VoiceCloning, VoiceTypeCloning)...)
	out = append(out, collectVoices(parsed.VoiceGeneration, VoiceTypeGeneration)...)
	out = append(out, collectVoices(parsed.SystemVoice, VoiceTypeSystem)...)
	return out, nil
}

// FilterVoices keeps voices of the given kind, or all when kind is all/empty.
func FilterVoices(voices []VoiceInfo, kind string) []VoiceInfo {
	kind = strings.TrimSpace(kind)
	if kind == "" || kind == VoiceTypeAll {
		return append([]VoiceInfo(nil), voices...)
	}
	out := make([]VoiceInfo, 0, len(voices))
	for _, v := range voices {
		if v.Kind == kind {
			out = append(out, v)
		}
	}
	return out
}

type rawVoice struct {
	VoiceID     string     `json:"voice_id"`
	VoiceName   string     `json:"voice_name"`
	Description stringList `json:"description"`
	CreatedTime string     `json:"created_time"`
}

func collectVoices(in []rawVoice, kind string) []VoiceInfo {
	out := make([]VoiceInfo, 0, len(in))
	for _, r := range in {
		id := strings.TrimSpace(r.VoiceID)
		if id == "" {
			continue
		}
		out = append(out, VoiceInfo{
			ID:          id,
			Name:        strings.TrimSpace(r.VoiceName),
			Kind:        kind,
			Description: strings.Join(r.Description, " "),
			CreatedTime: strings.TrimSpace(r.CreatedTime),
		})
	}
	return out
}

// stringList accepts either a JSON string or an array of strings.
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = nil
		return nil
	}
	if b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		one = strings.TrimSpace(one)
		if one == "" {
			*s = nil
			return nil
		}
		*s = []string{one}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	*s = out
	return nil
}
