package linkdl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	// Douyin API may hold connections longer than the shared 30s client.
	douyinAPIClient = &http.Client{Timeout: 180 * time.Second}

	douyinMu      sync.RWMutex
	douyinAPIBase = ""
	douyinAPIKey  = ""
)

// ConfigureDouyin sets the self-hosted Douyin_TikTok_Download_API endpoint.
// Call this after loading config; it is goroutine-safe.
func ConfigureDouyin(baseURL, apiKey string) {
	douyinMu.Lock()
	defer douyinMu.Unlock()
	if baseURL != "" {
		douyinAPIBase = strings.TrimRight(baseURL, "/")
	}
	if apiKey != "" {
		douyinAPIKey = apiKey
	}
}

func douyinCfg() (base, key string) {
	douyinMu.RLock()
	defer douyinMu.RUnlock()
	return douyinAPIBase, douyinAPIKey
}

type douyinParser struct{}

func (d *douyinParser) Name() string { return "抖音" }

func (d *douyinParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "douyin.com") ||
		strings.Contains(rawURL, "iesdouyin.com")
}

func (d *douyinParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	base, key := douyinCfg()
	if base == "" || key == "" {
		return nil, fmt.Errorf("抖音: 未配置 Douyin API（请在设置页填写地址和 Key）")
	}
	return d.parseViaAPI(ctx, rawURL, base, key)
}

// parseViaAPI calls the self-hosted Douyin_TikTok_Download_API.
// Exported base/key parameters make this testable without touching globals.
func (d *douyinParser) parseViaAPI(ctx context.Context, rawURL, apiBase, apiKey string) (*VideoInfo, error) {
	payload, _ := json.Marshal(map[string]string{"url": rawURL})
	endpoint := apiBase + "/api/v1/parse?wait=25"
	raw, err := douyinDo(ctx, http.MethodPost, endpoint, apiBase, apiKey, bytes.NewReader(payload), 2*1024*1024)
	if err != nil {
		return nil, err
	}
	return d.extractMedia(raw)
}

// apiEnvelope is the outer JSON wrapper the API returns.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *apiError       `json:"error,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiMedia struct {
	Title string `json:"title"`
	Media struct {
		Video struct {
			URL string `json:"url"`
		} `json:"video"`
	} `json:"media"`
}

type apiTaskRef struct {
	TaskID string `json:"task_id"`
}

type apiTaskPoll struct {
	State  string          `json:"state"`
	Result json.RawMessage `json:"result"`
	Error  *apiError       `json:"error,omitempty"`
}

// douyinDo performs an authenticated call and returns the success payload.
// HTTP 200 yields the envelope data. HTTP 202 polls /api/v1/tasks/{id} the
// same way single-video parse does, and returns the task result.
func douyinDo(ctx context.Context, method, endpoint, apiBase, apiKey string, body io.Reader, limit int64) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("抖音: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err := douyinAPIClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抖音: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBodyLimit(resp.Body, limit)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return envelopeData(respBody)
	case http.StatusAccepted:
		return pollTaskResult(ctx, respBody, apiBase, apiKey, limit)
	default:
		return nil, (&douyinParser{}).parseAPIError(respBody, resp.StatusCode)
	}
}

func envelopeData(body []byte) (json.RawMessage, error) {
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("抖音: 解析响应失败: %w", err)
	}
	if env.Error != nil && env.Error.Message != "" {
		return nil, fmt.Errorf("抖音: [%s] %s", env.Error.Code, env.Error.Message)
	}
	if len(bytes.TrimSpace(env.Data)) == 0 || string(bytes.TrimSpace(env.Data)) == "null" {
		return nil, fmt.Errorf("抖音: 响应中无数据")
	}
	return env.Data, nil
}

func pollTaskResult(ctx context.Context, initialBody []byte, apiBase, apiKey string, limit int64) (json.RawMessage, error) {
	var env apiEnvelope
	if err := json.Unmarshal(initialBody, &env); err != nil {
		return nil, fmt.Errorf("抖音: 解析 202 响应失败: %w", err)
	}

	var ref apiTaskRef
	if err := json.Unmarshal(env.Data, &ref); err != nil || ref.TaskID == "" {
		return nil, fmt.Errorf("抖音: 202 响应中无 task_id")
	}

	pollURL := fmt.Sprintf("%s/api/v1/tasks/%s", strings.TrimRight(apiBase, "/"), ref.TaskID)

	timeout := 120 * time.Second
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	interval := 2 * time.Second
	for {
		select {
		case <-deadline.Done():
			return nil, fmt.Errorf("抖音: 轮询超时（%s）", timeout)
		case <-time.After(interval):
		}

		req, err := http.NewRequestWithContext(deadline, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, fmt.Errorf("抖音: %w", err)
		}
		req.Header.Set("X-API-Key", apiKey)

		resp, err := douyinAPIClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("抖音: 轮询请求失败: %w", err)
		}
		body, err := readBodyLimit(resp.Body, limit)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusAccepted {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, (&douyinParser{}).parseAPIError(body, resp.StatusCode)
		}

		var pollEnv apiEnvelope
		if err := json.Unmarshal(body, &pollEnv); err != nil {
			return nil, fmt.Errorf("抖音: 轮询解析失败: %w", err)
		}
		if pollEnv.Error != nil && pollEnv.Error.Message != "" {
			return nil, fmt.Errorf("抖音: [%s] %s", pollEnv.Error.Code, pollEnv.Error.Message)
		}

		var task apiTaskPoll
		if err := json.Unmarshal(pollEnv.Data, &task); err != nil {
			return nil, fmt.Errorf("抖音: 轮询数据解析失败: %w", err)
		}

		switch task.State {
		case "done":
			if len(bytes.TrimSpace(task.Result)) == 0 || string(bytes.TrimSpace(task.Result)) == "null" {
				return nil, fmt.Errorf("抖音: 任务完成但没有结果")
			}
			return task.Result, nil
		case "failed":
			if task.Error != nil && task.Error.Message != "" {
				if task.Error.Code == "IDENTITY_LOGIN_REQUIRED" || strings.Contains(string(body), "author_posts") || strings.Contains(task.Error.Message, "author_posts") {
				return nil, fmt.Errorf("抖音: 作者作品列表需要已登录的抖音账号身份。请打开设置里填写的 Douyin API 控制台 → Identities → Import，粘贴浏览器里抖音登录后的 Cookie，再重试")
			}
			return nil, fmt.Errorf("抖音: 任务失败: [%s] %s", task.Error.Code, task.Error.Message)
			}
			return nil, fmt.Errorf("抖音: 任务失败")
		default:
			continue
		}
	}
}

// extractMedia tries two layouts:
//  1. data IS the content: data.media.video.url
//  2. data has a nested task result: data.result.media.video.url
func (d *douyinParser) extractMedia(raw json.RawMessage) (*VideoInfo, error) {
	var m apiMedia
	if err := json.Unmarshal(raw, &m); err == nil && m.Media.Video.URL != "" {
		return &VideoInfo{
			Platform: "douyin",
			VideoURL: m.Media.Video.URL,
			Title:    m.Title,
		}, nil
	}

	var nested struct {
		Result apiMedia `json:"result"`
	}
	if err := json.Unmarshal(raw, &nested); err == nil && nested.Result.Media.Video.URL != "" {
		return &VideoInfo{
			Platform: "douyin",
			VideoURL: nested.Result.Media.Video.URL,
			Title:    nested.Result.Title,
		}, nil
	}

	return nil, fmt.Errorf("抖音: 响应中未找到视频地址")
}

func (d *douyinParser) parseAPIError(body []byte, status int) error {
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error != nil {
		code := strings.TrimSpace(env.Error.Code)
		msg := strings.TrimSpace(env.Error.Message)
		if code == "IDENTITY_LOGIN_REQUIRED" || (code == "NOT_FOUND" && strings.Contains(string(body), "author_posts")) {
			return fmt.Errorf("抖音: 作者作品列表需要已登录的抖音账号身份。请打开设置里填写的 Douyin API 控制台 → Identities → Import，粘贴浏览器里抖音登录后的 Cookie，再重试")
		}
		if msg != "" {
			return fmt.Errorf("抖音: [%s] %s", code, msg)
		}
	}
	return fmt.Errorf("抖音: HTTP %d", status)
}
