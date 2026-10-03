//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/media"
)

func defaultDownloadDir(cfg *config.Config) string {
	root := cfg.Paths.ProjectsRoot
	if root == "" {
		root = config.Default().Paths.ProjectsRoot
	}
	return root
}

func newLinkDownloadPage(state *AppState) fyne.CanvasObject {
	var (
		mu        sync.Mutex
		busy      bool
		cancelFn  context.CancelFunc
		lastDir   string
	)

	subtitle := widget.NewLabel("粘贴视频分享链接 → 解析 → 下载无水印视频\n支持：抖音、快手、西瓜视频、微博、皮皮虾、好看视频、皮皮搞笑、梨视频")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	urlEntry := widget.NewMultiLineEntry()
	urlEntry.SetPlaceHolder("每行一个链接，支持分享文本（自动提取URL）\n例如：https://v.douyin.com/xxxxx/\n或：https://www.douyin.com/video/123456")
	urlEntry.SetMinRowsVisible(4)
	urlEntry.Wrapping = fyne.TextWrapWord

	outDirEntry := widget.NewEntry()
	outDirEntry.SetPlaceHolder("下载保存目录（留空使用工程根目录）")
	outDirEntry.SetText(defaultDownloadDir(state.Cfg))

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100

	logEntry := widget.NewMultiLineEntry()
	logEntry.SetMinRowsVisible(8)
	logEntry.Wrapping = fyne.TextWrapWord
	logEntry.Disable()

	appendLog := func(line string) {
		fyne.Do(func() {
			logEntry.SetText(logEntry.Text + line + "\n")
		})
	}

	var startBtn, cancelBtn, openOutBtn *widget.Button

	openOutBtn = widget.NewButton("打开输出目录", func() {
		mu.Lock()
		dir := lastDir
		mu.Unlock()
		if dir != "" {
			_ = media.OpenFile(dir)
		}
	})
	openOutBtn.Disable()

	setBusy := func(v bool) {
		busy = v
		if v {
			startBtn.Disable()
			cancelBtn.Enable()
			openOutBtn.Disable()
		} else {
			startBtn.Enable()
			cancelBtn.Disable()
		}
	}

	doDownload := func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		setBusy(true)
		mu.Unlock()

		lines := strings.Split(urlEntry.Text, "\n")
		var urls []string
		for _, line := range lines {
			u := strings.TrimSpace(line)
			if u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) == 0 {
			fyne.Do(func() {
				mu.Lock()
				setBusy(false)
				mu.Unlock()
				status.SetText("请输入至少一个链接")
			})
			return
		}

		outDir := strings.TrimSpace(outDirEntry.Text)
		if outDir == "" {
			outDir = defaultDownloadDir(state.Cfg)
		}

		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		cancelFn = cancel
		mu.Unlock()

		logEntry.SetText("")
		progress.SetValue(0)
		status.SetText(fmt.Sprintf("开始处理 %d 个链接…", len(urls)))

		go func() {
			defer func() {
				mu.Lock()
				cancelFn = nil
				setBusy(false)
				mu.Unlock()
			}()

			success := 0
			for i, rawURL := range urls {
				if ctx.Err() != nil {
					appendLog("已取消")
					break
				}

				tag := fmt.Sprintf("[%d/%d]", i+1, len(urls))
				appendLog(fmt.Sprintf("%s 解析: %s", tag, rawURL))
				fyne.Do(func() {
					status.SetText(fmt.Sprintf("%s 解析中…", tag))
					progress.SetValue(float64(i) / float64(len(urls)) * 100)
				})

				info, err := linkdl.DetectAndParse(ctx, rawURL)
				if err != nil {
					appendLog(fmt.Sprintf("%s 解析失败: %s", tag, err))
					continue
				}
				appendLog(fmt.Sprintf("%s [%s] %s — %s", tag, info.Platform, info.Title, info.Author))

				if info.VideoURL == "" {
					appendLog(fmt.Sprintf("%s 无视频地址（图集？已跳过）", tag))
					continue
				}

				fyne.Do(func() { status.SetText(fmt.Sprintf("%s 下载中…", tag)) })
				path, err := linkdl.Download(ctx, info, outDir, func(p linkdl.Progress) {
					fyne.Do(func() {
						base := float64(i) / float64(len(urls)) * 100
						step := float64(p.Percent) / float64(len(urls))
						progress.SetValue(base + step)
					})
				})
				if err != nil {
					appendLog(fmt.Sprintf("%s 下载失败: %s", tag, err))
					continue
				}
				appendLog(fmt.Sprintf("%s 已保存: %s", tag, path))
				success++
				mu.Lock()
				lastDir = outDir
				mu.Unlock()
			}

			fyne.Do(func() {
				progress.SetValue(100)
				status.SetText(fmt.Sprintf("完成: %d/%d 成功", success, len(urls)))
				mu.Lock()
				if lastDir != "" {
					openOutBtn.Enable()
				}
				mu.Unlock()
			})
		}()
	}

	startBtn = widget.NewButton("解析并下载", func() { doDownload() })
	startBtn.Importance = widget.HighImportance

	cancelBtn = widget.NewButton("取消", func() {
		mu.Lock()
		if cancelFn != nil {
			cancelFn()
		}
		mu.Unlock()
	})
	cancelBtn.Disable()

	clearBtn := widget.NewButton("清空", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		urlEntry.SetText("")
		logEntry.SetText("")
		status.SetText("")
		progress.SetValue(0)
		openOutBtn.Disable()
	})
	clearBtn.Importance = widget.LowImportance

	pasteBtn := widget.NewButton("粘贴", func() {
		clip := fyne.CurrentApp().Clipboard().Content()
		if clip != "" {
			cur := urlEntry.Text
			if cur != "" && !strings.HasSuffix(cur, "\n") {
				cur += "\n"
			}
			urlEntry.SetText(cur + clip)
		}
	})
	pasteBtn.Importance = widget.LowImportance

	sec := func(text string) *widget.Label {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}

	toolbar := container.NewHBox(pasteBtn, startBtn, cancelBtn, openOutBtn, clearBtn)

	body := container.NewVBox(
		subtitle,
		sec("视频链接"),
		urlEntry,
		container.NewBorder(nil, nil, widget.NewLabel("保存目录"), nil, outDirEntry),
		toolbar,
		progress,
		status,
		sec("日志"),
		logEntry,
	)
	return container.NewVScroll(body)
}
