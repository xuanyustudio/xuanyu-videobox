package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type lispinParser struct{}

func (l *lispinParser) Name() string { return "梨视频" }

func (l *lispinParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "pearvideo.com")
}

func (l *lispinParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	path := strings.Split(rawURL, "?")[0]
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var videoID string
	for _, p := range parts {
		if strings.HasPrefix(p, "detail_") {
			videoID = strings.TrimPrefix(p, "detail_")
			break
		}
	}
	if videoID == "" && len(parts) > 0 {
		videoID = parts[len(parts)-1]
	}
	if videoID == "" {
		return nil, fmt.Errorf("梨视频: 无法提取视频ID")
	}

	now := time.Now().Unix()
	apiURL := fmt.Sprintf("https://www.pearvideo.com/videoStatus.jsp?contId=%s&mrd=%d", videoID, now)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", fmt.Sprintf("https://www.pearvideo.com/detail_%s", videoID))

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("梨视频: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 512*1024)
	if err != nil {
		return nil, err
	}

	var result struct {
		VideoInfo struct {
			Videos struct {
				SrcURL string `json:"srcUrl"`
			} `json:"videos"`
			VideoImage string `json:"video_image"`
		} `json:"videoInfo"`
		SystemTime string `json:"systemTime"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("梨视频: 解析失败: %w", err)
	}

	videoURL := strings.ReplaceAll(result.VideoInfo.Videos.SrcURL, result.SystemTime, fmt.Sprintf("cont-%s", videoID))

	return &VideoInfo{
		Platform: "lishipin",
		VideoURL: videoURL,
		CoverURL: result.VideoInfo.VideoImage,
	}, nil
}
