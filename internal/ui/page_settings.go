//go:build fyne_gui

package ui

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/replicate"
)

const (
	asrChoiceBailian = "阿里百炼 Fun-ASR（在线）"
	asrChoiceFireRed = "本地 FireRedASR"
)

func newSettingsPage(state *AppState) fyne.CanvasObject {
	baseURL := widget.NewEntry()
	baseURL.SetText(state.Cfg.LLM.BaseURL)
	apiKey := widget.NewPasswordEntry()
	apiKey.SetText(state.Cfg.LLM.APIKey)
	model := widget.NewEntry()
	model.SetText(state.Cfg.LLM.Model)

	ffmpegPath := widget.NewEntry()
	ffmpegPath.SetText(state.Cfg.FFmpeg.Path)
	ffmpegPath.SetPlaceHolder("留空 = 系统 PATH")
	ffprobePath := widget.NewEntry()
	ffprobePath.SetText(state.Cfg.FFmpeg.FFprobePath)
	ffprobePath.SetPlaceHolder("留空 = 系统 PATH")

	backend := widget.NewSelect([]string{
		asrChoiceBailian,
		asr.LabelLocalFunASR,
		asrChoiceFireRed,
	}, nil)
	switch asr.NormalizeBackend(state.Cfg.ASR.Backend) {
	case asr.BackendFireRed:
		backend.SetSelected(asrChoiceFireRed)
	case asr.BackendLocal:
		backend.SetSelected(asr.LabelLocalFunASR)
	default:
		backend.SetSelected(asrChoiceBailian)
	}

	asrURL := widget.NewEntry()
	asrURL.SetText(state.Cfg.ASR.APIURL)
	asrURL.SetPlaceHolder(asr.DefaultAPIURL)
	fireredURL := widget.NewEntry()
	fireredURL.SetText(state.Cfg.ASR.FireRedAPIURL)
	fireredURL.SetPlaceHolder(asr.DefaultFireRedAPIURL)
	bailianKey := widget.NewPasswordEntry()
	bailianKey.SetText(state.Cfg.ASR.APIKey)
	hotword := widget.NewEntry()
	hotword.SetText(state.Cfg.ASR.Hotword)
	hotword.SetPlaceHolder("可选")
	spkCheck := widget.NewCheck("说话人分离", nil)
	spkCheck.SetChecked(state.Cfg.ASR.EnableSpk)

	repBaseURL := widget.NewEntry()
	repBaseURL.SetText(state.Cfg.Replicate.BaseURL)
	repBaseURL.SetPlaceHolder(replicate.DefaultBaseURL)
	repAPIKey := widget.NewPasswordEntry()
	repAPIKey.SetText(state.Cfg.Replicate.APIKey)
	repModel := widget.NewEntry()
	repModel.SetText(state.Cfg.Replicate.Model)
	repModel.SetPlaceHolder(replicate.DefaultModel)

	dyBaseURL := widget.NewEntry()
	dyBaseURL.SetText(state.Cfg.Douyin.BaseURL)
	dyBaseURL.SetPlaceHolder("http://39.109.101.66:8000")
	dyAPIKey := widget.NewPasswordEntry()
	dyAPIKey.SetText(state.Cfg.Douyin.APIKey)

	videoKey := widget.NewPasswordEntry()
	videoKey.SetText(state.Cfg.Video.APIKey)
	videoKey.SetPlaceHolder("vw_ 开头的工作台密钥")

	imgBaseURL := widget.NewEntry()
	imgBaseURL.SetText(state.Cfg.Image.BaseURL)
	imgBaseURL.SetPlaceHolder("留空则使用复刻 Base URL")
	imgAPIKey := widget.NewPasswordEntry()
	imgAPIKey.SetText(state.Cfg.Image.APIKey)
	imgAPIKey.SetPlaceHolder("留空则使用复刻 API Key")
	imgModel := widget.NewEntry()
	imgModel.SetText(state.Cfg.Image.Model)
	if strings.TrimSpace(imgModel.Text) == "" {
		imgModel.SetText("gpt-image-1")
	}
	imgModel.SetPlaceHolder("gpt-image-2")

	hideCheck := widget.NewCheck("原片段隐藏解说字幕", func(bool) {})
	hideCheck.SetChecked(state.Cfg.UI.SubtitleHideOnOriginal)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	oneClickStatus := widget.NewLabel("")
	oneClickStatus.Wrapping = fyne.TextWrapWord

	asrHint := widget.NewLabel("")
	asrHint.Wrapping = fyne.TextWrapWord
	asrHint.Importance = widget.LowImportance

	localBox := container.NewVBox()
	bailianBox := settingsField("百炼 API Key", bailianKey)
	fireredBox := settingsField("FireRed 地址", fireredURL)
	asrBody := container.NewVBox()

	var showASR func(string)
	oneClick := widget.NewButton("一键配置本地 FunASR", func() {
		backendName, apiURL := asr.LocalFunASRPreset()
		backend.SetSelected(asr.LabelLocalFunASR)
		asrURL.SetText(apiURL)
		state.Cfg.ASR.Backend = backendName
		state.Cfg.ASR.APIURL = apiURL
		if showASR != nil {
			showASR(asr.LabelLocalFunASR)
		}
		saveErr := config.Save(state.CfgPath, state.Cfg)
		if saveErr != nil {
			oneClickStatus.SetText("已填入本地 FunASR（SenseVoiceSmall），保存失败: " + saveErr.Error())
		} else {
			oneClickStatus.SetText("已保存本地 FunASR（SenseVoiceSmall），正在检测服务…")
		}
		go func() {
			err := (&asr.Client{APIURL: apiURL}).Probe()
			fyne.Do(func() {
				line := asr.FormatLocalProbeStatus(err)
				if saveErr != nil {
					oneClickStatus.SetText("已填入本地 FunASR（SenseVoiceSmall），保存失败: " + saveErr.Error() + " · " + line)
					return
				}
				oneClickStatus.SetText("已保存本地 FunASR（SenseVoiceSmall）· " + line)
			})
		}()
	})
	oneClick.Importance = widget.HighImportance
	localBox.Objects = []fyne.CanvasObject{
		settingsField("SenseVoice 地址", asrURL),
		settingsField("热词", hotword),
		spkCheck,
		oneClick,
		oneClickStatus,
	}

	showASR = func(selected string) {
		var fields fyne.CanvasObject
		switch selected {
		case asr.LabelLocalFunASR:
			asrURL.SetText(asr.EnsureFunASRURL(asrURL.Text))
			state.Cfg.ASR.Backend = asr.BackendLocal
			state.Cfg.ASR.APIURL = asrURL.Text
			asrHint.SetText(fmt.Sprintf("中文本地字幕使用 FunASR-Pack 的 SenseVoiceSmall（模型 %s）。请先启动整合包：%s", asr.SenseVoiceModel, asr.FunASRPackDownloadURL))
			fields = localBox
		case asrChoiceFireRed:
			fireredURL.SetText(asr.EnsureFireRedURL(fireredURL.Text))
			state.Cfg.ASR.Backend = asr.BackendFireRed
			state.Cfg.ASR.FireRedAPIURL = fireredURL.Text
			asrHint.SetText("请先启动 FireRedASR 整合包，地址可手动修改。")
			fields = fireredBox
		default:
			state.Cfg.ASR.Backend = asr.BackendBailian
			asrHint.SetText("在线识别。Key：https://bailian.console.aliyun.com")
			fields = bailianBox
		}
		asrBody.Objects = []fyne.CanvasObject{fields, asrHint}
		asrBody.Refresh()
	}
	backend.OnChanged = func(selected string) {
		showASR(selected)
		status.SetText("识别方式已切换，点「保存配置」后下次启动仍然有效")
	}
	showASR(backend.Selected)

	saveBtn := widget.NewButton("保存配置", func() {
		state.Cfg.LLM.BaseURL = strings.TrimSpace(baseURL.Text)
		state.Cfg.LLM.APIKey = apiKey.Text
		state.Cfg.LLM.Model = strings.TrimSpace(model.Text)
		state.Cfg.FFmpeg.Path = strings.TrimSpace(ffmpegPath.Text)
		state.Cfg.FFmpeg.FFprobePath = strings.TrimSpace(ffprobePath.Text)
		state.Cfg.ASR.APIURL = strings.TrimSpace(asrURL.Text)
		state.Cfg.ASR.FireRedAPIURL = strings.TrimSpace(fireredURL.Text)
		switch backend.Selected {
		case asrChoiceFireRed:
			state.Cfg.ASR.Backend = asr.BackendFireRed
			state.Cfg.ASR.FireRedAPIURL = asr.EnsureFireRedURL(fireredURL.Text)
			fireredURL.SetText(state.Cfg.ASR.FireRedAPIURL)
		case asr.LabelLocalFunASR:
			state.Cfg.ASR.Backend = asr.BackendLocal
			state.Cfg.ASR.APIURL = asr.EnsureFunASRURL(asrURL.Text)
			asrURL.SetText(state.Cfg.ASR.APIURL)
		default:
			state.Cfg.ASR.Backend = asr.BackendBailian
		}
		state.Cfg.ASR.APIKey = strings.TrimSpace(bailianKey.Text)
		state.Cfg.ASR.Hotword = strings.TrimSpace(hotword.Text)
		state.Cfg.ASR.EnableSpk = spkCheck.Checked
		state.Cfg.UI.SubtitleHideOnOriginal = hideCheck.Checked
		state.Cfg.Replicate.BaseURL = strings.TrimSpace(repBaseURL.Text)
		state.Cfg.Replicate.APIKey = strings.TrimSpace(repAPIKey.Text)
		state.Cfg.Replicate.Model = strings.TrimSpace(repModel.Text)
		state.Cfg.Douyin.BaseURL = strings.TrimSpace(dyBaseURL.Text)
		state.Cfg.Douyin.APIKey = strings.TrimSpace(dyAPIKey.Text)
		state.Cfg.Video.APIKey = strings.TrimSpace(videoKey.Text)
		state.Cfg.Image.BaseURL = strings.TrimSpace(imgBaseURL.Text)
		state.Cfg.Image.APIKey = strings.TrimSpace(imgAPIKey.Text)
		state.Cfg.Image.Model = strings.TrimSpace(imgModel.Text)
		linkdl.ConfigureDouyin(state.Cfg.Douyin.BaseURL, state.Cfg.Douyin.APIKey)
		if err := config.Save(state.CfgPath, state.Cfg); err != nil {
			status.SetText(fmt.Sprintf("保存失败: %v", err))
			return
		}
		status.SetText("已保存 · " + state.CfgPath)
	})
	saveBtn.Importance = widget.HighImportance

	workbenchURL, _ := url.Parse("https://workbench.ohmybb.xyz/")
	docsURL, _ := url.Parse("https://workbench.ohmybb.xyz/docs/api")
	videoLinks := container.NewHBox(
		widget.NewHyperlink("打开工作台", workbenchURL),
		widget.NewHyperlink("接口文档", docsURL),
	)
	videoHint := widget.NewLabel("介绍片使用 AI 视频工作台的密钥，额度与网页端共用。")
	videoHint.Wrapping = fyne.TextWrapWord
	videoHint.Importance = widget.LowImportance
	imageHint := widget.NewLabel("四视图走这里的地址和模型，与上方「视觉模型」分开。地址或密钥留空时，沿用复刻 Base URL 和复刻 API Key。")
	imageHint.Wrapping = fyne.TextWrapWord
	imageHint.Importance = widget.LowImportance

	cfgPath := widget.NewLabel("配置文件：" + state.CfgPath)
	cfgPath.Wrapping = fyne.TextWrapWord
	cfgPath.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		widget.NewCard("大模型", "", container.NewVBox(
			settingsField("Base URL", baseURL),
			container.NewGridWithColumns(2,
				settingsField("API Key", apiKey),
				settingsField("Model", model),
			),
		)),
		widget.NewCard("ffmpeg", "留空则使用系统 PATH", container.NewGridWithColumns(2,
			settingsField("ffmpeg 路径", ffmpegPath),
			settingsField("ffprobe 路径", ffprobePath),
		)),
		widget.NewCard("字幕识别", "选择本地 FunASR 后自动填入 SenseVoiceSmall 地址，仍可手改", container.NewVBox(
			settingsField("识别方式", backend),
			asrBody,
		)),
		widget.NewCard("爆款复刻", "默认 ModelScope Qwen3-VL", container.NewVBox(
			settingsField("Base URL", repBaseURL),
			container.NewGridWithColumns(2,
				settingsField("API Key", repAPIKey),
				settingsField("视觉模型", repModel),
			),
		)),
		widget.NewCard("链接下载", "抖音解析地址和 Key，可换成自己部署的实例", container.NewGridWithColumns(2,
			settingsField("API 地址", dyBaseURL),
			settingsField("API Key", dyAPIKey),
		)),
		widget.NewCard("介绍片", "视频工作台密钥", container.NewVBox(
			settingsField("视频 API Key", videoKey),
			videoLinks,
			videoHint,
		)),
		widget.NewCard("生图", "四视图使用，与视觉模型分开", container.NewVBox(
			settingsField("Base URL", imgBaseURL),
			container.NewGridWithColumns(2,
				settingsField("API Key", imgAPIKey),
				settingsField("模型", imgModel),
			),
			imageHint,
		)),
		widget.NewCard("界面", "", hideCheck),
		container.NewHBox(saveBtn),
		status,
		cfgPath,
	)))
}

func settingsField(label string, obj fyne.CanvasObject) fyne.CanvasObject {
	name := widget.NewLabel(label)
	return container.NewVBox(name, obj)
}

func saveProject(state *AppState) error {
	if state.Project == nil {
		return fmt.Errorf("no project")
	}
	return project.Save(state.ProjectsRoot, state.Project)
}
