package ui

// visibleCaret is the cursor position inside a widget after scroll, clamped to the visible box.
func visibleCaret(cursorX, cursorY, scrollX, scrollY, width, height, line float32) (x, y float32) {
	x = cursorX - scrollX
	y = cursorY - scrollY
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if width > 0 && x > width {
		x = width
	}
	maxY := height - line
	if maxY < 0 {
		maxY = 0
	}
	if height > 0 && y > maxY {
		y = maxY
	}
	return x, y
}
