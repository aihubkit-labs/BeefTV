package aihubkit

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"

	"infinite-canvas/backend/internal/workspace"
)

type defaultModelRule struct {
	group, capability, protocol, preferred, label string
}

// Platform onboarding policy lives here, independently of the host's model routing.
var defaultModelRules = []defaultModelRule{
	{"default-企业版", "text", "chat-completion", "", "文本"},
	{"gpt_image_web", "image", "openai-image", "gpt-image-2-w", "图片"},
	{"video_官key", "video", "aihubkit-video", "sd-mini-z", "视频"},
}

var gptVersionPattern = regexp.MustCompile(`(?i)^gpt-(\d+(?:\.\d+)*)(?:o)?(?:$|-)`)

func gptVersion(id string) []int {
	match := gptVersionPattern.FindStringSubmatch(id)
	if match == nil {
		return nil
	}
	parts := strings.Split(match[1], ".")
	version := make([]int, len(parts))
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		version[i] = value
	}
	return version
}

func newerGPT(candidate, current string) bool {
	left, right := gptVersion(candidate), gptVersion(current)
	for i := 0; i < len(left) || i < len(right); i++ {
		var a, b int
		if i < len(left) {
			a = left[i]
		}
		if i < len(right) {
			b = right[i]
		}
		if a != b {
			return a > b
		}
	}
	// Prefer the unqualified model over variants within the same GPT version.
	if len(candidate) != len(current) {
		return len(candidate) < len(current)
	}
	return candidate < current
}

func chooseDefaultModel(rule defaultModelRule, models []CatalogModel) string {
	var candidates []string
	preferred := ""
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		candidates = append(candidates, model.ID)
		if rule.preferred != "" && model.ID == rule.preferred {
			return model.ID
		}
		if rule.capability == "text" && gptVersion(model.ID) != nil && (preferred == "" || newerGPT(model.ID, preferred)) {
			preferred = model.ID
		}
	}
	if preferred != "" {
		return preferred
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[rand.IntN(len(candidates))]
}

func (a *account) groupCatalog(ctx context.Context, group string) ([]CatalogModel, error) {
	if err := a.ensureKey(ctx, group); err != nil {
		return nil, err
	}
	_, key, _, err := a.resolve(credentialPrefix + group)
	if err != nil {
		return nil, err
	}
	models := []CatalogModel{}
	err = a.client.call(ctx, "GET", "/v1/models", nil, key, 0, &models)
	return models, err
}

func (a *account) seedModels(ctx context.Context) error {
	store, err := workspace.NewProviderConfig(a.dir)
	if err != nil {
		return err
	}
	effective, _, err := store.LoadEffectiveModelConfig()
	if err != nil {
		return err
	}
	config := effective.Config
	if config == nil {
		config = map[string]any{}
	}
	// A saved workspace may predate onboarding. Only this marker proves it ran;
	// it also prevents a later explicit deletion from triggering a reimport.
	if config["aihubkitDefaultsInitialized"] == true {
		return nil
	}
	result, err := a.defaultModelConfig(ctx, config)
	if err != nil {
		return err
	}
	b, err := json.Marshal(result.Config)
	if err != nil {
		return err
	}
	if _, err = store.SaveLocalModelConfigRevision(b, effective.Revision); err != nil {
		return err
	}
	a.modelImportWarnings = result.Warnings
	return nil
}

type defaultModelConfigResult struct {
	Config   map[string]any `json:"config"`
	Warnings []string       `json:"warnings"`
}

// Both onboarding and manual initialization rebuild the managed channels.
// This only prepares a snapshot: failures leave the saved config untouched.
func (a *account) defaultModelConfig(ctx context.Context, config map[string]any) (defaultModelConfigResult, error) {
	result := defaultModelConfigResult{Config: config, Warnings: []string{}}
	channels := []any{}
	removed := map[string]bool{}
	existingIDs := map[string]bool{}
	existing, _ := config["channels"].([]any)
	for _, raw := range existing {
		channel, _ := raw.(map[string]any)
		id := stringConfig(channel["id"])
		if strings.HasPrefix(stringConfig(channel["credentialRef"]), credentialPrefix) {
			removed[id] = true
		} else {
			channels = append(channels, raw)
			existingIDs[id] = true
		}
	}
	for _, field := range []string{"model", "textModel", "imageModel", "videoModel", "audioModel", "assistantModel", "mediaReviewModel"} {
		id, _, _ := strings.Cut(stringConfig(config[field]), "::")
		if removed[id] {
			config[field] = ""
		}
	}
	for _, field := range []string{"models", "textModels", "imageModels", "videoModels", "audioModels"} {
		delete(config, field)
	}
	for _, rule := range defaultModelRules {
		if _, ok := a.groups[rule.group]; !ok {
			result.Warnings = append(result.Warnings, rule.label+"：账号不可用分组「"+rule.group+"」，已跳过默认模型导入。")
			continue
		}
		catalog, err := a.groupCatalog(ctx, rule.group)
		if err != nil {
			return defaultModelConfigResult{}, err
		}
		model := chooseDefaultModel(rule, catalog)
		if model == "" {
			result.Warnings = append(result.Warnings, rule.label+"：分组「"+rule.group+"」没有可用模型，已跳过默认模型导入。")
			continue
		}
		id := "aihubkit-" + rule.group
		if existingIDs[id] {
			return defaultModelConfigResult{}, fmt.Errorf("%s默认渠道与已有独立渠道 ID 冲突，请先在模型配置中调整该独立渠道", rule.label)
		}
		channel := map[string]any{"id": id, "name": "aihubkit · " + rule.group, "scope": "user", "enabled": true, "pinned": true, "apiFormat": "openai", "baseUrl": a.client.origin, "apiKey": "", "hasApiKey": true, "credentialRef": credentialPrefix + rule.group, "models": []string{model}, "modelProfiles": []map[string]any{{"model": model, "capability": rule.capability, "protocol": rule.protocol}}}
		channels = append(channels, channel)
		config[rule.capability+"Model"] = id + "::" + model
	}
	config["channels"] = channels
	config["aihubkitDefaultsInitialized"] = true
	return result, nil
}

func stringConfig(value any) string {
	valueString, _ := value.(string)
	return valueString
}
