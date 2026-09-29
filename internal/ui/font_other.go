//go:build fyne_gui && !windows

package ui

import "fyne.io/fyne/v2"

func loadSystemFonts() (regular, bold fyne.Resource) {
	return nil, nil
}

func preferSystemFontEnv() {}
