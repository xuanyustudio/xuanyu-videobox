//go:build !windows

package reverse

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
