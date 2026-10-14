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
	if cfg.Replicate.APIKey != "" {
		t.Fatal("replicate api key must not have a default")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg.Replicate.APIKey = "ms-test"
	cfg.Replicate.Frames = 12
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Replicate.APIKey != "ms-test" || got.Replicate.Frames != 12 {
		t.Fatalf("got %#v", got.Replicate)
	}
}
