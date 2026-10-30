package linkdl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchDouyinAuthorPosts_Pagination(t *testing.T) {
	page1 := map[string]any{
		"success": true,
		"data": map[string]any{
			"items": []any{
				map[string]any{
					"kind":       "video",
					"title":      "第一集\n标题",
					"content_id": "111",
					"created_at": "2026-10-23T14:19:43Z",
					"web_url":    "https://www.douyin.com/video/111",
					"author":     map[string]any{"nickname": "测试作者", "sec_uid": "MS4wsec"},
					"media":      map[string]any{"video": map[string]any{"url": "https://cdn.example/1.mp4", "watermark": false}},
				},
				map[string]any{
					"kind":        "image_album",
					"title":       "",
					"description": "图集说明",
					"content_id":  "222",
					"web_url":     "https://www.douyin.com/note/222",
				},
				map[string]any{
					"kind":       "video",
					"title":      "重复",
					"content_id": "111",
				},
			},
			"cursor":   "1792765183000",
			"has_more": true,
		},
	}
	page2 := map[string]any{
		"success": true,
		"data": map[string]any{
			"items": []any{
				map[string]any{
					"kind":       "video",
					"title":      "第二页",
					"content_id": "333",
					"web_url":    "https://www.douyin.com/video/333",
					"author":     map[string]any{"nickname": "测试作者"},
					"media":      map[string]any{"video": map[string]any{"url": "https://cdn.example/3.mp4", "watermark": false}},
				},
			},
			"cursor":   "",
			"has_more": false,
		},
	}

	var mu sync.Mutex
	var cursors []string
	var cursorPresent []bool
	var gotKey, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/douyin/user/posts" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		mu.Lock()
		cursors = append(cursors, q.Get("cursor"))
		_, present := q["cursor"]
		cursorPresent = append(cursorPresent, present)
		gotKey = r.Header.Get("X-API-Key")
		gotURL = q.Get("url")
		mu.Unlock()
		if q.Get("count") != "50" {
			t.Errorf("count = %q, want 50", q.Get("count"))
		}
		if q.Get("wait") != "25" {
			t.Errorf("wait = %q, want 25", q.Get("wait"))
		}
		w.Header().Set("Content-Type", "application/json")
		if q.Get("cursor") == "" {
			json.NewEncoder(w).Encode(page1)
			return
		}
		if q.Get("cursor") != "1792765183000" {
			t.Errorf("cursor = %q", q.Get("cursor"))
		}
		json.NewEncoder(w).Encode(page2)
	}))
	defer srv.Close()

	orig := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = orig }()

	profile := "复制打开抖音 https://www.douyin.com/user/abc123/ 看看"
	first, err := fetchDouyinAuthorPosts(context.Background(), profile, "", srv.URL, "posts-key")
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if gotKey != "posts-key" {
		t.Errorf("X-API-Key = %q", gotKey)
	}
	if gotURL != "https://www.douyin.com/user/abc123/" {
		t.Errorf("url = %q", gotURL)
	}
	if first.Author != "测试作者" || first.SecUID != "MS4wsec" {
		t.Errorf("author = %q sec = %q", first.Author, first.SecUID)
	}
	if !first.HasMore || first.Cursor != "1792765183000" {
		t.Errorf("cursor = %q hasMore = %v", first.Cursor, first.HasMore)
	}
	if len(first.Items) != 2 {
		t.Fatalf("items = %d, want 2 (duplicate dropped)", len(first.Items))
	}
	if first.Items[0].Title != "第一集 标题" || first.Items[0].VideoURL != "https://cdn.example/1.mp4" {
		t.Errorf("item0 = %+v", first.Items[0])
	}
	wantTime := time.Date(2026, 10, 23, 14, 19, 43, 0, time.UTC)
	if !first.Items[0].CreatedAt.Equal(wantTime) {
		t.Errorf("created = %s", first.Items[0].CreatedAt)
	}
	if first.Items[1].Kind != "image_album" || first.Items[1].Title != "图集说明" || first.Items[1].VideoURL != "" {
		t.Errorf("album = %+v", first.Items[1])
	}

	second, err := fetchDouyinAuthorPosts(context.Background(), profile, first.Cursor, srv.URL, "posts-key")
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second.HasMore || second.Cursor != "" {
		t.Errorf("second cursor = %q hasMore = %v", second.Cursor, second.HasMore)
	}
	if len(second.Items) != 1 || second.Items[0].ContentID != "333" {
		t.Fatalf("second items = %+v", second.Items)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(cursorPresent) != 2 || cursorPresent[0] || !cursorPresent[1] {
		t.Fatalf("cursor presence = %v", cursorPresent)
	}
	if cursors[1] != "1792765183000" {
		t.Errorf("sent cursor = %q", cursors[1])
	}
}

func TestFetchDouyinAuthorPosts_ErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"data":    nil,
			"error": map[string]any{
				"code":    "AUTHOR_NOT_FOUND",
				"message": "找不到该作者",
			},
		})
	}))
	defer srv.Close()

	orig := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = orig }()

	_, err := fetchDouyinAuthorPosts(context.Background(), "https://www.douyin.com/user/missing", "", srv.URL, "k")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AUTHOR_NOT_FOUND") || !strings.Contains(err.Error(), "找不到该作者") {
		t.Errorf("error = %v", err)
	}
}

func TestFetchDouyinAuthorPosts_ErrorEnvelope200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"data":    nil,
			"error": map[string]any{
				"code":    "INVALID_URL",
				"message": "无法识别的主页",
			},
		})
	}))
	defer srv.Close()

	orig := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = orig }()

	_, err := fetchDouyinAuthorPosts(context.Background(), "https://v.douyin.com/short/", "", srv.URL, "k")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "INVALID_URL") || !strings.Contains(err.Error(), "无法识别的主页") {
		t.Errorf("error = %v", err)
	}
}

func TestFetchDouyinAuthorPosts_PollAndFailedTask(t *testing.T) {
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/douyin/user/posts":
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"task_id": "posts-task", "state": "queued"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/posts-task":
			polls++
			if polls == 1 {
				w.WriteHeader(http.StatusAccepted)
				json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"state": "running"},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"state": "done",
					"result": map[string]any{
						"items": []any{
							map[string]any{
								"kind":       "video",
								"title":      "轮询作品",
								"content_id": "777",
								"web_url":    "https://www.douyin.com/video/777",
								"media":      map[string]any{"video": map[string]any{"url": "https://cdn.example/polled.mp4"}},
							},
						},
						"cursor":   "next-cursor",
						"has_more": true,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = orig }()

	page, err := fetchDouyinAuthorPosts(context.Background(), "https://www.douyin.com/user/polled", "", srv.URL, "k")
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].VideoURL != "https://cdn.example/polled.mp4" {
		t.Fatalf("page = %+v", page.Items)
	}
	if page.Cursor != "next-cursor" || !page.HasMore {
		t.Errorf("cursor = %q hasMore = %v", page.Cursor, page.HasMore)
	}
	if polls < 2 {
		t.Errorf("polls = %d, want at least 2", polls)
	}

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/douyin/user/posts":
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"task_id": "fail-posts", "state": "queued"},
			})
		case r.URL.Path == "/api/v1/tasks/fail-posts":
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"state": "failed",
					"error": map[string]any{
						"code":    "UPSTREAM_RISK_CONTROL",
						"message": "被风控",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer failSrv.Close()
	defaultClient = failSrv.Client()

	_, err = fetchDouyinAuthorPosts(context.Background(), "https://www.douyin.com/user/risk", "", failSrv.URL, "k")
	if err == nil {
		t.Fatal("expected failed task")
	}
	for _, part := range []string{"任务失败", "UPSTREAM_RISK_CONTROL", "被风控"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %v missing %s", err, part)
		}
	}
}

func TestFetchDouyinAuthorPosts_NotAPostList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"title": "单条视频",
				"media": map[string]any{"video": map[string]any{"url": "https://cdn.example/one.mp4"}},
			},
		})
	}))
	defer srv.Close()

	orig := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = orig }()

	_, err := fetchDouyinAuthorPosts(context.Background(), "https://www.douyin.com/user/x", "", srv.URL, "k")
	if err == nil || !strings.Contains(err.Error(), "未找到作品列表") {
		t.Fatalf("error = %v", err)
	}
}

func TestFetchDouyinAuthorPosts_RejectsNonProfile(t *testing.T) {
	_, err := fetchDouyinAuthorPosts(context.Background(), "https://www.kuaishou.com/profile/abc", "", "http://example", "k")
	if err == nil || !strings.Contains(err.Error(), "仅支持抖音") {
		t.Fatalf("kuaishou error = %v", err)
	}
	_, err = fetchDouyinAuthorPosts(context.Background(), "https://www.douyin.com/video/123", "", "http://example", "k")
	if err == nil || !strings.Contains(err.Error(), "单条作品") {
		t.Fatalf("video error = %v", err)
	}
	_, err = fetchDouyinAuthorPosts(context.Background(), "   ", "", "http://example", "k")
	if err == nil || !strings.Contains(err.Error(), "请输入作者主页") {
		t.Fatalf("empty error = %v", err)
	}
	_, err = fetchDouyinAuthorPosts(context.Background(), "https://www.douyin.com/user/abc", "", "", "k")
	if err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("config error = %v", err)
	}
}

func TestDecodeAuthorPosts_CursorShapes(t *testing.T) {
	page, err := decodeAuthorPosts([]byte(`{"items":[],"cursor":"","has_more":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("empty cursor should not report another page")
	}

	page, err = decodeAuthorPosts([]byte(`{"items":[{"content_id":"9","title":"n"}],"cursor":1792765183000,"has_more":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if page.Cursor != "1792765183000" || !page.HasMore {
		t.Fatalf("numeric cursor = %q hasMore = %v", page.Cursor, page.HasMore)
	}

	page, err = decodeAuthorPosts([]byte(`{"result":{"items":[{"content_id":"8","title":"nested"}],"cursor":{"next":"from-obj"},"has_more":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if page.Cursor != "from-obj" || len(page.Items) != 1 || page.Items[0].ContentID != "8" {
		t.Fatalf("nested = %+v", page)
	}
}

func TestVideoInfoForPost_UsesDirectURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("direct url should not call %s", r.URL.Path)
	}))
	defer srv.Close()

	orig := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = orig }()

	info, err := VideoInfoForPost(context.Background(), AuthorPost{
		Title:    "直链作品",
		Author:   "作者",
		VideoURL: "https://cdn.example/direct.mp4",
		WebURL:   "https://www.douyin.com/video/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.Platform != "douyin" || info.VideoURL != "https://cdn.example/direct.mp4" || info.Title != "直链作品" {
		t.Fatalf("info = %+v", info)
	}
}

func TestVideoInfoForPost_FallbackParse(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/parse" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		gotURL = body["url"]
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"title": "解析标题",
				"media": map[string]any{"video": map[string]any{"url": "https://cdn.example/parsed.mp4"}},
			},
		})
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()
	origBase, origKey := douyinCfg()
	ConfigureDouyin(srv.URL, "parse-key")
	defer ConfigureDouyin(origBase, origKey)

	info, err := VideoInfoForPost(context.Background(), AuthorPost{
		Title:     "清单标题",
		Author:    "清单作者",
		VideoURL:  "https://cdn.example/watermark.mp4",
		Watermark: true,
		WebURL:    "https://www.douyin.com/video/555",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://www.douyin.com/video/555" {
		t.Errorf("parsed url = %q", gotURL)
	}
	if info.VideoURL != "https://cdn.example/parsed.mp4" || info.Title != "清单标题" || info.Author != "清单作者" {
		t.Fatalf("info = %+v", info)
	}

	gotURL = ""
	info, err = VideoInfoForPost(context.Background(), AuthorPost{
		ContentID: "999",
		Title:     "只有编号",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://www.douyin.com/video/999" {
		t.Errorf("aweme fallback = %q", gotURL)
	}
	if info.VideoURL != "https://cdn.example/parsed.mp4" {
		t.Errorf("info = %+v", info)
	}

	_, err = VideoInfoForPost(context.Background(), AuthorPost{Title: "无处可去"})
	if err == nil || !strings.Contains(err.Error(), "无法定位") {
		t.Fatalf("missing target error = %v", err)
	}
}

func TestFetchDouyinAuthorPosts_UsesConfiguredClient(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"items":    []any{},
				"cursor":   "",
				"has_more": false,
			},
		})
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()
	origBase, origKey := douyinCfg()
	ConfigureDouyin(srv.URL, "configured-key")
	defer ConfigureDouyin(origBase, origKey)

	page, err := FetchDouyinAuthorPosts(context.Background(), "https://www.iesdouyin.com/share/user/abc", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "configured-key" {
		t.Errorf("key = %q", gotKey)
	}
	if page.HasMore || len(page.Items) != 0 {
		t.Fatalf("page = %+v", page)
	}
}
