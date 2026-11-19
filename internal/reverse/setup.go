package reverse

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/narratoai/narrato-go/internal/config"
)

// WriteAgentConfig writes an isolated Pi models.json / settings.json
// that points at the vision model already configured in Narrato.
func WriteAgentConfig(cfg *config.Config) (agentDir string, err error) {
	agentDir = DataDir()
	if err = os.MkdirAll(filepath.Join(agentDir, "skills", "video-reverse"), 0o755); err != nil {
		return "", err
	}
	rep := cfg.Replicate
	base := rep.BaseURL
	if base == "" {
		base = "https://api-inference.modelscope.cn/v1"
	}
	model := rep.Model
	if model == "" {
		model = "Qwen/Qwen3-VL-8B-Instruct"
	}
	models := map[string]any{
		"providers": map[string]any{
			"narrato": map[string]any{
				"baseUrl": base,
				"api":     "openai-completions",
				"apiKey":  rep.APIKey,
				"compat": map[string]any{
					"supportsDeveloperRole":   false,
					"supportsReasoningEffort": false,
				},
				"models": []map[string]any{{
					"id":            model,
					"name":          "玄宇视觉",
					"reasoning":     false,
					"input":         []string{"text", "image"},
					"contextWindow": 131072,
					"maxTokens":     32768,
				}},
			},
		},
	}
	raw, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(agentDir, "models.json"), raw, 0o600); err != nil {
		return "", err
	}
	settings := map[string]any{
		"defaultProjectTrust": "always",
		"quietStartup":        true,
	}
	sraw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(agentDir, "settings.json"), sraw, 0o644); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(agentDir, "skills", "video-reverse", "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		return "", err
	}
	return agentDir, nil
}

func ModelID(cfg *config.Config) string {
	m := cfg.Replicate.Model
	if m == "" {
		return "Qwen/Qwen3-VL-8B-Instruct"
	}
	return m
}
