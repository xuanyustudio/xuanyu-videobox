package replicate

import (
	_ "embed"
	"fmt"
	"strings"
)

// System / user prompts ported verbatim from the Python project (qwen3vl.py
// SORA2_SYSTEM_PROMPT / SORA2_USER_PROMPT_TEMPLATE), plus the two SORA2
// prompt-expert markdown documents and a Seedance 2.0 15s template.

//go:embed prompts/sora2_system.md
var sora2SystemPrompt string

//go:embed prompts/sora2_user.md
var sora2UserTemplate string

//go:embed prompts/replicate_sora2_expert.md
var replicateExpertPrompt string

//go:embed prompts/sora2_expert.md
var sora2ExpertPrompt string

//go:embed prompts/seedance2_system.md
var seedance2SystemPrompt string

//go:embed prompts/seedance2_user.md
var seedance2UserTemplate string

// Template selects which system prompt drives the vision model.
type Template string

const (
	// TemplateStandard is the exact prompt the Python app uses (default).
	TemplateStandard Template = "standard"
	// TemplateReplicateExpert uses 复刻SORA2视频提示词专家.md as the system prompt.
	TemplateReplicateExpert Template = "replicate_expert"
	// TemplateSora2Expert uses SORA2视频提示词专家.md (LangGPT) as the system prompt.
	TemplateSora2Expert Template = "sora2_expert"
	// TemplateSeedance2 writes a paste-ready 15s Seedance 2.0 (Jimeng) prompt.
	TemplateSeedance2 Template = "seedance2"
)

// TemplateOption is a UI label/value pair.
type TemplateOption struct {
	ID    Template
	Label string
}

// TemplateOptions lists selectable prompt templates in display order.
func TemplateOptions() []TemplateOption {
	return []TemplateOption{
		{TemplateStandard, "标准（与原版一致）"},
		{TemplateReplicateExpert, "复刻SORA2视频提示词专家"},
		{TemplateSora2Expert, "SORA2视频提示词专家"},
		{TemplateSeedance2, "Seedance 2.0（15秒）"},
	}
}

// TemplateByLabel maps a UI label back to its ID (unknown → standard).
func TemplateByLabel(label string) Template {
	for _, o := range TemplateOptions() {
		if o.Label == label {
			return o.ID
		}
	}
	return TemplateStandard
}

// SystemPrompt returns the system prompt text for t.
func SystemPrompt(t Template) string {
	var s string
	switch t {
	case TemplateReplicateExpert:
		s = replicateExpertPrompt
	case TemplateSora2Expert:
		s = sora2ExpertPrompt
	case TemplateSeedance2:
		s = seedance2SystemPrompt
	default:
		s = sora2SystemPrompt
	}
	return normalizeNewlines(s)
}

// BuildUserPrompt fills the user template for t, appending a duration hint
// when ffprobe succeeds. Seedance 2.0 always maps the source onto a 15s timeline.
func BuildUserPrompt(t Template, numFrames int, durationSec float64) string {
	src := sora2UserTemplate
	if t == TemplateSeedance2 {
		src = seedance2UserTemplate
	}
	p := strings.ReplaceAll(normalizeNewlines(src), "{num_frames}", fmt.Sprintf("%d", numFrames))
	if t == TemplateSeedance2 {
		if durationSec > 0 {
			p += fmt.Sprintf("\n\n源视频实际时长: %.1f秒。请把叙事压缩/映射为恰好 15 秒的 Seedance 时间轴，不要按源时长写秒数。", durationSec)
		} else {
			p += "\n\n源视频时长未知。请生成恰好 15 秒的 Seedance 时间轴。"
		}
		return p
	}
	if durationSec > 0 {
		p += fmt.Sprintf("\n\n视频实际时长: %.1f秒，请根据此时长分配各场景时间。", durationSec)
	}
	return p
}

func normalizeNewlines(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}
