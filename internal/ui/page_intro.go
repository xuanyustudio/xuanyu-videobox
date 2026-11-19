//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
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
	"github.com/narratoai/narrato-go/internal/voiceclone"
)

func copyLocalRefAudio(src, dir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(src))
	if ext == "" {
		ext = ".mp3"
	}
	dest := filepath.Join(dir, "voice_ref"+ext)
	if filepath.Clean(src) == filepath.Clean(dest) {
		return dest, nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("读取本地参考音频失败: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("本地参考音频是空文件")
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("保存本地参考音频失败: %w", err)
	}
	return dest, nil
}

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
	promptEntry.SetMinRowsVisible(6)
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
	var picker *voicePicker

	setCancel := func(fn context.CancelFunc) {
		runMu.Lock()
		cancelRun = fn
		runMu.Unlock()
	}

	setBusy := func(v bool) {
		busy = v
		if v {
			fourBtn.Disable()
			videoBtn.Disable()
			cancelBtn.Enable()
			openOutBtn.Disable()
			if picker != nil {
				picker.SetEnabled(false)
			}
		} else {
			fourBtn.Enable()
			videoBtn.Enable()
			cancelBtn.Disable()
			if outDir != "" {
				openOutBtn.Enable()
			}
			if picker != nil {
				picker.SetEnabled(true)
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
		if picker != nil {
			picker.ClearSelection()
		}
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
		var (
			voiceID    string
			voiceLabel string
			reuseAudio string
			spoken     string
			useVoice   bool
			localAudio string
		)
		if picker != nil && kind != "four" {
			v, preview, ok := picker.Selection()
			if ok {
				useVoice = true
				voiceID = v.ID
				voiceLabel = v.Display()
				spoken = intro.SpokenLines(prompt)
				reuseAudio = picker.PreviewFor(spoken)
				if reuseAudio == "" {
					reuseAudio = preview
				}
			} else if path, ok := picker.UploadedAudio(); ok {
				localAudio = path
				voiceLabel = filepath.Base(path)
			}
		}
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
				state.Log.Append("[口播视频] 四视图 " + src)
				saved, runErr = intro.GenerateFourView(ctx, intro.FourViewOptions{
					ImagePath: src,
					OutPath:   filepath.Join(dir, "fourview.png"),
					BaseURL:   base,
					APIKey:    apiKey,
					Model:     imageModel,
				})
			default:
				audioPath := ""
				if useVoice {
					if strings.TrimSpace(spoken) == "" {
						runErr = fmt.Errorf("请填写介绍文案，才能按参考音色合成试听音频")
					} else if reuseAudio != "" && picker.PreviewFor(spoken) == reuseAudio {
						audioPath = reuseAudio
					} else if reuseAudio != "" && spoken != "" {
						// Prefer a fresh performance of the current copy when the
						// existing preview was made with different text.
						audioPath = ""
					}
					if runErr == nil && audioPath == "" {
						onProgress(6, "正在按参考音色合成试听音频…")
						state.Log.Append("[口播视频] 合成参考音频 " + voiceID)
						client := &voiceclone.Client{
							BaseURL: state.Cfg.MiniMax.BaseURL,
							APIKey:  strings.TrimSpace(state.Cfg.MiniMax.APIKey),
							Model:   strings.TrimSpace(state.Cfg.MiniMax.Model),
						}
						var result *voiceclone.SynthesizeResult
						result, runErr = client.Synthesize(ctx, voiceID, spoken)
						if runErr == nil {
							audioPath = filepath.Join(dir, "voice_ref."+extOrMP3(result.Format))
							runErr = os.WriteFile(audioPath, result.Audio, 0o644)
							if runErr == nil {
								picker.rememberPreview(audioPath, spoken)
							}
						}
					} else if runErr == nil && audioPath != "" {
						state.Log.Append("[口播视频] 使用已有试听音频 " + audioPath)
					}
				} else if localAudio != "" {
					onProgress(6, "正在使用本地参考音频…")
					audioPath, runErr = copyLocalRefAudio(localAudio, dir)
					if runErr == nil {
						state.Log.Append("[口播视频] 使用本地参考音频 " + audioPath)
					}
				}
				if runErr == nil {
					state.Log.Append("[口播视频] 视频 " + ref)
					if audioPath != "" {
						state.Log.Append("[口播视频] 参考音频 " + audioPath + "（" + voiceLabel + "）")
					}
					saved, runErr = intro.GenerateIntro(ctx, intro.IntroOptions{
						ImagePath:  ref,
						AudioPath:  audioPath,
						Prompt:     prompt,
						OutPath:    filepath.Join(dir, "intro.mp4"),
						APIKey:     videoKey,
						Duration:   intro.VideoDuration,
						OnProgress: onProgress,
					})
				}
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
					state.Log.Append("[口播视频] 失败: " + runErr.Error())
					dialog.ShowError(runErr, state.Win)
					return
				}
				progress.SetValue(100)
				if kind == "four" {
					fourPath = saved
					showImage(fourBox, saved)
					fourHint.SetText("四视图: " + saved)
					status.SetText("四视图已保存: " + saved)
					state.Log.Append("[口播视频] 四视图完成: " + saved)
					return
				}
				status.SetText("口播视频已保存: " + saved)
				state.Log.Append("[口播视频] 视频完成: " + saved)
			})
		}()
	}

	fourBtn = widget.NewButton("生成四视图", func() { start("four") })
	videoBtn = widget.NewButton("生成口播视频", func() { start("video") })
	videoBtn.Importance = widget.HighImportance
	cancelBtn = widget.NewButton("取消", func() {
		runMu.Lock()
		if cancelRun != nil {
			cancelRun()
		}
		runMu.Unlock()
	})
	cancelBtn.Disable()

	hint := widget.NewLabel("四视图使用「设置」底部的生图地址和生图模型。口播视频默认用四视图做参考，固定 15 秒竖屏。换模板会覆盖文案，改完的字会按你写的生成。参考音色可留空；选了列表里的音色后会先按介绍文案合成试听，再把这段音频作为参考音提交。也可以点「上传音色」直接选用本地 wav、mp3 等音频，不必先查询。查询或选用列表音色时请先在「设置」里填写 MiniMax API Key。若当前默认视频模型不支持参考音频，生成时会自动改用账号里可用的模型。")
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	sec := sectionTitle

	toolbar := container.NewHBox(pickBtn, fourBtn, videoBtn, cancelBtn, openOutBtn, clearBtn)
	previews := container.NewGridWithColumns(2,
		container.NewVBox(sec("参考图"), sourceHint, sourceBox),
		container.NewVBox(sec("四视图"), fourHint, fourBox),
	)

	picker = newVoicePicker(state, progress, status, func() string {
		return intro.SpokenLines(promptEntry.Text)
	}, func() bool { return busy }, setBusy, setCancel)

	top := container.NewVBox(
		pageTitle("口播视频"),
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
	)
	return container.NewBorder(container.NewPadded(top), container.NewPadded(hint), nil, nil, picker.root)
}
