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
	if t == TemplateSeedance2 {
		// seedance2_user.md hardcodes "恰好 15 秒" and a 15s beat list.
		// Rewrite those clauses in place so a 10s segment does not keep the 15s clock.
		p = strings.ReplaceAll(p, "恰好 15 秒", fmt.Sprintf("恰好 %d 秒", segmentSec))
		const hardcodedBeats = "压缩进 15 秒五段：0-3 / 3-6 / 6-10 / 10-13 / 13-15"
		p = strings.ReplaceAll(p, hardcodedBeats, fmt.Sprintf("压缩进 %d 秒五段：%s", segmentSec, SeedanceBeats(segmentSec)))
	}
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
