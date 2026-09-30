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
	"github.com/narratoai/narrato-go/internal/original"
)

type opControl struct {
	def    original.OpDef
	check  *widget.Check
	slider *widget.Slider
	hint   *widget.Label
	row    fyne.CanvasObject
}

func newOriginalPage(state *AppState) fyne.CanvasObject {
	videoPath := ""
	srtPath := ""
	bgmPath := ""
	lastOut := ""
	watermarkCorner := "tl"

	var (
		runMu     sync.Mutex
		cancelRun context.CancelFunc
		busy      bool
	)

	videoLabel := widget.NewLabel("未选择视频")
	videoLabel.Truncation = fyne.TextTruncateEllipsis
	srtLabel := widget.NewLabel("未选字幕")
	srtLabel.Truncation = fyne.TextTruncateEllipsis
	srtLabel.Importance = widget.LowImportance
	bgmLabel := widget.NewLabel("未选音频")
	bgmLabel.Truncation = fyne.TextTruncateEllipsis
	bgmLabel.Importance = widget.LowImportance

	status := widget.NewLabel("")
	status.Truncation = fyne.TextTruncateEllipsis
	status.Importance = widget.LowImportance
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	subtitle := widget.NewLabel("导出差异化（标准编码）· 勾选+滑条 · 可预览")
	subtitle.Importance = widget.LowImportance

	style := original.DefaultSubtitleStyle()
	var fontSelect *widget.Select
	var primaryEntry, outlineEntry, wmText *widget.Entry

	controls := map[string]*opControl{}
	collectSettings := func() []original.OpSettings {
		defs := original.AllOpDefs()
		out := make([]original.OpSettings, 0, len(defs))
		for _, d := range defs {
			c := controls[d.ID]
			if c == nil {
				continue
			}
			inten := d.DefaultInt
			if c.slider != nil {
				inten = int(c.slider.Value)
			}
			out = append(out, original.OpSettings{
				ID:        d.ID,
				Enabled:   c.check.Checked,
				Intensity: inten,
			})
		}
		return out
	}

	chainEntry := widget.NewMultiLineEntry()
	chainEntry.SetText("勾选后显示滤镜链")
	chainEntry.Wrapping = fyne.TextWrapWord
	chainEntry.SetMinRowsVisible(3)
	chainEntry.Disable()

	refreshing := false
	refreshChain := func() {
		if refreshing {
			return
		}
		refreshing = true
		defer func() { refreshing = false }()

		settings, notice := original.NormalizeSettings(collectSettings())
		for _, s := range settings {
			if c := controls[s.ID]; c != nil && c.check.Checked != s.Enabled {
				c.check.SetChecked(s.Enabled)
			}
		}
		st := style
		if fontSelect != nil {
			st.FontName = fontSelect.Selected
		}
		if primaryEntry != nil {
			st.PrimaryColor = strings.TrimSpace(primaryEntry.Text)
		}
		if outlineEntry != nil {
			st.OutlineColor = strings.TrimSpace(outlineEntry.Text)
		}
		wm := original.DefaultWatermarkText
		if wmText != nil {
			if t := strings.TrimSpace(wmText.Text); t != "" {
				wm = t
			}
		}
		p := original.Params{
			Settings:        settings,
			SubtitleSRT:     srtPath,
			Style:           st,
			WatermarkText:   wm,
			WatermarkCorner: watermarkCorner,
		}
		chain := original.DescribeFilters(p)
		var b strings.Builder
		b.WriteString(chain.Summary)
		if notice != "" {
			b.WriteString("\n")
			b.WriteString(notice)
		}
		b.WriteString("\nvf: ")
		if chain.VFJoin == "" {
			b.WriteString("（无）")
		} else {
			b.WriteString(chain.VFJoin)
		}
		b.WriteString("\naf: ")
		if chain.AFJoin == "" {
			b.WriteString("（无）")
		} else {
			b.WriteString(chain.AFJoin)
		}
		chainEntry.SetText(b.String())
	}

	// Single compact row: [check] [短滑条≈原宽20%] [完整提示，不省略]
	const opSliderWidth float32 = 64 // 原先 Border 中心拉满时约 300+，取约 20%
	makeOpRow := func(d original.OpDef) fyne.CanvasObject {
		hint := widget.NewLabel("")
		hint.Importance = widget.LowImportance
		updateHint := func(v float64) {
			if d.HintAt != nil {
				hint.SetText(d.HintAt(int(v)))
			} else if !d.HasIntensity {
				hint.SetText("")
			} else {
				hint.SetText(fmt.Sprintf("%d", int(v)))
			}
		}
		chk := widget.NewCheck(d.Name, func(bool) { refreshChain() })
		chk.SetChecked(d.DefaultOn)
		var slider *widget.Slider
		var row fyne.CanvasObject
		if d.HasIntensity {
			slider = widget.NewSlider(float64(d.MinInt), float64(d.MaxInt))
			slider.Step = 1
			slider.SetValue(float64(d.DefaultInt))
			updateHint(slider.Value)
			slider.OnChanged = func(v float64) {
				updateHint(v)
				refreshChain()
			}
			short := container.New(&fixedWidthLayout{width: opSliderWidth}, slider)
			row = container.NewHBox(chk, short, hint)
		} else {
			updateHint(0)
			desc := widget.NewLabel(d.Desc)
			desc.Importance = widget.LowImportance
			row = container.NewHBox(chk, desc)
		}
		controls[d.ID] = &opControl{def: d, check: chk, slider: slider, hint: hint, row: row}
		return row
	}

	makeGroupGrid := func(defs []original.OpDef) fyne.CanvasObject {
		items := make([]fyne.CanvasObject, 0, len(defs))
		for _, d := range defs {
			items = append(items, makeOpRow(d))
		}
		return container.NewGridWithColumns(2, items...)
	}

	commonBox := makeGroupGrid(original.OpDefsByGroup(original.GroupCommon))
	structBox := makeGroupGrid(original.OpDefsByGroup(original.GroupStruct))
	advancedBox := makeGroupGrid(original.OpDefsByGroup(original.GroupAdvanced))
	advancedAcc := widget.NewAccordion(widget.NewAccordionItem("高级 / 实验", advancedBox))

	fontSizeVal := widget.NewLabel(fmt.Sprintf("%d", style.FontSize))
	fontSizeSlider := widget.NewSlider(10, 72)
	fontSizeSlider.Step = 1
	fontSizeSlider.SetValue(float64(style.FontSize))
	fontSizeSlider.OnChanged = func(v float64) {
		style.FontSize = int(v)
		fontSizeVal.SetText(fmt.Sprintf("%d", style.FontSize))
		refreshChain()
	}

	fontSelect = widget.NewSelect([]string{"Arial", "Microsoft YaHei", "SimHei", "SimSun"}, func(string) { refreshChain() })
	fontSelect.SetSelected(style.FontName)

	primaryEntry = widget.NewEntry()
	primaryEntry.SetText(style.PrimaryColor)
	outlineEntry = widget.NewEntry()
	outlineEntry.SetText(style.OutlineColor)

	outlineVal := widget.NewLabel(fmt.Sprintf("%d", style.Outline))
	outlineSlider := widget.NewSlider(0, 8)
	outlineSlider.Step = 1
	outlineSlider.SetValue(float64(style.Outline))
	outlineSlider.OnChanged = func(v float64) {
		style.Outline = int(v)
		outlineVal.SetText(fmt.Sprintf("%d", style.Outline))
		refreshChain()
	}

	shadowVal := widget.NewLabel(fmt.Sprintf("%d", style.Shadow))
	shadowSlider := widget.NewSlider(0, 8)
	shadowSlider.Step = 1
	shadowSlider.SetValue(float64(style.Shadow))
	shadowSlider.OnChanged = func(v float64) {
		style.Shadow = int(v)
		shadowVal.SetText(fmt.Sprintf("%d", style.Shadow))
		refreshChain()
	}

	marginVal := widget.NewLabel(fmt.Sprintf("%d", style.MarginV))
	marginSlider := widget.NewSlider(0, 200)
	marginSlider.Step = 1
	marginSlider.SetValue(float64(style.MarginV))
	marginSlider.OnChanged = func(v float64) {
		style.MarginV = int(v)
		marginVal.SetText(fmt.Sprintf("%d", style.MarginV))
		refreshChain()
	}

	bgmVolVal := widget.NewLabel("25%")
	bgmVolSlider := widget.NewSlider(0, 100)
	bgmVolSlider.Step = 1
	bgmVolSlider.SetValue(25)
	bgmVolSlider.OnChanged = func(v float64) {
		bgmVolVal.SetText(fmt.Sprintf("%.0f%%", v))
	}

	wmText = widget.NewEntry()
	wmText.SetPlaceHolder("水印文字")
	wmText.SetText(original.DefaultWatermarkText)
	wmText.OnChanged = func(string) { refreshChain() }
	cornerSelect := widget.NewSelect([]string{"左上", "右上", "左下", "右下"}, func(s string) {
		switch s {
		case "右上":
			watermarkCorner = "tr"
		case "左下":
			watermarkCorner = "bl"
		case "右下":
			watermarkCorner = "br"
		default:
			watermarkCorner = "tl"
		}
		refreshChain()
	})
	cornerSelect.SetSelected("左上")

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

	pickSrtBtn := widget.NewButton("字幕", func() {
		files, err := pickSubtitleFiles()
		if err != nil {
			status.SetText("选择失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		srtPath = files[0]
		srtLabel.SetText(filepath.Base(srtPath))
		srtLabel.Importance = widget.MediumImportance
		status.SetText("")
		refreshChain()
	})
	clearSrtBtn := widget.NewButton("清", func() {
		srtPath = ""
		srtLabel.SetText("未选字幕")
		srtLabel.Importance = widget.LowImportance
		refreshChain()
	})
	clearSrtBtn.Importance = widget.LowImportance

	pickBgmBtn := widget.NewButton("BGM", func() {
		files, err := pickAudioFiles()
		if err != nil {
			status.SetText("选择失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		bgmPath = files[0]
		bgmLabel.SetText(filepath.Base(bgmPath))
		bgmLabel.Importance = widget.MediumImportance
		status.SetText("")
	})
	clearBgmBtn := widget.NewButton("清", func() {
		bgmPath = ""
		bgmLabel.SetText("未选音频")
		bgmLabel.Importance = widget.LowImportance
	})
	clearBgmBtn.Importance = widget.LowImportance

	openOutBtn := widget.NewButton("打开输出", nil)
	openOutBtn.Disable()
	openOutBtn.OnTapped = func() {
		if lastOut == "" {
			return
		}
		_ = media.OpenFile(filepath.Dir(lastOut))
	}

	var startBtn, previewBtn, cancelBtn *widget.Button

	setBusy := func(v bool) {
		busy = v
		if v {
			startBtn.Disable()
			previewBtn.Disable()
			cancelBtn.Enable()
			openOutBtn.Disable()
		} else {
			startBtn.Enable()
			previewBtn.Enable()
			cancelBtn.Disable()
			if lastOut != "" {
				openOutBtn.Enable()
			}
		}
	}

	buildParams := func() original.Params {
		style.FontName = fontSelect.Selected
		style.PrimaryColor = strings.TrimSpace(primaryEntry.Text)
		style.OutlineColor = strings.TrimSpace(outlineEntry.Text)
		return original.Params{
			VideoPath:       videoPath,
			Settings:        collectSettings(),
			SubtitleSRT:     srtPath,
			Style:           style,
			BGMPath:         bgmPath,
			BGMVolume:       bgmVolSlider.Value,
			WatermarkText:   strings.TrimSpace(wmText.Text),
			WatermarkCorner: watermarkCorner,
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
	}

	startJob := func(preview bool) {
		runMu.Lock()
		if busy {
			runMu.Unlock()
			return
		}
		if strings.TrimSpace(videoPath) == "" {
			runMu.Unlock()
			status.SetText("请先选择视频文件")
			return
		}
		params := buildParams()
		if err := original.ValidateParams(params); err != nil {
			runMu.Unlock()
			status.SetText(err.Error())
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		runMu.Unlock()

		progress.SetValue(5)
		if preview {
			status.SetText("预览中…")
		} else {
			status.SetText("导出中…")
		}

		go func() {
			var out string
			var err error
			if preview {
				out, err = original.Preview(ctx, params)
			} else {
				out, err = original.Run(ctx, params)
			}
			fyne.Do(func() {
				runMu.Lock()
				cancelRun = nil
				setBusy(false)
				runMu.Unlock()
				if err != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText("失败: " + err.Error())
					dialog.ShowError(err, state.Win)
					return
				}
				lastOut = out
				progress.SetValue(100)
				if preview {
					status.SetText("预览完成 · " + out)
					_ = media.OpenFile(out)
				} else {
					status.SetText("完成 · " + out)
				}
				openOutBtn.Enable()
			})
		}()
	}

	startBtn = widget.NewButton("导出", func() { startJob(false) })
	startBtn.Importance = widget.HighImportance
	previewBtn = widget.NewButton("预览5秒", func() { startJob(true) })
	cancelBtn = widget.NewButton("取消", func() {
		runMu.Lock()
		if cancelRun != nil {
			cancelRun()
		}
		runMu.Unlock()
	})
	cancelBtn.Disable()

	sec := func(text string) *widget.Label {
		l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		return l
	}

	shortSlide := func(s *widget.Slider, trail *widget.Label) fyne.CanvasObject {
		return container.NewHBox(
			container.New(&fixedWidthLayout{width: opSliderWidth}, s),
			trail,
		)
	}

	styleForm := widget.NewForm(
		widget.NewFormItem("字号", shortSlide(fontSizeSlider, fontSizeVal)),
		widget.NewFormItem("字体", fontSelect),
		widget.NewFormItem("主色", primaryEntry),
		widget.NewFormItem("描边色", outlineEntry),
		widget.NewFormItem("描边", shortSlide(outlineSlider, outlineVal)),
		widget.NewFormItem("阴影", shortSlide(shadowSlider, shadowVal)),
		widget.NewFormItem("距底", shortSlide(marginSlider, marginVal)),
		widget.NewFormItem("BGM音量", shortSlide(bgmVolSlider, bgmVolVal)),
	)

	moreAcc := widget.NewAccordion(
		widget.NewAccordionItem("字幕样式 / BGM 音量", styleForm),
		widget.NewAccordionItem("滤镜链（只读）", chainEntry),
	)

	toolbar := container.NewHBox(pickVideoBtn, previewBtn, startBtn, cancelBtn, openOutBtn)
	// 三列均分，避免字幕/BGM/水印控件挤成一团
	mediaRow := container.NewGridWithColumns(3,
		container.NewBorder(nil, nil, container.NewHBox(pickSrtBtn, clearSrtBtn), nil, srtLabel),
		container.NewBorder(nil, nil, container.NewHBox(pickBgmBtn, clearBgmBtn), nil, bgmLabel),
		container.NewBorder(nil, nil, widget.NewLabel("水印"), cornerSelect, wmText),
	)

	body := container.NewVBox(
		subtitle,
		container.NewBorder(nil, nil, toolbar, nil, videoLabel),
		progress,
		status,
		sec("常用"),
		commonBox,
		sec("结构"),
		structBox,
		advancedAcc,
		mediaRow,
		moreAcc,
	)

	refreshChain()
	return container.NewVScroll(body)
}
