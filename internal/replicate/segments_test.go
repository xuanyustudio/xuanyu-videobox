package replicate_test

import (
	"math"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func spansEqual(t *testing.T, got []replicate.Span, want []replicate.Span) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len %d != %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if math.Abs(got[i].Start-want[i].Start) > 1e-9 || math.Abs(got[i].End-want[i].End) > 1e-9 {
			t.Fatalf("span %d got %v want %v", i, got[i], want[i])
		}
	}
}

func TestNormalizeSegmentSec(t *testing.T) {
	if replicate.NormalizeSegmentSec(10) != 10 || replicate.NormalizeSegmentSec(15) != 15 {
		t.Fatal("10 and 15 must pass through")
	}
	for _, n := range []int{0, 12, 30, -1} {
		if replicate.NormalizeSegmentSec(n) != 15 {
			t.Fatalf("%d should become 15", n)
		}
	}
}

func TestPlanSegments(t *testing.T) {
	spansEqual(t, replicate.PlanSegments(0, 15), []replicate.Span{{Start: 0, End: 60}})
	spansEqual(t, replicate.PlanSegments(10, 15), []replicate.Span{{Start: 0, End: 10}})
	spansEqual(t, replicate.PlanSegments(15, 15), []replicate.Span{{Start: 0, End: 15}})
	spansEqual(t, replicate.PlanSegments(15.4, 15), []replicate.Span{{Start: 0, End: 15.4}})
	spansEqual(t, replicate.PlanSegments(16, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 16}})
	spansEqual(t, replicate.PlanSegments(30, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30}})
	spansEqual(t, replicate.PlanSegments(30.4, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30.4}})
	spansEqual(t, replicate.PlanSegments(31, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30}, {Start: 30, End: 31}})
	spansEqual(t, replicate.PlanSegments(46, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30}, {Start: 30, End: 45}, {Start: 45, End: 46}})
	spansEqual(t, replicate.PlanSegments(10, 10), []replicate.Span{{Start: 0, End: 10}})
	spansEqual(t, replicate.PlanSegments(10.4, 10), []replicate.Span{{Start: 0, End: 10.4}})
	spansEqual(t, replicate.PlanSegments(11, 10), []replicate.Span{{Start: 0, End: 10}, {Start: 10, End: 11}})
	spansEqual(t, replicate.PlanSegments(20, 10), []replicate.Span{{Start: 0, End: 10}, {Start: 10, End: 20}})
	spansEqual(t, replicate.PlanSegments(25, 10), []replicate.Span{{Start: 0, End: 10}, {Start: 10, End: 20}, {Start: 20, End: 25}})
	spansEqual(t, replicate.PlanSegments(16, 12), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 16}})
}
