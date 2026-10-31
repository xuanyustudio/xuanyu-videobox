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

func LockPrompts() (system, user string) {
	return lockSystem, lockUser
}

func ParseLock(text string) (zh, en string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	zh = firstMatch(text, reLockZH)
	en = firstMatch(text, reLockEN)
	return zh, en
}

type SegmentDraft struct {
	Index    int
	Start    float64
	End      float64
	PromptEN string
	PromptZH string
	Analysis string
}

type Assembled struct {
	PromptEN  string
	PromptZH  string
	Analysis  string
	SegmentEN []string
	SegmentZH []string
}

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
		if len(segs) == 0 {
			return out
		}
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
