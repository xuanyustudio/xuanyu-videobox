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

// MixTTSOntoVideo mixes delayed TTS inputs onto the video's audio bed at equal volume.
func MixTTSOntoVideo(ctx context.Context, bins FFmpegBins, videoPath string, ttsClips []TimedAudio, outPath string) error {
	return MixAudioOntoVideo(ctx, bins, videoPath, ttsClips, MixOptions{OriginalVolume: 1, VoiceVolume: 1}, outPath)
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
