package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type pipixiaParser struct{}

func (p *pipixiaParser) Name() string { return "皮皮虾" }

func (p *pipixiaParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "pipix.com")
}

func (p *pipixiaParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	videoID, err := p.resolveShare(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("皮皮虾: %w", err)
	}

	apiURL := fmt.Sprintf("https://is.snssdk.com/bds/cell/detail/?cell_type=1&aid=1319&app_name=super&cell_id=%s", videoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("皮皮虾: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 1*1024*1024)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Data struct {
				Item struct {
					OriginVideoDownload struct {
						URLList []struct{ URL string `json:"url"` } `json:"url_list"`
					} `json:"origin_video_download"`
					Cover struct {
						URLList []struct{ URL string `json:"url"` } `json:"url_list"`
					} `json:"cover"`
					Share struct {
						Title string `json:"title"`
					} `json:"share"`
					Author struct {
						ID     int64  `json:"id"`
						Name   string `json:"name"`
					} `json:"author"`
				} `json:"item"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("皮皮虾: 解析失败: %w", err)
	}

	item := result.Data.Data.Item
	videoURL := ""
	if len(item.OriginVideoDownload.URLList) > 0 {
		videoURL = item.OriginVideoDownload.URLList[0].URL
	}
	coverURL := ""
	if len(item.Cover.URLList) > 0 {
		coverURL = item.Cover.URLList[0].URL
	}

	return &VideoInfo{
		Platform: "pipixia",
		VideoURL: videoURL,
		CoverURL: coverURL,
		Title:    item.Share.Title,
		Author:   item.Author.Name,
	}, nil
}

func (p *pipixiaParser) resolveShare(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("未获取到重定向地址")
	}
	parts := strings.Split(strings.Split(loc, "?")[0], "/")
	vid := parts[len(parts)-1]
	if vid == "" {
		return "", fmt.Errorf("重定向地址中无视频ID")
	}
	return vid, nil
}
