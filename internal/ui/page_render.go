//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/pipeline"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/tts"
)

func newRenderPage(state *AppState) fyne.CanvasObject {
	voiceLabels := tts.VoiceLabels()
	voiceSelect := widget.NewSelect(voiceLabels, nil)
	voiceSelect.PlaceHolder = "选择配音"
	currentLabel := tts.LabelForVoiceID(state.Cfg.TTS.Voice)
	found := false
	for _, lab := range voiceLabels {
		if lab == currentLabel {
			found = true
			break
		}
	}
	if !found && currentLabel != "" {
		voiceLabels = append([]string{currentLabel}, voiceLabels...)
		voiceSelect.Options = voiceLabels
	}
	voiceSelect.SetSelected(currentLabel)

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100

	logEntry := widget.NewMultiLineEntry()
	logEntry.SetMinRowsVisible(12)
	logEntry.Wrapping = fyne.TextWrapWord

	refreshLog := func() {
		logEntry.SetText(state.Log.String())
		logEntry.CursorRow = len(logEntry.Text)
	}

	startBtn := widget.NewButton("开始合成", nil)
	startBtn.Importance = widget.HighImportance
	startBtn.OnTapped = func() {
		if state.Project == nil {
			state.Log.Append("请先打开工程")
			refreshLog()
			return
		}
		if len(state.Project.VideoPaths) == 0 {
			state.Log.Append("需要至少一个视频")
			refreshLog()
			return
		}
		if len(state.Project.ScriptItems) == 0 {
			state.Log.Append("脚本为空，请先生成脚本")
			refreshLog()
			return
		}

		voice := tts.VoiceIDForLabel(voiceSelect.Selected)
		if voice == "" {
			voice = state.Cfg.TTS.Voice
		}
		if voice == "" {
			voice = tts.EdgeVoices()[0].ID
		}
		state.Cfg.TTS.Voice = voice

		taskID := time.Now().Format("20060102-150405")
		workDir := filepath.Join(state.ProjectsRoot, state.Project.Slug, "tasks", taskID)
		outPath := filepath.Join(workDir, "final.mp4")

		state.Log.Clear()
		state.Log.Append("任务 " + taskID)
		refreshLog()
		progress.SetValue(0)
		startBtn.Disable()

		go func() {
			synth := pickSynth(state)
			hide := state.Cfg.UI.SubtitleHideOnOriginal
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
			appendDiskLog("start voice=" + voice)

			params := pipeline.Params{
				Items:      append([]script.Item(nil), state.Project.ScriptItems...),
				VideoPaths: append([]string(nil), state.Project.VideoPaths...),
				WorkDir:    workDir,
				OutputPath: outPath,
				TTS:        synth,
				Voice:      voice,
				Bins: media.FFmpegBins{
					FFmpeg:  state.Cfg.FFmpeg.Path,
					FFprobe: state.Cfg.FFmpeg.FFprobePath,
				},
				HideOnOriginal: &hide,
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

			out, err := pipeline.Run(context.Background(), params)
			fyne.Do(func() {
				startBtn.Enable()
				if err != nil {
					msg := "失败: " + err.Error()
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
	}

	top := widget.NewCard("成片合成", "按脚本裁剪、配音并输出 final.mp4", container.NewVBox(
		widget.NewForm(widget.NewFormItem("语音", voiceSelect)),
		startBtn,
		progress,
	))

	return container.NewPadded(container.NewBorder(
		top, nil, nil, nil,
		widget.NewCard("运行日志", "", logEntry),
	))
}

func pickSynth(state *AppState) tts.Synthesizer {
	_ = state
	return tts.Edge{}
}

func synthName(s tts.Synthesizer) string {
	switch s.(type) {
	case tts.Edge:
		return "edge"
	case tts.Fake:
		return "fake"
	default:
		return "other"
	}
}
