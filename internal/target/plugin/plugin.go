// Package plugin renders the common single-package native plugin skill subset.
package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

var pluginName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// Render emits a native plugin manifest alongside its skill tree.
func Render(target model.TargetID, manifestPath string, packages []model.NormalizedPackage, manifest map[string]any) (model.TargetPlan, []model.Diagnostic) {
	if len(packages) != 1 {
		return model.TargetPlan{Target: target}, []model.Diagnostic{{Code: "unsupported-package-aggregation", Severity: model.SeverityError, Message: "target-native plugin output requires exactly one package"}}
	}
	if !pluginName.MatchString(string(packages[0].Identity)) {
		return model.TargetPlan{Target: target}, []model.Diagnostic{{Code: "invalid-plugin-name", Severity: model.SeverityError, Message: fmt.Sprintf("package identity %q is not a valid native plugin name", packages[0].Identity)}}
	}
	plan, diagnostics := skills.Render(target, "skills", packages)
	if len(diagnostics) != 0 {
		return plan, diagnostics
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return model.TargetPlan{Target: target}, []model.Diagnostic{{Code: "invalid-plugin-manifest", Severity: model.SeverityError, Message: err.Error()}}
	}
	plan.Files = append([]model.PlannedFile{{Path: model.RelativePath(manifestPath), Bytes: append(data, '\n')}}, plan.Files...)
	return plan, nil
}
