package hookopen

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSRT(t *testing.T) {
	raw := "1\n00:00:01,000 --> 00:00:03,500\n你竟然敢背叛我\n\n2\n00:00:04,000 --> 00:00:07,200\n这不可能\n"
	cues := parseSRT(raw)
	if len(cues) != 2 {
		t.Fatalf("got %d cues: %+v", len(cues), cues)
	}
	if math.Abs(cues[0].Start-1) > 0.001 || math.Abs(cues[0].End-3.5) > 0.001 {
		t.Fatalf("first cue times: %+v", cues[0])
	}
	if cues[0].Text != "你竟然敢背叛我" {
		t.Fatalf("text = %q", cues[0].Text)
	}
	q := quoteInRange(cues, 1, 8)
	if !strings.Contains(q, "背叛") || !strings.Contains(q, "不可能") {
		t.Fatalf("quote %q", q)
	}
}

func TestFormatClockAndRange(t *testing.T) {
	if got := formatClock(65.5); got != "00:01:05.500" {
		t.Fatalf("clock = %q", got)
	}
	if got := formatRange(12.4, 17.9); !strings.Contains(got, "12.40") {
		t.Fatalf("range = %q", got)
	}
}

func TestBuildWindowsPrefersHookKeywords(t *testing.T) {
	cues := []Cue{
		{Start: 1, End: 3, Text: "今天天气不错"},
		{Start: 3, End: 6, Text: "我们先吃饭吧"},
		{Start: 40, End: 43, Text: "你竟然敢背叛我"},
		{Start: 43, End: 46, Text: "原来这一切都是假的"},
		{Start: 80, End: 84, Text: "那我们走吧"},
	}
	wins := buildWindows(cues, []float64{42}, nil, 90)
	if len(wins) == 0 {
		t.Fatal("no windows")
	}
	top := wins[0]
	if top.Start > 43 || top.End < 43 {
		t.Fatalf("expected twist window first, got %.2f–%.2f %q score=%.2f", top.Start, top.End, top.Quote, top.Score)
	}
	if !strings.Contains(top.Quote, "背叛") && !strings.Contains(top.Quote, "原来") {
		t.Fatalf("top quote %q", top.Quote)
	}
}

func TestClampWindow(t *testing.T) {
	s, e := clampWindow(-1, 20, 12)
	if s < 0 || e-s > MaxHookSec+0.01 || e > 12 {
		t.Fatalf("clamped %.2f–%.2f", s, e)
	}
	s, e = clampWindow(10, 11, 12)
	if e-s < MinHookSec-0.01 {
		t.Fatalf("short window not expanded: %.2f–%.2f", s, e)
	}
}

func TestParseHookJSON(t *testing.T) {
	raw := "```json\n{\"candidates\":[{\"rank\":1,\"hook_type\":\"强反转\",\"reason\":\"身份揭穿\",\"start_sec\":12.4,\"end_sec\":17.9,\"quote\":\"你竟然是\",\"is_recommended\":true},{\"rank\":2,\"hook_type\":\"强冲突\",\"reason\":\"对峙\",\"timestamp\":\"00:01:10.000 --> 00:01:15.500\",\"quote\":\"滚出去\",\"is_recommended\":false}]}\n```"
	cs, err := parseHookJSON(raw, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d: %+v", len(cs), cs)
	}
	if math.Abs(cs[0].StartSec-12.4) > 0.01 || math.Abs(cs[0].EndSec-17.9) > 0.01 {
		t.Fatalf("first = %+v", cs[0])
	}
	if !cs[0].Recommended || cs[0].HookType != "强反转" {
		t.Fatalf("first flags %+v", cs[0])
	}
	if math.Abs(cs[1].StartSec-70) > 0.01 || math.Abs(cs[1].EndSec-75.5) > 0.01 {
		t.Fatalf("timestamp parse %+v", cs[1])
	}
}

func TestHeuristicCandidates(t *testing.T) {
	wins := []scoredWindow{
		{Start: 40, End: 46, Quote: "你竟然敢背叛我", Score: 4, Why: "台词段落"},
		{Start: 10, End: 16, Quote: "吃饭了", Score: 1, Why: "高能点"},
		{Start: 70, End: 76, Quote: "这不可能", Score: 2, Why: "高能点"},
	}
	cs := heuristicCandidates(wins, nil)
	if len(cs) != 3 || !cs[0].Recommended || cs[0].StartSec != 40 {
		t.Fatalf("%+v", cs)
	}
}

func TestNormalizeTransition(t *testing.T) {
	if NormalizeTransition("闪黑") != "fadeblack" {
		t.Fatalf("闪黑")
	}
	if NormalizeTransition("") != "fade" {
		t.Fatalf("default")
	}
	if NormalizeTransition("slideleft") != "slideleft" {
		t.Fatalf("slideleft")
	}
}

func TestDefaultOutputPath(t *testing.T) {
	p := DefaultOutputPath(filepath.Join("D:", "片", "demo.mp4"))
	if !strings.HasSuffix(p, "demo_爆点开头.mp4") {
		t.Fatalf("path = %q", p)
	}
}

func TestRMSPeaks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	rate := 16000
	// 2s quiet, 0.4s loud, 2s quiet
	n := rate * 5
	samples := make([]int16, n)
	for i := rate * 2; i < rate*2+rate*2/5; i++ {
		samples[i] = 20000
	}
	writePCM16Wav(t, path, rate, samples)
	series, err := rmsSeries(path, 0.2, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	peaks := peakTimes(series, 3)
	if len(peaks) == 0 {
		t.Fatal("no peaks")
	}
	if peaks[0] < 1.6 || peaks[0] > 2.8 {
		t.Fatalf("peak at %.2f, want ~2.2", peaks[0])
	}
}

func writePCM16Wav(t *testing.T, path string, rate int, samples []int16) {
	t.Helper()
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	buf := make([]byte, 44+len(data))
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+len(data)))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint16(buf[22:24], 1)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(rate*2))
	binary.LittleEndian.PutUint16(buf[32:34], 2)
	binary.LittleEndian.PutUint16(buf[34:36], 16)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(len(data)))
	copy(buf[44:], data)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}
