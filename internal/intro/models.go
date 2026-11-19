package intro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// VideoModelInfo is one entry from GET /api/v1/models.
type VideoModelInfo struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	MaxAudios     int     `json:"max_audios"`
	SupportsImage bool    `json:"supports_image"`
	SupportsAudio bool    `json:"supports_audio"`
	Durations     []int   `json:"durations"`
	DefaultDur    int     `json:"default_duration"`
	UnitPrice     float64 `json:"unit_price"`
	Price         float64 `json:"price"`
	Ratios        []ratio `json:"ratios"`
}

type ratio struct {
	Ratio string `json:"ratio"`
}

// ListVideoModels returns models available to the current workbench key.
func ListVideoModels(ctx context.Context, base, apiKey string, client *http.Client) ([]VideoModelInfo, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = DefaultVideoBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	resp, err := videoHTTP(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询视频模型失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("查询视频模型失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", formatVideoHTTPError(resp.StatusCode, raw))
	}
	var parsed struct {
		Data []VideoModelInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("视频模型列表无法解析: %w", err)
	}
	return parsed.Data, nil
}

// SuggestAudioModel picks a model that can take a reference image and audio
// for the requested duration and ratio. preferred is used when it qualifies.
func SuggestAudioModel(models []VideoModelInfo, preferred string, duration int, ratio string) (VideoModelInfo, error) {
	preferred = strings.TrimSpace(preferred)
	ratio = strings.TrimSpace(ratio)
	if duration <= 0 {
		duration = VideoDuration
	}
	var ok []VideoModelInfo
	for _, m := range models {
		if !m.SupportsAudio || m.MaxAudios < 1 || !m.SupportsImage {
			continue
		}
		if !durationAllowed(m.Durations, duration) {
			continue
		}
		if ratio != "" && !ratioAllowed(m.Ratios, ratio) {
			continue
		}
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		ok = append(ok, m)
	}
	if len(ok) == 0 {
		return VideoModelInfo{}, fmt.Errorf("当前账号下没有同时支持参考图和参考音频、且能生成 %d 秒 %s 的口播模型，请在工作台开通后再试", duration, ratio)
	}
	if preferred != "" {
		for _, m := range ok {
			if m.ID == preferred {
				return m, nil
			}
		}
	}
	best := ok[0]
	bestScore := audioModelScore(best, duration)
	for _, m := range ok[1:] {
		if s := audioModelScore(m, duration); s > bestScore {
			best, bestScore = m, s
		}
	}
	return best, nil
}

func durationAllowed(durations []int, want int) bool {
	if len(durations) == 0 {
		return true
	}
	for _, d := range durations {
		if d == want {
			return true
		}
	}
	return false
}

func ratioAllowed(ratios []ratio, want string) bool {
	if len(ratios) == 0 || want == "" {
		return true
	}
	for _, r := range ratios {
		if strings.TrimSpace(r.Ratio) == want {
			return true
		}
	}
	return false
}

func audioModelScore(m VideoModelInfo, duration int) int {
	score := 0
	id := strings.ToLower(m.ID + " " + m.Name)
	if strings.Contains(id, "seedance") {
		score += 50
	}
	if strings.Contains(id, "fast") {
		score += 20
	}
	if strings.Contains(id, "15s") || (duration > 0 && m.DefaultDur == duration) {
		score += 10
	}
	// Prefer cheaper models; unknown price is treated as expensive.
	price := m.UnitPrice
	if price <= 0 {
		price = m.Price
	}
	switch {
	case price <= 0:
		score -= 5
	case price < 3:
		score += 15
	case price < 8:
		score += 8
	case price >= 20:
		score -= 20
	}
	return score
}
