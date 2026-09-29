package asr

import (
	"bytes"
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
	dashscopeBase       = "https://dashscope.aliyuncs.com"
	uploadPolicyURL     = dashscopeBase + "/api/v1/uploads"
	transcriptionURL    = dashscopeBase + "/api/v1/services/audio/asr/transcription"
	bailianModel        = "fun-asr"
)

// BailianClient calls Aliyun DashScope Fun-ASR (online).
type BailianClient struct {
	APIKey  string
	Timeout time.Duration
	HTTP    *http.Client
}

func (c *BailianClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	to := c.Timeout
	if to <= 0 {
		to = 10 * time.Minute
	}
	return &http.Client{Timeout: to}
}

func (c *BailianClient) authHeaders(extra map[string]string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	h.Set("Content-Type", "application/json")
	for k, v := range extra {
		h.Set(k, v)
	}
	return h
}

// TranscribeFile uploads media via Bailian temp OSS and writes SRT.
func (c *BailianClient) TranscribeFile(mediaPath, outPath string) error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("百炼 API Key 为空")
	}
	if _, err := os.Stat(mediaPath); err != nil {
		return fmt.Errorf("media: %w", err)
	}
	policy, err := c.requestPolicy()
	if err != nil {
		return err
	}
	ossURL, err := c.uploadOSS(mediaPath, policy)
	if err != nil {
		return err
	}
	taskID, err := c.submitTask(ossURL)
	if err != nil {
		return err
	}
	result, err := c.pollTask(taskID)
	if err != nil {
		return err
	}
	trURL, _ := result["transcription_url"].(string)
	if trURL == "" {
		return fmt.Errorf("百炼结果缺少 transcription_url")
	}
	payload, err := c.getJSON(trURL)
	if err != nil {
		return err
	}
	srt, err := bailianResultToSRT(payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(srt), 0o644)
}

type uploadPolicy struct {
	UploadHost           string
	UploadDir            string
	Policy               string
	Signature            string
	OSSAccessKeyID       string
	XOSSObjectACL        string
	XOSSForbidOverwrite  string
	MaxFileSizeMB        float64
}

func (c *BailianClient) requestPolicy() (*uploadPolicy, error) {
	req, err := http.NewRequest(http.MethodGet, uploadPolicyURL+"?action=getPolicy&model="+bailianModel, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.authHeaders(nil)
	req.Header.Del("Content-Type")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取百炼上传凭证: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取百炼上传凭证 HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var wrap struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("解析上传凭证: %w", err)
	}
	d := wrap.Data
	p := &uploadPolicy{
		UploadHost:          strField(d, "upload_host"),
		UploadDir:           strings.TrimRight(strField(d, "upload_dir"), "/"),
		Policy:              strField(d, "policy"),
		Signature:           strField(d, "signature"),
		OSSAccessKeyID:      strField(d, "oss_access_key_id"),
		XOSSObjectACL:       strField(d, "x_oss_object_acl"),
		XOSSForbidOverwrite: strField(d, "x_oss_forbid_overwrite"),
	}
	if p.XOSSObjectACL == "" {
		p.XOSSObjectACL = "private"
	}
	if p.XOSSForbidOverwrite == "" {
		p.XOSSForbidOverwrite = "true"
	}
	if v, ok := d["max_file_size_mb"].(float64); ok {
		p.MaxFileSizeMB = v
	}
	if p.UploadHost == "" || p.UploadDir == "" || p.Policy == "" || p.Signature == "" || p.OSSAccessKeyID == "" {
		return nil, fmt.Errorf("临时存储上传凭证缺少字段")
	}
	return p, nil
}

func (c *BailianClient) uploadOSS(localFile string, p *uploadPolicy) (string, error) {
	if p.MaxFileSizeMB > 0 {
		st, err := os.Stat(localFile)
		if err != nil {
			return "", err
		}
		if float64(st.Size()) > p.MaxFileSizeMB*1024*1024 {
			return "", fmt.Errorf("文件超过百炼临时存储限制 %.0fMB", p.MaxFileSizeMB)
		}
	}
	key := p.UploadDir + "/" + filepath.Base(localFile)
	f, err := os.Open(localFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("OSSAccessKeyId", p.OSSAccessKeyID)
	_ = w.WriteField("policy", p.Policy)
	_ = w.WriteField("Signature", p.Signature)
	_ = w.WriteField("key", key)
	_ = w.WriteField("x-oss-object-acl", p.XOSSObjectACL)
	_ = w.WriteField("x-oss-forbid-overwrite", p.XOSSForbidOverwrite)
	_ = w.WriteField("success_action_status", "200")
	part, err := w.CreateFormFile("file", filepath.Base(localFile))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	_ = w.Close()

	req, err := http.NewRequest(http.MethodPost, p.UploadHost, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("上传到百炼临时存储: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("上传临时存储 HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return "oss://" + key, nil
}

func (c *BailianClient) submitTask(ossURL string) (string, error) {
	payload := map[string]any{
		"model": bailianModel,
		"input": map[string]any{
			"file_urls": []string{ossURL},
		},
		"parameters": map[string]any{
			"diarization_enabled": true,
		},
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, transcriptionURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header = c.authHeaders(map[string]string{
		"X-DashScope-Async":             "enable",
		"X-DashScope-OssResourceResolve": "enable",
	})
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("提交百炼转写: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("提交转写 HTTP %d: %s", resp.StatusCode, truncate(string(body), 240))
	}
	var wrap struct {
		Output struct {
			TaskID string `json:"task_id"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return "", err
	}
	if wrap.Output.TaskID == "" {
		return "", fmt.Errorf("提交转写未返回 task_id: %s", truncate(string(body), 200))
	}
	return wrap.Output.TaskID, nil
}

func (c *BailianClient) pollTask(taskID string) (map[string]any, error) {
	deadline := time.Now().Add(c.Timeout)
	if c.Timeout <= 0 {
		deadline = time.Now().Add(10 * time.Minute)
	}
	url := dashscopeBase + "/api/v1/tasks/" + taskID
	lastStatus := "PENDING"
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodPost, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header = c.authHeaders(nil)
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("查询转写任务: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("查询任务 HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		}
		var wrap struct {
			Output struct {
				TaskStatus string           `json:"task_status"`
				Results    []map[string]any `json:"results"`
			} `json:"output"`
		}
		if err := json.Unmarshal(body, &wrap); err != nil {
			return nil, err
		}
		lastStatus = strings.ToUpper(wrap.Output.TaskStatus)
		switch lastStatus {
		case "SUCCEEDED":
			if len(wrap.Output.Results) == 0 {
				return nil, fmt.Errorf("转写成功但无结果")
			}
			return wrap.Output.Results[0], nil
		case "FAILED", "CANCELED", "UNKNOWN":
			return nil, fmt.Errorf("百炼转写失败: %s", lastStatus)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("百炼转写超时，最后状态: %s", lastStatus)
}

func (c *BailianClient) getJSON(u string) (map[string]any, error) {
	resp, err := c.httpClient().Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载转写结果 HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func strField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
