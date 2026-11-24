package asr

import (
	"bytes"
	"context"
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

	"github.com/narratoai/narrato-go/internal/media"
)

const DefaultAPIURL = "http://127.0.0.1:7860"
const DefaultFireRedAPIURL = "http://127.0.0.1:7867"

// SenseVoiceModel is the FunASR-Pack OpenAI-compatible model id for SenseVoiceSmall.
// The pack serves Chinese local captions over HTTP; this binary does not embed the model.
const SenseVoiceModel = "sensevoice"

// FunASRPackDownloadURL is the existing FunASR-Pack Windows bundle hint.
const FunASRPackDownloadURL = "https://cutagent.online/resources/funasr-v110-windows"

// LabelLocalFunASR is the settings/status name for the local SenseVoiceSmall path.
const LabelLocalFunASR = "本地 FunASR（SenseVoiceSmall）"

// Client talks to a local FunASR-Pack / FireRed compatible HTTP service.
type Client struct {
	APIURL    string
	Hotword   string
	EnableSpk bool
	FFmpeg    string
	Timeout   time.Duration
	HTTP      *http.Client
	// extractAudio, if set, writes a wav to dst (tests).
	extractAudio func(src, dst string) error
}

func (c *Client) baseURL() string {
	u := strings.TrimSpace(c.APIURL)
	if u == "" {
		u = DefaultAPIURL
	}
	u = strings.TrimRight(u, "/")
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return DefaultAPIURL
	}
	p := strings.TrimRight(parsed.Path, "/")
	for _, suf := range []string{"/v1/audio/transcriptions", "/v1", "/asr"} {
		if strings.HasSuffix(p, suf) {
			p = strings.TrimSuffix(p, suf)
			break
		}
	}
	parsed.Path = p
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	to := c.Timeout
	if to <= 0 {
		to = 10 * time.Minute
	}
	return &http.Client{Timeout: to}
}

// TranscribeFile uploads media and writes an SRT to outPath.
func (c *Client) TranscribeFile(mediaPath, outPath string) error {
	if _, err := os.Stat(mediaPath); err != nil {
		return fmt.Errorf("media: %w", err)
	}
	if err := c.Probe(); err != nil {
		return err
	}
	uploadPath, cleanup, err := c.prepareUpload(mediaPath)
	if err != nil {
		return err
	}
	defer cleanup()

	base := c.baseURL()
	openaiURL := base + "/v1/audio/transcriptions"
	asrURL := base + "/asr"

	fields := map[string]string{"enable_spk": spkFlag(c.EnableSpk)}
	if hw := strings.TrimSpace(c.Hotword); hw != "" {
		fields["hotword"] = hw
	}
	result, err := c.postMultipart(asrURL, uploadPath, fields)
	if err != nil {
		if isDialErr(err) {
			return friendlyDialError(err, base)
		}
		if !isNotFound(err) {
			return err
		}
		result, err = c.postMultipart(openaiURL, uploadPath, map[string]string{
			"model":           SenseVoiceModel,
			"response_format": "verbose_json",
			"spk":             spkFlag(c.EnableSpk),
		})
		if err != nil {
			if isDialErr(err) {
				return friendlyDialError(err, base)
			}
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	if srtFile, _ := result["srt_file"].(string); srtFile != "" {
		if st, e := os.Stat(srtFile); e == nil && !st.IsDir() {
			data, e := os.ReadFile(srtFile)
			if e != nil {
				return e
			}
			return os.WriteFile(outPath, data, 0o644)
		}
	}

	if dl := downloadURLFromResult(result); dl != "" {
		abs := resolveDownloadURL(base, dl)
		data, e := c.getBytes(abs)
		if e != nil {
			return e
		}
		if strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("empty SRT from %s", abs)
		}
		return os.WriteFile(outPath, data, 0o644)
	}

	srt, e := resultToSRT(result)
	if e != nil {
		return e
	}
	return os.WriteFile(outPath, []byte(srt), 0o644)
}

// TranscribeFireRed calls local FireRedASR2-AED-Pack /asr endpoint.
func (c *Client) TranscribeFireRed(mediaPath, outPath string) error {
	if _, err := os.Stat(mediaPath); err != nil {
		return fmt.Errorf("media: %w", err)
	}
	if err := c.Probe(); err != nil {
		return err
	}
	uploadPath, cleanup, err := c.prepareUpload(mediaPath)
	if err != nil {
		return err
	}
	defer cleanup()
	base := c.baseURL()
	result, err := c.postMultipart(base+"/asr", uploadPath, map[string]string{
		"enable_vad":       "true",
		"enable_lid":       "true",
		"enable_punc":      "true",
		"return_timestamp": "true",
	})
	if err != nil {
		if isDialErr(err) {
			return friendlyDialError(err, base)
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if srtFile, _ := result["srt_file"].(string); srtFile != "" {
		if st, e := os.Stat(srtFile); e == nil && !st.IsDir() {
			data, e := os.ReadFile(srtFile)
			if e != nil {
				return e
			}
			return os.WriteFile(outPath, data, 0o644)
		}
	}
	if dl := downloadURLFromResult(result); dl != "" {
		abs := resolveDownloadURL(base, dl)
		data, e := c.getBytes(abs)
		if e != nil {
			return e
		}
		return os.WriteFile(outPath, data, 0o644)
	}
	// FireRed often returns sentences[]
	if sents, ok := result["sentences"].([]any); ok && len(sents) > 0 {
		srt, e := fireRedSentencesToSRT(sents)
		if e != nil {
			return e
		}
		return os.WriteFile(outPath, []byte(srt), 0o644)
	}
	srt, e := resultToSRT(result)
	if e != nil {
		return e
	}
	return os.WriteFile(outPath, []byte(srt), 0o644)
}

func spkFlag(on bool) string {
	if on {
		return "true"
	}
	return "false"
}

func fireRedSentencesToSRT(sents []any) (string, error) {
	var b strings.Builder
	n := 0
	for _, raw := range sents {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(m["text"]))
		if text == "" || text == "<nil>" {
			continue
		}
		start := asSeconds(m["start"])
		if start == 0 {
			start = asSeconds(m["begin_time"]) / 1000
		}
		end := asSeconds(m["end"])
		if end == 0 {
			end = asSeconds(m["end_time"]) / 1000
		}
		if end <= start {
			end = start + 0.5
		}
		n++
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, formatSRTTime(start), formatSRTTime(end), text)
	}
	if n == 0 {
		return "", fmt.Errorf("FireRed 结果为空")
	}
	return b.String(), nil
}

func isAudioFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav", ".mp3", ".m4a", ".flac", ".aac", ".ogg", ".opus", ".wma":
		return true
	default:
		return false
	}
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found:")
}

func (c *Client) prepareUpload(mediaPath string) (string, func(), error) {
	nop := func() {}
	if isAudioFile(mediaPath) {
		return mediaPath, nop, nil
	}
	tmp, err := os.CreateTemp("", "xuanyu-asr-*.wav")
	if err != nil {
		return "", nop, err
	}
	dst := tmp.Name()
	tmp.Close()
	cleanup := func() { _ = os.Remove(dst) }
	if err := c.extractTrack(mediaPath, dst); err != nil {
		cleanup()
		return "", nop, err
	}
	return dst, cleanup, nil
}

func (c *Client) extractTrack(src, dst string) error {
	if c.extractAudio != nil {
		return c.extractAudio(src, dst)
	}
	ffmpeg, err := c.resolveFFmpeg()
	if err != nil || strings.TrimSpace(ffmpeg) == "" {
		return fmt.Errorf("识别视频需要先抽出音轨，请在「设置」里填写 ffmpeg 路径，或把 ffmpeg 加入系统 PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	log, err := media.Run(ctx, ffmpeg,
		"-y",
		"-i", src,
		"-map", "0:a:0",
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		dst,
	)
	if err != nil {
		_ = os.Remove(dst)
		msg := strings.TrimSpace(log)
		if msg == "" {
			msg = err.Error()
		}
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "does not contain any stream") || strings.Contains(msg, "Stream map '0:a:0'") {
			return fmt.Errorf("这个视频没有音轨，无法识别字幕")
		}
		return fmt.Errorf("抽音频失败: %s", truncate(msg, 240))
	}
	st, err := os.Stat(dst)
	if err != nil || st.Size() == 0 {
		_ = os.Remove(dst)
		return fmt.Errorf("抽音频完成但没有生成可用的音轨文件")
	}
	return nil
}

func (c *Client) resolveFFmpeg() (string, error) {
	beside := ""
	if exe, err := os.Executable(); err == nil {
		beside = filepath.Dir(exe)
	}
	return media.ResolveFFmpeg(c.FFmpeg, beside)
}

func (c *Client) postMultipart(endpoint, mediaPath string, fields map[string]string) (map[string]any, error) {
	f, err := os.Open(mediaPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	part, err := w.CreateFormFile("file", filepath.Base(mediaPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("asr request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found: %s", endpoint)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("asr %s: HTTP %d: %s", endpoint, resp.StatusCode, truncate(string(data), 200))
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("asr json: %w (%s)", err, truncate(string(data), 120))
	}
	return out, nil
}

func friendlyDialError(err error, base string) error {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "actively refused") ||
		strings.Contains(msg, "connectex") || strings.Contains(msg, "No connection could be made") {
		return fmt.Errorf("无法连接本地 FunASR（%s）。请先启动 FunASR-Pack 整合包，再到浏览器打开该地址确认服务已就绪；也可在「设置」里改 API 地址。下载：%s", base, FunASRPackDownloadURL)
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "DeadlineExceeded") {
		return fmt.Errorf("连接 FunASR 超时（%s），请确认服务已启动且未被防火墙拦截", base)
	}
	return fmt.Errorf("无法连接 FunASR（%s）: %v", base, err)
}

// Probe reports whether the local ASR HTTP service responds.
func (c *Client) Probe() error {
	base := c.baseURL()
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for _, path := range []string{"/health", "/v1/models", "/"} {
		resp, err := client.Get(base + path)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()
		return nil
	}
	if lastErr != nil {
		return friendlyDialError(lastErr, base)
	}
	return fmt.Errorf("无法连接本地 FunASR（%s）", base)
}

func (c *Client) getBytes(u string) ([]byte, error) {
	resp, err := c.httpClient().Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: HTTP %d", u, resp.StatusCode)
	}
	return data, nil
}

func downloadURLFromResult(result map[string]any) string {
	if downloads, ok := result["downloads"].(map[string]any); ok {
		if s, ok := downloads["srt"].(string); ok && s != "" {
			return s
		}
	}
	for _, key := range []string{"srt_url", "srt_download_url", "download_url"} {
		if s, ok := result[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func resolveDownloadURL(base, dl string) string {
	if strings.Contains(dl, "://") {
		return dl
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(dl, "/")
}

func resultToSRT(result map[string]any) (string, error) {
	type cue struct {
		start, end float64
		text       string
	}
	var cues []cue

	if segs, ok := result["segments"].([]any); ok {
		for _, raw := range segs {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(m["text"]))
			if text == "" || text == "<nil>" {
				continue
			}
			start := asSeconds(m["start"])
			end := asSeconds(m["end"])
			if end <= start {
				end = start + 0.5
			}
			cues = append(cues, cue{start, end, text})
		}
	}

	if len(cues) == 0 {
		if text := strings.TrimSpace(fmt.Sprint(result["text"])); text != "" && text != "<nil>" {
			cues = append(cues, cue{0, float64(max(1500, len([]rune(text))*180)) / 1000, text})
		}
	}
	if len(cues) == 0 {
		return "", fmt.Errorf("ASR 结果为空，未找到字幕内容（请确认本地 FunASR-Pack 已启动）")
	}

	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, formatSRTTime(c.start), formatSRTTime(c.end), c.text)
	}
	return b.String(), nil
}

func asSeconds(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func formatSRTTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int(sec*1000 + 0.5)
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func isDialErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused") ||
		strings.Contains(msg, "connectex") ||
		strings.Contains(msg, "No connection could be made") ||
		strings.Contains(msg, "dial tcp")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DefaultOutPath builds a subtitle path under projectDir/subtitles.
func DefaultOutPath(projectDir, mediaPath string) string {
	stem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	name := stem + "_fun_asr.srt"
	return filepath.Join(projectDir, "subtitles", name)
}

// Ensure unique unused by appending _2, _3…
func UniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return path
}
