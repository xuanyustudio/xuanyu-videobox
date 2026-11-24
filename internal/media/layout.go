package media

import (
	"context"
	"fmt"
	"strings"
)

// VideoLayout describes optional mask + scale/crop applied to a clip or frame.
type VideoLayout struct {
	Mask   SubtitleMask
	Width  int
	Height int
}

// ApplyVideoLayout re-encodes inPath with the mask and/or target size written to outPath.
func ApplyVideoLayout(ctx context.Context, bins FFmpegBins, inPath, outPath string, layout VideoLayout, src VideoSize) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	if ok, err := HasVideoStream(ctx, bins, inPath); err == nil && !ok {
		return fmt.Errorf("片段没有画面，无法调整尺寸或遮罩（可能是时间范围已超出片源时长）")
	}
	vf := LayoutFilter(layout, src)
	if vf == "" {
		return fmt.Errorf("empty layout filter")
	}
	args := []string{"-y", "-i", inPath, "-map", "0:v:0", "-vf", vf}
	args = append(args, defaultVideoEncodeArgs()...)
	args = append(args, "-map", "0:a?", "-c:a", "copy", "-dn")
	args = append(args, "-movflags", "+faststart", outPath)
	out, err := Run(ctx, bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// LayoutFilter joins mask and scale/crop filters. Mask is applied first
// (covering source subtitles), then the frame is fit to the target size.
func LayoutFilter(layout VideoLayout, src VideoSize) string {
	var parts []string
	if layout.Mask.Enabled && src.Width > 0 && src.Height > 0 {
		if f := SubtitleMaskFilter(src.Width, src.Height, layout.Mask); f != "" {
			parts = append(parts, f)
		}
	}
	if layout.Width >= 2 && layout.Height >= 2 {
		parts = append(parts, ScaleCropFilter(layout.Width, layout.Height))
	}
	return strings.Join(parts, ",")
}

// PreviewSubtitleMask writes a still frame with the mask applied, for UI preview.
func PreviewSubtitleMask(ctx context.Context, bins FFmpegBins, videoPath, outImage string, mask SubtitleMask, atSec float64) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	size, err := ProbeVideoSize(ctx, bins, videoPath)
	if err != nil {
		return err
	}
	mask.Enabled = true
	vf := SubtitleMaskFilter(size.Width, size.Height, mask)
	if vf == "" {
		return fmt.Errorf("无法构建遮罩滤镜")
	}
	if atSec < 0 {
		atSec = 0
	}
	args := []string{
		"-y",
		"-ss", formatMixDuration(atSec),
		"-i", videoPath,
		"-vf", vf,
		"-frames:v", "1",
		outImage,
	}
	out, err := Run(ctx, bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}
