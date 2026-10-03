package linkdl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://v.douyin.com/abc123/", "https://v.douyin.com/abc123/"},
		{"复制打开抖音 https://v.douyin.com/abc123/ 看看", "https://v.douyin.com/abc123/"},
		{"no url here", ""},
		{"https://www.douyin.com/video/123456?a=1", "https://www.douyin.com/video/123456?a=1"},
	}
	for _, tt := range tests {
		got := ExtractURL(tt.input)
		if got != tt.want {
			t.Errorf("ExtractURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello/world", "hello_world"},
		{"a:b*c?d", "a_b_c_d"},
		{"正常标题", "正常标题"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDouyinParserMatch(t *testing.T) {
	p := &douyinParser{}
	if !p.Match("https://www.douyin.com/video/123") {
		t.Error("should match douyin.com")
	}
	if !p.Match("https://v.douyin.com/abc/") {
		t.Error("should match v.douyin.com")
	}
	if !p.Match("https://www.iesdouyin.com/share/video/123/") {
		t.Error("should match iesdouyin.com")
	}
	if p.Match("https://www.kuaishou.com/abc") {
		t.Error("should not match kuaishou")
	}
}

func TestDouyinParseAPI200(t *testing.T) {
	resp200 := map[string]any{
		"success": true,
		"data": map[string]any{
			"title": "测试抖音视频",
			"media": map[string]any{
				"video": map[string]any{
					"url": "https://example.com/douyin_nowm.mp4",
				},
			},
		},
	}
	var gotKey, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		gotCT = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/parse" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if body["url"] == "" {
			http.Error(w, "missing url", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp200)
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &douyinParser{}
	info, err := p.parseViaAPI(context.Background(), "https://v.douyin.com/abc123/", srv.URL, "test-key-123")
	if err != nil {
		t.Fatalf("parseViaAPI: %v", err)
	}
	if info.Platform != "douyin" {
		t.Errorf("Platform = %q, want douyin", info.Platform)
	}
	if info.Title != "测试抖音视频" {
		t.Errorf("Title = %q, want %q", info.Title, "测试抖音视频")
	}
	if info.VideoURL != "https://example.com/douyin_nowm.mp4" {
		t.Errorf("VideoURL = %q", info.VideoURL)
	}
	if gotKey != "test-key-123" {
		t.Errorf("X-API-Key = %q, want test-key-123", gotKey)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}

func TestDouyinParseAPI200_NestedResult(t *testing.T) {
	resp200 := map[string]any{
		"success": true,
		"data": map[string]any{
			"result": map[string]any{
				"title": "嵌套格式视频",
				"media": map[string]any{
					"video": map[string]any{
						"url": "https://example.com/nested.mp4",
					},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp200)
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &douyinParser{}
	info, err := p.parseViaAPI(context.Background(), "https://www.douyin.com/video/123", srv.URL, "k")
	if err != nil {
		t.Fatalf("parseViaAPI nested: %v", err)
	}
	if info.VideoURL != "https://example.com/nested.mp4" {
		t.Errorf("VideoURL = %q", info.VideoURL)
	}
	if info.Title != "嵌套格式视频" {
		t.Errorf("Title = %q", info.Title)
	}
}

func TestDouyinParseAPI202_Poll(t *testing.T) {
	taskID := "task-abc-123"
	pollCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/parse":
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"task_id": taskID},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/"+taskID:
			pollCount++
			if pollCount < 2 {
				w.WriteHeader(http.StatusAccepted)
				json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"state": "running"},
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"state": "done",
					"result": map[string]any{
						"title": "轮询完成视频",
						"media": map[string]any{
							"video": map[string]any{
								"url": "https://example.com/polled.mp4",
							},
						},
					},
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &douyinParser{}
	info, err := p.parseViaAPI(context.Background(), "https://www.douyin.com/user/X?modal_id=789", srv.URL, "poll-key")
	if err != nil {
		t.Fatalf("parseViaAPI 202 poll: %v", err)
	}
	if info.VideoURL != "https://example.com/polled.mp4" {
		t.Errorf("VideoURL = %q", info.VideoURL)
	}
	if info.Title != "轮询完成视频" {
		t.Errorf("Title = %q", info.Title)
	}
	if pollCount < 2 {
		t.Errorf("expected at least 2 poll rounds, got %d", pollCount)
	}
}

func TestDouyinParseAPI_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "INVALID_URL",
				"message": "无法识别的链接格式",
			},
		})
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &douyinParser{}
	_, err := p.parseViaAPI(context.Background(), "https://douyin.com/bad", srv.URL, "k")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "INVALID_URL") || !strings.Contains(err.Error(), "无法识别的链接格式") {
		t.Errorf("error should contain code and message, got: %v", err)
	}
}

func TestDouyinParseAPI202_TaskFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/parse":
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"task_id": "fail-task"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/fail-task":
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"state": "failed"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &douyinParser{}
	_, err := p.parseViaAPI(context.Background(), "https://douyin.com/x", srv.URL, "k")
	if err == nil {
		t.Fatal("expected error for failed task")
	}
	if !strings.Contains(err.Error(), "任务失败") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKuaishouParserMatch(t *testing.T) {
	p := &kuaishouParser{}
	if !p.Match("https://v.kuaishou.com/abc123") {
		t.Error("should match kuaishou.com")
	}
	if !p.Match("https://m.gifshow.com/abc") {
		t.Error("should match gifshow.com")
	}
}

func TestXiguaParserMatch(t *testing.T) {
	p := &xiguaParser{}
	if !p.Match("https://www.ixigua.com/12345") {
		t.Error("should match ixigua.com")
	}
}

func TestWeiboParserMatch(t *testing.T) {
	p := &weiboParser{}
	if !p.Match("https://h5.video.weibo.com/show/abc") {
		t.Error("should match weibo.com")
	}
	if p.Match("https://weibo.cn/abc") {
		t.Error("should not match weibo.cn (lvzhou)")
	}
}

func TestHaokanParserMatch(t *testing.T) {
	p := &haokanParser{}
	if !p.Match("https://haokan.baidu.com/v?vid=123") {
		t.Error("should match haokan.baidu.com")
	}
}

func TestHaokanParseWithFixture(t *testing.T) {
	response := map[string]any{
		"errno": 0,
		"data": map[string]any{
			"apiData": map[string]any{
				"curVideoMeta": map[string]any{
					"playurl": "https://example.com/video.mp4",
					"poster":  "https://example.com/cover.jpg",
					"title":   "好看测试",
					"mth": map[string]any{
						"author_name":  "作者",
						"author_photo": "https://example.com/avatar.jpg",
					},
				},
			},
		},
	}
	respJSON, _ := json.Marshal(response)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &haokanParser{}
	info, err := p.parseWithBase(context.Background(), "123", srv.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.Title != "好看测试" {
		t.Errorf("Title = %q, want %q", info.Title, "好看测试")
	}
	if info.VideoURL != "https://example.com/video.mp4" {
		t.Errorf("VideoURL = %q", info.VideoURL)
	}
}

func TestPipixiaParserMatch(t *testing.T) {
	p := &pipixiaParser{}
	if !p.Match("https://h5.pipix.com/item/123") {
		t.Error("should match pipix.com")
	}
}

func TestLispinParserMatch(t *testing.T) {
	p := &lispinParser{}
	if !p.Match("https://www.pearvideo.com/detail_123") {
		t.Error("should match pearvideo.com")
	}
}

func TestPipigaoxiaoParserMatch(t *testing.T) {
	p := &pipigaoxiaoParser{}
	if !p.Match("https://h5.pipigx.com/pp/post/123") {
		t.Error("should match pipigx.com")
	}
}

func TestDownload(t *testing.T) {
	content := []byte("fake video content for testing")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	outDir := t.TempDir()
	info := &VideoInfo{
		Platform: "test",
		VideoURL: srv.URL + "/video.mp4",
		Title:    "测试下载",
	}

	var lastProgress Progress
	path, err := Download(context.Background(), info, outDir, func(p Progress) {
		lastProgress = p
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !strings.HasSuffix(path, ".mp4") {
		t.Errorf("path %q should end with .mp4", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(content) {
		t.Error("downloaded content mismatch")
	}
	if lastProgress.Percent != 100 {
		t.Errorf("final progress = %d, want 100", lastProgress.Percent)
	}
}

func TestDownloadEmptyVideoURL(t *testing.T) {
	info := &VideoInfo{Platform: "test", Title: "no video"}
	_, err := Download(context.Background(), info, t.TempDir(), nil)
	if err == nil {
		t.Error("expected error for empty video URL")
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "test.mp4")
	if got := uniquePath(base); got != base {
		t.Errorf("uniquePath should return original when file doesn't exist, got %q", got)
	}
	os.WriteFile(base, []byte("x"), 0o644)
	got := uniquePath(base)
	if got == base {
		t.Error("uniquePath should return different path when file exists")
	}
	if !strings.Contains(got, "_1") {
		t.Errorf("expected _1 suffix, got %q", got)
	}
}

func TestDetectAndParse_Unsupported(t *testing.T) {
	_, err := DetectAndParse(context.Background(), "https://unknown-site.com/video")
	if err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Errorf("expected unsupported error, got %v", err)
	}
}

func TestDetectAndParse_Empty(t *testing.T) {
	_, err := DetectAndParse(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParsers(t *testing.T) {
	ps := Parsers()
	if len(ps) == 0 {
		t.Error("expected at least one parser")
	}
	names := map[string]bool{}
	for _, p := range ps {
		if names[p.Name()] {
			t.Errorf("duplicate parser name: %s", p.Name())
		}
		names[p.Name()] = true
	}
}

func TestConfigureDouyin(t *testing.T) {
	origBase, origKey := douyinCfg()
	defer ConfigureDouyin(origBase, origKey)

	ConfigureDouyin("http://localhost:9999", "my-key")
	base, key := douyinCfg()
	if base != "http://localhost:9999" {
		t.Errorf("base = %q", base)
	}
	if key != "my-key" {
		t.Errorf("key = %q", key)
	}
}
