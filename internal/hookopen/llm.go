package hookopen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/script"
)

type chatContent struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type llmHookResult struct {
	Candidates []llmCandidate `json:"candidates"`
}

type llmCandidate struct {
	Rank          int     `json:"rank"`
	HookType      string  `json:"hook_type"`
	Reason        string  `json:"reason"`
	StartSec      float64 `json:"start_sec"`
	EndSec        float64 `json:"end_sec"`
	Start         any     `json:"start"`
	End           any     `json:"end"`
	Timestamp     string  `json:"timestamp"`
	Quote         string  `json:"quote"`
	IsRecommended bool    `json:"is_recommended"`
}

func modelConfig(cfg *config.Config) (base, key, model string) {
	if cfg == nil {
		return "", "", ""
	}
	base = strings.TrimSpace(cfg.Replicate.BaseURL)
	key = strings.TrimSpace(cfg.Replicate.APIKey)
	model = strings.TrimSpace(cfg.Replicate.Model)
	if key == "" {
		key = strings.TrimSpace(cfg.LLM.APIKey)
	}
	if base == "" {
		base = strings.TrimSpace(cfg.LLM.BaseURL)
	}
	if model == "" {
		model = strings.TrimSpace(cfg.LLM.Model)
	}
	return base, key, model
}

func pickWithModel(ctx context.Context, cfg *config.Config, workDir, transcript string, windows []scoredWindow, thumbs []string, duration float64) ([]Candidate, error) {
	base, key, model := modelConfig(cfg)
	if key == "" || model == "" {
		return nil, fmt.Errorf("请先在「设置」里填写视觉模型的 API Key 和模型名")
	}
	user := buildUserPrompt(transcript, windows, duration)
	msgs := []chatMessage{
		{Role: "system", Content: hookSystemPrompt},
		{Role: "user", Content: user},
	}
	// attach up to 6 thumbnails
	var parts []chatContent
	parts = append(parts, chatContent{Type: "text", Text: user})
	n := 0
	for i, p := range thumbs {
		if p == "" || n >= 6 {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			continue
		}
		mime := "image/jpeg"
		if strings.EqualFold(filepath.Ext(p), ".png") {
			mime = "image/png"
		}
		url := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
		label := fmt.Sprintf("候选片段 %d 的画面", i+1)
		parts = append(parts, chatContent{Type: "text", Text: label})
		parts = append(parts, chatContent{Type: "image_url", ImageURL: &imageURL{URL: url}})
		n++
	}
	if n > 0 {
		msgs[1].Content = parts
	}

	content, err := chatComplete(ctx, base, key, model, msgs)
	if err != nil && n > 0 {
		// retry text only
		msgs[1].Content = user
		content, err = chatComplete(ctx, base, key, model, msgs)
	}
	if err != nil {
		return nil, err
	}
	return parseHookJSON(content, duration)
}

func chatComplete(ctx context.Context, base, key, model string, msgs []chatMessage) (string, error) {
	url := script.ChatCompletionsURL(base)
	reqBody := chatRequest{Model: model, Messages: msgs, MaxTokens: 2048}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("模型请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("模型请求失败（HTTP %d）: %s", resp.StatusCode, truncateRunes(string(body), 240))
	}
	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("解析模型回复失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("模型错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("模型没有返回内容")
	}
	return messageContent(parsed.Choices[0].Message.Content), nil
}

func messageContent(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if s, ok := m["text"].(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func parseHookJSON(content string, duration float64) ([]Candidate, error) {
	s := stripFence(content)
	obj, err := extractJSONObject(s)
	if err != nil {
		return nil, err
	}
	var parsed llmHookResult
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		// maybe a bare array
		var arr []llmCandidate
		if err2 := json.Unmarshal([]byte(obj), &arr); err2 != nil {
			return nil, fmt.Errorf("模型返回的不是可用 JSON: %w", err)
		}
		parsed.Candidates = arr
	}
	var out []Candidate
	for i, raw := range parsed.Candidates {
		c := Candidate{
			Rank:        raw.Rank,
			HookType:    strings.TrimSpace(raw.HookType),
			Reason:      strings.TrimSpace(raw.Reason),
			Quote:       strings.TrimSpace(raw.Quote),
			Recommended: raw.IsRecommended,
			StartSec:    raw.StartSec,
			EndSec:      raw.EndSec,
		}
		if c.StartSec == 0 && c.EndSec == 0 {
			if raw.Timestamp != "" {
				c.StartSec, c.EndSec = parseTimeRange(raw.Timestamp)
			} else {
				c.StartSec = anyFloat(raw.Start)
				c.EndSec = anyFloat(raw.End)
			}
		}
		c.StartSec, c.EndSec = clampWindow(c.StartSec, c.EndSec, duration)
		if c.EndSec-c.StartSec < MinHookSec-0.05 {
			continue
		}
		if c.HookType == "" {
			c.HookType = "强悬念"
		}
		if c.Rank <= 0 {
			c.Rank = i + 1
		}
		out = append(out, c)
		if len(out) >= 5 {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("模型没有给出可用的片头时段")
	}
	normalizeRanks(out)
	return out, nil
}

func normalizeRanks(cs []Candidate) {
	hasRec := false
	for i := range cs {
		if cs[i].Rank <= 0 {
			cs[i].Rank = i + 1
		}
		if cs[i].Recommended {
			hasRec = true
		}
	}
	if !hasRec && len(cs) > 0 {
		cs[0].Recommended = true
	}
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "json") {
		s = strings.TrimSpace(s[4:])
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func extractJSONObject(s string) (string, error) {
	s = strings.TrimSpace(s)
	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")
	start := -1
	open := byte('{')
	close := byte('}')
	if startArr >= 0 && (startObj < 0 || startArr < startObj) {
		start = startArr
		open, close = '[', ']'
	} else if startObj >= 0 {
		start = startObj
	}
	if start < 0 {
		return "", fmt.Errorf("回复里没有 JSON")
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("JSON 不完整")
}

func parseTimeRange(s string) (float64, float64) {
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "—", "-")
	s = strings.ReplaceAll(s, "至", "-")
	if strings.Contains(s, "-->") {
		parts := strings.Split(s, "-->")
		if len(parts) == 2 {
			a, e1 := parseFlexible(parts[0])
			b, e2 := parseFlexible(parts[1])
			if e1 == nil && e2 == nil {
				return a, b
			}
		}
	}
	if i := strings.Index(s, "-"); i > 0 {
		a, e1 := parseFlexible(s[:i])
		b, e2 := parseFlexible(s[i+1:])
		if e1 == nil && e2 == nil {
			return a, b
		}
	}
	return 0, 0
}

func parseFlexible(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "秒")
	s = strings.TrimSpace(s)
	if strings.Contains(s, ":") {
		return parseStamp(strings.ReplaceAll(s, ".", ","))
	}
	return strconv.ParseFloat(s, 64)
}

func anyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		f, _ := parseFlexible(t)
		return f
	default:
		return 0
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
