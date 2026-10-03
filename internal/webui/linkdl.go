package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/narratoai/narrato-go/internal/linkdl"
)

func (s *server) handleLinkDLRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		URLs   []string `json:"urls"`
		OutDir string   `json:"out_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(body.URLs) == 0 {
		http.Error(w, "请输入至少一个链接", 400)
		return
	}

	s.mu.Lock()
	if s.linkDLBusy {
		s.mu.Unlock()
		http.Error(w, "正在下载中", 409)
		return
	}
	s.linkDLBusy = true
	outDir := strings.TrimSpace(body.OutDir)
	if outDir == "" {
		outDir = s.projectsRoot
	}
	s.mu.Unlock()

	type result struct {
		URL      string `json:"url"`
		Platform string `json:"platform,omitempty"`
		Title    string `json:"title,omitempty"`
		Author   string `json:"author,omitempty"`
		FilePath string `json:"file_path,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	results := make([]result, 0, len(body.URLs))
	ctx := r.Context()

	for _, rawURL := range body.URLs {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		res := result{URL: rawURL}

		info, err := linkdl.DetectAndParse(ctx, rawURL)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			s.appendLog(fmt.Sprintf("[链接下载] 解析失败: %s — %s", rawURL, err))
			continue
		}
		res.Platform = info.Platform
		res.Title = info.Title
		res.Author = info.Author

		if info.VideoURL == "" {
			res.Error = "无视频地址（图集）"
			results = append(results, res)
			continue
		}

		path, err := linkdl.Download(ctx, info, outDir, func(p linkdl.Progress) {
			s.appendLog(fmt.Sprintf("[链接下载] %s: %d%%", info.Title, p.Percent))
		})
		if err != nil {
			res.Error = "下载失败: " + err.Error()
			results = append(results, res)
			s.appendLog(fmt.Sprintf("[链接下载] 下载失败: %s — %s", rawURL, err))
			continue
		}
		res.FilePath = path
		results = append(results, res)
		s.appendLog(fmt.Sprintf("[链接下载] 已保存: %s", path))
	}

	s.mu.Lock()
	s.linkDLBusy = false
	s.mu.Unlock()

	writeJSON(w, map[string]any{"results": results})
}
