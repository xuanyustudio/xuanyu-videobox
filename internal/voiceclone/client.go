package voiceclone

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.minimax.cn"
	DefaultModel   = "speech-2.8-hd"
	defaultTimeout = 3 * time.Minute
)

// Client talks to the MiniMax voice clone and T2A APIs.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// CloneResult is the outcome of a voice clone request.
type CloneResult struct {
	VoiceID   string
	FileID    string
	DemoAudio string
}

// SynthesizeResult is the outcome of a T2A request.
type SynthesizeResult struct {
	Audio      []byte
	Format     string
	LengthMS   int64
	Characters int64
}

func (c *Client) baseURL() string {
	return NormalizeBaseURL(c.BaseURL)
}

func (c *Client) model() string {
	m := strings.TrimSpace(c.Model)
	if m == "" {
		return DefaultModel
	}
	return m
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: defaultTimeout}
}

// NormalizeBaseURL trims a MiniMax API root and falls back to the China default.
func NormalizeBaseURL(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return DefaultBaseURL
	}
	return s
}

// UploadVoiceClone uploads a reference audio and returns its file_id.
func (c *Client) UploadVoiceClone(ctx context.Context, audioPath string) (string, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return "", fmt.Errorf("请先在「设置」里填写 MiniMax API Key")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("purpose", "voice_clone"); err != nil {
		return "", err
	}
	fw, err := w.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	f, err := os.Open(audioPath)
	if err != nil {
		return "", fmt.Errorf("打开参考音失败: %w", err)
	}
	_, copyErr := io.Copy(fw, f)
	closeErr := f.Close()
	if copyErr != nil {
		_ = w.Close()
		return "", copyErr
	}
	if closeErr != nil {
		_ = w.Close()
		return "", closeErr
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/v1/files/upload", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("上传参考音失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := checkHTTP(resp.StatusCode, body); err != nil {
		return "", fmt.Errorf("上传参考音失败: %w", err)
	}
	var parsed struct {
		File struct {
			FileID json.Number `json:"file_id"`
		} `json:"file"`
		BaseResp baseResp `json:"base_resp"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("上传参考音响应无法解析: %w", err)
	}
	if err := parsed.BaseResp.err(); err != nil {
		return "", err
	}
	id := parsed.File.FileID.String()
	if id == "" {
		return "", fmt.Errorf("上传参考音成功，但没有返回 file_id")
	}
	return id, nil
}

// CloneOptions are optional fields for a clone request.
type CloneOptions struct {
	Text               string
	LanguageBoost      string
	NeedNoiseReduction bool
}

// CloneVoice creates a custom voice_id from an uploaded file_id.
func (c *Client) CloneVoice(ctx context.Context, fileID, voiceID string, opt CloneOptions) (*CloneResult, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("请先在「设置」里填写 MiniMax API Key")
	}
	voiceID = NormalizeVoiceID(voiceID)
	if err := ValidateVoiceID(voiceID); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"file_id":                   mustJSONNumber(fileID),
		"voice_id":                  voiceID,
		"need_noise_reduction":      opt.NeedNoiseReduction,
		"need_volume_normalization": true,
	}
	text := strings.TrimSpace(opt.Text)
	if text != "" {
		if utf8Len(text) > 1000 {
			return nil, fmt.Errorf("试听文案不能超过 1000 字")
		}
		payload["text"] = text
		payload["model"] = c.model()
	}
	if boost := strings.TrimSpace(opt.LanguageBoost); boost != "" {
		payload["language_boost"] = boost
	} else {
		payload["language_boost"] = "auto"
	}

	raw, err := c.postJSON(ctx, "/v1/voice_clone", payload)
	if err != nil {
		return nil, fmt.Errorf("音色复刻失败: %w", err)
	}
	var parsed struct {
		DemoAudio string   `json:"demo_audio"`
		BaseResp  baseResp `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("音色复刻响应无法解析: %w", err)
	}
	if err := parsed.BaseResp.err(); err != nil {
		return nil, err
	}
	return &CloneResult{VoiceID: voiceID, FileID: fileID, DemoAudio: parsed.DemoAudio}, nil
}

// Synthesize generates speech with a cloned (or system) voice_id.
func (c *Client) Synthesize(ctx context.Context, voiceID, text string) (*SynthesizeResult, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("请先在「设置」里填写 MiniMax API Key")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("请填写要合成的文案")
	}
	if utf8Len(text) > 10000 {
		return nil, fmt.Errorf("合成文案不能超过 10000 字")
	}
	voiceID = strings.TrimSpace(voiceID)
	if voiceID == "" {
		return nil, fmt.Errorf("请先克隆音色，或填写已有的音色 ID")
	}
	payload := map[string]any{
		"model":          c.model(),
		"text":           text,
		"stream":         false,
		"language_boost": "auto",
		"voice_setting": map[string]any{
			"voice_id": voiceID,
			"speed":    1,
			"vol":      1,
			"pitch":    0,
		},
		"audio_setting": map[string]any{
			"sample_rate": 32000,
			"bitrate":     128000,
			"format":      "mp3",
			"channel":     1,
		},
	}
	raw, err := c.postJSON(ctx, "v1/t2a_v2", payload)
	if err != nil {
		return nil, fmt.Errorf("语音合成失败: %w", err)
	}
	var parsed struct {
		Data struct {
			Audio  string `json:"audio"`
			Status int    `json:"status"`
		} `json:"data"`
		ExtraInfo struct {
			AudioLength     int64  `json:"audio_length"`
			UsageCharacters int64  `json:"usage_characters"`
			AudioFormat     string `json:"audio_format"`
		} `json:"extra_info"`
		BaseResp baseResp `json:"base_resp"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("语音合成响应无法解析: %w", err)
	}
	if err := parsed.BaseResp.err(); err != nil {
		return nil, err
	}
	audioHex := strings.TrimSpace(parsed.Data.Audio)
	if audioHex == "" {
		return nil, fmt.Errorf("语音合成成功，但没有返回音频数据")
	}
	audio, err := hex.DecodeString(audioHex)
	if err != nil {
		return nil, fmt.Errorf("语音合成音频无法解码: %w", err)
	}
	format := strings.TrimSpace(parsed.ExtraInfo.AudioFormat)
	if format == "" {
		format = "mp3"
	}
	return &SynthesizeResult{
		Audio:      audio,
		Format:     format,
		LengthMS:   parsed.ExtraInfo.AudioLength,
		Characters: parsed.ExtraInfo.UsageCharacters,
	}, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err := checkHTTP(resp.StatusCode, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// DownloadDemo saves a preview audio URL to dest.
func DownloadDemo(ctx context.Context, httpc *http.Client, url, dest string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("没有试听音频地址")
	}
	if httpc == nil {
		httpc = &http.Client{Timeout: defaultTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return fmt.Errorf("下载试听音频失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载试听音频失败（HTTP %d）", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, io.LimitReader(resp.Body, 40<<20))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

type baseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

func (r baseResp) err() error {
	if r.StatusCode == 0 {
		return nil
	}
	msg := strings.TrimSpace(r.StatusMsg)
	if msg == "" {
		msg = "未知错误"
	}
	if mapped := mapStatus(r.StatusCode, msg); mapped != "" {
		return fmt.Errorf("%s", mapped)
	}
	return fmt.Errorf("MiniMax 返回错误 %d: %s", r.StatusCode, msg)
}

func mapStatus(code int, msg string) string {
	switch code {
	case 1004:
		return "MiniMax 鉴权失败，请检查「设置」里的 API Key 是否正确"
	case 1002:
		return "MiniMax 请求过于频繁，请稍后再试"
	case 1001:
		return "MiniMax 请求超时，请稍后重试"
	case 2013:
		return "参考音格式不符合要求：需为 10 秒到 5 分钟的 mp3、m4a 或 wav，且不超过 20 MB"
	case 2038:
		return "当前账号还没有音色复刻权限，请先在 MiniMax 开放平台完成个人或企业认证"
	case 1043:
		return "参考音内容校验未通过：" + msg
	default:
		return ""
	}
}

func checkHTTP(code int, body []byte) error {
	if code >= 200 && code < 300 {
		return nil
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	if msg == "" {
		return fmt.Errorf("HTTP %d", code)
	}
	// Some MiniMax errors still put details in base_resp with a non-2xx status.
	var parsed struct {
		BaseResp baseResp `json:"base_resp"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.BaseResp.StatusCode != 0 {
		return parsed.BaseResp.err()
	}
	return fmt.Errorf("HTTP %d: %s", code, msg)
}

func mustJSONNumber(id string) any {
	id = strings.TrimSpace(id)
	var n json.Number
	if err := json.Unmarshal([]byte(id), &n); err == nil {
		return n
	}
	return id
}

func utf8Len(s string) int {
	return len([]rune(s))
}
