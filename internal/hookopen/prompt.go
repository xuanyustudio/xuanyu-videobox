package hookopen

import (
	"fmt"
	"strings"
)

const hookSystemPrompt = `你是短视频剪辑师，擅长「黄金开头」：把片中最抓眼球的 3–8 秒放到片头，抓住前几秒注意力。
你必须只输出 JSON，不要输出说明文字或代码块标记。
不要编造台词，不要选择片头字幕、片尾、或节奏很慢的铺垫段。`

func buildUserPrompt(transcript string, windows []scoredWindow, duration float64) string {
	var b strings.Builder
	b.WriteString("# 爆点开头候选提取\n\n")
	b.WriteString("从下面的台词和预选片段中，选出 3 个最适合放到片头的 3–8 秒片段。\n")
	b.WriteString("选取原则（优先）：\n")
	b.WriteString("1. 强反转：身份揭露、真相反转、关系颠覆\n")
	b.WriteString("2. 强冲突：对峙、怒吼、撕破脸、极端情绪\n")
	b.WriteString("3. 强悬念：关键时刻戛然而止、意外发现\n")
	b.WriteString("4. 强反差：前后境遇骤变、表面与事实相反\n")
	b.WriteString("时长必须在 3 到 8 秒之间，推荐 4–7 秒。避开片头铺垫和片尾，也不要剧透最终结局。\n")
	b.WriteString("3 个候选必须来自不同时段，按推荐程度排序，rank=1 最推荐。\n\n")
	fmt.Fprintf(&b, "片长约 %.2f 秒。\n\n", duration)
	b.WriteString("## 台词转写\n\n")
	if strings.TrimSpace(transcript) == "" {
		b.WriteString("（没有识别到台词，请主要根据画面候选判断）\n\n")
	} else {
		b.WriteString(transcript)
		if !strings.HasSuffix(transcript, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if len(windows) > 0 {
		b.WriteString("## 程序预选的高能窗口（可微调起止，但不要大幅偏离）\n\n")
		for i, w := range windows {
			fmt.Fprintf(&b, "%d. %.2f–%.2f 秒", i+1, w.Start, w.End)
			if w.Quote != "" {
				fmt.Fprintf(&b, "  台词：%s", w.Quote)
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString("请严格按下面格式输出：\n")
	b.WriteString(`{"candidates":[{"rank":1,"hook_type":"强反转","reason":"一句话理由","start_sec":12.4,"end_sec":17.9,"quote":"这段原台词或画面要点","is_recommended":true},{"rank":2,"hook_type":"强冲突","reason":"理由","start_sec":40.0,"end_sec":45.5,"quote":"…","is_recommended":false},{"rank":3,"hook_type":"强悬念","reason":"理由","start_sec":88.0,"end_sec":93.2,"quote":"…","is_recommended":false}]}`)
	b.WriteString("\n")
	return b.String()
}
