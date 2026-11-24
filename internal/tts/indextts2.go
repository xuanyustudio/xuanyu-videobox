package tts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// IndexTTS2ExampleID is a backward-compatible alias of official example 01.
	IndexTTS2ExampleID    = "indextts2_example"
	IndexTTS2CloneID      = "indextts2_clone"
	IndexTTS2ExampleLabel = "默认示例音色"
	IndexTTS2CloneLabel   = "克隆音色"
	// IndexTTS2MaxRunes stays under the model's default 120-token segment size.
	IndexTTS2MaxRunes = 120
	// IndexTTS2PreviewTimeout covers first-request latency after the model is loaded.
	IndexTTS2PreviewTimeout = 3 * time.Minute
)

// Official example stems shipped in E:\indextts2\examples (voice_10 is not in the pack).
var indexTTS2OfficialStems = []string{
	"voice_01", "voice_02", "voice_03", "voice_04", "voice_05",
	"voice_06", "voice_07", "voice_08", "voice_09", "voice_11", "voice_12",
}

// IndexTTS2ExamplesDir is the official example-wav folder on this install.
var IndexTTS2ExamplesDir = `E:\indextts2\examples`

var (
	indexTTS2StemRe  = regexp.MustCompile(`(?i)^(?:indextts2_)?voice_(\d{1,2})$`)
	indexTTS2LabelRe = regexp.MustCompile(`^官方示例\s*0*(\d{1,2})$`)
)

// IndexTTS2 talks to the local IndexTTS-2 HTTP API (default http://127.0.0.1:3213).
type IndexTTS2 struct {
	BaseURL   string
	Reference string
	Params    VoiceParams
}

func IndexTTS2VoiceID(stem string) string {
	return "indextts2_" + stem
}

func IndexTTS2VoiceLabel(stem string) string {
	n := strings.TrimPrefix(stem, "voice_")
	if len(n) == 1 {
		n = "0" + n
	}
	return "官方示例 " + n
}

func BuiltinIndexTTS2Voices() []VoiceOption {
	out := make([]VoiceOption, 0, len(indexTTS2OfficialStems)+1)
	for _, stem := range indexTTS2OfficialStems {
		out = append(out, VoiceOption{ID: IndexTTS2VoiceID(stem), Label: IndexTTS2VoiceLabel(stem)})
	}
	out = append(out, VoiceOption{ID: IndexTTS2CloneID, Label: IndexTTS2CloneLabel})
	return out
}

func indexTTS2StemFromID(id string) string {
	id = StripRatingMark(strings.TrimSpace(id))
	switch strings.ToLower(id) {
	case "":
		return ""
	case IndexTTS2ExampleID, "example", "默认示例", "默认示例音色", "官方示例音色":
		return "voice_01"
	case IndexTTS2CloneID, "clone", "克隆音色":
		return ""
	}
	if m := indexTTS2LabelRe.FindStringSubmatch(id); m != nil {
		n, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("voice_%02d", n)
	}
	if m := indexTTS2StemRe.FindStringSubmatch(id); m != nil {
		n, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("voice_%02d", n)
	}
	return ""
}

func IsIndexTTS2ID(id string) bool {
	id = StripRatingMark(strings.TrimSpace(id))
	if id == "" {
		return false
	}
	switch id {
	case IndexTTS2ExampleID, IndexTTS2CloneID, IndexTTS2ExampleLabel, IndexTTS2CloneLabel,
		"example", "clone", "默认示例", "官方示例音色":
		return true
	default:
		return indexTTS2StemFromID(id) != ""
	}
}

func IndexTTS2LabelForID(id string) string {
	switch StripRatingMark(strings.TrimSpace(id)) {
	case IndexTTS2CloneID, IndexTTS2CloneLabel, "clone":
		return IndexTTS2CloneLabel
	default:
		if stem := indexTTS2StemFromID(id); stem != "" {
			return IndexTTS2VoiceLabel(stem)
		}
		if strings.TrimSpace(id) == "" {
			return IndexTTS2VoiceLabel("voice_01")
		}
		return id
	}
}

func IndexTTS2IDForLabel(label string) string {
	switch StripRatingMark(strings.TrimSpace(label)) {
	case IndexTTS2CloneID, IndexTTS2CloneLabel, "clone":
		return IndexTTS2CloneID
	default:
		if stem := indexTTS2StemFromID(label); stem != "" {
			return IndexTTS2VoiceID(stem)
		}
		if strings.TrimSpace(label) == "" {
			return IndexTTS2VoiceID("voice_01")
		}
		return label
	}
}

// IndexTTS2ExamplePath returns the official example wav for a preset id, if present.
func IndexTTS2ExamplePath(id string) string {
	stem := indexTTS2StemFromID(id)
	if stem == "" {
		return ""
	}
	p := filepath.Join(IndexTTS2ExamplesDir, stem+".wav")
	st, err := os.Stat(p)
	if err != nil || st.IsDir() || st.Size() <= 0 {
		return ""
	}
	return p
}

func mergeIndexTTS2Voices(api []VoiceOption) []VoiceOption {
	out := BuiltinIndexTTS2Voices()
	seen := make(map[string]bool, len(out))
	for _, v := range out {
		seen[v.ID] = true
	}
	extra := make([]VoiceOption, 0)
	for _, v := range api {
		id := IndexTTS2IDForLabel(strings.TrimSpace(v.ID))
		if id == IndexTTS2ExampleID {
			id = IndexTTS2VoiceID("voice_01")
		}
		if id == "" || id == IndexTTS2CloneID || seen[id] {
			continue
		}
		lab := strings.TrimSpace(v.Label)
		if lab == "" || lab == IndexTTS2ExampleLabel {
			lab = IndexTTS2LabelForID(id)
		}
		extra = append(extra, VoiceOption{ID: id, Label: lab})
		seen[id] = true
	}
	if len(extra) == 0 {
		return out
	}
	clone := out[len(out)-1]
	out = append(out[:len(out)-1], extra...)
	return append(out, clone)
}

func ListIndexTTS2Voices(ctx context.Context, baseURL string) ([]VoiceOption, error) {
	baseURL = NormalizeLocalURL(baseURL, DefaultIndexTTS2URL)
	list, err := fetchLocalVoices(ctx, baseURL)
	if err != nil {
		return BuiltinIndexTTS2Voices(), localDownError("IndexTTS-2", IndexTTS2StartBat, err)
	}
	return mergeIndexTTS2Voices(list), nil
}

func (x IndexTTS2) Synthesize(ctx context.Context, text, voice, outPath string) (float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("indextts2: 文本为空")
	}
	id := IndexTTS2IDForLabel(voice)
	ref := strings.TrimSpace(x.Reference)
	if id == IndexTTS2CloneID && ref == "" {
		return 0, fmt.Errorf("克隆音色需要一段参考音频，请先在合成页或设置里选择 wav/mp3")
	}
	base := NormalizeLocalURL(x.BaseURL, DefaultIndexTTS2URL)
	speed := ClampVoiceParams(x.Params).Speed
	return synthesizeChunkedWAV(ctx, base, "IndexTTS-2", IndexTTS2StartBat, IndexTTS2MaxRunes, text, func(chunk string) any {
		payload := map[string]any{
			"text":  chunk,
			"voice": id,
			"speed": speed,
		}
		if id == IndexTTS2CloneID && ref != "" {
			payload["reference"] = ref
		} else if p := IndexTTS2ExamplePath(id); p != "" {
			// Send the official wav path so an older API (which only
			// understood "example") still uses the selected speaker.
			payload["reference"] = p
		}
		return payload
	}, outPath)
}
