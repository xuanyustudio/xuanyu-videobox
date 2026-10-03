package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds local application settings persisted as YAML.
type Config struct {
	LLM    LLM    `yaml:"llm"`
	TTS    TTS    `yaml:"tts"`
	FFmpeg FFmpeg `yaml:"ffmpeg"`
	ASR    ASR    `yaml:"asr"`
	UI     UI     `yaml:"ui"`
	Paths  Paths  `yaml:"paths"`

	Replicate Replicate `yaml:"replicate" json:"replicate"`
	Douyin    Douyin    `yaml:"douyin" json:"douyin"`
}

type LLM struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type TTS struct {
	Voice string `yaml:"voice"`
}

type FFmpeg struct {
	Path        string `yaml:"path"`
	FFprobePath string `yaml:"ffprobe_path"`
}

// ASR holds subtitle transcription settings (aligned with Python fun_asr).
type ASR struct {
	Backend       string `yaml:"backend"`         // local | firered | bailian
	APIURL        string `yaml:"api_url"`         // local FunASR-Pack
	FireRedAPIURL string `yaml:"firered_api_url"` // local FireRed
	APIKey        string `yaml:"api_key"`         // 阿里百炼
	Hotword       string `yaml:"hotword"`
	EnableSpk     bool   `yaml:"enable_spk"`
}

type UI struct {
	TargetDurationSec      int  `yaml:"target_duration_sec"`
	SubtitleHideOnOriginal bool `yaml:"subtitle_hide_on_original"`
	SubtitleMaskEnabled    bool `yaml:"subtitle_mask_enabled"`
}

// Replicate holds 爆款复刻 settings: an OpenAI-compatible vision model
// (default ModelScope Qwen3-VL, aligned with Python QWEN_API_BASE_URL / QWEN_API_KEY / QWEN_MODEL_ID).
type Replicate struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
	Model   string `yaml:"model" json:"model"`
	Frames  int    `yaml:"frames" json:"frames"` // keyframes sampled per video
}

// Douyin holds the self-hosted Douyin_TikTok_Download_API connection info.
type Douyin struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
}

type Paths struct {
	ProjectsRoot string `yaml:"projects_root"`
}

// Default returns spec defaults for a new installation.
func Default() *Config {
	return &Config{
		LLM: LLM{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o-mini",
		},
		TTS: TTS{
			Voice: "zh-CN-XiaoxiaoNeural",
		},
		ASR: ASR{
			Backend:       "bailian",
			APIURL:        "http://127.0.0.1:7860",
			FireRedAPIURL: "http://127.0.0.1:7867",
		},
		UI: UI{
			TargetDurationSec:      180,
			SubtitleHideOnOriginal: true,
		},
		Paths: Paths{
			ProjectsRoot: defaultProjectsRoot(),
		},
		Replicate: Replicate{
			BaseURL: "https://api-inference.modelscope.cn/v1",
			Model:   "Qwen/Qwen3-VL-8B-Instruct",
			Frames:  8,
		},
		Douyin: Douyin{
			BaseURL: "http://39.109.101.66:8000",
			APIKey:  "dtk_0ebb3c9076ad_NVJtZ8xXbGvYWPi1GCkOuFx5hjGwFgZV",
		},
	}
}

func defaultProjectsRoot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Narrato", "projects")
	}
	return filepath.Join("..", "resource", "projects")
}

// Load reads config from path. A missing file returns Default() without error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes cfg to path as YAML, creating parent directories if needed.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
