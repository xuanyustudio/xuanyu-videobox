package reverse

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	skillCopyName     = "VIDEO_REVERSE_SKILL.md"
	contextFileName   = "SYNTH_CONTEXT.md"
	shotWriteBatch    = 24
	fileStallTimeout  = 5 * time.Minute
	synthSystemPrompt = "你是玄宇视频工具箱「视频逆向」成稿助手。成稿材料已经完整写在已附加的 SYNTH_CONTEXT.md 里（含 NOTES.md、JOB.md、frames.json、台词转写和技能说明）。只把本轮要求的那一个 Markdown 文件写在回复里。不要调用任何工具，不要说缺少文件，不要向用户提问，不要写开场白或结尾说明，不要用代码块包裹全文。"
)

var deliverableFiles = []string{"CAST.md", "SHOTS.md", "REMAKE.md", "REPORT.md"}

var (
	shotRowRe     = regexp.MustCompile(`(?m)^\s*\|\s*(?:镜头\s*)?(\d+)\s*\|`)
	shotHeadingRe = regexp.MustCompile(`(?m)^(?:#{1,6}\s*|\*\*)\s*镜头\s*(\d+)`)
	shotSepRe     = regexp.MustCompile(`^\s*\|?[\s:|-]+$`)
)

func writeSkillCopy(outDir string) error {
	path := filepath.Join(outDir, skillCopyName)
	return os.WriteFile(path, []byte(skillMD), 0o644)
}

func missingDeliverables(outDir string, nShots int) []string {
	var missing []string
	for _, name := range deliverableFiles {
		if name == "SHOTS.md" {
			if !shotsLooksComplete(outDir, nShots) {
				missing = append(missing, name)
			}
			continue
		}
		if fileIfExists(outDir, name) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func countShotRows(md string) int {
	seen := map[int]struct{}{}
	add := func(re *regexp.Regexp) {
		for _, m := range re.FindAllStringSubmatch(md, -1) {
			n, err := strconv.Atoi(m[1])
			if err == nil && n > 0 {
				seen[n] = struct{}{}
			}
		}
	}
	add(shotRowRe)
	add(shotHeadingRe)
	return len(seen)
}

func shotsLooksComplete(outDir string, nShots int) bool {
	path := fileIfExists(outDir, "SHOTS.md")
	if path == "" {
		return false
	}
	if nShots <= 0 {
		return true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	n := countShotRows(string(raw))
	if n == 0 {
		// 有文件但无法解析行数时，不要覆盖已有成稿
		return true
	}
	return n >= nShots
}

func expectedShotCount(outDir string) int {
	job, err := LoadJob(outDir)
	if err != nil {
		return 0
	}
	return len(job.Shots)
}

func nextShotIndex(outDir string) int {
	raw, err := os.ReadFile(filepath.Join(outDir, "SHOTS.md"))
	if err != nil {
		return 1
	}
	n := countShotRows(string(raw))
	if n <= 0 {
		return 1
	}
	return n + 1
}

func synthAttach(outDir string) []string {
	if fileIfExists(outDir, contextFileName) != "" {
		return []string{contextFileName}
	}
	return nil
}

func writeSynthContext(outDir, task string) error {
	var b strings.Builder
	if strings.TrimSpace(task) != "" {
		b.WriteString("# 本轮任务\n\n")
		b.WriteString(strings.TrimSpace(task))
		b.WriteString("\n\n")
	}
	b.WriteString("# 成稿材料\n\n")
	b.WriteString("这些正文由程序写入并附加在本轮对话里，请当作已经收到 NOTES.md、JOB.md、frames.json、VIDEO_REVERSE_SKILL.md")
	if fileIfExists(outDir, transcriptMD) != "" {
		b.WriteString(" 和 TRANSCRIPT.md")
	}
	b.WriteString("。不要再说缺少文件，也不要调用工具去读盘。\n\n")

	type section struct {
		title string
		name  string
	}
	sections := []section{
		{"VIDEO_REVERSE_SKILL.md", skillCopyName},
		{"JOB.md", "JOB.md"},
		{"TRANSCRIPT.md", transcriptMD},
		{"frames.json", "frames.json"},
		{"NOTES.md", "NOTES.md"},
		{"CAST.md（已有部分）", "CAST.md"},
		{"SHOTS.md（已有部分）", "SHOTS.md"},
		{"REMAKE.md（已有部分）", "REMAKE.md"},
		{"REPORT.md（已有部分）", "REPORT.md"},
	}
	hasTranscript := false
	for _, sec := range sections {
		raw, err := os.ReadFile(filepath.Join(outDir, sec.name))
		if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		if sec.name == transcriptMD {
			hasTranscript = true
		}
		fmt.Fprintf(&b, "## %s\n\n", sec.title)
		body := strings.TrimSpace(string(raw))
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	if !hasTranscript {
		b.WriteString("## TRANSCRIPT.md\n\n（尚未转写或原片没有可用音轨。对白请仅根据 NOTES 里看得见的字幕或口型，并标明不确定。）\n\n")
	}
	return os.WriteFile(filepath.Join(outDir, contextFileName), []byte(b.String()), 0o644)
}

func synthPromptFor(job *Job, file string, shotFrom, shotTo int) string {
	var b strings.Builder
	b.WriteString("这是最终成稿任务，不是分批观察。成稿材料已经完整写在已附加的 SYNTH_CONTEXT.md 里，包括 NOTES.md、JOB.md、frames.json、台词转写（如有）和技能说明。请根据这些正文直接写本轮要求的文件，不要调用任何工具，不要说缺少文件，不要向用户索要文件。\n\n")
	fmt.Fprintf(&b, "片长约 %.2f 秒，镜头 %d 个，关键帧 %d 张。分镜请按 frames.json 里 shots 的起止时间来写，不要把抽样间隔当成镜头边界。\n", job.Duration, len(job.Shots), len(job.Frames))
	switch file {
	case "CAST.md":
		b.WriteString("本轮只写 CAST.md，不要写 SHOTS.md、REMAKE.md、REPORT.md。\n")
		b.WriteString("回复的第一行必须是：# 角色与场景\n")
		b.WriteString("按技能要求写出角色 / 场景 / 道具表：每个角色稳定编号（A、B…），写清外观（脸、发、服装、体型，尽量具体），列出出现的大致时间；若有台词转写，可据此区分谁在说话。最后写：「复刻时每个角色固定一张参考图，所有生成单元都用同一张。」\n")
		b.WriteString("程序会把你的回复保存为 CAST.md。\n")
	case "SHOTS.md":
		fmt.Fprintf(&b, "本轮只写 SHOTS.md 里的镜头 %d 到 %d（共 %d 个镜头中的这一段），不要写其他三个文件。\n", shotFrom, shotTo, len(job.Shots))
		b.WriteString("一行一个镜头，建议用 Markdown 表格，列至少包括：镜头编号、起止时间、景别、机位、谁在场、动作和表情、台词、场景。台词按 SYNTH_CONTEXT.md 里 TRANSCRIPT.md 的时间轴填写，不要凭画面猜测对白。\n")
		if shotFrom <= 1 {
			b.WriteString("这是该文件的开头：第一行用「# 分镜表」，然后写表头，再写镜头 1 到本段结尾的行。\n")
		} else {
			fmt.Fprintf(&b, "SHOTS.md 里已经有镜头 1 到 %d。不要重复总标题和表头，只写镜头 %d 到 %d 的表格行。\n", shotFrom-1, shotFrom, shotTo)
		}
		b.WriteString("程序会把你的回复追加到 SHOTS.md。\n")
	case "REMAKE.md":
		b.WriteString("本轮只写 REMAKE.md，不要写其他三个文件。若已附加 CAST.md / SHOTS.md，请与它们保持一致。\n")
		b.WriteString("回复的第一行必须是：# 复刻方案\n")
		b.WriteString("按技能要求写出：把同一角色、同一场景的连续镜头合并成生成单元；单元之间怎么衔接；每个角色 / 商品要准备的参考图；每个生成单元的提示词草稿（中文，可附英文，不要写字幕、花字、水印）；配音按 TRANSCRIPT.md 的台词来写建议。优先假设模型能一次多镜头、能吃参考图。\n")
		b.WriteString("程序会把你的回复保存为 REMAKE.md。\n")
	case "REPORT.md":
		b.WriteString("本轮只写 REPORT.md，不要写其他三个文件。若已附加前面的成稿，请与它们保持一致。\n")
		b.WriteString("回复的第一行必须是：# 拆解报告\n")
		b.WriteString("写一份给人看的短报告：这条视频讲什么、怎么做的、人物如何锁定、按什么顺序复刻，以及台词和配音要注意什么。看不清的地方标明不确定，不要编造。\n")
		b.WriteString("程序会把你的回复保存为 REPORT.md。\n")
	}
	return b.String()
}

func synthCLIPrompt(file string) string {
	switch file {
	case "CAST.md":
		return "请根据已附加的 SYNTH_CONTEXT.md 直接写出 CAST.md 正文。第一行必须是：# 角色与场景。不要调用工具，不要说缺少文件，不要写开场白或结尾说明。程序会把你的回复保存为 CAST.md。"
	case "SHOTS.md":
		return "请根据已附加的 SYNTH_CONTEXT.md 直接写出本轮要求的 SHOTS.md 分镜正文。不要调用工具，不要说缺少文件，不要写开场白或结尾说明。程序会把你的回复保存或追加到 SHOTS.md。"
	case "REMAKE.md":
		return "请根据已附加的 SYNTH_CONTEXT.md 直接写出 REMAKE.md 正文。第一行必须是：# 复刻方案。不要调用工具，不要说缺少文件，不要写开场白或结尾说明。程序会把你的回复保存为 REMAKE.md。"
	case "REPORT.md":
		return "请根据已附加的 SYNTH_CONTEXT.md 直接写出 REPORT.md 正文。第一行必须是：# 拆解报告。不要调用工具，不要说缺少文件，不要写开场白或结尾说明。程序会把你的回复保存为 REPORT.md。"
	default:
		return "请根据已附加的 SYNTH_CONTEXT.md 直接写出本轮要求的那个文件正文。不要调用工具，不要说缺少文件，不要写开场白或结尾说明。"
	}
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func extractDeliverable(text, file string) (string, bool) {
	text = unwrapFence(text)
	text = strings.TrimSpace(text)
	if text == "" || looksLikePlanOnly(text) {
		return "", false
	}
	minRunes := 120
	if file == "SHOTS.md" {
		minRunes = 80
	} else if file == "REMAKE.md" {
		minRunes = 150
	}
	body := text
	if loc := strings.Index(text, "#"); loc >= 0 {
		// keep from the first markdown heading if the model added a short preface
		if loc == 0 || strings.TrimSpace(text[:loc]) == "" || runeLen(strings.TrimSpace(text[:loc])) <= 40 {
			from := loc
			if loc > 0 {
				if i := strings.LastIndex(text[:loc], "\n"); i >= 0 {
					from = i + 1
				}
			}
			body = strings.TrimSpace(text[from:])
		}
	}
	if runeLen(body) < minRunes {
		return "", false
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body, true
}

func runeLen(s string) int {
	return len([]rune(s))
}

func isTableHeaderLine(line string) bool {
	trim := strings.TrimSpace(line)
	if trim == "" {
		return true
	}
	if strings.HasPrefix(trim, "#") {
		return true
	}
	if shotSepRe.MatchString(trim) {
		return true
	}
	if strings.Contains(trim, "|") && (strings.Contains(trim, "景别") || strings.Contains(trim, "起止") || (strings.Contains(trim, "镜头") && !shotRowRe.MatchString(trim))) {
		return true
	}
	return false
}

func stripShotPreamble(content string, first bool) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if first {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content
	}
	lines := strings.Split(content, "\n")
	start := 0
	found := false
	for i, line := range lines {
		if shotRowRe.MatchString(line) || shotHeadingRe.MatchString(strings.TrimSpace(line)) {
			start = i
			found = true
			break
		}
		if !isTableHeaderLine(line) && strings.TrimSpace(line) != "" {
			start = i
			found = true
			break
		}
	}
	if !found {
		return content + "\n"
	}
	body := strings.TrimSpace(strings.Join(lines[start:], "\n"))
	if body == "" {
		return ""
	}
	return body + "\n"
}

func saveDeliverable(outDir, file, content string, appendShots bool) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("成稿内容为空")
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	path := filepath.Join(outDir, file)
	if file == "SHOTS.md" && appendShots {
		existing, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		first := len(existing) == 0
		chunk := stripShotPreamble(content, first)
		if strings.TrimSpace(chunk) == "" {
			return fmt.Errorf("本段分镜内容为空")
		}
		var b strings.Builder
		b.Write(existing)
		s := b.String()
		if s != "" && !strings.HasSuffix(s, "\n") {
			b.WriteByte('\n')
		}
		if s != "" && !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteByte('\n')
		}
		b.WriteString(chunk)
		return os.WriteFile(path, []byte(b.String()), 0o644)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
