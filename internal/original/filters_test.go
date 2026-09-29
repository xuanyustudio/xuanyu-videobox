package original

import (
	"strings"
	"testing"
)

func TestNormalizeSettings_ZoomMutex(t *testing.T) {
	in := []OpSettings{
		{ID: "zoom_in", Enabled: true, Intensity: 40},
		{ID: "zoom_out", Enabled: true, Intensity: 40},
	}
	out, notice := NormalizeSettings(in)
	if notice == "" {
		t.Fatal("expected mutex notice")
	}
	var zi, zo bool
	for _, s := range out {
		if s.ID == "zoom_in" {
			zi = s.Enabled
		}
		if s.ID == "zoom_out" {
			zo = s.Enabled
		}
	}
	if zi && zo {
		t.Fatal("both zoom ops still enabled")
	}
	if !zo {
		t.Fatal("expected later zoom_out to win")
	}
}

func TestValidateParams_EmptyOps(t *testing.T) {
	err := ValidateParams(Params{VideoPath: "x.mp4", Settings: nil})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildFilters_IntensitySnapshots(t *testing.T) {
	cases := []struct {
		id    string
		level int
		wantV string // substring in vf
		wantA string // substring in af
	}{
		{"fps", 50, "setpts=", "atempo="},
		{"fps", 100, "setpts=", "atempo="},
		{"brightness", 40, "eq=brightness=", ""},
		{"hue", 30, "hue=h=", ""},
		{"noise", 25, "noise=alls=", ""},
		{"audio_micro", 30, "", "volume="},
		{"rotate", 30, "rotate=a=", ""},
		{"freeze", 40, "tpad=", "apad="},
		{"freeze", 80, "start_mode=clone", "adelay="},
		{"edge_pad", 30, "fade=t=in", ""},
		{"watermark", 35, "drawtext=", ""},
		{"hflip", 0, "hflip", ""},
	}
	for _, tc := range cases {
		chain := BuildFilters(FilterInput{
			Settings: []OpSettings{{ID: tc.id, Enabled: true, Intensity: tc.level}},
			DurationSec: 10,
			WatermarkText: "T",
		})
		joinedV := chain.VFJoin
		joinedA := chain.AFJoin
		if tc.wantV != "" && !strings.Contains(joinedV, tc.wantV) {
			t.Fatalf("%s@%d vf missing %q; got %q", tc.id, tc.level, tc.wantV, joinedV)
		}
		if tc.wantA != "" && !strings.Contains(joinedA, tc.wantA) {
			t.Fatalf("%s@%d af missing %q; got %q", tc.id, tc.level, tc.wantA, joinedA)
		}
		if !strings.Contains(joinedV, "format=yuv420p") && tc.wantV != "" {
			t.Fatalf("%s expected yuv420p footer", tc.id)
		}
	}
}

func TestBuildFilters_DisabledNoOp(t *testing.T) {
	chain := BuildFilters(FilterInput{
		Settings: []OpSettings{{ID: "fps", Enabled: false, Intensity: 50}},
	})
	if chain.VFJoin != "" || chain.AFJoin != "" {
		t.Fatalf("disabled op produced filters: vf=%q af=%q", chain.VFJoin, chain.AFJoin)
	}
}

func TestBuildFilters_PreviewSameChainShape(t *testing.T) {
	settings := []OpSettings{
		{ID: "fps", Enabled: true, Intensity: 50},
		{ID: "brightness", Enabled: true, Intensity: 40},
	}
	a := BuildFilters(FilterInput{Settings: settings, DurationSec: 30})
	b := BuildFilters(FilterInput{Settings: settings, DurationSec: 30})
	if a.VFJoin != b.VFJoin || a.AFJoin != b.AFJoin {
		t.Fatal("identical inputs must yield identical filter chains")
	}
}

func TestAtlasLayers_NoConstructionAdvice(t *testing.T) {
	banned := []string{"如何构造", "改 NAL", "篡改 CRC", "ffmpeg -bsf", "bitstream filter"}
	for _, layer := range AtlasLayers() {
		blob := layer.Title + layer.Intro
		for _, e := range layer.Entries {
			blob += e.Name + e.Feel + e.Note
		}
		lower := strings.ToLower(blob)
		for _, b := range banned {
			if strings.Contains(lower, strings.ToLower(b)) {
				t.Fatalf("atlas contains banned phrase %q", b)
			}
		}
	}
	if !strings.Contains(AtlasHeader, "不提供") {
		t.Fatal("header should state product does not provide risky ops")
	}
}
