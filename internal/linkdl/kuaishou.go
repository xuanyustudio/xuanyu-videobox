package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type kuaishouParser struct{}

func (k *kuaishouParser) Name() string { return "快手" }

func (k *kuaishouParser) Match(rawURL string) bool {
	return strings.Contains(rawURL, "kuaishou.com") ||
		strings.Contains(rawURL, "gifshow.com")
}

func (k *kuaishouParser) Parse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	videoID, cookies, err := k.resolveShare(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("快手: %w", err)
	}

	postBody := fmt.Sprintf(`{"fid":"0","shareResourceType":"PHOTO_OTHER","shareChannel":"share_copylink","kpn":"KUAISHOU","subBiz":"BROWSE_SLIDE_PHOTO","env":"SHARE_VIEWER_ENV_TX_TRICK","h5Domain":"m.gifshow.com","photoId":"%s","isLongVideo":false}`, videoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://m.gifshow.com/rest/wd/photo/info?kpn=KUAISHOU&captchaToken=",
		strings.NewReader(postBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://m.gifshow.com")
	req.Header.Set("Referer", rawURL)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("快手: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyLimit(resp.Body, 1*1024*1024)
	if err != nil {
		return nil, err
	}

	var data struct {
		Result int `json:"result"`
		Photo  struct {
			Caption     string `json:"caption"`
			UserName    string `json:"userName"`
			HeadURL     string `json:"headUrl"`
			CoverURLs   []struct{ URL string `json:"url"` } `json:"coverUrls"`
			MainMvURLs  []struct{ URL string `json:"url"` } `json:"mainMvUrls"`
		} `json:"photo"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("快手: 解析JSON失败: %w", err)
	}
	if data.Result != 1 {
		return nil, fmt.Errorf("快手: 接口返回 result=%d", data.Result)
	}

	videoURL := ""
	if len(data.Photo.MainMvURLs) > 0 {
		videoURL = data.Photo.MainMvURLs[0].URL
	}
	coverURL := ""
	if len(data.Photo.CoverURLs) > 0 {
		coverURL = data.Photo.CoverURLs[0].URL
	}

	return &VideoInfo{
		Platform: "kuaishou",
		VideoURL: videoURL,
		CoverURL: coverURL,
		Title:    data.Photo.Caption,
		Author:   data.Photo.UserName,
	}, nil
}

func (k *kuaishouParser) resolveShare(ctx context.Context, rawURL string) (string, []*http.Cookie, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://v.kuaishou.com/")

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", nil, fmt.Errorf("未获取到重定向地址")
	}

	parts := strings.Split(strings.Split(loc, "?")[0], "/")
	videoID := parts[len(parts)-1]
	if videoID == "" {
		return "", nil, fmt.Errorf("重定向地址中无视频ID")
	}
	return videoID, resp.Cookies(), nil
}
