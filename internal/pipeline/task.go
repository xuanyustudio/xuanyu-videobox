package pipeline

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/sonilo"
	"github.com/narratoai/narrato-go/internal/subtitleburn"
	"github.com/narratoai/narrato-go/internal/tts"
)

// Params configures a full render from script items through ffmpeg.
type Params struct {
	Items          []script.Item
	VideoPaths     []string // 1-based video_id maps to index video_id-1
	WorkDir        string
	OutputPath     string // final mp4 path; empty → workDir/final.mp4
	TTS            tts.Synthesizer
	Voice          string
	Bins           media.FFmpegBins // empty paths → Resolve inside Run
	HideOnOriginal *bool            // nil → true (hide OST=1 narration in SRT)
	OnProgress     func(step string, percent int, msg string)

	OriginalVolume float64
	VoiceVolume    float64
	BGMPath        string
	BGMVolume      float64
	SFXPath        string
	SFXVolume      float64

	BurnSubtitles    *bool // nil → true
	SubtitleStyle    subtitleburn.Style
	SubtitlePosition string
	SubtitleOnMask   bool // place burned narration on the mask; default keeps SubtitlePosition

	Mask       media.SubtitleMask
	Aspect     string // source | vertical | horizontal; empty = keep source size
	Resolution string // 480p–2160p; ignored when Aspect is empty and Resolution is empty

	// Optional Sonilo client. Failures are logged and skipped; they never abort the render.
	Sonilo          *sonilo.Client
	SoniloBGM       bool
	SoniloBGMPrompt string
	SoniloSFX       bool
	SoniloSFXPrompt string
}

// Run executes clip → concat → mix TTS → burn subtitles.
func Run(ctx context.Context, p Params) (outputMP4 string, err error) {
	if len(p.Items) == 0 {
		return "", fmt.Errorf("items: at least one script row required")
	}
	if p.WorkDir == "" {
		return "", fmt.Errorf("workDir required")
	}
	if p.TTS == nil {
		return "", fmt.Errorf("TTS synthesizer required")
	}
	if len(p.VideoPaths) == 0 {
		return "", fmt.Errorf("videoPaths: at least one source video required")
	}

	bins, err := resolveBins(p.Bins)
	if err != nil {
		return "", err
	}

	outPath := p.OutputPath
	if outPath == "" {
		outPath = filepath.Join(p.WorkDir, "final.mp4")
	}

	progress := p.OnProgress
	report := func(step string, percent int, msg string) {
		if progress != nil {
			progress(step, percent, msg)
		}
	}

	if err := os.MkdirAll(filepath.Join(p.WorkDir, "tts"), 0o755); err != nil {
		return "", fmt.Errorf("mkdir tts: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(p.WorkDir, "clips"), 0o755); err != nil {
		return "", fmt.Errorf("mkdir clips: %w", err)
	}
	if p.OriginalVolume == 0 && p.VoiceVolume == 0 {
		p.OriginalVolume = 1.2
		p.VoiceVolume = 1
	}

	workItems := append([]script.Item(nil), p.Items...)
	ttsDurByID := make(map[int]float64)
	ttsPathByID := make(map[int]string)

	report("tts", 5, "synthesizing narration")
	for i := range workItems {
		item := &workItems[i]
		if item.OST != 0 && item.OST != 2 {
			continue
		}
		narration := strings.TrimSpace(item.Narration)
		if narration == "" {
			continue
		}
		id := item.ID
		if id == 0 {
			id = i + 1
		}
		ttsPath := filepath.Join(p.WorkDir, "tts", fmt.Sprintf("%d.mp3", id))
		dur, err := p.TTS.Synthesize(ctx, narration, p.Voice, ttsPath)
		if err != nil {
			return "", fmt.Errorf("tts item %d: %w", id, err)
		}
		if actual, aerr := probeDurationSec(ctx, bins, ttsPath); aerr == nil {
			if actual < 0.2 {
				return "", fmt.Errorf("tts item %d: audio too short (%.3fs) — Edge TTS 可能失败或被静音占位", id, actual)
			}
			if actual > dur {
				dur = actual
			}
		}
		paddedPath := filepath.Join(p.WorkDir, "tts", fmt.Sprintf("%d_padded.wav", id))
		if err := padAudioToDuration(ctx, bins, ttsPath, dur, paddedPath); err != nil {
			return "", fmt.Errorf("pad tts item %d: %w", id, err)
		}
		ttsDurByID[id] = dur
		ttsPathByID[id] = paddedPath
	}

	layout, layoutNote, err := resolveLayout(ctx, bins, p)
	if err != nil {
		return "", err
	}
	if layoutNote != "" {
		report("layout", 20, layoutNote)
	}

	report("clip", 25, "cutting segments")
	sourceTimeline, err := media.ProbeSourceTimeline(ctx, bins, p.VideoPaths)
	if err != nil {
		return "", err
	}
	if err := validateClipRanges(ctx, bins, workItems, p.VideoPaths, ttsDurByID, sourceTimeline); err != nil {
		return "", err
	}
	clipPaths := make([]string, 0, len(workItems))
	var timeline float64
	for i := range workItems {
		item := &workItems[i]
		ts := strings.TrimSpace(item.Timestamp)
		if ts == "" {
			return "", fmt.Errorf("第 %d 条脚本缺少时间范围", i+1)
		}
		seg, localTs, err := script.LocalizeItem(*item, sourceTimeline)
		if err != nil {
			return "", fmt.Errorf("第 %d 条脚本无法匹配片源: %w", i+1, err)
		}
		videoPath := seg.Path
		ts = localTs

		id := item.ID
		if id == 0 {
			id = i + 1
		}
		ttsDur := ttsDurByID[id]
		isHook := item.HookSourceID != nil

		clipPath := filepath.Join(p.WorkDir, "clips", fmt.Sprintf("%d.mp4", i))
		if err := media.ClipSegment(ctx, bins, videoPath, ts, item.OST, ttsDur, isHook, clipPath); err != nil {
			return "", fmt.Errorf("裁剪第 %d 条脚本失败（片源「%s」）: %w", i+1, seg.Name, err)
		}
		if media.LayoutFilter(layout, media.VideoSize{}) != "" || layout.Mask.Enabled || (layout.Width >= 2 && layout.Height >= 2) {
			srcSize, serr := media.ProbeVideoSize(ctx, bins, clipPath)
			if serr != nil {
				srcSize, _ = media.ProbeVideoSize(ctx, bins, videoPath)
			}
			fitted := filepath.Join(p.WorkDir, "clips", fmt.Sprintf("%d_fit.mp4", i))
			if ferr := media.ApplyVideoLayout(ctx, bins, clipPath, fitted, layout, srcSize); ferr != nil {
				return "", fmt.Errorf("layout clip %d: %w", i, ferr)
			}
			clipPath = fitted
		}
		clipPaths = append(clipPaths, clipPath)

		clipDur, err := probeDurationSec(ctx, bins, clipPath)
		if err != nil {
			return "", fmt.Errorf("probe clip %d: %w", i, err)
		}
		start := timeline
		end := timeline + clipDur
		item.EditedTimeRange = formatEditedTimeRange(start, end)
		d := clipDur
		item.Duration = &d
		timeline = end
	}

	hideOriginal := hideOnOriginal(p.HideOnOriginal)
	if p.Mask.Enabled {
		report("mask", 40, "已对画面应用原字幕遮罩")
	}

	report("subtitle", 45, "building SRT")
	srtBody, err := media.BuildScriptSRT(workItems, media.SubtitleOptions{
		HideOnOriginal: hideOriginal,
	})
	if err != nil {
		return "", fmt.Errorf("build srt: %w", err)
	}
	srtPath := filepath.Join(p.WorkDir, "script.srt")
	if srtBody != "" {
		if err := os.WriteFile(srtPath, []byte(srtBody), 0o644); err != nil {
			return "", fmt.Errorf("write srt: %w", err)
		}
	}

	concatPath := filepath.Join(p.WorkDir, "concat.mp4")
	report("concat", 55, "joining clips")
	if err := media.ConcatVideos(ctx, bins, clipPaths, concatPath); err != nil {
		return "", fmt.Errorf("concat: %w", err)
	}

	narratedPath := filepath.Join(p.WorkDir, "narrated.mp4")
	report("mix", 70, "mixing TTS")
	timed := buildTimedTTS(workItems, ttsPathByID)
	if len(timed) == 0 {
		if err := copyFile(concatPath, narratedPath); err != nil {
			return "", fmt.Errorf("copy concat to narrated: %w", err)
		}
	} else {
		if err := media.MixAudioOntoVideo(ctx, bins, concatPath, timed, media.MixOptions{
			OriginalVolume: volumeOr(p.OriginalVolume, 1.2),
			VoiceVolume:    volumeOr(p.VoiceVolume, 1.0),
		}, narratedPath); err != nil {
			return "", fmt.Errorf("mix tts: %w", err)
		}
	}

	bgmPath := strings.TrimSpace(p.BGMPath)
	sfxPath := strings.TrimSpace(p.SFXPath)
	if p.SoniloBGM || p.SoniloSFX {
		if p.Sonilo == nil || strings.TrimSpace(p.Sonilo.APIKey) == "" {
			report("sonilo", 75, "未配置 Sonilo API Key，已跳过 AI 配乐/音效")
		} else {
			dur, _ := probeDurationSec(ctx, bins, narratedPath)
			if p.SoniloBGM && bgmPath == "" {
				if dur > sonilo.MaxBGMVideoDurationSec {
					report("bgm", 76, fmt.Sprintf("成片约 %.0f 秒，超过 Sonilo 配乐上限 %d 秒，已跳过", dur, sonilo.MaxBGMVideoDurationSec))
				} else {
					report("bgm", 76, "正在用 Sonilo 生成配乐")
					dest := filepath.Join(p.WorkDir, "sonilo_bgm.m4a")
					if err := p.Sonilo.GenerateBGM(ctx, narratedPath, p.SoniloBGMPrompt, dest); err != nil {
						report("bgm", 76, "Sonilo 配乐失败，已跳过: "+err.Error())
					} else {
						bgmPath = dest
					}
				}
			}
			if p.SoniloSFX && sfxPath == "" {
				if dur > sonilo.MaxSFXVideoDurationSec {
					report("sfx", 78, fmt.Sprintf("成片约 %.0f 秒，超过 Sonilo 音效上限 %d 秒，已跳过", dur, sonilo.MaxSFXVideoDurationSec))
				} else {
					report("sfx", 78, "正在用 Sonilo 生成音效")
					dest := filepath.Join(p.WorkDir, "sonilo_sfx.m4a")
					if err := p.Sonilo.GenerateSFX(ctx, narratedPath, p.SoniloSFXPrompt, dest); err != nil {
						report("sfx", 78, "Sonilo 音效失败，已跳过: "+err.Error())
					} else {
						sfxPath = dest
					}
				}
			}
		}
	}

	mixedPath := filepath.Join(p.WorkDir, "mixed.mp4")
	if bgmPath == "" && sfxPath == "" {
		if err := copyFile(narratedPath, mixedPath); err != nil {
			return "", fmt.Errorf("copy narrated to mixed: %w", err)
		}
	} else {
		report("mix", 80, "混入背景音乐/音效")
		if err := media.MixAudioOntoVideo(ctx, bins, narratedPath, nil, media.MixOptions{
			OriginalVolume: 1,
			BGMPath:        bgmPath,
			BGMVolume:      volumeOr(p.BGMVolume, 0.3),
			SFXPath:        sfxPath,
			SFXVolume:      volumeOr(p.SFXVolume, 0.6),
		}, mixedPath); err != nil {
			return "", fmt.Errorf("mix bgm/sfx: %w", err)
		}
	}

	burn := true
	if p.BurnSubtitles != nil {
		burn = *p.BurnSubtitles
	}
	report("burn", 85, "burning subtitles")
	if srtBody == "" || !burn {
		if !burn && srtBody != "" {
			report("burn", 90, "已关闭烧录，字幕文件: "+srtPath)
		}
		if err := copyFile(mixedPath, outPath); err != nil {
			return "", fmt.Errorf("copy output: %w", err)
		}
	} else {
		style := burnStyle(ctx, bins, p, layout, mixedPath)
		force := subtitleburn.ForceStyle(style)
		if err := media.BurnSubtitlesWithForceStyle(ctx, bins, mixedPath, srtPath, outPath, force); err != nil {
			return "", fmt.Errorf("burn subtitles: %w", err)
		}
	}

	report("done", 100, outPath)
	return outPath, nil
}

func hideOnOriginal(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}


func validateClipRanges(ctx context.Context, bins media.FFmpegBins, items []script.Item, videoPaths []string, ttsDurByID map[int]float64, sourceTimeline []script.SourceSegment) error {
	var lines []string
	for i := range items {
		item := items[i]
		ts := strings.TrimSpace(item.Timestamp)
		if ts == "" {
			return fmt.Errorf("第 %d 条脚本缺少时间范围", i+1)
		}
		seg, localTs, err := script.LocalizeItem(item, sourceTimeline)
		if err != nil {
			lines = append(lines, fmt.Sprintf("第 %d 条  %s  %s", i+1, ts, err.Error()))
			continue
		}
		id := item.ID
		if id == 0 {
			id = i + 1
		}
		startMs, endMs, err := media.PlanClipRange(localTs, item.OST, ttsDurByID[id], item.HookSourceID != nil)
		if err != nil {
			return fmt.Errorf("第 %d 条脚本时间范围无效: %w", i+1, err)
		}
		if _, _, ferr := media.FitClipRangeToSource(startMs, endMs, seg.DurationMs); ferr != nil {
			lines = append(lines, fmt.Sprintf("第 %d 条  %s  对应片源「%s」（时长 %s）", i+1, ts, seg.Name, media.FormatScriptClock(seg.DurationMs)))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("有 %d 条脚本的时间范围超出对应片源时长，无法裁剪：\n%s\n请调整这些段落的时间，或检查素材是否完整。", len(lines), strings.Join(lines, "\n"))
}

func resolveBins(bins media.FFmpegBins) (media.FFmpegBins, error) {
	out := bins
	var err error
	if out.FFmpeg == "" {
		out.FFmpeg, err = media.ResolveFFmpeg("", "")
		if err != nil {
			return out, fmt.Errorf("resolve ffmpeg: %w", err)
		}
	}
	if out.FFprobe == "" {
		out.FFprobe, err = media.ResolveFFprobe("", "")
		if err != nil {
			return out, fmt.Errorf("resolve ffprobe: %w", err)
		}
	}
	return out, nil
}

func resolveVideoPath(videoID int, paths []string) (string, error) {
	if videoID <= 0 {
		videoID = 1
	}
	idx := videoID - 1
	if idx < 0 || idx >= len(paths) {
		return "", fmt.Errorf("video_id %d out of range (have %d videos)", videoID, len(paths))
	}
	p := strings.TrimSpace(paths[idx])
	if p == "" {
		return "", fmt.Errorf("video_id %d path is empty", videoID)
	}
	return p, nil
}

func formatEditedTimeRange(startSec, endSec float64) string {
	return formatClockSec(startSec) + "-" + formatClockSec(endSec)
}

func formatClockSec(sec float64) string {
	s := int(math.Round(sec))
	if s < 0 {
		s = 0
	}
	h := s / 3600
	s %= 3600
	m := s / 60
	s %= 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func probeDurationSec(ctx context.Context, bins media.FFmpegBins, path string) (float64, error) {
	if bins.FFprobe == "" {
		return 0, fmt.Errorf("ffprobe required")
	}
	out, err := media.Run(ctx, bins.FFprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	if err != nil {
		return 0, err
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return d, nil
}

func buildTimedTTS(items []script.Item, ttsPathByID map[int]string) []media.TimedAudio {
	var clips []media.TimedAudio
	for i, item := range items {
		if item.OST != 0 && item.OST != 2 {
			continue
		}
		id := item.ID
		if id == 0 {
			id = i + 1
		}
		path, ok := ttsPathByID[id]
		if !ok || path == "" {
			continue
		}
		start, ok := editedRangeStartSec(item.EditedTimeRange)
		if !ok {
			continue
		}
		clips = append(clips, media.TimedAudio{Path: path, StartSec: start})
	}
	return clips
}

func editedRangeStartSec(edited string) (float64, bool) {
	edited = strings.TrimSpace(edited)
	if edited == "" {
		return 0, false
	}
	parts := strings.SplitN(edited, "-", 2)
	if len(parts) != 2 {
		return 0, false
	}
	sec, ok := clockToSec(parts[0])
	return sec, ok
}

func clockToSec(clock string) (float64, bool) {
	clock = strings.TrimSpace(clock)
	seg := strings.Split(clock, ":")
	if len(seg) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(seg[0])
	m, err2 := strconv.Atoi(seg[1])
	s, err3 := strconv.Atoi(seg[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return float64(h*3600 + m*60 + s), true
}

func ost1EditedRanges(items []script.Item) [][2]float64 {
	var ranges [][2]float64
	for _, item := range items {
		if item.OST != 1 {
			continue
		}
		start, ok := editedRangeStartSec(item.EditedTimeRange)
		if !ok {
			continue
		}
		end := start
		if item.Duration != nil && *item.Duration > 0 {
			end = start + *item.Duration
		} else if d, ok := editedRangeEndSec(item.EditedTimeRange); ok {
			end = d
		}
		if end > start {
			ranges = append(ranges, [2]float64{start, end})
		}
	}
	return ranges
}

func editedRangeEndSec(edited string) (float64, bool) {
	parts := strings.SplitN(strings.TrimSpace(edited), "-", 2)
	if len(parts) != 2 {
		return 0, false
	}
	return clockToSec(parts[1])
}

func padAudioToDuration(ctx context.Context, bins media.FFmpegBins, inPath string, durSec float64, outPath string) error {
	if durSec <= 0 {
		return copyFile(inPath, outPath)
	}
	durStr := formatDurationSec(durSec)
	args := []string{
		"-y", "-i", inPath,
		"-af", "apad",
		"-t", durStr,
		"-c:a", "pcm_s16le",
		outPath,
	}
	_, err := media.Run(ctx, bins.FFmpeg, args...)
	return err
}

func formatDurationSec(sec float64) string {
	s := strconv.FormatFloat(sec, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func volumeOr(v, fallback float64) float64 {
	if v != v || v == 0 {
		return fallback
	}
	if v < 0 {
		return 0
	}
	if v > 2 {
		return 2
	}
	return v
}

func burnStyle(ctx context.Context, bins media.FFmpegBins, p Params, layout media.VideoLayout, mixedPath string) subtitleburn.Style {
	dstW, dstH := layout.Width, layout.Height
	if dstW < 2 || dstH < 2 {
		if sz, err := media.ProbeVideoSize(ctx, bins, mixedPath); err == nil {
			dstW, dstH = sz.Width, sz.Height
		}
	}
	srcW, srcH := dstW, dstH
	if len(p.VideoPaths) > 0 {
		if sz, err := media.ProbeVideoSize(ctx, bins, p.VideoPaths[0]); err == nil && sz.Width > 0 && sz.Height > 0 {
			srcW, srcH = sz.Width, sz.Height
		}
	}
	return subtitleburn.BurnStyle(p.SubtitleStyle, p.SubtitlePosition, p.SubtitleOnMask, srcW, srcH, dstW, dstH, p.Mask)
}

func resolveLayout(ctx context.Context, bins media.FFmpegBins, p Params) (media.VideoLayout, string, error) {
	layout := media.VideoLayout{Mask: p.Mask}
	aspect := strings.TrimSpace(p.Aspect)
	res := strings.TrimSpace(p.Resolution)
	if aspect == "" && res == "" && !p.Mask.Enabled {
		return layout, "", nil
	}
	var src media.VideoSize
	if len(p.VideoPaths) > 0 {
		if sz, err := media.ProbeVideoSize(ctx, bins, p.VideoPaths[0]); err == nil {
			src = sz
		}
	}
	if aspect != "" || res != "" {
		if src.Width == 0 || src.Height == 0 {
			// Fall back to a vertical 1080p canvas if we cannot probe the source.
			src = media.VideoSize{Width: 1080, Height: 1920}
		}
		if aspect == "" {
			aspect = media.AspectSource
		}
		if res == "" {
			res = "1080p"
		}
		w, h := media.TargetSize(src.Width, src.Height, aspect, res)
		layout.Width, layout.Height = w, h
		return layout, fmt.Sprintf("画面 %s %s（%dx%d）", media.NormalizeAspect(aspect), res, w, h), nil
	}
	return layout, "", nil
}
