package absplice

import (
	"math"
	"testing"
)

func TestSuggestIDRCount(t *testing.T) {
	if g := SuggestIDRCount(10, 0, 1); g != 10 {
		t.Fatalf("auto 10s -> %d", g)
	}
	if g := SuggestIDRCount(10, 7, 1); g != 7 {
		t.Fatalf("manual -> %d", g)
	}
	if g := SuggestIDRCount(0.5, 0, 1); g != 1 {
		t.Fatalf("short -> %d", g)
	}
}

func TestEvenSampleTimes(t *testing.T) {
	ts := EvenSampleTimes(10, 5)
	if len(ts) != 5 {
		t.Fatal(len(ts))
	}
	if math.Abs(ts[0]-1) > 0.01 {
		t.Fatalf("first=%v want ~1", ts[0])
	}
	if math.Abs(ts[4]-9) > 0.01 {
		t.Fatalf("last=%v want ~9", ts[4])
	}
}

func TestSegmentWindows(t *testing.T) {
	w := SegmentWindows(1.0, 10.0, 4)
	if len(w) != 4 {
		t.Fatal(len(w))
	}
	sum := 0.0
	for _, x := range w {
		sum += x[1]
	}
	if math.Abs(sum-10) > 0.05 {
		t.Fatalf("sum dur=%v", sum)
	}
	if math.Abs(w[0][0]-1.0) > 1e-9 {
		t.Fatalf("start=%v", w[0][0])
	}
}

func TestIsImageVideoPath(t *testing.T) {
	if !IsImagePath(`a\b.JPG`) || IsVideoPath(`a\b.JPG`) {
		t.Fatal("jpg")
	}
	if !IsVideoPath(`x.mp4`) || IsImagePath(`x.mp4`) {
		t.Fatal("mp4")
	}
}
