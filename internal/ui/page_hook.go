//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/hookopen"
	"github.com/narratoai/narrato-go/internal/media"
)

func newHookPage(state *AppState) fyne.CanvasObject {
	var (
		mu        sync.Mutex
		busy      bool
		cancel    context.CancelFunc
		videoPath string
		result    *hookopen.Result
		selected  = -1
		lastOut   string
	)

	intro := widget.NewLabel("选择一条视频，程序会结合画面、台词和音量，找出大约 3 到 8 秒最抓眼球的片段，放到片头后再加转场，合成一条新视频。原片高潮会再出现一次，也就是常见的「高潮前置」。")
	intro.Wrapping = fyne.TextWrapWord
	intro.Importance = widget.LowImportance

	pathLabel := widget.NewLabel("还没有选择视频")
	pathLabel.Wrapping = fyne.TextWrapWord
	pathLabel.Importance = widget.LowImportance

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	sourceHint := widget.NewLabel("")
	sourceHint.Wrapping = fyne.TextWrapWord
	sourceHint.Importance = widget.LowImportance

	detail := widget.NewLabel("分析完成后，这里会列出几个候选片头。选一个即可预览或合成。")
	detail.Wrapping = fyne.TextWrapWord

	thumb := canvas.NewImageFromResource(theme.MediaVideoIcon())
	thumb.FillMode = canvas.ImageFillContain
	thumb.SetMinSize(fyne.NewSize(160, 90))

	transNames := make([]string, 0, len(hookopen.Transitions))
	for _, t := range hookopen.Transitions {
		transNames = append(transNames, t.Label)
	}
	transSel := widget.NewSelect(transNames, nil)
	transSel.SetSelected("淡入淡出")

	var (
		pickBtn    *widget.Button
		analyzeBtn *widget.Button
		cancelBtn  *widget.Button
		previewBtn *widget.Button
		renderBtn  *widget.Button
		openBtn    *widget.Button
		folderBtn  *widget.Button
		list       *widget.List
	)

	setStatus := func(msg string) {
		status.SetText(msg)
	}

	candidates := func() []hookopen.Candidate {
		if result == nil {
			return nil
		}
		return result.Candidates
	}

	refreshDetail := func() {
		cs := candidates()
		list.Refresh()
		if selected < 0 || selected >= len(cs) {
			detail.SetText("分析完成后，这里会列出几个候选片头。选一个即可预览或合成。")
			thumb.File = ""
			thumb.Resource = theme.MediaVideoIcon()
			thumb.Refresh()
			return
		}
		c := cs[selected]
		mark := ""
		if c.Recommended {
			mark = "（推荐）"
		}
		detail.SetText(fmt.Sprintf("第 %d 个%s · %s · %s（约 %.1f 秒）\n理由：%s\n内容：%s",
			c.Rank, mark, c.HookType, hookopen.FormatRange(c.StartSec, c.EndSec), c.Duration(),
			orDash(c.Reason), orDash(c.Quote)))
		if c.ThumbPath != "" {
			thumb.Resource = nil
			thumb.File = c.ThumbPath
		} else {
			thumb.File = ""
			thumb.Resource = theme.MediaVideoIcon()
		}
		thumb.Refresh()
	}

	setBusy := func(v bool) {
		busy = v
		if v {
			pickBtn.Disable()
			analyzeBtn.Disable()
			previewBtn.Disable()
			renderBtn.Disable()
			cancelBtn.Enable()
			return
		}
		pickBtn.Enable()
		if videoPath != "" {
			analyzeBtn.Enable()
		} else {
			analyzeBtn.Disable()
		}
		cs := candidates()
		hasSel := selected >= 0 && selected < len(cs)
		if hasSel && cs[selected].PreviewPath != "" {
			previewBtn.Enable()
		} else {
			previewBtn.Disable()
		}
		if hasSel {
			renderBtn.Enable()
		} else {
			renderBtn.Disable()
		}
		cancelBtn.Disable()
		if lastOut != "" {
			openBtn.Enable()
			folderBtn.Enable()
		}
	}

	list = widget.NewList(
		func() int { return len(candidates()) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("候选")
			title.Truncation = fyne.TextTruncateEllipsis
			sub := widget.NewLabel("")
			sub.Importance = widget.LowImportance
			sub.Truncation = fyne.TextTruncateEllipsis
			return container.NewVBox(title, sub)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			sub := box.Objects[1].(*widget.Label)
			cs := candidates()
			if int(id) >= len(cs) {
				title.SetText("")
				sub.SetText("")
				return
			}
			c := cs[id]
			mark := ""
			if c.Recommended {
				mark = " · 推荐"
			}
			title.SetText(fmt.Sprintf("%d. %s%s  %s（%.1f 秒）", c.Rank, c.HookType, mark, hookopen.FormatRange(c.StartSec, c.EndSec), c.Duration()))
			reason := c.Reason
			if reason == "" {
				reason = c.Quote
			}
			sub.SetText(reason)
		},
	)
	enableActions := func() {
		if videoPath != "" {
			analyzeBtn.Enable()
		} else {
			analyzeBtn.Disable()
		}
		cs := candidates()
		hasSel := selected >= 0 && selected < len(cs)
		if hasSel && cs[selected].PreviewPath != "" {
			previewBtn.Enable()
		} else {
			previewBtn.Disable()
		}
		if hasSel {
			renderBtn.Enable()
		} else {
			renderBtn.Disable()
		}
		if lastOut != "" {
			openBtn.Enable()
			folderBtn.Enable()
		}
	}

	list.OnSelected = func(id widget.ListItemID) {
		selected = int(id)
		refreshDetail()
		if !busy {
			enableActions()
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
		videoPath = paths[0]
		result = nil
		selected = -1
		lastOut = ""
		pathLabel.SetText(videoPath)
		sourceHint.SetText("")
		setStatus("")
		progress.SetValue(0)
		refreshDetail()
		openBtn.Disable()
		folderBtn.Disable()
		mu.Lock()
		setBusy(false)
		mu.Unlock()
	})

	analyzeBtn = widget.NewButton("开始分析", nil)
	analyzeBtn.Importance = widget.HighImportance
	analyzeBtn.Disable()

	cancelBtn = widget.NewButton("取消", nil)
	cancelBtn.Disable()

	previewBtn = widget.NewButton("预览片段", nil)
	previewBtn.Disable()

	renderBtn = widget.NewButton("合成成片", nil)
	renderBtn.Importance = widget.HighImportance
	renderBtn.Disable()

	openBtn = widget.NewButton("打开成片", nil)
	openBtn.Disable()

	folderBtn = widget.NewButton("打开所在目录", nil)
	folderBtn.Disable()

	uiProgress := func(pct int, msg string) {
		fyne.Do(func() {
			if pct >= 0 {
				progress.SetValue(float64(pct))
			}
			if msg != "" {
				setStatus(msg)
			}
		})
	}

	analyzeBtn.OnTapped = func() {
		mu.Lock()
		if busy || videoPath == "" {
			mu.Unlock()
			return
		}
		ctx, cfn := context.WithCancel(context.Background())
		cancel = cfn
		setBusy(true)
		mu.Unlock()
		progress.SetValue(0)
		setStatus("开始分析…")
		go func(path string) {
			res, err := hookopen.Analyze(ctx, state.Cfg, path, uiProgress)
			fyne.Do(func() {
				mu.Lock()
				cancel = nil
				if err != nil {
					if ctx.Err() != nil {
						setStatus("已取消")
					} else {
						setStatus("分析失败: " + err.Error())
					}
					setBusy(false)
					mu.Unlock()
					if ctx.Err() == nil {
						dialog.ShowError(err, state.Win)
					}
					return
				}
				result = res
				selected = 0
				for i, c := range res.Candidates {
					if c.Recommended {
						selected = i
						break
					}
				}
				if res.UsedModel {
					sourceHint.SetText("已结合台词、画面和模型判断，给出下面几个候选。")
				} else {
					sourceHint.SetText("模型未返回结果，已按台词冲击力和音量变化排出候选，仍可直接合成。")
				}
				sel := selected
				n := len(res.Candidates)
				setBusy(false)
				mu.Unlock()
				list.Select(sel)
				refreshDetail()
				enableActions()
				setStatus(fmt.Sprintf("分析完成，共 %d 个候选，默认选中第 %d 个。", n, sel+1))
				progress.SetValue(100)
			})
		}(videoPath)
	}

	cancelBtn.OnTapped = func() {
		mu.Lock()
		cfn := cancel
		mu.Unlock()
		if cfn != nil {
			cfn()
		}
	}

	previewBtn.OnTapped = func() {
		cs := candidates()
		if selected < 0 || selected >= len(cs) || cs[selected].PreviewPath == "" {
			return
		}
		if err := media.OpenFile(cs[selected].PreviewPath); err != nil {
			dialog.ShowError(err, state.Win)
		}
	}

	renderBtn.OnTapped = func() {
		mu.Lock()
		if busy || result == nil || selected < 0 || selected >= len(result.Candidates) {
			mu.Unlock()
			return
		}
		hook := result.Candidates[selected]
		src := result.VideoPath
		def := hookopen.DefaultOutputPath(src)
		mu.Unlock()

		outPath, err := pickSaveFile("保存爆点开头成片", def, "mp4")
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if outPath == "" {
			return
		}

		mu.Lock()
		ctx, cfn := context.WithCancel(context.Background())
		cancel = cfn
		setBusy(true)
		mu.Unlock()
		progress.SetValue(0)
		setStatus("开始合成…")
		trans := transSel.Selected
		go func() {
			err := hookopen.Render(ctx, state.Cfg, src, hook, trans, hookopen.DefaultXFadeSec, outPath, uiProgress)
			fyne.Do(func() {
				mu.Lock()
				defer mu.Unlock()
				cancel = nil
				if err != nil {
					if ctx.Err() != nil {
						setStatus("已取消")
					} else {
						setStatus("合成失败: " + err.Error())
						dialog.ShowError(err, state.Win)
					}
					setBusy(false)
					return
				}
				lastOut = outPath
				setStatus("已保存 " + outPath)
				progress.SetValue(100)
				setBusy(false)
			})
		}()
	}

	openBtn.OnTapped = func() {
		if lastOut == "" {
			return
		}
		if err := media.OpenFile(lastOut); err != nil {
			dialog.ShowError(err, state.Win)
		}
	}
	folderBtn.OnTapped = func() {
		target := lastOut
		if target == "" && result != nil {
			target = result.WorkDir
		}
		if target == "" {
			return
		}
		openExplorerOrFile(filepath.Dir(target))
	}

	form := container.NewVBox(
		pageTitle("爆点开头"),
		intro,
		container.NewHBox(pickBtn, analyzeBtn, cancelBtn),
		pathLabel,
		progress,
		status,
		sourceHint,
		sectionTitle("候选片头"),
	)

	right := container.NewVBox(
		sectionTitle("选中的片段"),
		thumb,
		detail,
		widget.NewSeparator(),
		widget.NewLabel("转场"),
		transSel,
		container.NewHBox(previewBtn, renderBtn),
		container.NewHBox(openBtn, folderBtn),
	)

	body := container.NewHSplit(
		container.NewBorder(nil, nil, nil, nil, list),
		container.NewVScroll(right),
	)
	body.Offset = 0.46

	return container.NewBorder(form, nil, nil, nil, body)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
