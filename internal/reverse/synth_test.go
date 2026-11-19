package reverse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountShotRows(t *testing.T) {
	md := `# 分镜表

| 镜头 | 起止时间 | 景别 | 机位 | 谁在场 | 动作和表情 | 场景 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 0.00-2.10 | 全景 | 平视 | A、B | A举手发言 | 会议厅 |
| 2 | 2.10-5.00 | 中近景 | 平视 | B | B皱眉回应 | 会议厅 |
| 镜头 3 | 5.00-8.00 | 近景 | 稍俯 | A | A点头 | 会议厅 |
`

	if got := countShotRows(md); got != 3 {
		t.Fatalf("countShotRows=%d, want 3", got)
	}
	if countShotRows("") != 0 {
		t.Fatal("empty should be 0")
	}
}

func TestMissingDeliverablesAndComplete(t *testing.T) {
	dir := t.TempDir()
	frames := []Frame{{Path: "frame_000.jpg", Index: 1, Timestamp: 0, Shot: 1}, {Path: "frame_001.jpg", Index: 2, Timestamp: 2, Shot: 2}}
	shots := []Shot{{Index: 1, Start: 0, End: 2}, {Index: 2, Start: 2, End: 4}}
	man := []byte(`{"video":"demo.mp4","duration":4,"shots":[{"index":1,"start":0,"end":2},{"index":2,"start":2,"end":4}],"frames":[{"path":"frame_000.jpg","index":1,"timestamp":0,"shot":1},{"path":"frame_001.jpg","index":2,"timestamp":2,"shot":2}]}`)
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frames.json"), man, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = frames
	_ = shots
	if got := missingDeliverables(dir, 2); strings.Join(got, ",") != "CAST.md,SHOTS.md,REMAKE.md,REPORT.md" {
		t.Fatalf("missing=%v", got)
	}
	if isComplete(dir) {
		t.Fatal("empty dir should not be complete")
	}
	for _, name := range []string{"CAST.md", "REMAKE.md", "REPORT.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("内容内容内容"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	partial := `# 分镜表

| 镜头 | 起止时间 | 景别 |
| --- | --- | --- |
| 1 | 0.00-2.00 | 全景 |
`
	if err := os.WriteFile(filepath.Join(dir, "SHOTS.md"), []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}
	if isComplete(dir) {
		t.Fatal("partial shots should not be complete")
	}
	if got := missingDeliverables(dir, 2); len(got) != 1 || got[0] != "SHOTS.md" {
		t.Fatalf("missing=%v, want only SHOTS.md", got)
	}
	if err := saveDeliverable(dir, "SHOTS.md", "| 2 | 2.00-4.00 | 近景 |\n", true); err != nil {
		t.Fatal(err)
	}
	if !isComplete(dir) {
		t.Fatal("expected complete after second shot row")
	}
	if nextShotIndex(dir) != 3 {
		t.Fatalf("nextShotIndex=%d, want 3", nextShotIndex(dir))
	}
}

func TestExtractDeliverable(t *testing.T) {
	plan := "我先阅读 NOTES.md，然后写出 CAST.md。"
	if _, ok := extractDeliverable(plan, "CAST.md"); ok {
		t.Fatal("plan should be rejected")
	}
	body := "# 角色与场景\n\n- 角色 A：白发男性，深色袍，会议主位，0-90 秒出现。面容清瘦，眉骨较高，胡须花白，双手常撑桌。\n- 角色 B：橙发金甲武将，站在侧席发言，神情激动。盔甲有龙纹，披风暗红。\n- 场景：天庭会议厅，金色雕花与灯笼。\n\n复刻时每个角色固定一张参考图，所有生成单元都用同一张。\n"
	got, ok := extractDeliverable("下面是成稿。\n\n"+body, "CAST.md")
	if !ok || !strings.HasPrefix(strings.TrimSpace(got), "# 角色与场景") {
		t.Fatalf("ok=%v got=%q", ok, got)
	}
	fenced := "```markdown\n" + body + "\n```"
	got, ok = extractDeliverable(fenced, "CAST.md")
	if !ok || strings.Contains(got, "```") {
		t.Fatalf("fenced ok=%v got=%q", ok, got)
	}
}

func TestStripShotPreamble(t *testing.T) {
	first := stripShotPreamble("# 分镜表\n\n| 镜头 | 起止 |\n| --- | --- |\n| 1 | 0-1 |\n", true)
	if !strings.Contains(first, "# 分镜表") || !strings.Contains(first, "| 1 |") {
		t.Fatalf("first chunk: %q", first)
	}
	later := stripShotPreamble("# 分镜表\n\n| 镜头 | 起止 |\n| --- | --- |\n| 2 | 1-2 |\n| 3 | 2-3 |\n", false)
	if strings.Contains(later, "# 分镜表") || strings.Contains(later, "起止") {
		t.Fatalf("later should drop header: %q", later)
	}
	if !strings.Contains(later, "| 2 |") || !strings.Contains(later, "| 3 |") {
		t.Fatalf("later missing rows: %q", later)
	}
}

func TestWriteSkillCopy(t *testing.T) {
	dir := t.TempDir()
	if err := writeSkillCopy(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, skillCopyName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "视频逆向") || !strings.Contains(string(raw), "CAST.md") {
		t.Fatalf("skill copy unexpected: %s", raw)
	}
}

func TestWriteSynthContext(t *testing.T) {
	dir := t.TempDir()
	if err := writeSkillCopy(dir); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"JOB.md":      "# 逆向任务\n\n- 原片：demo.mp4\n- 时长：12.00 秒\n",
		"NOTES.md":    "# 观察笔记\n\n## 批次 frame_000-frame_001（镜头 1-1，时间 0.00-2.00 秒）\n\n华丽宫殿，两人开会。\n",
		"frames.json": `{"video":"demo.mp4","duration":12,"shots":[{"index":1,"start":0,"end":12}],"frames":[{"path":"frame_000.jpg","index":1,"timestamp":0,"shot":1}]}`,
		transcriptMD:  "# 台词转写\n\n- 0.50–2.00  今日开会\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	task := "本轮只写 CAST.md，第一行必须是：# 角色与场景。"
	if err := writeSynthContext(dir, task); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, contextFileName))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"本轮任务", "本轮只写 CAST.md", "VIDEO_REVERSE_SKILL.md", "JOB.md", "NOTES.md", "frames.json", "TRANSCRIPT.md", "今日开会", "华丽宫殿", "不要再说缺少文件"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in context:\n%s", want, got)
		}
	}
	if attach := synthAttach(dir); len(attach) != 1 || attach[0] != contextFileName {
		t.Fatalf("attach=%v", attach)
	}
}

func TestSynthCLIPromptOneLine(t *testing.T) {
	for _, file := range []string{"CAST.md", "SHOTS.md", "REMAKE.md", "REPORT.md"} {
		p := synthCLIPrompt(file)
		if strings.ContainsAny(p, "\n\r") {
			t.Fatalf("%s prompt has newline: %q", file, p)
		}
		if !strings.Contains(p, file) || !strings.Contains(p, contextFileName) {
			t.Fatalf("%s prompt unexpected: %q", file, p)
		}
	}
	if got := oneLine("a\nb\r\nc"); got != "a b c" {
		t.Fatalf("oneLine=%q", got)
	}
}
