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
	{Key: "asset.hook", State: model.CapabilityStateNative},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "hook.async", State: model.CapabilityStateNative},
	{Key: "hook.command.exec", State: model.CapabilityStateNative},
	{Key: "hook.command.shell", State: model.CapabilityStateNative},
	{Key: "hook.decision.block", State: model.CapabilityStateNative},
	{Key: "hook.decision.rewrite-input", State: model.CapabilityStateNative},
	{Key: "hook.event.notification", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.post-compact", State: model.CapabilityStateNative},
	{Key: "hook.event.post-tool", State: model.CapabilityStateNative},
	{Key: "hook.event.post-tool-failure", State: model.CapabilityStateNative},
	{Key: "hook.event.pre-compact", State: model.CapabilityStateNative},
	{Key: "hook.event.pre-tool", State: model.CapabilityStateNative},
	{Key: "hook.event.prompt-submit", State: model.CapabilityStateNative},
	{Key: "hook.event.session-end", State: model.CapabilityStateNative},
	{Key: "hook.event.session-start", State: model.CapabilityStateNative},
	{Key: "hook.event.stop", State: model.CapabilityStateNative},
	{Key: "hook.failure.closed", State: model.CapabilityStateNative},
	{Key: "hook.matcher.tool-category", State: model.CapabilityStateNative},
}

var separateCapabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateEquivalent},
	{Key: "asset.hook", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
}

// PackageCodec owns separate Pi package manifest and subagent serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:       Target,
		ManifestPath: "package.json",
		AgentRoot:    "agents",
		Capabilities: append([]model.CapabilityRule(nil), separateCapabilityRules...),
		Manifest:     manifest,
		Agent:        subagent,
	}
}

func aggregatePackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:          Target,
		ManifestPath:    "package.json",
		AgentRoot:       "agents",
		HookPayloadRoot: "hooks/payloads",
		Capabilities:    append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:        aggregateManifest,
		Agent:           subagent,
		Hooks:           hookManifest,
		ValidatePackage: validateHookSemantics,
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
	return packageManifest(pkg, false)
}

func aggregateManifest(pkg model.NormalizedPackage) ([]byte, error) {
	return packageManifest(pkg, true)
}

func packageManifest(pkg model.NormalizedPackage, registerHooks bool) ([]byte, error) {
	values := packageoutput.ManifestBase(pkg)
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "keywords", "license", "homepage", "repository")
	if value, ok := pkg.Metadata["dependencies"]; ok {
		dependencies, err := dependencies(value)
		if err != nil {
			return nil, fmt.Errorf("dependencies: %w", err)
		}
		if len(dependencies) != 0 {
			values["dependencies"] = dependencies
		}
	}
	values["keywords"] = keywords(values["keywords"])
	pi := map[string]any{"skills": []string{"./skills"}}
	if registerHooks {
		pi["extensions"] = []string{"./extensions/agentbundler-hooks.ts"}
	}
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

type hookConfigV1 struct {
	Version int                    `json:"version"`
	Hooks   []model.HookDescriptor `json:"hooks"`
}

func hookManifest(input packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
	descriptors := make([]model.HookDescriptor, 0, len(input.Hooks()))
	for _, hook := range input.Hooks() {
		descriptor := hook.Descriptor()
		if descriptor.Handler.Mode == model.HookHandlerModeShell && descriptor.Handler.Arguments == nil {
			descriptor.Handler.Arguments = []model.HookArgument{}
		}
		for index, argument := range descriptor.Handler.Arguments {
			if argument.PackageFile == nil {
				continue
			}
			packagePath, ok := piPackageFilePath(hook, *argument.PackageFile)
			if !ok {
				return packageoutput.HookManifest{}, fmt.Errorf("hook %q package file %q is missing from its rendered payload", descriptor.Identity, *argument.PackageFile)
			}
			descriptor.Handler.Arguments[index].PackageFile = &packagePath
		}
		descriptors = append(descriptors, descriptor)
	}
	model.SortHookDescriptors(descriptors)
	data, err := json.Marshal(hookConfigV1{Version: 1, Hooks: descriptors})
	if err != nil {
		return packageoutput.HookManifest{}, err
	}
	return packageoutput.HookManifest{Path: "hooks/hooks.v1.json", Bytes: append(data, '\n')}, nil
}

func piPackageFilePath(hook packageoutput.HookInput, path model.RelativePath) (model.RelativePath, bool) {
	for _, file := range hook.PayloadFiles() {
		if file.Path() == path {
			return file.PackagePath(), true
		}
	}
	return "", false
}

func validateHookSemantics(pkg model.NormalizedPackage) []model.Diagnostic {
	for _, asset := range pkg.Assets {
		if asset.Kind != model.AssetKindHook || asset.Hook == nil {
			continue
		}
		if asset.Hook.Event == model.HookEventNotification {
			return []model.Diagnostic{piHookDiagnostic(asset, "event notification has no lossless Pi extension mapping")}
		}
		if asset.Hook.FailurePolicy == model.HookFailurePolicyClosed && asset.Hook.Event != model.HookEventPreTool {
			return []model.Diagnostic{piHookDiagnostic(asset, fmt.Sprintf("hook.failure.closed is enforceable only for Pi pre-tool hooks, not %q", asset.Hook.Event))}
		}
		for _, use := range asset.CapabilityUses {
			if (use.Key == "hook.decision.block" || use.Key == "hook.decision.rewrite-input") && asset.Hook.Event != model.HookEventPreTool {
				return []model.Diagnostic{piHookDiagnostic(asset, fmt.Sprintf("capability %q is supported only for Pi pre-tool hooks", use.Key))}
			}
		}
	}
	return nil
}

func piHookDiagnostic(asset model.NormalizedAsset, message string) model.Diagnostic {
	location := model.CloneSourceLocation(asset.Hook.Location)
	return model.Diagnostic{
		Code:     "unsupported-hook-semantics",
		Severity: model.SeverityError,
		Location: &location,
		Message:  fmt.Sprintf("hook %q: %s", asset.Identity, message),
		Asset:    asset.Identity,
		Targets:  []model.TargetID{Target},
	}
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
