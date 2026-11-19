package reverse

import "testing"

func TestShotsFromCutsAndFallback(t *testing.T) {
	shots := shotsFromCuts(10, []float64{-1, 0.1, 2.5, 2.6, 9.9, 12})
	if len(shots) != 2 {
		t.Fatalf("got %d shots: %+v", len(shots), shots)
	}
	if shots[0].Start != 0 || shots[0].End != 2.5 || shots[1].Start != 2.5 || shots[1].End != 10 {
		t.Fatalf("unexpected bounds: %+v", shots)
	}
	one := shotsFromCuts(8, nil)
	if len(one) != 1 || one[0].End != 8 {
		t.Fatalf("no-cut should be one shot, got %+v", one)
	}
}

func TestMergeToMaxShots(t *testing.T) {
	var shots []Shot
	for i := 0; i < 10; i++ {
		shots = append(shots, Shot{Start: float64(i), End: float64(i) + 1})
	}
	got := mergeToMaxShots(shots, 4)
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Start != 0 || got[len(got)-1].End != 10 {
		t.Fatalf("range broken: %+v", got)
	}
	for i, s := range got {
		if s.Index != i+1 {
			t.Fatalf("index %d: %+v", i, s)
		}
	}
}

func TestPlanSampleTimesShortShots(t *testing.T) {
	shots := []Shot{
		{Index: 1, Start: 0, End: 0.6},
		{Index: 2, Start: 0.6, End: 2.6},
		{Index: 3, Start: 2.6, End: 8.6},
	}
	pts := planSampleTimes(shots, 96)
	if len(pts) < 5 {
		t.Fatalf("expected first/last plus fills, got %d: %+v", len(pts), pts)
	}
	var byShot [4]int
	for _, p := range pts {
		byShot[p.Shot]++
	}
	if byShot[1] != 1 {
		t.Fatalf("short shot should have 1 frame, got %d", byShot[1])
	}
	if byShot[2] < 2 {
		t.Fatalf("2s shot should have first+last, got %d", byShot[2])
	}
	if byShot[3] < 3 {
		t.Fatalf("6s shot should get mid fills, got %d", byShot[3])
	}
	for i := 1; i < len(pts); i++ {
		if pts[i].Timestamp <= pts[i-1].Timestamp {
			t.Fatalf("not increasing: %+v", pts)
		}
	}
}

func TestPlanSampleTimesLongTakeAndCap(t *testing.T) {
	pts := planSampleTimes([]Shot{{Index: 1, Start: 0, End: 190}}, 96)
	if len(pts) < 50 || len(pts) > 96 {
		t.Fatalf("190s single take should be densely sampled, got %d", len(pts))
	}
	if pts[0].Timestamp > 0.2 || pts[len(pts)-1].Timestamp < 189 {
		t.Fatalf("missing ends: first=%.3f last=%.3f", pts[0].Timestamp, pts[len(pts)-1].Timestamp)
	}
	// 80 two-second shots would want 160 first+last; budget keeps at least one each, then tails
	var many []Shot
	for i := 0; i < 80; i++ {
		many = append(many, Shot{Index: i + 1, Start: float64(i) * 2, End: float64(i+1) * 2})
	}
	capped := planSampleTimes(many, 96)
	if len(capped) > 96 {
		t.Fatalf("over budget: %d", len(capped))
	}
	seen := map[int]bool{}
	for _, p := range capped {
		seen[p.Shot] = true
	}
	if len(seen) != 80 {
		t.Fatalf("should keep at least one frame per shot, covered %d/80", len(seen))
	}
}

func TestCompareLuma(t *testing.T) {
	a := make([]uint8, 100)
	b := make([]uint8, 100)
	f, m := compareLuma(a, b)
	if f != 0 || m != 0 {
		t.Fatalf("identical: %v %v", f, m)
	}
	for i := 0; i < 50; i++ {
		b[i] = 40
	}
	f, m = compareLuma(a, b)
	if f < 0.4 || f > 0.6 {
		t.Fatalf("half changed frac=%v mean=%v", f, m)
	}
}
