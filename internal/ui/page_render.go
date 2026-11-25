//go:build fyne_gui

package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/pipeline"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/tts"
	"github.com/narratoai/narrato-go/internal/voiceclone"
)

const (
	engineLabelEdge      = "Edge TTS"
	engineLabelMiniMax   = "MiniMax"
	engineLabelKokoro    = "Kokoro"
	engineLabelOpenVoice = "OpenVoice"
	engineLabelIndexTTS2 = "IndexTTS-2"
)

func newRenderPage(state *AppState) fyne.CanvasObject {
	var (
		mu          sync.Mutex
		busy        bool
		cancelFn    context.CancelFunc
		lastOutput  string
		previewBusy bool
		mmVoices    []voiceclone.VoiceInfo
		mmIDByLabel = map[string]string{}
		kkIDByLabel = map[string]string{}
		ovIDByLabel = map[string]string{}
		ixIDByLabel = map[string]string{}
		ovRefPath   = ""
		ixRefPath   = ""
	)
	if state.Cfg != nil {
		ovRefPath = strings.TrimSpace(state.Cfg.TTS.OpenVoiceReference)
		ixRefPath = strings.TrimSpace(state.Cfg.TTS.IndexTTS2Reference)
	}

	logEntry := widget.NewMultiLineEntry()
	logEntry.SetMinRowsVisible(10)
	logEntry.Wrapping = fyne.TextWrapWord
	refreshLog := func() {
		logEntry.SetText(state.Log.String())
		logEntry.CursorRow = len(logEntry.Text)
	}
	appendLog := func(line string) {
		state.Log.Append(line)
		refreshLog()
	}

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100

	engineSelect := widget.NewSelect([]string{engineLabelEdge, engineLabelMiniMax, engineLabelKokoro, engineLabelOpenVoice, engineLabelIndexTTS2}, nil)
	switch tts.NormalizeEngine(state.Cfg.TTS.Engine) {
	case tts.EngineMiniMax:
		engineSelect.SetSelected(engineLabelMiniMax)
	case tts.EngineKokoro:
		engineSelect.SetSelected(engineLabelKokoro)
	case tts.EngineOpenVoice:
		engineSelect.SetSelected(engineLabelOpenVoice)
	case tts.EngineIndexTTS2:
		engineSelect.SetSelected(engineLabelIndexTTS2)
	default:
		engineSelect.SetSelected(engineLabelEdge)
	}

	voiceSelect := newScrollSelect(nil, nil)
	voiceSelect.PlaceHolder = "选择配音"

	var refreshMiniMaxBtn, previewBtn, previewBgmBtn, startBtn, cancelBtn, openBtn *widget.Button

	shortSlide := func(s *widget.Slider, trail *widget.Label) fyne.CanvasObject {
		return container.NewHBox(container.New(&fixedWidthLayout{width: 140}, s), trail)
	}

	var collectRenderForm func(*config.Config)

	setVoiceOptions := func(labels []string, selected string, allowExtra bool) {
		if selected != "" {
			found := false
			for _, lab := range labels {
				if lab == selected {
					found = true
					break
				}
			}
			if !found && allowExtra {
				labels = append([]string{selected}, labels...)
			} else if !found {
				selected = ""
			}
		}
		voiceSelect.Options = labels
		if selected != "" {
			voiceSelect.SetSelected(selected)
		} else if len(labels) > 0 {
			voiceSelect.SetSelected(labels[0])
		} else {
			voiceSelect.ClearSelected()
		}
		voiceSelect.Refresh()
	}

	fillEdgeVoices := func() {
		labels := tts.VoiceLabels()
		current := ""
		if tts.IsEdgeVoice(state.Cfg.TTS.Voice) {
			current = tts.LabelForVoiceID(state.Cfg.TTS.Voice)
		}
		setVoiceOptions(labels, current, false)
	}

	fillMiniMaxVoices := func(voices []voiceclone.VoiceInfo) {
		mmVoices = voices
		mmIDByLabel = map[string]string{}
		sorted := append([]voiceclone.VoiceInfo(nil), voices...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return tts.LessMiniMax(sorted[i].ID, sorted[i].Kind, sorted[j].ID, sorted[j].Kind)
		})
		labels := make([]string, 0, len(sorted)+1)
		for _, v := range sorted {
			level := tts.MiniMaxVoiceLevel(v.ID, v.Kind)
			lab := tts.WithRatingMark(v.Display(), level)
			mmIDByLabel[lab] = v.ID
			labels = append(labels, lab)
		}
		selected := ""
		want := state.Cfg.TTS.Voice
		for _, v := range sorted {
			if v.ID == want {
				selected = tts.WithRatingMark(v.Display(), tts.MiniMaxVoiceLevel(v.ID, v.Kind))
				break
			}
		}
		allowExtra := selected == "" && want != "" && !tts.IsEdgeVoice(want)
		if allowExtra {
			selected = want
			mmIDByLabel[want] = want
		}
		setVoiceOptions(labels, selected, allowExtra)
	}

	loadMiniMaxVoices := func(fromUser bool) {
		if fromUser {
			appendLog("正在刷新 MiniMax 音色…")
		}
		go func() {
			client := &voiceclone.Client{
				BaseURL: state.Cfg.MiniMax.BaseURL,
				APIKey:  state.Cfg.MiniMax.APIKey,
				Model:   state.Cfg.MiniMax.Model,
			}
			voices, err := client.ListVoices(context.Background(), voiceclone.VoiceTypeAll)
			fyne.Do(func() {
				if err != nil {
					appendLog("刷新 MiniMax 音色失败: " + err.Error())
					if len(voiceSelect.Options) == 0 && state.Cfg.TTS.Voice != "" && !tts.IsEdgeVoice(state.Cfg.TTS.Voice) {
						setVoiceOptions([]string{state.Cfg.TTS.Voice}, state.Cfg.TTS.Voice, true)
						mmIDByLabel[state.Cfg.TTS.Voice] = state.Cfg.TTS.Voice
					}
					return
				}
				fillMiniMaxVoices(voices)
				appendLog(fmt.Sprintf("已加载 %d 个 MiniMax 音色（复刻优先）", len(voices)))
			})
		}()
	}

	fillKokoroVoices := func(voices []tts.VoiceOption) {
		if len(voices) == 0 {
			voices = tts.BuiltinKokoroVoices()
		}
		kkIDByLabel = map[string]string{}
		labels := make([]string, 0, len(voices))
		for _, v := range voices {
			lab := v.Label
			if lab == "" {
				lab = v.ID
			}
			kkIDByLabel[lab] = v.ID
			kkIDByLabel[v.ID] = v.ID
			labels = append(labels, lab)
		}
		selected := ""
		want := state.Cfg.TTS.Voice
		for _, v := range voices {
			if v.ID == want || v.Label == want {
				selected = v.Label
				if selected == "" {
					selected = v.ID
				}
				break
			}
		}
		if selected == "" && tts.IsKokoroVoice(want) {
			selected = tts.KokoroLabelForID(want)
		}
		setVoiceOptions(labels, selected, false)
	}

	loadKokoroVoices := func(fromUser bool) {
		fillKokoroVoices(tts.BuiltinKokoroVoices())
		if fromUser {
			appendLog("正在刷新 Kokoro 音色…")
		}
		go func() {
			list, err := tts.ListKokoroVoices(context.Background(), state.Cfg.TTS.KokoroURL)
			fyne.Do(func() {
				if err != nil && !strings.Contains(err.Error(), "启动Kokoro.bat") {
					appendLog("刷新 Kokoro 音色失败: " + err.Error())
					return
				}
				if err != nil {
					appendLog(err.Error())
					return
				}
				fillKokoroVoices(list)
				if fromUser {
					appendLog(fmt.Sprintf("已加载 %d 个 Kokoro 音色", len(list)))
				}
			})
		}()
	}

	fillOpenVoiceVoices := func(voices []tts.VoiceOption) {
		if len(voices) == 0 {
			voices = tts.BuiltinOpenVoiceVoices()
		}
		ovIDByLabel = map[string]string{}
		labels := make([]string, 0, len(voices))
		for _, v := range voices {
			lab := v.Label
			if lab == "" {
				lab = v.ID
			}
			ovIDByLabel[lab] = tts.OpenVoiceIDForLabel(v.ID)
			ovIDByLabel[v.ID] = tts.OpenVoiceIDForLabel(v.ID)
			labels = append(labels, lab)
		}
		selected := ""
		want := state.Cfg.TTS.Voice
		for _, v := range voices {
			if v.ID == want || v.Label == want || tts.OpenVoiceIDForLabel(v.ID) == tts.OpenVoiceIDForLabel(want) {
				selected = v.Label
				if selected == "" {
					selected = v.ID
				}
				break
			}
		}
		if selected == "" && tts.IsOpenVoiceID(want) {
			selected = tts.OpenVoiceLabelForID(want)
		}
		setVoiceOptions(labels, selected, false)
	}

	loadOpenVoiceVoices := func(fromUser bool) {
		fillOpenVoiceVoices(tts.BuiltinOpenVoiceVoices())
		if fromUser {
			appendLog("正在检查 OpenVoice 服务…")
		}
		go func() {
			list, err := tts.ListOpenVoiceVoices(context.Background(), state.Cfg.TTS.OpenVoiceURL)
			fyne.Do(func() {
				if err != nil {
					appendLog(err.Error())
					return
				}
				fillOpenVoiceVoices(list)
				if fromUser {
					appendLog(fmt.Sprintf("已连接 OpenVoice，%d 个音色", len(list)))
				}
			})
		}()
	}

	fillIndexTTS2Voices := func(voices []tts.VoiceOption) {
		voices = tts.RankedIndexTTS2Voices(voices)
		ixIDByLabel = map[string]string{}
		labels := make([]string, 0, len(voices))
		for _, v := range voices {
			lab := v.Label
			if lab == "" {
				lab = v.ID
			}
			id := tts.IndexTTS2IDForLabel(v.ID)
			ixIDByLabel[lab] = id
			ixIDByLabel[tts.StripRatingMark(lab)] = id
			ixIDByLabel[v.ID] = id
			labels = append(labels, lab)
		}
		selected := ""
		want := state.Cfg.TTS.Voice
		for _, v := range voices {
			if v.ID == want || v.Label == want || tts.StripRatingMark(v.Label) == tts.StripRatingMark(want) || tts.IndexTTS2IDForLabel(v.ID) == tts.IndexTTS2IDForLabel(want) {
				selected = v.Label
				if selected == "" {
					selected = v.ID
				}
				break
			}
		}
		if selected == "" && tts.IsIndexTTS2ID(want) {
			plain := tts.IndexTTS2LabelForID(want)
			selected = tts.WithRatingMark(plain, tts.IndexTTS2Recommendation(want))
		}
		setVoiceOptions(labels, selected, false)
	}

	loadIndexTTS2Voices := func(fromUser bool) {
		fillIndexTTS2Voices(tts.BuiltinIndexTTS2Voices())
		if fromUser {
			appendLog("正在检查 IndexTTS-2 服务…")
		}
		go func() {
			list, err := tts.ListIndexTTS2Voices(context.Background(), state.Cfg.TTS.IndexTTS2URL)
			fyne.Do(func() {
				if err != nil {
					appendLog(err.Error())
					return
				}
				fillIndexTTS2Voices(list)
				if fromUser {
					appendLog(fmt.Sprintf("已连接 IndexTTS-2，%d 个音色", len(list)))
				}
			})
		}()
	}

	var engineHint *widget.Label
	var ovRefRow fyne.CanvasObject
	var ixRefRow fyne.CanvasObject
	applyEngineChrome := func(label string) {
		switch label {
		case engineLabelMiniMax:
			refreshMiniMaxBtn.Enable()
			if engineHint != nil {
				engineHint.SetText("")
			}
			if ovRefRow != nil {
				ovRefRow.Hide()
			}
			if ixRefRow != nil {
				ixRefRow.Hide()
			}
		case engineLabelKokoro:
			refreshMiniMaxBtn.Enable()
			if engineHint != nil {
				engineHint.SetText("请先双击运行 E:\\kokoro-tts\\启动Kokoro.bat（http://127.0.0.1:3211）")
			}
			if ovRefRow != nil {
				ovRefRow.Hide()
			}
			if ixRefRow != nil {
				ixRefRow.Hide()
			}
		case engineLabelOpenVoice:
			refreshMiniMaxBtn.Enable()
			if engineHint != nil {
				engineHint.SetText("请先双击运行 E:\\OpenVoice\\启动配音服务.bat（http://127.0.0.1:3212）。克隆音色需要参考音频。")
			}
			if ovRefRow != nil {
				ovRefRow.Show()
			}
			if ixRefRow != nil {
				ixRefRow.Hide()
			}
		case engineLabelIndexTTS2:
			refreshMiniMaxBtn.Enable()
			if engineHint != nil {
				engineHint.SetText("请先双击运行 E:\\indextts2\\启动IndexTTS2.bat（http://127.0.0.1:3213）。下拉里的「官方示例」可直接预听本地 wav，不必启动服务、也不必另选参考音频；「克隆音色」预听已选的参考音频。正式合成前仍需启动服务。")
			}
			if ovRefRow != nil {
				ovRefRow.Hide()
			}
			if ixRefRow != nil {
				ixRefRow.Show()
			}
		default:
			refreshMiniMaxBtn.Disable()
			if engineHint != nil {
				engineHint.SetText("")
			}
			if ovRefRow != nil {
				ovRefRow.Hide()
			}
			if ixRefRow != nil {
				ixRefRow.Hide()
			}
		}
	}

	engineSelect.OnChanged = func(v string) {
		switch v {
		case engineLabelMiniMax:
			state.Cfg.TTS.Engine = tts.EngineMiniMax
			applyEngineChrome(v)
			if len(mmVoices) == 0 {
				loadMiniMaxVoices(false)
			} else {
				fillMiniMaxVoices(mmVoices)
			}
		case engineLabelKokoro:
			state.Cfg.TTS.Engine = tts.EngineKokoro
			applyEngineChrome(v)
			loadKokoroVoices(false)
		case engineLabelOpenVoice:
			state.Cfg.TTS.Engine = tts.EngineOpenVoice
			applyEngineChrome(v)
			loadOpenVoiceVoices(false)
		case engineLabelIndexTTS2:
			state.Cfg.TTS.Engine = tts.EngineIndexTTS2
			applyEngineChrome(v)
			loadIndexTTS2Voices(false)
		default:
			state.Cfg.TTS.Engine = tts.EngineEdge
			applyEngineChrome(v)
			fillEdgeVoices()
		}
		_ = config.Save(state.CfgPath, state.Cfg)
	}

	refreshMiniMaxBtn = widget.NewButton("刷新音色", func() {
		switch engineSelect.Selected {
		case engineLabelMiniMax:
			loadMiniMaxVoices(true)
		case engineLabelKokoro:
			loadKokoroVoices(true)
		case engineLabelOpenVoice:
			loadOpenVoiceVoices(true)
		case engineLabelIndexTTS2:
			loadIndexTTS2Voices(true)
		}
	})

	engineHint = widget.NewLabel("")
	engineHint.Wrapping = fyne.TextWrapWord
	ovRefLabel := widget.NewLabel("未选择参考音频")
	if ovRefPath != "" {
		ovRefLabel.SetText(ovRefPath)
	}
	ovRefLabel.Wrapping = fyne.TextWrapWord
	ovRefRow = container.NewVBox(
		widget.NewLabel("OpenVoice 参考音频（选「克隆音色」时必填）"),
		container.NewBorder(nil, nil,
			container.NewHBox(
				widget.NewButton("选择参考音频", func() {
					files, err := pickAudioFiles()
					if err != nil {
						appendLog("选择参考音频失败: " + err.Error())
						return
					}
					if len(files) == 0 {
						return
					}
					ovRefPath = files[0]
					state.Cfg.TTS.OpenVoiceReference = ovRefPath
					ovRefLabel.SetText(ovRefPath)
					_ = config.Save(state.CfgPath, state.Cfg)
					appendLog("已设置 OpenVoice 参考音频: " + ovRefPath)
				}),
				widget.NewButton("清除", func() {
					ovRefPath = ""
					state.Cfg.TTS.OpenVoiceReference = ""
					ovRefLabel.SetText("未选择参考音频")
					_ = config.Save(state.CfgPath, state.Cfg)
				}),
			),
			nil, ovRefLabel),
	)
	ovRefRow.Hide()

	ixRefLabel := widget.NewLabel("未选择参考音频")
	if ixRefPath != "" {
		ixRefLabel.SetText(ixRefPath)
	}
	ixRefLabel.Wrapping = fyne.TextWrapWord
	ixRefRow = container.NewVBox(
		widget.NewLabel("IndexTTS-2 参考音频（仅「克隆音色」必填；选官方示例时不必选）"),
		container.NewBorder(nil, nil,
			container.NewHBox(
				widget.NewButton("选择参考音频", func() {
					files, err := pickAudioFiles()
					if err != nil {
						appendLog("选择参考音频失败: " + err.Error())
						return
					}
					if len(files) == 0 {
						return
					}
					ixRefPath = files[0]
					state.Cfg.TTS.IndexTTS2Reference = ixRefPath
					ixRefLabel.SetText(ixRefPath)
					_ = config.Save(state.CfgPath, state.Cfg)
					appendLog("已设置 IndexTTS-2 参考音频: " + ixRefPath)
				}),
				widget.NewButton("清除", func() {
					ixRefPath = ""
					state.Cfg.TTS.IndexTTS2Reference = ""
					ixRefLabel.SetText("未选择参考音频")
					_ = config.Save(state.CfgPath, state.Cfg)
				}),
			),
			nil, ixRefLabel),
	)
	ixRefRow.Hide()

	// --- sliders: TTS speed / volume / pitch ---
	speedVal := widget.NewLabel(fmt.Sprintf("%.2f×", valueOr(state.Cfg.TTS.Speed, 1)))
	speedSlider := widget.NewSlider(0.5, 2.0)
	speedSlider.Step = 0.05
	speedSlider.SetValue(valueOr(state.Cfg.TTS.Speed, 1))
	speedSlider.OnChanged = func(v float64) { speedVal.SetText(fmt.Sprintf("%.2f×", v)) }

	ttsVolVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.TTS.Volume, 1)*100))
	ttsVolSlider := widget.NewSlider(0.1, 2.0)
	ttsVolSlider.Step = 0.05
	ttsVolSlider.SetValue(valueOr(state.Cfg.TTS.Volume, 1))
	ttsVolSlider.OnChanged = func(v float64) { ttsVolVal.SetText(fmt.Sprintf("%.0f%%", v*100)) }

	pitchVal := widget.NewLabel(fmt.Sprintf("%+.0f", state.Cfg.TTS.Pitch))
	pitchSlider := widget.NewSlider(-12, 12)
	pitchSlider.Step = 1
	pitchSlider.SetValue(state.Cfg.TTS.Pitch)
	pitchSlider.OnChanged = func(v float64) { pitchVal.SetText(fmt.Sprintf("%+.0f", v)) }

	origVolVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.OriginalVolume, 1.2)*100))
	origVolSlider := widget.NewSlider(0, 2)
	origVolSlider.Step = 0.05
	origVolSlider.SetValue(valueOr(state.Cfg.UI.OriginalVolume, 1.2))
	origVolSlider.OnChanged = func(v float64) { origVolVal.SetText(fmt.Sprintf("%.0f%%", v*100)) }

	voiceVolVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.VoiceVolume, 1)*100))
	voiceVolSlider := widget.NewSlider(0, 2)
	voiceVolSlider.Step = 0.05
	voiceVolSlider.SetValue(valueOr(state.Cfg.UI.VoiceVolume, 1))
	voiceVolSlider.OnChanged = func(v float64) { voiceVolVal.SetText(fmt.Sprintf("%.0f%%", v*100)) }

	bgmVolVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.BGMVolume, 0.3)*100))
	bgmVolSlider := widget.NewSlider(0, 2)
	bgmVolSlider.Step = 0.05
	bgmVolSlider.SetValue(valueOr(state.Cfg.UI.BGMVolume, 0.3))
	bgmVolSlider.OnChanged = func(v float64) { bgmVolVal.SetText(fmt.Sprintf("%.0f%%", v*100)) }

	sfxVolVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.SFXVolume, 0.6)*100))
	sfxVolSlider := widget.NewSlider(0, 2)
	sfxVolSlider.Step = 0.05
	sfxVolSlider.SetValue(valueOr(state.Cfg.UI.SFXVolume, 0.6))
	sfxVolSlider.OnChanged = func(v float64) { sfxVolVal.SetText(fmt.Sprintf("%.0f%%", v*100)) }

	// --- BGM ---
	bgmSourceLabels := []string{"关闭", "曲库", "本地文件", "Sonilo AI 配乐"}
	bgmSourceSelect := widget.NewSelect(bgmSourceLabels, nil)
	switch state.Cfg.UI.BGMSource {
	case "library":
		bgmSourceSelect.SetSelected("曲库")
	case "upload":
		bgmSourceSelect.SetSelected("本地文件")
	case "sonilo":
		bgmSourceSelect.SetSelected("Sonilo AI 配乐")
	default:
		bgmSourceSelect.SetSelected("关闭")
	}

	bgmLibrarySelect := widget.NewSelect(nil, nil)
	bgmLibrarySelect.PlaceHolder = "选择曲库音乐"
	bgmFileLabel := widget.NewLabel("未选择本地音频")
	bgmFileLabel.Truncation = fyne.TextTruncateEllipsis
	if state.Cfg.UI.BGMSource == "upload" && state.Cfg.UI.BGMPath != "" {
		bgmFileLabel.SetText(filepath.Base(state.Cfg.UI.BGMPath))
	}
	var selectedUpload string
	if state.Cfg.UI.BGMSource == "upload" {
		selectedUpload = state.Cfg.UI.BGMPath
	}

	refreshLibrary := func() {
		exeDir := ""
		if exe, err := os.Executable(); err == nil {
			exeDir = filepath.Dir(exe)
		}
		dirs := media.DefaultBGMLibraryDirs(state.Cfg.UI.BGMLibraryDir, exeDir, state.ProjectsRoot)
		entries := media.ListBGMLibrary(dirs...)
		labels := make([]string, 0, len(entries))
		pathBy := map[string]string{}
		for _, e := range entries {
			lab := e.Title
			if pathBy[lab] != "" && pathBy[lab] != e.Path {
				lab = e.Title + "  (" + filepath.Base(filepath.Dir(e.Path)) + ")"
			}
			pathBy[lab] = e.Path
			labels = append(labels, lab)
		}
		bgmLibrarySelect.Options = labels
		wanted := ""
		if state.Cfg.UI.BGMSource == "library" {
			for lab, pth := range pathBy {
				if pth == state.Cfg.UI.BGMPath {
					wanted = lab
					break
				}
			}
		}
		if wanted != "" {
			bgmLibrarySelect.SetSelected(wanted)
		} else if len(labels) > 0 && bgmLibrarySelect.Selected == "" {
			// keep empty placeholder when nothing saved
		}
		bgmLibrarySelect.Refresh()
		bgmLibrarySelect.OnChanged = func(lab string) {
			if pth := pathBy[lab]; pth != "" {
				state.Cfg.UI.BGMPath = pth
			}
		}
		if len(labels) == 0 {
			bgmLibrarySelect.PlaceHolder = "曲库为空，请放入音频或选择目录"
		}
	}
	refreshLibrary()

	pickLibDirBtn := widget.NewButton("曲库目录", func() {
		dir, err := pickDirectory("选择 BGM 曲库目录")
		if err != nil || dir == "" {
			return
		}
		state.Cfg.UI.BGMLibraryDir = dir
		_ = config.Save(state.CfgPath, state.Cfg)
		appendLog("已设置曲库目录: " + dir)
		refreshLibrary()
	})
	pickBgmBtn := widget.NewButton("选择音频", func() {
		files, err := pickAudioFiles()
		if err != nil || len(files) == 0 {
			return
		}
		selectedUpload = files[0]
		state.Cfg.UI.BGMPath = files[0]
		bgmFileLabel.SetText(filepath.Base(files[0]))
	})
	previewBgmBtn = widget.NewButton("预听 BGM", func() {
		if media.AudioPlaying() && previewBgmBtn.Text == "停止" {
			media.StopAudio()
			previewBgmBtn.SetText("预听 BGM")
			if previewBtn != nil {
				previewBtn.SetText("预听")
			}
			appendLog("已停止预听")
			return
		}
		path := currentBGMPath(bgmSourceSelect.Selected, bgmLibrarySelect, selectedUpload, state)
		if path == "" {
			appendLog("没有可预听的背景音乐（Sonilo 会在合成时生成）")
			return
		}
		if err := playPreviewAudio(state, path); err != nil {
			appendLog("预听 BGM 失败: " + err.Error())
			return
		}
		previewBgmBtn.SetText("停止")
		appendLog("预听已开始播放")
		go watchPreviewEnd(func() {
			if previewBgmBtn != nil && previewBgmBtn.Text == "停止" && !media.AudioPlaying() {
				previewBgmBtn.SetText("预听 BGM")
			}
		})
	})

	soniloPrompt := widget.NewEntry()
	soniloPrompt.SetPlaceHolder("可选：配乐风格提示，例如 紧张、空灵、古风")
	soniloPrompt.SetText(state.Cfg.Sonilo.BGMPrompt)
	sfxCheck := widget.NewCheck("开启 AI 音效（Sonilo，默认关闭）", nil)
	sfxCheck.SetChecked(state.Cfg.UI.SFXEnabled)
	sfxPrompt := widget.NewEntry()
	sfxPrompt.SetPlaceHolder("可选：音效风格提示")
	sfxPrompt.SetText(state.Cfg.UI.SFXPrompt)

	bgmBody := container.NewVBox()
	showBGM := func(src string) {
		var extra []fyne.CanvasObject
		switch src {
		case "曲库":
			extra = []fyne.CanvasObject{
				widget.NewForm(widget.NewFormItem("曲库", bgmLibrarySelect)),
				container.NewHBox(pickLibDirBtn, previewBgmBtn),
			}
		case "本地文件":
			extra = []fyne.CanvasObject{
				container.NewBorder(nil, nil, pickBgmBtn, previewBgmBtn, bgmFileLabel),
			}
		case "Sonilo AI 配乐":
			extra = []fyne.CanvasObject{
				widget.NewForm(widget.NewFormItem("配乐提示", soniloPrompt)),
				widget.NewLabel("将在合成时根据成片画面生成，需要在「设置」填写 Sonilo API Key。"),
			}
		}
		bgmBody.Objects = extra
		bgmBody.Refresh()
	}
	bgmSourceSelect.OnChanged = func(v string) { showBGM(v) }
	showBGM(bgmSourceSelect.Selected)

	// --- subtitles ---
	burnCheck := widget.NewCheck("烧录解说字幕", nil)
	burnCheck.SetChecked(state.Cfg.UI.SubtitleBurnEnabled)
	hideCheck := widget.NewCheck("原片段隐藏解说字幕（OST=1）", nil)
	hideCheck.SetChecked(state.Cfg.UI.SubtitleHideOnOriginal)
	fontSelect := widget.NewSelect([]string{"Microsoft YaHei", "Microsoft YaHei UI", "SimHei", "SimSun", "KaiTi", "FangSong", "Arial"}, nil)
	if state.Cfg.UI.SubtitleFont != "" {
		found := false
		for _, f := range fontSelect.Options {
			if f == state.Cfg.UI.SubtitleFont {
				found = true
				break
			}
		}
		if !found {
			fontSelect.Options = append([]string{state.Cfg.UI.SubtitleFont}, fontSelect.Options...)
		}
		fontSelect.SetSelected(state.Cfg.UI.SubtitleFont)
	} else {
		fontSelect.SetSelected("Microsoft YaHei")
	}
	fontSizeVal := widget.NewLabel(fmt.Sprintf("%d", intOr(state.Cfg.UI.SubtitleFontSize, 15)))
	fontSizeSlider := widget.NewSlider(10, 72)
	fontSizeSlider.Step = 1
	fontSizeSlider.SetValue(float64(intOr(state.Cfg.UI.SubtitleFontSize, 15)))
	fontSizeSlider.OnChanged = func(v float64) { fontSizeVal.SetText(fmt.Sprintf("%d", int(v))) }
	colorEntry := widget.NewEntry()
	colorEntry.SetText(stringOr(state.Cfg.UI.SubtitleColor, "#FFFFFF"))
	posSelect := widget.NewSelect([]string{"底部", "顶部", "居中"}, nil)
	switch state.Cfg.UI.SubtitlePosition {
	case "top":
		posSelect.SetSelected("顶部")
	case "center", "middle":
		posSelect.SetSelected("居中")
	default:
		posSelect.SetSelected("底部")
	}
	onMaskCheck := widget.NewCheck("解说字幕显示在遮罩上", nil)
	onMaskCheck.SetChecked(state.Cfg.UI.SubtitleOnMask)

	// --- picture ---
	aspectSelect := widget.NewSelect([]string{"源视频", "竖屏 9:16", "横屏 16:9"}, nil)
	switch media.NormalizeAspect(state.Cfg.UI.AspectRatio) {
	case media.AspectVertical:
		aspectSelect.SetSelected("竖屏 9:16")
	case media.AspectHorizontal:
		aspectSelect.SetSelected("横屏 16:9")
	default:
		if state.Cfg.UI.AspectRatio == "" {
			aspectSelect.SetSelected("竖屏 9:16")
		} else {
			aspectSelect.SetSelected("源视频")
		}
	}
	resSelect := widget.NewSelect([]string{"480p", "720p", "1080p", "1440p", "2160p（4K）"}, nil)
	switch media.ParseResolutionHeight(state.Cfg.UI.Resolution) {
	case 480:
		resSelect.SetSelected("480p")
	case 720:
		resSelect.SetSelected("720p")
	case 1440:
		resSelect.SetSelected("1440p")
	case 2160:
		resSelect.SetSelected("2160p（4K）")
	default:
		resSelect.SetSelected("1080p")
	}

	// --- mask ---
	maskCheck := widget.NewCheck("启用原字幕遮罩（遮住片源烧录字幕）", nil)
	maskCheck.SetChecked(state.Cfg.UI.SubtitleMaskEnabled)
	maskYVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.SubtitleMaskY, 79)))
	maskYSlider := widget.NewSlider(0, 99)
	maskYSlider.Step = 1
	maskYSlider.SetValue(valueOr(state.Cfg.UI.SubtitleMaskY, 79))
	maskYSlider.OnChanged = func(v float64) { maskYVal.SetText(fmt.Sprintf("%.0f%%", v)) }
	maskHVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.SubtitleMaskHeight, 16)))
	maskHSlider := widget.NewSlider(2, 60)
	maskHSlider.Step = 1
	maskHSlider.SetValue(valueOr(state.Cfg.UI.SubtitleMaskHeight, 16))
	maskHSlider.OnChanged = func(v float64) { maskHVal.SetText(fmt.Sprintf("%.0f%%", v)) }
	maskXVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.SubtitleMaskX, 8)))
	maskXSlider := widget.NewSlider(0, 40)
	maskXSlider.Step = 1
	maskXSlider.SetValue(valueOr(state.Cfg.UI.SubtitleMaskX, 8))
	maskXSlider.OnChanged = func(v float64) { maskXVal.SetText(fmt.Sprintf("%.0f%%", v)) }
	maskWVal := widget.NewLabel(fmt.Sprintf("%.0f%%", valueOr(state.Cfg.UI.SubtitleMaskWidth, 84)))
	maskWSlider := widget.NewSlider(20, 100)
	maskWSlider.Step = 1
	maskWSlider.SetValue(valueOr(state.Cfg.UI.SubtitleMaskWidth, 84))
	maskWSlider.OnChanged = func(v float64) { maskWVal.SetText(fmt.Sprintf("%.0f%%", v)) }
	maskBlurVal := widget.NewLabel(fmt.Sprintf("%d", intOr(state.Cfg.UI.SubtitleMaskBlur, 26)))
	maskBlurSlider := widget.NewSlider(0, 80)
	maskBlurSlider.Step = 1
	maskBlurSlider.SetValue(float64(intOr(state.Cfg.UI.SubtitleMaskBlur, 26)))
	maskBlurSlider.OnChanged = func(v float64) { maskBlurVal.SetText(fmt.Sprintf("%d", int(v))) }
	maskOpVal := widget.NewLabel(fmt.Sprintf("%d%%", intOr(state.Cfg.UI.SubtitleMaskOpacity, 84)))
	maskOpSlider := widget.NewSlider(0, 100)
	maskOpSlider.Step = 1
	maskOpSlider.SetValue(float64(intOr(state.Cfg.UI.SubtitleMaskOpacity, 84)))
	maskOpSlider.OnChanged = func(v float64) { maskOpVal.SetText(fmt.Sprintf("%d%%", int(v))) }

	previewMaskBtn := widget.NewButton("预览遮罩", func() {
		if state.Project == nil || len(state.Project.VideoPaths) == 0 {
			appendLog("请先打开工程并添加视频，才能预览遮罩")
			return
		}
		bins, err := resolveFFmpeg(state)
		if err != nil {
			appendLog("预览遮罩失败: " + err.Error())
			return
		}
		dir := filepath.Join(os.TempDir(), "xuanyu-mask-preview")
		_ = os.MkdirAll(dir, 0o755)
		out := filepath.Join(dir, fmt.Sprintf("mask_%d.jpg", time.Now().UnixNano()))
		mask := media.SubtitleMask{
			Enabled: true,
			X:       maskXSlider.Value,
			Y:       maskYSlider.Value,
			Width:   maskWSlider.Value,
			Height:  maskHSlider.Value,
			Blur:    int(maskBlurSlider.Value),
			Opacity: int(maskOpSlider.Value),
		}
		go func() {
			err := media.PreviewSubtitleMask(context.Background(), bins, state.Project.VideoPaths[0], out, mask, 1)
			fyne.Do(func() {
				if err != nil {
					appendLog("预览遮罩失败: " + err.Error())
					return
				}
				appendLog("遮罩预览: " + out)
				_ = media.OpenFile(out)
			})
		}()
	})

	collectRenderForm = func(cfg *config.Config) {
		switch engineSelect.Selected {
		case engineLabelMiniMax:
			cfg.TTS.Engine = tts.EngineMiniMax
			if id, ok := mmIDByLabel[voiceSelect.Selected]; ok {
				cfg.TTS.Voice = id
			} else if voiceSelect.Selected != "" {
				cfg.TTS.Voice = voiceSelect.Selected
			}
		case engineLabelKokoro:
			cfg.TTS.Engine = tts.EngineKokoro
			if id, ok := kkIDByLabel[voiceSelect.Selected]; ok {
				cfg.TTS.Voice = id
			} else if id := tts.KokoroIDForLabel(voiceSelect.Selected); id != "" {
				cfg.TTS.Voice = id
			} else if voiceSelect.Selected != "" {
				cfg.TTS.Voice = voiceSelect.Selected
			}
		case engineLabelOpenVoice:
			cfg.TTS.Engine = tts.EngineOpenVoice
			sel := voiceSelect.Selected
			if id, ok := ovIDByLabel[sel]; ok {
				sel = id
			}
			cfg.TTS.Voice = tts.OpenVoiceIDForLabel(sel)
		case engineLabelIndexTTS2:
			cfg.TTS.Engine = tts.EngineIndexTTS2
			sel := voiceSelect.Selected
			if id, ok := ixIDByLabel[sel]; ok {
				sel = id
			}
			cfg.TTS.Voice = tts.IndexTTS2IDForLabel(sel)
		default:
			cfg.TTS.Engine = tts.EngineEdge
			if id := tts.VoiceIDForLabel(voiceSelect.Selected); id != "" {
				cfg.TTS.Voice = id
			}
		}
		cfg.TTS.OpenVoiceReference = ovRefPath
		cfg.TTS.IndexTTS2Reference = ixRefPath
		cfg.TTS.Speed = speedSlider.Value
		cfg.TTS.Volume = ttsVolSlider.Value
		cfg.TTS.Pitch = pitchSlider.Value
		cfg.UI.OriginalVolume = origVolSlider.Value
		cfg.UI.VoiceVolume = voiceVolSlider.Value
		cfg.UI.BGMVolume = bgmVolSlider.Value
		cfg.UI.SFXVolume = sfxVolSlider.Value
		switch bgmSourceSelect.Selected {
		case "曲库":
			cfg.UI.BGMSource = "library"
		case "本地文件":
			cfg.UI.BGMSource = "upload"
			if selectedUpload != "" {
				cfg.UI.BGMPath = selectedUpload
			}
		case "Sonilo AI 配乐":
			cfg.UI.BGMSource = "sonilo"
			cfg.UI.BGMPath = ""
		default:
			cfg.UI.BGMSource = "off"
			cfg.UI.BGMPath = ""
		}
		cfg.Sonilo.BGMPrompt = soniloPrompt.Text
		cfg.UI.SFXEnabled = sfxCheck.Checked
		cfg.UI.SFXPrompt = sfxPrompt.Text
		cfg.UI.SubtitleBurnEnabled = burnCheck.Checked
		cfg.UI.SubtitleHideOnOriginal = hideCheck.Checked
		cfg.UI.SubtitleFont = fontSelect.Selected
		cfg.UI.SubtitleFontSize = int(fontSizeSlider.Value)
		cfg.UI.SubtitleColor = colorEntry.Text
		switch posSelect.Selected {
		case "顶部":
			cfg.UI.SubtitlePosition = "top"
		case "居中":
			cfg.UI.SubtitlePosition = "center"
		default:
			cfg.UI.SubtitlePosition = "bottom"
		}
		cfg.UI.SubtitleOnMask = onMaskCheck.Checked
		switch aspectSelect.Selected {
		case "竖屏 9:16":
			cfg.UI.AspectRatio = media.AspectVertical
		case "横屏 16:9":
			cfg.UI.AspectRatio = media.AspectHorizontal
		default:
			cfg.UI.AspectRatio = media.AspectSource
		}
		switch resSelect.Selected {
		case "480p":
			cfg.UI.Resolution = "480p"
		case "720p":
			cfg.UI.Resolution = "720p"
		case "1440p":
			cfg.UI.Resolution = "1440p"
		case "2160p（4K）":
			cfg.UI.Resolution = "2160p"
		default:
			cfg.UI.Resolution = "1080p"
		}
		cfg.UI.SubtitleMaskEnabled = maskCheck.Checked
		cfg.UI.SubtitleMaskX = maskXSlider.Value
		cfg.UI.SubtitleMaskY = maskYSlider.Value
		cfg.UI.SubtitleMaskWidth = maskWSlider.Value
		cfg.UI.SubtitleMaskHeight = maskHSlider.Value
		cfg.UI.SubtitleMaskBlur = int(maskBlurSlider.Value)
		cfg.UI.SubtitleMaskOpacity = int(maskOpSlider.Value)
	}

	saveForm := func() {
		if collectRenderForm != nil {
			collectRenderForm(state.Cfg)
		}
		_ = config.Save(state.CfgPath, state.Cfg)
	}

	setBusy := func(v bool) {
		busy = v
		if v {
			media.StopAudio()
			if previewBtn != nil {
				previewBtn.SetText("预听")
			}
			if previewBgmBtn != nil {
				previewBgmBtn.SetText("预听 BGM")
			}
			startBtn.Disable()
			cancelBtn.Enable()
			openBtn.Disable()
			previewBtn.Disable()
			return
		}
		startBtn.Enable()
		cancelBtn.Disable()
		previewBtn.Enable()
		if lastOutput != "" {
			openBtn.Enable()
		}
	}

	previewBtn = widget.NewButton("预听", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		if previewBusy {
			mu.Unlock()
			return
		}
		if media.AudioPlaying() {
			media.StopAudio()
			previewBtn.SetText("预听")
			if previewBgmBtn != nil {
				previewBgmBtn.SetText("预听 BGM")
			}
			mu.Unlock()
			appendLog("已停止预听")
			return
		}
		previewBusy = true
		previewBtn.Disable()
		previewBtn.SetText("预听")
		mu.Unlock()
		saveForm()
		var items []script.Item
		if state.Project != nil {
			items = state.Project.ScriptItems
		}
		sample := tts.PreviewText(items, 80)
		officialURL := ""
		switch engineSelect.Selected {
		case engineLabelMiniMax:
			want := state.Cfg.TTS.Voice
			for _, v := range mmVoices {
				if v.ID == want && v.PreviewURL() != "" {
					officialURL = v.PreviewURL()
					break
				}
			}
			if officialURL != "" {
				appendLog("正在预听官方样音（不调用合成）")
			} else {
				appendLog("该音色没有官方预听音频，改为合成试听: " + sample)
			}
		case engineLabelKokoro:
			appendLog("正在预听 Kokoro: " + sample)
		case engineLabelOpenVoice:
			appendLog("正在预听 OpenVoice: " + sample)
		case engineLabelIndexTTS2:
			path, ok, err := tts.DirectPreviewPath(state.Cfg.TTS.Engine, state.Cfg.TTS.Voice, state.Cfg.TTS.IndexTTS2Reference)
			if err != nil {
				mu.Lock()
				previewBusy = false
				previewBtn.Enable()
				mu.Unlock()
				appendLog("预听失败: " + err.Error())
				return
			}
			if ok {
				if tts.IndexTTS2IDForLabel(state.Cfg.TTS.Voice) == tts.IndexTTS2CloneID {
					appendLog("正在预听参考音频（不调用合成）")
				} else {
					appendLog("正在预听官方示例音频（不调用合成）")
				}
				if err := playPreviewAudio(state, path); err != nil {
					mu.Lock()
					previewBusy = false
					previewBtn.Enable()
					mu.Unlock()
					appendLog("预听失败: " + err.Error())
					return
				}
				mu.Lock()
				previewBusy = false
				previewBtn.SetText("停止")
				previewBtn.Enable()
				mu.Unlock()
				appendLog("预听已开始播放")
				go watchPreviewEnd(func() {
					if previewBtn != nil && previewBtn.Text == "停止" && !media.AudioPlaying() {
						previewBtn.SetText("预听")
					}
				})
				return
			}
			appendLog("正在预听 IndexTTS-2: " + sample)
		default:
			appendLog("正在预听 Edge TTS: " + sample)
		}
		go func() {
			err := runVoicePreview(state, sample, officialURL)
			fyne.Do(func() {
				mu.Lock()
				previewBusy = false
				if !busy {
					previewBtn.Enable()
					if err == nil {
						previewBtn.SetText("停止")
					} else {
						previewBtn.SetText("预听")
					}
				}
				mu.Unlock()
				if err != nil {
					appendLog("预听失败: " + err.Error())
					return
				}
				appendLog("预听已开始播放")
				go watchPreviewEnd(func() {
					if previewBtn != nil && previewBtn.Text == "停止" && !media.AudioPlaying() {
						previewBtn.SetText("预听")
					}
				})
			})
		}()
	})

	openBtn = widget.NewButton("打开成片", func() {
		mu.Lock()
		out := lastOutput
		mu.Unlock()
		if out == "" {
			appendLog("还没有合成完成的成片")
			return
		}
		if err := media.OpenFile(out); err != nil {
			appendLog("打开成片失败: " + err.Error())
		}
	})
	openBtn.Disable()

	cancelBtn = widget.NewButton("取消任务", func() {
		mu.Lock()
		fn := cancelFn
		mu.Unlock()
		if fn != nil {
			fn()
			appendLog("已请求取消…")
		}
	})
	cancelBtn.Disable()

	startBtn = widget.NewButton("开始合成", func() {
		if state.Project == nil {
			appendLog("请先打开工程")
			return
		}
		if len(state.Project.VideoPaths) == 0 {
			appendLog("需要至少一个视频")
			return
		}
		if len(state.Project.ScriptItems) == 0 {
			appendLog("脚本为空，请先生成脚本")
			return
		}
		saveForm()
		voice := state.Cfg.TTS.Voice
		if voice == "" {
			appendLog("请选择配音")
			return
		}
		switch tts.NormalizeEngine(state.Cfg.TTS.Engine) {
		case tts.EngineMiniMax:
			if tts.IsEdgeVoice(voice) {
				appendLog("当前是 MiniMax 引擎，请选择 MiniMax 音色，不要使用 Edge 音色 ID")
				return
			}
		case tts.EngineOpenVoice:
			if tts.OpenVoiceIDForLabel(voice) == tts.OpenVoiceCloneID && strings.TrimSpace(state.Cfg.TTS.OpenVoiceReference) == "" {
				appendLog("克隆音色需要一段参考音频，请先在合成页选择 wav/mp3。")
				return
			}
		case tts.EngineIndexTTS2:
			if tts.IndexTTS2IDForLabel(voice) == tts.IndexTTS2CloneID && strings.TrimSpace(state.Cfg.TTS.IndexTTS2Reference) == "" {
				appendLog("克隆音色需要一段参考音频，请先在合成页选择 wav/mp3。")
				return
			}
		}
		synth, err := pipeline.NewSynthesizerFromConfig(state.Cfg)
		if err != nil {
			appendLog(err.Error())
			return
		}

		taskID := time.Now().Format("20060102-150405")
		workDir := filepath.Join(state.ProjectsRoot, state.Project.Slug, "tasks", taskID)
		outPath := filepath.Join(workDir, "final.mp4")
		state.Log.Clear()
		appendLog("任务 " + taskID)
		progress.SetValue(0)

		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		cancelFn = cancel
		setBusy(true)
		mu.Unlock()

		go func() {
			logPath := filepath.Join(workDir, "render.log")
			appendDiskLog := func(line string) {
				_ = os.MkdirAll(workDir, 0o755)
				f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					return
				}
				defer f.Close()
				_, _ = f.WriteString(time.Now().Format("15:04:05") + " " + line + "\n")
			}
			appendDiskLog("start engine=" + state.Cfg.TTS.Engine + " voice=" + voice)

			params := pipeline.Params{
				Items:      append([]script.Item(nil), state.Project.ScriptItems...),
				VideoPaths: append([]string(nil), state.Project.VideoPaths...),
				WorkDir:    workDir,
				OutputPath: outPath,
				TTS:        synth,
				Voice:      voice,
				OnProgress: func(step string, percent int, msg string) {
					line := fmt.Sprintf("[%s %d%%] %s", step, percent, msg)
					appendDiskLog(line)
					fyne.Do(func() {
						progress.SetValue(float64(percent))
						state.Log.Append(line)
						refreshLog()
					})
				},
			}
			pipeline.ApplyConfig(&params, state.Cfg)

			out, err := pipeline.Run(ctx, params)
			fyne.Do(func() {
				mu.Lock()
				cancelFn = nil
				if err == nil {
					lastOutput = out
				}
				setBusy(false)
				mu.Unlock()
				if err != nil {
					msg := "失败: " + err.Error()
					if ctx.Err() != nil {
						msg = "已取消"
					}
					state.Log.Append(msg)
					state.Log.Append("详细日志: " + logPath)
					appendDiskLog(msg)
				} else {
					state.Log.Append("完成: " + out)
					appendDiskLog("done " + out)
					_ = project.Save(state.ProjectsRoot, state.Project)
				}
				refreshLog()
			})
		}()
	})
	startBtn.Importance = widget.HighImportance

	applyEngineChrome(engineSelect.Selected)
	switch tts.NormalizeEngine(state.Cfg.TTS.Engine) {
	case tts.EngineMiniMax:
		loadMiniMaxVoices(false)
	case tts.EngineKokoro:
		loadKokoroVoices(false)
	case tts.EngineOpenVoice:
		loadOpenVoiceVoices(false)
	case tts.EngineIndexTTS2:
		loadIndexTTS2Voices(false)
	default:
		fillEdgeVoices()
	}

	voiceRow := container.NewBorder(nil, nil, nil, container.NewHBox(refreshMiniMaxBtn, previewBtn), voiceSelect)

	top := widget.NewCard("成片合成", "按脚本裁剪、配音并输出 final.mp4", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("引擎", engineSelect),
			widget.NewFormItem("语音", voiceRow),
			widget.NewFormItem("语速", shortSlide(speedSlider, speedVal)),
			widget.NewFormItem("合成音量", shortSlide(ttsVolSlider, ttsVolVal)),
			widget.NewFormItem("音调", shortSlide(pitchSlider, pitchVal)),
			widget.NewFormItem("原声音量", shortSlide(origVolSlider, origVolVal)),
			widget.NewFormItem("配音音量", shortSlide(voiceVolSlider, voiceVolVal)),
		),
		engineHint,
		ovRefRow,
		ixRefRow,
		container.NewHBox(startBtn, cancelBtn, openBtn),
		progress,
	))

	audioCard := widget.NewCard("背景音乐与 AI 音效", "曲库 / 本地文件 / 关闭；Sonilo 为可选 AI 配乐", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("BGM 来源", bgmSourceSelect),
			widget.NewFormItem("BGM 音量", shortSlide(bgmVolSlider, bgmVolVal)),
			widget.NewFormItem("音效音量", shortSlide(sfxVolSlider, sfxVolVal)),
		),
		bgmBody,
		sfxCheck,
		widget.NewForm(widget.NewFormItem("音效提示", sfxPrompt)),
	))

	subCard := widget.NewCard("字幕与画面", "", container.NewVBox(
		burnCheck, hideCheck, onMaskCheck,
		widget.NewForm(
			widget.NewFormItem("字体", fontSelect),
			widget.NewFormItem("字号", shortSlide(fontSizeSlider, fontSizeVal)),
			widget.NewFormItem("颜色", colorEntry),
			widget.NewFormItem("位置", posSelect),
			widget.NewFormItem("画面比例", aspectSelect),
			widget.NewFormItem("清晰度", resSelect),
		),
	))

	maskCard := widget.NewCard("原字幕遮罩", "用模糊条盖住片源自带字幕，可先预览再合成", container.NewVBox(
		maskCheck,
		widget.NewForm(
			widget.NewFormItem("垂直位置", shortSlide(maskYSlider, maskYVal)),
			widget.NewFormItem("高度", shortSlide(maskHSlider, maskHVal)),
			widget.NewFormItem("水平位置", shortSlide(maskXSlider, maskXVal)),
			widget.NewFormItem("宽度", shortSlide(maskWSlider, maskWVal)),
			widget.NewFormItem("模糊", shortSlide(maskBlurSlider, maskBlurVal)),
			widget.NewFormItem("透明度", shortSlide(maskOpSlider, maskOpVal)),
		),
		previewMaskBtn,
	))

	// Scroll the whole form. A Border "top" slot with this many cards
	// overflows the window and sits under the status bar
	// ("在素材、脚本、合成里继续"), clipping 原字幕遮罩 and later controls.
	return container.NewPadded(container.NewVScroll(container.NewVBox(
		top, audioCard, subCard, maskCard,
		widget.NewCard("运行日志", "", logEntry),
	)))
}

func valueOr(v, fallback float64) float64 {
	if v == 0 {
		return fallback
	}
	return v
}

func intOr(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

func stringOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func currentBGMPath(source string, lib *widget.Select, upload string, state *AppState) string {
	switch source {
	case "本地文件":
		return upload
	case "曲库":
		return state.Cfg.UI.BGMPath
	default:
		return ""
	}
}

func playPreviewAudio(state *AppState, path string) error {
	ffmpeg := ""
	if state != nil && state.Cfg != nil {
		ffmpeg = state.Cfg.FFmpeg.Path
	}
	return media.PlayAudioConfigured(path, ffmpeg)
}

func watchPreviewEnd(onStop func()) {
	for media.AudioPlaying() {
		time.Sleep(200 * time.Millisecond)
	}
	if onStop == nil {
		return
	}
	fyne.Do(onStop)
}

func runVoicePreview(state *AppState, sample, officialURL string) error {
	if path, ok, err := tts.DirectPreviewPath(state.Cfg.TTS.Engine, state.Cfg.TTS.Voice, state.Cfg.TTS.IndexTTS2Reference); err != nil {
		return err
	} else if ok {
		return playPreviewAudio(state, path)
	}
	dir := filepath.Join(os.TempDir(), "xuanyu-voice-preview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	previewFor := tts.PreviewTimeout
	if tts.NormalizeEngine(state.Cfg.TTS.Engine) == tts.EngineIndexTTS2 {
		previewFor = tts.IndexTTS2PreviewTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), previewFor)
	defer cancel()

	if tts.NormalizeEngine(state.Cfg.TTS.Engine) == tts.EngineMiniMax && officialURL != "" {
		out := filepath.Join(dir, officialPreviewFileName(officialURL, state.Cfg.TTS.Voice))
		if st, err := os.Stat(out); err != nil || st.Size() < 2048 {
			if err := voiceclone.DownloadDemo(ctx, officialPreviewHTTPClient(), officialURL, out); err != nil {
				return fmt.Errorf("下载官方预听音频失败: %w", err)
			}
		}
		st, err := os.Stat(out)
		if err != nil {
			return err
		}
		if st.Size() < 2048 {
			return fmt.Errorf("官方预听音频过小（%d bytes）", st.Size())
		}
		return playPreviewAudio(state, out)
	}

	synth, err := pipeline.NewSynthesizerFromConfig(state.Cfg)
	if err != nil {
		return err
	}
	ext := ".mp3"
	switch tts.NormalizeEngine(state.Cfg.TTS.Engine) {
	case tts.EngineKokoro, tts.EngineOpenVoice, tts.EngineIndexTTS2:
		ext = ".wav"
	}
	out := filepath.Join(dir, fmt.Sprintf("preview_%d%s", time.Now().UnixNano(), ext))
	if _, err := synth.Synthesize(ctx, sample, state.Cfg.TTS.Voice, out); err != nil {
		return err
	}
	st, err := os.Stat(out)
	if err != nil {
		return err
	}
	if st.Size() < 2048 {
		return fmt.Errorf("预听音频过小（%d bytes）", st.Size())
	}
	return playPreviewAudio(state, out)
}

func officialPreviewHTTPClient() *http.Client {
	return &http.Client{Timeout: tts.PreviewTimeout}
}

func officialPreviewFileName(url, voiceID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(voiceID) + "\n" + strings.TrimSpace(url)))
	ext := ".mp3"
	u := strings.ToLower(url)
	switch {
	case strings.Contains(u, ".wav"):
		ext = ".wav"
	case strings.Contains(u, ".m4a"):
		ext = ".m4a"
	case strings.Contains(u, ".ogg"):
		ext = ".ogg"
	}
	return "official_" + hex.EncodeToString(sum[:8]) + ext
}
