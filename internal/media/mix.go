package media

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MixOptions controls how original audio, narration, BGM and SFX are mixed.
// Volume values use the same 0–2 scale as NarratoAI (1.0 = 100%).
type MixOptions struct {
	OriginalVolume float64
	VoiceVolume    float64
	BGMPath        string
	BGMVolume      float64
	SFXPath        string
	SFXVolume      float64
	DurationSec    float64 // used to loop/trim BGM; 0 = probe
}

// MixAudioOntoVideo mixes TTS, optional BGM and optional SFX onto the video audio bed.
func MixAudioOntoVideo(ctx context.Context, bins FFmpegBins, videoPath string, ttsClips []TimedAudio, opt MixOptions, outPath string) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	hasBGM := strings.TrimSpace(opt.BGMPath) != ""
	hasSFX := strings.TrimSpace(opt.SFXPath) != ""
	if len(ttsClips) == 0 && !hasBGM && !hasSFX {
		return fmt.Errorf("no audio clips to mix")
	}

	hasAudio, err := probeHasAudioStream(ctx, bins, videoPath)
	if err != nil {
		return err
	}
	dur := opt.DurationSec
	if dur <= 0 {
		if d, derr := ProbeDurationSec(ctx, bins, videoPath); derr == nil {
			dur = d
		}
	}

	args := []string{"-y", "-i", videoPath}
	for _, clip := range ttsClips {
		args = append(args, "-i", clip.Path)
	}
	if hasBGM {
		args = append(args, "-i", opt.BGMPath)
	}
	if hasSFX {
		args = append(args, "-i", opt.SFXPath)
	}

	filter := BuildMixFilter(hasAudio, ttsClips, opt, dur)
	args = append(args,
		"-filter_complex", filter,
		"-map", "0:v",
		"-map", "[aout]",
	)
	args = append(args, defaultVideoEncodeArgs()...)
	args = append(args, defaultAudioEncodeArgs()...)
	if dur > 0 {
		args = append(args, "-t", formatMixDuration(dur))
	} else {
		args = append(args, "-shortest")
	}
	args = append(args, "-movflags", "+faststart", outPath)

	_, err = Run(ctx, bins.FFmpeg, args...)
	return err
}

func formatMixDuration(sec float64) string {
	s := strconv.FormatFloat(sec, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

func mixVolume(v, fallback float64) float64 {
	if v != v { // NaN
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

func relabelLast(filter, oldLabel string) string {
	if strings.HasSuffix(filter, oldLabel+";") {
		return strings.TrimSuffix(filter, oldLabel+";") + "[aout]"
	}
	if strings.HasSuffix(filter, oldLabel) {
		return strings.TrimSuffix(filter, oldLabel) + "[aout]"
	}
	return filter + oldLabel + "acopy[aout]"
}

// BuildMixFilter is exported for unit tests.
func BuildMixFilter(hasVideoAudio bool, clips []TimedAudio, opt MixOptions, durSec float64) string {
	origVol := mixVolume(opt.OriginalVolume, 1)
	voiceVol := mixVolume(opt.VoiceVolume, 1)
	bgmVol := mixVolume(opt.BGMVolume, 0.3)
	sfxVol := mixVolume(opt.SFXVolume, 0.6)
	hasBGM := strings.TrimSpace(opt.BGMPath) != ""
	hasSFX := strings.TrimSpace(opt.SFXPath) != ""

	var b strings.Builder
	labels := make([]string, 0, len(clips)+3)

	if hasVideoAudio {
		fmt.Fprintf(&b, "[0:a]aformat=sample_rates=44100:channel_layouts=stereo,volume=%s[abase];", formatFFmpegFloat(origVol))
		labels = append(labels, "[abase]")
	}

	next := 1
	for i, clip := range clips {
		delayMs := int(clip.StartSec * 1000)
		if delayMs < 0 {
			delayMs = 0
		}
		label := fmt.Sprintf("[t%d]", i+1)
		fmt.Fprintf(&b, "[%d:a]adelay=%d|%d,aformat=sample_rates=44100:channel_layouts=stereo,volume=%s%s;",
			next, delayMs, delayMs, formatFFmpegFloat(voiceVol), label)
		labels = append(labels, label)
		next++
	}
	if hasBGM {
		trim := durSec
		if trim <= 0 {
			trim = 86400
		}
		fmt.Fprintf(&b, "[%d:a]aloop=loop=-1:size=2147483647,atrim=duration=%s,asetpts=N/SR/TB,aformat=sample_rates=44100:channel_layouts=stereo,volume=%s[bgm];",
			next, formatFFmpegFloat(trim), formatFFmpegFloat(bgmVol))
		labels = append(labels, "[bgm]")
		next++
	}
	if hasSFX {
		fmt.Fprintf(&b, "[%d:a]aformat=sample_rates=44100:channel_layouts=stereo,volume=%s[sfx];",
			next, formatFFmpegFloat(sfxVol))
		labels = append(labels, "[sfx]")
	}

	if len(labels) == 0 {
		b.WriteString("anullsrc=r=44100:cl=stereo,atrim=duration=1[aout]")
		return b.String()
	}
	if len(labels) == 1 {
		return relabelLast(b.String(), labels[0])
	}

	b.WriteString(strings.Join(labels, ""))
	fmt.Fprintf(&b, "amix=inputs=%d:duration=longest:dropout_transition=0:normalize=0[aout]", len(labels))
	return b.String()
}
