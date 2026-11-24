package media

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/script"
)

func TestFormatScriptClock(t *testing.T) {
	if got := FormatScriptClock(174720); got != "00:02:54,720" {
		t.Fatalf("got %s", got)
	}
	if got := FormatScriptClock(178000); got != "00:02:58,000" {
		t.Fatalf("got %s", got)
	}
}

func TestFitClipRangeToSourcePastEOF(t *testing.T) {
	// Source is 174.720s; clip 11 starts at 00:02:58.
	_, _, err := FitClipRangeToSource(178000, 195000, 174720)
	if err == nil {
		t.Fatal("expected error when start is past EOF")
	}
	msg := err.Error()
	if !strings.Contains(msg, "00:02:58,000") || !strings.Contains(msg, "00:02:54,720") || !strings.Contains(msg, "已经超出片尾") {
		t.Fatalf("unexpected error: %s", msg)
	}
}

func TestFitClipRangeToSourceClampsEnd(t *testing.T) {
	start, end, err := FitClipRangeToSource(162000, 178000, 174720)
	if err != nil {
		t.Fatal(err)
	}
	if start != 162000 || end != 174720 {
		t.Fatalf("got %d-%d", start, end)
	}
}

func TestFitClipRangeToSourceTinyRemainder(t *testing.T) {
	_, _, err := FitClipRangeToSource(174700, 180000, 174720)
	if err == nil {
		t.Fatal("expected error when leftover is under 50ms")
	}
}

func TestPlanClipRangeOST0UsesTTS(t *testing.T) {
	start, end, err := PlanClipRange("00:02:42,000-00:02:58,000", 0, 10.21, false)
	if err != nil {
		t.Fatal(err)
	}
	if start != 162000 {
		t.Fatalf("start %d", start)
	}
	// OST 0 length is max(tts+tailPad, generalMin) = 10.21+0.35 = 10.56s
	wantEnd := 162000 + int((10.21+script.TailPad)*1000)
	// durationToMs uses Round
	if end != 162000+10560 && end != wantEnd {
		// Allow either exact rounded 10.56s or computed
		gotDur := end - start
		if gotDur < 10500 || gotDur > 10600 {
			t.Fatalf("end %d (dur %d) want around 10560", end, gotDur)
		}
	}
}
