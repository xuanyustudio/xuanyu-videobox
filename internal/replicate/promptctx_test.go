package replicate_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func TestBuildSegmentUserPrompt(t *testing.T) {
	cues := []replicate.Cue{{Start: 1, End: 2, Text: "说话人2：开场白"}}
	span := replicate.Span{Start: 0, End: 12}
	std := replicate.BuildSegmentUserPrompt(replicate.TemplateStandard, 8, span, 15, cues, false)
	if !strings.Contains(std, "8 帧") || !strings.Contains(std, "0.0–12.0") || !strings.Contains(std, "1.0–2.0秒：开场白") {
		t.Fatalf("%s", std)
	}
	if strings.Contains(std, "说话人") || strings.Contains(std, "第一张图只用于统一外形") {
		t.Fatal("single segment should not mention the lock frame or speaker label")
	}
	if !strings.Contains(std, "时间戳从 0 开始") {
		t.Fatal("missing relative clock")
	}

	none := replicate.BuildSegmentUserPrompt(replicate.TemplateStandard, 8, span, 15, nil, false)
	if !strings.Contains(none, "本段字幕：无对白") {
		t.Fatal(none)
	}

	sd := replicate.BuildSegmentUserPrompt(replicate.TemplateSeedance2, 8, replicate.Span{Start: 15, End: 30}, 10, nil, true)
	if !strings.Contains(sd, "0–2、2–4、4–7、7–9、9–10") || !strings.Contains(sd, "恰好 10 秒") {
		t.Fatalf("10s beats: %s", sd)
	}
	if strings.Contains(sd, "恰好 15 秒") {
		t.Fatal("segment prompt must override the template's 15s clock")
	}
	if !strings.Contains(sd, "第一张图只用于统一外形") {
		t.Fatal("multi segment missing ref-frame note")
	}

	sd15 := replicate.BuildSegmentUserPrompt(replicate.TemplateSeedance2, 8, replicate.Span{Start: 0, End: 15}, 15, nil, false)
	if !strings.Contains(sd15, "0–3、3–6、6–10、10–13、13–15") {
		t.Fatalf("15s beats: %s", sd15)
	}
}
