//go:build fyne_gui

package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/intro"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/replicate"
)

func newIntroPage(state *AppState) fyne.CanvasObject {
	imagePath := ""
	fourPath := ""
	outDir := ""

	var (
		runMu     sync.Mutex
		cancelRun context.CancelFunc
		busy      bool
	)

	imageLabel := widget.NewLabel("未选择图片")
	imageLabel.Truncation = fyne.TextTruncateEllipsis

	status := widget.NewLabel("")
	status.Truncation = fyne.TextTruncateEllipsis
	status.Importance = widget.LowImportance
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100

	subtitle := widget.NewLabel("上传角色图，生成四视图，再让演员对着镜头用 15 秒介绍自己的新片子。")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	tpls := intro.PromptTemplates()
	tplLabels := make([]string, len(tpls))
	for i, tpl := range tpls {
		tplLabels[i] = tpl.Name
	}
	promptEntry := widget.NewMultiLineEntry()
	promptEntry.Wrapping = fyne.TextWrapWord
	promptEntry.SetMinRowsVisible(8)
	tplSelect := widget.NewSelect(tplLabels, func(name string) {
		promptEntry.SetText(intro.PromptByName(name))
	})
	tplSelect.SetSelected(tplLabels[0])
	promptEntry.SetText(intro.DefaultIntroPrompt)

	refSelect := widget.NewRadioGroup([]string{"四视图", "原图"}, nil)
	refSelect.Horizontal = true
	refSelect.SetSelected("四视图")

	const previewW, previewH float32 = 320, 220
	sourceBox := container.NewGridWrap(fyne.NewSize(previewW, previewH))
	fourBox := container.NewGridWrap(fyne.NewSize(previewW, previewH))
	sourceHint := widget.NewLabel("选择图片后在此预览")
	sourceHint.Importance = widget.LowImportance
	fourHint := widget.NewLabel("生成后在此预览四视图")
	fourHint.Importance = widget.LowImportance

	showImage := func(box *fyne.Container, path string) {
		box.Objects = nil
		if path != "" {
			img := canvas.NewImageFromFile(path)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(previewW, previewH))
			box.Add(img)
		}
		box.Refresh()
	}

	openOutBtn := widget.NewButton("打开输出", nil)
	openOutBtn.Disable()
	openOutBtn.OnTapped = func() {
		if outDir == "" {
			return
		}
		_ = media.OpenFile(outDir)
	}

	var fourBtn, videoBtn, cancelBtn *widget.Button

	setBusy := func(v bool) {
		busy = v
		if v {
			fourBtn.Disable()
			videoBtn.Disable()
			cancelBtn.Enable()
			openOutBtn.Disable()
		} else {
			fourBtn.Enable()
			videoBtn.Enable()
			cancelBtn.Disable()
			if outDir != "" {
				openOutBtn.Enable()
			}
		}
	}

	ensureDir := func() (string, error) {
		if outDir != "" {
			return outDir, nil
		}
		dir := intro.OutputDir(imagePath, time.Now())
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		outDir = dir
		return dir, nil
	}

	pickBtn := widget.NewButton("选择图片", func() {
		if busy {
			return
		}
		path, err := pickImageFile()
		if err != nil {
			status.SetText("选择失败: " + err.Error())
			return
		}
		if path == "" {
			return
		}
		imagePath = path
		fourPath = ""
		outDir = ""
		imageLabel.SetText(filepath.Base(path))
		showImage(sourceBox, path)
		sourceHint.SetText(filepath.Base(path))
		showImage(fourBox, "")
		fourHint.SetText("生成后在此预览四视图")
		refSelect.SetSelected("四视图")
		openOutBtn.Disable()
		progress.SetValue(0)
		status.SetText("")
	})

	clearBtn := widget.NewButton("清空", func() {
		if busy {
			return
		}
		imagePath = ""
		fourPath = ""
		outDir = ""
		imageLabel.SetText("未选择图片")
		showImage(sourceBox, "")
		showImage(fourBox, "")
		sourceHint.SetText("选择图片后在此预览")
		fourHint.SetText("生成后在此预览四视图")
		refSelect.SetSelected("四视图")
		tplSelect.SetSelected(tplLabels[0])
		promptEntry.SetText(intro.DefaultIntroPrompt)
		progress.SetValue(0)
		status.SetText("")
		openOutBtn.Disable()
	})
	clearBtn.Importance = widget.LowImportance

	start := func(kind string) {
		runMu.Lock()
		if busy {
			runMu.Unlock()
			return
		}
		if strings.TrimSpace(imagePath) == "" {
			runMu.Unlock()
			status.SetText("请先选择图片")
			return
		}
		ref := imagePath
		if kind != "four" && refSelect.Selected != "原图" {
			if strings.TrimSpace(fourPath) == "" {
				runMu.Unlock()
				status.SetText("请先生成四视图，或把参考图改成原图")
				return
			}
			ref = fourPath
		}
		dir, err := ensureDir()
		if err != nil {
			runMu.Unlock()
			status.SetText(err.Error())
			return
		}
		base := strings.TrimSpace(state.Cfg.Image.BaseURL)
		if base == "" {
			base = strings.TrimSpace(state.Cfg.Replicate.BaseURL)
		}
		if base == "" {
			base = replicate.DefaultBaseURL
		}
		apiKey := strings.TrimSpace(state.Cfg.Image.APIKey)
		if apiKey == "" {
			apiKey = state.Cfg.Replicate.APIKey
		}
		imageModel := strings.TrimSpace(state.Cfg.Image.Model)
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		prompt := promptEntry.Text
		videoKey := state.Cfg.Video.APIKey
		src := imagePath
		runMu.Unlock()

		progress.SetValue(5)
		onProgress := func(percent int, msg string) {
			fyne.Do(func() {
				progress.SetValue(float64(percent))
				if msg != "" {
					status.SetText(msg)
				}
			})
		}

		go func() {
			var runErr error
			var saved string
			switch kind {
			case "four":
				statusText := "正在生成四视图…"
				fyne.Do(func() { status.SetText(statusText) })
				state.Log.Append("[介绍片] 四视图 " + src)
				saved, runErr = intro.GenerateFourView(ctx, intro.FourViewOptions{
					ImagePath: src,
					OutPath:   filepath.Join(dir, "fourview.png"),
					BaseURL:   base,
					APIKey:    apiKey,
					Model:     imageModel,
				})
			default:
				state.Log.Append("[介绍片] 视频 " + ref)
				saved, runErr = intro.GenerateIntro(ctx, intro.IntroOptions{
					ImagePath:  ref,
					Prompt:     prompt,
					OutPath:    filepath.Join(dir, "intro.mp4"),
					APIKey:     videoKey,
					Duration:   intro.VideoDuration,
					OnProgress: onProgress,
				})
			}
			fyne.Do(func() {
				runMu.Lock()
				cancelRun = nil
				setBusy(false)
				runMu.Unlock()
				if runErr != nil {
					progress.SetValue(0)
					if ctx.Err() != nil {
						status.SetText("已取消")
						return
					}
					status.SetText(runErr.Error())
					state.Log.Append("[介绍片] 失败: " + runErr.Error())
					dialog.ShowError(runErr, state.Win)
					return
				}
				progress.SetValue(100)
				if kind == "four" {
					fourPath = saved
					showImage(fourBox, saved)
					fourHint.SetText("四视图 · " + saved)
					status.SetText("四视图已保存 · " + saved)
					state.Log.Append("[介绍片] 四视图完成: " + saved)
					return
				}
				status.SetText("介绍片已保存 · " + saved)
				state.Log.Append("[介绍片] 视频完成: " + saved)
			})
		}()
	}

	fourBtn = widget.NewButton("生成四视图", func() { start("four") })
	videoBtn = widget.NewButton("生成介绍片", func() { start("video") })
	videoBtn.Importance = widget.HighImportance
	cancelBtn = widget.NewButton("取消", func() {
		runMu.Lock()
		if cancelRun != nil {
			cancelRun()
		}
		runMu.Unlock()
	})
	cancelBtn.Disable()

	hint := widget.NewLabel("四视图使用「设置」底部的生图地址和生图模型。介绍片默认用四视图做参考，固定 15 秒竖屏。换模板会覆盖文案，改完的字会按你写的生成。")
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	sec := func(text string) *widget.Label {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}

	toolbar := container.NewHBox(pickBtn, fourBtn, videoBtn, cancelBtn, openOutBtn, clearBtn)
	previews := container.NewGridWithColumns(2,
		container.NewVBox(sec("参考图"), sourceHint, sourceBox),
		container.NewVBox(sec("四视图"), fourHint, fourBox),
	)
	body := container.NewVBox(
		subtitle,
		container.NewBorder(nil, nil, toolbar, nil, imageLabel),
		progress,
		status,
		previews,
		sec("视频参考图"),
		refSelect,
		sec("介绍文案"),
		container.NewBorder(nil, nil, widget.NewLabel("模板"), nil, tplSelect),
		promptEntry,
		hint,
	)
	return container.NewVScroll(body)
}
