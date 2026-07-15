package cursor

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "asset.agent", State: model.CapabilityStateNative},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
}

// PackageCodec owns Cursor package manifest and agent serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:       model.TargetCursor,
		ManifestPath: ".cursor-plugin/plugin.json",
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
			return nil, "", &packageoutput.UnsupportedAgentFieldError{Target: model.TargetCursor, Field: key}
		}
		frontmatter[key] = value
	}
	data, err := packageoutput.Markdown(frontmatter, asset.Content.Body)
	return data, ".md", err
}

func manifest(pkg model.NormalizedPackage) ([]byte, error) {
	values := packageoutput.ManifestBase(pkg)
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "displayName", "homepage", "repository", "license", "keywords")
	values["skills"] = "./skills/"
	return packageoutput.ManifestJSON(values)
}
