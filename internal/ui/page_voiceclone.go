//go:build fyne_gui

package ui

import (
	"context"
	"encoding/json"
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

func newVoiceClonePage(state *AppState) fyne.CanvasObject {
	var (
		mu         sync.Mutex
		busy       bool
		cancelRun  context.CancelFunc
		srcPath    string
		outDir     string
		lastOut    string
		voicesAll  []voiceclone.VoiceInfo
		voicesView []voiceclone.VoiceInfo
	)

	subtitle := widget.NewLabel("上传一段参考音，用 MiniMax 复刻音色，再用这段音色合成口播。也可以查询账号里已有的音色直接使用。")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	pathLabel := widget.NewLabel("未选择参考音")
	pathLabel.Wrapping = fyne.TextWrapWord
	pathLabel.Truncation = fyne.TextTruncateEllipsis

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100

	voiceID := widget.NewEntry()
	voiceID.SetPlaceHolder("留空则自动生成，例如 XuanyuA1B2C3；也可从列表点选")

	noiseCheck := widget.NewCheck("上传前降噪", nil)

	textEntry := widget.NewMultiLineEntry()
	textEntry.Wrapping = fyne.TextWrapWord
	textEntry.SetMinRowsVisible(4)
	textEntry.SetPlaceHolder("选填。克隆时可生成试听；也可以在克隆完成后，用已有音色 ID 再点「合成配音」。")

	hint := widget.NewLabel("参考音需 10 秒到 5 分钟，格式为 mp3、m4a 或 wav，不超过 20 MB。选择视频会先抽出音轨；超过 5 分钟时只用前 5 分钟。请先在「设置」里填写 MiniMax API Key。复刻得到的音色若 7 天内未用于合成，平台会删除。刚复刻的音色要先成功合成一次，才能出现在查询列表里。")
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	listHint := widget.NewLabel("还没有查询。点「查询已有音色」可拉取当前账号下的系统音色、已激活的复刻音色和文生音色。")
	listHint.Wrapping = fyne.TextWrapWord
	listHint.Importance = widget.LowImportance

	filterLabels := []string{"全部", "复刻音色", "文生音色", "系统音色"}
	filterKind := map[string]string{
		"全部":   voiceclone.VoiceTypeAll,
		"复刻音色": voiceclone.VoiceTypeCloning,
		"文生音色": voiceclone.VoiceTypeGeneration,
		"系统音色": voiceclone.VoiceTypeSystem,
	}

	var pickAudioBtn, pickVideoBtn, cloneBtn, synthBtn, listBtn, cancelBtn, openBtn, clearBtn *widget.Button
	var filterSelect *widget.Select
	var searchEntry *widget.Entry
	var voiceList *widget.List
	var setBusy func(bool)
	var clientFromSettings func() *voiceclone.Client

	applyFilter := func() {
		kind := voiceclone.VoiceTypeAll
		if filterSelect != nil {
			if k, ok := filterKind[filterSelect.Selected]; ok {
				kind = k
			}
		}
		filtered := voiceclone.FilterVoices(voicesAll, kind)
		query := ""
		if searchEntry != nil {
			query = strings.TrimSpace(searchEntry.Text)
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
			voicesView = out
		} else {
			voicesView = filtered
		}
		if voiceList != nil {
			voiceList.UnselectAll()
			voiceList.Refresh()
		}
		switch {
		case len(voicesAll) == 0:
			listHint.SetText("还没有查询。点「查询已有音色」可拉取当前账号下的系统音色、已激活的复刻音色和文生音色。")
		case len(filtered) == 0:
			listHint.SetText("这一类下暂时没有音色。刚复刻的音色要先成功合成一次，才能出现在列表里。")
		case len(voicesView) == 0:
			listHint.SetText("没有匹配当前筛选条件的音色，可改一下关键字或分类。")
		default:
			nClone, nGen, nSys := 0, 0, 0
			for _, v := range voicesAll {
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
				extra = fmt.Sprintf("，筛选后 %d 条", len(voicesView))
			}
			if kind == voiceclone.VoiceTypeAll && query == "" {
				listHint.SetText(fmt.Sprintf("共 %d 条（复刻 %d、文生 %d、系统 %d）。点一项填入音色 ID，或点「试听」合成一小段试听。", len(voicesView), nClone, nGen, nSys))
			} else {
				listHint.SetText(fmt.Sprintf("共 %d 条%s。点一项填入音色 ID，或点「试听」。", len(filtered), extra))
			}
		}
	}

	var previewVoiceAt func(int)

	voiceList = widget.NewList(
		func() int { return len(voicesView) },
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
			if id < 0 || id >= len(voicesView) {
				idx.SetText("0")
				title.SetText("")
				meta.SetText("")
				return
			}
			v := voicesView[id]
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
	voiceList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(voicesView) {
			return
		}
		voiceID.SetText(voicesView[id].ID)
		status.SetText("已选用音色：" + voicesView[id].Display())
	}

	previewVoiceAt = func(i int) {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		if i < 0 || i >= len(voicesView) {
			mu.Unlock()
			return
		}
		if strings.TrimSpace(state.Cfg.MiniMax.APIKey) == "" {
			mu.Unlock()
			status.SetText("请先在「设置」里填写 MiniMax API Key")
			return
		}
		v := voicesView[i]
		text := strings.TrimSpace(textEntry.Text)
		if text == "" {
			text = "你好，这是这段音色的试听。"
		} else {
			runes := []rune(text)
			if len(runes) > 80 {
				text = string(runes[:80])
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		mu.Unlock()
		voiceID.SetText(v.ID)
		progress.SetValue(20)
		status.SetText("正在试听：" + v.Display())
		state.Log.Append("[克隆音频] 试听音色 " + v.ID)
		go func() {
			client := clientFromSettings()
			result, err := client.Synthesize(ctx, v.ID, text)
			var saved string
			if err == nil {
				dir := filepath.Join(os.TempDir(), "xuanyu-voice-preview")
				if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
					err = mkErr
				} else {
					saved = filepath.Join(dir, "preview_"+safeVoiceFileName(v.ID)+"."+extOrMP3(result.Format))
					err = os.WriteFile(saved, result.Audio, 0o644)
				}
			}
			fyne.Do(func() {
				mu.Lock()
				cancelRun = nil
				if saved != "" {
					lastOut = saved
				}
				setBusy(false)
				mu.Unlock()
				if err != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText(err.Error())
					state.Log.Append("[克隆音频] 试听失败: " + err.Error())
					dialog.ShowError(err, state.Win)
					return
				}
				progress.SetValue(100)
				status.SetText("试听已打开: " + saved)
				state.Log.Append("[克隆音频] 试听完成: " + saved)
				if openErr := media.OpenFile(saved); openErr != nil {
					dialog.ShowError(openErr, state.Win)
				}
			})
		}()
	}

	filterSelect = widget.NewSelect(filterLabels, func(string) { applyFilter() })
	filterSelect.SetSelected("全部")

	searchEntry = widget.NewEntry()
	searchEntry.SetPlaceHolder("按名称、音色 ID 或描述筛选")
	searchEntry.OnChanged = func(string) { applyFilter() }

	setBusy = func(v bool) {
		busy = v
		if v {
			pickAudioBtn.Disable()
			pickVideoBtn.Disable()
			cloneBtn.Disable()
			synthBtn.Disable()
			listBtn.Disable()
			clearBtn.Disable()
			cancelBtn.Enable()
			openBtn.Disable()
			return
		}
		pickAudioBtn.Enable()
		pickVideoBtn.Enable()
		cloneBtn.Enable()
		synthBtn.Enable()
		listBtn.Enable()
		clearBtn.Enable()
		cancelBtn.Disable()
		if lastOut != "" || outDir != "" {
			openBtn.Enable()
		}
	}

	clientFromSettings = func() *voiceclone.Client {
		return &voiceclone.Client{
			BaseURL: state.Cfg.MiniMax.BaseURL,
			APIKey:  strings.TrimSpace(state.Cfg.MiniMax.APIKey),
			Model:   strings.TrimSpace(state.Cfg.MiniMax.Model),
		}
	}

	setSource := func(path string) {
		srcPath = path
		outDir = ""
		lastOut = ""
		pathLabel.SetText(path)
		progress.SetValue(0)
		status.SetText("")
		openBtn.Disable()
	}

	pickAudioBtn = widget.NewButton("选择音频", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		paths, err := pickAudioFiles()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if len(paths) == 0 {
			return
		}
		mu.Lock()
		setSource(paths[0])
		mu.Unlock()
	})

	pickVideoBtn = widget.NewButton("选择视频", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		path, err := pickOneVideoFile()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if path == "" {
			return
		}
		mu.Lock()
		setSource(path)
		mu.Unlock()
	})

	openBtn = widget.NewButton("打开输出", func() {
		mu.Lock()
		dir := outDir
		if dir == "" && lastOut != "" {
			dir = filepath.Dir(lastOut)
		}
		mu.Unlock()
		if dir == "" {
			return
		}
		_ = media.OpenFile(dir)
	})
	openBtn.Disable()

	cancelBtn = widget.NewButton("取消", func() {
		mu.Lock()
		fn := cancelRun
		mu.Unlock()
		if fn != nil {
			fn()
		}
	})
	cancelBtn.Disable()

	clearBtn = widget.NewButton("清空", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		srcPath = ""
		outDir = ""
		lastOut = ""
		mu.Unlock()
		pathLabel.SetText("未选择参考音")
		voiceID.SetText("")
		textEntry.SetText("")
		noiseCheck.SetChecked(false)
		progress.SetValue(0)
		status.SetText("")
		openBtn.Disable()
		if voiceList != nil {
			voiceList.UnselectAll()
		}
	})
	clearBtn.Importance = widget.LowImportance

	listBtn = widget.NewButton("查询已有音色", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		if strings.TrimSpace(state.Cfg.MiniMax.APIKey) == "" {
			mu.Unlock()
			status.SetText("请先在「设置」里填写 MiniMax API Key")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		mu.Unlock()
		progress.SetValue(20)
		status.SetText("正在查询已有音色…")
		state.Log.Append("[克隆音频] 查询已有音色")

		go func() {
			client := clientFromSettings()
			voices, err := client.ListVoices(ctx, voiceclone.VoiceTypeAll)
			fyne.Do(func() {
				mu.Lock()
				cancelRun = nil
				if err == nil {
					voicesAll = voices
				}
				setBusy(false)
				mu.Unlock()
				if err != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText(err.Error())
					state.Log.Append("[克隆音频] 查询失败: " + err.Error())
					dialog.ShowError(err, state.Win)
					return
				}
				applyFilter()
				progress.SetValue(100)
				status.SetText(fmt.Sprintf("已查询到 %d 条音色", len(voices)))
				state.Log.Append(fmt.Sprintf("[克隆音频] 查询完成，共 %d 条", len(voices)))
			})
		}()
	})

	run := func(kind string) {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		src := strings.TrimSpace(srcPath)
		wantID := strings.TrimSpace(voiceID.Text)
		text := textEntry.Text
		denoise := noiseCheck.Checked
		if kind == "clone" && src == "" {
			mu.Unlock()
			status.SetText("请先选择参考音或视频")
			return
		}
		if kind == "synth" {
			if strings.TrimSpace(wantID) == "" {
				mu.Unlock()
				status.SetText("请先克隆音色，或从已有音色中选择")
				return
			}
			if strings.TrimSpace(text) == "" {
				mu.Unlock()
				status.SetText("请填写要合成的文案")
				return
			}
		}
		if strings.TrimSpace(state.Cfg.MiniMax.APIKey) == "" {
			mu.Unlock()
			status.SetText("请先在「设置」里填写 MiniMax API Key")
			return
		}
		bins, binsErr := resolveFFmpeg(state)
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		mu.Unlock()
		if binsErr != nil && kind == "clone" {
			// Duration / conversion need ffmpeg; still try later after a clear error.
		}

		progress.SetValue(8)
		if kind == "clone" {
			status.SetText("正在准备参考音…")
		} else {
			status.SetText("正在合成配音…")
		}
		state.Log.Append("[克隆音频] 开始 " + kind)

		go func() {
			var runErr error
			var saved string
			var clonedID string
			client := clientFromSettings()

			report := func(p int, msg string) {
				fyne.Do(func() {
					progress.SetValue(float64(p))
					if msg != "" {
						status.SetText(msg)
					}
				})
			}

			if kind == "clone" {
				dir := voiceclone.OutputDir(src, time.Now())
				if err := os.MkdirAll(dir, 0o755); err != nil {
					runErr = err
				} else {
					fyne.Do(func() {
						mu.Lock()
						outDir = dir
						mu.Unlock()
					})
					report(15, "正在检查参考音…")
					if binsErr != nil {
						runErr = binsErr
					} else {
						var prepared *voiceclone.PreparedAudio
						prepared, runErr = voiceclone.PrepareAudio(ctx, bins, src, dir)
						if runErr == nil {
							if prepared.Trimmed {
								report(25, "参考音超过 5 分钟，已截取前 5 分钟并上传…")
							} else {
								report(30, "正在上传参考音…")
							}
							var fileID string
							fileID, runErr = client.UploadVoiceClone(ctx, prepared.Path)
							if runErr == nil {
								report(60, "正在复刻音色…")
								var result *voiceclone.CloneResult
								result, runErr = client.CloneVoice(ctx, fileID, wantID, voiceclone.CloneOptions{
									Text:               text,
									NeedNoiseReduction: denoise,
								})
								if runErr == nil {
									clonedID = result.VoiceID
									metaPath := filepath.Join(dir, "clone.json")
									_ = writeCloneMeta(metaPath, map[string]any{
										"voice_id":   result.VoiceID,
										"file_id":    result.FileID,
										"source":     src,
										"prepared":   prepared.Path,
										"model":      client.Model,
										"created_at": time.Now().Format(time.RFC3339),
									})
									if strings.TrimSpace(result.DemoAudio) != "" {
										report(80, "正在下载试听音频…")
										demo := filepath.Join(dir, "preview.mp3")
										if err := voiceclone.DownloadDemo(ctx, nil, result.DemoAudio, demo); err != nil {
											runErr = err
										} else {
											saved = demo
										}
									}
									if saved == "" {
										saved = metaPath
									}
								}
							}
						}
					}
				}
			} else {
				dir := outDir
				if dir == "" && src != "" {
					dir = voiceclone.OutputDir(src, time.Now())
				}
				if dir == "" {
					dir = filepath.Join(os.TempDir(), "voiceclone_"+time.Now().Format("20060102_150405"))
				}
				if err := os.MkdirAll(dir, 0o755); err != nil {
					runErr = err
				} else {
					fyne.Do(func() {
						mu.Lock()
						outDir = dir
						mu.Unlock()
					})
					report(30, "正在合成配音…")
					var result *voiceclone.SynthesizeResult
					result, runErr = client.Synthesize(ctx, wantID, text)
					if runErr == nil {
						saved = filepath.Join(dir, "tts."+extOrMP3(result.Format))
						runErr = os.WriteFile(saved, result.Audio, 0o644)
					}
				}
			}

			fyne.Do(func() {
				mu.Lock()
				cancelRun = nil
				if saved != "" {
					lastOut = saved
				}
				if clonedID != "" {
					voiceID.SetText(clonedID)
				}
				setBusy(false)
				mu.Unlock()
				if runErr != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText(runErr.Error())
					state.Log.Append("[克隆音频] 失败: " + runErr.Error())
					dialog.ShowError(runErr, state.Win)
					return
				}
				progress.SetValue(100)
				if kind == "clone" {
					if strings.Contains(saved, "preview.mp3") {
						status.SetText("音色已复刻，试听已保存: " + saved + "（音色 ID: " + clonedID + "）")
					} else {
						status.SetText("音色已复刻，音色 ID: " + clonedID + "。可继续填写文案后点「合成配音」。刚复刻的音色要先成功合成一次，才能出现在查询列表里。")
					}
				} else {
					status.SetText("配音已保存: " + saved)
				}
				state.Log.Append("[克隆音频] 完成: " + saved)
			})
		}()
	}

	cloneBtn = widget.NewButton("克隆音色", func() { run("clone") })
	cloneBtn.Importance = widget.HighImportance
	synthBtn = widget.NewButton("合成配音", func() { run("synth") })

	sec := sectionTitle
	toolbar := container.NewHBox(pickAudioBtn, pickVideoBtn, cloneBtn, synthBtn, cancelBtn, openBtn, clearBtn)
	top := container.NewVBox(
		pageTitle("克隆音频"),
		subtitle,
		toolbar,
		pathLabel,
		progress,
		status,
		sec("已有音色"),
		container.NewBorder(nil, nil, widget.NewLabel("分类"), listBtn, filterSelect),
		searchEntry,
		listHint,
	)
	bottom := container.NewVBox(
		sec("音色 ID"),
		voiceID,
		noiseCheck,
		sec("合成文案"),
		textEntry,
		hint,
	)
	return container.NewBorder(container.NewPadded(top), container.NewPadded(bottom), nil, nil, container.NewPadded(voiceList))
}

func writeCloneMeta(path string, data map[string]any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func extOrMP3(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "mp3"
	}
	return strings.TrimPrefix(format, ".")
}

func safeVoiceFileName(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		s = "voice"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
