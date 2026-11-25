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
	Video     Video     `yaml:"video" json:"video"`
	Image     ImageGen  `yaml:"image" json:"image"`
	MiniMax   MiniMax   `yaml:"minimax" json:"minimax"`
	Sonilo    Sonilo    `yaml:"sonilo" json:"sonilo"`
}

type LLM struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type TTS struct {
	Voice              string  `yaml:"voice" json:"voice"`
	Engine              string  `yaml:"engine" json:"engine"` // edge | minimax | kokoro | openvoice | indextts2
	Speed               float64 `yaml:"speed" json:"speed"`   // 0.5–2.0, default 1
	Volume              float64 `yaml:"volume" json:"volume"` // synthesis loudness 0.1–2.0, default 1
	Pitch               float64 `yaml:"pitch" json:"pitch"`   // -12–12, default 0
	KokoroURL           string  `yaml:"kokoro_url" json:"kokoro_url"`
	OpenVoiceURL        string  `yaml:"openvoice_url" json:"openvoice_url"`
	OpenVoiceReference  string  `yaml:"openvoice_reference" json:"openvoice_reference"`
	IndexTTS2URL        string  `yaml:"indextts2_url" json:"indextts2_url"`
	IndexTTS2Reference  string  `yaml:"indextts2_reference" json:"indextts2_reference"`
}

type FFmpeg struct {
	Path        string `yaml:"path"`
	FFprobePath string `yaml:"ffprobe_path"`
}

// ASR holds subtitle transcription settings (aligned with Python fun_asr).
type ASR struct {
	Backend       string `yaml:"backend"`         // local | firered | bailian
	APIURL        string `yaml:"api_url"`         // local FunASR-Pack SenseVoiceSmall
	FireRedAPIURL string `yaml:"firered_api_url"` // local FireRed
	APIKey        string `yaml:"api_key"`         // 阿里百炼
	Hotword       string `yaml:"hotword"`
	EnableSpk     bool   `yaml:"enable_spk"`
}

type UI struct {
	TargetDurationSec      int     `yaml:"target_duration_sec" json:"target_duration_sec"`
	SubtitleHideOnOriginal bool    `yaml:"subtitle_hide_on_original" json:"subtitle_hide_on_original"`
	SubtitleMaskEnabled    bool    `yaml:"subtitle_mask_enabled" json:"subtitle_mask_enabled"`
	SubtitleBurnEnabled    bool    `yaml:"subtitle_burn_enabled" json:"subtitle_burn_enabled"`
	SubtitleFont           string  `yaml:"subtitle_font" json:"subtitle_font"`
	SubtitleFontSize       int     `yaml:"subtitle_font_size" json:"subtitle_font_size"`
	SubtitleColor          string  `yaml:"subtitle_color" json:"subtitle_color"`
	SubtitlePosition       string  `yaml:"subtitle_position" json:"subtitle_position"` // bottom | top | center
	SubtitleOnMask         bool    `yaml:"subtitle_on_mask" json:"subtitle_on_mask"`   // burn narration on the mask; default keeps subtitle_position
	SubtitleMaskX          float64 `yaml:"subtitle_mask_x" json:"subtitle_mask_x"`
	SubtitleMaskY          float64 `yaml:"subtitle_mask_y" json:"subtitle_mask_y"`
	SubtitleMaskWidth      float64 `yaml:"subtitle_mask_width" json:"subtitle_mask_width"`
	SubtitleMaskHeight     float64 `yaml:"subtitle_mask_height" json:"subtitle_mask_height"`
	SubtitleMaskBlur       int     `yaml:"subtitle_mask_blur" json:"subtitle_mask_blur"`
	SubtitleMaskOpacity    int     `yaml:"subtitle_mask_opacity" json:"subtitle_mask_opacity"`
	AspectRatio            string  `yaml:"aspect_ratio" json:"aspect_ratio"` // source | vertical | horizontal
	Resolution             string  `yaml:"resolution" json:"resolution"`     // 480p–2160p
	VoiceVolume            float64 `yaml:"voice_volume" json:"voice_volume"`
	OriginalVolume         float64 `yaml:"original_volume" json:"original_volume"`
	BGMSource              string  `yaml:"bgm_source" json:"bgm_source"` // off | library | upload | sonilo
	BGMPath                string  `yaml:"bgm_path" json:"bgm_path"`
	BGMVolume              float64 `yaml:"bgm_volume" json:"bgm_volume"`
	BGMLibraryDir          string  `yaml:"bgm_library_dir" json:"bgm_library_dir"`
	SFXEnabled             bool    `yaml:"sfx_enabled" json:"sfx_enabled"`
	SFXPrompt              string  `yaml:"sfx_prompt" json:"sfx_prompt"`
	SFXVolume              float64 `yaml:"sfx_volume" json:"sfx_volume"`
}

// Replicate holds 提示词提取 / 视频逆向 settings: an OpenAI-compatible vision model
// (default ModelScope Qwen3-VL, aligned with Python QWEN_API_BASE_URL / QWEN_API_KEY / QWEN_MODEL_ID).
type Replicate struct {
	BaseURL    string `yaml:"base_url" json:"base_url"`
	APIKey     string `yaml:"api_key" json:"api_key"`
	Model      string `yaml:"model" json:"model"`
	Frames     int    `yaml:"frames" json:"frames"`           // keyframes sampled per video
	SegmentSec int    `yaml:"segment_sec" json:"segment_sec"` // 10 or 15
}

// Douyin holds the self-hosted Douyin_TikTok_Download_API connection info.
type Douyin struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
}

// Video holds the AI 视频工作台 key used by 口播视频.
type Video struct {
	APIKey string `yaml:"api_key" json:"api_key"`
}

// ImageGen holds the OpenAI-compatible image edit endpoint used for four-view sheets.
// APIKey may be empty; the intro page then falls back to the replicate key.
type ImageGen struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
	Model   string `yaml:"model" json:"model"`
}

// MiniMax holds the 克隆音频 settings for MiniMax voice cloning / T2A.
// BaseURL and Model may be empty; the client then uses the China defaults.
type MiniMax struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
	Model   string `yaml:"model" json:"model"`
}

// Sonilo holds the optional AI BGM / SFX key. Empty key leaves the feature off.
type Sonilo struct {
	BaseURL   string `yaml:"base_url" json:"base_url"`
	APIKey    string `yaml:"api_key" json:"api_key"`
	BGMPrompt string `yaml:"bgm_prompt" json:"bgm_prompt"`
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
			Voice:        "zh-CN-XiaoxiaoNeural",
			Engine:       "edge",
			Speed:        1,
			Volume:       1,
			KokoroURL:     "http://127.0.0.1:3211",
			OpenVoiceURL:  "http://127.0.0.1:3212",
			IndexTTS2URL:  "http://127.0.0.1:3213",
		},
		ASR: ASR{
			Backend:       "bailian",
			APIURL:        "http://127.0.0.1:7860",
			FireRedAPIURL: "http://127.0.0.1:7867",
		},
		UI: UI{
			TargetDurationSec:      180,
			SubtitleHideOnOriginal: true,
			SubtitleBurnEnabled:    true,
			SubtitleFont:           "Microsoft YaHei",
			SubtitleFontSize:       15,
			SubtitleColor:          "#FFFFFF",
			SubtitlePosition:       "bottom",
			AspectRatio:            "vertical",
			Resolution:             "1080p",
			VoiceVolume:            1.0,
			OriginalVolume:         1.2,
			BGMSource:              "off",
			BGMVolume:              0.3,
			SFXVolume:              0.6,
		},
		Paths: Paths{
			ProjectsRoot: defaultProjectsRoot(),
		},
		Replicate: Replicate{
			BaseURL:    "https://api-inference.modelscope.cn/v1",
			Model:      "Qwen/Qwen3-VL-8B-Instruct",
			Frames:     8,
			SegmentSec: 15,
		},
		Douyin: Douyin{},
		Image: ImageGen{
			Model: "gpt-image-1",
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
