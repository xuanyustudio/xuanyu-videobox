//go:build fyne_gui

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// scrollSelect is a drop-down whose popup is height-capped and scrolls,
// unlike widget.Select, which sizes its menu to every option.
type scrollSelect struct {
	widget.BaseWidget
	Options     []string
	Selected    string
	PlaceHolder string
	OnChanged   func(string)

	button *widget.Button
	pop    *widget.PopUp
}

func newScrollSelect(options []string, changed func(string)) *scrollSelect {
	s := &scrollSelect{
		Options:     options,
		OnChanged:   changed,
		PlaceHolder: "请选择",
	}
	s.ExtendBaseWidget(s)
	s.button = widget.NewButton(s.displayText(), s.togglePop)
	s.button.Alignment = widget.ButtonAlignLeading
	s.button.Importance = widget.MediumImportance
	return s
}

func (s *scrollSelect) displayText() string {
	if s.Selected != "" {
		return s.Selected
	}
	if s.PlaceHolder != "" {
		return s.PlaceHolder
	}
	return "请选择"
}

func (s *scrollSelect) syncButton() {
	if s.button != nil {
		s.button.SetText(s.displayText())
	}
}

func (s *scrollSelect) CreateRenderer() fyne.WidgetRenderer {
	s.ExtendBaseWidget(s)
	if s.button == nil {
		s.button = widget.NewButton(s.displayText(), s.togglePop)
		s.button.Alignment = widget.ButtonAlignLeading
		s.button.Importance = widget.MediumImportance
	}
	s.syncButton()
	return widget.NewSimpleRenderer(s.button)
}

func (s *scrollSelect) SetSelected(text string) {
	changed := s.Selected != text
	s.Selected = text
	s.syncButton()
	s.Refresh()
	if changed && s.OnChanged != nil && text != "" {
		s.OnChanged(text)
	}
}

func (s *scrollSelect) ClearSelected() {
	s.Selected = ""
	s.syncButton()
	s.Refresh()
}

func (s *scrollSelect) hidePop() {
	if s.pop != nil {
		s.pop.Hide()
		s.pop = nil
	}
}

func (s *scrollSelect) togglePop() {
	if s.pop != nil {
		s.hidePop()
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(s)
	if c == nil {
		return
	}

	opts := append([]string(nil), s.Options...)
	list := widget.NewList(
		func() int { return len(opts) },
		func() fyne.CanvasObject {
			lab := widget.NewLabel("")
			lab.Truncation = fyne.TextTruncateEllipsis
			return lab
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= 0 && id < len(opts) {
				obj.(*widget.Label).SetText(opts[id])
			}
		},
	)
	for i, o := range opts {
		if o == s.Selected {
			list.Select(i)
			break
		}
	}
	list.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(opts) {
			s.SetSelected(opts[id])
		}
		s.hidePop()
	}

	itemH := float32(36)
	count := float32(len(opts))
	if count < 1 {
		count = 1
	}
	h := itemH*count + 8
	maxH := float32(280)
	if cv := c.Size().Height; cv > 0 {
		if half := cv * 0.5; half > 0 && half < maxH {
			maxH = half
		}
		if maxH < 140 {
			maxH = 140
		}
	}
	if h > maxH {
		h = maxH
	}
	w := s.Size().Width
	if w < 260 {
		w = 260
	}

	pop := widget.NewPopUp(list, c)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(s)
	pop.ShowAtPosition(pos.Add(fyne.NewPos(0, s.Size().Height)))
	pop.Resize(fyne.NewSize(w, h))
	s.pop = pop
}

func (s *scrollSelect) Hide() {
	s.hidePop()
	s.BaseWidget.Hide()
}

func (s *scrollSelect) Refresh() {
	s.syncButton()
	s.BaseWidget.Refresh()
}
