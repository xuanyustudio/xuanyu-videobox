//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/replicate"
)

const replicateWholeView = "整片"

func replicateSegmentLabel(seg replicate.SegmentResult) string {
	return fmt.Sprintf("第 %d 段 · %.1f–%.1f秒", seg.Index, seg.StartSec, seg.EndSec)
}

func newReplicatePage(state *AppState) fyne.CanvasObject {
	videoPath := ""
	var last *replicate.Result

	var (
		runMu     sync.Mutex
		cancelRun context.CancelFunc
		busy      bool
	)

	videoLabel := widget.NewLabel("未选择视频")
	videoLabel.Truncation = fyne.TextTruncateEllipsis

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	subtitle := widget.NewLabel("上传视频，按 10 或 15 秒分段抽帧；先提取整片字幕，再逐段生成提示词。")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	frameOpts := []string{}
	for n := 4; n <= replicate.MaxFrames; n += 2 {
		frameOpts = append(frameOpts, strconv.Itoa(n))
	}
	framesSelect := widget.NewSelect(frameOpts, nil)
	framesSelect.SetSelected(strconv.Itoa(replicate.ClampFrames(state.Cfg.Replicate.Frames)))
	if framesSelect.Selected == "" {
		framesSelect.SetSelected(strconv.Itoa(replicate.DefaultFrames))
	}

	segOpts := []string{"10", "15"}
	segSelect := widget.NewSelect(segOpts, nil)
	segSelect.SetSelected(strconv.Itoa(replicate.NormalizeSegmentSec(state.Cfg.Replicate.SegmentSec)))
	if segSelect.Selected == "" {
		segSelect.SetSelected("15")
	}

	tplLabels := []string{}
	for _, o := range replicate.TemplateOptions() {
		tplLabels = append(tplLabels, o.Label)
	}
	tplSelect := widget.NewSelect(tplLabels, nil)
	tplSelect.SetSelected(tplLabels[0])

	const thumbW, thumbH float32 = 150, 90
	framesGrid := container.NewGridWrap(fyne.NewSize(thumbW, thumbH))
	framesHint := widget.NewLabel("提取后在此预览关键帧")
	framesHint.Wrapping = fyne.TextWrapWord
	framesHint.Importance = widget.LowImportance

	promptEnEntry := widget.NewMultiLineEntry()
	promptEnEntry.SetPlaceHolder("提取出的英文 SORA2 提示词（可修改后复制使用）")
	promptEnEntry.Wrapping = fyne.TextWrapWord
	promptEnEntry.SetMinRowsVisible(8)

	promptZhEntry := widget.NewMultiLineEntry()
	promptZhEntry.SetPlaceHolder("提取出的中文提示词将显示在这里…")
	promptZhEntry.Wrapping = fyne.TextWrapWord
	promptZhEntry.SetMinRowsVisible(8)

	promptTabs := container.NewAppTabs(
		container.NewTabItem("英文", promptEnEntry),
		container.NewTabItem("中文", promptZhEntry),
	)

	analysisEntry := widget.NewMultiLineEntry()
	analysisEntry.SetPlaceHolder("提取出的提示词分析将显示在这里…")
	analysisEntry.Wrapping = fyne.TextWrapWord
	analysisEntry.SetMinRowsVisible(10)
	analysisEntry.Disable()

	var applyView func(*replicate.Result, string)
	resultSelect := widget.NewSelect([]string{replicateWholeView}, func(choice string) {
		if applyView != nil {
			applyView(last, choice)
		}
	})
	resultSelect.SetSelected(replicateWholeView)

	applyView = func(res *replicate.Result, choice string) {
		var frames []string
		en, zh, analysis := "", "", ""
		hint := "提取后在此预览关键帧"
		if res != nil {
			frames = res.Frames
			en = res.PromptEN
			if en == "" {
				en = res.Prompt
			}
			zh = res.PromptZH
			analysis = res.Analysis
			if choice != replicateWholeView {
				for _, seg := range res.Segments {
					if replicateSegmentLabel(seg) != choice {
						continue
					}
					frames = seg.Frames
					en = seg.PromptEN
					zh = seg.PromptZH
					analysis = seg.Analysis
					break
				}
			}
			hint = fmt.Sprintf("共 %d 张关键帧 · 已保存到 %s", len(frames), res.OutputDir)
		}
		framesGrid.Objects = nil
		for _, f := range frames {
			img := canvas.NewImageFromFile(f)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(thumbW, thumbH))
			framesGrid.Add(img)
		}
		framesHint.SetText(hint)
		promptEnEntry.SetText(en)
		promptZhEntry.SetText(zh)
		analysisEntry.SetText(analysis)
		framesGrid.Refresh()
	}

	showResult := func(res *replicate.Result) {
		opts := []string{replicateWholeView}
		if res != nil && len(res.Segments) >= 2 {
			for _, seg := range res.Segments {
				opts = append(opts, replicateSegmentLabel(seg))
			}
		}
		cb := resultSelect.OnChanged
		resultSelect.OnChanged = nil
		resultSelect.SetOptions(opts)
		resultSelect.SetSelected(replicateWholeView)
		resultSelect.OnChanged = cb
		applyView(res, replicateWholeView)
	}

	openOutBtn := widget.NewButton("打开输出", nil)
	openOutBtn.Disable()
	openOutBtn.OnTapped = func() {
		if last == nil {
			return
		}
		_ = media.OpenFile(last.OutputDir)
	}

	copyBtn := widget.NewButton("复制提示词", func() {
		text := strings.TrimSpace(promptEnEntry.Text)
		if text == "" {
			status.SetText("暂无提示词")
			return
		}
		fyne.CurrentApp().Clipboard().SetContent(text)
		status.SetText("已复制英文提示词到剪贴板")
	})
	copyZhBtn := widget.NewButton("复制中文", func() {
		text := strings.TrimSpace(promptZhEntry.Text)
		if text == "" {
			status.SetText("未解析到中文提示词")
			return
		}
		fyne.CurrentApp().Clipboard().SetContent(text)
		status.SetText("已复制中文提示词到剪贴板")
	})
	copyZhBtn.Importance = widget.LowImportance

	var startBtn, cancelBtn *widget.Button

	setBusy := func(v bool) {
		busy = v
		if v {
			startBtn.Disable()
			cancelBtn.Enable()
			openOutBtn.Disable()
		} else {
			startBtn.Enable()
			cancelBtn.Disable()
			if last != nil {
				openOutBtn.Enable()
			}
		}
	}

	pickVideoBtn := widget.NewButton("选择视频", func() {
		files, err := pickVideoFiles()
		if err != nil {
			status.SetText("选择失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		videoPath = files[0]
		videoLabel.SetText(filepath.Base(videoPath))
		status.SetText("")
	})

	clearBtn := widget.NewButton("清空", func() {
		if busy {
			return
		}
		videoPath = ""
		last = nil
		videoLabel.SetText("未选择视频")
		progress.SetValue(0)
		status.SetText("")
		openOutBtn.Disable()
		showResult(nil)
	})
	clearBtn.Importance = widget.LowImportance

	startJob := func() {
		runMu.Lock()
		if busy {
			runMu.Unlock()
			return
		}
		frames, _ := strconv.Atoi(framesSelect.Selected)
		segSec, _ := strconv.Atoi(segSelect.Selected)
		params := replicate.Params{
			VideoPath:  videoPath,
			Frames:     frames,
			Template:   replicate.TemplateByLabel(tplSelect.Selected),
			BaseURL:    state.Cfg.Replicate.BaseURL,
			APIKey:     state.Cfg.Replicate.APIKey,
			Model:      state.Cfg.Replicate.Model,
			SegmentSec: segSec,
			ASR: asr.Options{
				Backend:       state.Cfg.ASR.Backend,
				APIURL:        state.Cfg.ASR.APIURL,
				FireRedAPIURL: state.Cfg.ASR.FireRedAPIURL,
				BailianAPIKey: state.Cfg.ASR.APIKey,
				Hotword:       state.Cfg.ASR.Hotword,
				EnableSpk:     state.Cfg.ASR.EnableSpk,
			},
			Bins: media.FFmpegBins{
				FFmpeg:  state.Cfg.FFmpeg.Path,
				FFprobe: state.Cfg.FFmpeg.FFprobePath,
			},
			OnProgress: func(percent int, msg string) {
				fyne.Do(func() {
					progress.SetValue(float64(percent))
					if msg != "" {
						status.SetText(msg)
					}
				})
			},
		}
		if err := replicate.Validate(params); err != nil {
			runMu.Unlock()
			status.SetText(err.Error())
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		runMu.Unlock()

		progress.SetValue(5)
		status.SetText("分析中…")
		state.Log.Append("[爆款复刻] " + videoPath)
		state.Cfg.Replicate.SegmentSec = replicate.NormalizeSegmentSec(segSec)
		if err := config.Save(state.CfgPath, state.Cfg); err != nil {
			state.Log.Append("[爆款复刻] 保存分段时长失败: " + err.Error())
		}

		go func() {
			res, err := replicate.Run(ctx, params)
			fyne.Do(func() {
				runMu.Lock()
				cancelRun = nil
				if err == nil {
					last = res
				}
				setBusy(false)
				runMu.Unlock()
				if err != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText("提示词提取失败: " + err.Error())
					state.Log.Append("[爆款复刻] 失败: " + err.Error())
					dialog.ShowError(err, state.Win)
					return
				}
				progress.SetValue(100)
				showResult(res)
				if len(res.Segments) >= 2 {
					status.SetText("完成 · 各段提示词已写入同一段锁定形象。到即梦或 Sora 分段生成时，请每一段都上传 " + res.CharacterRef)
				} else {
					status.SetText("完成 · " + res.OutputDir)
				}
				state.Log.Append("[爆款复刻] 完成: " + res.OutputDir)
			})
		}()
	}

	startBtn = widget.NewButton("提取视频提示词 & 关键帧", startJob)
	startBtn.Importance = widget.HighImportance
	cancelBtn = widget.NewButton("取消", func() {
		runMu.Lock()
		if cancelRun != nil {
			cancelRun()
		}
		runMu.Unlock()
	})
	cancelBtn.Disable()

	sec := func(text string) *widget.Label {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}

	toolbar := container.NewHBox(pickVideoBtn, startBtn, cancelBtn, openOutBtn, clearBtn)
	optsRow := container.NewHBox(
		widget.NewLabel("关键帧数"), framesSelect,
		widget.NewLabel("分段时长"), segSelect,
		widget.NewLabel("提示词模板"), tplSelect,
	)
	modelHint := widget.NewLabel("视觉模型在「设置」中配置（默认 ModelScope Qwen3-VL）；结果保存在视频同目录 replicate_时间戳/ 下")
	modelHint.Wrapping = fyne.TextWrapWord
	modelHint.Importance = widget.LowImportance

	moreAcc := widget.NewAccordion(
		widget.NewAccordionItem("提取结果分析（只读）", analysisEntry),
	)

	body := container.NewVBox(
		subtitle,
		container.NewBorder(nil, nil, toolbar, nil, videoLabel),
		progress,
		status,
		optsRow,
		modelHint,
		container.NewHBox(widget.NewLabel("查看"), resultSelect),
		sec("关键帧"),
		framesHint,
		framesGrid,
		sec("SORA2 提示词"),
		promptTabs,
		container.NewHBox(copyBtn, copyZhBtn),
		moreAcc,
	)
	return container.NewVScroll(body)
}
