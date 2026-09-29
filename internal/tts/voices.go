package tts

import "strings"

// VoiceOption is a selectable Edge TTS voice shown in the UI.
type VoiceOption struct {
	ID    string
	Label string
}

// EdgeVoices returns common Chinese Edge Neural voices (short-drama MVP).
func EdgeVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "zh-CN-XiaoxiaoNeural", Label: "晓晓（女）· 普通话"},
		{ID: "zh-CN-XiaoyiNeural", Label: "晓伊（女）· 普通话"},
		{ID: "zh-CN-XiaochenNeural", Label: "晓辰（女）· 普通话"},
		{ID: "zh-CN-XiaohanNeural", Label: "晓涵（女）· 普通话"},
		{ID: "zh-CN-XiaomengNeural", Label: "晓梦（女）· 普通话"},
		{ID: "zh-CN-XiaomoNeural", Label: "晓墨（女）· 普通话"},
		{ID: "zh-CN-XiaoqiuNeural", Label: "晓秋（女）· 普通话"},
		{ID: "zh-CN-XiaoruiNeural", Label: "晓睿（女）· 普通话"},
		{ID: "zh-CN-XiaoshuangNeural", Label: "晓双（女）· 普通话"},
		{ID: "zh-CN-XiaoxuanNeural", Label: "晓萱（女）· 普通话"},
		{ID: "zh-CN-XiaoyanNeural", Label: "晓颜（女）· 普通话"},
		{ID: "zh-CN-XiaoyouNeural", Label: "晓悠（女）· 普通话"},
		{ID: "zh-CN-XiaozhenNeural", Label: "晓甄（女）· 普通话"},
		{ID: "zh-CN-YunjianNeural", Label: "云健（男）· 普通话"},
		{ID: "zh-CN-YunxiNeural", Label: "云希（男）· 普通话"},
		{ID: "zh-CN-YunxiaNeural", Label: "云夏（男）· 普通话"},
		{ID: "zh-CN-YunyangNeural", Label: "云扬（男）· 普通话"},
		{ID: "zh-CN-YunfengNeural", Label: "云枫（男）· 普通话"},
		{ID: "zh-CN-YunhaoNeural", Label: "云皓（男）· 普通话"},
		{ID: "zh-CN-YunyeNeural", Label: "云野（男）· 普通话"},
		{ID: "zh-CN-YunzeNeural", Label: "云泽（男）· 普通话"},
		{ID: "zh-CN-liaoning-XiaobeiNeural", Label: "晓北（女）· 辽宁"},
		{ID: "zh-CN-shaanxi-XiaoniNeural", Label: "晓妮（女）· 陕西"},
		{ID: "zh-HK-HiuMaanNeural", Label: "曉曼（女）· 粤语"},
		{ID: "zh-HK-HiuGaaiNeural", Label: "曉佳（女）· 粤语"},
		{ID: "zh-HK-WanLungNeural", Label: "雲龍（男）· 粤语"},
		{ID: "zh-TW-HsiaoChenNeural", Label: "曉臻（女）· 台湾"},
		{ID: "zh-TW-HsiaoYuNeural", Label: "曉雨（女）· 台湾"},
		{ID: "zh-TW-YunJheNeural", Label: "雲哲（男）· 台湾"},
	}
}

// NormalizeVoiceID strips Python-era "-Female"/"-Male" suffixes.
func NormalizeVoiceID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimSuffix(id, "-Female")
	id = strings.TrimSuffix(id, "-Male")
	id = strings.TrimSuffix(id, "-V2")
	return id
}

// VoiceLabels returns dropdown labels in the same order as EdgeVoices.
func VoiceLabels() []string {
	vs := EdgeVoices()
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Label)
	}
	return out
}

// VoiceIDForLabel maps a UI label back to the Edge voice ID.
func VoiceIDForLabel(label string) string {
	for _, v := range EdgeVoices() {
		if v.Label == label {
			return v.ID
		}
	}
	return NormalizeVoiceID(label)
}

// LabelForVoiceID maps a stored voice ID to the dropdown label.
func LabelForVoiceID(id string) string {
	id = NormalizeVoiceID(id)
	for _, v := range EdgeVoices() {
		if strings.EqualFold(v.ID, id) {
			return v.Label
		}
	}
	if id == "" {
		return EdgeVoices()[0].Label
	}
	return id
}
