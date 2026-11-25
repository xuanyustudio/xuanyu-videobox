package webui

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/pipeline"
	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/replicate"
	"github.com/narratoai/narrato-go/internal/script"
	"github.com/narratoai/narrato-go/internal/tts"
	"github.com/narratoai/narrato-go/internal/voiceclone"
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

	cancel context.CancelFunc

	mmVoices []voiceclone.VoiceInfo
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
	mux.HandleFunc("/api/render/cancel", s.handleRenderCancel)
	mux.HandleFunc("/api/render/open", s.handleRenderOpen)
	mux.HandleFunc("/api/tts/preview", s.handleTTSPreview)
	mux.HandleFunc("/api/tts/preview/stop", s.handleTTSPreviewStop)
	mux.HandleFunc("/api/tts/voices", s.handleTTSVoices)
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
	fmt.Fprintf(os.Stdout, "玄宇视频工具箱已启动：%s\n请在浏览器中使用（关闭本窗口将停止服务）。\n", addr)

	go func() {
		time.Sleep(400 * time.Millisecond)
		_ = openBrowser(addr)
		infoBox("玄宇视频工具箱", "界面已在默认浏览器打开：\n"+addr+"\n\n请保持本程序运行；关闭托盘/后台进程后页面将不可用。")
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
		Cfg            *config.Config   `json:"cfg"`
		CfgPath        string           `json:"cfg_path"`
		ProjectsRoot   string           `json:"projects_root"`
		Project        *project.Project `json:"project"`
		EstimatedSec   float64          `json:"estimated_sec"`
		EstimatedClock string           `json:"estimated_clock"`
		TargetSec      int              `json:"target_sec"`
		OverTarget     bool             `json:"over_target"`
		Busy           bool             `json:"busy"`
		Progress       int              `json:"progress"`
		LastOutput     string           `json:"last_output"`
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
	if s.cfg == nil {
		s.cfg = config.Default()
	}
	mergeWebSettings(s.cfg, &body)
	if s.cfg.Paths.ProjectsRoot != "" {
		s.projectsRoot = s.cfg.Paths.ProjectsRoot
	}
	path := s.cfgPath
	cfg := s.cfg
	s.mu.Unlock()
	if err := config.Save(path, cfg); err != nil {
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
	if len(s.proj.VideoPaths) == 0 {
		s.mu.Unlock()
		http.Error(w, "请先添加视频", 400)
		return
	}
	subPaths := append([]string(nil), s.proj.SubtitlePaths...)
	videoPaths := append([]string(nil), s.proj.VideoPaths...)
	cfg := *s.cfg
	s.mu.Unlock()

	ffmpegBin, err := media.ResolveFFmpeg(cfg.FFmpeg.Path, "")
	if err != nil {
		http.Error(w, "找不到 ffmpeg: "+err.Error(), 500)
		return
	}
	ffprobeBin, err := media.ResolveFFprobe(cfg.FFmpeg.FFprobePath, filepath.Dir(ffmpegBin))
	if err != nil {
		http.Error(w, "找不到 ffprobe: "+err.Error(), 500)
		return
	}
	spans, err := media.ProbeSourceTimeline(r.Context(), media.FFmpegBins{FFmpeg: ffmpegBin, FFprobe: ffprobeBin}, videoPaths)
	if err != nil {
		http.Error(w, "无法读取片源时长: "+err.Error(), 500)
		return
	}
	subText, err := script.ComposeSubtitleText(subPaths, spans)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if strings.TrimSpace(subText) == "" {
		http.Error(w, "请先添加或识别字幕", 400)
		return
	}
	client := script.NewLLM(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)
	items, err := client.GenerateScript(r.Context(), script.GenerateInput{
		SubtitleText:  subText,
		WordCount:     body.WordCount,
		OriginalRatio: body.OriginalRatio,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = script.NormalizeItemsSource(items, spans)
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
	if r.Body != nil && r.ContentLength != 0 {
		var body config.Config
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			mergeWebSettings(s.cfg, &body)
			_ = config.Save(s.cfgPath, s.cfg)
		}
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
	s.mu.Unlock()

	synth, err := pipeline.NewSynthesizerFromConfig(&cfg)
	if err != nil {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
		http.Error(w, err.Error(), 400)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.cancel = nil
			s.mu.Unlock()
		}()
		taskID := time.Now().Format("20060102-150405")
		workDir := filepath.Join(root, slug, "tasks", taskID)
		outPath := filepath.Join(workDir, "final.mp4")
		s.appendLog("任务 " + taskID)
		s.appendLog("引擎 " + tts.NormalizeEngine(cfg.TTS.Engine) + " 音色 " + cfg.TTS.Voice)

		params := pipeline.Params{
			Items:      items,
			VideoPaths: videos,
			WorkDir:    workDir,
			OutputPath: outPath,
			TTS:        synth,
			Voice:      cfg.TTS.Voice,
			OnProgress: func(step string, percent int, msg string) {
				s.mu.Lock()
				s.progress = percent
				s.mu.Unlock()
				s.appendLog(fmt.Sprintf("[%d%%] %s %s", percent, step, msg))
			},
		}
		pipeline.ApplyConfig(&params, &cfg)
		out, err := pipeline.Run(ctx, params)
		if err != nil {
			if ctx.Err() != nil {
				s.appendLog("已取消")
				return
			}
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

func (s *server) handleRenderCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	fn := s.cancel
	s.mu.Unlock()
	if fn != nil {
		fn()
		s.appendLog("已请求取消…")
	}
	writeJSON(w, map[string]string{"ok": "1"})
}

func (s *server) handleRenderOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	out := s.lastOutput
	s.mu.Unlock()
	if out == "" {
		http.Error(w, "还没有合成完成的成片", 400)
		return
	}
	if err := media.OpenFile(out); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"ok": "1", "path": out})
}

func (s *server) handleTTSPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	cfg := *s.cfg
	var items []script.Item
	if s.proj != nil {
		items = s.proj.ScriptItems
	}
	s.mu.Unlock()
	sample := strings.TrimSpace(body.Text)
	if sample == "" {
		sample = tts.PreviewText(items, 80)
	} else {
		sample = tts.TruncateRunes(sample, 80)
	}
	if path, ok, err := tts.DirectPreviewPath(cfg.TTS.Engine, cfg.TTS.Voice, cfg.TTS.IndexTTS2Reference); err != nil {
		http.Error(w, err.Error(), 400)
		return
	} else if ok {
		if err := s.playPreviewAudio(path); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		source := "official"
		if tts.IndexTTS2IDForLabel(cfg.TTS.Voice) == tts.IndexTTS2CloneID {
			source = "reference"
		}
		writeJSON(w, map[string]string{"ok": "1", "text": sample, "path": path, "source": source})
		return
	}

	dir := filepath.Join(os.TempDir(), "xuanyu-voice-preview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	previewFor := tts.PreviewTimeout
	if tts.NormalizeEngine(cfg.TTS.Engine) == tts.EngineIndexTTS2 {
		previewFor = tts.IndexTTS2PreviewTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), previewFor)
	defer cancel()

	officialURL := ""
	if tts.NormalizeEngine(cfg.TTS.Engine) == tts.EngineMiniMax {
		s.mu.Lock()
		cached := append([]voiceclone.VoiceInfo(nil), s.mmVoices...)
		s.mu.Unlock()
		officialURL = previewURLForVoice(cached, cfg.TTS.Voice)
		if officialURL == "" {
			client := &voiceclone.Client{BaseURL: cfg.MiniMax.BaseURL, APIKey: cfg.MiniMax.APIKey, Model: cfg.MiniMax.Model}
			if voices, err := client.ListVoices(ctx, voiceclone.VoiceTypeAll); err == nil {
				s.mu.Lock()
				s.mmVoices = append([]voiceclone.VoiceInfo(nil), voices...)
				s.mu.Unlock()
				officialURL = previewURLForVoice(voices, cfg.TTS.Voice)
			}
		}
	}
	if officialURL != "" {
		out := filepath.Join(dir, officialPreviewFileName(officialURL, cfg.TTS.Voice))
		if st, err := os.Stat(out); err != nil || st.Size() < 2048 {
			httpClient := &http.Client{Timeout: tts.PreviewTimeout}
			if err := voiceclone.DownloadDemo(ctx, httpClient, officialURL, out); err != nil {
				http.Error(w, "下载官方预听音频失败: "+err.Error(), 500)
				return
			}
		}
		if err := s.playPreviewAudio(out); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"ok": "1", "text": sample, "path": out, "source": "official"})
		return
	}

	synth, err := pipeline.NewSynthesizerFromConfig(&cfg)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ext := ".mp3"
	switch tts.NormalizeEngine(cfg.TTS.Engine) {
	case tts.EngineKokoro, tts.EngineOpenVoice, tts.EngineIndexTTS2:
		ext = ".wav"
	}
	out := filepath.Join(dir, fmt.Sprintf("preview_%d%s", time.Now().UnixNano(), ext))
	if _, err := synth.Synthesize(ctx, sample, cfg.TTS.Voice, out); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.playPreviewAudio(out); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"ok": "1", "text": sample, "path": out, "source": "synthesize"})
}

func (s *server) handleTTSPreviewStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	media.StopAudio()
	writeJSON(w, map[string]string{"ok": "1"})
}

func (s *server) playPreviewAudio(path string) error {
	ffmpeg := ""
	if s != nil && s.cfg != nil {
		ffmpeg = s.cfg.FFmpeg.Path
	}
	return media.PlayAudioConfigured(path, ffmpeg)
}

func (s *server) handleTTSVoices(w http.ResponseWriter, r *http.Request) {
	engine := tts.NormalizeEngine(r.URL.Query().Get("engine"))
	type localItem struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	writeLocal := func(list []tts.VoiceOption) {
		out := make([]localItem, 0, len(list))
		for _, v := range list {
			lab := v.Label
			if lab == "" {
				lab = v.ID
			}
			out = append(out, localItem{ID: v.ID, Label: lab})
		}
		writeJSON(w, out)
	}
	if engine == tts.EngineKokoro {
		s.mu.Lock()
		cfg := *s.cfg
		s.mu.Unlock()
		list, _ := tts.ListKokoroVoices(r.Context(), cfg.TTS.KokoroURL)
		if len(list) == 0 {
			list = tts.BuiltinKokoroVoices()
		}
		writeLocal(list)
		return
	}
	if engine == tts.EngineOpenVoice {
		s.mu.Lock()
		cfg := *s.cfg
		s.mu.Unlock()
		list, _ := tts.ListOpenVoiceVoices(r.Context(), cfg.TTS.OpenVoiceURL)
		if len(list) == 0 {
			list = tts.BuiltinOpenVoiceVoices()
		}
		writeLocal(list)
		return
	}
	if engine == tts.EngineIndexTTS2 {
		s.mu.Lock()
		cfg := *s.cfg
		s.mu.Unlock()
		list, _ := tts.ListIndexTTS2Voices(r.Context(), cfg.TTS.IndexTTS2URL)
		list = tts.RankedIndexTTS2Voices(list)
		type item struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			Rating int    `json:"rating,omitempty"`
		}
		out := make([]item, 0, len(list))
		for _, v := range list {
			lab := v.Label
			if lab == "" {
				lab = v.ID
			}
			out = append(out, item{ID: v.ID, Label: lab, Rating: tts.IndexTTS2Recommendation(v.ID)})
		}
		writeJSON(w, out)
		return
	}
	if engine != tts.EngineMiniMax {
		type item struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			Rating int    `json:"rating,omitempty"`
		}
		out := make([]item, 0)
		for _, v := range tts.RankedEdgeVoices() {
			out = append(out, item{ID: v.ID, Label: v.Label, Rating: tts.EdgeRecommendation(v.ID)})
		}
		writeJSON(w, out)
		return
	}
	s.mu.Lock()
	cfg := *s.cfg
	s.mu.Unlock()
	client := &voiceclone.Client{BaseURL: cfg.MiniMax.BaseURL, APIKey: cfg.MiniMax.APIKey, Model: cfg.MiniMax.Model}
	voices, err := client.ListVoices(r.Context(), voiceclone.VoiceTypeAll)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	s.mmVoices = append([]voiceclone.VoiceInfo(nil), voices...)
	s.mu.Unlock()
	type item struct {
		ID         string `json:"id"`
		Label      string `json:"label"`
		Kind       string `json:"kind"`
		Rating     int    `json:"rating,omitempty"`
		PreviewURL string `json:"preview_url,omitempty"`
	}
	sorted := append([]voiceclone.VoiceInfo(nil), voices...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return tts.LessMiniMax(sorted[i].ID, sorted[i].Kind, sorted[j].ID, sorted[j].Kind)
	})
	out := make([]item, 0, len(sorted))
	for _, v := range sorted {
		level := tts.MiniMaxVoiceLevel(v.ID, v.Kind)
		out = append(out, item{ID: v.ID, Label: tts.WithRatingMark(v.Display(), level), Kind: v.Kind, Rating: level, PreviewURL: v.PreviewURL()})
	}
	writeJSON(w, out)
}

func mergeWebSettings(dst, src *config.Config) {
	if dst == nil || src == nil {
		return
	}
	dst.LLM = src.LLM
	if src.TTS.Voice != "" {
		dst.TTS.Voice = src.TTS.Voice
	}
	if src.TTS.Engine != "" {
		dst.TTS.Engine = src.TTS.Engine
	}
	if src.TTS.KokoroURL != "" {
		dst.TTS.KokoroURL = src.TTS.KokoroURL
	}
	if src.TTS.OpenVoiceURL != "" {
		dst.TTS.OpenVoiceURL = src.TTS.OpenVoiceURL
	}
	if src.TTS.OpenVoiceURL != "" || src.TTS.OpenVoiceReference != "" || tts.NormalizeEngine(src.TTS.Engine) == tts.EngineOpenVoice {
		dst.TTS.OpenVoiceReference = src.TTS.OpenVoiceReference
	}
	if src.TTS.IndexTTS2URL != "" {
		dst.TTS.IndexTTS2URL = src.TTS.IndexTTS2URL
	}
	if src.TTS.IndexTTS2URL != "" || src.TTS.IndexTTS2Reference != "" || tts.NormalizeEngine(src.TTS.Engine) == tts.EngineIndexTTS2 {
		dst.TTS.IndexTTS2Reference = src.TTS.IndexTTS2Reference
	}
	if src.TTS.Speed != 0 {
		dst.TTS.Speed = src.TTS.Speed
	}
	if src.TTS.Volume != 0 {
		dst.TTS.Volume = src.TTS.Volume
	}
	dst.TTS.Pitch = src.TTS.Pitch
	if src.FFmpeg.Path != "" || src.FFmpeg.FFprobePath != "" {
		if src.FFmpeg.Path != "" {
			dst.FFmpeg.Path = src.FFmpeg.Path
		}
		if src.FFmpeg.FFprobePath != "" {
			dst.FFmpeg.FFprobePath = src.FFmpeg.FFprobePath
		}
	}
	dst.UI.TargetDurationSec = src.UI.TargetDurationSec
	dst.UI.SubtitleHideOnOriginal = src.UI.SubtitleHideOnOriginal
	// Overlay extended UI/TTS fields when the form provided them.
	if src.UI.AspectRatio != "" {
		dst.UI.AspectRatio = src.UI.AspectRatio
	}
	if src.UI.Resolution != "" {
		dst.UI.Resolution = src.UI.Resolution
	}
	if src.UI.SubtitleFont != "" {
		dst.UI.SubtitleFont = src.UI.SubtitleFont
	}
	if src.UI.SubtitleFontSize != 0 {
		dst.UI.SubtitleFontSize = src.UI.SubtitleFontSize
	}
	if src.UI.SubtitleColor != "" {
		dst.UI.SubtitleColor = src.UI.SubtitleColor
	}
	if src.UI.SubtitlePosition != "" {
		dst.UI.SubtitlePosition = src.UI.SubtitlePosition
	}
	// Only apply these booleans when the payload is a render form (has aspect/font).
	if src.UI.SubtitleFont != "" || src.UI.AspectRatio != "" || src.UI.Resolution != "" || src.UI.BGMSource != "" {
		dst.UI.SubtitleBurnEnabled = src.UI.SubtitleBurnEnabled
		dst.UI.SubtitleMaskEnabled = src.UI.SubtitleMaskEnabled
		dst.UI.SubtitleOnMask = src.UI.SubtitleOnMask
	}
	if src.UI.SubtitleMaskY != 0 {
		dst.UI.SubtitleMaskY = src.UI.SubtitleMaskY
	}
	if src.UI.SubtitleMaskHeight != 0 {
		dst.UI.SubtitleMaskHeight = src.UI.SubtitleMaskHeight
	}
	if src.UI.SubtitleMaskX != 0 {
		dst.UI.SubtitleMaskX = src.UI.SubtitleMaskX
	}
	if src.UI.SubtitleMaskWidth != 0 {
		dst.UI.SubtitleMaskWidth = src.UI.SubtitleMaskWidth
	}
	if src.UI.SubtitleMaskBlur != 0 {
		dst.UI.SubtitleMaskBlur = src.UI.SubtitleMaskBlur
	}
	if src.UI.SubtitleMaskOpacity != 0 {
		dst.UI.SubtitleMaskOpacity = src.UI.SubtitleMaskOpacity
	}
	if src.UI.VoiceVolume != 0 {
		dst.UI.VoiceVolume = src.UI.VoiceVolume
	}
	if src.UI.OriginalVolume != 0 {
		dst.UI.OriginalVolume = src.UI.OriginalVolume
	}
	if src.UI.BGMSource != "" {
		dst.UI.BGMSource = src.UI.BGMSource
	}
	dst.UI.BGMPath = src.UI.BGMPath
	if src.UI.BGMVolume != 0 {
		dst.UI.BGMVolume = src.UI.BGMVolume
	}
	if src.UI.BGMLibraryDir != "" {
		dst.UI.BGMLibraryDir = src.UI.BGMLibraryDir
	}
	if src.UI.SFXPrompt != "" {
		dst.UI.SFXPrompt = src.UI.SFXPrompt
	}
	if src.UI.SFXVolume != 0 {
		dst.UI.SFXVolume = src.UI.SFXVolume
	}
	if src.Paths.ProjectsRoot != "" {
		dst.Paths.ProjectsRoot = src.Paths.ProjectsRoot
	}
	if src.Replicate.BaseURL != "" || src.Replicate.APIKey != "" || src.Replicate.Model != "" {
		if src.Replicate.BaseURL != "" {
			dst.Replicate.BaseURL = src.Replicate.BaseURL
		}
		if src.Replicate.APIKey != "" {
			dst.Replicate.APIKey = src.Replicate.APIKey
		}
		if src.Replicate.Model != "" {
			dst.Replicate.Model = src.Replicate.Model
		}
		if src.Replicate.Frames != 0 {
			dst.Replicate.Frames = src.Replicate.Frames
		}
		if src.Replicate.SegmentSec != 0 {
			dst.Replicate.SegmentSec = src.Replicate.SegmentSec
		}
	}
	if src.MiniMax.BaseURL != "" || src.MiniMax.APIKey != "" || src.MiniMax.Model != "" {
		if src.MiniMax.BaseURL != "" {
			dst.MiniMax.BaseURL = src.MiniMax.BaseURL
		}
		if src.MiniMax.APIKey != "" {
			dst.MiniMax.APIKey = src.MiniMax.APIKey
		}
		if src.MiniMax.Model != "" {
			dst.MiniMax.Model = src.MiniMax.Model
		}
	}
	if src.Sonilo.BaseURL != "" || src.Sonilo.APIKey != "" || src.Sonilo.BGMPrompt != "" {
		if src.Sonilo.BaseURL != "" {
			dst.Sonilo.BaseURL = src.Sonilo.BaseURL
		}
		if src.Sonilo.APIKey != "" {
			dst.Sonilo.APIKey = src.Sonilo.APIKey
		}
		if src.Sonilo.BGMPrompt != "" {
			dst.Sonilo.BGMPrompt = src.Sonilo.BGMPrompt
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func previewURLForVoice(voices []voiceclone.VoiceInfo, id string) string {
	id = strings.TrimSpace(id)
	for _, v := range voices {
		if v.ID == id {
			return v.PreviewURL()
		}
	}
	return ""
}

func officialPreviewFileName(url, voiceID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(voiceID) + "\n" + strings.TrimSpace(url)))
	ext := ".mp3"
	u := strings.ToLower(url)
	switch {
	case strings.Contains(u, ".wav"):
		ext = ".wav"
	case strings.Contains(u, ".m4a"):
		ext = ".m4a"
	case strings.Contains(u, ".ogg"):
		ext = ".ogg"
	}
	return "official_" + hex.EncodeToString(sum[:8]) + ext
}
