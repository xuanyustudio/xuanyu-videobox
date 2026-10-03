package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type weiboParser struct{}

func (w *weiboParser) Name() string { return "微博" }

func (w *weiboParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "weibo.com") && !strings.Contains(rawURL, "weibo.cn")
}

func (w *weiboParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	videoID := ""
	if strings.Contains(rawURL, "show?fid=") {
		videoID = queryParam(rawURL, "fid")
	} else {
		parts := strings.Split(strings.Trim(strings.Split(rawURL, "?")[0], "/"), "/")
		if len(parts) > 0 {
			videoID = parts[len(parts)-1]
		}
	}
	if videoID == "" {
		return nil, fmt.Errorf("微博: 无法提取视频ID")
	}

	reqURL := fmt.Sprintf("https://h5.video.weibo.com/api/component?page=/show/%s", videoID)
	postContent := `data={"Component_Play_Playinfo":{"oid":"` + videoID + `"}}`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(postContent))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", fmt.Sprintf("https://h5.video.weibo.com/show/%s", videoID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("微博: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 1*1024*1024)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Info struct {
				StreamURL  string            `json:"stream_url"`
				CoverImage string            `json:"cover_image"`
				Title      string            `json:"title"`
				Author     string            `json:"author"`
				Avatar     string            `json:"avatar"`
				URLs       map[string]string `json:"urls"`
				User       struct {
					ID int64 `json:"id"`
				} `json:"user"`
			} `json:"Component_Play_Playinfo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("微博: 解析失败: %w", err)
	}

	info := result.Data.Info
	videoURL := info.StreamURL
	for _, u := range info.URLs {
		if u != "" {
			videoURL = "https:" + u
			break
		}
	}
	coverURL := info.CoverImage
	if coverURL != "" && !strings.HasPrefix(coverURL, "http") {
		coverURL = "https:" + coverURL
	}

	return &VideoInfo{
		Platform: "weibo",
		VideoURL: videoURL,
		CoverURL: coverURL,
		Title:    info.Title,
		Author:   info.Author,
	}, nil
}
