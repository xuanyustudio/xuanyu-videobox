package script

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

//go:embed prompt_xuanyu.txt
var xuanyuSystemPrompt string

// LLM calls an OpenAI-compatible chat/completions API.
type LLM struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// NewLLM builds a client with default HTTP transport.
func NewLLM(baseURL, apiKey, model string) *LLM {
	return &LLM{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
	}
}

// GenerateInput parameters for script generation.
type GenerateInput struct {
	SubtitleText  string
	WordCount     int
	OriginalRatio int
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GenerateScript requests script items from the LLM and parses the JSON array in the reply.
func (c *LLM) GenerateScript(ctx context.Context, in GenerateInput) ([]Item, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: nil client")
	}
	url := ChatCompletionsURL(c.BaseURL)

	userContent := fmt.Sprintf(
		"字幕内容：\n%s\n\n目标解说字数：%d\n原片占比（%%）：%d",
		in.SubtitleText,
		in.WordCount,
		in.OriginalRatio,
	)

	reqBody := chatCompletionRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: strings.TrimSpace(xuanyuSystemPrompt)},
			{Role: "user", Content: userContent},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", formatHTTPError(resp.StatusCode, body))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return nil, fmt.Errorf("llm: decode response: %w", err)
	}
	if completion.Error != nil && completion.Error.Message != "" {
		return nil, fmt.Errorf("llm: api error: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("llm: empty choices")
	}

	content := completion.Choices[0].Message.Content
	jsonText, err := extractJSONArray(content)
	if err != nil {
		return nil, err
	}

	var items []Item
	if err := json.Unmarshal([]byte(jsonText), &items); err != nil {
		return nil, fmt.Errorf("llm: decode items: %w", err)
	}
	return items, nil
}

// ChatCompletionsURL normalizes an OpenAI-compatible base URL and appends /chat/completions.
func ChatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// Common mistake: paste https://ai.lupoapi.com without /v1 (homepage HTML, not API).
	if u, err := url.Parse(base); err == nil {
		if u.Scheme == "https" && strings.Trim(u.Path, "/") == "" {
			base = base + "/v1"
		}
	}
	return base + "/chat/completions"
}

func extractJSONArray(content string) (string, error) {
	s := strings.TrimSpace(content)
	if s == "" {
		return "", fmt.Errorf("llm: empty message content")
	}
	s = stripMarkdownJSONFences(s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("llm: empty json payload")
	}
	return s, nil
}

func stripMarkdownJSONFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "json") {
		s = strings.TrimSpace(s[4:])
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func formatHTTPError(code int, body []byte) string {
	msg := strings.TrimSpace(string(body))
	switch code {
	case 401, 403:
		return fmt.Sprintf("LLM 鉴权失败（HTTP %d），请检查设置里的 API Key", code)
	case 404:
		return fmt.Sprintf("LLM 接口不存在（HTTP %d），请检查 Base URL 是否以 /v1 结尾", code)
	case 429:
		return fmt.Sprintf("LLM 请求过于频繁（HTTP %d），请稍后重试", code)
	case 502, 503, 504:
		hint := "上游服务暂时不可用"
		if strings.Contains(strings.ToLower(msg), "upstream") {
			hint = "上游模型服务暂时不可用（中转站/模型侧故障）"
		}
		return fmt.Sprintf("LLM %s（HTTP %d）。可稍后重试，或在设置里换模型/换 API 地址。详情: %s", hint, code, truncateRunes(msg, 180))
	default:
		return fmt.Sprintf("LLM 请求失败（HTTP %d）: %s", code, truncateRunes(msg, 240))
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
