package intro

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ImageModel is the OpenAI image-edit model. Credentials and host come from
	// the replicate Base URL / API Key; the vision chat model cannot draw.
	ImageModel = "gpt-image-1"
	ImageSize  = "1536x1024"

	// FourViewPrompt asks for one character sheet: front, left, right, back.
	FourViewPrompt = "根据参考图中的角色，生成一张角色四视图设定图：同一角色从左到右依次为正面、左侧面、右侧面、背面，纯浅灰背景，全身，服装发型与参考图一致，不要文字、不要水印。"
)

// FourViewOptions calls an OpenAI-compatible POST /v1/images/edits endpoint.
type FourViewOptions struct {
	ImagePath string
	OutPath   string
	BaseURL   string
	APIKey    string
	Model     string
	HTTP      *http.Client
}

type imageEditResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ImagesEditsURL joins an OpenAI-compatible base URL with /images/edits.
func ImagesEditsURL(baseURL string) string {
	return openAIEndpoint(baseURL, "/images/edits")
}

func openAIEndpoint(baseURL, suffix string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u, err := url.Parse(base); err == nil && u.Scheme != "" && strings.Trim(u.Path, "/") == "" {
		base += "/v1"
	}
	return base + suffix
}

// GenerateFourView uploads the reference image and writes the edited sheet to OutPath.
func GenerateFourView(ctx context.Context, opt FourViewOptions) (string, error) {
	if strings.TrimSpace(opt.APIKey) == "" {
		return "", fmt.Errorf("请先在「设置」中填写复刻 API Key")
	}
	if strings.TrimSpace(opt.ImagePath) == "" {
		return "", fmt.Errorf("请先选择图片")
	}
	src, err := os.Open(opt.ImagePath)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	defer src.Close()

	model := strings.TrimSpace(opt.Model)
	if model == "" {
		model = ImageModel
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("model", model); err != nil {
		return "", err
	}
	if err := w.WriteField("prompt", FourViewPrompt); err != nil {
		return "", err
	}
	if err := w.WriteField("size", ImageSize); err != nil {
		return "", err
	}
	if err := w.WriteField("n", "1"); err != nil {
		return "", err
	}
	part, err := w.CreateFormFile("image[]", filepath.Base(opt.ImagePath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, src); err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ImagesEditsURL(opt.BaseURL), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opt.APIKey))

	resp, err := imageHTTP(opt.HTTP).Do(req)
	if err != nil {
		return "", fmt.Errorf("四视图请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取四视图响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", formatImageHTTPError(resp.StatusCode, raw))
	}
	var out imageEditResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("四视图响应无法解析: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("四视图生成失败: %s", out.Error.Message)
	}
	if len(out.Data) == 0 {
		return "", fmt.Errorf("四视图响应为空")
	}
	var img []byte
	switch {
	case strings.TrimSpace(out.Data[0].B64JSON) != "":
		img, err = base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
		if err != nil {
			return "", fmt.Errorf("四视图图片解码失败: %w", err)
		}
	case strings.TrimSpace(out.Data[0].URL) != "":
		img, err = downloadImage(ctx, imageHTTP(opt.HTTP), out.Data[0].URL)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("四视图响应没有图片")
	}
	if len(img) == 0 {
		return "", fmt.Errorf("四视图图片为空")
	}
	outPath := opt.OutPath
	if strings.TrimSpace(outPath) == "" {
		outPath = filepath.Join(filepath.Dir(opt.ImagePath), "fourview.png")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, img, 0o644); err != nil {
		return "", fmt.Errorf("保存四视图失败: %w", err)
	}
	return outPath, nil
}

func imageHTTP(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func downloadImage(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载四视图失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("下载四视图失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载四视图失败（HTTP %d）", resp.StatusCode)
	}
	return body, nil
}

func formatImageHTTPError(code int, body []byte) string {
	msg := apiMessage(body)
	switch code {
	case 401, 403:
		return fmt.Sprintf("四视图鉴权失败（HTTP %d），请检查设置里的「复刻 API Key」", code)
	case 404:
		return fmt.Sprintf("图像编辑接口不存在（HTTP %d），请确认「复刻 Base URL」支持 OpenAI /v1/images/edits", code)
	case 503:
		if strings.Contains(strings.ToLower(msg), "no available compatible accounts") {
			return "四视图没有可用的生图账号。复刻 Key 可以调用对话模型，但这把 Key 在网关上没有能处理图像编辑的上游账号。请在 lupoapi 为该密钥开通 gpt-image 生图，或换成已开通生图的 Key。"
		}
		if msg == "" {
			msg = truncate(string(body), 240)
		}
		return fmt.Sprintf("四视图请求失败（HTTP %d）: %s", code, msg)
	default:
		if msg == "" {
			msg = truncate(string(body), 240)
		}
		return fmt.Sprintf("四视图请求失败（HTTP %d）: %s", code, msg)
	}
}

func apiMessage(body []byte) string {
	var wrap struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &wrap) != nil {
		return ""
	}
	if wrap.Error != nil && wrap.Error.Message != "" {
		return wrap.Error.Message
	}
	return wrap.Message
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
