package linkdl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// VideoInfo holds parsed metadata for a single video.
type VideoInfo struct {
	Platform string   `json:"platform"`
	VideoURL string   `json:"video_url"`
	CoverURL string   `json:"cover_url"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	Images   []string `json:"images,omitempty"`
}

// Progress reports download state to the caller.
type Progress struct {
	URL        string
	Percent    int
	BytesDone  int64
	BytesTotal int64
	Err        error
	FilePath   string
}

// Parser can resolve a share/page URL into VideoInfo.
type Parser interface {
	Name() string
	Match(rawURL string) bool
	Parse(ctx context.Context, rawURL string) (*VideoInfo, error)
}

var defaultClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// downloadClient is used for video body transfers; 30s is too short for large files.
var downloadClient = &http.Client{
	Timeout: 30 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

var noRedirectClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

var parsers []Parser

func init() {
	parsers = []Parser{
		&douyinParser{},
		&kuaishouParser{},
		&xiguaParser{},
		&weiboParser{},
		&pipixiaParser{},
		&haokanParser{},
		&pipigaoxiaoParser{},
		&lispinParser{},
	}
}

// Parsers returns all registered parsers (useful for tests/display).
func Parsers() []Parser { return parsers }

// DetectAndParse identifies the platform from a URL and parses it.
func DetectAndParse(ctx context.Context, rawURL string) (*VideoInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("空链接")
	}
	rawURL = ExtractURL(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("未找到有效链接")
	}
	for _, p := range parsers {
		if p.Match(rawURL) {
			return p.Parse(ctx, rawURL)
		}
	}
	return nil, fmt.Errorf("不支持的链接: %s", rawURL)
}

var urlRE = regexp.MustCompile(`https?://[^\s<>"']+`)

// ExtractURL pulls the first HTTP(S) URL out of arbitrary text (e.g. share messages).
func ExtractURL(text string) string {
	m := urlRE.FindString(text)
	return strings.TrimRight(m, ".,;!?)")
}

// Download fetches the video from info.VideoURL and saves it to outDir.
// It calls onProgress (if non-nil) periodically.
func Download(ctx context.Context, info *VideoInfo, outDir string, onProgress func(Progress)) (string, error) {
	if info.VideoURL == "" {
		return "", fmt.Errorf("无视频地址（可能是图集）")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.VideoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", info.VideoURL)

	resp, err := downloadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回 %d", resp.StatusCode)
	}

	filename := sanitizeFilename(info.Title)
	if filename == "" {
		filename = fmt.Sprintf("%s_%d", info.Platform, time.Now().Unix())
	}
	ext := ".mp4"
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "video/") {
		if strings.Contains(ct, "mp4") {
			ext = ".mp4"
		}
	}
	outPath := filepath.Join(outDir, filename+ext)
	outPath = uniquePath(outPath)

	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	total := resp.ContentLength
	var done int64
	buf := make([]byte, 64*1024)
	lastReport := time.Time{}
	for {
		select {
		case <-ctx.Done():
			_ = f.Close()
			_ = os.Remove(outPath)
			return "", ctx.Err()
		default:
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return "", wErr
			}
			done += int64(n)
			if onProgress != nil && time.Since(lastReport) > 200*time.Millisecond {
				pct := 0
				if total > 0 {
					pct = int(done * 100 / total)
				}
				onProgress(Progress{URL: info.VideoURL, Percent: pct, BytesDone: done, BytesTotal: total})
				lastReport = time.Now()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return "", readErr
		}
	}
	if onProgress != nil {
		onProgress(Progress{URL: info.VideoURL, Percent: 100, BytesDone: done, BytesTotal: total, FilePath: outPath})
	}
	return outPath, nil
}

const userAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"

func sanitizeFilename(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return '_'
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}


// ExistingDownload returns the planned .mp4 path for info.Title and whether a
// non-empty file already exists there (used by "skip already downloaded").
func ExistingDownload(outDir string, info *VideoInfo) (string, bool) {
	if info == nil {
		return "", false
	}
	filename := sanitizeFilename(info.Title)
	if filename == "" {
		return "", false
	}
	outPath := filepath.Join(outDir, filename+".mp4")
	st, err := os.Stat(outPath)
	if err != nil || st.Size() <= 0 {
		return outPath, false
	}
	return outPath, true
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; i < 1000; i++ {
		cand := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return p
}

func queryParam(rawURL, key string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}
