package media

import (
	"math"
	"testing"
)

func TestResolveClipDuration(t *testing.T) {
	d := ResolveClipDuration(1.2, false)
	if d < 2.0 {
		t.Fatal(d)
	}
	d = ResolveClipDuration(1.2, true)
	if d < 5.001 {
		t.Fatal(d)
	}
}

func TestResolveClipDurationLongTTSNonHook(t *testing.T) {
	want := 3.5 + 0.35
	d := ResolveClipDuration(3.5, false)
	if math.Abs(d-want) > 1e-9 {
		t.Fatalf("got %v want %v", d, want)
	}
}
