package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type xiguaParser struct{}

func (x *xiguaParser) Name() string { return "西瓜视频" }

func (x *xiguaParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "ixigua.com")
}

func (x *xiguaParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	videoID, err := x.extractVideoID(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("西瓜视频: %w", err)
	}

	shareURL := fmt.Sprintf("https://m.ixigua.com/douyin/share/video/%s?aweme_type=107&schema_type=1&utm_source=copy&utm_campaign=client_share&utm_medium=android&app=aweme", videoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shareURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("西瓜视频: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 2*1024*1024)
	if err != nil {
		return nil, err
	}

	matches := routerDataRE.FindSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("西瓜视频: 页面中未找到 _ROUTER_DATA")
	}

	var routerData struct {
		LoaderData struct {
			VideoPage struct {
				VideoInfoRes struct {
					ItemList   []douyinItem `json:"item_list"`
					FilterList []struct {
						DetailMsg string `json:"detail_msg"`
					} `json:"filter_list"`
				} `json:"videoInfoRes"`
			} `json:"video_(id)/page"`
		} `json:"loaderData"`
	}
	if err := json.Unmarshal(matches[1], &routerData); err != nil {
		return nil, fmt.Errorf("西瓜视频: 解析失败: %w", err)
	}

	items := routerData.LoaderData.VideoPage.VideoInfoRes.ItemList
	if len(items) == 0 {
		return nil, fmt.Errorf("西瓜视频: 无视频信息")
	}
	item := items[0]
	videoURL := ""
	if len(item.Video.PlayAddr.URLList) > 0 {
		videoURL = strings.ReplaceAll(item.Video.PlayAddr.URLList[0], "playwm", "play")
	}
	coverURL := ""
	if len(item.Video.Cover.URLList) > 0 {
		coverURL = item.Video.Cover.URLList[0]
	}

	return &VideoInfo{
		Platform: "xigua",
		VideoURL: videoURL,
		CoverURL: coverURL,
		Title:    item.Desc,
		Author:   item.Author.Nickname,
	}, nil
}

func (x *xiguaParser) extractVideoID(ctx context.Context, rawURL string) (string, error) {
	if strings.Contains(rawURL, "www.ixigua.com") {
		parts := strings.Split(strings.Trim(strings.Split(rawURL, "?")[0], "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

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
