package media_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
)

func TestBuildMixFilterVolumes(t *testing.T) {
	clips := []media.TimedAudio{{Path: "a.wav", StartSec: 1.5}}
	opt := media.MixOptions{OriginalVolume: 1.2, VoiceVolume: 0.8, BGMPath: "bgm.mp3", BGMVolume: 0.3}
	f := media.BuildMixFilter(true, clips, opt, 10)
	if !strings.Contains(f, "volume=1.2[abase]") {
		t.Fatalf("missing original volume: %s", f)
	}
	if !strings.Contains(f, "adelay=1500|1500") {
		t.Fatalf("missing delay: %s", f)
	}
	if !strings.Contains(f, "volume=0.8[t1]") {
		t.Fatalf("missing voice volume: %s", f)
	}
	if !strings.Contains(f, "volume=0.3[bgm]") {
		t.Fatalf("missing bgm volume: %s", f)
	}
	if !strings.Contains(f, "normalize=0") {
		t.Fatalf("amix should not auto-normalize: %s", f)
	}
	if !strings.Contains(f, "amix=inputs=3") {
		t.Fatalf("expected 3 inputs: %s", f)
	}
}

func TestBuildMixFilterTTSOnlyNoOriginal(t *testing.T) {
	f := media.BuildMixFilter(false, []media.TimedAudio{{Path: "a.wav", StartSec: 0}}, media.MixOptions{VoiceVolume: 1}, 5)
	if strings.Contains(f, "[0:a]") {
		t.Fatalf("should not use missing video audio: %s", f)
	}
	if !strings.Contains(f, "[aout]") {
		t.Fatalf("missing aout: %s", f)
	}
}
