package hookopen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
)

// Transition names accepted by the UI.
var Transitions = []struct {
	Label string
	Name  string
}{
	{Label: "淡入淡出", Name: "fade"},
	{Label: "闪黑", Name: "fadeblack"},
	{Label: "闪白", Name: "fadewhite"},
	{Label: "左滑", Name: "slideleft"},
}

func NormalizeTransition(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "fade", "fadeblack", "fadewhite", "slideleft", "slideright", "circleclose", "wipeleft":
		return name
	case "淡入淡出":
		return "fade"
	case "闪黑":
		return "fadeblack"
	case "闪白":
		return "fadewhite"
	case "左滑":
		return "slideleft"
	default:
		return "fade"
	}
}

// DefaultOutputPath returns {stem}_爆点开头.mp4 beside the source video.
func DefaultOutputPath(videoPath string) string {
	return defaultOutputPath(videoPath)
}

// Render prepends the chosen hook to the full original video with a transition.
func Render(ctx context.Context, cfg *config.Config, videoPath string, hook Candidate, transition string, xfadeSec float64, outPath string, progress Progress) error {
	if progress == nil {
		progress = func(int, string) {}
	}
	if hook.EndSec-hook.StartSec < MinHookSec-0.2 {
		return fmt.Errorf("片头片段太短")
	}
	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		outPath = defaultOutputPath(videoPath)
	}
	if !strings.EqualFold(filepath.Ext(outPath), ".mp4") {
		outPath += ".mp4"
	}
	bins := resolveBins(cfg)
	if err := ffmpegExists(bins); err != nil {
		return err
	}
	info, err := probeVideo(ctx, bins, videoPath)
	if err != nil {
		return err
	}
	if xfadeSec <= 0 {
		xfadeSec = DefaultXFadeSec
	}
	hookDur := hook.EndSec - hook.StartSec
	if xfadeSec >= hookDur-0.2 {
		xfadeSec = hookDur / 3
		if xfadeSec < 0.2 {
			xfadeSec = 0.2
		}
	}
	trans := NormalizeTransition(transition)
	workDir := workDirFor(videoPath)
	_ = os.MkdirAll(workDir, 0o755)

	progress(5, "正在裁出片头片段…")
	hookPath := filepath.Join(workDir, "hook_clip.mp4")
	if err := cutPreview(ctx, bins, videoPath, hook.StartSec, hook.EndSec, hookPath); err != nil {
		return fmt.Errorf("裁切片头失败: %w", err)
	}

	progress(20, "正在加转场并与原片合并…")
	offset := hookDur - xfadeSec
	filter := buildXfadeFilter(info, hookDur, xfadeSec, offset, trans)

	args := []string{
		"-y",
		"-i", hookPath,
		"-i", videoPath,
		"-filter_complex", filter,
		"-map", "[v]",
	}
	if strings.Contains(filter, "[a]") {
		args = append(args, "-map", "[a]")
	}
	args = append(args,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
		"-pix_fmt", "yuv420p", "-profile:v", "high",
		"-c:a", "aac", "-b:a", "160k",
		"-movflags", "+faststart",
		outPath,
	)
	log, err := media.Run(ctx, bins.FFmpeg, args...)
	if err != nil && ctx.Err() == nil {
		// retry video-only if audio mix failed (e.g. hook clip has no soundtrack)
		progress(-1, "音轨合成不顺利，改为只保留画面转场…")
		vfilter := videoXfadeFilter(info, xfadeSec, offset, trans)
		args = []string{
			"-y",
			"-i", hookPath,
			"-i", videoPath,
			"-filter_complex", vfilter,
			"-map", "[v]",
			"-an",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
			"-pix_fmt", "yuv420p", "-profile:v", "high",
			"-movflags", "+faststart",
			outPath,
		}
		log, err = media.Run(ctx, bins.FFmpeg, args...)
	}
	if err != nil {
		if ctx.Err() != nil {
			_ = os.Remove(outPath)
			return ctx.Err()
		}
		return fmt.Errorf("合成失败: %s", lastLines(log, 8))
	}
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		return fmt.Errorf("合成完成但没有生成文件")
	}
	progress(100, "已保存 "+outPath)
	return nil
}

func buildXfadeFilter(info videoInfo, hookDur, xfade, offset float64, trans string) string {
	w, h := info.Width, info.Height
	if w < 16 {
		w = 1080
	}
	if h < 16 {
		h = 1920
	}
	w, h = even(w), even(h)
	fps := info.FPS
	if fps == "" {
		fps = "30"
	}
	v := fmt.Sprintf(
		"[0:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,format=yuv420p,settb=AVTB,setpts=PTS-STARTPTS[v0];"+
			"[1:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,format=yuv420p,settb=AVTB,setpts=PTS-STARTPTS[v1];"+
			"[v0][v1]xfade=transition=%s:duration=%.3f:offset=%.3f,format=yuv420p[v]",
		w, h, w, h, fps,
		w, h, w, h, fps,
		trans, xfade, offset,
	)
	// Always produce an audio mix so the output has a soundtrack when possible.
	// Silent sources are replaced with anullsrc of matching length.
	total := hookDur + info.Duration - xfade
	if total < hookDur {
		total = hookDur + info.Duration
	}
	a := fmt.Sprintf(
		";anullsrc=r=44100:cl=stereo,atrim=0:%.3f,asetpts=PTS-STARTPTS[sil]",
		total+1,
	)
	if info.HasAudio {
		a += fmt.Sprintf(
			";[0:a]aformat=sample_fmts=fltp:sample_rates=44100:channel_layouts=stereo,aresample=async=1,asetpts=PTS-STARTPTS[a0raw];"+
				"[1:a]aformat=sample_fmts=fltp:sample_rates=44100:channel_layouts=stereo,aresample=async=1,asetpts=PTS-STARTPTS[a1];"+
				"[a0raw][sil]amix=inputs=2:duration=first:dropout_transition=0,atrim=0:%.3f,asetpts=PTS-STARTPTS[a0];"+
				"[a0][a1]acrossfade=d=%.3f:c1=tri:c2=tri[a]",
			hookDur, xfade,
		)
	} else {
		// keep hook audio if the hook cut has it; otherwise silent
		a += fmt.Sprintf(
			";[0:a]aformat=sample_fmts=fltp:sample_rates=44100:channel_layouts=stereo,aresample=async=1,asetpts=PTS-STARTPTS[a0raw];"+
				"[a0raw][sil]amix=inputs=2:duration=first:dropout_transition=0,atrim=0:%.3f,asetpts=PTS-STARTPTS[a0];"+
				"[sil]atrim=0:%.3f,asetpts=PTS-STARTPTS[a1];"+
				"[a0][a1]acrossfade=d=%.3f:c1=tri:c2=tri[a]",
			hookDur, info.Duration, xfade,
		)
	}
	return v + a
}

func videoXfadeFilter(info videoInfo, xfade, offset float64, trans string) string {
	w, h := info.Width, info.Height
	if w < 16 {
		w = 1080
	}
	if h < 16 {
		h = 1920
	}
	w, h = even(w), even(h)
	fps := info.FPS
	if fps == "" {
		fps = "30"
	}
	return fmt.Sprintf(
		"[0:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,format=yuv420p,settb=AVTB,setpts=PTS-STARTPTS[v0];"+
			"[1:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,format=yuv420p,settb=AVTB,setpts=PTS-STARTPTS[v1];"+
			"[v0][v1]xfade=transition=%s:duration=%.3f:offset=%.3f,format=yuv420p[v]",
		w, h, w, h, fps,
		w, h, w, h, fps,
		trans, xfade, offset,
	)
}

func lastLines(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "ffmpeg 失败"
	}
	lines := strings.Split(s, "\n")
	var kept []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return strings.Join(kept, " / ")
}
