package claude

import (
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

// PackageCodec owns Claude package manifest and agent serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:       Target,
		ManifestPath: ".claude-plugin/plugin.json",
		AgentRoot:    "agents",
		Capabilities: append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:     manifest,
		Agent:        markdownAgent,
	}
}

func markdownAgent(asset model.NormalizedAsset) ([]byte, string, error) {
	frontmatter := make(map[string]any, len(asset.Content.Frontmatter))
	for key, value := range asset.Content.Frontmatter {
		if key == "sandbox_mode" {
			return nil, "", &packageoutput.UnsupportedAgentFieldError{Target: Target, Field: key}
		}
		frontmatter[key] = value
	}
	data, err := packageoutput.Markdown(frontmatter, asset.Content.Body)
	return data, ".md", err
}

func manifest(pkg model.NormalizedPackage) ([]byte, error) {
	values := packageoutput.ManifestBase(pkg)
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "author", "license", "homepage", "repository", "keywords")
	if value, ok := values["author"]; ok {
		values["author"] = packageoutput.PersonMetadata(value)
	}
	values["$schema"] = "https://json.schemastore.org/claude-code-plugin-manifest.json"
	return packageoutput.ManifestJSON(values)
}
