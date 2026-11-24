package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultKokoroURL     = "http://127.0.0.1:3211"
	DefaultOpenVoiceURL  = "http://127.0.0.1:3212"
	DefaultIndexTTS2URL  = "http://127.0.0.1:3213"
	KokoroStartBat       = `E:\kokoro-tts\启动Kokoro.bat`
	OpenVoiceStartBat    = `E:\OpenVoice\启动配音服务.bat`
	IndexTTS2StartBat    = `E:\indextts2\启动IndexTTS2.bat`
)

func NormalizeLocalURL(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	raw = strings.TrimRight(raw, "/")
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw
	}
	return raw
}

func isUnreachable(err error) bool {
	if err == nil {
		return false
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return true
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		if errors.As(ue.Err, &op) {
			return true
		}
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connectex") ||
		strings.Contains(s, "actively refused") ||
		strings.Contains(s, "no connection could be made") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "server misbehaving")
}

func localDownError(service, bat string, err error) error {
	if isUnreachable(err) {
		return fmt.Errorf("未检测到 %s 服务，请先双击运行 %s", service, bat)
	}
	return fmt.Errorf("%s 调用失败: %w", service, err)
}

type localVoice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

func fetchLocalVoices(ctx context.Context, baseURL string) ([]VoiceOption, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/voices", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var list []localVoice
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]VoiceOption, 0, len(list))
	for _, v := range list {
		id := strings.TrimSpace(v.ID)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(v.Label)
		if label == "" {
			label = strings.TrimSpace(v.Name)
		}
		if label == "" {
			label = id
		}
		out = append(out, VoiceOption{ID: id, Label: label})
	}
	return out, nil
}

func postLocalTTS(ctx context.Context, baseURL string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/tts", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 40<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		var obj map[string]any
		if json.Unmarshal(raw, &obj) == nil {
			if e, ok := obj["error"].(string); ok && strings.TrimSpace(e) != "" {
				msg = e
			}
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if len(raw) < 44 || string(raw[:4]) != "RIFF" {
		return nil, fmt.Errorf("服务没有返回有效的 WAV 音频")
	}
	return raw, nil
}

func synthesizeChunkedWAV(ctx context.Context, baseURL, service, bat string, maxRunes int, text string, payload func(chunk string) any, outPath string) (float64, error) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}
	chunks := SplitNarration(text, maxRunes)
	if len(chunks) == 0 {
		return 0, fmt.Errorf("%s: 文本为空", service)
	}
	tmpDir, err := os.MkdirTemp("", "xuanyu-local-tts-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmpDir)
	parts := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		raw, err := postLocalTTS(ctx, baseURL, payload(chunk))
		if err != nil {
			return 0, localDownError(service, bat, err)
		}
		part := filepath.Join(tmpDir, fmt.Sprintf("part_%d.wav", i))
		if err := os.WriteFile(part, raw, 0o644); err != nil {
			return 0, err
		}
		parts = append(parts, part)
	}
	if err := ConcatWAVFiles(parts, outPath); err != nil {
		return 0, err
	}
	if dur, err := WAVDurationSec(outPath); err == nil && dur > 0 {
		return dur, nil
	}
	return EstimateDurationSecAtSpeed(text, 1), nil
}
