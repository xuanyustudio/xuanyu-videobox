package replicate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/narratoai/narrato-go/internal/script"
)

const (
	// DefaultBaseURL / DefaultModel mirror the Python QWEN_* defaults.
	DefaultBaseURL = "https://api-inference.modelscope.cn/v1"
	DefaultModel   = "Qwen/Qwen3-VL-8B-Instruct"
)

// Client calls an OpenAI-compatible chat/completions endpoint with image inputs.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type visionMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string (system) or []contentPart (user)
}

type visionRequest struct {
	Model    string          `json:"model"`
	Messages []visionMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type visionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Analyze sends the frames (base64 JPEG data URLs, in time order) with the given
// prompts and returns the model's full text reply (non-streaming, like the Gradio app).
func (c *Client) Analyze(ctx context.Context, systemPrompt, userPrompt string, framePaths []string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("replicate: nil client")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return "", fmt.Errorf("请先在「设置」中填写爆款复刻 API Key")
	}
	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		model = DefaultModel
	}

	parts := []contentPart{{Type: "text", Text: userPrompt}}
	for _, p := range framePaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("读取关键帧失败: %w", err)
		}
		parts = append(parts, contentPart{
			Type:     "image_url",
			ImageURL: &imageURL{URL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)},
		})
	}

	var msgs []visionMessage
	if strings.TrimSpace(systemPrompt) != "" {
		msgs = append(msgs, visionMessage{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, visionMessage{Role: "user", Content: parts})

	payload, err := json.Marshal(visionRequest{Model: model, Messages: msgs})
	if err != nil {
		return "", fmt.Errorf("replicate: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, script.ChatCompletionsURL(base), bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("replicate: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("视觉模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("replicate: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", formatHTTPError(resp.StatusCode, body))
	}
	var out visionResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("replicate: decode response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("视觉模型返回错误: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("视觉模型返回为空")
	}
	return out.Choices[0].Message.Content, nil
}

func formatHTTPError(code int, body []byte) string {
	msg := []rune(strings.TrimSpace(string(body)))
	if len(msg) > 240 {
		msg = append(msg[:240], '…')
	}
	switch code {
	case 401, 403:
		return fmt.Sprintf("爆款复刻鉴权失败（HTTP %d），请检查设置里的「复刻 API Key」", code)
	case 404:
		return fmt.Sprintf("爆款复刻接口不存在（HTTP %d），请检查「复刻 Base URL」是否以 /v1 结尾，以及模型名是否正确", code)
	case 413:
		return fmt.Sprintf("请求体过大（HTTP %d），请减少关键帧数量", code)
	case 429:
		return fmt.Sprintf("爆款复刻请求过于频繁或额度用尽（HTTP %d），请稍后重试", code)
	default:
		return fmt.Sprintf("爆款复刻请求失败（HTTP %d）: %s", code, string(msg))
	}
}
