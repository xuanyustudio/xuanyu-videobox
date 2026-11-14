# 长视频爆款复刻 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 超过所选段长的参考视频按 10 或 15 秒逐段分析，整片字幕按时间塞进对应段，多段时用同一段锁定形象和一张参考帧。

**Architecture:** 纯函数负责切段、字幕归属、提示词拼接和锁定形象解析。`replicate.Run` 先对原片调用可替换的识别函数，再按段抽帧；两段及以上时先做一次锁定形象调用，再逐段调用现有视觉客户端。桌面页和网页只增加段长选择和结果切换，并展示总稿。

**Tech Stack:** Go，现有 `internal/replicate`、`internal/asr`、`internal/config`、Fyne 桌面页、`internal/webui`。

规格：`docs/superpowers/specs/2026-10-31-long-video-replicate-design.md`

---

### Task 1: 段长与切段

**Files:**
- Create: `internal/replicate/segments.go`
- Test: `internal/replicate/segments_test.go`

**Step 1: Write the failing test**

`internal/replicate/segments_test.go`，包名 `replicate_test`：

```go
package replicate_test

import (
	"math"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func spansEqual(t *testing.T, got []replicate.Span, want []replicate.Span) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len %d != %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if math.Abs(got[i].Start-want[i].Start) > 1e-9 || math.Abs(got[i].End-want[i].End) > 1e-9 {
			t.Fatalf("span %d got %v want %v", i, got[i], want[i])
		}
	}
}

func TestNormalizeSegmentSec(t *testing.T) {
	if replicate.NormalizeSegmentSec(10) != 10 || replicate.NormalizeSegmentSec(15) != 15 {
		t.Fatal("10 and 15 must pass through")
	}
	for _, n := range []int{0, 12, 30, -1} {
		if replicate.NormalizeSegmentSec(n) != 15 {
			t.Fatalf("%d should become 15", n)
		}
	}
}

func TestPlanSegments(t *testing.T) {
	spansEqual(t, replicate.PlanSegments(0, 15), []replicate.Span{{Start: 0, End: 60}})
	spansEqual(t, replicate.PlanSegments(10, 15), []replicate.Span{{Start: 0, End: 10}})
	spansEqual(t, replicate.PlanSegments(15, 15), []replicate.Span{{Start: 0, End: 15}})
	spansEqual(t, replicate.PlanSegments(15.4, 15), []replicate.Span{{Start: 0, End: 15.4}})
	spansEqual(t, replicate.PlanSegments(16, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 16}})
	spansEqual(t, replicate.PlanSegments(30, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30}})
	spansEqual(t, replicate.PlanSegments(30.4, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30.4}})
	spansEqual(t, replicate.PlanSegments(31, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30}, {Start: 30, End: 31}})
	spansEqual(t, replicate.PlanSegments(46, 15), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 30}, {Start: 30, End: 45}, {Start: 45, End: 46}})
	spansEqual(t, replicate.PlanSegments(10, 10), []replicate.Span{{Start: 0, End: 10}})
	spansEqual(t, replicate.PlanSegments(10.4, 10), []replicate.Span{{Start: 0, End: 10.4}})
	spansEqual(t, replicate.PlanSegments(11, 10), []replicate.Span{{Start: 0, End: 10}, {Start: 10, End: 11}})
	spansEqual(t, replicate.PlanSegments(20, 10), []replicate.Span{{Start: 0, End: 10}, {Start: 10, End: 20}})
	spansEqual(t, replicate.PlanSegments(25, 10), []replicate.Span{{Start: 0, End: 10}, {Start: 10, End: 20}, {Start: 20, End: 25}})
	spansEqual(t, replicate.PlanSegments(16, 12), []replicate.Span{{Start: 0, End: 15}, {Start: 15, End: 16}})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/replicate/ -count=1 -run "TestNormalizeSegmentSec|TestPlanSegments"`

Expected: FAIL，`NormalizeSegmentSec` 未定义。

**Step 3: Write minimal implementation**

`internal/replicate/segments.go`：

```go
package replicate

// Span is one analysis window on the source timeline, in seconds.
type Span struct {
	Start float64
	End   float64
}

// NormalizeSegmentSec accepts only 10 or 15. Anything else is 15.
func NormalizeSegmentSec(n int) int {
	if n == 10 || n == 15 {
		return n
	}
	return 15
}

// PlanSegments splits durationSec into windows of segmentSec.
// A non-positive duration becomes one 0–60s span (the existing frame fallback).
// A remainder shorter than 1 second is absorbed into the previous window.
func PlanSegments(durationSec float64, segmentSec int) []Span {
	s := float64(NormalizeSegmentSec(segmentSec))
	if durationSec <= 0 {
		return []Span{{Start: 0, End: fallbackDurationSec}}
	}
	if durationSec <= s {
		return []Span{{Start: 0, End: durationSec}}
	}
	var out []Span
	start := 0.0
	for start+s < durationSec-1e-9 {
		end := start + s
		if durationSec-end < 1 {
			out = append(out, Span{Start: start, End: durationSec})
			return out
		}
		out = append(out, Span{Start: start, End: end})
		start = end
	}
	if start < durationSec-1e-9 {
		out = append(out, Span{Start: start, End: durationSec})
	}
	return out
}
```

`fallbackDurationSec` 已在 `internal/replicate/frames.go`。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/replicate/ -count=1 -run "TestNormalizeSegmentSec|TestPlanSegments"`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/replicate/segments.go internal/replicate/segments_test.go
git commit -m "按 10 或 15 秒规划爆款复刻分段。"
```

---

### Task 2: 字幕解析与归属

**Files:**
- Create: `internal/replicate/cues.go`
- Test: `internal/replicate/cues_test.go`

**Step 1: Write the failing test**

```go
package replicate_test

import (
	"math"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func TestParseSRTAndAssign(t *testing.T) {
	raw := "1\n00:00:01,000 --> 00:00:03,000\n你好\n\n" +
		"2\n00:00:14,000 --> 00:00:15,000\n正好结束\n\n" +
		"3\n00:00:15,000 --> 00:00:16,000\n正好开始\n\n" +
		"4\n00:00:14,500 --> 00:00:15,500\n跨过去\n\n" +
		"5\n00:00:20,000 --> 00:00:21,000\n后一段\n\n"
	cues := replicate.ParseSRT(raw)
	if len(cues) != 5 {
		t.Fatalf("cues=%d", len(cues))
	}
	first := replicate.CuesForSpan(cues, replicate.Span{Start: 0, End: 15})
	second := replicate.CuesForSpan(cues, replicate.Span{Start: 15, End: 30})
	if len(first) != 3 || first[0].Text != "你好" || first[1].Text != "正好结束" || first[2].Text != "跨过去" {
		t.Fatalf("first=%+v", first)
	}
	if len(second) != 2 || second[0].Text != "正好开始" || second[1].Text != "跨过去" {
		t.Fatalf("second=%+v", second)
	}
	if math.Abs(first[2].Start-14.5) > 1e-9 || math.Abs(first[2].End-15.5) > 1e-9 {
		t.Fatalf("crossing cue changed: %+v", first[2])
	}
	if len(replicate.CuesForSpan(nil, replicate.Span{Start: 0, End: 15})) != 0 {
		t.Fatal("nil cues")
	}
	outside := replicate.CuesForSpan(cues, replicate.Span{Start: 3, End: 14})
	if len(outside) != 0 {
		t.Fatalf("outside=%+v", outside)
	}
}

func TestFormatCueLinesStripsSpeaker(t *testing.T) {
	cues := []replicate.Cue{{Start: 15.2, End: 18, Text: "说话人1：卖点来了"}}
	got := replicate.FormatCueLines(cues)
	if got != "15.2–18.0秒：卖点来了" {
		t.Fatalf("%q", got)
	}
	if replicate.FormatCueLines(nil) != "" {
		t.Fatal("empty")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/replicate/ -count=1 -run "TestParseSRTAndAssign|TestFormatCueLinesStripsSpeaker"`

Expected: FAIL，`ParseSRT` 未定义。

**Step 3: Write minimal implementation**

`internal/replicate/cues.go`：

```go
package replicate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/script"
)

// Cue is one subtitle line on the source timeline.
type Cue struct {
	Start float64
	End   float64
	Text  string
}

// ParseSRT reads a standard SRT body. Malformed blocks are skipped.
// Cue text is kept as written; speaker labels are stripped later for prompts.
func ParseSRT(raw string) []Cue {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []Cue
	for _, block := range strings.Split(raw, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 {
			continue
		}
		timeLine := lines[0]
		textLines := lines[1:]
		if !strings.Contains(timeLine, "-->") && len(lines) >= 3 {
			timeLine = lines[1]
			textLines = lines[2:]
		}
		start, end, ok := parseSRTRange(timeLine)
		if !ok {
			continue
		}
		text := strings.TrimSpace(strings.Join(textLines, "\n"))
		if text == "" {
			continue
		}
		out = append(out, Cue{Start: start, End: end, Text: text})
	}
	return out
}

func parseSRTRange(line string) (float64, float64, bool) {
	parts := strings.Split(line, "-->")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, ok1 := parseSRTClock(parts[0])
	end, ok2 := parseSRTClock(parts[1])
	if !ok1 || !ok2 || end < start {
		return 0, 0, false
	}
	return start, end, true
}

func parseSRTClock(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	chunk := strings.Split(s, ":")
	if len(chunk) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(chunk[0])
	m, err2 := strconv.Atoi(chunk[1])
	sec, err3 := strconv.ParseFloat(chunk[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return float64(h*3600+m*60) + sec, true
}

// CuesForSpan returns cues that overlap span, in start-time order, text uncut.
func CuesForSpan(cues []Cue, span Span) []Cue {
	var out []Cue
	for _, c := range cues {
		if c.Start < span.End && c.End > span.Start {
			out = append(out, c)
		}
	}
	return out
}

// FormatCueLines renders prompt lines. Speaker prefixes are removed here only.
func FormatCueLines(cues []Cue) string {
	var lines []string
	for _, c := range cues {
		text := script.StripSpeakerLabels(c.Text)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%.1f–%.1f秒：%s", c.Start, c.End, text))
	}
	return strings.Join(lines, "\n")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/replicate/ -count=1 -run "TestParseSRTAndAssign|TestFormatCueLinesStripsSpeaker"`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/replicate/cues.go internal/replicate/cues_test.go
git commit -m "按时间重叠把完整字幕句归入分段。"
```

---

### Task 3: 每段用户提示

**Files:**
- Create: `internal/replicate/promptctx.go`
- Test: `internal/replicate/promptctx_test.go`

**Step 1: Write the failing test**

```go
package replicate_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func TestBuildSegmentUserPrompt(t *testing.T) {
	cues := []replicate.Cue{{Start: 1, End: 2, Text: "说话人2：开场白"}}
	span := replicate.Span{Start: 0, End: 12}
	std := replicate.BuildSegmentUserPrompt(replicate.TemplateStandard, 8, span, 15, cues, false)
	if !strings.Contains(std, "8 帧") || !strings.Contains(std, "0.0–12.0") || !strings.Contains(std, "1.0–2.0秒：开场白") {
		t.Fatalf("%s", std)
	}
	if strings.Contains(std, "说话人") || strings.Contains(std, "第一张图只用于统一外形") {
		t.Fatal("single segment should not mention the lock frame or speaker label")
	}
	if !strings.Contains(std, "时间戳从 0 开始") {
		t.Fatal("missing relative clock")
	}

	none := replicate.BuildSegmentUserPrompt(replicate.TemplateStandard, 8, span, 15, nil, false)
	if !strings.Contains(none, "本段字幕：无对白") {
		t.Fatal(none)
	}

	sd := replicate.BuildSegmentUserPrompt(replicate.TemplateSeedance2, 8, replicate.Span{Start: 15, End: 30}, 10, nil, true)
	if !strings.Contains(sd, "0–2、2–4、4–7、7–9、9–10") || !strings.Contains(sd, "恰好 10 秒") {
		t.Fatalf("10s beats: %s", sd)
	}
	if strings.Contains(sd, "恰好 15 秒") {
		t.Fatal("segment prompt must override the template's 15s clock")
	}
	if !strings.Contains(sd, "第一张图只用于统一外形") {
		t.Fatal("multi segment missing ref-frame note")
	}

	sd15 := replicate.BuildSegmentUserPrompt(replicate.TemplateSeedance2, 8, replicate.Span{Start: 0, End: 15}, 15, nil, false)
	if !strings.Contains(sd15, "0–3、3–6、6–10、10–13、13–15") {
		t.Fatalf("15s beats: %s", sd15)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/replicate/ -count=1 -run TestBuildSegmentUserPrompt`

Expected: FAIL，函数未定义。

**Step 3: Write minimal implementation**

`internal/replicate/promptctx.go`：

```go
package replicate

import (
	"fmt"
	"strings"
)

// SeedanceBeats is the five-beat clock for a 10s or 15s segment.
func SeedanceBeats(segmentSec int) string {
	if NormalizeSegmentSec(segmentSec) == 10 {
		return "0–2、2–4、4–7、7–9、9–10"
	}
	return "0–3、3–6、6–10、10–13、13–15"
}

// BuildSegmentUserPrompt is the user message for one segment.
// It does not append BuildUserPrompt's old duration sentence, so Seedance is not told to stay on 15s.
func BuildSegmentUserPrompt(t Template, numFrames int, span Span, segmentSec int, cues []Cue, multi bool) string {
	segmentSec = NormalizeSegmentSec(segmentSec)
	src := sora2UserTemplate
	if t == TemplateSeedance2 {
		src = seedance2UserTemplate
	}
	p := strings.ReplaceAll(normalizeNewlines(src), "{num_frames}", fmt.Sprintf("%d", numFrames))
	dur := span.End - span.Start
	var b strings.Builder
	b.WriteString(p)
	fmt.Fprintf(&b, "\n\n本段对应原片 %.1f–%.1f 秒（本段时长 %.1f 秒）。", span.Start, span.End, dur)
	if multi {
		b.WriteString("第一张图只用于统一外形，不是这一段的新镜头。只把后面的画面当作这一段的镜头。正文里的人物、服装、产品外形必须与锁定形象一致，不要换说法。")
	} else {
		b.WriteString("请只根据这一段画面来写。")
	}
	if t == TemplateSeedance2 {
		fmt.Fprintf(&b, "\n请输出恰好 %d 秒的 Seedance 时间轴，五段为 %s。这覆盖模板里写死的 15 秒。", segmentSec, SeedanceBeats(segmentSec))
		b.WriteString("若本段短于该时长，仍铺满这段时间轴，只使用这一段的画面和字幕。")
	} else {
		b.WriteString("\n本段提示词里的时间戳从 0 开始，长度等于本段时长。")
	}
	b.WriteString("\n\n本段字幕（语音识别，仅作台词参考；不要编造列表里没有的句子，也不要漏掉下面列出的句子）：\n")
	if lines := FormatCueLines(cues); lines != "" {
		b.WriteString(lines)
	} else {
		b.WriteString("本段字幕：无对白")
	}
	return b.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/replicate/ -count=1 -run TestBuildSegmentUserPrompt`

Expected: PASS。再跑 `go test ./internal/replicate/ -count=1 -run TestBuildUserPrompt`，旧的 15 秒模板测试仍 PASS。

**Step 5: Commit**

```bash
git add internal/replicate/promptctx.go internal/replicate/promptctx_test.go
git commit -m "分段提示词带上时间范围和对应字幕。"
```

---

### Task 4: 锁定形象与总稿拼接

**Files:**
- Create: `internal/replicate/lock.go`
- Test: `internal/replicate/lock_test.go`

**Step 1: Write the failing test**

```go
package replicate_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/replicate"
)

func TestParseLock(t *testing.T) {
	raw := "## 锁定形象 (中文)\n```\n黑发白衬衫。\n```\n\n## 锁定形象 (English)\n```\nBlack hair, white shirt.\n```\n"
	zh, en := replicate.ParseLock(raw)
	if zh != "黑发白衬衫。" || en != "Black hair, white shirt." {
		t.Fatalf("zh=%q en=%q", zh, en)
	}
	if z, e := replicate.ParseLock("no fences"); z != "" || e != "" {
		t.Fatal("empty parse")
	}
}

func TestAssemblePrompts(t *testing.T) {
	segs := []replicate.SegmentDraft{
		{Index: 1, Start: 0, End: 15, PromptEN: "EN1", PromptZH: "ZH1", Analysis: "A1"},
		{Index: 2, Start: 15, End: 30, PromptEN: "", PromptZH: "ZH2", Analysis: "A2"},
	}
	got := replicate.AssemblePrompts(segs, "锁定中文", "Locked EN")
	if !strings.HasPrefix(got.PromptZH, "【锁定形象】\n锁定中文\n\n## 第 1 段 · 原片 0.0–15.0 秒\n\nZH1") {
		t.Fatalf("zh=%q", got.PromptZH)
	}
	if strings.Count(got.PromptZH, "【锁定形象】") != 1 {
		t.Fatal("combined zh repeated the lock")
	}
	if !strings.Contains(got.PromptEN, "（本段未解析出该语言提示词）") {
		t.Fatalf("en=%q", got.PromptEN)
	}
	if !strings.HasPrefix(got.SegmentZH[1], "【锁定形象】\n锁定中文\n\n") || !strings.Contains(got.SegmentEN[1], "（本段未解析出该语言提示词）") {
		t.Fatalf("seg zh=%q en=%q", got.SegmentZH[1], got.SegmentEN[1])
	}
	if strings.Contains(got.Analysis, "【锁定形象】") || !strings.Contains(got.Analysis, "## 第 2 段 · 原片 15.0–30.0 秒") {
		t.Fatalf("analysis=%q", got.Analysis)
	}

	one := replicate.AssemblePrompts([]replicate.SegmentDraft{{Index: 1, Start: 0, End: 8, PromptEN: "ONLY", PromptZH: "仅此", Analysis: "RAW"}}, "", "")
	if one.PromptEN != "ONLY" || one.PromptZH != "仅此" || strings.Contains(one.PromptZH, "第 1 段") {
		t.Fatalf("single %+v", one)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/replicate/ -count=1 -run "TestParseLock|TestAssemblePrompts"`

Expected: FAIL，函数未定义。

**Step 3: Write minimal implementation**

`internal/replicate/lock.go`：

```go
package replicate

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	lockMissing = "（本段未解析出该语言提示词）"
	lockSystem  = "你只根据这些关键帧写需要跨段保持不变的主体，不写动作、分镜和对白。主体可以是人，也可以是产品。人要写清人数、年龄段、发型、脸部可辨认特征和衣服。产品要写清形状、颜色和材质。中文 40 到 80 字，英文 25 到 50 词，两边说同一件事。不要写 @图片 @视频 @音频。"
	lockUser    = "请只输出下面两块，不要加分析：\n\n## 锁定形象 (中文)\n```\n[中文锁定]\n```\n\n## 锁定形象 (English)\n```\n[English lock]\n```"
)

var (
	reLockZH = regexp.MustCompile("(?s)## 锁定形象 \\(中文\\)\\s*```\\s*(.*?)\\s*```")
	reLockEN = regexp.MustCompile("(?s)## 锁定形象 \\(English\\)\\s*```\\s*(.*?)\\s*```")
)

// LockPrompts are the system and user texts for the appearance call.
func LockPrompts() (system, user string) {
	return lockSystem, lockUser
}

// ParseLock extracts the two fenced appearance blocks.
func ParseLock(text string) (zh, en string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	zh = firstMatch(text, reLockZH)
	en = firstMatch(text, reLockEN)
	return zh, en
}

// SegmentDraft is one segment's parsed model output before files are written.
type SegmentDraft struct {
	Index    int
	Start    float64
	End      float64
	PromptEN string
	PromptZH string
	Analysis string
}

// Assembled is the combined document plus per-segment prompt bodies.
type Assembled struct {
	PromptEN  string
	PromptZH  string
	Analysis  string
	SegmentEN []string
	SegmentZH []string
}

// AssemblePrompts builds saved prompt text.
// Multiple segments get one lock at the top of the combined file, and the same lock at the top of each segment file.
func AssemblePrompts(segs []SegmentDraft, lockZH, lockEN string) Assembled {
	out := Assembled{}
	multi := len(segs) > 1
	var en, zh, analysis strings.Builder
	if multi && strings.TrimSpace(lockZH) != "" {
		fmt.Fprintf(&zh, "【锁定形象】\n%s\n\n", strings.TrimSpace(lockZH))
	}
	if multi && strings.TrimSpace(lockEN) != "" {
		fmt.Fprintf(&en, "[Locked appearance]\n%s\n\n", strings.TrimSpace(lockEN))
	}
	for i, seg := range segs {
		bodyEN := strings.TrimSpace(seg.PromptEN)
		bodyZH := strings.TrimSpace(seg.PromptZH)
		if bodyEN == "" {
			bodyEN = lockMissing
		}
		if bodyZH == "" {
			bodyZH = lockMissing
		}
		segEN := bodyEN
		segZH := bodyZH
		if multi {
			if strings.TrimSpace(lockEN) != "" {
				segEN = fmt.Sprintf("[Locked appearance]\n%s\n\n%s", strings.TrimSpace(lockEN), bodyEN)
			}
			if strings.TrimSpace(lockZH) != "" {
				segZH = fmt.Sprintf("【锁定形象】\n%s\n\n%s", strings.TrimSpace(lockZH), bodyZH)
			}
			header := fmt.Sprintf("## 第 %d 段 · 原片 %.1f–%.1f 秒\n\n", seg.Index, seg.Start, seg.End)
			if i > 0 {
				en.WriteString("\n\n")
				zh.WriteString("\n\n")
				analysis.WriteString("\n\n")
			}
			en.WriteString(header)
			en.WriteString(bodyEN)
			zh.WriteString(header)
			zh.WriteString(bodyZH)
			analysis.WriteString(header)
			analysis.WriteString(strings.TrimSpace(seg.Analysis))
		}
		out.SegmentEN = append(out.SegmentEN, segEN)
		out.SegmentZH = append(out.SegmentZH, segZH)
	}
	if !multi {
		out.PromptEN = strings.TrimSpace(segs[0].PromptEN)
		out.PromptZH = strings.TrimSpace(segs[0].PromptZH)
		if out.PromptEN == "" {
			out.PromptEN = lockMissing
		}
		if out.PromptZH == "" {
			out.PromptZH = lockMissing
		}
		out.Analysis = strings.TrimSpace(segs[0].Analysis)
		out.SegmentEN = []string{out.PromptEN}
		out.SegmentZH = []string{out.PromptZH}
		return out
	}
	out.PromptEN = en.String()
	out.PromptZH = zh.String()
	out.Analysis = analysis.String()
	return out
}
```

单段且某一语言为空时，段文件和总稿都只写说明句，不加锁定标题。多段缺语言时，段文件是锁定形象加说明句，总稿在标题下只写说明句。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/replicate/ -count=1 -run "TestParseLock|TestAssemblePrompts"`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/replicate/lock.go internal/replicate/lock_test.go
git commit -m "拼接分段提示词并固定锁定形象原文。"
```

---

### Task 5: 在时间窗内抽帧

**Files:**
- Modify: `internal/replicate/frames.go`
- Test: `internal/replicate/segments_test.go`

**Step 1: Write the failing test**

追加到 `segments_test.go`：

```go
func TestSpanTimestamps(t *testing.T) {
	got := replicate.SpanTimestamps(replicate.Span{Start: 15, End: 24}, 8)
	if len(got) != 8 {
		t.Fatalf("len=%d", len(got))
	}
	if math.Abs(got[0]-16) > 1e-9 || math.Abs(got[7]-23) > 1e-9 {
		t.Fatalf("%v", got)
	}
}
```

9 秒窗口、8 帧时，局部时间是 1,2,…,8，平移 15 秒后为 16…23。

**Step 2: Run test to verify it fails**

Run: `go test ./internal/replicate/ -count=1 -run TestSpanTimestamps`

Expected: FAIL，`SpanTimestamps` 未定义。

**Step 3: Write minimal implementation**

在 `frames.go` 的 `FrameTimestamps` 后面增加：

```go
// SpanTimestamps samples n points inside span and shifts them onto the source timeline.
func SpanTimestamps(span Span, n int) []float64 {
	local := FrameTimestamps(span.End-span.Start, n)
	for i := range local {
		local[i] += span.Start
	}
	return local
}
```

把 `ExtractFrames` 里的 `stamps := FrameTimestamps(dur, n)` 抽成可复用循环不强制。新增：

```go
// ExtractSpanFrames writes frame_000.jpg… for one span into outDir.
func ExtractSpanFrames(ctx context.Context, bins media.FFmpegBins, videoPath string, span Span, n int, outDir string, progress func(int, string)) ([]string, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	if bins.FFmpeg == "" {
		bin, err := media.ResolveFFmpeg("", "")
		if err != nil {
			return nil, fmt.Errorf("未找到 ffmpeg: %w", err)
		}
		bins.FFmpeg = bin
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	q := fmt.Sprintf("%d", (100-frameQuality)/10+1)
	var frames []string
	stamps := SpanTimestamps(span, n)
	for i, ts := range stamps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out := filepath.Join(outDir, fmt.Sprintf("frame_%03d.jpg", i))
		_, err := media.Run(ctx, bins.FFmpeg, "-y", "-ss", fmt.Sprintf("%.3f", ts), "-i", videoPath, "-vframes", "1", "-q:v", q, out)
		if err != nil {
			continue
		}
		if st, err := os.Stat(out); err == nil && st.Size() > 0 {
			frames = append(frames, out)
		}
	}
	return frames, nil
}
```

现有 `ExtractFrames` 保持不动，短路径改由 `Run` 调用 `ExtractSpanFrames`。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/replicate/ -count=1 -run TestSpanTimestamps`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/replicate/frames.go internal/replicate/segments_test.go
git commit -m "按分段时间窗抽取关键帧。"
```

---

### Task 6: 把识别、分段、锁定形象接进 Run

**Files:**
- Modify: `internal/replicate/replicate.go`
- Modify: `internal/replicate/replicate_test.go`

**Step 1: Write the failing tests**

在 `TestRunEndToEnd` 的 `replicate.Run` 参数里增加：

```go
Transcribe: func(_, outPath string) error {
    return os.WriteFile(outPath, []byte("1\n00:00:00,000 --> 00:00:01,000\n你好\n\n"), 0o644)
},
```

断言用户消息含 `你好`，且输出目录没有 `character_ref.jpg`，提示词不含 `【锁定形象】`。从已记录的请求里取出文本块：

```go
var parts []struct {
    Type string `json:"type"`
    Text string `json:"text"`
}
_ = json.Unmarshal(got.Body.Messages[1].Content, &parts)
if len(parts) != 5 || !strings.Contains(parts[0].Text, "你好") {
    t.Fatalf("user parts=%d", len(parts))
}
if _, err := os.Stat(filepath.Join(dir, "out", "character_ref.jpg")); !os.IsNotExist(err) {
    t.Fatal("short run saved a character ref")
}
```

现有「1 个文本 + 4 张图」断言改为上面这一条。系统提示仍是标准模板。

再加 `TestRunUsesSubtitleFailure`：3 秒测试视频，`Transcribe` 返回 `fmt.Errorf("连不上")`。期望错误含 `字幕提取失败`，假视觉服务收到 0 次请求。用一个计数器包住 `fakeServer`。

再加 `TestRunLongVideoSegments`：`duration=16` 的 lavfi 视频，`Frames: 2`，`SegmentSec: 15`。`Transcribe` 写入两句：`00:00:01,000 --> 00:00:02,000` 文本 `早句`，`00:00:14,500 --> 00:00:15,500` 文本 `跨句`。假服务按调用次数返回：

1. 锁定形象的两块围栏，中文 `黑发。`，英文 `Black hair.`
2. 和 3. 现有 `sampleReply`

断言：

- 调用 3 次。
- 后两次用户文本都含 `跨句` 和 `黑发。`；只有第二次含 `早句`。
- 后两次图片列表的第一张相同，且等于 `character_ref.jpg` 的字节。
- `segment_01/prompt_zh.txt` 与 `segment_02/prompt_zh.txt` 都以同一段 `【锁定形象】\n黑发。` 开头。
- 根上 `prompt_zh.txt` 里 `【锁定形象】` 只出现一次，并含 `## 第 2 段 · 原片 15.0–16.0 秒`。
- 存在 `subtitles.srt`，其内容仍含 `说话人` 前缀如果测试字幕写了前缀；本测试字幕不写前缀，只断言文件存在且含 `跨句`。
- `sora2_analysis.md` 不含 `【锁定形象】`。

假服务要记下每一次请求，不要只留最后一次。把 `captured` 扩成切片，或在测试里用 `var calls []captured`。

**Step 2: Run test to verify it fails**

Run: `go test ./internal/replicate/ -count=1 -run "TestRunEndToEnd|TestRunUsesSubtitleFailure|TestRunLongVideoSegments"`

Expected: FAIL。短视频测试在用户文本里找不到 `你好`，或识别函数被忽略后去连真实 ASR。

**Step 3: Write minimal implementation**

扩展 `Params` 和 `Result`：

```go
type Params struct {
    // existing fields
    SegmentSec int
    ASR        asr.Options
    // Transcribe writes an SRT to outPath. Nil uses asr.Transcribe with ASR.
    Transcribe func(mediaPath, outPath string) error
}

type SegmentResult struct {
    Index      int
    StartSec   float64
    EndSec     float64
    Frames     []string
    Analysis   string
    PromptEN   string
    PromptZH   string
    FrameStart int
}

type Result struct {
    // existing fields describe the combined prompt
    SubtitlePath     string
    CharacterLockZH  string
    CharacterLockEN  string
    CharacterRef     string
    Segments         []SegmentResult
}
```

用新的 `Run` 替换现有流程，保留 `Validate`、输出目录规则和 `ParsePrompts` / `PickPrompt`：

1. 校验、确认视频存在、创建输出目录。
2. `segSec := NormalizeSegmentSec(p.SegmentSec)`，`n := ClampFrames(p.Frames)`。
3. `dur, _ := media.ProbeDurationSec`。`spans := PlanSegments(dur, segSec)`。进度 8，文字「正在准备分段…」。
4. 进度 12，文字「正在提取字幕…」。`outPath := filepath.Join(outDir, "subtitles.srt")`。`Transcribe == nil` 时调用 `asr.RequireReady(p.ASR)` 再 `asr.Transcribe(video, outPath, p.ASR)`。失败返回 `fmt.Errorf("字幕提取失败: %w", err)`。读文件失败同样包上这个前缀。进度 20。
5. `cues := ParseSRT(string(raw))`。
6. 对每个 span 调用 `ExtractSpanFrames`。多段时目录为 `segment_01` 这种两位序号；一段时目录就是输出根目录。某一段帧数为 0：返回 `fmt.Errorf("第 %d 段无法提取视频帧，请确保已安装 ffmpeg", i+1)`。多段时抽帧进度落在 20–40，一段时落在 20–55。
7. `multi := len(spans) > 1`。多段时把第一段第一张帧复制为 `character_ref.jpg`。取每段第一张，最多 8 张，调用 `client.Analyze(ctx, system, user, refs)`，提示词来自 `LockPrompts()`。进度 50，文字「正在锁定形象…」。调用失败或 `ParseLock` 任一侧为空：`fmt.Errorf("锁定形象失败: %w", err)`，空解析用 `fmt.Errorf("锁定形象失败: 未解析出中英文")`。
8. 逐段 `BuildSegmentUserPrompt`。多段时图片为 `append([]string{refPath}, frames...)`。`client.Analyze` 失败返回 `fmt.Errorf("第 %d 段分析失败: %w", i+1, err)`。用 `ParsePrompts` 得到中英文。分析进度在多段时均分 50–90，一段时均分 55–90。
9. `AssemblePrompts` 后写根上的 `prompt_en.txt`、`prompt_zh.txt`、`sora2_analysis.md`。多段时再写各段目录里的这三份，段里的中英文用 `SegmentZH` / `SegmentEN`，分析用该段原始回复。一段时不建 `segment_01`。
10. `Result.Frames` 按时间拼全部帧。`Result.PromptEN/ZH/Analysis` 是总稿。`Prompt` 仍用 `PickPrompt`。`Segments[i].PromptZH` 使用已经贴好锁定形象的段文本。`FrameStart` 是该段在总列表中的下标。进度 100。

字幕文件保持识别函数写下的原文，不要在 `Run` 里再改。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/replicate/ -count=1`

Expected: PASS。无 ffmpeg 时两个跑视频的测试 Skip，纯函数测试仍 PASS。

**Step 5: Commit**

```bash
git add internal/replicate/replicate.go internal/replicate/replicate_test.go
git commit -m "爆款复刻按段分析并引用整片字幕。"
```

---

### Task 7: 记住分段时长

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing test**

在 `TestReplicateDefaultsAndRoundTrip` 里断言 `cfg.Replicate.SegmentSec == 15`。保存前设 `cfg.Replicate.SegmentSec = 10`，读回后仍为 10。

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -count=1 -run TestReplicateDefaultsAndRoundTrip`

Expected: FAIL，没有 `SegmentSec` 字段，或默认不是 15。

**Step 3: Write minimal implementation**

`Replicate` 增加：

```go
SegmentSec int `yaml:"segment_sec" json:"segment_sec"` // 10 or 15
```

`Default()` 里设 `SegmentSec: 15`。

旧配置文件没有这个字段时，`Load` 先 `Default()` 再反序列化，缺省仍是 15。不要在 `Load` 里把已保存的 10 改回 15。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -count=1 -run TestReplicateDefaultsAndRoundTrip`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "记住爆款复刻的分段时长。"
```

---

### Task 8: 桌面页

**Files:**
- Modify: `internal/ui/page_replicate.go`

这一文件有 `//go:build fyne_gui`。不要为它新开一套 GUI 测试。行为由 Task 6 的核心测试覆盖。

**Step 1: 加分段时长**

在关键帧数选择旁边加 `widget.NewSelect([]string{"10", "15"}, nil)`，标签「分段时长」。初始值 `strconv.Itoa(replicate.NormalizeSegmentSec(state.Cfg.Replicate.SegmentSec))`。

`startJob` 里读取该值，写入 `params.SegmentSec`，并填：

```go
ASR: asr.Options{
    Backend:       state.Cfg.ASR.Backend,
    APIURL:        state.Cfg.ASR.APIURL,
    FireRedAPIURL: state.Cfg.ASR.FireRedAPIURL,
    BailianAPIKey: state.Cfg.ASR.APIKey,
    Hotword:       state.Cfg.ASR.Hotword,
    EnableSpk:     state.Cfg.ASR.EnableSpk,
},
```

`asr.Options` 的百炼字段名是 `BailianAPIKey`，对应配置 `ASR.APIKey`。开始前把 `state.Cfg.Replicate.SegmentSec` 设为规范化后的值，并 `config.Save(state.CfgPath, state.Cfg)`。保存失败只记到 `state.Log`，不阻止分析。

**Step 2: 结果切换**

增加选择框，默认「整片」。`showResult` 在多段时追加「第 N 段 · 起–止秒」，一段时只有「整片」。

- 整片：三个文本框用总稿，关键帧用 `res.Frames`。
- 某一段：文本框用该段已贴好锁定形象的中英文和该段 `Analysis`，关键帧只用该段 `Frames`。
- 切换时直接覆盖文本框，不保留未保存编辑。

多段成功时状态行：

```text
完成 · 各段提示词已写入同一段锁定形象。到即梦或 Sora 分段生成时，请每一段都上传 <CharacterRef>
```

一段成功时保持现在的「完成 · 目录」。

页面副标题改为：上传视频，按 10 或 15 秒分段抽帧；先提取整片字幕，再逐段生成提示词。

**Step 3: 编译**

Run: `go test ./internal/replicate/ ./internal/config/ ./internal/webui/ -count=1`

Expected: PASS。桌面文件不在这些包里。若本机有 Fyne 构建标签，再跑 `go test -tags fyne_gui ./internal/ui/ -count=1`。没有标签或依赖缺失就记下，不要为了编译去改无关页面。

**Step 4: Commit**

```bash
git add internal/ui/page_replicate.go
git commit -m "桌面爆款复刻可选段长并查看各段结果。"
```

---

### Task 9: 网页

**Files:**
- Modify: `internal/webui/replicate.go`
- Modify: `internal/webui/index.html`
- Modify: `internal/webui/replicate_test.go`

**Step 1: Write the failing test**

`TestIndexHasReplicateTab` 增加断言，HTML 含 `id="repSegmentSec"`，并且有 `<option value="10">` 与 `<option value="15" selected>` 或默认选中 15 的等价写法。

`TestReplicateRunRequiresAPIKey` 保持不变。它在进识别之前就会因缺少 API Key 返回 400。

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webui/ -count=1 -run TestIndexHasReplicateTab`

Expected: FAIL，页面没有段长选择。

**Step 3: Write minimal implementation**

`handleReplicateRun` 的 JSON 增加 `SegmentSec int \`json:"segment_sec"\``。写入 `params.SegmentSec` 和与桌面相同的 `asr.Options`。规范化后写回 `s.cfg.Replicate.SegmentSec` 并 `config.Save(s.cfgPath, s.cfg)`。保存失败返回 500。

成功 JSON 在现有字段外增加：

```go
"subtitle_path":     res.SubtitlePath,
"character_lock_zh": res.CharacterLockZH,
"character_lock_en": res.CharacterLockEN,
"character_ref":     res.CharacterRef,
"segments":          segments,
```

每个 segment：`index`、`start_sec`、`end_sec`、`frame_start`、`frame_count`、`prompt_en`、`prompt_zh`、`analysis`。`prompt_en` 仍是总稿。`/api/replicate/frame?i=` 仍按下标取 `res.Frames`。

`index.html` 在关键帧数同一行增加：

```html
<div>
  <label>分段时长</label>
  <select id="repSegmentSec">
    <option value="10">10 秒</option>
    <option value="15" selected>15 秒</option>
  </select>
</div>
```

关键帧标题上方增加 `<select id="repSegmentPick"></select>`。`runReplicate` 把 `segment_sec: +$('repSegmentSec').value || 15` 放进请求。返回后：

- 填充 `repSegmentPick`：第一项「整片」，其后每段「第 N 段 · start–end 秒」。
- 选整片时文本框用总稿，图片用 `0 .. res.frames-1`。
- 选某段时文本框用该段的 `prompt_en`、`prompt_zh`、`analysis`，图片用 `frame_start` 起的 `frame_count` 张，地址仍是 `/api/replicate/frame?i=`。
- 多段且有 `character_ref` 时，`repHint` 写明锁定形象和参考图路径。

设置页读配置时，若 `cfg.replicate.segment_sec` 是 10，就把 `#repSegmentSec` 设为 10。现有设置保存会把整个 config 交回去；给 replicate 对象补上 `segment_sec`，避免保存设置时把 10 写丢。查找 `replicate: {` 的那一段，加入 `segment_sec: state?.cfg?.replicate?.segment_sec || 15`。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/webui/ ./internal/replicate/ ./internal/config/ -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/webui/replicate.go internal/webui/index.html internal/webui/replicate_test.go
git commit -m "网页爆款复刻可选段长并切换各段结果。"
```

---

## 做完后的核对

- `go test ./internal/replicate/ ./internal/config/ ./internal/webui/ -count=1` 通过。
- 15 秒以内：一次视觉调用，有字幕参考，没有 `character_ref.jpg`。
- 约 16 秒且段长 15：一次锁定形象加两次分段分析，跨切点的句子出现在相邻两段，总稿只贴一次锁定形象。
- 段长 10 的切分表与规格里的例子一致。
- 不改脚本、原创、片头页面。
