//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/voiceclone"
)

// voicePicker is the reusable “查询已有音色 / 试听 / 点选 / 上传本地音频” block.
type voicePicker struct {
	mu           sync.Mutex
	selected     voiceclone.VoiceInfo
	hasSelected  bool
	previewPath  string
	previewText  string // text used to produce previewPath
	uploadedPath string // local wav/mp3/etc. used when no list voice is selected

	voicesAll  []voiceclone.VoiceInfo
	voicesView []voiceclone.VoiceInfo

	root         fyne.CanvasObject
	listBtn      *widget.Button
	uploadBtn    *widget.Button
	clearSelBtn  *widget.Button
	filterSelect *widget.Select
	searchEntry  *widget.Entry
	voiceList    *widget.List
	selLabel     *widget.Label
	listHint     *widget.Label
}

const voicePickerEmptyLabel = "未选择参考音色。查询后点一项即可选用，或点「上传音色」选用本地 wav、mp3 等音频。点「试听」可先听效果。"

func (p *voicePicker) Selection() (voiceclone.VoiceInfo, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.selected, p.previewPath, p.hasSelected
}

func (p *voicePicker) UploadedAudio() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasSelected && strings.TrimSpace(p.uploadedPath) != "" {
		return p.uploadedPath, true
	}
	return "", false
}

func (p *voicePicker) PreviewFor(text string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hasSelected && p.previewPath != "" && p.previewText == strings.TrimSpace(text) {
		return p.previewPath
	}
	return ""
}

func (p *voicePicker) rememberPreview(path, text string) {
	p.mu.Lock()
	p.previewPath = path
	p.previewText = strings.TrimSpace(text)
	p.mu.Unlock()
}

func (p *voicePicker) ClearSelection() {
	p.mu.Lock()
	p.selected = voiceclone.VoiceInfo{}
	p.hasSelected = false
	p.previewPath = ""
	p.previewText = ""
	p.uploadedPath = ""
	p.mu.Unlock()
	p.selLabel.SetText(voicePickerEmptyLabel)
	if p.voiceList != nil {
		p.voiceList.UnselectAll()
	}
}

func (p *voicePicker) SetEnabled(v bool) {
	if v {
		p.listBtn.Enable()
		p.uploadBtn.Enable()
		p.clearSelBtn.Enable()
		p.filterSelect.Enable()
		p.searchEntry.Enable()
		return
	}
	p.listBtn.Disable()
	p.uploadBtn.Disable()
	p.clearSelBtn.Disable()
	p.filterSelect.Disable()
	p.searchEntry.Disable()
}

func newVoicePicker(state *AppState, progress *widget.ProgressBar, status *widget.Label, previewText func() string, parentBusy func() bool, setBusy func(bool), setCancel func(context.CancelFunc)) *voicePicker {
	p := &voicePicker{}

	p.selLabel = widget.NewLabel(voicePickerEmptyLabel)
	p.selLabel.Wrapping = fyne.TextWrapWord
	p.selLabel.Importance = widget.LowImportance

	p.listHint = widget.NewLabel("还没有查询。点「查询已有音色」可拉取当前账号下的系统音色、已激活的复刻音色和文生音色；也可以直接「上传音色」。")
	p.listHint.Wrapping = fyne.TextWrapWord
	p.listHint.Importance = widget.LowImportance

	filterLabels := []string{"全部", "复刻音色", "文生音色", "系统音色"}
	filterKind := map[string]string{
		"全部":   voiceclone.VoiceTypeAll,
		"复刻音色": voiceclone.VoiceTypeCloning,
		"文生音色": voiceclone.VoiceTypeGeneration,
		"系统音色": voiceclone.VoiceTypeSystem,
	}

	applyFilter := func() {
		kind := voiceclone.VoiceTypeAll
		if p.filterSelect != nil {
			if k, ok := filterKind[p.filterSelect.Selected]; ok {
				kind = k
			}
		}
		filtered := voiceclone.FilterVoices(p.voicesAll, kind)
		query := ""
		if p.searchEntry != nil {
			query = strings.TrimSpace(p.searchEntry.Text)
		}
		if query != "" {
			q := strings.ToLower(query)
			out := make([]voiceclone.VoiceInfo, 0, len(filtered))
			for _, v := range filtered {
				blob := strings.ToLower(strings.Join([]string{v.Name, v.ID, v.Description, v.KindLabel(), v.CreatedTime}, " "))
				if strings.Contains(blob, q) {
					out = append(out, v)
				}
			}
			p.voicesView = out
		} else {
			p.voicesView = filtered
		}
		if p.voiceList != nil {
			p.voiceList.UnselectAll()
			p.voiceList.Refresh()
		}
		switch {
		case len(p.voicesAll) == 0:
			p.listHint.SetText("还没有查询。点「查询已有音色」可拉取当前账号下的系统音色、已激活的复刻音色和文生音色；也可以直接「上传音色」。")
		case len(filtered) == 0:
			p.listHint.SetText("这一类下暂时没有音色。刚复刻的音色要先成功合成一次，才能出现在列表里。")
		case len(p.voicesView) == 0:
			p.listHint.SetText("没有匹配当前筛选条件的音色，可改一下关键字或分类。")
		default:
			nClone, nGen, nSys := 0, 0, 0
			for _, v := range p.voicesAll {
				switch v.Kind {
				case voiceclone.VoiceTypeCloning:
					nClone++
				case voiceclone.VoiceTypeGeneration:
					nGen++
				default:
					nSys++
				}
			}
			extra := ""
			if query != "" {
				extra = fmt.Sprintf("，筛选后 %d 条", len(p.voicesView))
			}
			if kind == voiceclone.VoiceTypeAll && query == "" {
				p.listHint.SetText(fmt.Sprintf("共 %d 条（复刻 %d、文生 %d、系统 %d）。点一项选用，或点「试听」。", len(p.voicesView), nClone, nGen, nSys))
			} else {
				p.listHint.SetText(fmt.Sprintf("共 %d 条%s。点一项选用，或点「试听」。", len(filtered), extra))
			}
		}
	}

	markSelected := func(v voiceclone.VoiceInfo, preview string) {
		p.mu.Lock()
		p.selected = v
		p.hasSelected = true
		p.uploadedPath = ""
		if preview != "" {
			p.previewPath = preview
		}
		p.mu.Unlock()
		label := "已选参考音色：" + v.Display()
		if preview != "" {
			label += "。生成时会把试听音频作为参考音。"
		} else {
			label += "。生成前会先合成试听音频，再把它作为参考音。"
		}
		p.selLabel.SetText(label)
	}

	markUploaded := func(path string) {
		p.mu.Lock()
		p.selected = voiceclone.VoiceInfo{}
		p.hasSelected = false
		p.previewPath = ""
		p.previewText = ""
		p.uploadedPath = path
		p.mu.Unlock()
		if p.voiceList != nil {
			p.voiceList.UnselectAll()
		}
		p.selLabel.SetText("已选本地参考音频：" + filepath.Base(path) + "。生成时会把它作为参考音。")
	}

	var previewVoiceAt func(int)

	p.voiceList = widget.NewList(
		func() int { return len(p.voicesView) },
		func() fyne.CanvasObject {
			idx := widget.NewLabel("0")
			idx.Hide()
			title := widget.NewLabel("")
			title.Truncation = fyne.TextTruncateEllipsis
			meta := widget.NewLabel("")
			meta.Importance = widget.LowImportance
			meta.Truncation = fyne.TextTruncateEllipsis
			preview := widget.NewButton("试听", nil)
			preview.Importance = widget.LowImportance
			preview.OnTapped = func() {
				n, _ := strconv.Atoi(idx.Text)
				previewVoiceAt(n)
			}
			return container.NewBorder(nil, nil, idx, preview, container.NewVBox(title, meta))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			idx := box.Objects[1].(*widget.Label)
			text := box.Objects[0].(*fyne.Container)
			title := text.Objects[0].(*widget.Label)
			meta := text.Objects[1].(*widget.Label)
			if id < 0 || id >= len(p.voicesView) {
				idx.SetText("0")
				title.SetText("")
				meta.SetText("")
				return
			}
			v := p.voicesView[id]
			idx.SetText(strconv.Itoa(int(id)))
			name := v.Name
			if name == "" {
				name = v.ID
			}
			title.SetText(v.KindLabel() + " · " + name)
			parts := make([]string, 0, 3)
			if v.Description != "" {
				parts = append(parts, v.Description)
			}
			if v.Name != "" && v.Name != v.ID {
				parts = append(parts, v.ID)
			}
			if v.CreatedTime != "" && v.CreatedTime != "1970-01-01" {
				parts = append(parts, v.CreatedTime)
			}
			meta.SetText(strings.Join(parts, "  ·  "))
		},
	)
	p.voiceList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(p.voicesView) {
			return
		}
		markSelected(p.voicesView[id], "")
		status.SetText("已选用参考音色：" + p.voicesView[id].Display())
	}

	previewVoiceAt = func(i int) {
		if parentBusy() {
			return
		}
		if i < 0 || i >= len(p.voicesView) {
			return
		}
		if strings.TrimSpace(state.Cfg.MiniMax.APIKey) == "" {
			status.SetText("请先在「设置」里填写 MiniMax API Key")
			return
		}
		v := p.voicesView[i]
		text := ""
		if previewText != nil {
			text = strings.TrimSpace(previewText())
		}
		if text == "" {
			text = "你好，这是这段音色的试听。"
		}
		ctx, cancel := context.WithCancel(context.Background())
		setCancel(cancel)
		setBusy(true)
		markSelected(v, "")
		progress.SetValue(20)
		status.SetText("正在试听：" + v.Display())
		state.Log.Append("[口播视频] 试听音色 " + v.ID)
		go func() {
			client := &voiceclone.Client{
				BaseURL: state.Cfg.MiniMax.BaseURL,
				APIKey:  strings.TrimSpace(state.Cfg.MiniMax.APIKey),
				Model:   strings.TrimSpace(state.Cfg.MiniMax.Model),
			}
			var saved string
			var err error
			dir := filepath.Join(os.TempDir(), "xuanyu-voice-preview")
			if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
				err = mkErr
			} else if u := v.PreviewURL(); u != "" {
				saved = filepath.Join(dir, "intro_"+safeVoiceFileName(v.ID)+filepath.Ext(officialPreviewFileName(u, v.ID)))
				if st, stErr := os.Stat(saved); stErr != nil || st.Size() < 2048 {
					err = voiceclone.DownloadDemo(ctx, officialPreviewHTTPClient(), u, saved)
				}
			} else {
				var result *voiceclone.SynthesizeResult
				result, err = client.Synthesize(ctx, v.ID, text)
				if err == nil {
					saved = filepath.Join(dir, "intro_"+safeVoiceFileName(v.ID)+"_"+time.Now().Format("150405")+"."+extOrMP3(result.Format))
					err = os.WriteFile(saved, result.Audio, 0o644)
				}
			}
			fyne.Do(func() {
				setCancel(nil)
				setBusy(false)
				if err != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText(err.Error())
					state.Log.Append("[口播视频] 试听失败: " + err.Error())
					dialog.ShowError(err, state.Win)
					return
				}
				p.mu.Lock()
				p.previewPath = saved
				p.previewText = text
				p.mu.Unlock()
				markSelected(v, saved)
				progress.SetValue(100)
				status.SetText("试听已打开: " + saved)
				state.Log.Append("[口播视频] 试听完成: " + saved)
				if openErr := media.OpenFile(saved); openErr != nil {
					dialog.ShowError(openErr, state.Win)
				}
			})
		}()
	}

	p.filterSelect = widget.NewSelect(filterLabels, func(string) { applyFilter() })
	p.filterSelect.SetSelected("全部")

	p.searchEntry = widget.NewEntry()
	p.searchEntry.SetPlaceHolder("按名称、音色 ID 或描述筛选")
	p.searchEntry.OnChanged = func(string) { applyFilter() }

	p.listBtn = widget.NewButton("查询已有音色", func() {
		if parentBusy() {
			return
		}
		if strings.TrimSpace(state.Cfg.MiniMax.APIKey) == "" {
			status.SetText("请先在「设置」里填写 MiniMax API Key")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		setCancel(cancel)
		setBusy(true)
		progress.SetValue(20)
		status.SetText("正在查询已有音色…")
		state.Log.Append("[口播视频] 查询参考音色")
		go func() {
			client := &voiceclone.Client{
				BaseURL: state.Cfg.MiniMax.BaseURL,
				APIKey:  strings.TrimSpace(state.Cfg.MiniMax.APIKey),
				Model:   strings.TrimSpace(state.Cfg.MiniMax.Model),
			}
			voices, err := client.ListVoices(ctx, voiceclone.VoiceTypeAll)
			fyne.Do(func() {
				setCancel(nil)
				setBusy(false)
				if err != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText(err.Error())
					state.Log.Append("[口播视频] 查询音色失败: " + err.Error())
					dialog.ShowError(err, state.Win)
					return
				}
				p.voicesAll = voices
				applyFilter()
				progress.SetValue(100)
				status.SetText(fmt.Sprintf("已查询到 %d 条音色", len(voices)))
				state.Log.Append(fmt.Sprintf("[口播视频] 查询音色完成，共 %d 条", len(voices)))
			})
		}()
	})

	p.uploadBtn = widget.NewButton("上传音色", func() {
		if parentBusy() {
			return
		}
		paths, err := pickAudioFiles()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if len(paths) == 0 {
			return
		}
		path := strings.TrimSpace(paths[0])
		if path == "" {
			return
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			dialog.ShowError(fmt.Errorf("无法读取音频文件: %w", statErr), state.Win)
			return
		}
		if info.Size() == 0 {
			status.SetText("这个音频文件是空的，请换一个")
			return
		}
		markUploaded(path)
		status.SetText("已选用本地参考音频：" + path)
		state.Log.Append("[口播视频] 本地参考音频 " + path)
	})

	p.clearSelBtn = widget.NewButton("取消选用", func() {
		if parentBusy() {
			return
		}
		p.ClearSelection()
		status.SetText("已取消参考音色")
	})
	p.clearSelBtn.Importance = widget.LowImportance

	sec := sectionTitle
	top := container.NewVBox(
		sec("参考音色"),
		container.NewBorder(nil, nil, widget.NewLabel("分类"), container.NewHBox(p.listBtn, p.uploadBtn, p.clearSelBtn), p.filterSelect),
		p.searchEntry,
		p.listHint,
		p.selLabel,
	)
	p.root = container.NewBorder(top, nil, nil, nil, container.NewPadded(p.voiceList))
	return p
}
