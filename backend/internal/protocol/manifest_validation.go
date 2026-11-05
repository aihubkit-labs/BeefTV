package protocol

import (
	"fmt"
	"strings"

	"infinite-canvas/backend/internal/buildinfo"
)

func validateManifestRequest(rules []ManifestValidation, request GenerationRequest, variables ...map[string]any) error {
	if draftTaskID, ok := request.Extra["draft_task_id"].(string); len(rules) == 0 || (ok && strings.TrimSpace(draftTaskID) != "") {
		return nil
	}
	env := map[string]any{"request": manifestRequestValues(request)}
	if len(variables) > 0 {
		env["vars"] = variables[0]
	}
	for _, rule := range rules {
		value, err := evaluateManifestValue(rule.Assert, env)
		if err != nil {
			return fmt.Errorf("协议参数校验表达式错误：%w", err)
		}
		if !manifestTruthy(value) {
			return fmt.Errorf("%s", buildinfo.DisplayText(strings.TrimSpace(rule.Message)))
		}
	}
	return nil
}
