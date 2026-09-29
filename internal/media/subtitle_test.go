package media_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/script"
)

func TestBuildScriptSRTHidesOriginalWhenConfigured(t *testing.T) {
	items := []script.Item{
		{
			ID:              1,
			OST:             0,
			Narration:       "解说开头测试内容。",
			EditedTimeRange: "00:00:00-00:00:04",
		},
		{
			ID:              2,
			OST:             1,
			Narration:       "播放原片对白测试",
			EditedTimeRange: "00:00:04-00:00:08",
		},
	}

	srt, err := media.BuildScriptSRT(items, media.SubtitleOptions{
		HideOnOriginal: true,
		MaxCharsPerCue: 12,
	})
	if err != nil {
		t.Fatalf("BuildScriptSRT: %v", err)
	}
	if !strings.Contains(srt, "解说开头测试内容") {
		t.Fatalf("expected narration in SRT, got:\n%s", srt)
	}
	if strings.Contains(srt, "播放原片对白测试") {
		t.Fatalf("OST=1 narration must be hidden, got:\n%s", srt)
	}
	if !strings.Contains(srt, "00:00:00,000 --> 00:00:04,000") {
		t.Fatalf("expected editedTimeRange start window, got:\n%s", srt)
	}
}

func TestBuildScriptSRTStripsSpeakerLabels(t *testing.T) {
	items := []script.Item{
		{
			ID:              1,
			OST:             0,
			Narration:       "说话人3: 俺妈说了",
			EditedTimeRange: "00:00:00-00:00:03",
		},
	}

	srt, err := media.BuildScriptSRT(items, media.SubtitleOptions{
		HideOnOriginal: true,
		MaxCharsPerCue: 12,
	})
	if err != nil {
		t.Fatalf("BuildScriptSRT: %v", err)
	}
	if !strings.Contains(srt, "俺妈说了") {
		t.Fatalf("expected cleaned narration, got:\n%s", srt)
	}
	if strings.Contains(srt, "说话人") {
		t.Fatalf("speaker labels must be stripped, got:\n%s", srt)
	}
}

func TestBuildScriptSRTChunksLongNarrationAtMaxCharsPerCue(t *testing.T) {
	const maxChars = 12
	narration := strings.Repeat("中", 30)

	items := []script.Item{
		{
			ID:              1,
			OST:             0,
			Narration:       narration,
			EditedTimeRange: "00:00:00-00:00:12",
		},
	}

	srt, err := media.BuildScriptSRT(items, media.SubtitleOptions{
		HideOnOriginal: true,
		MaxCharsPerCue: maxChars,
	})
	if err != nil {
		t.Fatalf("BuildScriptSRT: %v", err)
	}

	cues := parseSRTCueTexts(srt)
	if len(cues) < 2 {
		t.Fatalf("expected multiple cues for 30 runes, got %d: %v", len(cues), cues)
	}
	for i, cue := range cues {
		if n := len([]rune(cue)); n > maxChars {
			t.Errorf("cue %d has %d runes, want <= %d: %q", i+1, n, maxChars, cue)
		}
	}
}

func parseSRTCueTexts(srt string) []string {
	srt = strings.TrimSpace(srt)
	if srt == "" {
		return nil
	}
	var texts []string
	for _, block := range strings.Split(srt, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 3 {
			continue
		}
		texts = append(texts, strings.Join(lines[2:], "\n"))
	}
	return texts
}
