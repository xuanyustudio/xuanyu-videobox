package reverse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndFormatSRT(t *testing.T) {
	raw := "1\n00:00:01,500 --> 00:00:03,000\n大家好\n\n2\n00:00:14,620 --> 00:00:18,100\n[spk1] 今日评审天庭降雨项目\n\n3\n00:02:01.05 --> 00:02:03.2\n通过。\n"
	lines := parseSRT(raw)
	if len(lines) != 3 {
		t.Fatalf("got %d lines: %#v", len(lines), lines)
	}
	if lines[0].Start != 1.5 || lines[0].End != 3.0 || lines[0].Text != "大家好" {
		t.Fatalf("line0=%#v", lines[0])
	}
	if abs(lines[1].Start-14.62) > 0.001 || lines[1].Text != "[spk1] 今日评审天庭降雨项目" {
		t.Fatalf("line1=%#v", lines[1])
	}
	if abs(lines[2].Start-121.05) > 0.001 || abs(lines[2].End-123.2) > 0.001 {
		t.Fatalf("line2=%#v", lines[2])
	}
	md := formatTranscriptMD(raw)
	if !strings.Contains(md, "# 台词转写") || !strings.Contains(md, "1.50–3.00  大家好") {
		t.Fatalf("md=%s", md)
	}
	if !strings.Contains(md, "14.62–18.10") {
		t.Fatalf("missing second cue: %s", md)
	}
}

func TestFormatTranscriptEmpty(t *testing.T) {
	md := formatTranscriptMD("")
	if !strings.Contains(md, "没有识别到对白") {
		t.Fatalf("md=%s", md)
	}
}

func TestEnsureTranscriptSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, transcriptMD)
	if err := os.WriteFile(path, []byte("# 台词转写\n\n- 0.00–1.00  已有\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := &Job{OutDir: dir, VideoPath: filepath.Join(dir, "missing.mp4")}
	var logs []string
	if err := ensureTranscript(t.Context(), nil, job, func(s string) { logs = append(logs, s) }, func(int, string) {}); err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "已有台词转写") {
		t.Fatalf("logs=%v", logs)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
