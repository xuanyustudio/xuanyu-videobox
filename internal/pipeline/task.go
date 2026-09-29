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
	HideOnOriginal *bool // nil → true (hide OST=1 narration in SRT)
	OnProgress     func(step string, percent int, msg string)
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

	report("clip", 25, "cutting segments")
	clipPaths := make([]string, 0, len(workItems))
	var timeline float64
	for i := range workItems {
		item := &workItems[i]
		videoPath, err := resolveVideoPath(item.VideoID, p.VideoPaths)
		if err != nil {
			return "", fmt.Errorf("item %d video: %w", i, err)
		}
		ts := strings.TrimSpace(item.Timestamp)
		if ts == "" {
			return "", fmt.Errorf("item %d: timestamp required", i)
		}

		id := item.ID
		if id == 0 {
			id = i + 1
		}
		ttsDur := ttsDurByID[id]
		isHook := item.HookSourceID != nil

		clipPath := filepath.Join(p.WorkDir, "clips", fmt.Sprintf("%d.mp4", i))
		if err := media.ClipSegment(ctx, bins, videoPath, ts, item.OST, ttsDur, isHook, clipPath); err != nil {
			return "", fmt.Errorf("clip item %d: %w", i, err)
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
	if hideOriginal {
		ranges := ost1EditedRanges(workItems)
		if expr := media.MaskEnableExpr(ranges); expr != "" {
			report("mask", 40, fmt.Sprintf("mask disable expr (phase 1 log only): %s", expr))
		}
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

	mixedPath := filepath.Join(p.WorkDir, "mixed.mp4")
	report("mix", 70, "mixing TTS")
	timed := buildTimedTTS(workItems, ttsPathByID)
	if len(timed) == 0 {
		if err := copyFile(concatPath, mixedPath); err != nil {
			return "", fmt.Errorf("copy concat to mixed: %w", err)
		}
	} else {
		if err := media.MixTTSOntoVideo(ctx, bins, concatPath, timed, mixedPath); err != nil {
			return "", fmt.Errorf("mix tts: %w", err)
		}
	}

	report("burn", 85, "burning subtitles")
	if srtBody == "" {
		if err := copyFile(mixedPath, outPath); err != nil {
			return "", fmt.Errorf("copy output: %w", err)
		}
	} else {
		if err := media.BurnSubtitles(ctx, bins, mixedPath, srtPath, outPath); err != nil {
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
	return float64(h*3600+m*60+s), true
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
