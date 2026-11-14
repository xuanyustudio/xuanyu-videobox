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
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/replicate"
)

const replicateWholeView = "整片"

func replicateSegmentLabel(seg replicate.SegmentResult) string {
	return fmt.Sprintf("第 %d 段 %.1f-%.1f秒", seg.Index, seg.StartSec, seg.EndSec)
}

type replicateJob struct {
	Path   string
	State  string // 等待 / 进行中 / 完成 / 失败 / 已取消
	ErrMsg string
	Result *replicate.Result
}

func (j *replicateJob) label() string {
	base := filepath.Base(j.Path)
	switch j.State {
	case "进行中":
		return "▶ " + base
	case "完成":
		return "✓ " + base
	case "失败":
		return "✗ " + base
	case "已取消":
		return "– " + base
	default:
		return "· " + base
	}
}

func newReplicatePage(state *AppState) fyne.CanvasObject {
	var (
		jobs      []replicateJob
		selected  int = -1
		last      *replicate.Result
		runMu     sync.Mutex
		cancelRun context.CancelFunc
		busy      bool
	)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	subtitle := widget.NewLabel("可一次多选视频，按队列逐个提取：整片字幕 → 分段抽帧 → 生成提示词。失败会跳过并继续下一条。")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	queueHint := widget.NewLabel("未选择视频（支持多选）")
	queueHint.Wrapping = fyne.TextWrapWord
	queueHint.Importance = widget.LowImportance

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
			hint = fmt.Sprintf("共 %d 张关键帧, 已保存到 %s", len(frames), res.OutputDir)
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

	var openOutBtn, startBtn, cancelBtn *widget.Button

	queueList := widget.NewList(
		func() int { return len(jobs) },
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			lab := obj.(*widget.Label)
			if id < 0 || id >= len(jobs) {
				lab.SetText("")
				return
			}
			j := jobs[id]
			extra := j.State
			if j.State == "失败" && j.ErrMsg != "" {
				extra = "失败: " + j.ErrMsg
			}
			lab.SetText(fmt.Sprintf("%s  [%s]", j.label(), extra))
			lab.Truncation = fyne.TextTruncateEllipsis
		},
	)
	queueList.OnSelected = func(id widget.ListItemID) {
		selected = id
		if id < 0 || id >= len(jobs) {
			return
		}
		j := jobs[id]
		if j.Result != nil {
			last = j.Result
			showResult(j.Result)
			openOutBtn.Enable()
			status.SetText("查看: " + filepath.Base(j.Path))
		} else if j.ErrMsg != "" {
			status.SetText(filepath.Base(j.Path) + " — " + j.ErrMsg)
		}
	}

	refreshQueue := func() {
		n := len(jobs)
		if n == 0 {
			queueHint.SetText("未选择视频（支持多选）")
		} else {
			done, fail, pending := 0, 0, 0
			for _, j := range jobs {
				switch j.State {
				case "完成":
					done++
				case "失败", "已取消":
					fail++
				default:
					pending++
				}
			}
			queueHint.SetText(fmt.Sprintf("队列 %d 个：完成 %d · 失败/取消 %d · 待处理 %d（点列表可查看结果）", n, done, fail, pending))
		}
		queueList.Refresh()
	}

	setBusy := func(v bool) {
		busy = v
		if v {
			startBtn.Disable()
			cancelBtn.Enable()
		} else {
			startBtn.Enable()
			cancelBtn.Disable()
			anyOK := false
			for _, j := range jobs {
				if j.Result != nil {
					anyOK = true
					break
				}
			}
			if anyOK {
				openOutBtn.Enable()
			}
		}
	}

	openOutBtn = widget.NewButton("打开输出", nil)
	openOutBtn.Disable()
	openOutBtn.OnTapped = func() {
		dir := ""
		if selected >= 0 && selected < len(jobs) && jobs[selected].Result != nil {
			dir = jobs[selected].Result.OutputDir
		} else if last != nil {
			dir = last.OutputDir
		} else {
			for i := len(jobs) - 1; i >= 0; i-- {
				if jobs[i].Result != nil {
					dir = jobs[i].Result.OutputDir
					break
				}
			}
		}
		if dir == "" {
			status.SetText("暂无输出目录")
			return
		}
		_ = media.OpenFile(dir)
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

	pickVideoBtn := widget.NewButton("选择视频（可多选）", func() {
		if busy {
			status.SetText("处理中，请先取消或等完成后再选")
			return
		}
		files, err := pickVideoFiles()
		if err != nil {
			status.SetText("选择失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		seen := map[string]struct{}{}
		jobs = jobs[:0]
		for _, f := range files {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			jobs = append(jobs, replicateJob{Path: f, State: "等待"})
		}
		selected = -1
		last = nil
		openOutBtn.Disable()
		progress.SetValue(0)
		status.SetText("")
		showResult(nil)
		refreshQueue()
		if len(jobs) == 1 {
			status.SetText("已选 1 个视频，点「开始批量提取」")
		} else if len(jobs) > 1 {
			status.SetText(fmt.Sprintf("已选 %d 个视频，将按顺序逐个提取", len(jobs)))
		}
	})

	addVideoBtn := widget.NewButton("追加", func() {
		if busy {
			status.SetText("处理中，请先取消或等完成后再追加")
			return
		}
		files, err := pickVideoFiles()
		if err != nil {
			status.SetText("选择失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		seen := map[string]struct{}{}
		for _, j := range jobs {
			seen[j.Path] = struct{}{}
		}
		added := 0
		for _, f := range files {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			jobs = append(jobs, replicateJob{Path: f, State: "等待"})
			added++
		}
		refreshQueue()
		if added == 0 {
			status.SetText("没有新文件可追加")
		} else {
			status.SetText(fmt.Sprintf("已追加 %d 个，队列共 %d 个", added, len(jobs)))
		}
	})
	addVideoBtn.Importance = widget.LowImportance

	clearBtn := widget.NewButton("清空", func() {
		if busy {
			return
		}
		jobs = nil
		selected = -1
		last = nil
		progress.SetValue(0)
		status.SetText("")
		openOutBtn.Disable()
		showResult(nil)
		refreshQueue()
	})
	clearBtn.Importance = widget.LowImportance

	buildParams := func(videoPath string, frames, segSec int, total, index int) replicate.Params {
		return replicate.Params{
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
					if total <= 0 {
						progress.SetValue(float64(percent))
					} else {
						base := float64(index) / float64(total) * 100
						step := float64(percent) / float64(total)
						progress.SetValue(base + step)
					}
					if msg != "" {
						status.SetText(fmt.Sprintf("[%d/%d] %s — %s", index+1, total, filepath.Base(videoPath), msg))
					}
				})
			},
		}
	}

	startJob := func() {
		runMu.Lock()
		if busy {
			runMu.Unlock()
			return
		}
		if len(jobs) == 0 {
			runMu.Unlock()
			status.SetText("请先选择至少一个视频")
			return
		}
		pendingIdx := make([]int, 0, len(jobs))
		for i := range jobs {
			if jobs[i].State == "完成" && jobs[i].Result != nil {
				continue
			}
			jobs[i].State = "等待"
			jobs[i].ErrMsg = ""
			jobs[i].Result = nil
			pendingIdx = append(pendingIdx, i)
		}
		if len(pendingIdx) == 0 {
			runMu.Unlock()
			status.SetText("队列里的视频都已完成")
			refreshQueue()
			return
		}

		frames, _ := strconv.Atoi(framesSelect.Selected)
		segSec, _ := strconv.Atoi(segSelect.Selected)
		// validate with first pending
		probe := buildParams(jobs[pendingIdx[0]].Path, frames, segSec, len(pendingIdx), 0)
		if err := replicate.Validate(probe); err != nil {
			runMu.Unlock()
			status.SetText(err.Error())
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		runMu.Unlock()

		refreshQueue()
		progress.SetValue(1)
		status.SetText(fmt.Sprintf("开始批量提取，共 %d 个…", len(pendingIdx)))
		state.Cfg.Replicate.SegmentSec = replicate.NormalizeSegmentSec(segSec)
		if err := config.Save(state.CfgPath, state.Cfg); err != nil {
			state.Log.Append("[爆款复刻] 保存分段时长失败: " + err.Error())
		}

		go func(indices []int, frames, segSec int) {
			total := len(indices)
			okCount, failCount := 0, 0
			for step, ji := range indices {
				if ctx.Err() != nil {
					fyne.Do(func() {
						for _, k := range indices[step:] {
							if jobs[k].State == "等待" || jobs[k].State == "进行中" {
								jobs[k].State = "已取消"
							}
						}
						refreshQueue()
					})
					break
				}
				path := jobs[ji].Path
				fyne.Do(func() {
					jobs[ji].State = "进行中"
					selected = ji
					queueList.Select(ji)
					refreshQueue()
					status.SetText(fmt.Sprintf("[%d/%d] 开始: %s", step+1, total, filepath.Base(path)))
				})
				state.Log.Append(fmt.Sprintf("[爆款复刻] [%d/%d] %s", step+1, total, path))

				params := buildParams(path, frames, segSec, total, step)
				res, err := replicate.Run(ctx, params)

				if err != nil {
					canceled := ctx.Err() != nil
					errText := err.Error()
					if !canceled {
						failCount++
						state.Log.Append("[爆款复刻] 失败: " + errText)
					}
					fyne.Do(func() {
						if canceled {
							jobs[ji].State = "已取消"
							jobs[ji].ErrMsg = "已取消"
						} else {
							jobs[ji].State = "失败"
							jobs[ji].ErrMsg = errText
						}
						refreshQueue()
						progress.SetValue(float64(step+1) / float64(total) * 100)
					})
				} else {
					okCount++
					state.Log.Append("[爆款复刻] 完成: " + res.OutputDir)
					fyne.Do(func() {
						jobs[ji].State = "完成"
						jobs[ji].Result = res
						jobs[ji].ErrMsg = ""
						last = res
						showResult(res)
						openOutBtn.Enable()
						refreshQueue()
						progress.SetValue(float64(step+1) / float64(total) * 100)
					})
				}
			}

			fyne.Do(func() {
				runMu.Lock()
				cancelRun = nil
				setBusy(false)
				runMu.Unlock()
				if ctx.Err() != nil {
					status.SetText(fmt.Sprintf("已取消。完成 %d，失败 %d", okCount, failCount))
					return
				}
				progress.SetValue(100)
				status.SetText(fmt.Sprintf("全部结束：成功 %d / 失败 %d / 共 %d", okCount, failCount, total))
				refreshQueue()
			})
		}(append([]int(nil), pendingIdx...), frames, segSec)
	}

	startBtn = widget.NewButton("开始批量提取", startJob)
	startBtn.Importance = widget.HighImportance
	cancelBtn = widget.NewButton("取消", func() {
		runMu.Lock()
		if cancelRun != nil {
			cancelRun()
		}
		runMu.Unlock()
	})
	cancelBtn.Disable()

	sec := sectionTitle
	toolbar := container.NewHBox(pickVideoBtn, addVideoBtn, startBtn, cancelBtn, openOutBtn, clearBtn)
	optsRow := container.NewHBox(
		widget.NewLabel("关键帧数"), framesSelect,
		widget.NewLabel("分段时长"), segSelect,
		widget.NewLabel("提示词模板"), tplSelect,
	)
	modelHint := widget.NewLabel("视觉模型在「设置」中配置（默认 ModelScope Qwen3-VL）；每个视频结果保存在各自同目录 replicate_时间戳/ 下")
	modelHint.Wrapping = fyne.TextWrapWord
	modelHint.Importance = widget.LowImportance

	moreAcc := widget.NewAccordion(
		widget.NewAccordionItem("提取结果分析（只读）", analysisEntry),
	)

	queueScroll := container.NewVScroll(queueList)
	queueScroll.SetMinSize(fyne.NewSize(520, 140))

	body := container.NewVBox(
		pageTitle("爆款复刻"),
		subtitle,
		toolbar,
		queueHint,
		queueScroll,
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
