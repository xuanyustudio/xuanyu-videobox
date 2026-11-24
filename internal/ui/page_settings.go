//go:build fyne_gui

package ui

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/replicate"
	"github.com/narratoai/narrato-go/internal/tts"
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
	dyBaseURL.SetPlaceHolder("例如 http://127.0.0.1:8000")
	dyAPIKey := widget.NewPasswordEntry()
	dyAPIKey.SetText(state.Cfg.Douyin.APIKey)

	videoKey := widget.NewPasswordEntry()
	videoKey.SetText(state.Cfg.Video.APIKey)
	videoKey.SetPlaceHolder("vw_ 开头的工作台密钥")

	imgBaseURL := widget.NewEntry()
	imgBaseURL.SetText(state.Cfg.Image.BaseURL)
	imgBaseURL.SetPlaceHolder("留空则使用视觉模型 Base URL")
	imgAPIKey := widget.NewPasswordEntry()
	imgAPIKey.SetText(state.Cfg.Image.APIKey)
	imgAPIKey.SetPlaceHolder("留空则使用视觉模型 API Key")
	imgModel := widget.NewEntry()
	imgModel.SetText(state.Cfg.Image.Model)
	if strings.TrimSpace(imgModel.Text) == "" {
		imgModel.SetText("gpt-image-1")
	}
	imgModel.SetPlaceHolder("gpt-image-2")

	mmBaseURL := widget.NewEntry()
	mmBaseURL.SetText(state.Cfg.MiniMax.BaseURL)
	mmBaseURL.SetPlaceHolder("https://api.minimax.cn")
	mmAPIKey := widget.NewPasswordEntry()
	mmAPIKey.SetText(state.Cfg.MiniMax.APIKey)
	mmAPIKey.SetPlaceHolder("在 MiniMax 开放平台申请的接口密钥")
	mmModel := widget.NewEntry()
	mmModel.SetText(state.Cfg.MiniMax.Model)
	mmModel.SetPlaceHolder("speech-2.8-hd")

	kokoroURL := widget.NewEntry()
	if v := strings.TrimSpace(state.Cfg.TTS.KokoroURL); v != "" {
		kokoroURL.SetText(v)
	} else {
		kokoroURL.SetText(tts.DefaultKokoroURL)
	}
	kokoroURL.SetPlaceHolder(tts.DefaultKokoroURL)
	openvoiceURL := widget.NewEntry()
	if v := strings.TrimSpace(state.Cfg.TTS.OpenVoiceURL); v != "" {
		openvoiceURL.SetText(v)
	} else {
		openvoiceURL.SetText(tts.DefaultOpenVoiceURL)
	}
	openvoiceURL.SetPlaceHolder(tts.DefaultOpenVoiceURL)
	ovRef := widget.NewEntry()
	ovRef.SetText(state.Cfg.TTS.OpenVoiceReference)
	ovRef.SetPlaceHolder("克隆音色用的参考 wav/mp3，可留空")
	indextts2URL := widget.NewEntry()
	if v := strings.TrimSpace(state.Cfg.TTS.IndexTTS2URL); v != "" {
		indextts2URL.SetText(v)
	} else {
		indextts2URL.SetText(tts.DefaultIndexTTS2URL)
	}
	indextts2URL.SetPlaceHolder(tts.DefaultIndexTTS2URL)
	ixRef := widget.NewEntry()
	ixRef.SetText(state.Cfg.TTS.IndexTTS2Reference)
	ixRef.SetPlaceHolder("仅克隆音色需要；选官方示例时不必填")

	snBaseURL := widget.NewEntry()
	snBaseURL.SetText(state.Cfg.Sonilo.BaseURL)
	snBaseURL.SetPlaceHolder("https://api.sonilo.com")
	snAPIKey := widget.NewPasswordEntry()
	snAPIKey.SetText(state.Cfg.Sonilo.APIKey)
	snAPIKey.SetPlaceHolder("可选：用于合成页的 AI 配乐与音效")

	hideCheck := widget.NewCheck("原片段隐藏解说字幕", func(bool) {})
	hideCheck.SetChecked(state.Cfg.UI.SubtitleHideOnOriginal)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	asrHint := widget.NewLabel("")
	asrHint.Wrapping = fyne.TextWrapWord
	asrHint.Importance = widget.LowImportance

	localBox := container.NewVBox()
	bailianBox := settingsForm(widget.NewLabel("百炼 API Key"), bailianKey)
	fireredBox := settingsForm(widget.NewLabel("FireRed 地址"), fireredURL)
	asrBody := container.NewVBox()

	localBox.Objects = []fyne.CanvasObject{
		settingsForm(
			widget.NewLabel("SenseVoice 地址"), asrURL,
			widget.NewLabel("热词"), hotword,
		),
		spkCheck,
	}

	showASR := func(selected string) {
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
		state.Cfg.MiniMax.BaseURL = strings.TrimSpace(mmBaseURL.Text)
		state.Cfg.MiniMax.APIKey = strings.TrimSpace(mmAPIKey.Text)
		state.Cfg.MiniMax.Model = strings.TrimSpace(mmModel.Text)
		if v := strings.TrimSpace(kokoroURL.Text); v != "" {
			state.Cfg.TTS.KokoroURL = v
		} else {
			state.Cfg.TTS.KokoroURL = tts.DefaultKokoroURL
		}
		if v := strings.TrimSpace(openvoiceURL.Text); v != "" {
			state.Cfg.TTS.OpenVoiceURL = v
		} else {
			state.Cfg.TTS.OpenVoiceURL = tts.DefaultOpenVoiceURL
		}
		state.Cfg.TTS.OpenVoiceReference = strings.TrimSpace(ovRef.Text)
		if v := strings.TrimSpace(indextts2URL.Text); v != "" {
			state.Cfg.TTS.IndexTTS2URL = v
		} else {
			state.Cfg.TTS.IndexTTS2URL = tts.DefaultIndexTTS2URL
		}
		state.Cfg.TTS.IndexTTS2Reference = strings.TrimSpace(ixRef.Text)
		state.Cfg.Sonilo.BaseURL = strings.TrimSpace(snBaseURL.Text)
		state.Cfg.Sonilo.APIKey = strings.TrimSpace(snAPIKey.Text)
		linkdl.ConfigureDouyin(state.Cfg.Douyin.BaseURL, state.Cfg.Douyin.APIKey)
		if err := config.Save(state.CfgPath, state.Cfg); err != nil {
			status.SetText(fmt.Sprintf("保存失败: %v", err))
			return
		}
		status.SetText("已保存: " + state.CfgPath)
	})
	saveBtn.Importance = widget.HighImportance

	workbenchURL, _ := url.Parse("https://workbench.ohmybb.xyz/")
	docsURL, _ := url.Parse("https://workbench.ohmybb.xyz/docs/api")
	videoLinks := container.NewHBox(
		widget.NewHyperlink("打开工作台", workbenchURL),
		widget.NewHyperlink("接口文档", docsURL),
	)
	mmKeyURL, _ := url.Parse("https://platform.minimax.cn/user-center/basic-information/interface-key")
	mmDocsURL, _ := url.Parse("https://platform.minimax.cn/docs/guides/speech-voice-clone")
	mmLinks := container.NewHBox(
		widget.NewHyperlink("申请密钥", mmKeyURL),
		widget.NewHyperlink("复刻说明", mmDocsURL),
	)

	cfgPath := widget.NewLabel("配置文件：" + state.CfgPath)
	cfgPath.Wrapping = fyne.TextWrapWord
	cfgPath.Importance = widget.LowImportance

	page := container.NewVBox(
		settingsCard("大模型", "", settingsForm(
			widget.NewLabel("Base URL"), baseURL,
			widget.NewLabel("API Key"), apiKey,
			widget.NewLabel("Model"), model,
		)),
		settingsCard("ffmpeg", "留空则使用系统 PATH", settingsForm(
			widget.NewLabel("ffmpeg 路径"), ffmpegPath,
			widget.NewLabel("ffprobe 路径"), ffprobePath,
		)),
		settingsCard("字幕识别", "选择本地 FunASR 后自动填入 SenseVoiceSmall 地址，仍可手改", container.NewVBox(
			settingsForm(widget.NewLabel("识别方式"), backend),
			asrBody,
		)),
		settingsCard("提示词提取", "与视频逆向共用视觉模型，默认 ModelScope Qwen3-VL", settingsForm(
			widget.NewLabel("Base URL"), repBaseURL,
			widget.NewLabel("API Key"), repAPIKey,
			widget.NewLabel("视觉模型"), repModel,
		)),
		settingsCard("链接下载", "抖音解析地址和 Key，可换成自己部署的实例", settingsForm(
			widget.NewLabel("API 地址"), dyBaseURL,
			widget.NewLabel("API Key"), dyAPIKey,
		)),
		settingsCard("口播视频", "额度与网页端共用", settingsForm(
			widget.NewLabel("视频 API Key"), container.NewBorder(nil, nil, nil, videoLinks, videoKey),
		)),
		settingsCard("克隆音频", "使用 MiniMax 音色复刻，国内默认 api.minimax.cn", settingsForm(
			widget.NewLabel("API 地址"), mmBaseURL,
			widget.NewLabel("API Key"), container.NewBorder(nil, nil, nil, mmLinks, mmAPIKey),
			widget.NewLabel("合成模型"), mmModel,
		)),
		settingsCard("本地配音", "请先双击运行对应启动脚本，再在合成页选择引擎。Kokoro：E:\\kokoro-tts\\启动Kokoro.bat；OpenVoice：E:\\OpenVoice\\启动配音服务.bat；IndexTTS-2：E:\\indextts2\\启动IndexTTS2.bat（不要占用 FunASR 的 7860 端口）", settingsForm(
			widget.NewLabel("Kokoro 地址"), kokoroURL,
			widget.NewLabel("OpenVoice 地址"), openvoiceURL,
			widget.NewLabel("OpenVoice 参考音频"), ovRef,
			widget.NewLabel("IndexTTS-2 地址"), indextts2URL,
			widget.NewLabel("IndexTTS-2 参考音频"), ixRef,
		)),
		settingsCard("Sonilo", "可选。用于成片合成的 AI 配乐与 AI 音效，不填写则只能使用曲库或本地 BGM", settingsForm(
			widget.NewLabel("API 地址"), snBaseURL,
			widget.NewLabel("API Key"), snAPIKey,
		)),
		settingsCard("生图", "四视图使用；地址或密钥留空时沿用视觉模型设置", settingsForm(
			widget.NewLabel("Base URL"), imgBaseURL,
			widget.NewLabel("API Key"), imgAPIKey,
			widget.NewLabel("模型"), imgModel,
		)),
		settingsCard("界面", "", hideCheck),
		container.NewHBox(saveBtn),
		status,
		cfgPath,
	)
	return container.NewVScroll(container.NewPadded(page))
}

func settingsForm(items ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewFormLayout(), items...)
}

// settingsCard keeps the subtitle on the title row and paints a theme surface.
func settingsCard(title, subtitle string, body fyne.CanvasObject) fyne.CanvasObject {
	titleLbl := sectionTitle(title)
	head := fyne.CanvasObject(titleLbl)
	if strings.TrimSpace(subtitle) != "" {
		sub := widget.NewLabel(subtitle)
		sub.Importance = widget.LowImportance
		sub.Truncation = fyne.TextTruncateEllipsis
		sub.SizeName = theme.SizeNameCaptionText
		head = container.NewBorder(nil, nil, titleLbl, nil, &looseWidth{CanvasObject: sub, minW: 24})
	}
	return newPanel(container.NewVBox(head, body))
}

// looseWidth reports a small minimum width so a one-line subtitle can sit
// after the title and ellipsize instead of stretching the page.
type looseWidth struct {
	fyne.CanvasObject
	minW float32
}

func (l *looseWidth) MinSize() fyne.Size {
	s := l.CanvasObject.MinSize()
	return fyne.NewSize(l.minW, s.Height)
}

func saveProject(state *AppState) error {
	if state.Project == nil {
		return fmt.Errorf("no project")
	}
	return project.Save(state.ProjectsRoot, state.Project)
}
