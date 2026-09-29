//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/script"
)

func newScriptPage(state *AppState) (fyne.CanvasObject, func()) {
	wordEntry := widget.NewEntry()
	wordEntry.SetText("800")
	origEntry := widget.NewEntry()
	origEntry.SetText("30")
	targetEntry := widget.NewEntry()
	targetSec := state.Cfg.UI.TargetDurationSec
	if targetSec <= 0 {
		targetSec = 180
	}
	targetEntry.SetText(strconv.Itoa(targetSec))

	estimateLabel := widget.NewLabel("预估时长: —")
	warnLabel := widget.NewLabel("")
	warnLabel.Importance = widget.WarningImportance
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance

	updateEstimate := func() {
		if state.Project == nil {
			estimateLabel.SetText("预估时长: —")
			warnLabel.SetText("")
			return
		}
		est := script.EstimateOutputDurationSec(state.Project.ScriptItems)
		estimateLabel.SetText(fmt.Sprintf("预估时长: %s (%.0f 秒)", script.FormatDurationClock(est), est))
		target, _ := strconv.Atoi(strings.TrimSpace(targetEntry.Text))
		if target > 0 && est > float64(target)+0.5 {
			warnLabel.SetText(fmt.Sprintf("⚠ 超过目标 %d 秒", target))
		} else {
			warnLabel.SetText("")
		}
	}

	var list *widget.List
	refresh := func() {
		if list != nil {
			list.Refresh()
		}
		updateEstimate()
	}

	previewClipAt := func(i int) {
		if state.Project == nil || i < 0 || i >= len(state.Project.ScriptItems) {
			status.SetText("请先生成脚本")
			return
		}
		item := state.Project.ScriptItems[i]
		ts := strings.TrimSpace(item.Timestamp)
		vid := item.VideoID
		if vid <= 0 {
			vid = 1
		}
		idx := vid - 1
		if idx < 0 || idx >= len(state.Project.VideoPaths) {
			status.SetText(fmt.Sprintf("第 %d 条视频序号 %d 超出素材列表（共 %d 个）", i+1, vid, len(state.Project.VideoPaths)))
			return
		}
		video := state.Project.VideoPaths[idx]
		ffmpegBin, err := media.ResolveFFmpeg(state.Cfg.FFmpeg.Path, "")
		if err != nil {
			status.SetText("找不到 ffmpeg: " + err.Error())
			return
		}
		status.SetText(fmt.Sprintf("正在截取第 %d 条预览…", i+1))
		go func() {
			dir := filepath.Join(os.TempDir(), "narrato-preview")
			_ = os.MkdirAll(dir, 0o755)
			out := filepath.Join(dir, fmt.Sprintf("clip_%d.mp4", i+1))
			err := media.PreviewClip(context.Background(), media.FFmpegBins{FFmpeg: ffmpegBin}, video, ts, out)
			fyne.Do(func() {
				if err != nil {
					status.SetText("预览失败: " + err.Error())
					return
				}
				if err := media.OpenFile(out); err != nil {
					status.SetText("已生成但无法打开: " + out)
					return
				}
				status.SetText(fmt.Sprintf("已播放第 %d 条预览", i+1))
			})
		}()
	}

	showEditDialog := func(i int) {
		if state.Project == nil || i < 0 || i >= len(state.Project.ScriptItems) {
			status.SetText("没有可编辑的片段")
			return
		}
		item := state.Project.ScriptItems[i]

		tsEntry := widget.NewEntry()
		tsEntry.SetPlaceHolder("00:00:01,000-00:00:04,000")
		tsEntry.SetText(item.Timestamp)
		ostEntry := widget.NewEntry()
		ostEntry.SetPlaceHolder("0 解说 / 1 原声 / 2 原声+解说")
		ostEntry.SetText(strconv.Itoa(item.OST))
		vidEntry := widget.NewEntry()
		vid := item.VideoID
		if vid <= 0 {
			vid = 1
		}
		vidEntry.SetText(strconv.Itoa(vid))
		narrEntry := widget.NewMultiLineEntry()
		narrEntry.SetMinRowsVisible(5)
		narrEntry.Wrapping = fyne.TextWrapWord
		narrEntry.SetText(item.Narration)
		pictEntry := widget.NewMultiLineEntry()
		pictEntry.SetMinRowsVisible(3)
		pictEntry.Wrapping = fyne.TextWrapWord
		pictEntry.SetText(item.Picture)
		editStatus := widget.NewLabel("")

		save := func() bool {
			ts := strings.TrimSpace(tsEntry.Text)
			if _, _, err := script.ParseTimestampRange(ts); err != nil {
				editStatus.SetText("时间范围格式应为 00:00:00,000-00:00:02,000")
				return false
			}
			ost, err := strconv.Atoi(strings.TrimSpace(ostEntry.Text))
			if err != nil || ost < 0 || ost > 2 {
				editStatus.SetText("OST 只能是 0、1 或 2")
				return false
			}
			vidN, err := strconv.Atoi(strings.TrimSpace(vidEntry.Text))
			if err != nil || vidN < 1 {
				editStatus.SetText("视频序号从 1 开始")
				return false
			}
			it := &state.Project.ScriptItems[i]
			it.Timestamp = ts
			it.OST = ost
			it.VideoID = vidN
			it.Narration = narrEntry.Text
			it.Picture = pictEntry.Text
			if err := project.Save(state.ProjectsRoot, state.Project); err != nil {
				editStatus.SetText("保存失败: " + err.Error())
				return false
			}
			refresh()
			status.SetText(fmt.Sprintf("已保存第 %d 条", i+1))
			return true
		}

		var d dialog.Dialog
		previewInDlg := widget.NewButton("预览片段", func() {
			if !save() {
				return
			}
			previewClipAt(i)
		})
		saveBtn := widget.NewButton("保存", func() {
			if save() && d != nil {
				d.Hide()
			}
		})
		saveBtn.Importance = widget.HighImportance
		form := widget.NewForm(
			widget.NewFormItem("时间范围", tsEntry),
			widget.NewFormItem("视频序号", vidEntry),
			widget.NewFormItem("OST", ostEntry),
			widget.NewFormItem("画面描述", pictEntry),
			widget.NewFormItem("解说词", narrEntry),
		)
		body := container.NewVBox(form, container.NewHBox(saveBtn, previewInDlg), editStatus)
		d = dialog.NewCustom(
			fmt.Sprintf("编辑第 %d 条", i+1),
			"关闭",
			container.NewPadded(body),
			state.Win,
		)
		d.Resize(fyne.NewSize(640, 520))
		d.Show()
	}

	list = widget.NewList(
		func() int {
			if state.Project == nil {
				return 0
			}
			return len(state.Project.ScriptItems)
		},
		func() fyne.CanvasObject {
			idx := widget.NewLabel("0")
			idx.Hide()
			title := widget.NewLabel("")
			title.Truncation = fyne.TextTruncateEllipsis
			meta := widget.NewLabel("")
			meta.Importance = widget.LowImportance
			edit := widget.NewButton("编辑", nil)
			edit.Importance = widget.LowImportance
			prev := widget.NewButton("预览", nil)
			prev.Importance = widget.LowImportance
			edit.OnTapped = func() {
				n, _ := strconv.Atoi(idx.Text)
				showEditDialog(n)
			}
			prev.OnTapped = func() {
				n, _ := strconv.Atoi(idx.Text)
				previewClipAt(n)
			}
			text := container.NewVBox(title, meta)
			return container.NewBorder(nil, nil, idx, container.NewHBox(edit, prev), text)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			idx := box.Objects[1].(*widget.Label)
			actions := box.Objects[2].(*fyne.Container)
			_ = actions
			text := box.Objects[0].(*fyne.Container)
			title := text.Objects[0].(*widget.Label)
			meta := text.Objects[1].(*widget.Label)
			if state.Project == nil || int(id) >= len(state.Project.ScriptItems) {
				return
			}
			item := state.Project.ScriptItems[id]
			idx.SetText(strconv.Itoa(int(id)))
			line := strings.TrimSpace(item.Narration)
			if line == "" {
				line = strings.TrimSpace(item.Picture)
			}
			if line == "" {
				line = "（无解说）"
			}
			title.SetText(fmt.Sprintf("%d. %s", int(id)+1, line))
			vid := item.VideoID
			if vid <= 0 {
				vid = 1
			}
			meta.SetText(fmt.Sprintf("%s  ·  视频 %d  ·  OST %d", item.Timestamp, vid, item.OST))
		},
	)
	generateBtn := widget.NewButton("生成脚本", nil)
	generateBtn.Importance = widget.HighImportance
	generateBtn.OnTapped = func() {
		if state.Project == nil {
			warnLabel.SetText("请先打开或新建工程")
			return
		}
		wordCount, err := strconv.Atoi(strings.TrimSpace(wordEntry.Text))
		if err != nil || wordCount <= 0 {
			warnLabel.SetText("字数无效")
			return
		}
		origRatio, err := strconv.Atoi(strings.TrimSpace(origEntry.Text))
		if err != nil || origRatio < 0 {
			warnLabel.SetText("原片占比无效")
			return
		}
		if t, err := strconv.Atoi(strings.TrimSpace(targetEntry.Text)); err == nil && t > 0 {
			state.Cfg.UI.TargetDurationSec = t
		}

		subText, err := loadSubtitleText(state.Project.SubtitlePaths)
		if err != nil {
			warnLabel.SetText(err.Error())
			return
		}
		if strings.TrimSpace(subText) == "" {
			warnLabel.SetText("请先在「素材」添加/匹配字幕，或点「识别生成字幕」")
			return
		}

		generateBtn.Disable()
		warnLabel.SetText("生成中…")

		go func() {
			llm := script.NewLLM(state.Cfg.LLM.BaseURL, state.Cfg.LLM.APIKey, state.Cfg.LLM.Model)
			items, err := llm.GenerateScript(context.Background(), script.GenerateInput{
				SubtitleText:  subText,
				WordCount:     wordCount,
				OriginalRatio: origRatio,
			})
			fyne.Do(func() {
				generateBtn.Enable()
				if err != nil {
					warnLabel.SetText(fmt.Sprintf("生成失败: %v", err))
					return
				}
				state.Project.ScriptItems = items
				if err := project.Save(state.ProjectsRoot, state.Project); err != nil {
					warnLabel.SetText(fmt.Sprintf("保存失败: %v", err))
				} else {
					warnLabel.SetText(fmt.Sprintf("已生成 %d 条", len(items)))
				}
				refresh()
			})
		}()
	}

	params := container.NewGridWithColumns(4,
		labeledField("目标字数", wordEntry),
		labeledField("原片占比 %", origEntry),
		labeledField("目标时长(秒)", targetEntry),
		container.NewVBox(widget.NewLabel(" "), generateBtn),
	)
	header := container.NewVBox(
		container.NewBorder(nil, nil,
			widget.NewLabelWithStyle("脚本", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			nil, estimateLabel),
		params,
		warnLabel,
		status,
	)

	return container.NewBorder(container.NewPadded(header), nil, nil, nil, container.NewPadded(list)), refresh
}

func labeledField(title string, w fyne.CanvasObject) fyne.CanvasObject {
	lab := widget.NewLabel(title)
	lab.Importance = widget.LowImportance
	return container.NewVBox(lab, w)
}

func loadSubtitleText(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	var parts []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("读取字幕 %s: %w", p, err)
		}
		text := script.StripSpeakerLabels(string(data))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}
