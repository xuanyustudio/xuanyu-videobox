package tts

import (
	"sort"
	"strings"
)

// Recommendation levels for 电影解说 / 短剧解说 voice lists.
const (
	RecNone        = 0
	RecCommon      = 2
	RecRecommended = 3
)

// RatingMark is the visible grade appended after a voice name.
func RatingMark(level int) string {
	switch {
	case level >= RecRecommended:
		return "★★★ 推荐"
	case level >= RecCommon:
		return "★★ 常用"
	default:
		return ""
	}
}

// WithRatingMark appends a recommendation grade when level is non-zero.
func WithRatingMark(label string, level int) string {
	label = strings.TrimSpace(label)
	if m := RatingMark(level); m != "" {
		return label + "  " + m
	}
	return label
}

// StripRatingMark removes a trailing recommendation grade from a UI label.
func StripRatingMark(s string) string {
	s = strings.TrimSpace(s)
	for _, m := range []string{
		"  ★★★ 推荐", " ★★★ 推荐", "★★★ 推荐",
		"  ★★ 常用", " ★★ 常用", "★★ 常用",
	} {
		s = strings.TrimSuffix(s, m)
		s = strings.TrimSpace(s)
	}
	return s
}

// EdgeRecommendation returns a narration recommendation level for an Edge voice ID.
// Ratings follow common 电影解说 / 短剧解说 / 短视频旁白 practice:
// 云希、云健、云扬、晓晓 are the voices most often chosen for commentary;
// 晓伊、晓墨、晓秋、云泽、云野 are frequently used backups.
func EdgeRecommendation(id string) int {
	id = NormalizeVoiceID(id)
	if v, ok := edgeRatings[id]; ok {
		return v
	}
	for k, v := range edgeRatings {
		if strings.EqualFold(k, id) {
			return v
		}
	}
	return RecNone
}

// MiniMaxRecommendation returns a narration recommendation level for a MiniMax
// system voice ID. Cloned / text-generated voices should be passed through
// MiniMaxVoiceLevel with their kind so they stay unmarked.
func MiniMaxRecommendation(id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return RecNone
	}
	if v, ok := miniMaxRatings[id]; ok {
		return v
	}
	lower := strings.ToLower(id)
	for k, v := range miniMaxRatings {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	// Official example IDs (audiobook_*, presenter_*) may still appear on
	// some accounts even when omitted from the latest public table.
	switch {
	case strings.HasPrefix(lower, "audiobook_male"),
		strings.HasPrefix(lower, "audiobook_female"),
		strings.HasPrefix(lower, "presenter_male"),
		strings.HasPrefix(lower, "presenter_female"):
		return RecRecommended
	}
	return RecNone
}

// MiniMaxVoiceLevel is MiniMaxRecommendation for system voices, else 0.
func MiniMaxVoiceLevel(id, kind string) int {
	switch strings.TrimSpace(kind) {
	case "voice_cloning", "voice_generation":
		return RecNone
	default:
		return MiniMaxRecommendation(id)
	}
}

// MiniMaxSortKey groups list order: recommended system first, then common,
// then user clones / generated voices, then remaining system voices.
func MiniMaxSortKey(level int, kind string) int {
	if level >= RecCommon {
		return -level
	}
	switch strings.TrimSpace(kind) {
	case "voice_cloning", "voice_generation":
		return 10
	default:
		return 20
	}
}

// LessMiniMax reports whether voice i should appear before voice j.
func LessMiniMax(idI, kindI, idJ, kindJ string) bool {
	li := MiniMaxVoiceLevel(idI, kindI)
	lj := MiniMaxVoiceLevel(idJ, kindJ)
	ki := MiniMaxSortKey(li, kindI)
	kj := MiniMaxSortKey(lj, kindJ)
	return ki < kj
}

var edgeRatings = map[string]int{
	"zh-CN-YunxiNeural":    RecRecommended, // 短视频 / 影视解说最常被点名
	"zh-CN-YunjianNeural":  RecRecommended, // 纪录片、短剧男声解说常用
	"zh-CN-YunyangNeural":  RecRecommended, // 新闻 / 正式解说腔
	"zh-CN-XiaoxiaoNeural": RecRecommended, // 默认女声，综合解说最常用
	"zh-CN-XiaoyiNeural":   RecCommon,      // 故事 / 情感向
	"zh-CN-XiaomoNeural":   RecCommon,      // 播音、情绪表达
	"zh-CN-XiaoqiuNeural":  RecCommon,      // 沉稳讲述
	"zh-CN-YunzeNeural":    RecCommon,      // 沉稳男声
	"zh-CN-YunyeNeural":    RecCommon,      // 成熟男声
}

var miniMaxRatings = map[string]int{
	"Chinese (Mandarin)_Male_Announcer":     RecRecommended, // 播报男声
	"Chinese (Mandarin)_Radio_Host":         RecRecommended, // 电台男主播
	"Chinese (Mandarin)_News_Anchor":        RecRecommended, // 新闻女声
	"Chinese (Mandarin)_Lyrical_Voice":      RecRecommended, // 抒情男声
	"Chinese (Mandarin)_Gentleman":          RecRecommended, // 温润男声
	"audiobook_male_1":                      RecRecommended,
	"audiobook_male_2":                      RecRecommended,
	"audiobook_female_1":                    RecRecommended,
	"audiobook_female_2":                    RecRecommended,
	"presenter_male":                        RecRecommended,
	"presenter_female":                      RecRecommended,
	"male-qn-qingse":                        RecCommon, // 官方列表首位，最常用默认男声
	"male-qn-jingying":                      RecCommon, // 精英青年，更偏稳重旁白
	"male-qn-qingse-jingpin":                RecCommon,
	"male-qn-jingying-jingpin":              RecCommon,
	"female-chengshu":                       RecCommon, // 成熟女声，适合旁白
	"female-yujie":                          RecCommon, // 短剧女声旁白常用
	"female-chengshu-jingpin":               RecCommon,
	"female-yujie-jingpin":                  RecCommon,
	"Chinese (Mandarin)_Reliable_Executive": RecCommon, // 沉稳高管
	"Chinese (Mandarin)_Wise_Women":         RecCommon, // 阅历姐姐
	"Chinese (Mandarin)_Gentle_Youth":       RecCommon, // 温润青年
}

// IndexTTS2Recommendation returns a 电影解说 recommendation for an official
// IndexTTS-2 example voice. Ratings come from listening to / analyzing the
// official prompt wavs in E:\indextts2\examples (pitch, timbre, language,
// and delivery). 克隆音色 is unmarked (custom reference).
func IndexTTS2Recommendation(id string) int {
	id = IndexTTS2IDForLabel(id)
	if id == IndexTTS2CloneID {
		return RecNone
	}
	if v, ok := indexTTS2Ratings[id]; ok {
		return v
	}
	if stem := indexTTS2StemFromID(id); stem != "" {
		if v, ok := indexTTS2Ratings[IndexTTS2VoiceID(stem)]; ok {
			return v
		}
	}
	return RecNone
}

var indexTTS2Ratings = map[string]int{
	"indextts2_voice_05": RecRecommended, // 男声沉稳，接近纪录片旁白，最适合电影解说
	"indextts2_voice_03": RecCommon,      // 普通话女声口播，可作解说备选
	"indextts2_voice_04": RecCommon,      // 沉稳女声，偏专业讲述
	"indextts2_voice_06": RecCommon,      // 普通话讲述腔，可作旁白常用
	// 01/02 为英语，08 偏外语/过亮，07/09/11 偏情绪对白，12 偏低沉惊吓腔，均不作为解说推荐
}

// RankedIndexTTS2Voices sorts official examples by recommendation (recommended
// first) and appends 克隆音色 last. Labels include the same ★ marks as Edge.
func RankedIndexTTS2Voices(list []VoiceOption) []VoiceOption {
	if len(list) == 0 {
		list = BuiltinIndexTTS2Voices()
	}
	official := make([]VoiceOption, 0, len(list))
	clones := make([]VoiceOption, 0, 1)
	for _, v := range list {
		id := IndexTTS2IDForLabel(v.ID)
		lab := StripRatingMark(v.Label)
		if lab == "" {
			lab = IndexTTS2LabelForID(id)
		}
		item := VoiceOption{ID: id, Label: WithRatingMark(lab, IndexTTS2Recommendation(id))}
		if id == IndexTTS2CloneID {
			clones = append(clones, item)
		} else {
			official = append(official, item)
		}
	}
	sort.SliceStable(official, func(i, j int) bool {
		li := IndexTTS2Recommendation(official[i].ID)
		lj := IndexTTS2Recommendation(official[j].ID)
		if li != lj {
			return li > lj
		}
		return official[i].ID < official[j].ID
	})
	return append(official, clones...)
}
