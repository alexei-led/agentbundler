package codex

import (
	"fmt"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateNative},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// PackageCodec owns Codex package manifest and agent serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:       Target,
		ManifestPath: ".codex-plugin/plugin.json",
		AgentRoot:    ".codex/agents",
		Capabilities: append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:     manifest,
		Agent:        codexAgent,
	}
}

func codexAgent(asset model.NormalizedAsset) ([]byte, string, error) {
	name := strings.TrimPrefix(string(asset.Identity), "agent/")
	if value, ok := asset.Content.Frontmatter["name"].(string); ok && value != "" {
		name = value
	}
	description, _ := asset.Content.Frontmatter["description"].(string)
	if description == "" {
		return nil, "", fmt.Errorf("agent %q requires string description frontmatter", asset.Identity)
	}
	lines := []string{fmt.Sprintf("name = %s", tomlString(name)), fmt.Sprintf("description = %s", tomlString(description))}
	if value, ok := asset.Content.Frontmatter["sandbox_mode"].(string); ok && value != "" {
		lines = append(lines, fmt.Sprintf("sandbox_mode = %s", tomlString(value)))
	}
	lines = append(lines, "developer_instructions = "+tomlMultiline(asset.Content.Body), "")
	return []byte(strings.Join(lines, "\n")), ".toml", nil
}

func tomlString(value string) string {
	value = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\b", "\\b", "\t", "\\t", "\n", "\\n", "\f", "\\f", "\r", "\\r").Replace(value)
	return `"` + value + `"`
}

func tomlMultiline(value string) string {
	return "\"\"\"\n" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"\"\"", "\\\"\\\"\\\"") + "\n\"\"\""
}

func manifest(pkg model.NormalizedPackage) ([]byte, error) {
	values := packageoutput.ManifestBase(pkg)
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "author", "license", "homepage", "repository", "keywords", "interface")
	if value, ok := values["author"]; ok {
		values["author"] = packageoutput.PersonMetadata(value)
	}
	values["skills"] = "./skills"
	return packageoutput.ManifestJSON(values)
}
