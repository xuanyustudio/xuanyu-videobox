package replicate_test

import (
	"math"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func TestParseSRTAndAssign(t *testing.T) {
	raw := "1\n00:00:01,000 --> 00:00:03,000\n你好\n\n" +
		"2\n00:00:14,000 --> 00:00:15,000\n正好结束\n\n" +
		"3\n00:00:15,000 --> 00:00:16,000\n正好开始\n\n" +
		"4\n00:00:14,500 --> 00:00:15,500\n跨过去\n\n" +
		"5\n00:00:20,000 --> 00:00:21,000\n后一段\n\n"
	cues := replicate.ParseSRT(raw)
	if len(cues) != 5 {
		t.Fatalf("cues=%d", len(cues))
	}
	first := replicate.CuesForSpan(cues, replicate.Span{Start: 0, End: 15})
	second := replicate.CuesForSpan(cues, replicate.Span{Start: 15, End: 30})
	if len(first) != 3 || first[0].Text != "你好" || first[1].Text != "正好结束" || first[2].Text != "跨过去" {
		t.Fatalf("first=%+v", first)
	}
	if len(second) != 3 || second[0].Text != "跨过去" || second[1].Text != "正好开始" || second[2].Text != "后一段" {
		t.Fatalf("second=%+v", second)
	}
	if math.Abs(first[2].Start-14.5) > 1e-9 || math.Abs(first[2].End-15.5) > 1e-9 {
		t.Fatalf("crossing cue changed: %+v", first[2])
	}
	if len(replicate.CuesForSpan(nil, replicate.Span{Start: 0, End: 15})) != 0 {
		t.Fatal("nil cues")
	}
	outside := replicate.CuesForSpan(cues, replicate.Span{Start: 3, End: 14})
	if len(outside) != 0 {
		t.Fatalf("outside=%+v", outside)
	}
}

func TestFormatCueLinesStripsSpeaker(t *testing.T) {
	cues := []replicate.Cue{{Start: 15.2, End: 18, Text: "说话人1：卖点来了"}}
	got := replicate.FormatCueLines(cues)
	if got != "15.2–18.0秒：卖点来了" {
		t.Fatalf("%q", got)
	}
	if replicate.FormatCueLines(nil) != "" {
		t.Fatal("empty")
	}
}
