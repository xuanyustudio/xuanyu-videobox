package intro

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultVideoBase = "https://workbench.ohmybb.xyz"
	VideoModel       = "wb-seedance-2-fast"
	VideoDuration    = 15
	VideoRatio       = "9:16"

	defaultPollEvery = 12 * time.Second
	videoWait        = 20 * time.Minute
)

// IntroOptions generates one intro clip from a reference image and copy.
type IntroOptions struct {
	ImagePath  string
	Prompt     string
	OutPath    string
	APIKey     string
	BaseURL    string
	Model      string
	Duration   int
	Ratio      string
	HTTP       *http.Client
	PollEvery  time.Duration
	OnProgress func(percent int, msg string)
}

type videoTask struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"`
	Progress int       `json:"progress"`
	Error    flexError `json:"error"`
	Warnings []string  `json:"warnings"`
}

// flexError accepts a string, an object, or null.
type flexError struct {
	text string
}

func (e *flexError) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		e.text = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		e.text = s
		return nil
	}
	var obj struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		e.text = string(b)
		return nil
	}
	if obj.Message != "" {
		e.text = obj.Message
		return nil
	}
	e.text = obj.Code
	return nil
}

// GenerateIntro submits an image-to-video task, polls, and saves the mp4.
func GenerateIntro(ctx context.Context, opt IntroOptions) (string, error) {
	if strings.TrimSpace(opt.APIKey) == "" {
		return "", fmt.Errorf("请先在「设置」中填写视频 API Key")
	}
	prompt := strings.TrimSpace(opt.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("请填写介绍文案")
	}
	if len([]rune(prompt)) > 4000 {
		return "", fmt.Errorf("介绍文案超过 4000 字")
	}
	if strings.TrimSpace(opt.ImagePath) == "" {
		return "", fmt.Errorf("请先选择图片")
	}
	dataURL, err := imageDataURL(opt.ImagePath)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(strings.TrimSpace(opt.BaseURL), "/")
	if base == "" {
		base = DefaultVideoBase
	}
	model := strings.TrimSpace(opt.Model)
	if model == "" {
		model = VideoModel
	}
	duration := opt.Duration
	if duration <= 0 {
		duration = VideoDuration
	}
	ratio := strings.TrimSpace(opt.Ratio)
	if ratio == "" {
		ratio = VideoRatio
	}
	outPath := opt.OutPath
	if strings.TrimSpace(outPath) == "" {
		outPath = filepath.Join(filepath.Dir(opt.ImagePath), "intro.mp4")
	}

	ctx, cancel := context.WithTimeout(ctx, videoWait)
	defer cancel()

	payload, err := json.Marshal(map[string]any{
		"prompt":   prompt,
		"model":    model,
		"duration": duration,
		"ratio":    ratio,
		"images":   []string{dataURL},
	})
	if err != nil {
		return "", err
	}
	progress(opt, 5, "正在提交介绍片…")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/videos", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opt.APIKey))

	client := videoHTTP(opt.HTTP)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("介绍片请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取介绍片响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", formatVideoHTTPError(resp.StatusCode, raw))
	}
	var created videoTask
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("介绍片响应无法解析: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("介绍片响应缺少任务 id")
	}
	if len(created.Warnings) > 0 {
		progress(opt, 8, strings.Join(created.Warnings, "；"))
	}

	every := opt.PollEvery
	if every <= 0 {
		every = defaultPollEvery
	}
	for {
		task, err := fetchVideo(ctx, client, base, opt.APIKey, created.ID)
		if err != nil {
			return "", err
		}
		switch task.Status {
		case "completed":
			progress(opt, 95, "正在下载成片…")
			if err := downloadVideo(ctx, client, base, opt.APIKey, created.ID, outPath); err != nil {
				return "", err
			}
			progress(opt, 100, "介绍片已保存")
			return outPath, nil
		case "failed":
			msg := task.Error.text
			if msg == "" {
				msg = "生成失败，额度已退还"
			}
			return "", fmt.Errorf("介绍片生成失败: %s", msg)
		default:
			p := task.Progress
			if p < 10 {
				p = 10
			}
			if p > 90 {
				p = 90
			}
			label := "排队中…"
			if task.Status == "in_progress" {
				label = "生成中…"
			}
			progress(opt, p, label)
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", fmt.Errorf("介绍片生成超时")
			}
			return "", ctx.Err()
		case <-time.After(every):
		}
	}
}

func fetchVideo(ctx context.Context, client *http.Client, base, apiKey, id string) (videoTask, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/videos/"+urlPathEscape(id), nil)
	if err != nil {
		return videoTask{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	resp, err := client.Do(req)
	if err != nil {
		return videoTask{}, fmt.Errorf("查询介绍片失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return videoTask{}, fmt.Errorf("查询介绍片失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return videoTask{}, fmt.Errorf("%s", formatVideoHTTPError(resp.StatusCode, raw))
	}
	var task videoTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return videoTask{}, fmt.Errorf("介绍片状态无法解析: %w", err)
	}
	return task, nil
}

func downloadVideo(ctx context.Context, client *http.Client, base, apiKey, id, outPath string) error {
	u := base + "/api/v1/videos/" + urlPathEscape(id) + "/content?compat=1&download=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载介绍片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s", formatVideoHTTPError(resp.StatusCode, raw))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(outPath)
		return fmt.Errorf("保存介绍片失败: %w", err)
	}
	if n == 0 {
		os.Remove(outPath)
		return fmt.Errorf("下载到的介绍片是空文件")
	}
	return nil
}

func imageDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	mime := "image/jpeg"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mime = "image/png"
	case ".webp":
		mime = "image/webp"
	case ".gif":
		mime = "image/gif"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func urlPathEscape(id string) string {
	return strings.ReplaceAll(id, "/", "")
}

func progress(opt IntroOptions, percent int, msg string) {
	if opt.OnProgress != nil {
		opt.OnProgress(percent, msg)
	}
}

func videoHTTP(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func formatVideoHTTPError(code int, body []byte) string {
	msg := apiMessage(body)
	switch code {
	case 401, 403:
		if msg == "" {
			msg = "密钥无效或账号未绑定"
		}
		return fmt.Sprintf("视频接口鉴权失败（HTTP %d）: %s", code, msg)
	case 429:
		if msg == "" {
			msg = "额度已用完"
		}
		return fmt.Sprintf("视频额度不足（HTTP %d）: %s", code, msg)
	default:
		if msg == "" {
			msg = truncate(string(body), 240)
		}
		return fmt.Sprintf("介绍片请求失败（HTTP %d）: %s", code, msg)
	}
}
