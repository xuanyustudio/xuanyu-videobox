package subtitleburn

import (
	"fmt"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
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
	Alignment    int // ASS alignment; 0 means default 2 (bottom center)
}

// DefaultStyle matches original.DefaultSubtitleStyle, with font size 15.
func DefaultStyle() Style {
	return Style{
		FontSize:     15,
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
	align := s.Alignment
	if align <= 0 {
		align = 2
	}
	return fmt.Sprintf("FontSize=%d,FontName=%s,PrimaryColour=%s,OutlineColour=%s,Outline=%d,Shadow=%d,MarginV=%d,Alignment=%d",
		clamp(s.FontSize, 10, 72), fn, hexToAssBGR(s.PrimaryColor), hexToAssBGR(s.OutlineColor),
		clamp(s.Outline, 0, 8), clamp(s.Shadow, 0, 8), clamp(s.MarginV, 0, 4096), clamp(align, 1, 9))
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

// ApplyPosition sets ASS alignment and a default margin for top/center/bottom.
func ApplyPosition(style Style, position string) Style {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top":
		style.Alignment = 8
		if style.MarginV < 0 {
			style.MarginV = 36
		}
	case "center", "middle":
		style.Alignment = 5
		if style.MarginV < 0 {
			style.MarginV = 0
		}
	default:
		style.Alignment = 2
		if style.MarginV < 0 {
			style.MarginV = 36
		}
	}
	return style
}

// BurnStyle applies the saved position. When onMask is set, the narration
// is moved onto the mask band instead. onMask false leaves position unchanged.
func BurnStyle(style Style, position string, onMask bool, srcW, srcH, dstW, dstH int, mask media.SubtitleMask) Style {
	style = ApplyPosition(style, position)
	if !onMask {
		return style
	}
	return PlaceOnMask(style, srcW, srcH, dstW, dstH, mask)
}

// PlaceOnMask sets alignment and MarginV so the cue sits inside the mask
// on the output frame. The mask is drawn on the source and then scaled and
// center-cropped, so the band is mapped through that same transform.
func PlaceOnMask(style Style, srcW, srcH, dstW, dstH int, mask media.SubtitleMask) Style {
	if dstW < 2 || dstH < 2 {
		dstW, dstH = srcW, srcH
	}
	if dstW < 2 {
		dstW = 1080
	}
	if dstH < 2 {
		dstH = 1920
	}
	region := media.OutputMaskRect(srcW, srcH, dstW, dstH, mask)
	font := style.FontSize
	if font <= 0 {
		font = DefaultStyle().FontSize
	}
	center := region.Y + region.H/2
	if center < dstH/3 {
		style.Alignment = 8
		margin := center - font/2
		if margin < 0 {
			margin = 0
		}
		style.MarginV = margin
		return style
	}
	style.Alignment = 2
	margin := dstH - center - font/2
	if margin < 0 {
		margin = 0
	}
	style.MarginV = margin
	return style
}
