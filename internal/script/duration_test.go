package script_test

import (
	"testing"

	"github.com/narratoai/narrato-go/internal/script"
)

func TestEstimateOST1UsesTimestamp(t *testing.T) {
	items := []script.Item{
		{ID: 1, OST: 0, Narration: "这是一段大约二十个汉字左右的解说文案内容测试。"},
		{ID: 2, OST: 1, Timestamp: "00:00:10,000-00:00:20,000", Narration: "播放原片"},
	}
	total := script.EstimateOutputDurationSec(items)
	if total <= 10 || total >= 40 {
		t.Fatalf("total=%v", total)
	}
}

func TestFormatDurationClock(t *testing.T) {
	if g := script.FormatDurationClock(180); g != "03:00" {
		t.Fatal(g)
	}
}

func TestStripSpeakerLabels(t *testing.T) {
	got := script.StripSpeakerLabels("说话人3: 俺妈说了")
	if got != "俺妈说了" {
		t.Fatalf("%q", got)
	}
}
