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
