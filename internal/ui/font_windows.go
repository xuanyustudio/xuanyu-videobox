//go:build fyne_gui && windows

package ui

import (
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

// loadSystemFonts picks sharp Windows CJK TTF fonts.
// Note: .ttc collections (msyh.ttc) are rejected by go-text ("collections not allowed")
// and would crash Fyne at startup — only use .ttf/.otf.
func loadSystemFonts() (regular, bold fyne.Resource) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	fonts := filepath.Join(windir, "Fonts")

	// Prefer clear UI-friendly single-file TTFs. Avoid *.ttc.
	regular = firstFontResource(
		filepath.Join(fonts, "simhei.ttf"),     // 黑体
		filepath.Join(fonts, "simsunb.ttf"),    // 宋体 Bold as regular fallback
		filepath.Join(fonts, "SimsunExtG.ttf"), // 扩展宋体
		filepath.Join(fonts, "segoeui.ttf"),    // Latin UI fallback
	)
	bold = firstFontResource(
		filepath.Join(fonts, "simhei.ttf"),
		filepath.Join(fonts, "simsunb.ttf"),
		filepath.Join(fonts, "segoeuib.ttf"),
	)
	if bold == nil {
		bold = regular
	}
	return regular, bold
}

func firstFontResource(paths ...string) fyne.Resource {
	for _, p := range paths {
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".ttc" {
			continue // Fyne/go-text does not support font collections
		}
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		res, err := fyne.LoadResourceFromPath(p)
		if err != nil {
			continue
		}
		return res
	}
	return nil
}

// preferSystemFontEnv hints Fyne's built-in theme loader before app start.
func preferSystemFontEnv() {
	if os.Getenv("FYNE_FONT") != "" {
		ext := strings.ToLower(filepath.Ext(os.Getenv("FYNE_FONT")))
		if ext != ".ttc" {
			return
		}
		// Clear invalid TTC path left from a previous broken build/session.
		_ = os.Unsetenv("FYNE_FONT")
	}
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	for _, name := range []string{"simhei.ttf", "simsunb.ttf", "SimsunExtG.ttf"} {
		p := filepath.Join(windir, "Fonts", name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			_ = os.Setenv("FYNE_FONT", p)
			return
		}
	}
}
