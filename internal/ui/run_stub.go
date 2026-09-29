//go:build !fyne_gui

package ui

import (
	"github.com/narratoai/narrato-go/internal/webui"
)

// Run starts the default browser-based UI (no CGO / no MinGW required).
func Run() {
	webui.Run()
}
