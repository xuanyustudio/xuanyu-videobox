package reverse

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const npmPackage = "@earendil-works/pi-coding-agent"

// DataDir is the isolated Pi agent directory used by Narrato (models.json, skills, sessions).
func DataDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "Narrato", "pi-agent")
	}
	return filepath.Join(os.TempDir(), "narrato-pi-agent")
}

// RuntimeDir is where the app installs the Pi npm package (no admin required).
func RuntimeDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "Narrato", "pi-runtime")
	}
	return filepath.Join(os.TempDir(), "narrato-pi-runtime")
}

func piBinName() string {
	if runtime.GOOS == "windows" {
		return "pi.cmd"
	}
	return "pi"
}

// LookPath finds an existing Pi binary: app install, then PATH.
func LookPath() string {
	local := filepath.Join(RuntimeDir(), "node_modules", ".bin", piBinName())
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local
	}
	if p, err := exec.LookPath("pi"); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("pi.cmd"); err == nil {
			return p
		}
	}
	return ""
}

// Status describes whether Pi is available.
type Status struct {
	Installed bool
	Path      string
	Version   string
	Message   string
}

func Probe(ctx context.Context) Status {
	bin := LookPath()
	if bin == "" {
		return Status{Message: "尚未安装 Pi Agent。将使用「设置」里的视觉模型，安装到本机用户目录，不需要管理员权限。"}
	}
	s := Status{Installed: true, Path: bin, Message: "Pi Agent 已就绪"}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "--version")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err == nil {
		s.Version = strings.TrimSpace(string(out))
		if s.Version != "" {
			s.Message = "Pi Agent " + s.Version
		}
	}
	return s
}

// Install installs Pi into the app runtime directory via npm.
func Install(ctx context.Context, log func(string)) error {
	if log == nil {
		log = func(string) {}
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		if runtime.GOOS == "windows" {
			if p, e := exec.LookPath("npm.cmd"); e == nil {
				npm = p
				err = nil
			}
		}
	}
	if err != nil {
		return errors.New("未找到 npm。请先安装 Node.js 22 或更高版本，然后再点「安装 Pi Agent」")
	}
	dir := RuntimeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}
	log("正在安装 Pi Agent 到 " + dir)
	cmd := exec.CommandContext(ctx, npm, "install", "--prefix", dir, "--ignore-scripts", "--engine-strict=false", npmPackage)
	hideWindow(cmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		log(strings.TrimSpace(string(out)))
	}
	if err != nil {
		return fmt.Errorf("安装 Pi Agent 失败: %w", err)
	}
	if LookPath() == "" {
		return fmt.Errorf("安装完成但找不到 pi 可执行文件，请检查 %s", dir)
	}
	log("Pi Agent 安装完成")
	return nil
}
