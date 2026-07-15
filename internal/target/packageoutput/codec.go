package packageoutput

import (
	"fmt"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Codec isolates target-specific package rules and serialization from package
// aggregation and filesystem layout.
type Codec struct {
	Target          model.TargetID
	ManifestPath    string
	AgentRoot       string
	Capabilities    []model.CapabilityRule
	Manifest        func(model.NormalizedPackage) ([]byte, error)
	Agent           func(model.NormalizedAsset) ([]byte, string, error)
	ValidatePackage func(model.NormalizedPackage) []model.Diagnostic
}

// UnsupportedAgentFieldError reports an agent field a target codec cannot encode.
type UnsupportedAgentFieldError struct {
	Target model.TargetID
	Asset  model.AssetID
	Field  string
}

func (e *UnsupportedAgentFieldError) Error() string {
	return fmt.Sprintf("agent %q field %q is unsupported by target %q", e.Asset, e.Field, e.Target)
}

func (e *UnsupportedAgentFieldError) Diagnostic() model.Diagnostic {
	return model.Diagnostic{
		Code:     "unsupported-agent-field",
		Severity: model.SeverityError,
		Message:  e.Error(),
		Hint:     `move "sandbox_mode" from base agent frontmatter to <agent-directory>/.agentbundler/targets/codex.json: {"frontmatterPatch":{"sandbox_mode":"read-only"}}`,
		Asset:    e.Asset,
		Field:    e.Field,
		Targets:  []model.TargetID{e.Target},
	}
}
