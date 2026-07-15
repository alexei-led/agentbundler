package cursor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

const cursorHooksPath = "hooks/hooks.json"

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateNative},
	{Key: "asset.hook", State: model.CapabilityStateNative},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "hook.async", State: model.CapabilityStateUnsupported},
	{Key: "hook.command.exec", State: model.CapabilityStateAdvisory},
	{Key: "hook.command.shell", State: model.CapabilityStateNative},
	{Key: "hook.decision.block", State: model.CapabilityStateNative},
	{Key: "hook.decision.rewrite-input", State: model.CapabilityStateNative},
	{Key: "hook.event.notification", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.post-compact", State: model.CapabilityStateUnsupported},
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

// PackageCodec owns Cursor package manifest, agent, and hook serialization.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:          model.TargetCursor,
		ManifestPath:    ".cursor-plugin/plugin.json",
		AgentRoot:       "agents",
		HookPayloadRoot: "hooks",
		Capabilities:    append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:        manifest,
		Agent:           markdownAgent,
		Hooks:           hookManifest,
		ValidatePackage: validatePackage,
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
	packageoutput.CopyMetadata(values, pkg.Metadata, "version", "description", "displayName", "author", "homepage", "repository", "license", "keywords")
	if value, ok := values["author"]; ok {
		values["author"] = packageoutput.PersonMetadata(value)
	}
	values["skills"] = "./skills/"
	if packageoutput.PackageHasAsset(pkg, model.AssetKindAgent) {
		values["agents"] = "./agents/"
	}
	if packageoutput.PackageHasAsset(pkg, model.AssetKindHook) {
		values["hooks"] = "./" + cursorHooksPath
	}
	return packageoutput.ManifestJSON(values)
}

type nativeHookManifest struct {
	Version int                            `json:"version"`
	Hooks   map[string][]nativeHookHandler `json:"hooks"`
}

type nativeHookHandler struct {
	Command    string `json:"command"`
	Matcher    string `json:"matcher,omitempty"`
	Timeout    any    `json:"timeout"`
	FailClosed bool   `json:"failClosed,omitempty"`
}

func hookManifest(input packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
	manifest := nativeHookManifest{Version: 1, Hooks: make(map[string][]nativeHookHandler)}
	for _, hook := range input.Hooks() {
		descriptor := hook.Descriptor()
		event, ok := cursorEvent(descriptor.Event)
		if !ok {
			return packageoutput.HookManifest{}, fmt.Errorf("hook %q event %q is unsupported by Cursor", descriptor.Identity, descriptor.Event)
		}
		matcher, err := cursorMatcher(descriptor)
		if err != nil {
			return packageoutput.HookManifest{}, err
		}
		command, err := cursorCommand(descriptor, hook)
		if err != nil {
			return packageoutput.HookManifest{}, err
		}
		manifest.Hooks[event] = append(manifest.Hooks[event], nativeHookHandler{
			Command: command, Matcher: matcher, Timeout: cursorTimeout(descriptor.TimeoutMilliseconds),
			FailClosed: descriptor.FailurePolicy == model.HookFailurePolicyClosed,
		})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return packageoutput.HookManifest{}, err
	}
	return packageoutput.HookManifest{Path: cursorHooksPath, Bytes: append(data, '\n')}, nil
}

func cursorCommand(descriptor model.HookDescriptor, hook packageoutput.HookInput) (string, error) {
	switch descriptor.Handler.Mode {
	case model.HookHandlerModeShell:
		if descriptor.Handler.ShellCommand == nil {
			return "", fmt.Errorf("hook %q shell handler has no command", descriptor.Identity)
		}
		return *descriptor.Handler.ShellCommand, nil
	case model.HookHandlerModeExec:
		if descriptor.Handler.Program == nil {
			return "", fmt.Errorf("hook %q exec handler has no program", descriptor.Identity)
		}
		parts := []string{shellQuote(*descriptor.Handler.Program)}
		for _, argument := range descriptor.Handler.Arguments {
			switch {
			case argument.Literal != nil:
				parts = append(parts, shellQuote(*argument.Literal))
			case argument.PackageFile != nil:
				path, ok := packageFilePath(hook, *argument.PackageFile)
				if !ok {
					return "", fmt.Errorf("hook %q package file %q is missing from its rendered payload", descriptor.Identity, *argument.PackageFile)
				}
				parts = append(parts, shellQuote("./"+string(path)))
			default:
				return "", fmt.Errorf("hook %q has an invalid command argument", descriptor.Identity)
			}
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("hook %q handler mode %q is unsupported by Cursor", descriptor.Identity, descriptor.Handler.Mode)
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

func cursorTimeout(milliseconds int) any {
	if milliseconds%1_000 == 0 {
		return milliseconds / 1_000
	}
	return float64(milliseconds) / 1_000
}

func cursorEvent(event model.HookEvent) (string, bool) {
	value, ok := map[model.HookEvent]string{
		model.HookEventSessionStart:    "sessionStart",
		model.HookEventSessionEnd:      "sessionEnd",
		model.HookEventPromptSubmit:    "beforeSubmitPrompt",
		model.HookEventPreTool:         "preToolUse",
		model.HookEventPostTool:        "postToolUse",
		model.HookEventPostToolFailure: "postToolUseFailure",
		model.HookEventStop:            "stop",
		model.HookEventPreCompact:      "preCompact",
	}[event]
	return value, ok
}

func cursorMatcher(descriptor model.HookDescriptor) (string, error) {
	if descriptor.Matcher == nil {
		return "", nil
	}
	var names []string
	for _, tool := range descriptor.Matcher.Tools {
		switch tool {
		case model.HookToolCategoryCommand:
			names = append(names, "Shell")
		case model.HookToolCategoryRead:
			names = append(names, "Read")
		case model.HookToolCategoryWrite:
			names = append(names, "Write")
		case model.HookToolCategoryTask:
			names = append(names, "Task")
		case model.HookToolCategoryMCP:
			names = append(names, "^MCP:.*$")
		default:
			return "", fmt.Errorf("hook %q tool category %q has no lossless Cursor matcher", descriptor.Identity, tool)
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
		if _, ok := cursorEvent(descriptor.Event); !ok {
			return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("event %q has no equivalent Cursor hook", descriptor.Event))}
		}
		if descriptor.Asynchronous {
			return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("hook.async is unsupported for Cursor event %q", descriptor.Event))}
		}
		if descriptor.FailurePolicy == model.HookFailurePolicyClosed && descriptor.Event != model.HookEventPreTool && descriptor.Event != model.HookEventPromptSubmit {
			return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("hook.failure.closed is equivalent only for Cursor pre-tool and prompt-submit hooks, not %q", descriptor.Event))}
		}
		if descriptor.Matcher != nil {
			if _, err := cursorMatcher(*descriptor); err != nil {
				return []model.Diagnostic{hookDiagnostic(asset, err.Error())}
			}
		}
		for _, use := range asset.CapabilityUses {
			switch use.Key {
			case "hook.decision.block":
				if descriptor.Event != model.HookEventPreTool && descriptor.Event != model.HookEventPromptSubmit {
					return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("capability %q is supported only for Cursor pre-tool and prompt-submit hooks", use.Key))}
				}
			case "hook.decision.rewrite-input":
				if descriptor.Event != model.HookEventPreTool {
					return []model.Diagnostic{hookDiagnostic(asset, fmt.Sprintf("capability %q is supported only for Cursor pre-tool hooks", use.Key))}
				}
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
		Asset: asset.Identity, Targets: []model.TargetID{model.TargetCursor},
	}
}
