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

func TestDouyinExtractVideoID(t *testing.T) {
	p := &douyinParser{}
	ctx := context.Background()

	tests := []struct {
		url  string
		want string
	}{
		{"https://www.douyin.com/video/7396822576074460467", "7396822576074460467"},
		{"https://www.douyin.com/note/7396822576074460467", "7396822576074460467"},
		{"https://www.douyin.com/user/MS4wLjAB?modal_id=7691653163225993317", "7691653163225993317"},
		{"https://www.iesdouyin.com/share/video/123456/", "123456"},
	}
	for _, tt := range tests {
		got, err := p.extractVideoID(ctx, tt.url)
		if err != nil {
			t.Errorf("extractVideoID(%q): %v", tt.url, err)
			continue
		}
		if got != tt.want {
			t.Errorf("extractVideoID(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestDouyinParseWithFixture(t *testing.T) {
	routerJSON := makeDouyinRouterData("测试视频标题", "测试作者", "https://example.com/video.mp4", "https://example.com/cover.jpg")
	html := fmt.Sprintf(`<html><script>window._ROUTER_DATA = %s</script></html>`, routerJSON)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/share/video/") {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, html)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	origClient := defaultClient
	defaultClient = srv.Client()
	defer func() { defaultClient = origClient }()

	p := &douyinParser{}
	info, err := p.parseWithBase(context.Background(), "12345", srv.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.Title != "测试视频标题" {
		t.Errorf("Title = %q, want %q", info.Title, "测试视频标题")
	}
	if info.Author != "测试作者" {
		t.Errorf("Author = %q, want %q", info.Author, "测试作者")
	}
	if info.Platform != "douyin" {
		t.Errorf("Platform = %q, want %q", info.Platform, "douyin")
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

// makeDouyinRouterData builds a minimal _ROUTER_DATA JSON for testing.
func makeDouyinRouterData(title, author, videoURL, coverURL string) string {
	data := map[string]any{
		"loaderData": map[string]any{
			"video_(id)/page": map[string]any{
				"videoInfoRes": map[string]any{
					"item_list": []map[string]any{
						{
							"desc": title,
							"video": map[string]any{
								"play_addr": map[string]any{
									"url_list": []string{videoURL},
								},
								"cover": map[string]any{
									"url_list": []string{coverURL},
								},
							},
							"author": map[string]any{
								"sec_uid":  "sec123",
								"nickname": author,
								"avatar_thumb": map[string]any{
									"url_list": []string{"https://example.com/avatar.jpg"},
								},
							},
						},
					},
					"filter_list": []any{},
				},
			},
		},
	}
	b, _ := json.Marshal(data)
	return string(b)
}
