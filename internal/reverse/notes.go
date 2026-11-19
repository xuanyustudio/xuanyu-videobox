package reverse

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const observeSystemPrompt = "你是视频镜头观察助手。根据消息里已附加的关键帧，直接写出一节中文观察笔记。第一行必须是指定的「## 批次 …」标题。不要调用工具，不要向用户提问，不要写开场白或结尾说明，不要用代码块包裹全文。"

func previousNotesExcerpt(outDir string) string {
	raw, err := os.ReadFile(filepath.Join(outDir, notesFileName))
	if err != nil {
		return ""
	}
	notes := string(raw)
	matches := batchHeaderRe.FindAllStringIndex(notes, -1)
	if len(matches) == 0 {
		return ""
	}
	last := strings.TrimSpace(notes[matches[len(matches)-1][0]:])
	return truncateRunes(last, 1800)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n…（已截断）"
}

func unwrapFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		first := strings.TrimSpace(s[:i])
		if first != "" && !strings.ContainsAny(first, " \t") && len(first) < 24 {
			s = s[i+1:]
		}
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func cutAtNextHeader(section string) string {
	lines := strings.Split(section, "\n")
	if len(lines) <= 1 {
		return section
	}
	var b strings.Builder
	b.WriteString(lines[0])
	for _, line := range lines[1:] {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			break
		}
		b.WriteByte('\n')
		b.WriteString(line)
	}
	return b.String()
}

func looksLikePlanOnly(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if batchHeaderRe.MatchString(s) && substantialBody(s) {
		return false
	}
	hints := []string{
		"我先核对", "我先读", "我先查看", "我会在", "我会按", "我现在将",
		"现在只在", "现在把本批", "接下来只在", "接下来只根据",
		"已核对", "已确认", "请补充要观察", "不要向用户索要",
	}
	hit := false
	for _, h := range hints {
		if strings.Contains(s, h) {
			hit = true
			break
		}
	}
	if !hit {
		return false
	}
	return !substantialBody(s)
}

func substantialBody(s string) bool {
	body := s
	if loc := batchHeaderRe.FindStringIndex(s); loc != nil {
		body = s[loc[1]:]
	}
	body = strings.TrimSpace(body)
	n := 0
	for _, r := range body {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	if n >= 200 {
		return true
	}
	if n < 80 {
		return false
	}
	return strings.Contains(body, "镜头") && (strings.Contains(body, "场景") || strings.Contains(body, "景别") || strings.Contains(body, "字幕") || strings.Contains(body, "外貌"))
}

func extractNotesSection(text, header string) (string, bool) {
	text = unwrapFence(text)
	header = strings.TrimSpace(header)
	if text == "" || header == "" || looksLikePlanOnly(text) {
		return "", false
	}

	section := ""
	if idx := strings.Index(text, header); idx >= 0 {
		section = strings.TrimSpace(cutAtNextHeader(text[idx:]))
	} else if loc := batchHeaderRe.FindStringIndex(text); loc != nil {
		raw := strings.TrimSpace(cutAtNextHeader(text[loc[0]:]))
		// keep required header wording even if the model slightly rephrased it
		rest := raw
		if i := strings.IndexAny(raw, "\r\n"); i >= 0 {
			rest = strings.TrimSpace(raw[i+1:])
		} else {
			rest = ""
		}
		if rest != "" {
			section = header + "\n\n" + rest
		} else {
			section = header
		}
	} else if substantialBody(text) {
		section = header + "\n\n" + text
	}
	if !substantialBody(section) {
		return "", false
	}
	if !strings.HasPrefix(strings.TrimSpace(section), "##") {
		section = header + "\n\n" + strings.TrimSpace(section)
	}
	section = strings.TrimSpace(section) + "\n"
	return section, true
}

func appendNotesSection(outDir, section string) error {
	section = strings.TrimSpace(section)
	if section == "" {
		return nil
	}
	if !strings.HasSuffix(section, "\n") {
		section += "\n"
	}
	path := filepath.Join(outDir, notesFileName)
	if err := ensureNotesFile(outDir); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	head := firstLine(section)
	if head != "" && strings.Contains(string(existing), head) {
		return nil
	}
	var b strings.Builder
	b.Write(existing)
	s := b.String()
	if s != "" && !strings.HasSuffix(s, "\n") {
		b.WriteByte('\n')
		s = b.String()
	}
	if s != "" && !strings.HasSuffix(s, "\n\n") {
		b.WriteByte('\n')
	}
	b.WriteString(section)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
