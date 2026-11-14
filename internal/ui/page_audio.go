//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
)

func newAudioSplitPage(state *AppState) fyne.CanvasObject {
	var (
		mu      sync.Mutex
		busy    bool
		cancel  context.CancelFunc
		videos  []string
		lastOut string
	)

	subtitle := widget.NewLabel("选择视频，抽出音轨，保存为同目录下的 MP3。")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	pathLabel := widget.NewLabel("未选择视频")
	pathLabel.Wrapping = fyne.TextWrapWord
	pathLabel.Truncation = fyne.TextTruncateEllipsis

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	var pickBtn, startBtn, cancelBtn, openBtn *widget.Button

	setBusy := func(v bool) {
		busy = v
		if v {
			pickBtn.Disable()
			startBtn.Disable()
			cancelBtn.Enable()
			openBtn.Disable()
			return
		}
		pickBtn.Enable()
		startBtn.Enable()
		cancelBtn.Disable()
		if lastOut != "" {
			openBtn.Enable()
		}
	}

	pickBtn = widget.NewButton("选择视频", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		paths, err := pickVideoFiles()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if len(paths) == 0 {
			return
		}
		mu.Lock()
		videos = append([]string(nil), paths...)
		mu.Unlock()
		if len(paths) == 1 {
			pathLabel.SetText(paths[0])
		} else {
			pathLabel.SetText(fmt.Sprintf("已选 %d 个视频", len(paths)))
		}
		status.SetText("")
	})

	openBtn = widget.NewButton("打开所在文件夹", func() {
		mu.Lock()
		out := lastOut
		mu.Unlock()
		if out == "" {
			return
		}
		_ = media.OpenFile(filepath.Dir(out))
	})
	openBtn.Disable()

	cancelBtn = widget.NewButton("停止", func() {
		mu.Lock()
		fn := cancel
		mu.Unlock()
		if fn != nil {
			fn()
		}
	})
	cancelBtn.Disable()

	startBtn = widget.NewButton("分离为 MP3", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		batch := append([]string(nil), videos...)
		mu.Unlock()
		if len(batch) == 0 {
			dialog.ShowInformation("未选择视频", "请先选择要分离的视频。", state.Win)
			return
		}
		bins, err := resolveFFmpeg(state)
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		ctx, cancelFn := context.WithCancel(context.Background())
		mu.Lock()
		cancel = cancelFn
		setBusy(true)
		mu.Unlock()
		status.SetText("正在分离…")

		go func() {
			var done []string
			var fail error
			for i, video := range batch {
				fyne.Do(func() {
					name := filepath.Base(video)
					if len(batch) == 1 {
						status.SetText("正在分离 " + name + "…")
					} else {
						status.SetText(fmt.Sprintf("正在分离 %d/%d %s…", i+1, len(batch), name))
					}
				})
				out, err := media.ExtractMP3(ctx, bins, video)
				if err != nil {
					fail = err
					break
				}
				done = append(done, out)
			}
			fyne.Do(func() {
				mu.Lock()
				cancel = nil
				if len(done) > 0 {
					lastOut = done[len(done)-1]
				}
				setBusy(false)
				mu.Unlock()
				switch {
				case fail == nil:
					if len(done) == 1 {
						status.SetText("已保存 " + done[0])
					} else {
						status.SetText(fmt.Sprintf("已保存 %d 个 MP3，最后一个：%s", len(done), done[len(done)-1]))
					}
				case ctx.Err() != nil:
					if len(done) == 0 {
						status.SetText("已停止")
					} else {
						status.SetText(fmt.Sprintf("已停止，已保存 %d 个 MP3", len(done)))
					}
				default:
					if len(done) == 0 {
						status.SetText(fail.Error())
					} else {
						status.SetText(fmt.Sprintf("已保存 %d 个，随后失败：%s", len(done), fail.Error()))
					}
				}
			})
		}()
	})
	startBtn.Importance = widget.HighImportance

	return container.NewPadded(container.NewVBox(
		pageTitle("音频分离"),
		subtitle,
		container.NewHBox(pickBtn, startBtn, cancelBtn, openBtn),
		pathLabel,
		status,
	))
}

func resolveFFmpeg(state *AppState) (media.FFmpegBins, error) {
	beside := ""
	if exe, err := os.Executable(); err == nil {
		beside = filepath.Dir(exe)
	}
	ff, err := locateTool(strings.TrimSpace(state.Cfg.FFmpeg.Path), beside, "ffmpeg")
	if err != nil {
		return media.FFmpegBins{}, err
	}
	fp, _ := locateTool(strings.TrimSpace(state.Cfg.FFmpeg.FFprobePath), beside, "ffprobe")
	if fp == "" {
		if abs, err := existingAbs(filepath.Join(filepath.Dir(ff), "ffprobe.exe")); err == nil {
			fp = abs
		}
	}
	return media.FFmpegBins{FFmpeg: ff, FFprobe: fp}, nil
}

func locateTool(configured, beside, name string) (string, error) {
	if configured != "" {
		if abs, err := existingAbs(configured); err == nil {
			return abs, nil
		}
		if beside != "" {
			if abs, err := existingAbs(filepath.Join(beside, filepath.Base(configured))); err == nil {
				return abs, nil
			}
		}
		return "", fmt.Errorf("找不到 %s：%s（请在「设置」里填写完整路径）", name, configured)
	}
	var (
		found string
		err   error
	)
	if name == "ffprobe" {
		found, err = media.ResolveFFprobe("", beside)
	} else {
		found, err = media.ResolveFFmpeg("", beside)
	}
	if err != nil {
		return "", fmt.Errorf("找不到 %s：%w（请在「设置」里填写完整路径）", name, err)
	}
	if abs, err := existingAbs(found); err == nil {
		return abs, nil
	}
	return found, nil
}

func existingAbs(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("%s 是目录", abs)
	}
	return abs, nil
}
