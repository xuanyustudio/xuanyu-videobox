package media

import (
	"bufio"
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
// A relative bin is made absolute first, so changing dir does not hide the executable.
func RunInDir(ctx context.Context, dir, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, absCommand(bin), args...)
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

// RunInDirLines runs bin and calls onLine for each stdout line.
// stderr is returned as text. A relative bin is made absolute before dir is applied.
func RunInDirLines(ctx context.Context, dir, bin string, onLine func(string), args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, absCommand(bin), args...)
	if dir != "" {
		cmd.Dir = dir
	}
	hideConsole(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return stderr.String(), err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if onLine != nil {
			onLine(scanner.Text())
		}
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		return stderr.String(), waitErr
	}
	if err := scanner.Err(); err != nil {
		return stderr.String(), err
	}
	return stderr.String(), nil
}

func absCommand(bin string) string {
	if bin == "" || filepath.IsAbs(bin) {
		return bin
	}
	if abs, err := filepath.Abs(bin); err == nil {
		return abs
	}
	return bin
}
