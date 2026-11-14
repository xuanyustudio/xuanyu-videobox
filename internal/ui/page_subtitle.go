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

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func newSubtitlePage(state *AppState) fyne.CanvasObject {
	var (
		mu      sync.Mutex
		busy    bool
		cancel  context.CancelFunc
		videos  []string
		subs    []string
		lastOut string
	)

	intro := widget.NewLabel("选择一个或多个视频，再选择字幕。同名的字幕会烧进去，对不上的视频用设置里的语音识别。成片和识别出的字幕写到视频目录下的 srt 文件夹。")
	intro.Wrapping = fyne.TextWrapWord
	intro.Importance = widget.LowImportance

	videoLabel := widget.NewLabel("未选择视频")
	videoLabel.Wrapping = fyne.TextWrapWord
	subLabel := widget.NewLabel("未选择字幕")
	subLabel.Wrapping = fyne.TextWrapWord

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	style := subtitleburn.DefaultStyle()
	const sliderWidth float32 = 220

	fontSizeVal := widget.NewLabel(fmt.Sprintf("%d", style.FontSize))
	fontSizeSlider := widget.NewSlider(10, 72)
	fontSizeSlider.Step = 1
	fontSizeSlider.SetValue(float64(style.FontSize))
	fontSizeSlider.OnChanged = func(v float64) {
		fontSizeVal.SetText(fmt.Sprintf("%d", int(v)))
	}

	fontSelect := widget.NewSelect([]string{"Arial", "Microsoft YaHei", "SimHei", "SimSun"}, nil)
	fontSelect.SetSelected(style.FontName)

	primaryEntry := widget.NewEntry()
	primaryEntry.SetText(style.PrimaryColor)
	outlineEntry := widget.NewEntry()
	outlineEntry.SetText(style.OutlineColor)

	outlineVal := widget.NewLabel(fmt.Sprintf("%d", style.Outline))
	outlineSlider := widget.NewSlider(0, 8)
	outlineSlider.Step = 1
	outlineSlider.SetValue(float64(style.Outline))
	outlineSlider.OnChanged = func(v float64) {
		outlineVal.SetText(fmt.Sprintf("%d", int(v)))
	}

	shadowVal := widget.NewLabel(fmt.Sprintf("%d", style.Shadow))
	shadowSlider := widget.NewSlider(0, 8)
	shadowSlider.Step = 1
	shadowSlider.SetValue(float64(style.Shadow))
	shadowSlider.OnChanged = func(v float64) {
		shadowVal.SetText(fmt.Sprintf("%d", int(v)))
	}

	marginVal := widget.NewLabel(fmt.Sprintf("%d", style.MarginV))
	marginSlider := widget.NewSlider(0, 200)
	marginSlider.Step = 1
	marginSlider.SetValue(float64(style.MarginV))
	marginSlider.OnChanged = func(v float64) {
		marginVal.SetText(fmt.Sprintf("%d", int(v)))
	}

	shortSlide := func(s *widget.Slider, trail *widget.Label) fyne.CanvasObject {
		return container.NewHBox(
			container.New(&fixedWidthLayout{width: sliderWidth}, s),
			trail,
		)
	}

	var pickVideoBtn, pickSubBtn, clearSubBtn, startBtn, cancelBtn, openBtn *widget.Button

	setBusy := func(v bool) {
		busy = v
		if v {
			pickVideoBtn.Disable()
			pickSubBtn.Disable()
			clearSubBtn.Disable()
			startBtn.Disable()
			cancelBtn.Enable()
			openBtn.Disable()
			return
		}
		pickVideoBtn.Enable()
		pickSubBtn.Enable()
		clearSubBtn.Enable()
		startBtn.Enable()
		cancelBtn.Disable()
		if lastOut != "" {
			openBtn.Enable()
		}
	}

	readStyle := func() subtitleburn.Style {
		return subtitleburn.Style{
			FontSize:     int(fontSizeSlider.Value),
			FontName:     fontSelect.Selected,
			PrimaryColor: strings.TrimSpace(primaryEntry.Text),
			OutlineColor: strings.TrimSpace(outlineEntry.Text),
			Outline:      int(outlineSlider.Value),
			Shadow:       int(shadowSlider.Value),
			MarginV:      int(marginSlider.Value),
		}
	}

	pickVideoBtn = widget.NewButton("选择视频", func() {
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
		mu.Lock()
		videos = append([]string(nil), paths...)
		mu.Unlock()
		videoLabel.SetText(selectionText(paths, "未选择视频", "已选 %d 个视频"))
		status.SetText("")
	})

	pickSubBtn = widget.NewButton("选择字幕", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		paths, err := pickSubtitleFiles()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if len(paths) == 0 {
			return
		}
		mu.Lock()
		subs = append([]string(nil), paths...)
		mu.Unlock()
		subLabel.SetText(selectionText(paths, "未选择字幕", "已选 %d 个字幕"))
		status.SetText("")
	})

	clearSubBtn = widget.NewButton("清除字幕", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		subs = nil
		mu.Unlock()
		subLabel.SetText("未选择字幕")
	})
	clearSubBtn.Importance = widget.LowImportance

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

	runJob := func(bins media.FFmpegBins, items []subtitleburn.Item, style subtitleburn.Style, opt asr.Options) {
		ctx, cancelFn := context.WithCancel(context.Background())
		mu.Lock()
		cancel = cancelFn
		setBusy(true)
		mu.Unlock()
		progress.SetValue(0)
		status.SetText("正在处理…")

		go func() {
			var done []string
			var fail error
			var failCount int
			readyErr := error(nil)
			needASR := false
			for _, it := range items {
				if it.Subtitle == "" {
					needASR = true
					break
				}
			}
			if needASR {
				readyErr = asr.RequireReady(opt)
			}
			for i, it := range items {
				if ctx.Err() != nil {
					break
				}
				name := filepath.Base(it.Video)
				n, total := i+1, len(items)
				subPath := it.Subtitle
				if subPath == "" {
					if readyErr != nil {
						if fail == nil {
							fail = readyErr
						}
						failCount++
						continue
					}
					fyne.Do(func() {
						progress.SetValue(float64(i) / float64(total) * 100)
						status.SetText(fmt.Sprintf("正在识别 %d/%d %s…", n, total, name))
					})
					asrOut, err := subtitleburn.PrepareOutput(it.Video, "_asr.srt")
					if err != nil {
						if fail == nil {
							fail = err
						}
						failCount++
						continue
					}
					if err := asr.Transcribe(it.Video, asrOut, opt); err != nil {
						if ctx.Err() != nil {
							break
						}
						if fail == nil {
							fail = err
						}
						failCount++
						continue
					}
					if ctx.Err() != nil {
						break
					}
					subPath = asrOut
				}
				fyne.Do(func() {
					progress.SetValue(float64(i) / float64(total) * 100)
					status.SetText(fmt.Sprintf("正在烧录 %d/%d %s…", n, total, name))
				})
				outPath, err := subtitleburn.PrepareOutput(it.Video, "_sub.mp4")
				if err != nil {
					if fail == nil {
						fail = err
					}
					failCount++
					continue
				}
				err = subtitleburn.BurnProgress(ctx, bins, it.Video, subPath, outPath, style, func(pct int) {
					fyne.Do(func() {
						overall := (float64(i) + float64(pct)/100) / float64(total) * 100
						progress.SetValue(overall)
						status.SetText(fmt.Sprintf("正在烧录 %d/%d %s… %d%%", n, total, name, pct))
					})
				})
				if err != nil {
					if ctx.Err() != nil {
						break
					}
					if fail == nil {
						fail = err
					}
					failCount++
					continue
				}
				done = append(done, outPath)
			}
			fyne.Do(func() {
				mu.Lock()
				cancel = nil
				if len(done) > 0 {
					lastOut = done[len(done)-1]
				}
				setBusy(false)
				mu.Unlock()
				switch {
				case ctx.Err() != nil:
					status.SetText(fmt.Sprintf("已停止，已完成 %d 个", len(done)))
				case fail != nil:
					if len(items) > 0 {
						progress.SetValue(float64(len(done)) / float64(len(items)) * 100)
					}
					status.SetText(fmt.Sprintf("成功 %d 个，失败 %d 个：%s", len(done), failCount, fail.Error()))
				case len(done) == 1:
					progress.SetValue(100)
					status.SetText("已保存 " + done[0])
				case len(done) > 1:
					progress.SetValue(100)
					status.SetText(fmt.Sprintf("已保存 %d 个，最后一个：%s", len(done), done[len(done)-1]))
				default:
					status.SetText("已停止，已完成 0 个")
				}
			})
		}()
	}

	startBtn = widget.NewButton("开始", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		batchV := append([]string(nil), videos...)
		batchS := append([]string(nil), subs...)
		mu.Unlock()
		if len(batchV) == 0 {
			dialog.ShowInformation("未选择视频", "请先选择要加字幕的视频。", state.Win)
			return
		}
		bins, err := resolveFFmpeg(state)
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		items, unused := subtitleburn.Pair(batchV, batchS)
		jobStyle := readStyle()
		opt := asr.Options{
			Backend:       state.Cfg.ASR.Backend,
			APIURL:        state.Cfg.ASR.APIURL,
			FireRedAPIURL: state.Cfg.ASR.FireRedAPIURL,
			BailianAPIKey: state.Cfg.ASR.APIKey,
			Hotword:       state.Cfg.ASR.Hotword,
			EnableSpk:     state.Cfg.ASR.EnableSpk,
		}
		launch := func() {
			mu.Lock()
			if busy {
				mu.Unlock()
				return
			}
			mu.Unlock()
			runJob(bins, items, jobStyle, opt)
		}
		if len(unused) > 0 {
			names := make([]string, 0, len(unused))
			for _, p := range unused {
				names = append(names, filepath.Base(p))
			}
			mu.Lock()
			busy = true
			pickVideoBtn.Disable()
			pickSubBtn.Disable()
			clearSubBtn.Disable()
			startBtn.Disable()
			mu.Unlock()
			d := dialog.NewInformation("有字幕未配对", "以下字幕没有匹配的视频，将不会使用：\n"+strings.Join(names, "\n"), state.Win)
			d.SetOnClosed(func() {
				mu.Lock()
				busy = false
				mu.Unlock()
				launch()
			})
			d.Show()
			return
		}
		launch()
	})
	startBtn.Importance = widget.HighImportance

	styleForm := widget.NewForm(
		widget.NewFormItem("字号", shortSlide(fontSizeSlider, fontSizeVal)),
		widget.NewFormItem("字体", fontSelect),
		widget.NewFormItem("主色", primaryEntry),
		widget.NewFormItem("描边色", outlineEntry),
		widget.NewFormItem("描边", shortSlide(outlineSlider, outlineVal)),
		widget.NewFormItem("阴影", shortSlide(shadowSlider, shadowVal)),
		widget.NewFormItem("距底部", shortSlide(marginSlider, marginVal)),
	)

	body := container.NewVBox(
		pageTitle("加字幕"),
		intro,
		container.NewHBox(pickVideoBtn, pickSubBtn, clearSubBtn, startBtn, cancelBtn, openBtn),
		videoLabel,
		subLabel,
		progress,
		status,
		sectionTitle("字幕样式"),
		styleForm,
	)
	return container.NewVScroll(container.NewPadded(body))
}

func selectionText(paths []string, empty, manyFmt string) string {
	switch len(paths) {
	case 0:
		return empty
	case 1:
		return paths[0]
	default:
		return fmt.Sprintf(manyFmt, len(paths))
	}
}
