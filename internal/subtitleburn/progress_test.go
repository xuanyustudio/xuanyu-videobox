package subtitleburn

import "testing"

func TestProgressPercent(t *testing.T) {
	pct, ok := progressPercent("out_time=00:00:30.000000", 60)
	if !ok || pct != 50 {
		t.Fatalf("half: %d %v", pct, ok)
	}
	pct, ok = progressPercent("progress=end", 60)
	if !ok || pct != 100 {
		t.Fatalf("end: %d %v", pct, ok)
	}
	if _, ok = progressPercent("out_time=N/A", 60); ok {
		t.Fatal("N/A should be ignored")
	}
	pct, ok = progressPercent("out_time=00:01:30.000000", 60)
	if !ok || pct != 99 {
		t.Fatalf("cap: %d %v", pct, ok)
	}
}
