package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type haokanParser struct{}

func (h *haokanParser) Name() string { return "好看视频" }

func (h *haokanParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "haokan.baidu.com") ||
		strings.Contains(rawURL, "haokan.hao123.com")
}

func (h *haokanParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	vid := queryParam(rawURL, "vid")
	if vid == "" {
		return nil, fmt.Errorf("好看视频: 链接中无 vid 参数")
	}
	return h.parseWithBase(ctx, vid, "https://haokan.baidu.com")
}

func (h *haokanParser) parseWithBase(ctx context.Context, vid, baseURL string) (*VideoInfo, error) {
	apiURL := fmt.Sprintf("%s/v?_format=json&vid=%s", baseURL, vid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("好看视频: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 1*1024*1024)
	if err != nil {
		return nil, err
	}

	var result struct {
		Errno int `json:"errno"`
		Error string `json:"error"`
		Data  struct {
			APIData struct {
				CurVideoMeta struct {
					PlayURL string `json:"playurl"`
					Poster  string `json:"poster"`
					Title   string `json:"title"`
					Mth     struct {
						AuthorName  string `json:"author_name"`
						AuthorPhoto string `json:"author_photo"`
					} `json:"mth"`
				} `json:"curVideoMeta"`
			} `json:"apiData"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("好看视频: 解析失败: %w", err)
	}
	if result.Errno != 0 {
		return nil, fmt.Errorf("好看视频: %s", result.Error)
	}

	meta := result.Data.APIData.CurVideoMeta
	return &VideoInfo{
		Platform: "haokan",
		VideoURL: meta.PlayURL,
		CoverURL: meta.Poster,
		Title:    meta.Title,
		Author:   meta.Mth.AuthorName,
	}, nil
}
