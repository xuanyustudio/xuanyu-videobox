package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TimedAudio is one TTS (or other) audio file placed at StartSec on the output timeline.
type TimedAudio struct {
	Path     string
	StartSec float64
}

// ConcatVideos joins clipPaths in order using the concat demuxer.
func ConcatVideos(ctx context.Context, bins FFmpegBins, clipPaths []string, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	if len(clipPaths) == 0 {
		return fmt.Errorf("no clips to concat")
	}

	listPath := filepath.Join(filepath.Dir(outPath), "concat_list.txt")
	var b strings.Builder
	for _, p := range clipPaths {
		b.WriteString("file ")
		b.WriteString(quoteConcatFilePath(p))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(listPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}
	defer os.Remove(listPath)

	args := []string{
		"-y",
		"-f", "concat", "-safe", "0",
		"-i", listPath,
	}
	args = append(args, defaultVideoEncodeArgs()...)
	args = append(args, defaultAudioEncodeArgs()...)
	args = append(args, "-movflags", "+faststart", outPath)

	_, err := Run(ctx, bins.FFmpeg, args...)
	return err
}

func quoteConcatFilePath(path string) string {
	escaped := strings.ReplaceAll(path, `'`, `'\''`)
	return "'" + escaped + "'"
}

// MixTTSOntoVideo mixes delayed TTS inputs onto the video's audio bed.
//
// Phase 1: keep [0:a] when present; otherwise mix only the delayed TTS tracks.
// Pipeline later may prepend a silent bed; here amix duration=longest covers both cases.
func MixTTSOntoVideo(ctx context.Context, bins FFmpegBins, videoPath string, ttsClips []TimedAudio, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	if len(ttsClips) == 0 {
		return fmt.Errorf("no TTS clips to mix")
	}

	hasAudio, err := probeHasAudioStream(ctx, bins, videoPath)
	if err != nil {
		return err
	}

	args := []string{"-y", "-i", videoPath}
	for _, clip := range ttsClips {
		args = append(args, "-i", clip.Path)
	}

	filter := buildTTSMixFilter(hasAudio, len(ttsClips), ttsClips)
	args = append(args,
		"-filter_complex", filter,
		"-map", "0:v",
		"-map", "[aout]",
	)
	args = append(args, defaultVideoEncodeArgs()...)
	args = append(args, defaultAudioEncodeArgs()...)
	args = append(args, "-shortest", "-movflags", "+faststart", outPath)

	_, err = Run(ctx, bins.FFmpeg, args...)
	return err
}

func buildTTSMixFilter(hasVideoAudio bool, ttsCount int, clips []TimedAudio) string {
	var b strings.Builder
	inputLabels := make([]string, 0, ttsCount+1)

	if hasVideoAudio {
		b.WriteString("[0:a]aformat=sample_rates=44100:channel_layouts=stereo[abase];")
		inputLabels = append(inputLabels, "[abase]")
	}

	for i, clip := range clips {
		idx := i + 1
		delayMs := int(clip.StartSec * 1000)
		label := fmt.Sprintf("[t%d]", idx)
		fmt.Fprintf(&b, "[%d:a]adelay=%d|%d,aformat=sample_rates=44100:channel_layouts=stereo%s;",
			idx, delayMs, delayMs, label)
		inputLabels = append(inputLabels, label)
	}

	b.WriteString(strings.Join(inputLabels, ""))
	fmt.Fprintf(&b, "amix=inputs=%d:duration=longest:dropout_transition=0[aout]", len(inputLabels))
	return b.String()
}

func probeHasAudioStream(ctx context.Context, bins FFmpegBins, videoPath string) (bool, error) {
	probe := bins.FFprobe
	if probe == "" {
		var err error
		probe, err = ResolveFFprobe("", "")
		if err != nil {
			return false, err
		}
	}
	out, err := Run(ctx, probe,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		videoPath,
	)
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(out) == "audio", nil
}
