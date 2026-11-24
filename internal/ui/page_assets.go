//go:build fyne_gui

package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/project"
)

func newAssetsPage(state *AppState) (fyne.CanvasObject, func()) {
	status := widget.NewLabel("")
	status.Importance = widget.MediumImportance

	type row struct {
		video string
		sub   string
	}
	var rows []row
	selectedRow := -1

	table := widget.NewTableWithHeaders(
		func() (int, int) { return len(rows), 3 },
		func() fyne.CanvasObject {
			return container.NewStack(widget.NewLabel("template"))
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			if id.Row < 0 || id.Row >= len(rows) {
				return
			}
			r := rows[id.Row]
			switch id.Col {
			case 0:
				box.Objects = []fyne.CanvasObject{widget.NewLabel(fmt.Sprintf("%d", id.Row+1))}
			case 1:
				lab := widget.NewLabel(filepath.Base(r.video))
				lab.Truncation = fyne.TextTruncateEllipsis
				box.Objects = []fyne.CanvasObject{lab}
			case 2:
				text := "未匹配"
				if strings.TrimSpace(r.sub) != "" {
					text = filepath.Base(r.sub)
				}
				lab := widget.NewLabel(text)
				lab.Truncation = fyne.TextTruncateEllipsis
				if strings.TrimSpace(r.sub) == "" {
					lab.Importance = widget.LowImportance
				}
				box.Objects = []fyne.CanvasObject{lab}
			}
			box.Refresh()
		},
	)
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabel("header")
	}
	table.UpdateHeader = func(id widget.TableCellID, obj fyne.CanvasObject) {
		label := obj.(*widget.Label)
		label.TextStyle = fyne.TextStyle{Bold: true}
		switch id.Col {
		case 0:
			label.SetText("#")
		case 1:
			label.SetText("视频")
		case 2:
			label.SetText("字幕")
		default:
			label.SetText("")
		}
	}
	table.SetColumnWidth(0, 48)
	table.SetColumnWidth(1, 420)
	table.SetColumnWidth(2, 360)
	table.OnSelected = func(id widget.TableCellID) {
		selectedRow = id.Row
	}

	syncRows := func() {
		rows = nil
		selectedRow = -1
		if state.Project == nil {
			table.Refresh()
			return
		}
		subs := media.AlignSubtitlesToVideos(state.Project.VideoPaths, state.Project.SubtitlePaths)
		for i, v := range state.Project.VideoPaths {
			sub := ""
			if i < len(subs) {
				sub = subs[i]
			}
			rows = append(rows, row{video: v, sub: sub})
		}
		table.Refresh()
	}

	persist := func() {
		if err := saveProject(state); err != nil {
			status.SetText("保存失败: " + err.Error())
			return
		}
		syncRows()
	}

	refresh := func() {
		syncRows()
		if state.Project == nil {
			status.SetText("请先在「短剧解说」打开或新建")
			return
		}
		status.SetText(fmt.Sprintf("%d 个视频，%d 个已匹配字幕",
			len(state.Project.VideoPaths), countNonEmpty(state.Project.SubtitlePaths)))
	}

	requireProject := func() bool {
		if state.Project == nil {
			status.SetText("请先打开或新建工程")
			return false
		}
		return true
	}

	addVideos := func(paths []string, autoMatch bool) {
		if state.Project == nil || len(paths) == 0 {
			return
		}
		aligned := media.AlignSubtitlesToVideos(state.Project.VideoPaths, state.Project.SubtitlePaths)
		state.Project.SubtitlePaths = aligned
		added := 0
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			exists := false
			for _, old := range state.Project.VideoPaths {
				if strings.EqualFold(filepath.Clean(old), filepath.Clean(path)) {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			state.Project.VideoPaths = append(state.Project.VideoPaths, path)
			sub := ""
			if autoMatch {
				sub = media.FindSidecarSubtitle(path)
			}
			state.Project.SubtitlePaths = append(state.Project.SubtitlePaths, sub)
			added++
		}
		status.SetText(fmt.Sprintf("已添加 %d 个视频", added))
		persist()
	}

	setSubAt := func(idx int, sub string) {
		if state.Project == nil || idx < 0 || idx >= len(state.Project.VideoPaths) {
			return
		}
		for len(state.Project.SubtitlePaths) < len(state.Project.VideoPaths) {
			state.Project.SubtitlePaths = append(state.Project.SubtitlePaths, "")
		}
		state.Project.SubtitlePaths[idx] = sub
		persist()
	}

	addVideo := widget.NewButton("添加视频", func() {
		if !requireProject() {
			return
		}
		paths, err := pickVideoFiles()
		if err != nil {
			status.SetText(err.Error())
			return
		}
		addVideos(paths, true)
	})
	addVideo.Importance = widget.HighImportance

	batchVideos := widget.NewButton("从文件夹", func() {
		if !requireProject() {
			return
		}
		dir, err := pickDirectory("选择视频文件夹")
		if err != nil || dir == "" {
			if err != nil {
				status.SetText(err.Error())
			}
			return
		}
		videos, err := media.ListVideosInDir(dir)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		addVideos(videos, true)
	})

	matchBtn := widget.NewButton("匹配同名", func() {
		if !requireProject() {
			return
		}
		aligned := media.AlignSubtitlesToVideos(state.Project.VideoPaths, state.Project.SubtitlePaths)
		filled := 0
		for i, v := range state.Project.VideoPaths {
			if strings.TrimSpace(aligned[i]) != "" {
				continue
			}
			if sub := media.FindSidecarSubtitle(v); sub != "" {
				aligned[i] = sub
				filled++
			}
		}
		state.Project.SubtitlePaths = aligned
		status.SetText(fmt.Sprintf("匹配完成，新填充 %d 个", filled))
		persist()
	})

	pickSubBtn := widget.NewButton("为选中行选字幕", func() {
		if !requireProject() {
			return
		}
		if selectedRow < 0 || selectedRow >= len(rows) {
			status.SetText("请先在表格中点选一行")
			return
		}
		paths, err := pickSubtitleFiles()
		if err != nil || len(paths) == 0 {
			if err != nil {
				status.SetText(err.Error())
			}
			return
		}
		setSubAt(selectedRow, paths[0])
		status.SetText("已为该视频设置字幕")
	})

	clearSubBtn := widget.NewButton("清除行字幕", func() {
		if !requireProject() {
			return
		}
		if selectedRow < 0 || selectedRow >= len(rows) {
			status.SetText("请先点选一行")
			return
		}
		setSubAt(selectedRow, "")
	})

	removeVideoBtn := widget.NewButton("移除行", func() {
		if !requireProject() {
			return
		}
		i := selectedRow
		if i < 0 || i >= len(state.Project.VideoPaths) {
			status.SetText("请先点选一行")
			return
		}
		subs := media.AlignSubtitlesToVideos(state.Project.VideoPaths, state.Project.SubtitlePaths)
		state.Project.VideoPaths = append(state.Project.VideoPaths[:i], state.Project.VideoPaths[i+1:]...)
		subs = append(subs[:i], subs[i+1:]...)
		state.Project.SubtitlePaths = subs
		persist()
	})

	recognizeSelectedBtn := widget.NewButton("识别选中行", nil)
	recognizeSelectedBtn.Importance = widget.HighImportance
	recognizeAllBtn := widget.NewButton("全部识别", nil)

	var recognizeBusy bool
	setRecognizeBusy := func(busy bool) {
		recognizeBusy = busy
		if busy {
			recognizeSelectedBtn.Disable()
			recognizeAllBtn.Disable()
			return
		}
		recognizeSelectedBtn.Enable()
		recognizeAllBtn.Enable()
	}

	runRecognize := func(indices []int) {
		if !requireProject() {
			return
		}
		if recognizeBusy {
			return
		}
		if len(state.Project.VideoPaths) == 0 {
			status.SetText("请先添加视频")
			return
		}
		if len(indices) == 0 {
			status.SetText("没有可识别的视频")
			return
		}
		opt := asr.Options{
			Backend:       state.Cfg.ASR.Backend,
			APIURL:        state.Cfg.ASR.APIURL,
			FireRedAPIURL: state.Cfg.ASR.FireRedAPIURL,
			BailianAPIKey: state.Cfg.ASR.APIKey,
			Hotword:       state.Cfg.ASR.Hotword,
			EnableSpk:     state.Cfg.ASR.EnableSpk,
			FFmpeg:        state.Cfg.FFmpeg.Path,
		}
		if err := asr.RequireReady(opt); err != nil {
			status.SetText(err.Error())
			return
		}
		label := asr.BackendLabel(opt.Backend)
		projDir := project.Dir(state.ProjectsRoot, state.Project.Slug)
		videos := append([]string{}, state.Project.VideoPaths...)
		for len(state.Project.SubtitlePaths) < len(videos) {
			state.Project.SubtitlePaths = append(state.Project.SubtitlePaths, "")
		}
		results := append([]string{}, state.Project.SubtitlePaths...)
		setRecognizeBusy(true)
		status.SetText(fmt.Sprintf("正在识别 %d 个视频（%s）…", len(indices), label))

		go func() {
			var lastErr error
			okCount := 0
			for n, idx := range indices {
				if idx < 0 || idx >= len(videos) {
					continue
				}
				v := videos[idx]
				out := asr.UniquePath(asr.OutPathFor(projDir, v, opt.Backend))
				fyne.Do(func() {
					status.SetText(fmt.Sprintf("识别中 %d/%d（%s）: %s", n+1, len(indices), label, filepath.Base(v)))
				})
				if err := asr.Transcribe(v, out, opt); err != nil {
					lastErr = err
					continue
				}
				results[idx] = out
				okCount++
			}
			fyne.Do(func() {
				setRecognizeBusy(false)
				if okCount == 0 {
					msg := "识别失败"
					if lastErr != nil {
						msg += ": " + lastErr.Error()
					}
					status.SetText(msg)
					return
				}
				if state.Project != nil {
					state.Project.SubtitlePaths = results
				}
				status.SetText(fmt.Sprintf("已生成 %d 个字幕（%s）", okCount, label))
				if lastErr != nil {
					status.SetText(status.Text + "；部分失败: " + lastErr.Error())
				}
				persist()
			})
		}()
	}

	recognizeSelectedBtn.OnTapped = func() {
		if selectedRow < 0 || selectedRow >= len(rows) {
			status.SetText("请先在表格中点选一行")
			return
		}
		runRecognize([]int{selectedRow})
	}
	recognizeAllBtn.OnTapped = func() {
		if state.Project == nil || len(state.Project.VideoPaths) == 0 {
			status.SetText("请先添加视频")
			return
		}
		indices := make([]int, len(state.Project.VideoPaths))
		for i := range indices {
			indices[i] = i
		}
		runRecognize(indices)
	}

	for _, b := range []*widget.Button{batchVideos, matchBtn, pickSubBtn, clearSubBtn, removeVideoBtn, recognizeAllBtn} {
		b.Importance = widget.LowImportance
	}

	toolbar := container.NewHBox(
		addVideo, batchVideos, matchBtn, widget.NewSeparator(),
		pickSubBtn, clearSubBtn, removeVideoBtn, widget.NewSeparator(),
		recognizeSelectedBtn, recognizeAllBtn,
	)

	top := container.NewVBox(
		container.NewBorder(nil, nil,
			pageTitle("素材"),
			nil, status),
		toolbar,
	)

	return container.NewBorder(container.NewPadded(top), nil, nil, nil, container.NewPadded(table)), refresh
}
