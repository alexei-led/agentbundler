package pi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"gopkg.in/yaml.v3"
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateEquivalent},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// PackageCodec owns Pi package manifest and subagent serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:       Target,
		ManifestPath: "package.json",
		AgentRoot:    "agents",
		Capabilities: append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:     manifest,
		Agent:        subagent,
	}
}

func subagent(asset model.NormalizedAsset) ([]byte, string, error) {
	values := make(map[string]any, len(asset.Content.Frontmatter))
	for key, value := range asset.Content.Frontmatter {
		if key == "sandbox_mode" {
			return nil, "", &packageoutput.UnsupportedAgentFieldError{Target: Target, Field: key}
		}
		if !validSubagentKey(key) {
			return nil, "", fmt.Errorf("pi subagent frontmatter key %q is invalid", key)
		}
		switch value.(type) {
		case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		default:
			return nil, "", fmt.Errorf("pi subagent frontmatter %q: must be a scalar", key)
		}
		if text, ok := value.(string); ok && strings.ContainsAny(text, "\r\n") {
			return nil, "", fmt.Errorf("pi subagent frontmatter %q: must be single-line", key)
		}
		values[key] = value
	}
	encoded, err := yaml.Marshal(values)
	if err != nil {
		return nil, "", err
	}
	return append(append(append([]byte("---\n"), encoded...), []byte("---\n")...), []byte(asset.Content.Body)...), ".md", nil
}

func validSubagentKey(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func manifest(pkg model.NormalizedPackage) ([]byte, error) {
	values := packageoutput.ManifestBase(pkg)
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "keywords", "license", "homepage", "repository")
	if value, ok := pkg.Metadata["dependencies"]; ok {
		dependencies, err := dependencies(value)
		if err != nil {
			return nil, fmt.Errorf("dependencies: %w", err)
		}
		values["dependencies"] = dependencies
	}
	values["keywords"] = keywords(values["keywords"])
	pi := map[string]any{"skills": []string{"./skills"}}
	if packageoutput.PackageHasAsset(pkg, model.AssetKindAgent) {
		dependencies, ok := values["dependencies"].(map[string]string)
		if !ok || dependencies["pi-subagents"] == "" {
			return nil, fmt.Errorf("pi packages with agents require a non-empty pi-subagents dependency")
		}
		pi["subagents"] = map[string]any{"agents": []string{"./agents"}}
	}
	values["pi"] = pi
	return packageoutput.ManifestJSON(values)
}

func dependencies(value any) (map[string]string, error) {
	values, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	result := make(map[string]string, len(values))
	for name, version := range values {
		if name == "" {
			return nil, fmt.Errorf("package name must not be empty")
		}
		text, ok := version.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("dependency %q must have a non-empty string version", name)
		}
		result[name] = text
	}
	return result, nil
}

func keywords(value any) []string {
	if values, ok := value.([]any); ok {
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	}
	if values, ok := value.([]string); ok {
		return append([]string(nil), values...)
	}
	return []string{"pi-package"}
}
