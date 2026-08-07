package copilot

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/hookdecision"
	"github.com/alexei-led/agentbundler/internal/target/marketplace"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

const (
	copilotHooksPath  = "hooks.json"
	copilotPluginRoot = "${PLUGIN_ROOT}"
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateNative},
	{Key: "asset.hook", State: model.CapabilityStateNative},
	{Key: "asset.command", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "hook.async", State: model.CapabilityStateNative},
	{Key: "hook.command.exec", State: model.CapabilityStateAdvisory},
	{Key: "hook.command.shell", State: model.CapabilityStateNative},
	{Key: "hook.decision.block", State: model.CapabilityStateNative},
	{Key: "hook.decision.rewrite-input", State: model.CapabilityStateNative},
	{Key: "hook.event.notification", State: model.CapabilityStateNative},
	{Key: "hook.event.post-compact", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.post-tool", State: model.CapabilityStateNative},
	{Key: "hook.event.post-tool-failure", State: model.CapabilityStateNative},
	{Key: "hook.event.pre-compact", State: model.CapabilityStateNative},
	{Key: "hook.event.pre-tool", State: model.CapabilityStateAdvisory},
	{Key: "hook.event.prompt-submit", State: model.CapabilityStateNative},
	{Key: "hook.event.session-end", State: model.CapabilityStateNative},
	{Key: "hook.event.session-start", State: model.CapabilityStateNative},
	{Key: "hook.event.stop", State: model.CapabilityStateNative},
	{Key: "hook.failure.closed", State: model.CapabilityStateAdvisory},
	{Key: "hook.matcher.tool-category", State: model.CapabilityStateNative},
}

// PackageCodec owns Copilot package manifest, agent, and hook serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:          Target,
		ManifestPath:    "plugin.json",
		AgentRoot:       "agents",
		HookPayloadRoot: "hooks",
		Capabilities:    append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:        manifest,
		Agent:           markdownAgent,
		Hooks:           hookManifest,
		Catalog:         catalogManifest,
		ValidatePackage: validatePackage,
	}
}

type catalogDocument struct {
	Name     string             `json:"name"`
	Owner    marketplace.Person `json:"owner"`
	Metadata catalogMetadata    `json:"metadata"`
	Plugins  []catalogEntry     `json:"plugins"`
}

type catalogMetadata struct {
	Description string `json:"description"`
	Version     string `json:"version"`
}

type catalogEntry struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Version     string             `json:"version"`
	Source      string             `json:"source"`
	Author      marketplace.Person `json:"author"`
	Homepage    string             `json:"homepage"`
	Repository  string             `json:"repository"`
	License     string             `json:"license"`
	Keywords    []string           `json:"keywords"`
}

func catalogManifest(catalog marketplace.Catalog) (packageoutput.CatalogManifest, error) {
	document := catalogDocument{
		Name: catalog.Name, Owner: catalog.Owner,
		Metadata: catalogMetadata{Description: catalog.Description, Version: catalog.Version},
		Plugins:  make([]catalogEntry, 0, len(catalog.Entries)),
	}
	for _, entry := range catalog.Entries {
		document.Plugins = append(document.Plugins, catalogEntry{
			Name: entry.Name, Description: entry.Description, Version: entry.Version,
			Source: copilotMarketplaceSource(entry.Source), Author: entry.Author, Homepage: entry.Homepage,
			Repository: entry.Repository, License: entry.License, Keywords: append([]string(nil), entry.Keywords...),
		})
	}
	data, err := json.Marshal(document)
	if err != nil {
		return packageoutput.CatalogManifest{}, err
	}
	return packageoutput.CatalogManifest{Path: ".github/plugin/marketplace.json", Bytes: append(data, '\n')}, nil
}

func copilotMarketplaceSource(root string) string {
	if root == "." {
		return "./"
	}
	return "./" + root
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
	return data, ".agent.md", err
}

func manifest(pkg model.NormalizedPackage) ([]byte, error) {
	values := packageoutput.ManifestBase(pkg)
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "author", "license", "homepage", "repository", "keywords")
	if value, ok := values["author"]; ok {
		values["author"] = packageoutput.PersonMetadata(value)
	}
	values["skills"] = []string{"skills/"}
	if packageoutput.PackageHasAsset(pkg, model.AssetKindAgent) {
		values["agents"] = "agents/"
	}
	if packageoutput.PackageHasAsset(pkg, model.AssetKindHook) {
		values["hooks"] = copilotHooksPath
	}
	return packageoutput.ManifestJSON(values)
}

type nativeHookManifest struct {
	Version int                            `json:"version"`
	Hooks   map[string][]nativeHookHandler `json:"hooks"`
}

type nativeHookHandler struct {
	Type       string `json:"type,omitempty"`
	Command    string `json:"command,omitempty"`
	Bash       string `json:"bash,omitempty"`
	Matcher    string `json:"matcher,omitempty"`
	TimeoutSec any    `json:"timeoutSec"`
}

func hookManifest(input packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
	manifest := nativeHookManifest{Version: 1, Hooks: make(map[string][]nativeHookHandler)}
	for _, hook := range input.Hooks() {
		descriptor := hook.Descriptor()
		event, ok := copilotEvent(descriptor.Event)
		if !ok {
			return packageoutput.HookManifest{}, fmt.Errorf("hook %q event %q is unsupported by Copilot CLI", descriptor.Identity, descriptor.Event)
		}
		matcher, err := copilotMatcher(descriptor)
		if err != nil {
			return packageoutput.HookManifest{}, err
		}
		command, bash, err := copilotCommands(descriptor, hook)
		if err != nil {
			return packageoutput.HookManifest{}, err
		}
		if descriptor.Event == model.HookEventPreTool && (descriptor.FailurePolicy == model.HookFailurePolicyClosed || hookdecision.UsesDecisionCapability(hook.CapabilityUses())) {
			original := bash
			if original == "" {
				original = command
			}
			command = ""
			bash = hookdecision.WrapPOSIX(original, hookdecision.ProtocolCopilot, string(descriptor.Identity))
		}
		manifest.Hooks[event] = append(manifest.Hooks[event], nativeHookHandler{
			Type: "command", Command: command, Bash: bash, Matcher: matcher,
			TimeoutSec: copilotTimeout(descriptor.TimeoutMilliseconds),
		})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return packageoutput.HookManifest{}, err
	}
	return packageoutput.HookManifest{Path: copilotHooksPath, Bytes: append(data, '\n')}, nil
}

func copilotCommands(descriptor model.HookDescriptor, hook packageoutput.HookInput) (command, bash string, err error) {
	switch descriptor.Handler.Mode {
	case model.HookHandlerModeShell:
		if descriptor.Handler.ShellCommand == nil {
			return "", "", fmt.Errorf("hook %q shell handler has no command", descriptor.Identity)
		}
		return *descriptor.Handler.ShellCommand, "", nil
	case model.HookHandlerModeExec:
		if descriptor.Handler.Program == nil {
			return "", "", fmt.Errorf("hook %q exec handler has no program", descriptor.Identity)
		}
		bashParts := []string{shellQuote(*descriptor.Handler.Program)}
		for _, argument := range descriptor.Handler.Arguments {
			switch {
			case argument.Literal != nil:
				bashParts = append(bashParts, shellQuote(*argument.Literal))
			case argument.PackageFile != nil:
				packagePath, ok := packageFilePath(hook, *argument.PackageFile)
				if !ok {
					return "", "", fmt.Errorf("hook %q package file %q is missing from its rendered payload", descriptor.Identity, *argument.PackageFile)
				}
				bashParts = append(bashParts, `"`+copilotPluginRoot+`"`+shellQuote("/"+string(packagePath)))
			default:
				return "", "", fmt.Errorf("hook %q has an invalid command argument", descriptor.Identity)
			}
		}
		return "", strings.Join(bashParts, " "), nil
	default:
		return "", "", fmt.Errorf("hook %q handler mode %q is unsupported by Copilot CLI", descriptor.Identity, descriptor.Handler.Mode)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func packageFilePath(hook packageoutput.HookInput, path model.RelativePath) (model.RelativePath, bool) {
	for _, file := range hook.PayloadFiles() {
		if file.Path() == path {
			return file.PackagePath(), true
		}
	}
	return "", false
}

func copilotTimeout(milliseconds int) any {
	if milliseconds%1_000 == 0 {
		return milliseconds / 1_000
	}
	return float64(milliseconds) / 1_000
}

func copilotEvent(event model.HookEvent) (string, bool) {
	value, ok := map[model.HookEvent]string{
		model.HookEventSessionStart:    "SessionStart",
		model.HookEventSessionEnd:      "SessionEnd",
		model.HookEventPromptSubmit:    "UserPromptSubmit",
		model.HookEventPreTool:         "PreToolUse",
		model.HookEventPostTool:        "PostToolUse",
		model.HookEventPostToolFailure: "PostToolUseFailure",
		model.HookEventStop:            "Stop",
		model.HookEventNotification:    "Notification",
		model.HookEventPreCompact:      "PreCompact",
	}[event]
	return value, ok
}

func copilotMatcher(descriptor model.HookDescriptor) (string, error) {
	if descriptor.Matcher == nil {
		return "", nil
	}
	var names []string
	for _, tool := range descriptor.Matcher.Tools {
		switch tool {
		case model.HookToolCategoryCommand:
			names = append(names, "Bash")
		case model.HookToolCategoryRead:
			names = append(names, "Read")
		case model.HookToolCategoryWrite:
			names = append(names, "Write")
		case model.HookToolCategoryEdit:
			names = append(names, "Edit")
		case model.HookToolCategorySearch:
			names = append(names, "Glob", "Grep")
		case model.HookToolCategoryWeb:
			names = append(names, "WebFetch", "WebSearch")
		case model.HookToolCategoryTask:
			names = append(names, "Agent", "Task")
		case model.HookToolCategoryMCP:
			names = append(names, "^mcp__.*$")
		default:
			return "", fmt.Errorf("hook %q tool category %q has no lossless Copilot CLI matcher", descriptor.Identity, tool)
		}
	}
	return strings.Join(names, "|"), nil
}

func validatePackage(pkg model.NormalizedPackage) []model.Diagnostic {
	for _, asset := range pkg.Assets {
		if asset.Kind != model.AssetKindHook || asset.Hook == nil {
			continue
		}
		descriptor := asset.Hook
		if _, ok := copilotEvent(descriptor.Event); !ok {
			return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("event %q is unsupported by Copilot CLI", descriptor.Event))}
		}
		if descriptor.FailurePolicy == model.HookFailurePolicyClosed && descriptor.Event != model.HookEventPreTool {
			return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("hook.failure.closed is equivalent only for Copilot pre-tool hooks, not %q", descriptor.Event))}
		}
		if descriptor.Event == model.HookEventNotification && !descriptor.Asynchronous {
			return []model.Diagnostic{hookDiagnostic(asset, "Copilot notification hooks are inherently asynchronous and cannot preserve synchronous portable execution")}
		}
		if descriptor.Asynchronous && descriptor.Event != model.HookEventNotification {
			return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("hook.async is unsupported for Copilot CLI event %q", descriptor.Event))}
		}
		if descriptor.Matcher != nil {
			if _, err := copilotMatcher(*descriptor); err != nil {
				return []model.Diagnostic{hookDiagnostic(asset, err.Error())}
			}
		}
	}
	return nil
}

func hookDiagnostic(asset model.NormalizedAsset, message string) model.Diagnostic {
	location := model.CloneSourceLocation(asset.Hook.Location)
	return model.Diagnostic{
		Code: "unsupported-hook-semantics", Severity: model.SeverityError,
		Location: &location, Message: fmt.Sprintf("hook %q: %s", asset.Identity, message),
		Asset: asset.Identity, Targets: []model.TargetID{Target},
	}
}
