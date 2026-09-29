//go:build fyne_gui

package ui

import "fyne.io/fyne/v2"

// fixedWidthLayout forces children to a fixed width (used for short sliders).
type fixedWidthLayout struct {
	width float32
}

func (f *fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	h := size.Height
	if h < objects[0].MinSize().Height {
		h = objects[0].MinSize().Height
	}
	objects[0].Resize(fyne.NewSize(f.width, h))
	objects[0].Move(fyne.NewPos(0, (size.Height-h)/2))
}

func (f *fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	if len(objects) > 0 {
		h = objects[0].MinSize().Height
	}
	return fyne.NewSize(f.width, h)
}
