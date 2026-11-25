package pipeline_test

import (
	"testing"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/pipeline"
)

func TestApplyConfigSubtitleOnMaskDefaultsOff(t *testing.T) {
	cfg := config.Default()
	var params pipeline.Params
	pipeline.ApplyConfig(&params, cfg)
	if params.SubtitleOnMask {
		t.Fatal("on-mask must stay off unless configured")
	}
	if params.SubtitlePosition != "bottom" {
		t.Fatalf("position %q", params.SubtitlePosition)
	}
}

func TestApplyConfigSubtitleOnMask(t *testing.T) {
	cfg := config.Default()
	cfg.UI.SubtitleOnMask = true
	cfg.UI.SubtitlePosition = "top"
	cfg.UI.SubtitleMaskY = 79
	cfg.UI.SubtitleMaskHeight = 16
	var params pipeline.Params
	pipeline.ApplyConfig(&params, cfg)
	if !params.SubtitleOnMask {
		t.Fatal("subtitle_on_mask was not applied")
	}
	if params.SubtitlePosition != "top" {
		t.Fatalf("position %q", params.SubtitlePosition)
	}
	if params.Mask.Y != 79 || params.Mask.Height != 16 {
		t.Fatalf("mask %+v", params.Mask)
	}
}
