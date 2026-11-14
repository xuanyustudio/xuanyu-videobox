package linkdl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const douyinAuthorPostsCount = 50

// AuthorPost is one Douyin item from an author feed.
// VideoURL is the direct media address when the list already included one.
type AuthorPost struct {
	ContentID string
	Title     string
	Author    string
	WebURL    string
	CreatedAt time.Time
	Kind      string
	VideoURL  string
	Watermark bool
}

// AuthorPostsPage is one page of a Douyin author's posts, newest first.
// Pass Cursor back to FetchDouyinAuthorPosts to read the next page.
// HasMore is false on the last page.
type AuthorPostsPage struct {
	Author  string
	SecUID  string
	Items   []AuthorPost
	Cursor  string
	HasMore bool
}

// FetchDouyinAuthorPosts loads one page of posts for a Douyin author homepage.
// profileURL may be the profile link itself or share text that contains one.
// cursor is empty for the first page.
func FetchDouyinAuthorPosts(ctx context.Context, profileURL, cursor string) (*AuthorPostsPage, error) {
	base, key := douyinCfg()
	if base == "" || key == "" {
		return nil, fmt.Errorf("抖音: 未配置 Douyin API（请在设置页填写地址和 Key）")
	}
	return fetchDouyinAuthorPosts(ctx, profileURL, cursor, base, key)
}

func fetchDouyinAuthorPosts(ctx context.Context, profileURL, cursor, apiBase, apiKey string) (*AuthorPostsPage, error) {
	if strings.TrimSpace(apiBase) == "" || strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("抖音: 未配置 Douyin API（请在设置页填写地址和 Key）")
	}
	profileURL, err := normalizeDouyinProfile(profileURL)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("url", profileURL)
	q.Set("count", strconv.Itoa(douyinAuthorPostsCount))
q.Set("wait", "25")
	q.Set("lang", "zh")
	if c := strings.TrimSpace(cursor); c != "" {
		q.Set("cursor", c)
	}
	endpoint := strings.TrimRight(apiBase, "/") + "/api/v1/douyin/user/posts?" + q.Encode()

	raw, err := douyinDo(ctx, http.MethodGet, endpoint, apiBase, apiKey, nil, 8*1024*1024)
	if err != nil {
		return nil, err
	}
	return decodeAuthorPosts(raw)
}

// VideoInfoForPost resolves a playable address for one feed item.
// A direct http(s) URL from the feed is used when it is not watermarked.
// Otherwise the item is parsed through the single-video path (share URL, or
// https://www.douyin.com/video/{content_id} when the feed has no link).
func VideoInfoForPost(ctx context.Context, post AuthorPost) (*VideoInfo, error) {
	if u := directPlayURL(post); u != "" {
		return &VideoInfo{
			Platform: "douyin",
			VideoURL: u,
			Title:    post.Title,
			Author:   post.Author,
		}, nil
	}

	raw := strings.TrimSpace(post.WebURL)
	if raw == "" && strings.TrimSpace(post.ContentID) != "" {
		raw = "https://www.douyin.com/video/" + strings.TrimSpace(post.ContentID)
	}
	if raw == "" {
		return nil, fmt.Errorf("抖音: 无法定位作品")
	}
	info, err := DetectAndParse(ctx, raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(post.Title) != "" {
		info.Title = post.Title
	}
	if strings.TrimSpace(post.Author) != "" {
		info.Author = post.Author
	}
	return info, nil
}

func directPlayURL(post AuthorPost) string {
	if post.Watermark {
		return ""
	}
	raw := strings.TrimSpace(post.VideoURL)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if parsed.Host == "" {
		return ""
	}
	return raw
}

func normalizeDouyinProfile(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("抖音: 请输入作者主页链接")
	}
	extracted := ExtractURL(raw)
	if extracted == "" {
		return "", fmt.Errorf("抖音: 未找到有效链接")
	}
	parsed, err := url.Parse(extracted)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("抖音: 未找到有效链接")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("抖音: 未找到有效链接")
	}
	if !douyinHost(parsed.Hostname()) {
		return "", fmt.Errorf("抖音: 作者主页目前仅支持抖音链接")
	}
	path := strings.ToLower(parsed.EscapedPath())
	if strings.Contains(path, "/video/") || strings.Contains(path, "/note/") {
		return "", fmt.Errorf("抖音: 这是单条作品链接，请粘贴作者主页")
	}
	return extracted, nil
}

func douyinHost(host string) bool {
	host = strings.ToLower(host)
	return host == "douyin.com" || strings.HasSuffix(host, ".douyin.com") ||
		host == "iesdouyin.com" || strings.HasSuffix(host, ".iesdouyin.com")
}

type douyinPostsPayload struct {
	Items   []douyinPostItem `json:"items"`
	Cursor  douyinCursor     `json:"cursor"`
	HasMore bool             `json:"has_more"`
}

type douyinPostItem struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ContentID   string `json:"content_id"`
	CreatedAt   string `json:"created_at"`
	WebURL      string `json:"web_url"`
	Author      struct {
		Nickname string `json:"nickname"`
		SecUID   string `json:"sec_uid"`
	} `json:"author"`
	Media struct {
		Video struct {
			URL       string `json:"url"`
			Watermark bool   `json:"watermark"`
		} `json:"video"`
	} `json:"media"`
}

// douyinCursor accepts the string cursor used by /user/posts, a bare number,
// or the {"next": "..."} object shape the API sometimes puts on meta.
type douyinCursor string

func (c *douyinCursor) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*c = ""
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*c = douyinCursor(s)
	case '{':
		var obj struct {
			Next string `json:"next"`
		}
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		*c = douyinCursor(obj.Next)
	default:
		*c = douyinCursor(string(b))
	}
	return nil
}

func decodeAuthorPosts(raw json.RawMessage) (*AuthorPostsPage, error) {
	page, ok, err := decodePostsOnce(raw)
	if err != nil {
		return nil, err
	}
	if ok {
		return page, nil
	}
	var nested struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &nested); err == nil &&
		len(bytes.TrimSpace(nested.Result)) > 0 &&
		string(bytes.TrimSpace(nested.Result)) != "null" {
		page, ok, err = decodePostsOnce(nested.Result)
		if err != nil {
			return nil, err
		}
		if ok {
			return page, nil
		}
	}
	return nil, fmt.Errorf("抖音: 响应中未找到作品列表")
}

func decodePostsOnce(raw json.RawMessage) (*AuthorPostsPage, bool, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false, fmt.Errorf("抖音: 解析作品列表失败: %w", err)
	}
	if _, ok := probe["items"]; !ok {
		return nil, false, nil
	}
	var payload douyinPostsPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, fmt.Errorf("抖音: 解析作品列表失败: %w", err)
	}
	return mapAuthorPosts(payload), true, nil
}

func mapAuthorPosts(payload douyinPostsPayload) *AuthorPostsPage {
	page := &AuthorPostsPage{
		Items:  make([]AuthorPost, 0, len(payload.Items)),
		Cursor: strings.TrimSpace(string(payload.Cursor)),
	}
	page.HasMore = payload.HasMore && page.Cursor != ""

	seen := map[string]struct{}{}
	for _, item := range payload.Items {
		id := strings.TrimSpace(item.ContentID)
		if id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(item.Description)
		}
		title = strings.Join(strings.Fields(title), " ")
		post := AuthorPost{
			ContentID: id,
			Title:     title,
			Author:    strings.TrimSpace(item.Author.Nickname),
			WebURL:    strings.TrimSpace(item.WebURL),
			Kind:      strings.TrimSpace(item.Kind),
			VideoURL:  strings.TrimSpace(item.Media.Video.URL),
			Watermark: item.Media.Video.Watermark,
		}
		if s := strings.TrimSpace(item.CreatedAt); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				post.CreatedAt = t
			} else if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				post.CreatedAt = t
			}
		}
		if page.Author == "" && post.Author != "" {
			page.Author = post.Author
		}
		if page.SecUID == "" {
			page.SecUID = strings.TrimSpace(item.Author.SecUID)
		}
		page.Items = append(page.Items, post)
	}
	return page
}
