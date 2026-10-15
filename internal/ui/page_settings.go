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

func newSettingsPage(state *AppState) fyne.CanvasObject {
	baseURL := widget.NewEntry()
	baseURL.SetText(state.Cfg.LLM.BaseURL)
	apiKey := widget.NewPasswordEntry()
	apiKey.SetText(state.Cfg.LLM.APIKey)
	model := widget.NewEntry()
	model.SetText(state.Cfg.LLM.Model)

	ffmpegPath := widget.NewEntry()
	ffmpegPath.SetText(state.Cfg.FFmpeg.Path)
	ffprobePath := widget.NewEntry()
	ffprobePath.SetText(state.Cfg.FFmpeg.FFprobePath)

	backend := widget.NewSelect([]string{
		"阿里百炼 Fun-ASR（在线）",
		asr.LabelLocalFunASR,
		"本地 FireRedASR",
	}, nil)
	switch asr.NormalizeBackend(state.Cfg.ASR.Backend) {
	case asr.BackendFireRed:
		backend.SetSelected("本地 FireRedASR")
	case asr.BackendLocal:
		backend.SetSelected(asr.LabelLocalFunASR)
	default:
		backend.SetSelected("阿里百炼 Fun-ASR（在线）")
	}

	asrURL := widget.NewEntry()
	asrURL.SetText(state.Cfg.ASR.APIURL)
	if asrURL.Text == "" {
		asrURL.SetText(asr.DefaultAPIURL)
	}
	fireredURL := widget.NewEntry()
	fireredURL.SetText(state.Cfg.ASR.FireRedAPIURL)
	if fireredURL.Text == "" {
		fireredURL.SetText(asr.DefaultFireRedAPIURL)
	}
	bailianKey := widget.NewPasswordEntry()
	bailianKey.SetText(state.Cfg.ASR.APIKey)
	hotword := widget.NewEntry()
	hotword.SetText(state.Cfg.ASR.Hotword)
	spkCheck := widget.NewCheck("本地 SenseVoice 说话人分离", nil)
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

	saveBtn := widget.NewButton("保存配置", func() {
		state.Cfg.LLM.BaseURL = baseURL.Text
		state.Cfg.LLM.APIKey = apiKey.Text
		state.Cfg.LLM.Model = model.Text
		state.Cfg.FFmpeg.Path = ffmpegPath.Text
		state.Cfg.FFmpeg.FFprobePath = ffprobePath.Text
		switch backend.Selected {
		case "本地 FireRedASR":
			state.Cfg.ASR.Backend = asr.BackendFireRed
		case asr.LabelLocalFunASR:
			state.Cfg.ASR.Backend = asr.BackendLocal
		default:
			state.Cfg.ASR.Backend = asr.BackendBailian
		}
		state.Cfg.ASR.APIURL = strings.TrimSpace(asrURL.Text)
		state.Cfg.ASR.FireRedAPIURL = strings.TrimSpace(fireredURL.Text)
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

	oneClick := widget.NewButton("一键配置本地 FunASR", func() {
		backendName, apiURL := asr.LocalFunASRPreset()
		backend.SetSelected(asr.LabelLocalFunASR)
		asrURL.SetText(apiURL)
		state.Cfg.ASR.Backend = backendName
		state.Cfg.ASR.APIURL = apiURL
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

	baseForm := widget.NewForm(
		widget.NewFormItem("LLM Base URL", baseURL),
		widget.NewFormItem("API Key", apiKey),
		widget.NewFormItem("Model", model),
		widget.NewFormItem("ffmpeg 路径", ffmpegPath),
		widget.NewFormItem("ffprobe 路径", ffprobePath),
	)
	asrForm := widget.NewForm(
		widget.NewFormItem("字幕识别方式", backend),
		widget.NewFormItem("百炼 API Key", bailianKey),
		widget.NewFormItem("本地 SenseVoice 地址", asrURL),
		widget.NewFormItem("本地 FireRed 地址", fireredURL),
		widget.NewFormItem("FunASR 热词", hotword),
	)
	tailForm := widget.NewForm(
		widget.NewFormItem("复刻 Base URL", repBaseURL),
		widget.NewFormItem("复刻 API Key", repAPIKey),
		widget.NewFormItem("复刻视觉模型", repModel),
		widget.NewFormItem("抖音 API 地址", dyBaseURL),
		widget.NewFormItem("抖音 API Key", dyAPIKey),
		widget.NewFormItem("视频 API Key", videoKey),
	)

	workbenchURL, _ := url.Parse("https://workbench.ohmybb.xyz/")
	docsURL, _ := url.Parse("https://workbench.ohmybb.xyz/docs/api")
	videoLinks := container.NewHBox(
		widget.NewHyperlink("打开工作台", workbenchURL),
		widget.NewHyperlink("接口文档", docsURL),
	)
	videoHint := widget.NewLabel("介绍片使用 AI 视频工作台的密钥，额度与网页端共用。")
	videoHint.Wrapping = fyne.TextWrapWord
	videoHint.Importance = widget.LowImportance

	imageForm := widget.NewForm(
		widget.NewFormItem("生图 Base URL", imgBaseURL),
		widget.NewFormItem("生图 API Key", imgAPIKey),
		widget.NewFormItem("生图模型", imgModel),
	)
	imageHint := widget.NewLabel("四视图走这里的地址和模型，与上方「复刻视觉模型」分开。地址或密钥留空时，沿用复刻 Base URL 和复刻 API Key。")
	imageHint.Wrapping = fyne.TextWrapWord
	imageHint.Importance = widget.LowImportance

	asrHint := widget.NewLabel(fmt.Sprintf(
		"中文本地字幕使用 FunASR-Pack 的 SenseVoiceSmall（HTTP 模型名 %s），不把模型打进本程序。点「一键配置本地 FunASR」会切换到本地、填入 %s 并保存；地址仍可手动修改后点「保存配置」。请先启动整合包：%s。在线识别选「阿里百炼」并填写 Key：https://bailian.console.aliyun.com",
		asr.SenseVoiceModel, asr.DefaultAPIURL, asr.FunASRPackDownloadURL,
	))
	asrHint.Wrapping = fyne.TextWrapWord
	asrHint.Importance = widget.LowImportance

	dyHint := widget.NewLabel("链接下载的抖音解析使用自建 Douyin_TikTok_Download_API，已预填默认地址和 Key，也可替换为自己部署的实例。")
	dyHint.Wrapping = fyne.TextWrapWord
	dyHint.Importance = widget.LowImportance

	repHint := widget.NewLabel("爆款复刻使用 OpenAI 兼容的视觉模型（与 Python 版 QWEN_* 配置一致），默认 ModelScope Qwen3-VL。Key：https://modelscope.cn")
	repHint.Wrapping = fyne.TextWrapWord
	repHint.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		widget.NewCard("设置", "配置文件: "+state.CfgPath, container.NewVBox(
			baseForm,
			asrForm,
			oneClick,
			oneClickStatus,
			asrHint,
			spkCheck,
			tailForm,
			videoLinks,
			videoHint,
			hideCheck,
			dyHint,
			repHint,
			imageForm,
			imageHint,
			saveBtn,
			status,
		)),
	)))
}

func saveProject(state *AppState) error {
	if state.Project == nil {
		return fmt.Errorf("no project")
	}
	return project.Save(state.ProjectsRoot, state.Project)
}
