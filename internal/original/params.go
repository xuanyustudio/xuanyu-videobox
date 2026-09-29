package original

import (
	"fmt"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
)

// OpSettings is one enabled/disabled op with intensity 0–100.
type OpSettings struct {
	ID        string
	Enabled   bool
	Intensity int
}

// Params drives one originality ffmpeg job (preview or full export).
type Params struct {
	VideoPath   string
	OutputPath  string
	Settings    []OpSettings
	SubtitleSRT string
	Style       SubtitleStyle
	BGMPath     string
	BGMVolume   float64 // 0–100 preferred; also accepts 0–1
	// Watermark
	WatermarkText   string // default Narrato
	WatermarkCorner string // tl|tr|bl|br
	// PreviewSeconds > 0 limits output duration (preview mode).
	PreviewSeconds float64
	Bins           media.FFmpegBins
	OnProgress     func(percent int, msg string)
}

// DefaultWatermarkText is used when watermark is on but text is empty.
const DefaultWatermarkText = "Narrato"

// NormalizeSettings clamps intensities, applies zoom mutual exclusion (last wins),
// and drops unknown ids. Returns normalized copy and optional notice.
func NormalizeSettings(in []OpSettings) (out []OpSettings, notice string) {
	seen := map[string]int{} // id → index in out
	for _, s := range in {
		id := strings.TrimSpace(s.ID)
		if id == "" {
			continue
		}
		def, ok := LookupOp(id)
		if !ok {
			continue
		}
		s.ID = id
		s.Intensity = clampInt(s.Intensity, def.MinInt, def.MaxInt)
		if !def.HasIntensity {
			s.Intensity = 0
		}
		if idx, exists := seen[id]; exists {
			out[idx] = s
			continue
		}
		seen[id] = len(out)
		out = append(out, s)
	}

	// zoom_in / zoom_out mutual exclusion: keep the one that appears later in out.
	zi, zo := -1, -1
	for i, s := range out {
		if !s.Enabled {
			continue
		}
		switch s.ID {
		case "zoom_in":
			zi = i
		case "zoom_out":
			zo = i
		}
	}
	if zi >= 0 && zo >= 0 {
		if zi > zo {
			out[zo].Enabled = false
			notice = "已关闭「缩小留边」（与微裁切放大互斥）"
		} else {
			out[zi].Enabled = false
			notice = "已关闭「微裁切放大」（与缩小留边互斥）"
		}
	}
	return out, notice
}

// EnabledCount returns how many settings are enabled.
func EnabledCount(settings []OpSettings) int {
	n := 0
	for _, s := range settings {
		if s.Enabled {
			n++
		}
	}
	return n
}

// DefaultSettings returns registry defaults (enabled flags + intensities).
func DefaultSettings() []OpSettings {
	defs := AllOpDefs()
	out := make([]OpSettings, 0, len(defs))
	for _, d := range defs {
		out = append(out, OpSettings{
			ID:        d.ID,
			Enabled:   d.DefaultOn,
			Intensity: d.DefaultInt,
		})
	}
	return out
}

// HumanSummary lists enabled op names for status / learning panel.
func HumanSummary(settings []OpSettings) string {
	var names []string
	for _, s := range settings {
		if !s.Enabled {
			continue
		}
		names = append(names, LabelFor(s.ID))
	}
	if len(names) == 0 {
		return "（未选择）"
	}
	return strings.Join(names, " + ")
}

// ValidateParams checks required fields before run/preview.
func ValidateParams(p Params) error {
	if strings.TrimSpace(p.VideoPath) == "" {
		return fmt.Errorf("请选择视频文件")
	}
	norm, _ := NormalizeSettings(p.Settings)
	if EnabledCount(norm) == 0 {
		return fmt.Errorf("请选择至少一种处理方式")
	}
	return nil
}
