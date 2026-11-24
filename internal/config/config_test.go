package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/config"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.LLM.APIKey = "sk-test"
	cfg.UI.TargetDurationSec = 180
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LLM.APIKey != "sk-test" {
		t.Fatalf("api key=%q", got.LLM.APIKey)
	}
	if got.UI.SubtitleHideOnOriginal != true {
		t.Fatal("hide on original should default true")
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.UI.TargetDurationSec != 180 {
		t.Fatalf("got %d", got.UI.TargetDurationSec)
	}
	_ = os.Stderr
}

func TestDefaultASRKeepsBailianAndLocalURL(t *testing.T) {
	cfg := config.Default()
	if cfg.ASR.Backend != "bailian" {
		t.Fatalf("backend=%q", cfg.ASR.Backend)
	}
	if cfg.ASR.APIURL != "http://127.0.0.1:7860" {
		t.Fatalf("api url=%q", cfg.ASR.APIURL)
	}
	if cfg.ASR.FireRedAPIURL != "http://127.0.0.1:7867" {
		t.Fatalf("firered url=%q", cfg.ASR.FireRedAPIURL)
	}
}

func TestReplicateDefaultsAndRoundTrip(t *testing.T) {
	cfg := config.Default()
	if cfg.Replicate.BaseURL == "" || cfg.Replicate.Model == "" || cfg.Replicate.Frames != 8 {
		t.Fatalf("replicate defaults: %#v", cfg.Replicate)
	}
	if cfg.Replicate.SegmentSec != 15 {
		t.Fatalf("replicate segment_sec default: %#v", cfg.Replicate)
	}
	if cfg.Replicate.APIKey != "" {
		t.Fatal("replicate api key must not have a default")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg.Replicate.APIKey = "ms-test"
	cfg.Replicate.Frames = 12
	cfg.Replicate.SegmentSec = 10
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Replicate.APIKey != "ms-test" || got.Replicate.Frames != 12 || got.Replicate.SegmentSec != 10 {
		t.Fatalf("got %#v", got.Replicate)
	}
}

func TestVideoKeyRoundTrip(t *testing.T) {
	cfg := config.Default()
	if cfg.Video.APIKey != "" {
		t.Fatal("video api key must not have a default")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg.Video.APIKey = "vw_test"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Video.APIKey != "vw_test" {
		t.Fatalf("video key=%q", got.Video.APIKey)
	}
}

func TestImageGenDefaultsAndRoundTrip(t *testing.T) {
	cfg := config.Default()
	if cfg.Image.Model != "gpt-image-1" || cfg.Image.APIKey != "" || cfg.Image.BaseURL != "" {
		t.Fatalf("image defaults: %#v", cfg.Image)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg.Image.BaseURL = "https://ai.example.com/v1"
	cfg.Image.APIKey = "sk-img"
	cfg.Image.Model = "gpt-image-2"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Image.BaseURL != cfg.Image.BaseURL || got.Image.APIKey != "sk-img" || got.Image.Model != "gpt-image-2" {
		t.Fatalf("got %#v", got.Image)
	}
}

func TestMiniMaxDefaultsAndRoundTrip(t *testing.T) {
	cfg := config.Default()
	if cfg.MiniMax.APIKey != "" || cfg.MiniMax.BaseURL != "" || cfg.MiniMax.Model != "" {
		t.Fatalf("minimax defaults: %#v", cfg.MiniMax)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg.MiniMax.BaseURL = "https://api.minimax.cn"
	cfg.MiniMax.APIKey = "mm-test"
	cfg.MiniMax.Model = "speech-2.8-hd"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.MiniMax.BaseURL != cfg.MiniMax.BaseURL || got.MiniMax.APIKey != "mm-test" || got.MiniMax.Model != "speech-2.8-hd" {
		t.Fatalf("got %#v", got.MiniMax)
	}
}

func TestRenderSettingsDefaultsAndRoundTrip(t *testing.T) {
	cfg := config.Default()
	if cfg.TTS.Engine != "edge" || cfg.TTS.Speed != 1 || cfg.TTS.Volume != 1 {
		t.Fatalf("tts defaults: %#v", cfg.TTS)
	}
	if !cfg.UI.SubtitleBurnEnabled || cfg.UI.AspectRatio != "vertical" || cfg.UI.Resolution != "1080p" {
		t.Fatalf("ui defaults: %#v", cfg.UI)
	}
	if cfg.UI.VoiceVolume != 1 || cfg.UI.OriginalVolume != 1.2 || cfg.UI.BGMVolume != 0.3 {
		t.Fatalf("volume defaults: %#v", cfg.UI)
	}
	if cfg.Sonilo.APIKey != "" {
		t.Fatal("sonilo key must not have a default")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg.TTS.Engine = "minimax"
	cfg.TTS.Voice = "XuanyuTestVoice"
	cfg.TTS.Speed = 1.2
	cfg.TTS.Volume = 0.8
	cfg.TTS.Pitch = -2
	cfg.UI.SubtitleBurnEnabled = false
	cfg.UI.SubtitleFont = "Microsoft YaHei"
	cfg.UI.SubtitleFontSize = 22
	cfg.UI.SubtitleColor = "#FFFF00"
	cfg.UI.SubtitlePosition = "top"
	cfg.UI.SubtitleMaskEnabled = true
	cfg.UI.SubtitleMaskY = 80
	cfg.UI.SubtitleMaskHeight = 16
	cfg.UI.AspectRatio = "horizontal"
	cfg.UI.Resolution = "720p"
	cfg.UI.VoiceVolume = 0.9
	cfg.UI.OriginalVolume = 1.1
	cfg.UI.BGMSource = "library"
	cfg.UI.BGMPath = "/tmp/theme.mp3"
	cfg.UI.BGMVolume = 0.25
	cfg.Sonilo.BaseURL = "https://api.sonilo.com"
	cfg.Sonilo.APIKey = "sn-test"
	cfg.Sonilo.BGMPrompt = "cinematic"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.TTS.Engine != "minimax" || got.TTS.Voice != "XuanyuTestVoice" || got.TTS.Speed != 1.2 || got.TTS.Pitch != -2 {
		t.Fatalf("tts: %#v", got.TTS)
	}
	if got.UI.SubtitleBurnEnabled || got.UI.AspectRatio != "horizontal" || got.UI.Resolution != "720p" {
		t.Fatalf("ui: %#v", got.UI)
	}
	if got.UI.BGMSource != "library" || got.UI.BGMPath != "/tmp/theme.mp3" {
		t.Fatalf("bgm: %#v", got.UI)
	}
	if got.Sonilo.APIKey != "sn-test" || got.Sonilo.BGMPrompt != "cinematic" {
		t.Fatalf("sonilo: %#v", got.Sonilo)
	}
}

func TestLoadLegacyConfigKeepsBurnAndVolumes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	legacy := []byte("tts:\n  voice: zh-CN-YunjianNeural\nui:\n  target_duration_sec: 180\n  subtitle_hide_on_original: true\n")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UI.SubtitleBurnEnabled {
		t.Fatal("missing burn flag should stay default true")
	}
	if got.UI.VoiceVolume != 1 || got.UI.OriginalVolume != 1.2 {
		t.Fatalf("legacy volumes: voice=%v orig=%v", got.UI.VoiceVolume, got.UI.OriginalVolume)
	}
	if got.TTS.Engine != "edge" || got.TTS.Voice != "zh-CN-YunjianNeural" {
		t.Fatalf("legacy tts: %#v", got.TTS)
	}
}
