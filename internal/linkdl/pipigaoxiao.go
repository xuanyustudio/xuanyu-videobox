package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type pipigaoxiaoParser struct{}

func (p *pipigaoxiaoParser) Name() string { return "皮皮搞笑" }

func (p *pipigaoxiaoParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "pipigx.com") || strings.Contains(rawURL, "ippzone.com")
}

func (p *pipigaoxiaoParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	parts := strings.Split(strings.Trim(strings.Split(rawURL, "?")[0], "/"), "/")
	videoID := parts[len(parts)-1]
	if videoID == "" {
		return nil, fmt.Errorf("皮皮搞笑: 无法提取视频ID")
	}

	apiURL := "https://share.ippzone.com/ppapi/share/fetch_content"
	postContent := `{"pid":` + videoID + `,"type":"post","mid":null}`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(postContent))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Referer", apiURL)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("皮皮搞笑: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 1*1024*1024)
	if err != nil {
		return nil, err
	}

	var result struct {
		Msg  string `json:"msg"`
		Data struct {
			Post struct {
				Content string `json:"content"`
				Imgs    []struct {
					ID string `json:"id"`
				} `json:"imgs"`
				Videos map[string]struct {
					URL string `json:"url"`
				} `json:"videos"`
			} `json:"post"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("皮皮搞笑: 解析失败: %w", err)
	}
	if result.Msg != "" {
		return nil, fmt.Errorf("皮皮搞笑: %s", result.Msg)
	}

	post := result.Data.Post
	videoURL := ""
	coverURL := ""
	if len(post.Imgs) > 0 {
		imgID := post.Imgs[0].ID
		coverURL = fmt.Sprintf("https://file.ippzone.com/img/view/id/%s", imgID)
		if v, ok := post.Videos[imgID]; ok {
			videoURL = v.URL
		}
	}

	return &VideoInfo{
		Platform: "pipigaoxiao",
		VideoURL: videoURL,
		CoverURL: coverURL,
		Title:    post.Content,
	}, nil
}
