package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type douyinParser struct{}

func (d *douyinParser) Name() string { return "抖音" }

func (d *douyinParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "douyin.com") ||
		strings.Contains(rawURL, "iesdouyin.com")
}

var routerDataRE = regexp.MustCompile(`window\._ROUTER_DATA\s*=\s*(.*?)</script>`)

func (d *douyinParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	videoID, err := d.extractVideoID(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("抖音: 提取视频ID失败: %w", err)
	}
	return d.parseWithBase(ctx, videoID, "https://www.iesdouyin.com")
}

func (d *douyinParser) parseWithBase(ctx context.Context, videoID, baseURL string) (*VideoInfo, error) {
	shareURL := fmt.Sprintf("%s/share/video/%s/", baseURL, videoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shareURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抖音: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("抖音: HTTP %d", resp.StatusCode)
	}

	body, err := readBodyLimit(resp.Body, 2*1024*1024)
	if err != nil {
		return nil, err
	}

	matches := routerDataRE.FindSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("抖音: 页面中未找到 _ROUTER_DATA")
	}

	var routerData map[string]json.RawMessage
	if err := json.Unmarshal(matches[1], &routerData); err != nil {
		return nil, fmt.Errorf("抖音: 解析 _ROUTER_DATA 失败: %w", err)
	}

	loaderRaw, ok := routerData["loaderData"]
	if !ok {
		return nil, fmt.Errorf("抖音: _ROUTER_DATA 中无 loaderData")
	}
	var loaderData map[string]json.RawMessage
	if err := json.Unmarshal(loaderRaw, &loaderData); err != nil {
		return nil, fmt.Errorf("抖音: loaderData 解析失败: %w", err)
	}

	var pageRaw json.RawMessage
	for _, key := range []string{"video_(id)/page", "note_(id)/page"} {
		if raw, found := loaderData[key]; found {
			pageRaw = raw
			break
		}
	}
	if pageRaw == nil {
		return nil, fmt.Errorf("抖音: loaderData 中无 video/note page 数据")
	}

	var page struct {
		VideoInfoRes struct {
			ItemList   []douyinItem `json:"item_list"`
			FilterList []struct {
				DetailMsg string `json:"detail_msg"`
			} `json:"filter_list"`
		} `json:"videoInfoRes"`
	}
	if err := json.Unmarshal(pageRaw, &page); err != nil {
		return nil, fmt.Errorf("抖音: page 数据解析失败: %w", err)
	}

	if len(page.VideoInfoRes.ItemList) == 0 {
		msg := "无视频信息"
		if len(page.VideoInfoRes.FilterList) > 0 && page.VideoInfoRes.FilterList[0].DetailMsg != "" {
			msg = page.VideoInfoRes.FilterList[0].DetailMsg
		}
		return nil, fmt.Errorf("抖音: %s", msg)
	}

	item := page.VideoInfoRes.ItemList[0]

	var images []string
	if item.Images != nil {
		for _, img := range item.Images {
			if len(img.URLList) > 0 && img.URLList[0] != "" {
				images = append(images, img.URLList[0])
			}
		}
	}

	videoURL := ""
	if len(item.Video.PlayAddr.URLList) > 0 {
		videoURL = strings.ReplaceAll(item.Video.PlayAddr.URLList[0], "playwm", "play")
	}
	if len(images) > 0 {
		videoURL = ""
	}

	if videoURL != "" {
		redirected, err := d.resolveRedirect(ctx, videoURL)
		if err == nil && redirected != "" {
			videoURL = redirected
		}
	}

	coverURL := ""
	if len(item.Video.Cover.URLList) > 0 {
		coverURL = item.Video.Cover.URLList[0]
	}

	authorName := ""
	if item.Author.Nickname != "" {
		authorName = item.Author.Nickname
	}

	return &VideoInfo{
		Platform: "douyin",
		VideoURL: videoURL,
		CoverURL: coverURL,
		Title:    item.Desc,
		Author:   authorName,
		Images:   images,
	}, nil
}

func (d *douyinParser) extractVideoID(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// User page with modal_id: the modal_id IS the video ID.
	if mid := u.Query().Get("modal_id"); mid != "" {
		return mid, nil
	}

	// Direct video URL: https://www.douyin.com/video/XXXXX
	if strings.Contains(u.Path, "/video/") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i, p := range parts {
			if p == "video" && i+1 < len(parts) {
				return parts[i+1], nil
			}
		}
	}

	// Note URL: https://www.douyin.com/note/XXXXX
	if strings.Contains(u.Path, "/note/") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i, p := range parts {
			if p == "note" && i+1 < len(parts) {
				return parts[i+1], nil
			}
		}
	}

	// Share short link: https://v.douyin.com/XXXXX → follow redirect to get ID
	if strings.Contains(rawURL, "v.douyin.com") || strings.Contains(rawURL, "iesdouyin.com") {
		return d.resolveShareLink(ctx, rawURL)
	}

	// iesdouyin.com/share/video/ID
	if strings.Contains(u.Path, "/share/video/") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i, p := range parts {
			if p == "video" && i+1 < len(parts) {
				return parts[i+1], nil
			}
		}
	}

	return "", fmt.Errorf("无法从链接提取视频ID: %s", rawURL)
}

func (d *douyinParser) resolveShareLink(ctx context.Context, shareURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shareURL, nil)
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

	u, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1], nil
	}
	return "", fmt.Errorf("重定向地址中无视频ID")
}

func (d *douyinParser) resolveRedirect(ctx context.Context, videoURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
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
	if loc != "" {
		return loc, nil
	}
	return videoURL, nil
}

type douyinItem struct {
	Desc   string `json:"desc"`
	Video  struct {
		PlayAddr struct {
			URLList []string `json:"url_list"`
		} `json:"play_addr"`
		Cover struct {
			URLList []string `json:"url_list"`
		} `json:"cover"`
	} `json:"video"`
	Images []struct {
		URLList []string `json:"url_list"`
	} `json:"images"`
	Author struct {
		SecUID    string `json:"sec_uid"`
		Nickname  string `json:"nickname"`
		AvatarThumb struct {
			URLList []string `json:"url_list"`
		} `json:"avatar_thumb"`
	} `json:"author"`
}
