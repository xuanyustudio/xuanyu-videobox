package asr

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Backends aligned with Python webui fun_asr options.
const (
	BackendLocal   = "local"   // FunASR-Pack
	BackendFireRed = "firered" // FireRedASR2-AED-Pack
	BackendBailian = "bailian" // 阿里百炼在线 Fun-ASR
)

// Options selects a transcription backend.
type Options struct {
	Backend       string
	APIURL        string // local FunASR
	FireRedAPIURL string
	BailianAPIKey string
	Hotword       string
	EnableSpk     bool
}

// NormalizeBackend returns a known backend id.
func NormalizeBackend(b string) string {
	switch strings.ToLower(strings.TrimSpace(b)) {
	case BackendFireRed, "fire", "fireredasr":
		return BackendFireRed
	case BackendBailian, "ali", "dashscope", "online":
		return BackendBailian
	case BackendLocal, "funasr", "":
		return BackendLocal
	default:
		return BackendLocal
	}
}

// Transcribe routes to the selected backend and writes outPath.
func Transcribe(mediaPath, outPath string, opt Options) error {
	switch NormalizeBackend(opt.Backend) {
	case BackendBailian:
		c := &BailianClient{APIKey: opt.BailianAPIKey}
		return c.TranscribeFile(mediaPath, outPath)
	case BackendFireRed:
		url := opt.FireRedAPIURL
		if strings.TrimSpace(url) == "" {
			url = DefaultFireRedAPIURL
		}
		c := &Client{APIURL: url}
		return c.TranscribeFireRed(mediaPath, outPath)
	default:
		url := opt.APIURL
		if strings.TrimSpace(url) == "" {
			url = DefaultAPIURL
		}
		c := &Client{APIURL: url, Hotword: opt.Hotword, EnableSpk: opt.EnableSpk}
		return c.TranscribeFile(mediaPath, outPath)
	}
}

// OutPathFor builds a subtitle path under projectDir/subtitles.
func OutPathFor(projectDir, mediaPath, backend string) string {
	stem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	suffix := "fun_asr"
	switch NormalizeBackend(backend) {
	case BackendFireRed:
		suffix = "firered_asr"
	case BackendBailian:
		suffix = "fun_asr"
	}
	return filepath.Join(projectDir, "subtitles", stem+"_"+suffix+".srt")
}

// BackendLabel returns a short Chinese label for status UI.
func BackendLabel(backend string) string {
	switch NormalizeBackend(backend) {
	case BackendBailian:
		return "阿里百炼 Fun-ASR"
	case BackendFireRed:
		return "本地 FireRedASR"
	default:
		return "本地 FunASR-Pack"
	}
}

// RequireReady validates options before starting a batch.
func RequireReady(opt Options) error {
	switch NormalizeBackend(opt.Backend) {
	case BackendBailian:
		if strings.TrimSpace(opt.BailianAPIKey) == "" {
			return fmt.Errorf("未配置百炼 API Key，请在「设置」填写（可到 https://bailian.console.aliyun.com 创建）")
		}
	}
	return nil
}
