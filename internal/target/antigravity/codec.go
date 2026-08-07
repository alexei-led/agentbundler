package antigravity

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

var (
	pluginNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	capabilityRules   = []model.CapabilityRule{
		{Key: "asset.agent", State: model.CapabilityStateNative},
		{Key: "asset.hook", State: model.CapabilityStateUnsupported},
		{Key: "asset.native-resource", State: model.CapabilityStateNative},
		{Key: "asset.command", State: model.CapabilityStateUnsupported},
		{Key: "asset.resource", State: model.CapabilityStateNative},
		{Key: "asset.skill", State: model.CapabilityStateNative},
		{Key: "hook.async", State: model.CapabilityStateUnsupported},
		{Key: "hook.command.exec", State: model.CapabilityStateUnsupported},
		{Key: "hook.command.shell", State: model.CapabilityStateUnsupported},
		{Key: "hook.decision.block", State: model.CapabilityStateUnsupported},
		{Key: "hook.decision.rewrite-input", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.notification", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.post-compact", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.post-tool", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.post-tool-failure", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.pre-compact", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.pre-tool", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.prompt-submit", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.session-end", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.session-start", State: model.CapabilityStateUnsupported},
		{Key: "hook.event.stop", State: model.CapabilityStateUnsupported},
		{Key: "hook.failure.closed", State: model.CapabilityStateUnsupported},
		{Key: "hook.matcher.tool-category", State: model.CapabilityStateUnsupported},
	}
)

// PackageCodec owns Antigravity's plugin serialization contract.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:          Target,
		ManifestPath:    "plugin.json",
		AgentRoot:       "agents",
		Capabilities:    append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:        manifest,
		Agent:           markdownAgent,
		NativeResource:  nativeResource,
		ValidatePackage: validatePackage,
	}
}

func manifest(pkg model.NormalizedPackage) ([]byte, error) {
	values := map[string]any{"name": pkg.Identity}
	if description, present := pkg.Metadata["description"]; present {
		text, ok := description.(string)
		if !ok {
			return nil, fmt.Errorf("package %q description must be a string", pkg.Identity)
		}
		values["description"] = text
	}
	return packageoutput.ManifestJSON(values)
}

func markdownAgent(asset model.NormalizedAsset) ([]byte, string, error) {
	keys := make([]string, 0, len(asset.Content.Frontmatter))
	for key := range asset.Content.Frontmatter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key != "name" && key != "description" {
			return nil, "", &packageoutput.UnsupportedAgentFieldError{Target: Target, Asset: asset.Identity, Field: key}
		}
	}
	frontmatter := make(map[string]any, 2)
	for _, key := range []string{"name", "description"} {
		value, ok := asset.Content.Frontmatter[key]
		text, stringValue := value.(string)
		if !ok || !stringValue || text == "" {
			return nil, "", fmt.Errorf("agent %q requires a non-empty string %q field", asset.Identity, key)
		}
		frontmatter[key] = text
	}
	data, err := packageoutput.Markdown(frontmatter, asset.Content.Body)
	return data, ".md", err
}

func nativeResource(asset model.NormalizedAsset) ([]packageoutput.NativeResourceFile, error) {
	if asset.Native != nil {
		return nil, fmt.Errorf("antigravity native resource %q must not declare piExtensions", asset.Identity)
	}
	paths := make([]model.RelativePath, 0, len(asset.Content.Files))
	for path := range asset.Content.Files {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(left, right int) bool { return paths[left] < paths[right] })
	if len(paths) == 0 {
		return nil, fmt.Errorf("antigravity native resource %q has no files", asset.Identity)
	}
	resources := make([]packageoutput.NativeResourceFile, 0, len(paths))
	for _, path := range paths {
		content := asset.Content.Files[path]
		content.Bytes = append([]byte(nil), content.Bytes...)
		content.Origin = model.CloneSourceLocations(content.Origin)
		resources = append(resources, packageoutput.NativeResourceFile{Path: path, Content: content})
	}
	return resources, nil
}

func validatePackage(pkg model.NormalizedPackage) []model.Diagnostic {
	if !pluginNamePattern.MatchString(string(pkg.Identity)) {
		return []model.Diagnostic{{
			Code: "invalid-package-name", Severity: model.SeverityError,
			Message: fmt.Sprintf("antigravity plugin name %q must match ^[A-Za-z0-9_-]+$", pkg.Identity),
			Targets: []model.TargetID{Target},
		}}
	}
	for _, asset := range pkg.Assets {
		if asset.Kind == model.AssetKindHook {
			return []model.Diagnostic{{Code: "unsupported-capability", Severity: model.SeverityError, Message: fmt.Sprintf("package target %q cannot render portable hook asset %q", Target, asset.Identity), Asset: asset.Identity, Targets: []model.TargetID{Target}}}
		}
		if asset.Kind == model.AssetKindAgent {
			if _, _, err := markdownAgent(asset); err != nil {
				if fieldError, ok := err.(*packageoutput.UnsupportedAgentFieldError); ok {
					return []model.Diagnostic{fieldError.Diagnostic()}
				}
				return []model.Diagnostic{{Code: "invalid-agent", Severity: model.SeverityError, Message: err.Error(), Asset: asset.Identity, Targets: []model.TargetID{Target}}}
			}
		}
		if asset.Kind == model.AssetKindNativeResource {
			if _, err := nativeResource(asset); err != nil {
				return []model.Diagnostic{{Code: "invalid-native-resource", Severity: model.SeverityError, Message: err.Error(), Asset: asset.Identity, Targets: []model.TargetID{Target}}}
			}
		}
	}
	return nil
}
