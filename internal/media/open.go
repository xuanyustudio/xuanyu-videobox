package media

import (
	"os/exec"
	"runtime"
)

// OpenFile opens path with the system default application without a console flash.
func OpenFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	hideConsole(cmd)
	return cmd.Start()
}
