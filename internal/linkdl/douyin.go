package linkdl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	douyinMu     sync.RWMutex
	douyinAPIBase = "http://39.109.101.66:8000"
	douyinAPIKey  = "dtk_0ebb3c9076ad_NVJtZ8xXbGvYWPi1GCkOuFx5hjGwFgZV"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("抖音: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抖音: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 2*1024*1024)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return d.extractFromBody(body)

	case http.StatusAccepted:
		return d.pollTask(ctx, body, apiBase, apiKey)

	default:
		return nil, d.parseAPIError(body, resp.StatusCode)
	}
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
}

func (d *douyinParser) extractFromBody(body []byte) (*VideoInfo, error) {
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("抖音: 解析响应失败: %w", err)
	}
	if env.Error != nil && env.Error.Message != "" {
		return nil, fmt.Errorf("抖音: [%s] %s", env.Error.Code, env.Error.Message)
	}

	media, err := d.extractMedia(env.Data)
	if err != nil {
		return nil, err
	}
	return media, nil
}

// extractMedia tries two layouts:
//   1. data IS the content: data.media.video.url
//   2. data has a nested task result: data.result.media.video.url
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

func (d *douyinParser) pollTask(ctx context.Context, initialBody []byte, apiBase, apiKey string) (*VideoInfo, error) {
	var env apiEnvelope
	if err := json.Unmarshal(initialBody, &env); err != nil {
		return nil, fmt.Errorf("抖音: 解析 202 响应失败: %w", err)
	}

	var ref apiTaskRef
	if err := json.Unmarshal(env.Data, &ref); err != nil || ref.TaskID == "" {
		return nil, fmt.Errorf("抖音: 202 响应中无 task_id")
	}

	pollURL := fmt.Sprintf("%s/api/v1/tasks/%s", apiBase, ref.TaskID)

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

		resp, err := defaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("抖音: 轮询请求失败: %w", err)
		}
		body, err := readBodyLimit(resp.Body, 2*1024*1024)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusAccepted {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, d.parseAPIError(body, resp.StatusCode)
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
			return d.extractMedia(task.Result)
		case "failed":
			return nil, fmt.Errorf("抖音: 任务失败")
		default:
			continue
		}
	}
}

func (d *douyinParser) parseAPIError(body []byte, status int) error {
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error != nil && env.Error.Message != "" {
		return fmt.Errorf("抖音: [%s] %s", env.Error.Code, env.Error.Message)
	}
	return fmt.Errorf("抖音: HTTP %d", status)
}
