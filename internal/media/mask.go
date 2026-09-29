package media

import (
	"fmt"
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
