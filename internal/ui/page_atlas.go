//go:build fyne_gui

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/original"
)

func newAtlasPage() fyne.CanvasObject {
	header := widget.NewLabel(original.AtlasHeader)
	header.Wrapping = fyne.TextWrapWord
	header.Importance = widget.MediumImportance

	sections := make([]fyne.CanvasObject, 0, 8)
	sections = append(sections,
		widget.NewLabelWithStyle("手法图鉴", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		header,
		widget.NewSeparator(),
	)

	for _, layer := range original.AtlasLayers() {
		title := widget.NewLabelWithStyle(layer.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		intro := widget.NewLabel(layer.Intro)
		intro.Wrapping = fyne.TextWrapWord
		intro.Importance = widget.LowImportance
		sections = append(sections, title, intro)

		for _, e := range layer.Entries {
			badge := "市场说法"
			if e.InTool {
				badge = "本工具可做"
			}
			name := widget.NewLabelWithStyle(e.Name+"  ·  "+badge, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			feel := widget.NewLabel("观感: " + e.Feel)
			feel.Wrapping = fyne.TextWrapWord
			note := widget.NewLabel("说明: " + e.Note)
			note.Wrapping = fyne.TextWrapWord
			note.Importance = widget.LowImportance
			sections = append(sections, name, feel, note, widget.NewSeparator())
		}
	}

	return container.NewVScroll(container.NewPadded(container.NewVBox(sections...)))
}
