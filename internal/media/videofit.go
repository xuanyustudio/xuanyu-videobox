package media

import (
	"fmt"
	"strconv"
	"strings"
)

// Aspect ratio choices for the synthesis pipeline.
const (
	AspectSource     = "source"
	AspectVertical   = "vertical"
	AspectHorizontal = "horizontal"
)

// NormalizeAspect maps a stored aspect value to source, vertical, or horizontal.
func NormalizeAspect(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "vertical", "portrait", "9:16":
		return AspectVertical
	case "horizontal", "landscape", "16:9":
		return AspectHorizontal
	default:
		return AspectSource
	}
}

// ParseResolutionHeight maps a quality label such as "1080p" to a pixel height.
// Unknown or empty values default to 1080.
func ParseResolutionHeight(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, "p")
	switch s {
	case "480":
		return 480
	case "720":
		return 720
	case "1080":
		return 1080
	case "1440", "2k":
		return 1440
	case "2160", "4k":
		return 2160
	default:
		return 1080
	}
}

func even(v int) int {
	if v < 2 {
		return 2
	}
	return v - v%2
}

// TargetSize computes the output width and height for a source frame.
func TargetSize(srcW, srcH int, aspect, resolution string) (w, h int) {
	if srcW < 2 {
		srcW = 2
	}
	if srcH < 2 {
		srcH = 2
	}
	short := ParseResolutionHeight(resolution)
	switch NormalizeAspect(aspect) {
	case AspectVertical:
		return even(short), even(short * 16 / 9)
	case AspectHorizontal:
		return even(short * 16 / 9), even(short)
	default:
		// Keep source aspect; the "p" value is the shorter side.
		if srcH >= srcW {
			w = even(short)
			h = even(int(float64(short) * float64(srcH) / float64(srcW)))
			return w, h
		}
		h = even(short)
		w = even(int(float64(short) * float64(srcW) / float64(srcH)))
		return w, h
	}
}

// ScaleCropFilter builds an ffmpeg vf that scales to cover dstWxdstH and center-crops.
func ScaleCropFilter(dstW, dstH int) string {
	dstW, dstH = even(dstW), even(dstH)
	return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1", dstW, dstH, dstW, dstH)
}

// FormatResolutionLabel is a canonical "1080p"-style label.
func FormatResolutionLabel(h int) string {
	return strconv.Itoa(h) + "p"
}
