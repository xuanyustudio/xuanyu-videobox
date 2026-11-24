package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// ErrFFplayMissing is returned when preview cannot start because ffplay is not installed.
var ErrFFplayMissing = fmt.Errorf("未找到 ffplay，无法内置预听。请把 ffplay.exe 放到 ffmpeg 同一目录，或加入系统 PATH")

var (
	playMu  sync.Mutex
	playCmd *exec.Cmd
)

// PlayAudio starts built-in playback of path with ffplay (no extra player window).
// A new call stops any previous preview first.
func PlayAudio(path string) error {
	return PlayAudioConfigured(path, "")
}

// PlayAudioConfigured looks for ffplay next to ffmpegPath, then next to this executable, then PATH.
func PlayAudioConfigured(path, ffmpegPath string) error {
	beside := ""
	if exe, err := os.Executable(); err == nil {
		beside = filepath.Dir(exe)
	}
	return PlayAudioWith(path, ffmpegPath, beside)
}

// PlayAudioWith is PlayAudio, also looking for ffplay next to ffmpegPath or besideDir.
func PlayAudioWith(path, ffmpegPath, besideDir string) error {
	StopAudio()
	if path == "" {
		return fmt.Errorf("没有可预听的音频文件")
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("预听音频不存在: %w", err)
	}
	if st.IsDir() || st.Size() == 0 {
		return fmt.Errorf("预听音频无效或为空")
	}
	bin, err := ResolveFFplay(ffmpegPath, besideDir)
	if err != nil {
		return ErrFFplayMissing
	}
	cmd := exec.Command(absCommand(bin), "-nodisp", "-autoexit", "-loglevel", "quiet", path)
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动内置预听失败: %w", err)
	}
	playMu.Lock()
	playCmd = cmd
	playMu.Unlock()
	go func() {
		_ = cmd.Wait()
		playMu.Lock()
		if playCmd == cmd {
			playCmd = nil
		}
		playMu.Unlock()
	}()
	return nil
}

// StopAudio stops the current built-in preview, if any.
func StopAudio() {
	playMu.Lock()
	cmd := playCmd
	playCmd = nil
	playMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		go func() { _ = cmd.Wait() }()
	}
}

// AudioPlaying reports whether a built-in preview is still running.
func AudioPlaying() bool {
	playMu.Lock()
	defer playMu.Unlock()
	return playCmd != nil
}
