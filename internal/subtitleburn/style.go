package subtitleburn

import (
	"fmt"
	"strings"
)

// Style is the ASS force_style used when burning a subtitle.
type Style struct {
	FontSize     int
	FontName     string
	PrimaryColor string // #RRGGBB
	OutlineColor string
	Outline      int
	Shadow       int
	MarginV      int
}

// DefaultStyle matches original.DefaultSubtitleStyle.
func DefaultStyle() Style {
	return Style{
		FontSize:     22,
		FontName:     "Arial",
		PrimaryColor: "#FFFFFF",
		OutlineColor: "#000000",
		Outline:      2,
		Shadow:       1,
		MarginV:      36,
	}
}

// ForceStyle builds the ffmpeg force_style value.
// MarginV 0 stays 0. Only a negative margin falls back to 36.
func ForceStyle(s Style) string {
	d := DefaultStyle()
	if s.FontSize <= 0 {
		s.FontSize = d.FontSize
	}
	if s.FontName == "" {
		s.FontName = d.FontName
	}
	if s.PrimaryColor == "" {
		s.PrimaryColor = d.PrimaryColor
	}
	if s.OutlineColor == "" {
		s.OutlineColor = d.OutlineColor
	}
	if s.Outline < 0 {
		s.Outline = d.Outline
	}
	if s.Shadow < 0 {
		s.Shadow = d.Shadow
	}
	if s.MarginV < 0 {
		s.MarginV = d.MarginV
	}
	fn := strings.ReplaceAll(s.FontName, "'", `\'`)
	return fmt.Sprintf("FontSize=%d,FontName=%s,PrimaryColour=%s,OutlineColour=%s,Outline=%d,Shadow=%d,MarginV=%d",
		clamp(s.FontSize, 10, 72), fn, hexToAssBGR(s.PrimaryColor), hexToAssBGR(s.OutlineColor),
		clamp(s.Outline, 0, 8), clamp(s.Shadow, 0, 8), clamp(s.MarginV, 0, 200))
}

func hexToAssBGR(hex string) string {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return "&H00FFFFFF"
	}
	rr, gg, bb := hex[0:2], hex[2:4], hex[4:6]
	return "&H00" + strings.ToUpper(bb+gg+rr)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
