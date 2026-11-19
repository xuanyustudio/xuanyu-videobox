package reverse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	notesFileName     = "NOTES.md"
	progressFileName  = "progress.json"
	noteBatchSize     = 8
	batchStallTimeout = 3 * time.Minute
)

type progressState struct {
	NextFrame int    `json:"nextFrame"`
	BatchSize int    `json:"batchSize"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type framesManifest struct {
	Video    string  `json:"video"`
	Duration float64 `json:"duration"`
	Shots    []Shot  `json:"shots"`
	Frames   []Frame `json:"frames"`
}

func outDirPrefix(videoPath string) string {
	base := filepath.Base(videoPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return "reverse_" + stem + "_"
}

func isComplete(outDir string) bool {
	if fileIfExists(outDir, "CAST.md") == "" ||
		fileIfExists(outDir, "SHOTS.md") == "" ||
		fileIfExists(outDir, "REMAKE.md") == "" ||
		fileIfExists(outDir, "REPORT.md") == "" {
		return false
	}
	return shotsLooksComplete(outDir, expectedShotCount(outDir))
}

func countFrameJPGs(outDir string) int {
	entries, err := os.ReadDir(filepath.Join(outDir, "frames"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".png") {
			n++
		}
	}
	return n
}

func isResumable(outDir string) bool {
	if fileIfExists(outDir, "frames.json") == "" {
		return false
	}
	if countFrameJPGs(outDir) == 0 {
		return false
	}
	return !isComplete(outDir)
}

// FindResumable returns the newest incomplete reverse output folder for a video.
func FindResumable(videoPath string) (outDir, summary string, ok bool) {
	videoPath = strings.TrimSpace(videoPath)
	if videoPath == "" {
		return "", "", false
	}
	dir := filepath.Dir(videoPath)
	prefix := outDirPrefix(videoPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false
	}
	var best string
	var bestTime time.Time
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if !isResumable(p) {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		t := st.ModTime()
		if t.After(bestTime) {
			best, bestTime = p, t
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, resumableSummary(best), true
}

func resumableSummary(outDir string) string {
	job, err := LoadJob(outDir)
	nFrames := 0
	if err == nil {
		nFrames = len(job.Frames)
	}
	if nFrames == 0 {
		nFrames = countFrameJPGs(outDir)
	}
	done := 0
	if err == nil {
		done = currentNoteFrame(outDir, job.Frames)
		if done > nFrames {
			done = nFrames
		}
	}
	missing := make([]string, 0, 4)
	for _, name := range []string{"CAST.md", "SHOTS.md", "REMAKE.md", "REPORT.md"} {
		if fileIfExists(outDir, name) == "" {
			missing = append(missing, name)
		}
	}
	if done < nFrames {
		return fmt.Sprintf("发现未完成的拆解（已抽 %d 帧，笔记 %d/%d），可跳过抽帧继续", nFrames, done, nFrames)
	}
	if len(missing) > 0 {
		return fmt.Sprintf("发现未完成的拆解（%d 帧笔记已齐，还缺 %s），可继续出成稿", nFrames, strings.Join(missing, "、"))
	}
	return "发现未完成的拆解，可继续"
}

// LoadJob reads a previously prepared output folder.
func LoadJob(outDir string) (*Job, error) {
	raw, err := os.ReadFile(filepath.Join(outDir, "frames.json"))
	if err != nil {
		return nil, fmt.Errorf("读取 frames.json 失败: %w", err)
	}
	var man framesManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return nil, fmt.Errorf("解析 frames.json 失败: %w", err)
	}
	if len(man.Frames) == 0 {
		return nil, fmt.Errorf("frames.json 里没有关键帧记录")
	}
	return &Job{
		VideoPath: man.Video,
		OutDir:    outDir,
		Duration:  man.Duration,
		Shots:     man.Shots,
		Frames:    man.Frames,
	}, nil
}

func frameFileIndex(f Frame) int {
	base := filepath.Base(f.Path)
	re := regexp.MustCompile(`(?i)frame_(\d+)`)
	if m := re.FindStringSubmatch(base); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err == nil {
			return n
		}
	}
	if f.Index > 0 {
		return f.Index - 1
	}
	return 0
}

var batchHeaderRe = regexp.MustCompile(`(?im)^##\s*批次\s*frame_(\d+)\s*[-–~到至]+\s*frame_(\d+)`)

func inferNextFrameFromNotes(notes string) int {
	matches := batchHeaderRe.FindAllStringSubmatch(notes, -1)
	maxEnd := -1
	for _, m := range matches {
		end, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if end > maxEnd {
			maxEnd = end
		}
	}
	if maxEnd < 0 {
		return 0
	}
	return maxEnd + 1
}

func loadProgressFile(outDir string) progressState {
	var st progressState
	raw, err := os.ReadFile(filepath.Join(outDir, progressFileName))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(raw, &st)
	return st
}

func saveProgressFile(outDir string, st progressState) {
	st.UpdatedAt = time.Now().Format(time.RFC3339)
	if st.BatchSize <= 0 {
		st.BatchSize = noteBatchSize
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(outDir, progressFileName), raw, 0o644)
}

func currentNoteFrame(outDir string, frames []Frame) int {
	fromNotes := 0
	if b, err := os.ReadFile(filepath.Join(outDir, notesFileName)); err == nil {
		fromNotes = inferNextFrameFromNotes(string(b))
	}
	fromFile := loadProgressFile(outDir).NextFrame
	n := fromNotes
	if fromFile > n {
		n = fromFile
	}
	if n < 0 {
		n = 0
	}
	if len(frames) > 0 && n > len(frames) {
		n = len(frames)
	}
	return n
}

func nextBatch(frames []Frame, start, size int) []Frame {
	if start < 0 {
		start = 0
	}
	if start >= len(frames) {
		return nil
	}
	if size <= 0 {
		size = noteBatchSize
	}
	end := start + size
	if end > len(frames) {
		end = len(frames)
	}
	return frames[start:end]
}

func frameLabel(f Frame) string {
	return fmt.Sprintf("%s（镜头 %d，%.2f 秒）", filepath.Base(f.Path), f.Shot, f.Timestamp)
}

func batchHeader(batch []Frame) string {
	if len(batch) == 0 {
		return ""
	}
	first := frameFileIndex(batch[0])
	last := frameFileIndex(batch[len(batch)-1])
	shot0, shot1 := batch[0].Shot, batch[len(batch)-1].Shot
	t0, t1 := batch[0].Timestamp, batch[len(batch)-1].Timestamp
	return fmt.Sprintf("## 批次 frame_%03d-frame_%03d（镜头 %d-%d，时间 %.2f-%.2f 秒）", first, last, shot0, shot1, t0, t1)
}

func batchInstruction(job *Job, batch []Frame, already int, prev string) string {
	var b strings.Builder
	b.WriteString("这是分批观察任务，不是最终成稿。本消息已经附加本批关键帧图片。请直接在回复中写出下面这一节观察笔记，不要调用任何工具，不要向用户索要文件。\n\n")
	b.WriteString("规则：\n")
	b.WriteString("- 回复的第一行必须恰好是下面这一行（一字不差）：\n")
	b.WriteString(batchHeader(batch) + "\n")
	b.WriteString("- 标题下面写下：场景、在场角色和外貌要点、动作和表情、机位景别、和上一批是否同一场/同一人。看不清就写不确定，不要编造。\n")
	b.WriteString("- 只根据本消息里的图片和「上一批摘要」来写。不要输出标题以外的开场白或结尾说明，不要用代码块包裹全文。\n")
	b.WriteString("- 不要写 CAST.md、SHOTS.md、REMAKE.md、REPORT.md。\n\n")
	fmt.Fprintf(&b, "进度：全部 %d 帧，本批之前已记录到 frame 索引 %d（0 起算，尚未包含本批）。\n", len(job.Frames), already)
	b.WriteString("上一批摘要：\n")
	if strings.TrimSpace(prev) == "" {
		b.WriteString("（这是第一批，没有上一批记录。）\n\n")
	} else {
		b.WriteString(strings.TrimSpace(prev) + "\n\n")
	}
	b.WriteString("本批关键帧：\n")
	for _, f := range batch {
		fmt.Fprintf(&b, "- frames/%s\n", frameLabel(f))
	}
	return b.String()
}

func writeBatchFile(outDir string, job *Job, batch []Frame, already int) (prompt string, attach []string, err error) {
	text := batchInstruction(job, batch, already, previousNotesExcerpt(outDir))
	path := filepath.Join(outDir, "BATCH.md")
	if err = os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", nil, fmt.Errorf("写入 BATCH.md 失败: %w", err)
	}
	attach = append(attach, "BATCH.md")
	for _, f := range batch {
		attach = append(attach, filepath.ToSlash(filepath.Join("frames", filepath.Base(f.Path))))
	}
	prompt = "请直接在回复中写出 BATCH.md 要求的观察笔记小节。不要调用工具，不要向用户提问，不要写开场白或结尾说明。程序会把你的回复追加到 NOTES.md。"
	return prompt, attach, nil
}

func ensureNotesFile(outDir string) error {
	path := filepath.Join(outDir, notesFileName)
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return nil
	}
	body := "# 观察笔记\n\n下面由程序分批追加。请不要删除已有章节。\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func batchPrompt(job *Job, batch []Frame, already int) string {
	return batchInstruction(job, batch, already, "")
}
