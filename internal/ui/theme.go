//go:build fyne_gui

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// narratoTheme is a cool desk palette: one teal accent, 8px corners, light and dark.
type narratoTheme struct {
	base          fyne.Theme
	regular, bold fyne.Resource
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
	table := lightColors
	if variant == theme.VariantDark {
		table = darkColors
	}
	if c, ok := table[name]; ok {
		return c
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
		return 8
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameLineSpacing:
		return 4
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInputRadius, theme.SizeNameButtonRadius, theme.SizeNameSelectionRadius,
		theme.SizeNameMenuRadius, theme.SizeNamePopupRadius, theme.SizeNameInnerWindowRadius:
		return 8
	case theme.SizeNameCardRadius, theme.SizeNameDialogRadius:
		return 10
	case theme.SizeNameScrollBar:
		return 8
	case theme.SizeNameScrollBarSmall:
		return 3
	case theme.SizeNameScrollBarRadius:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return t.base.Size(name)
}

// Accent is deep enough that near-white label text stays readable on primary buttons.
var (
	accent     = color.NRGBA{R: 0x0e, G: 0x6b, B: 0x64, A: 0xff}
	accentText = color.NRGBA{R: 0xf3, G: 0xfb, B: 0xfa, A: 0xff}
)

var lightColors = map[fyne.ThemeColorName]color.Color{
	theme.ColorNamePrimary:                   accent,
	theme.ColorNameHyperlink:                 accent,
	theme.ColorNameForegroundOnPrimary:       accentText,
	theme.ColorNameForeground:                color.NRGBA{R: 0x1a, G: 0x22, B: 0x2c, A: 0xff},
	theme.ColorNameBackground:                color.NRGBA{R: 0xe7, G: 0xed, B: 0xf2, A: 0xff},
	theme.ColorNameHeaderBackground:          color.NRGBA{R: 0xf4, G: 0xf7, B: 0xfa, A: 0xff},
	theme.ColorNameMenuBackground:            color.NRGBA{R: 0xf4, G: 0xf7, B: 0xfa, A: 0xff},
	theme.ColorNameOverlayBackground:         color.NRGBA{R: 0xf4, G: 0xf7, B: 0xfa, A: 0xf4},
	theme.ColorNameInputBackground:           color.NRGBA{R: 0xfb, G: 0xfc, B: 0xfd, A: 0xff},
	theme.ColorNameButton:                    color.NRGBA{R: 0xe1, G: 0xe7, B: 0xee, A: 0xff},
	theme.ColorNameDisabledButton:            color.NRGBA{R: 0xe1, G: 0xe7, B: 0xee, A: 0xff},
	theme.ColorNameInputBorder:               color.NRGBA{R: 0xc5, G: 0xd0, B: 0xda, A: 0xff},
	theme.ColorNameSeparator:                 color.NRGBA{R: 0xd3, G: 0xdc, B: 0xe4, A: 0xff},
	theme.ColorNamePlaceHolder:               color.NRGBA{R: 0x5e, G: 0x6c, B: 0x7a, A: 0xff},
	theme.ColorNameDisabled:                  color.NRGBA{R: 0x5e, G: 0x6c, B: 0x7a, A: 0xff},
	theme.ColorNameHover:                     color.NRGBA{R: 0x0e, G: 0x6b, B: 0x64, A: 0x38},
	theme.ColorNamePressed:                   color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x28},
	theme.ColorNameFocus:                     color.NRGBA{R: 0x0e, G: 0x6b, B: 0x64, A: 0x55},
	theme.ColorNameSelection:                 color.NRGBA{R: 0x0e, G: 0x6b, B: 0x64, A: 0x2a},
	theme.ColorNameScrollBar:                 color.NRGBA{R: 0xb7, G: 0xc3, B: 0xcf, A: 0xff},
	theme.ColorNameScrollBarBackground:       color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00},
	theme.ColorNameShadow:                    color.NRGBA{R: 0x1a, G: 0x22, B: 0x2c, A: 0x14},
	theme.ColorNameInnerWindowBorder:         color.NRGBA{R: 0xd3, G: 0xdc, B: 0xe4, A: 0xff},
	theme.ColorNameInnerWindowBorderInactive: color.NRGBA{R: 0xd3, G: 0xdc, B: 0xe4, A: 0x88},
	theme.ColorNameSuccess:                   color.NRGBA{R: 0x15, G: 0x72, B: 0x4a, A: 0xff},
	theme.ColorNameForegroundOnSuccess:       color.NRGBA{R: 0xf3, G: 0xfb, B: 0xf6, A: 0xff},
	theme.ColorNameWarning:                   color.NRGBA{R: 0x8f, G: 0x54, B: 0x10, A: 0xff},
	theme.ColorNameForegroundOnWarning:       color.NRGBA{R: 0xff, G: 0xf8, B: 0xf1, A: 0xff},
	theme.ColorNameError:                     color.NRGBA{R: 0xb4, G: 0x33, B: 0x2a, A: 0xff},
	theme.ColorNameForegroundOnError:         color.NRGBA{R: 0xff, G: 0xf6, B: 0xf5, A: 0xff},
}

var darkColors = map[fyne.ThemeColorName]color.Color{
	theme.ColorNamePrimary:                   accent,
	theme.ColorNameHyperlink:                 color.NRGBA{R: 0x8f, G: 0xd9, B: 0xd2, A: 0xff},
	theme.ColorNameForegroundOnPrimary:       accentText,
	theme.ColorNameForeground:                color.NRGBA{R: 0xe7, G: 0xee, B: 0xf4, A: 0xff},
	theme.ColorNameBackground:                color.NRGBA{R: 0x12, G: 0x17, B: 0x1d, A: 0xff},
	theme.ColorNameHeaderBackground:          color.NRGBA{R: 0x1b, G: 0x22, B: 0x2b, A: 0xff},
	theme.ColorNameMenuBackground:            color.NRGBA{R: 0x1b, G: 0x22, B: 0x2b, A: 0xff},
	theme.ColorNameOverlayBackground:         color.NRGBA{R: 0x1b, G: 0x22, B: 0x2b, A: 0xf2},
	theme.ColorNameInputBackground:           color.NRGBA{R: 0x10, G: 0x15, B: 0x1a, A: 0xff},
	theme.ColorNameButton:                    color.NRGBA{R: 0x2a, G: 0x33, B: 0x3e, A: 0xff},
	theme.ColorNameDisabledButton:            color.NRGBA{R: 0x2a, G: 0x33, B: 0x3e, A: 0xff},
	theme.ColorNameInputBorder:               color.NRGBA{R: 0x3a, G: 0x46, B: 0x54, A: 0xff},
	theme.ColorNameSeparator:                 color.NRGBA{R: 0x2e, G: 0x38, B: 0x44, A: 0xff},
	theme.ColorNamePlaceHolder:               color.NRGBA{R: 0x93, G: 0xa0, B: 0xae, A: 0xff},
	theme.ColorNameDisabled:                  color.NRGBA{R: 0x93, G: 0xa0, B: 0xae, A: 0xff},
	theme.ColorNameHover:                     color.NRGBA{R: 0x8f, G: 0xd9, B: 0xd2, A: 0x30},
	theme.ColorNamePressed:                   color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x22},
	theme.ColorNameFocus:                     color.NRGBA{R: 0x8f, G: 0xd9, B: 0xd2, A: 0x55},
	theme.ColorNameSelection:                 color.NRGBA{R: 0x0e, G: 0x6b, B: 0x64, A: 0x55},
	theme.ColorNameScrollBar:                 color.NRGBA{R: 0x4a, G: 0x58, B: 0x68, A: 0xff},
	theme.ColorNameScrollBarBackground:       color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00},
	theme.ColorNameShadow:                    color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x44},
	theme.ColorNameInnerWindowBorder:         color.NRGBA{R: 0x2e, G: 0x38, B: 0x44, A: 0xff},
	theme.ColorNameInnerWindowBorderInactive: color.NRGBA{R: 0x2e, G: 0x38, B: 0x44, A: 0x88},
	theme.ColorNameSuccess:                   color.NRGBA{R: 0x7d, G: 0xce, B: 0xa4, A: 0xff},
	theme.ColorNameForegroundOnSuccess:       color.NRGBA{R: 0x0e, G: 0x24, B: 0x18, A: 0xff},
	theme.ColorNameWarning:                   color.NRGBA{R: 0xe2, G: 0xb1, B: 0x5a, A: 0xff},
	theme.ColorNameForegroundOnWarning:       color.NRGBA{R: 0x24, G: 0x18, B: 0x06, A: 0xff},
	theme.ColorNameError:                     color.NRGBA{R: 0xf0, G: 0xa2, B: 0x9a, A: 0xff},
	theme.ColorNameForegroundOnError:         color.NRGBA{R: 0x2a, G: 0x12, B: 0x10, A: 0xff},
}
