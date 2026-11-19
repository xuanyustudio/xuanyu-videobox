package reverse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInferNextFrameFromNotes(t *testing.T) {
	notes := `# 观察笔记

## 批次 frame_000-frame_007（镜头 1-4，时间 0.00-12.50 秒）

一些内容

## 批次 frame_008-frame_015（镜头 5-8，时间 13.00-30.00 秒）

更多内容
`
	if got := inferNextFrameFromNotes(notes); got != 16 {
		t.Fatalf("got %d, want 16", got)
	}
	if got := inferNextFrameFromNotes(""); got != 0 {
		t.Fatalf("empty notes: got %d, want 0", got)
	}
}

func TestNextBatch(t *testing.T) {
	frames := make([]Frame, 10)
	for i := range frames {
		frames[i] = Frame{Path: "frame_00" + string(rune('0'+i)) + ".jpg", Index: i + 1}
	}
	got := nextBatch(frames, 8, 8)
	if len(got) != 2 {
		t.Fatalf("tail batch len=%d, want 2", len(got))
	}
	if len(nextBatch(frames, 10, 8)) != 0 {
		t.Fatalf("past end should be empty")
	}
}

func TestFindResumable(t *testing.T) {
	root := t.TempDir()
	video := filepath.Join(root, "demo.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := FindResumable(video); ok {
		t.Fatal("expected no resumable dir")
	}

	out := filepath.Join(root, "reverse_demo_20261117_144421")
	if err := os.MkdirAll(filepath.Join(out, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	frames := []Frame{
		{Path: "frame_000.jpg", Index: 1, Timestamp: 1, Shot: 1},
		{Path: "frame_001.jpg", Index: 2, Timestamp: 2, Shot: 1},
	}
	man, _ := json.Marshal(framesManifest{Video: video, Duration: 10, Frames: frames, Shots: []Shot{{Index: 1, Start: 0, End: 10}}})
	if err := os.WriteFile(filepath.Join(out, "frames.json"), man, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "frames", "frame_000.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "frames", "frame_001.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, summary, ok := FindResumable(video)
	if !ok || dir != out {
		t.Fatalf("FindResumable got ok=%v dir=%q", ok, dir)
	}
	if summary == "" {
		t.Fatal("expected summary")
	}

	job, err := LoadJob(out)
	if err != nil || len(job.Frames) != 2 {
		t.Fatalf("LoadJob: %+v %v", job, err)
	}

	for _, name := range []string{"CAST.md", "SHOTS.md", "REMAKE.md", "REPORT.md"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte("done"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, ok := FindResumable(video); ok {
		t.Fatal("complete dir should not be resumable")
	}
}

func TestBatchHeaderAndCurrentNoteFrame(t *testing.T) {
	dir := t.TempDir()
	frames := []Frame{
		{Path: "frame_000.jpg", Index: 1, Timestamp: 0, Shot: 1},
		{Path: "frame_001.jpg", Index: 2, Timestamp: 1.5, Shot: 1},
		{Path: "frame_002.jpg", Index: 3, Timestamp: 3, Shot: 2},
	}
	header := batchHeader(frames[:2])
	wantPrefix := "## 批次 frame_000-frame_001"
	if header[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("header %q", header)
	}
	notes := header + "\n\nok\n"
	if err := os.WriteFile(filepath.Join(dir, notesFileName), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := currentNoteFrame(dir, frames); got != 2 {
		t.Fatalf("currentNoteFrame=%d, want 2", got)
	}
}

func TestExtractNotesSection(t *testing.T) {
	header := "## 批次 frame_016-frame_023（镜头 17-24，时间 39.55-59.55 秒）"
	plan := "已核对：本节尚未存在，八帧对应镜头 17-24。现在只在 NOTES.md 末尾追加本批观察。"
	if _, ok := extractNotesSection(plan, header); ok {
		t.Fatal("planning message should be rejected")
	}

	body := header + "\n\n- 场景：同一宫殿会议厅。\n- 镜头 17（39.55 秒）：白发男性近景，神情认真，字幕看不清。\n- 镜头 18（41.05 秒）：橙发金甲者回应，表情不满。\n- 机位与景别：平视中近景切换。\n- 与上一批的连续性：同一场会议、同一批角色继续出现，外貌一致。"
	// pad to be substantial
	body += "\n- 补充：桌上有卷册，背景仍是金色雕花与灯笼，窗外可见云天。发言者手势明确，主位男性双手撑桌。后半段切到主位背后的对桌群像，黄袍人物进入近景。无法从静帧判断镜头运动，部分前景背影身份不确定。\n"
	got, ok := extractNotesSection(body, header)
	if !ok || !strings.Contains(got, header) || !strings.Contains(got, "镜头 17") {
		t.Fatalf("expected section, ok=%v got=%q", ok, got)
	}

	fenced := "```markdown\n" + body + "\n```"
	got, ok = extractNotesSection(fenced, header)
	if !ok || strings.Contains(got, "```") {
		t.Fatalf("fenced section, ok=%v got=%q", ok, got)
	}

	prefix := "下面是本批观察。\n\n" + body
	got, ok = extractNotesSection(prefix, header)
	if !ok || !strings.HasPrefix(strings.TrimSpace(got), "## 批次") {
		t.Fatalf("prefixed section, ok=%v got=%q", ok, got)
	}
}

func TestAppendNotesSection(t *testing.T) {
	dir := t.TempDir()
	if err := ensureNotesFile(dir); err != nil {
		t.Fatal(err)
	}
	header := "## 批次 frame_000-frame_007（镜头 1-8，时间 0.05-22.05 秒）"
	sec := header + "\n\n- 场景：会议厅。\n- 镜头 1（0.05 秒）：全景，主位男性举手。\n- 镜头 2（5.05 秒）：橙发金甲者发言。\n- 机位与景别：平视全景切中近景。\n- 与上一批的连续性：这是第一批。大厅里还有白发角色和多名参会者，服装与发色可辨，字幕提到人间已早三个月，后续发言围绕降雨项目展开。\n"
	if err := appendNotesSection(dir, sec); err != nil {
		t.Fatal(err)
	}
	if err := appendNotesSection(dir, sec); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, notesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), header); n != 1 {
		t.Fatalf("header count=%d, want 1; notes=%s", n, raw)
	}
	if got := inferNextFrameFromNotes(string(raw)); got != 8 {
		t.Fatalf("inferNextFrameFromNotes=%d, want 8", got)
	}
}
