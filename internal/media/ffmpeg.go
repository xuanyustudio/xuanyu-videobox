package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ResolveFFmpeg locates the ffmpeg binary: config path, then beside besideDir, then PATH.
func ResolveFFmpeg(configPath, besideDir string) (string, error) {
	return resolveBinary(configPath, besideDir, "ffmpeg")
}

// ResolveFFprobe locates the ffprobe binary: config path, then beside besideDir, then PATH.
func ResolveFFprobe(configPath, besideDir string) (string, error) {
	return resolveBinary(configPath, besideDir, "ffprobe")
}

func resolveBinary(configPath, besideDir, name string) (string, error) {
	if configPath != "" {
		if st, err := os.Stat(configPath); err == nil && !st.IsDir() {
			return configPath, nil
		}
	}
	if besideDir != "" {
		for _, base := range besideCandidateNames(name) {
			candidate := filepath.Join(besideDir, base)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}
	return lookPathBinary(name)
}

func besideCandidateNames(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{name + ".exe"}
	}
	return []string{name, name + ".exe"}
}

func lookPathBinary(name string) (string, error) {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath(name + ".exe"); err == nil {
			return p, nil
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return p, nil
}

// Run executes bin with args and returns combined stdout and stderr.
func Run(ctx context.Context, bin string, args ...string) (string, error) {
	return RunInDir(ctx, "", bin, args...)
}

// RunInDir is like Run but sets the process working directory when dir is non-empty.
func RunInDir(ctx context.Context, dir, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	hideConsole(cmd)
	err := cmd.Run()
	return combined.String(), err
}
