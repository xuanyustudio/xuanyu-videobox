package webui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/pipeline"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/replicate"
	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/tts"
)

//go:embed index.html
var indexHTML string

type server struct {
	mu           sync.Mutex
	cfg          *config.Config
	cfgPath      string
	projectsRoot string
	proj         *project.Project
	logs         []string
	progress     int
	busy         bool
	lastOutput   string

	replicating   bool
	replicateLast *replicate.Result

	linkDLBusy bool
}

// DefaultConfigPath prefers config.yaml beside the executable.
func DefaultConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(beside); err == nil {
			return beside
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(dir, "narrato-go", "config.yaml")
}

// Run starts a local HTTP UI and opens the default browser. Blocks until process exit.
func Run() {
	cfgPath := DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.Default()
	}
	root := cfg.Paths.ProjectsRoot
	if root == "" {
		root = config.Default().Paths.ProjectsRoot
	}
	_ = os.MkdirAll(root, 0o755)
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0o755)

	linkdl.ConfigureDouyin(cfg.Douyin.BaseURL, cfg.Douyin.APIKey)

	s := &server{cfg: cfg, cfgPath: cfgPath, projectsRoot: root}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/save_settings", s.handleSaveSettings)
	mux.HandleFunc("/api/project/create", s.handleProjectCreate)
	mux.HandleFunc("/api/project/open", s.handleProjectOpen)
	mux.HandleFunc("/api/project/save", s.handleProjectSave)
	mux.HandleFunc("/api/project/list", s.handleProjectList)
	mux.HandleFunc("/api/assets", s.handleAssets)
	mux.HandleFunc("/api/script/generate", s.handleScriptGenerate)
	mux.HandleFunc("/api/script/set", s.handleScriptSet)
	mux.HandleFunc("/api/render", s.handleRender)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/replicate/run", s.handleReplicateRun)
	mux.HandleFunc("/api/replicate/frame", s.handleReplicateFrame)
	mux.HandleFunc("/api/replicate/open", s.handleReplicateOpen)
	mux.HandleFunc("/api/linkdl/run", s.handleLinkDLRun)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalBox("无法监听本地端口: " + err.Error())
		os.Exit(1)
	}
	addr := "http://" + ln.Addr().String()
	fmt.Fprintf(os.Stdout, "Narrato 已启动：%s\n请在浏览器中使用（关闭本窗口将停止服务）。\n", addr)

	go func() {
		time.Sleep(400 * time.Millisecond)
		_ = openBrowser(addr)
		infoBox("Narrato", "界面已在默认浏览器打开：\n"+addr+"\n\n请保持本程序运行；关闭托盘/后台进程后页面将不可用。")
	}()

	srv := &http.Server{Handler: mux}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fatalBox("服务异常: " + err.Error())
		os.Exit(1)
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, indexHTML)
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type resp struct {
		Cfg            *config.Config  `json:"cfg"`
		CfgPath        string          `json:"cfg_path"`
		ProjectsRoot   string          `json:"projects_root"`
		Project        *project.Project `json:"project"`
		EstimatedSec   float64         `json:"estimated_sec"`
		EstimatedClock string          `json:"estimated_clock"`
		TargetSec      int             `json:"target_sec"`
		OverTarget     bool            `json:"over_target"`
		Busy           bool            `json:"busy"`
		Progress       int             `json:"progress"`
		LastOutput     string          `json:"last_output"`
	}
	out := resp{
		Cfg:          s.cfg,
		CfgPath:      s.cfgPath,
		ProjectsRoot: s.projectsRoot,
		Project:      s.proj,
		TargetSec:    s.cfg.UI.TargetDurationSec,
		Busy:         s.busy,
		Progress:     s.progress,
		LastOutput:   s.lastOutput,
	}
	if s.proj != nil {
		out.EstimatedSec = script.EstimateOutputDurationSec(s.proj.ScriptItems)
		out.EstimatedClock = script.FormatDurationClock(out.EstimatedSec)
		out.OverTarget = out.EstimatedSec > float64(out.TargetSec)+0.5
	}
	writeJSON(w, out)
}

func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, map[string]any{"logs": s.logs, "progress": s.progress, "busy": s.busy, "last_output": s.lastOutput})
}

func (s *server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body config.Config
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	s.cfg = &body
	if body.Paths.ProjectsRoot != "" {
		s.projectsRoot = body.Paths.ProjectsRoot
	}
	path := s.cfgPath
	s.mu.Unlock()
	if err := config.Save(path, &body); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"ok": "1"})
}

func (s *server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	root := s.projectsRoot
	s.mu.Unlock()
	list, err := project.List(root)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, list)
}

func (s *server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	root := s.projectsRoot
	s.mu.Unlock()
	p, err := project.Create(root, body.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	s.proj = p
	s.mu.Unlock()
	writeJSON(w, p)
}

func (s *server) handleProjectOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	root := s.projectsRoot
	s.mu.Unlock()
	p, err := project.Load(root, body.Slug)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	s.proj = p
	s.mu.Unlock()
	writeJSON(w, p)
}

func (s *server) handleProjectSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj == nil {
		http.Error(w, "无工程", 400)
		return
	}
	if err := project.Save(s.projectsRoot, s.proj); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"ok": "1"})
}

func (s *server) handleAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Videos    []string `json:"videos"`
		Subtitles []string `json:"subtitles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj == nil {
		http.Error(w, "请先创建或打开工程", 400)
		return
	}
	if body.Videos != nil {
		s.proj.VideoPaths = body.Videos
	}
	if body.Subtitles != nil {
		s.proj.SubtitlePaths = body.Subtitles
	}
	writeJSON(w, s.proj)
}

func (s *server) handleScriptSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Items []script.Item `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj == nil {
		http.Error(w, "请先创建或打开工程", 400)
		return
	}
	s.proj.ScriptItems = body.Items
	writeJSON(w, map[string]any{"count": len(body.Items)})
}

func (s *server) handleScriptGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		WordCount     int `json:"word_count"`
		OriginalRatio int `json:"original_ratio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	if s.proj == nil {
		s.mu.Unlock()
		http.Error(w, "请先创建或打开工程", 400)
		return
	}
	if len(s.proj.SubtitlePaths) == 0 {
		s.mu.Unlock()
		http.Error(w, "请先添加字幕文件路径", 400)
		return
	}
	subPath := s.proj.SubtitlePaths[0]
	cfg := *s.cfg
	s.mu.Unlock()

	data, err := os.ReadFile(subPath)
	if err != nil {
		http.Error(w, "读取字幕失败: "+err.Error(), 500)
		return
	}
	client := script.NewLLM(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)
	items, err := client.GenerateScript(r.Context(), script.GenerateInput{
		SubtitleText:  string(data),
		WordCount:     body.WordCount,
		OriginalRatio: body.OriginalRatio,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	s.proj.ScriptItems = items
	s.mu.Unlock()
	writeJSON(w, items)
}

func (s *server) handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		http.Error(w, "正在合成中", 409)
		return
	}
	if s.proj == nil || len(s.proj.ScriptItems) == 0 || len(s.proj.VideoPaths) == 0 {
		s.mu.Unlock()
		http.Error(w, "需要工程、视频和脚本", 400)
		return
	}
	s.busy = true
	s.progress = 0
	s.logs = nil
	s.lastOutput = ""
	items := append([]script.Item(nil), s.proj.ScriptItems...)
	videos := append([]string(nil), s.proj.VideoPaths...)
	slug := s.proj.Slug
	root := s.projectsRoot
	cfg := *s.cfg
	hide := cfg.UI.SubtitleHideOnOriginal
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}()
		taskID := time.Now().Format("20060102-150405")
		workDir := filepath.Join(root, slug, "tasks", taskID)
		outPath := filepath.Join(workDir, "final.mp4")
		s.appendLog("任务 " + taskID)

		synth := tts.Synthesizer(tts.Edge{})
		params := pipeline.Params{
			Items:             items,
			VideoPaths:        videos,
			WorkDir:           workDir,
			OutputPath:        outPath,
			TTS:               synth,
			Voice:             cfg.TTS.Voice,
			HideOnOriginal:    &hide,
			OnProgress: func(step string, percent int, msg string) {
				s.mu.Lock()
				s.progress = percent
				s.mu.Unlock()
				s.appendLog(fmt.Sprintf("[%d%%] %s %s", percent, step, msg))
			},
		}
		out, err := pipeline.Run(context.Background(), params)
		if err != nil {
			s.appendLog("Edge TTS/合成失败，改用 Fake 重试: " + err.Error())
			params.TTS = tts.Fake{}
			out, err = pipeline.Run(context.Background(), params)
		}
		if err != nil {
			s.appendLog("合成失败: " + err.Error())
			return
		}
		s.mu.Lock()
		s.lastOutput = out
		s.progress = 100
		s.mu.Unlock()
		s.appendLog("完成: " + out)
	}()

	writeJSON(w, map[string]string{"ok": "1"})
}

func (s *server) appendLog(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, time.Now().Format("15:04:05")+" "+line)
	if len(s.logs) > 500 {
		s.logs = s.logs[len(s.logs)-500:]
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
