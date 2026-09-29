package replicate

import (
	_ "embed"
	"fmt"
	"strings"
)

// System / user prompts ported verbatim from the Python project (qwen3vl.py
// SORA2_SYSTEM_PROMPT / SORA2_USER_PROMPT_TEMPLATE), plus the two SORA2
// prompt-expert markdown documents shipped alongside it.

//go:embed prompts/sora2_system.md
var sora2SystemPrompt string

//go:embed prompts/sora2_user.md
var sora2UserTemplate string

//go:embed prompts/replicate_sora2_expert.md
var replicateExpertPrompt string

//go:embed prompts/sora2_expert.md
var sora2ExpertPrompt string

// Template selects which system prompt drives the vision model.
type Template string

const (
	// TemplateStandard is the exact prompt the Python app uses (default).
	TemplateStandard Template = "standard"
	// TemplateReplicateExpert uses 复刻SORA2视频提示词专家.md as the system prompt.
	TemplateReplicateExpert Template = "replicate_expert"
	// TemplateSora2Expert uses SORA2视频提示词专家.md (LangGPT) as the system prompt.
	TemplateSora2Expert Template = "sora2_expert"
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
	default:
		s = sora2SystemPrompt
	}
	return normalizeNewlines(s)
}

// BuildUserPrompt fills the SORA2 user template, appending the real duration hint
// exactly like the Python version does when ffprobe succeeds.
func BuildUserPrompt(numFrames int, durationSec float64) string {
	p := strings.ReplaceAll(normalizeNewlines(sora2UserTemplate), "{num_frames}", fmt.Sprintf("%d", numFrames))
	if durationSec > 0 {
		p += fmt.Sprintf("\n\n视频实际时长: %.1f秒，请根据此时长分配各场景时间。", durationSec)
	}
	return p
}

func normalizeNewlines(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}
