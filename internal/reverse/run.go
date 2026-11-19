package reverse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
)

// Result is the files produced by a reverse run.
type Result struct {
	OutDir  string
	Cast    string
	Shots   string
	Remake  string
	Report  string
	Summary string
	LogPath string
}

func fileIfExists(dir, name string) string {
	p := filepath.Join(dir, name)
	if st, err := os.Stat(p); err == nil && st.Size() > 0 {
		return p
	}
	return ""
}

func readFile(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func collectResult(outDir, logPath, summary string) *Result {
	return &Result{
		OutDir:  outDir,
		Cast:    fileIfExists(outDir, "CAST.md"),
		Shots:   fileIfExists(outDir, "SHOTS.md"),
		Remake:  fileIfExists(outDir, "REMAKE.md"),
		Report:  fileIfExists(outDir, "REPORT.md"),
		Summary: summary,
		LogPath: logPath,
	}
}

func readyForRun(ctx context.Context, cfg *config.Config, progress func(int, string)) (Status, string, error) {
	if strings.TrimSpace(cfg.Replicate.APIKey) == "" {
		return Status{}, "", fmt.Errorf("请先在「设置」里填写视觉模型的 API Key（提示词提取 / 视频逆向共用）")
	}
	st := Probe(ctx)
	if !st.Installed {
		progress(2, "正在安装 Pi Agent…")
		if err := Install(ctx, func(s string) { progress(3, s) }); err != nil {
			return Status{}, "", err
		}
		st = Probe(ctx)
	}
	if !st.Installed {
		return Status{}, "", fmt.Errorf("未找到 Pi Agent")
	}
	agentDir, err := WriteAgentConfig(cfg)
	if err != nil {
		return Status{}, "", fmt.Errorf("写入 Pi 配置失败: %w", err)
	}
	return st, agentDir, nil
}

// Run prepares frames and asks Pi to write the breakdown.
func Run(ctx context.Context, cfg *config.Config, videoPath string, progress func(int, string)) (*Result, error) {
	return runJob(ctx, cfg, videoPath, "", progress)
}

// Resume continues an incomplete run in an existing output folder, skipping frame extraction.
func Resume(ctx context.Context, cfg *config.Config, outDir string, progress func(int, string)) (*Result, error) {
	return runJob(ctx, cfg, "", outDir, progress)
}

func runJob(ctx context.Context, cfg *config.Config, videoPath, outDir string, progress func(int, string)) (*Result, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	st, agentDir, err := readyForRun(ctx, cfg, progress)
	if err != nil {
		return nil, err
	}

	var job *Job
	resuming := strings.TrimSpace(outDir) != ""
	if resuming {
		job, err = LoadJob(outDir)
		if err != nil {
			return nil, err
		}
		progress(40, "从已有抽帧结果继续: "+outDir)
	} else {
		outDir = NewOutDir(videoPath)
		bins := media.FFmpegBins{FFmpeg: cfg.FFmpeg.Path, FFprobe: cfg.FFmpeg.FFprobePath}
		job, err = Prepare(ctx, bins, videoPath, outDir, progress)
		if err != nil {
			return nil, err
		}
	}

	logPath := filepath.Join(job.OutDir, "run.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("创建运行日志失败: %w", err)
	}
	defer logFile.Close()
	var logMu sync.Mutex
	writeLog := func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		line := time.Now().Format("15:04:05") + "  " + strings.TrimRight(s, "\n") + "\n"
		logMu.Lock()
		_, _ = logFile.WriteString(line)
		_ = logFile.Sync()
		logMu.Unlock()
	}

	if resuming {
		writeLog("从已有目录继续拆解，模型: " + ModelID(cfg) + "，输出目录: " + job.OutDir)
	} else {
		writeLog("Pi Agent 开始拆解，模型: " + ModelID(cfg) + "，输出目录: " + job.OutDir)
	}

	summary, err := analyzeInBatches(ctx, cfg, st, agentDir, job, writeLog, progress)
	res := collectResult(job.OutDir, logPath, summary)
	if err != nil {
		if ctx.Err() != nil {
			writeLog("已取消")
			return res, ctx.Err()
		}
		writeLog(err.Error())
		return res, err
	}
	if res.Cast == "" && res.Shots == "" && res.Remake == "" && res.Report == "" {
		writeLog("Pi Agent 已结束，但没有写出拆解文件")
		return res, fmt.Errorf("Pi Agent 已结束，但没有写出拆解文件。请查看运行日志，可点「继续上次」重试成稿")
	}
	writeLog("拆解完成")
	progress(100, "拆解完成")
	return res, nil
}

func analyzeInBatches(ctx context.Context, cfg *config.Config, st Status, agentDir string, job *Job, writeLog func(string), progress func(int, string)) (string, error) {
	n := len(job.Frames)
	if n == 0 {
		return "", fmt.Errorf("没有可分析的关键帧")
	}
	start := currentNoteFrame(job.OutDir, job.Frames)
	if start > n {
		start = n
	}
	if err := ensureNotesFile(job.OutDir); err != nil {
		return "", fmt.Errorf("准备 NOTES.md 失败: %w", err)
	}
	writeLog(fmt.Sprintf("分批观察：共 %d 帧，每批最多 %d 张，已完成 %d 张，模型: %s", n, noteBatchSize, start, ModelID(cfg)))
	progress(42, fmt.Sprintf("分批观察关键帧（%d/%d）…", start, n))

	summary := ""
	for start < n {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		batch := nextBatch(job.Frames, start, noteBatchSize)
		if len(batch) == 0 {
			break
		}
		first := frameFileIndex(batch[0])
		last := frameFileIndex(batch[len(batch)-1])
		msg := fmt.Sprintf("正在观察 frame_%03d–frame_%03d（%d/%d）", first, last, last+1, n)
		progress(batchPercent(start, n), msg)
		writeLog(msg)

		prompt, attach, errPrep := writeBatchFile(job.OutDir, job, batch, start)
		if errPrep != nil {
			return summary, errPrep
		}
		writeLog("本批已写入 BATCH.md，并附加 " + strings.Join(attach, ", "))
		header := batchHeader(batch)
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			if attempt > 1 {
				writeLog(fmt.Sprintf("本批第 %d 次尝试", attempt))
				progress(-1, "本批观察失败，正在重试…")
			}
			var s string
			s, err = runPiSession(ctx, st, agentDir, job.OutDir, prompt, ModelID(cfg), piSessionOpts{
				Attach:   attach,
				Stall:    batchStallTimeout,
				NoTools:  true,
				UseSkill: false,
				System:   observeSystemPrompt,
			}, writeLog, progress)
			if s != "" {
				summary = s
			}
			if sec, ok := extractNotesSection(s, header); ok {
				if aerr := appendNotesSection(job.OutDir, sec); aerr != nil {
					err = fmt.Errorf("写入 NOTES.md 失败: %w", aerr)
				} else {
					writeLog("已由程序把本批观察追加到 NOTES.md")
					err = nil
				}
			}
			now := currentNoteFrame(job.OutDir, job.Frames)
			if now > start {
				start = now
				err = nil
				break
			}
			if err == nil {
				err = fmt.Errorf("本批结束后未在回复或 NOTES.md 中看到可用的 frame_%03d–frame_%03d 观察", first, last)
			} else {
				writeLog("本批未得到完整观察: " + err.Error())
			}
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
		}
		if err != nil {
			saveProgressFile(job.OutDir, progressState{NextFrame: start, BatchSize: noteBatchSize})
			return summary, fmt.Errorf("观察 frame_%03d–frame_%03d 失败: %w。已保存进度，可点「继续上次」从断点接着跑", first, last, err)
		}
		saveProgressFile(job.OutDir, progressState{NextFrame: start, BatchSize: noteBatchSize})
		writeLog(fmt.Sprintf("本批已写入 NOTES.md，进度 %d/%d", start, n))
	}

	if isComplete(job.OutDir) {
		return summary, nil
	}
	if err := writeSkillCopy(job.OutDir); err != nil {
		return summary, fmt.Errorf("写入技能说明失败: %w", err)
	}
	if terr := ensureTranscript(ctx, cfg, job, writeLog, progress); terr != nil {
		writeLog("台词转写未完成，将仅根据画面笔记成稿: " + terr.Error())
		progress(-1, "台词转写未完成，先按画面笔记成稿")
	}
	progress(86, "笔记已齐，开始按文件写出拆解文稿…")
	writeLog("开始分文件写出 CAST.md、SHOTS.md、REMAKE.md、REPORT.md（不调用工具，由程序落盘）")
	s, err := writeDeliverables(ctx, st, agentDir, job, ModelID(cfg), writeLog, progress)
	if s != "" {
		summary = s
	}
	if isComplete(job.OutDir) {
		return summary, nil
	}
	if err == nil {
		err = fmt.Errorf("成稿结束后仍缺少拆解文件")
	}
	return summary, fmt.Errorf("写出拆解文件失败: %w。笔记已保存在 NOTES.md，可点「继续上次」重试成稿", err)
}

func writeDeliverables(ctx context.Context, st Status, agentDir string, job *Job, modelID string, writeLog func(string), progress func(int, string)) (string, error) {
	var summary string
	nShots := len(job.Shots)
	steps := []struct {
		file string
		pct  int
		msg  string
	}{
		{"CAST.md", 87, "正在写 CAST.md（角色与场景）…"},
		{"SHOTS.md", 90, "正在写 SHOTS.md（分镜表）…"},
		{"REMAKE.md", 95, "正在写 REMAKE.md（复刻方案）…"},
		{"REPORT.md", 98, "正在写 REPORT.md（拆解报告）…"},
	}
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if step.file == "SHOTS.md" {
			s, err := writeShotsDeliverable(ctx, st, agentDir, job, modelID, writeLog, progress, step.pct, step.msg)
			if s != "" {
				summary = s
			}
			if err != nil {
				return summary, err
			}
			continue
		}
		if fileIfExists(job.OutDir, step.file) != "" {
			writeLog(step.file + " 已存在，跳过")
			continue
		}
		progress(step.pct, step.msg)
		writeLog(step.msg)
		s, err := runSynthFile(ctx, st, agentDir, job, modelID, step.file, 0, 0, writeLog, progress)
		if s != "" {
			summary = s
		}
		if err != nil {
			return summary, err
		}
	}
	if missing := missingDeliverables(job.OutDir, nShots); len(missing) > 0 {
		return summary, fmt.Errorf("仍缺少 %s", strings.Join(missing, "、"))
	}
	return summary, nil
}

func writeShotsDeliverable(ctx context.Context, st Status, agentDir string, job *Job, modelID string, writeLog func(string), progress func(int, string), pct int, msg string) (string, error) {
	nShots := len(job.Shots)
	if nShots <= 0 {
		nShots = len(job.Frames)
	}
	if shotsLooksComplete(job.OutDir, nShots) {
		writeLog("SHOTS.md 已齐，跳过")
		return "", nil
	}
	progress(pct, msg)
	writeLog(msg)
	var summary string
	for {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		from := nextShotIndex(job.OutDir)
		if from > nShots {
			break
		}
		to := from + shotWriteBatch - 1
		if to > nShots {
			to = nShots
		}
		chunkMsg := fmt.Sprintf("正在写 SHOTS.md 镜头 %d–%d（共 %d）", from, to, nShots)
		progress(shotPercent(from-1, nShots), chunkMsg)
		writeLog(chunkMsg)
		s, err := runSynthFile(ctx, st, agentDir, job, modelID, "SHOTS.md", from, to, writeLog, progress)
		if s != "" {
			summary = s
		}
		if err != nil {
			return summary, err
		}
		if shotsLooksComplete(job.OutDir, nShots) {
			break
		}
		now := nextShotIndex(job.OutDir)
		if now <= from {
			return summary, fmt.Errorf("SHOTS.md 镜头 %d–%d 写入后进度没有前进", from, to)
		}
	}
	if !shotsLooksComplete(job.OutDir, nShots) {
		return summary, fmt.Errorf("SHOTS.md 尚未写全（已有 %d/%d）", nextShotIndex(job.OutDir)-1, nShots)
	}
	return summary, nil
}

func shotPercent(done, total int) int {
	if total <= 0 {
		return 90
	}
	pct := 88 + int(float64(done)/float64(total)*6)
	if pct < 88 {
		pct = 88
	}
	if pct > 94 {
		pct = 94
	}
	return pct
}

func runSynthFile(ctx context.Context, st Status, agentDir string, job *Job, modelID, file string, shotFrom, shotTo int, writeLog func(string), progress func(int, string)) (string, error) {
	task := synthPromptFor(job, file, shotFrom, shotTo)
	if err := writeSynthContext(job.OutDir, task); err != nil {
		return "", fmt.Errorf("写入成稿材料失败: %w", err)
	}
	prompt := synthCLIPrompt(file)
	writeLog("成稿材料已写入 " + contextFileName + "，使用单行提示并附加该文件")
	var last string
	var err error
	beforeShots := 0
	if file == "SHOTS.md" {
		beforeShots = nextShotIndex(job.OutDir)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			writeLog(fmt.Sprintf("%s 第 %d 次尝试", file, attempt))
			progress(-1, file+" 未写全，正在重试…")
		}
		var s string
		s, err = runPiSession(ctx, st, agentDir, job.OutDir, prompt, modelID, piSessionOpts{
			Attach:   synthAttach(job.OutDir),
			Stall:    fileStallTimeout,
			NoTools:  true,
			UseSkill: false,
			System:   synthSystemPrompt,
		}, writeLog, progress)
		if s != "" {
			last = s
		}
		body, ok := extractDeliverable(s, file)
		if !ok {
			if err == nil {
				err = fmt.Errorf("%s 的回复里没有可用正文", file)
			} else {
				writeLog(file + " 未得到可用正文: " + err.Error())
			}
			if ctx.Err() != nil {
				return last, ctx.Err()
			}
			continue
		}
		appendShots := file == "SHOTS.md" && beforeShots > 1
		if werr := saveDeliverable(job.OutDir, file, body, appendShots); werr != nil {
			err = fmt.Errorf("写入 %s 失败: %w", file, werr)
			writeLog(err.Error())
			continue
		}
		if file == "SHOTS.md" {
			now := nextShotIndex(job.OutDir)
			writeLog(fmt.Sprintf("已由程序把分镜追加到 SHOTS.md，进度 %d/%d", now-1, len(job.Shots)))
			if now > beforeShots {
				return last, nil
			}
			err = fmt.Errorf("SHOTS.md 写入后镜头数没有增加")
			continue
		}
		writeLog("已由程序把回复保存为 " + file)
		return last, nil
	}
	if err == nil {
		err = fmt.Errorf("写出 %s 失败", file)
	}
	return last, err
}

func batchPercent(done, total int) int {
	if total <= 0 {
		return 42
	}
	pct := 42 + int(float64(done)/float64(total)*43)
	if pct > 85 {
		pct = 85
	}
	if pct < 42 {
		pct = 42
	}
	return pct
}

type piSessionOpts struct {
	Attach       []string
	Stall        time.Duration
	NoTools      bool
	UseSkill     bool
	System       string
	AppendSystem []string
}

func runPiSession(ctx context.Context, st Status, agentDir, outDir, prompt, modelID string, opts piSessionOpts, writeLog func(string), progress func(int, string)) (string, error) {
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	prompt = oneLine(prompt)
	args := []string{
		"--mode", "json",
		"-p", prompt,
		"--provider", "narrato",
		"--model", modelID,
		"--no-session",
		"--thinking", "off",
		"--no-skills",
		"--no-extensions",
		"--no-context-files",
	}
	if opts.System != "" {
		args = append(args, "--system-prompt", oneLine(opts.System))
	}
	if opts.UseSkill {
		args = append(args, "--skill", filepath.Join(agentDir, "skills", "video-reverse"))
	}
	for _, p := range opts.AppendSystem {
		if strings.TrimSpace(p) != "" {
			args = append(args, "--append-system-prompt", oneLine(p))
		}
	}
	if opts.NoTools {
		args = append(args, "--no-tools")
	} else {
		args = append(args, "--tools", "read,edit,write,grep,find,ls")
	}
	for _, f := range opts.Attach {
		if strings.TrimSpace(f) != "" {
			args = append(args, "@"+f)
		}
	}
	cmd := exec.CommandContext(sessCtx, st.Path, args...)
	hideWindow(cmd)
	cmd.Dir = outDir
	cmd.Env = append(os.Environ(),
		"PI_CODING_AGENT_DIR="+agentDir,
		"PI_TELEMETRY=0",
		"AI_AGENT=pi",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	_ = stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动 Pi Agent 失败: %w", err)
	}
	writeLog("已启动 Pi Agent")

	var lastMu sync.Mutex
	lastEvent := time.Now()
	touch := func() {
		lastMu.Lock()
		lastEvent = time.Now()
		lastMu.Unlock()
	}
	stopHB := make(chan struct{})
	var stallOnce sync.Once
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopHB:
				return
			case <-t.C:
				lastMu.Lock()
				idle := time.Since(lastEvent)
				lastMu.Unlock()
				if idle >= 15*time.Second {
					msg := fmt.Sprintf("仍在运行，已有 %d 秒没有新输出（可能正在等模型回复）", int(idle.Seconds()))
					writeLog(msg)
					progress(-1, msg)
				}
				if opts.Stall > 0 && idle >= opts.Stall {
					stallOnce.Do(func() {
						msg := fmt.Sprintf("已有 %d 秒没有新输出，中断本轮以免一直空等", int(idle.Seconds()))
						writeLog(msg)
						progress(-1, msg)
						cancel()
					})
				}
			}
		}
	}()

	go drainStderr(stderr, func(pct int, msg string) {
		touch()
		writeLog("[stderr] " + msg)
		progress(pct, msg)
	})
	summary := consumeJSON(stdout, func(pct int, msg string) {
		touch()
		writeLog(msg)
		progress(pct, msg)
	})
	waitErr := cmd.Wait()
	close(stopHB)
	if waitErr != nil {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		if sessCtx.Err() != nil && ctx.Err() == nil {
			return summary, fmt.Errorf("等待模型回复超时")
		}
		return summary, fmt.Errorf("Pi Agent 运行失败: %w", waitErr)
	}
	return summary, nil
}

func drainStderr(r io.Reader, progress func(int, string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			progress(-1, line)
		}
	}
}

func consumeJSON(r io.Reader, progress func(int, string)) string {
	dec := json.NewDecoder(r)
	var last, best string
	pct := 50
	consider := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		last = text
		if len([]rune(text)) >= len([]rune(best)) {
			best = text
		}
	}
	for {
		var ev map[string]any
		if err := dec.Decode(&ev); err != nil {
			if err != io.EOF {
				progress(-1, "解析 Pi 输出时出错: "+err.Error())
			}
			break
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "agent_start":
			progress(pct, "模型已接入，开始分析…")
		case "turn_start":
			progress(-1, "开始一轮推理")
		case "tool_execution_start":
			msg := describeTool("Pi 正在调用", ev)
			if pct < 88 {
				pct += 2
			}
			progress(pct, msg)
		case "tool_execution_end":
			name, _ := ev["toolName"].(string)
			if isError, _ := ev["isError"].(bool); isError {
				errMsg := toolErrorText(ev)
				if errMsg != "" {
					progress(-1, "工具失败: "+name+" "+truncate(errMsg, 160))
				} else {
					progress(-1, "工具失败: "+name)
				}
			} else if name != "" {
				progress(-1, "完成: "+name)
			}
		case "message_end", "message_update":
			if text := messageText(ev["message"]); text != "" {
				consider(text)
				if typ == "message_end" {
					progress(-1, truncate(text, 200))
				}
			}
		case "auto_retry_start":
			progress(-1, "模型请求失败，正在重试…")
		case "auto_retry_end":
			if ev["success"] == true {
				progress(-1, "重试成功")
			} else if e, _ := ev["finalError"].(string); e != "" {
				progress(-1, "重试失败: "+truncate(e, 200))
			}
		case "agent_settled", "agent_end":
			progress(-1, "Pi 正在收尾…")
		case "session":
			progress(-1, "Pi 会话已建立")
		default:
			if text := messageText(ev["message"]); text != "" {
				consider(text)
			}
		}
	}
	if best != "" {
		return best
	}
	return last
}

func toolErrorText(ev map[string]any) string {
	for _, key := range []string{"error", "message", "result"} {
		switch v := ev[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ""
}

func describeTool(prefix string, ev map[string]any) string {
	name, _ := ev["toolName"].(string)
	msg := prefix + " " + name
	if args, ok := ev["args"].(map[string]any); ok {
		if c, ok := args["command"].(string); ok && c != "" {
			msg = "Pi 执行命令: " + truncate(c, 120)
		} else if p, ok := args["path"].(string); ok && p != "" {
			msg = "Pi 读写: " + p
		}
	}
	return msg
}

func messageText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if role, _ := m["role"].(string); role != "" && role != "assistant" {
		return ""
	}
	switch c := m["content"].(type) {
	case string:
		return strings.TrimSpace(c)
	case []any:
		var b strings.Builder
		for _, item := range c {
			im, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if im["type"] == "text" {
				if t, ok := im["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func ReadResultFiles(res *Result) (cast, shots, remake, report string) {
	if res == nil {
		return "", "", "", ""
	}
	return readFile(res.Cast), readFile(res.Shots), readFile(res.Remake), readFile(res.Report)
}
