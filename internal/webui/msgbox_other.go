//go:build !windows

package webui

import (
	"fmt"
	"os"
)

func fatalBox(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

func infoBox(title, text string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
}
