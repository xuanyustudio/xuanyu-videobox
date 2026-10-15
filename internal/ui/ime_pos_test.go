package ui

import "testing"

func TestVisibleCaret(t *testing.T) {
	x, y := visibleCaret(12, 18, 0, 0, 200, 80, 16)
	if x != 12 || y != 18 {
		t.Fatalf("unscrolled got %v,%v", x, y)
	}
	x, y = visibleCaret(40, 240, 0, 200, 200, 80, 16)
	if x != 40 || y != 40 {
		t.Fatalf("scrolled got %v,%v", x, y)
	}
	x, y = visibleCaret(10, 4, 30, 20, 100, 40, 16)
	if x != 0 || y != 0 {
		t.Fatalf("clamped origin got %v,%v", x, y)
	}
	x, y = visibleCaret(500, 90, 0, 0, 120, 40, 16)
	if x != 120 || y != 24 {
		t.Fatalf("clamped far got %v,%v", x, y)
	}
}
