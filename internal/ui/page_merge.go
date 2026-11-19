//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
)

func newMergePage(state *AppState) fyne.CanvasObject {
	var (
		mu       sync.Mutex
		busy     bool
		cancel   context.CancelFunc
		videos   []string
		selected = -1
		lastOut  string
	)

	intro := widget.NewLabel("添加要合并的视频，用上移、下移排好顺序。列表从上到下就是成片顺序。画面大小不一致时，按最大的那段对齐，其余补黑边。")
	intro.Wrapping = fyne.TextWrapWord
	intro.Importance = widget.LowImportance

	countLabel := widget.NewLabel("共 0 段")
	countLabel.Importance = widget.LowImportance

	pathLabel := widget.NewLabel("未选中。点列表里的视频后再调整顺序")
	pathLabel.Wrapping = fyne.TextWrapWord
	pathLabel.Importance = widget.LowImportance

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	var (
		addBtn    *widget.Button
		upBtn     *widget.Button
		downBtn   *widget.Button
		removeBtn *widget.Button
		clearBtn  *widget.Button
		startBtn  *widget.Button
		cancelBtn *widget.Button
		openBtn   *widget.Button
		list      *widget.List
	)

	setBusy := func(v bool) {
		busy = v
		edits := []*widget.Button{addBtn, upBtn, downBtn, removeBtn, clearBtn, startBtn}
		if v {
			for _, b := range edits {
				b.Disable()
			}
			cancelBtn.Enable()
			openBtn.Disable()
			return
		}
		for _, b := range edits {
			b.Enable()
		}
		cancelBtn.Disable()
		if lastOut != "" {
			openBtn.Enable()
		}
	}

	showSelection := func() {
		list.Refresh()
		countLabel.SetText(fmt.Sprintf("共 %d 段，从上到下就是成片顺序", len(videos)))
		if selected >= 0 && selected < len(videos) {
			pathLabel.SetText(fmt.Sprintf("%d. %s", selected+1, videos[selected]))
			list.Select(selected)
			return
		}
		selected = -1
		pathLabel.SetText("未选中。点列表里的视频后再调整顺序")
	}

	list = widget.NewList(
		func() int { return len(videos) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("视频")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if int(id) >= len(videos) {
				label.SetText("")
				return
			}
			label.SetText(fmt.Sprintf("%d.  %s", id+1, filepath.Base(videos[id])))
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		selected = int(id)
		if selected >= 0 && selected < len(videos) {
			pathLabel.SetText(fmt.Sprintf("%d. %s", selected+1, videos[selected]))
		}
	}

	addBtn = widget.NewButton("添加视频", func() {
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
		selected = len(videos)
		videos = append(videos, paths...)
		showSelection()
		status.SetText("")
	})

	upBtn = widget.NewButton("上移", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		if selected <= 0 || selected >= len(videos) {
			return
		}
		videos[selected], videos[selected-1] = videos[selected-1], videos[selected]
		selected--
		showSelection()
	})

	downBtn = widget.NewButton("下移", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		if selected < 0 || selected >= len(videos)-1 {
			return
		}
		videos[selected], videos[selected+1] = videos[selected+1], videos[selected]
		selected++
		showSelection()
	})

	removeBtn = widget.NewButton("移除", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		if selected < 0 || selected >= len(videos) {
			return
		}
		videos = append(videos[:selected], videos[selected+1:]...)
		if selected >= len(videos) {
			selected = len(videos) - 1
		}
		showSelection()
	})
	removeBtn.Importance = widget.LowImportance

	clearBtn = widget.NewButton("清空", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		videos = nil
		selected = -1
		list.UnselectAll()
		showSelection()
	})
	clearBtn.Importance = widget.LowImportance

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
	cancelBtn.Importance = widget.LowImportance
	cancelBtn.Disable()

	startBtn = widget.NewButton("开始合并", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		batch := append([]string(nil), videos...)
		mu.Unlock()
		if len(batch) < 2 {
			dialog.ShowInformation("视频不够", "请至少添加 2 个视频。", state.Win)
			return
		}
		bins, err := resolveFFmpeg(state)
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		defaultPath := filepath.Join(filepath.Dir(batch[0]), "合并.mp4")
		outPath, err := pickSaveFile("保存合并后的视频", defaultPath, "mp4")
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		outPath = strings.TrimSpace(outPath)
		if outPath == "" {
			return
		}
		if !strings.EqualFold(filepath.Ext(outPath), ".mp4") {
			outPath += ".mp4"
		}

		ctx, cancelFn := context.WithCancel(context.Background())
		mu.Lock()
		cancel = cancelFn
		setBusy(true)
		mu.Unlock()
		progress.SetValue(0)
		status.SetText("正在合并…")

		go func() {
			err := media.MergeOrderedVideos(ctx, bins, batch, outPath, func(pct int) {
				fyne.Do(func() {
					progress.SetValue(float64(pct))
					status.SetText(fmt.Sprintf("正在合并… %d%%", pct))
				})
			})
			fyne.Do(func() {
				mu.Lock()
				cancel = nil
				if err == nil {
					lastOut = outPath
				}
				setBusy(false)
				mu.Unlock()
				switch {
				case err == nil:
					progress.SetValue(100)
					status.SetText("已保存 " + outPath)
				case ctx.Err() != nil:
					status.SetText("已停止")
				default:
					status.SetText(err.Error())
				}
			})
		}()
	})
	startBtn.Importance = widget.HighImportance

	header := container.NewVBox(
		pageTitle("视频合并"),
		intro,
		container.NewHBox(addBtn, upBtn, downBtn, removeBtn, clearBtn),
		container.NewHBox(startBtn, cancelBtn, openBtn),
		countLabel,
		pathLabel,
	)
	footer := container.NewVBox(progress, status)
	return container.NewPadded(container.NewBorder(header, footer, nil, nil, list))
}
