//go:build fyne_gui

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
)

func currentColor(name fyne.ThemeColorName) color.Color {
	app := fyne.CurrentApp()
	if app == nil {
		return color.NRGBA{R: 0x1a, G: 0x22, B: 0x2c, A: 0xff}
	}
	return app.Settings().Theme().Color(name, app.Settings().ThemeVariant())
}

func pageTitle(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameHeadingText
	return l
}

func sectionTitle(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameSubHeadingText
	return l
}

// fill paints a theme color and refreshes when the variant changes.
type fill struct {
	widget.BaseWidget
	name fyne.ThemeColorName
}

func newFill(name fyne.ThemeColorName) *fill {
	f := &fill{name: name}
	f.ExtendBaseWidget(f)
	return f
}

func (f *fill) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	r := &fillRenderer{fill: f, rect: rect}
	r.Refresh()
	return r
}

type fillRenderer struct {
	fill *fill
	rect *canvas.Rectangle
}

func (r *fillRenderer) Layout(size fyne.Size) { r.rect.Resize(size) }
func (r *fillRenderer) MinSize() fyne.Size    { return fyne.NewSize(0, 0) }
func (r *fillRenderer) Refresh() {
	r.rect.FillColor = currentColor(r.fill.name)
	r.rect.Refresh()
}
func (r *fillRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.rect} }
func (r *fillRenderer) Destroy()                     {}

// panel is a rounded surface used for grouped settings and project cards' kin.
type panel struct {
	widget.BaseWidget
	body fyne.CanvasObject
}

func newPanel(body fyne.CanvasObject) *panel {
	p := &panel{body: body}
	p.ExtendBaseWidget(p)
	return p
}

func (p *panel) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	stroke := canvas.NewRectangle(color.Transparent)
	stroke.StrokeWidth = 1
	r := &panelRenderer{
		panel:  p,
		bg:     bg,
		stroke: stroke,
		inner:  container.NewPadded(p.body),
	}
	r.Refresh()
	return r
}

type panelRenderer struct {
	panel  *panel
	bg     *canvas.Rectangle
	stroke *canvas.Rectangle
	inner  *fyne.Container
}

func (r *panelRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.stroke.Resize(size)
	r.inner.Resize(size)
}

func (r *panelRenderer) MinSize() fyne.Size { return r.inner.MinSize() }

func (r *panelRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	radius := th.Size(theme.SizeNameCardRadius)
	r.bg.FillColor = th.Color(theme.ColorNameHeaderBackground, v)
	r.bg.CornerRadius = radius
	r.stroke.FillColor = color.Transparent
	r.stroke.StrokeColor = th.Color(theme.ColorNameSeparator, v)
	r.stroke.CornerRadius = radius
	r.bg.Refresh()
	r.stroke.Refresh()
}

func (r *panelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.stroke, r.inner}
}

func (r *panelRenderer) Destroy() {}

type railItem struct {
	Label string
	Icon  fyne.Resource
	Page  fyne.CanvasObject
}

// newRail builds the left navigation. The last item is pinned to the bottom.
func newRail(items []railItem) (rail, content fyne.CanvasObject) {
	holder := container.NewStack()
	if len(items) == 0 {
		return container.NewStack(), holder
	}

	btns := make([]*widget.Button, len(items))
	pages := make([]fyne.CanvasObject, len(items))
	var selectIndex func(int)
	selectIndex = func(i int) {
		if i < 0 || i >= len(items) {
			return
		}
		media.StopAudio()
		for n, b := range btns {
			if n == i {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.LowImportance
			}
			b.Refresh()
		}
		holder.Objects = []fyne.CanvasObject{pages[i]}
		holder.Refresh()
	}
	for i := range items {
		idx := i
		pages[idx] = container.NewPadded(items[idx].Page)
		btn := widget.NewButtonWithIcon(items[idx].Label, items[idx].Icon, func() {
			selectIndex(idx)
		})
		btn.Alignment = widget.ButtonAlignLeading
		btn.Importance = widget.LowImportance
		btns[idx] = btn
	}

	mark := widget.NewLabelWithStyle("玄宇视频工具箱", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	mark.SizeName = theme.SizeNameSubHeadingText
	tag := widget.NewLabel("短剧工作台")
	tag.Importance = widget.LowImportance

	main := container.NewVBox()
	for i := 0; i < len(btns)-1; i++ {
		main.Add(btns[i])
	}
	side := container.NewBorder(
		container.NewVBox(container.NewPadded(container.NewVBox(mark, tag)), widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), btns[len(btns)-1]),
		nil, nil,
		container.NewVScroll(main),
	)
	body := container.NewBorder(nil, nil, nil, widget.NewSeparator(),
		container.NewStack(newFill(theme.ColorNameHeaderBackground), side),
	)
	selectIndex(0)
	return container.New(&fixedWidthLayout{width: 208}, body), holder
}
