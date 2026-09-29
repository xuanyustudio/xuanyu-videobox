//go:build fyne_gui

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// narratoTheme softens default Fyne chrome with a cool editorial palette.
type narratoTheme struct {
	base           fyne.Theme
	regular, bold  fyne.Resource
}

func newNarratoTheme() fyne.Theme {
	reg, bold := loadSystemFonts()
	return &narratoTheme{
		base:    theme.DefaultTheme(),
		regular: reg,
		bold:    bold,
	}
}

func (t *narratoTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0x1c, G: 0x24, B: 0x30, A: 0xff}
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0xf3, G: 0xf5, B: 0xf8, A: 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0xe2, G: 0xe8, B: 0xf0, A: 0xff}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x8a, G: 0x96, B: 0xa6, A: 0xff}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0xb0, G: 0xb8, B: 0xc4, A: 0xff}
	case theme.ColorNameHover:
		// Fyne blends this OVER the button fill — use a strong translucent blue wash.
		return color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0x55}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x33}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0x40}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0x44}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0xd4, G: 0xdb, B: 0xe4, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x18}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xf2}
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x1f, G: 0x8a, B: 0x5b, A: 0xff}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xc4, G: 0x7a, B: 0x11, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xc0, G: 0x39, B: 0x2b, A: 0xff}
	}
	return t.base.Color(name, variant)
}

func (t *narratoTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace || style.Symbol {
		return t.base.Font(style)
	}
	if style.Bold && t.bold != nil {
		return t.bold
	}
	if t.regular != nil {
		return t.regular
	}
	return t.base.Font(style)
}

func (t *narratoTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *narratoTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameLineSpacing:
		return 2
	case theme.SizeNameText:
		// Integer DIP size reduces blur vs fractional rasterization.
		return 14
	case theme.SizeNameHeadingText:
		return 16
	case theme.SizeNameSubHeadingText:
		return 14
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameScrollBar:
		return 8 // ~文字量级，避免滚动条过粗
	case theme.SizeNameScrollBarSmall:
		return 3
	}
	return t.base.Size(name)
}
