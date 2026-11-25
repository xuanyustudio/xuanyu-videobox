package media

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MaskEnableExpr builds an ffmpeg enable expression that hides overlay during disableRanges.
// Returns empty string when there are no ranges.
func MaskEnableExpr(disableRanges [][2]float64) string {
	if len(disableRanges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(disableRanges))
	for _, r := range disableRanges {
		parts = append(parts, fmt.Sprintf("between(t,%s,%s)",
			formatFFmpegFloat(r[0]), formatFFmpegFloat(r[1])))
	}
	return "not(" + strings.Join(parts, "+") + ")"
}

func formatFFmpegFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

// SubtitleMask covers burned-in source subtitles with a blurred, darkened bar.
// Percent fields of 0 mean "use orientation defaults".
type SubtitleMask struct {
	Enabled bool
	X       float64 // left, 0–100
	Y       float64 // top, 0–100
	Width   float64 // 0–100
	Height  float64 // 0–100
	Blur    int     // boxblur radius, 0–200; 0 uses default
	Opacity int     // 0–100; 0 uses default
}

// MaskRegion is a resolved pixel region plus filter parameters.
type MaskRegion struct {
	X, Y, W, H int
	Blur       int
	Opacity    float64 // 0–1 dark overlay
	Portrait   bool
}

var defaultMaskLandscape = SubtitleMask{X: 10, Y: 78, Width: 80, Height: 14, Blur: 18, Opacity: 82}
var defaultMaskPortrait = SubtitleMask{X: 8, Y: 79, Width: 84, Height: 16, Blur: 26, Opacity: 84}

// ResolveSubtitleMask computes a pixel region for a frame of size videoW x videoH.
func ResolveSubtitleMask(videoW, videoH int, m SubtitleMask) MaskRegion {
	if videoW < 2 {
		videoW = 2
	}
	if videoH < 2 {
		videoH = 2
	}
	portrait := videoH > videoW
	def := defaultMaskLandscape
	if portrait {
		def = defaultMaskPortrait
	}
	xPct := pickPct(m.X, def.X, 0, 99)
	yPct := pickPct(m.Y, def.Y, 0, 99)
	wPct := pickPct(m.Width, def.Width, 2, 100-xPct)
	hPct := pickPct(m.Height, def.Height, 2, 100-yPct)
	blur := m.Blur
	if blur <= 0 {
		blur = def.Blur
	}
	if blur > 200 {
		blur = 200
	}
	opacity := m.Opacity
	if opacity <= 0 {
		opacity = def.Opacity
	}
	if opacity > 100 {
		opacity = 100
	}
	// Scale blur slightly with frame height, matching NarratoAI's approach.
	baseH := 1080.0
	if portrait {
		baseH = 1920
	}
	scaledBlur := int(math.Round(float64(blur) * float64(videoH) / baseH))
	if blur > 0 && scaledBlur < 1 {
		scaledBlur = 1
	}

	x := clampInt(int(math.Round(float64(videoW)*xPct/100)), 0, max(0, videoW-2))
	y := clampInt(int(math.Round(float64(videoH)*yPct/100)), 0, max(0, videoH-2))
	w := clampInt(int(math.Round(float64(videoW)*wPct/100)), 2, max(2, videoW-x))
	h := clampInt(int(math.Round(float64(videoH)*hPct/100)), 2, max(2, videoH-y))
	w, h = even(w), even(h)
	if x+w > videoW {
		x = max(0, videoW-w)
	}
	if y+h > videoH {
		y = max(0, videoH-h)
	}
	x, y = even(x), even(y)
	return MaskRegion{
		X: x, Y: y, W: w, H: h,
		Blur:     scaledBlur,
		Opacity:  float64(opacity) / 100,
		Portrait: portrait,
	}
}

// OutputMaskRect maps the mask onto the final frame. The layout draws the
// mask on the source, then scales to cover dst and center-crops. A zero
// destination size means the frame was not resized.
func OutputMaskRect(srcW, srcH, dstW, dstH int, mask SubtitleMask) MaskRegion {
	mask.Enabled = true
	if dstW < 2 || dstH < 2 {
		dstW, dstH = srcW, srcH
	}
	if srcW < 2 || srcH < 2 {
		srcW, srcH = dstW, dstH
	}
	region := ResolveSubtitleMask(srcW, srcH, mask)
	if srcW == dstW && srcH == dstH {
		return region
	}
	scaleW := float64(dstW) / float64(srcW)
	scaleH := float64(dstH) / float64(srcH)
	scale := scaleW
	if scaleH > scaleW {
		scale = scaleH
	}
	cropX := (float64(srcW)*scale - float64(dstW)) / 2
	cropY := (float64(srcH)*scale - float64(dstH)) / 2
	x := int(math.Round(float64(region.X)*scale - cropX))
	y := int(math.Round(float64(region.Y)*scale - cropY))
	w := int(math.Round(float64(region.W) * scale))
	h := int(math.Round(float64(region.H) * scale))
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > dstW {
		w = dstW - x
	}
	if y+h > dstH {
		h = dstH - y
	}
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	region.X, region.Y, region.W, region.H = x, y, w, h
	return region
}

// SubtitleMaskFilter builds an ffmpeg video filter that blurs the region
// and darkens it so burned-in source subtitles are covered.
func SubtitleMaskFilter(videoW, videoH int, m SubtitleMask) string {
	if !m.Enabled {
		return ""
	}
	r := ResolveSubtitleMask(videoW, videoH, m)
	return r.Filter()
}

// Filter is the ffmpeg vf fragment for this region (expects a single video input).
func (r MaskRegion) Filter() string {
	if r.W < 2 || r.H < 2 {
		return ""
	}
	blur := r.Blur
	if blur < 1 {
		blur = 1
	}
	alpha := formatFFmpegFloat(r.Opacity)
	// Crop the bar, blur it, overlay it back, then add a translucent black box.
	return fmt.Sprintf(
		"split[base][tmp];[tmp]crop=%d:%d:%d:%d,boxblur=%d:1[blur];[base][blur]overlay=%d:%d[m0];[m0]drawbox=x=%d:y=%d:w=%d:h=%d:color=black@%s:t=fill",
		r.W, r.H, r.X, r.Y, blur, r.X, r.Y, r.X, r.Y, r.W, r.H, alpha,
	)
}

func pickPct(v, fallback, lo, hi float64) float64 {
	if v <= 0 {
		v = fallback
	}
	return clampFloat(v, lo, hi)
}

func clampFloat(v, lo, hi float64) float64 {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
