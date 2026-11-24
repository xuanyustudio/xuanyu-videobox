package sonilo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL         = "https://api.sonilo.com"
	videoToMusicPath       = "/v1/video-to-music"
	videoToSFXPath         = "/v1/video-to-sfx"
	tasksPath              = "/v1/tasks"
	DefaultTimeout         = 10 * time.Minute
	connectTimeout         = 15 * time.Second
	pollReadTimeout        = 30 * time.Second
	sfxPollInterval        = 5 * time.Second
	MaxBGMVideoDurationSec = 360
	MaxSFXVideoDurationSec = 180
)

// Client talks to the Sonilo video-to-music and video-to-sfx APIs.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Timeout time.Duration
	// Sleep is used by SFX polling; tests may replace it.
	Sleep func(time.Duration)
}

func (c *Client) baseURL() string {
	s := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if s == "" {
		return DefaultBaseURL
	}
	return s
}

func (c *Client) timeout() time.Duration {
	if c != nil && c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: c.timeout() + connectTimeout}
}

func (c *Client) sleep() func(time.Duration) {
	if c != nil && c.Sleep != nil {
		return c.Sleep
	}
	return time.Sleep
}

// GenerateBGM uploads a finished video (before BGM) and writes an .m4a file to dest.
func (c *Client) GenerateBGM(ctx context.Context, videoPath, prompt, dest string) error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("请先在「设置」里填写 Sonilo API Key")
	}
	audio, err := c.requestVideoToMusic(ctx, videoPath, prompt)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, audio, 0o644)
}

// GenerateSFX uploads a finished video and writes an .m4a file to dest.
func (c *Client) GenerateSFX(ctx context.Context, videoPath, prompt, dest string) error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("请先在「设置」里填写 Sonilo API Key")
	}
	taskID, err := c.submitSFX(ctx, videoPath, prompt)
	if err != nil {
		return err
	}
	body, err := c.pollSFX(ctx, taskID)
	if err != nil {
		return err
	}
	url, err := extractSFXURL(body, taskID)
	if err != nil {
		return err
	}
	audio, err := downloadPresigned(ctx, c.http(), url, c.timeout())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, audio, 0o644)
}

func (c *Client) requestVideoToMusic(ctx context.Context, videoPath, prompt string) ([]byte, error) {
	req, err := c.newUpload(ctx, c.baseURL()+videoToMusicPath, videoPath, prompt)
	if err != nil {
		return nil, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Sonilo 配乐请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("Sonilo 配乐失败: %s", httpErr(resp.StatusCode, raw))
	}
	return ConsumeNDJSON(resp.Body)
}

func (c *Client) submitSFX(ctx context.Context, videoPath, prompt string) (string, error) {
	req, err := c.newUpload(ctx, c.baseURL()+videoToSFXPath, videoPath, prompt)
	if err != nil {
		return "", err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("Sonilo 音效任务提交失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("Sonilo 音效任务提交失败: %s", httpErr(resp.StatusCode, raw))
	}
	var parsed struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || strings.TrimSpace(parsed.TaskID) == "" {
		return "", fmt.Errorf("Sonilo 音效任务已受理但未返回 task_id")
	}
	return parsed.TaskID, nil
}

func (c *Client) pollSFX(ctx context.Context, taskID string) (map[string]any, error) {
	deadline := time.Now().Add(c.timeout())
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+tasksPath+"/"+taskID, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		resp, err := c.http().Do(req)
		if err == nil && resp != nil {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				// retry
			} else if resp.StatusCode >= 400 {
				return nil, fmt.Errorf("%s（任务已提交, task_id: %s）", httpErr(resp.StatusCode, raw), taskID)
			} else {
				var body map[string]any
				if json.Unmarshal(raw, &body) == nil {
					switch fmt.Sprint(body["status"]) {
					case "succeeded":
						return body, nil
					case "failed":
						return nil, fmt.Errorf("%s", taskFailMessage(body, taskID))
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("等待 Sonilo 音效任务超时, task_id: %s", taskID)
		}
		c.sleep()(sfxPollInterval)
	}
}

func (c *Client) newUpload(ctx context.Context, url, videoPath, prompt string) (*http.Request, error) {
	f, err := os.Open(videoPath)
	if err != nil {
		return nil, fmt.Errorf("打开视频失败: %w", err)
	}
	defer f.Close()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("video", filepath.Base(videoPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, err
	}
	if p := strings.TrimSpace(prompt); p != "" {
		if err := w.WriteField("prompt", p); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

func downloadPresigned(ctx context.Context, httpc *http.Client, url string, timeout time.Duration) ([]byte, error) {
	// Presigned result URLs already carry auth; never attach the API key.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if httpc == nil {
		httpc = &http.Client{Timeout: timeout}
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Sonilo 音效结果下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Sonilo 音效结果下载失败 (HTTP %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 80<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("Sonilo 音效结果为空")
	}
	return data, nil
}

func extractSFXURL(body map[string]any, taskID string) (string, error) {
	audio, _ := body["audio"].(map[string]any)
	if audio != nil {
		if url, _ := audio["url"].(string); strings.TrimSpace(url) != "" {
			return url, nil
		}
	}
	return "", fmt.Errorf("Sonilo 音效任务成功但未返回音频结果 (task_id: %s)", taskID)
}

func taskFailMessage(body map[string]any, taskID string) string {
	msg := "生成失败"
	switch err := body["error"].(type) {
	case string:
		if strings.TrimSpace(err) != "" {
			msg = err
		}
	case map[string]any:
		if m, _ := err["message"].(string); strings.TrimSpace(m) != "" {
			msg = m
		} else if m, _ := err["code"].(string); strings.TrimSpace(m) != "" {
			msg = m
		}
	}
	refund := ""
	if body["refunded"] == true {
		refund = "，费用已退还"
	}
	return fmt.Sprintf("Sonilo 音效生成失败: %s（task_id: %s%s）", msg, taskID, refund)
}

func httpErr(code int, body []byte) string {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	if detail := errorDetail(msg); detail != "" {
		msg = detail
	}
	if msg == "" {
		return fmt.Sprintf("HTTP %d", code)
	}
	return fmt.Sprintf("HTTP %d: %s", code, msg)
}

func errorDetail(body string) string {
	var parsed map[string]any
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return ""
	}
	for _, key := range []string{"detail", "error", "message"} {
		if s, ok := parsed[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// ConsumeNDJSON reads a Sonilo video-to-music event stream and returns the first audio track.
func ConsumeNDJSON(r io.Reader) ([]byte, error) {
	return consumeNDJSON(r)
}

func consumeNDJSON(r io.Reader) ([]byte, error) {
	streams := map[int]*bytes.Buffer{}
	completed := false
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		switch fmt.Sprint(event["type"]) {
		case "audio_chunk":
			chunk, _ := event["data"].(string)
			if chunk == "" {
				continue
			}
			idx := 0
			switch v := event["stream_index"].(type) {
			case float64:
				idx = int(v)
			case string:
				n, err := strconv.Atoi(v)
				if err != nil {
					continue
				}
				idx = n
			}
			if idx < 0 {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(chunk)
			if err != nil {
				continue
			}
			buf := streams[idx]
			if buf == nil {
				buf = &bytes.Buffer{}
				streams[idx] = buf
			}
			buf.Write(decoded)
		case "complete":
			completed = true
		case "error":
			msg := fmt.Sprint(event["message"])
			if msg == "" || msg == "<nil>" {
				msg = fmt.Sprint(event["code"])
			}
			if msg == "" || msg == "<nil>" {
				msg = "stream error"
			}
			return nil, fmt.Errorf("Sonilo 生成失败: %s", msg)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !completed {
		return nil, fmt.Errorf("Sonilo 事件流意外终止（未收到 complete 事件）")
	}
	if len(streams) == 0 {
		return nil, fmt.Errorf("Sonilo 事件流已完成但未返回音频数据")
	}
	keys := make([]int, 0, len(streams))
	for k := range streams {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return streams[keys[0]].Bytes(), nil
}
