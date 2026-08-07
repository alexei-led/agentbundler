package codex

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
	codexHooksPath  = "hooks/hooks.json"
	codexPluginRoot = "${PLUGIN_ROOT}"
)

var capabilityRules = []model.CapabilityRule{
	{Key: "asset.agent", State: model.CapabilityStateNative},
	{Key: "asset.hook", State: model.CapabilityStateNative},
	{Key: "asset.command", State: model.CapabilityStateUnsupported},
	{Key: "asset.resource", State: model.CapabilityStateNative},
	{Key: "asset.native-resource", State: model.CapabilityStateUnsupported},
	{Key: "asset.skill", State: model.CapabilityStateNative},
	{Key: "hook.async", State: model.CapabilityStateUnsupported},
	{Key: "hook.command.exec", State: model.CapabilityStateNative},
	{Key: "hook.command.shell", State: model.CapabilityStateNative},
	{Key: "hook.decision.block", State: model.CapabilityStateNative},
	{Key: "hook.decision.rewrite-input", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.notification", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.post-compact", State: model.CapabilityStateNative},
	{Key: "hook.event.post-tool", State: model.CapabilityStateNative},
	{Key: "hook.event.post-tool-failure", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.pre-compact", State: model.CapabilityStateNative},
	{Key: "hook.event.pre-tool", State: model.CapabilityStateNative},
	{Key: "hook.event.prompt-submit", State: model.CapabilityStateNative},
	{Key: "hook.event.session-end", State: model.CapabilityStateUnsupported},
	{Key: "hook.event.session-start", State: model.CapabilityStateNative},
	{Key: "hook.event.stop", State: model.CapabilityStateNative},
	{Key: "hook.failure.closed", State: model.CapabilityStateAdvisory},
	{Key: "hook.matcher.tool-category", State: model.CapabilityStateNative},
}

// PackageCodec owns Codex plugin manifest and hook serialization. Adapter.Render
// removes agents before package rendering and emits them as separate project profiles.
func PackageCodec() packageoutput.Codec {
	return packageoutput.Codec{
		Target:          Target,
		ManifestPath:    ".codex-plugin/plugin.json",
		Capabilities:    append([]model.CapabilityRule(nil), capabilityRules...),
		Manifest:        manifest,
		HookPayloadRoot: "assets/hooks",
		Hooks:           hookManifest,
		Catalog:         catalogManifest,
		ValidatePackage: validatePackage,
	}
}

type catalogDocument struct {
	Name      string           `json:"name"`
	Interface catalogInterface `json:"interface"`
	Plugins   []catalogEntry   `json:"plugins"`
}

type catalogInterface struct {
	DisplayName string `json:"displayName"`
}

type catalogEntry struct {
	Name     string        `json:"name"`
	Source   catalogSource `json:"source"`
	Policy   catalogPolicy `json:"policy"`
	Category string        `json:"category"`
}

type catalogSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type catalogPolicy struct {
	Installation   string `json:"installation"`
	Authentication string `json:"authentication"`
}

func catalogManifest(catalog marketplace.Catalog) (packageoutput.CatalogManifest, error) {
	document := catalogDocument{
		Name: catalog.Name, Interface: catalogInterface{DisplayName: catalog.Name},
		Plugins: make([]catalogEntry, 0, len(catalog.Entries)),
	}
	for _, entry := range catalog.Entries {
		document.Plugins = append(document.Plugins, catalogEntry{
			Name:     entry.Name,
			Source:   catalogSource{Source: "local", Path: codexMarketplaceSource(entry.Source)},
			Policy:   catalogPolicy{Installation: "AVAILABLE", Authentication: "ON_INSTALL"},
			Category: "Productivity",
		})
	}
	data, err := json.Marshal(document)
	if err != nil {
		return packageoutput.CatalogManifest{}, err
	}
	return packageoutput.CatalogManifest{Path: ".agents/plugins/marketplace.json", Bytes: append(data, '\n')}, nil
}

func codexMarketplaceSource(root string) string {
	if root == "." {
		return "./"
	}
	return "./" + root
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
	values["skills"] = "./skills/"
	return packageoutput.ManifestJSON(values)
}

type nativeHookManifest struct {
	Hooks map[string][]nativeHookGroup `json:"hooks"`
}

type nativeHookGroup struct {
	Matcher string              `json:"matcher,omitempty"`
	Hooks   []nativeHookHandler `json:"hooks"`
}

type nativeHookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	Async   bool   `json:"async,omitempty"`
}

func hookManifest(input packageoutput.HookRenderInput) (packageoutput.HookManifest, error) {
	manifest := nativeHookManifest{Hooks: make(map[string][]nativeHookGroup)}
	for _, hook := range input.Hooks() {
		descriptor := hook.Descriptor()
		event, ok := codexEvent(descriptor.Event)
		if !ok {
			return packageoutput.HookManifest{}, fmt.Errorf("hook %q event %q is unsupported by Codex", descriptor.Identity, descriptor.Event)
		}
		matcher, err := codexMatcher(descriptor)
		if err != nil {
			return packageoutput.HookManifest{}, err
		}
		command, err := codexCommand(descriptor, hook)
		if err != nil {
			return packageoutput.HookManifest{}, err
		}
		if descriptor.Event == model.HookEventPreTool && (descriptor.FailurePolicy == model.HookFailurePolicyClosed || hookdecision.UsesDecisionCapability(hook.CapabilityUses())) {
			command = hookdecision.WrapPOSIX(command, hookdecision.ProtocolCodex, string(descriptor.Identity))
		}
		manifest.Hooks[event] = append(manifest.Hooks[event], nativeHookGroup{
			Matcher: matcher,
			Hooks: []nativeHookHandler{{
				Type: "command", Command: command,
				Timeout: descriptor.TimeoutMilliseconds / 1_000,
				Async:   descriptor.Asynchronous,
			}},
		})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return packageoutput.HookManifest{}, err
	}
	return packageoutput.HookManifest{Path: codexHooksPath, Bytes: append(data, '\n')}, nil
}

func codexCommand(descriptor model.HookDescriptor, hook packageoutput.HookInput) (string, error) {
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
				packagePath, ok := packageFilePath(hook, *argument.PackageFile)
				if !ok {
					return "", fmt.Errorf("hook %q package file %q is missing from its rendered payload", descriptor.Identity, *argument.PackageFile)
				}
				parts = append(parts, `"`+codexPluginRoot+`"`+shellQuote("/"+string(packagePath)))
			default:
				return "", fmt.Errorf("hook %q has an invalid command argument", descriptor.Identity)
			}
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("hook %q handler mode %q is unsupported by Codex", descriptor.Identity, descriptor.Handler.Mode)
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

func codexEvent(event model.HookEvent) (string, bool) {
	value, ok := map[model.HookEvent]string{
		model.HookEventSessionStart: "SessionStart",
		model.HookEventPromptSubmit: "UserPromptSubmit",
		model.HookEventPreTool:      "PreToolUse",
		model.HookEventPostTool:     "PostToolUse",
		model.HookEventStop:         "Stop",
		model.HookEventPreCompact:   "PreCompact",
		model.HookEventPostCompact:  "PostCompact",
	}[event]
	return value, ok
}

func codexMatcher(descriptor model.HookDescriptor) (string, error) {
	if descriptor.Matcher == nil {
		return "", nil
	}
	var names []string
	for _, tool := range descriptor.Matcher.Tools {
		switch tool {
		case model.HookToolCategoryCommand:
			names = append(names, "^Bash$")
		case model.HookToolCategoryWrite, model.HookToolCategoryEdit:
			names = appendUnique(names, "^apply_patch$", "^Edit$", "^Write$")
		case model.HookToolCategoryMCP:
			names = append(names, "^mcp__.*$")
		default:
			return "", fmt.Errorf("hook %q tool category %q has no lossless Codex matcher; current Codex hooks intercept only Bash, apply_patch, and MCP calls", descriptor.Identity, tool)
		}
	}
	return strings.Join(names, "|"), nil
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func validatePackage(pkg model.NormalizedPackage) []model.Diagnostic {
	events := make(map[model.HookEvent][]model.NormalizedAsset)
	for _, asset := range pkg.Assets {
		if asset.Kind == model.AssetKindAgent {
			return []model.Diagnostic{assetDiagnostic(asset, "asset.agent must be separated from Codex plugin serialization and rendered as .codex/agents/*.toml")}
		}
		if asset.Kind != model.AssetKindHook || asset.Hook == nil {
			continue
		}
		descriptor := asset.Hook
		if _, ok := codexEvent(descriptor.Event); !ok {
			return []model.Diagnostic{assetDiagnostic(asset, fmt.Sprintf("hook event %q is unsupported by Codex", descriptor.Event))}
		}
		if descriptor.FailurePolicy == model.HookFailurePolicyClosed && descriptor.Event != model.HookEventPreTool {
			return []model.Diagnostic{assetDiagnostic(asset, fmt.Sprintf("hook.failure.closed is equivalent only for Codex pre-tool hooks, not %q", descriptor.Event))}
		}
		for _, previous := range events[descriptor.Event] {
			if codexHooksCanMatchTogether(*previous.Hook, *descriptor) {
				return []model.Diagnostic{assetDiagnostic(asset, fmt.Sprintf("Codex launches matching %q hooks concurrently and cannot preserve portable order between %q and %q", descriptor.Event, previous.Identity, asset.Identity))}
			}
		}
		events[descriptor.Event] = append(events[descriptor.Event], asset)
		if (descriptor.Event == model.HookEventPreTool || descriptor.Event == model.HookEventPostTool) && descriptor.Matcher == nil {
			return []model.Diagnostic{assetDiagnostic(asset, fmt.Sprintf("Codex %q hooks do not intercept every portable tool category and require a lossless tool matcher", descriptor.Event))}
		}
		if _, err := codexMatcher(*descriptor); err != nil {
			return []model.Diagnostic{assetDiagnostic(asset, err.Error())}
		}
		if descriptor.Asynchronous {
			return []model.Diagnostic{assetDiagnostic(asset, "hook.async is unsupported because Codex currently skips async command hooks")}
		}
		if descriptor.TimeoutMilliseconds%1_000 != 0 {
			return []model.Diagnostic{assetDiagnostic(asset, "Codex hook timeouts use whole seconds and cannot preserve the requested millisecond timeout")}
		}
	}
	return nil
}

func codexHooksCanMatchTogether(left, right model.HookDescriptor) bool {
	if left.Matcher == nil || right.Matcher == nil {
		return true
	}
	leftTools := make(map[model.HookToolCategory]struct{}, len(left.Matcher.Tools))
	for _, tool := range left.Matcher.Tools {
		leftTools[tool] = struct{}{}
	}
	for _, tool := range right.Matcher.Tools {
		if _, exists := leftTools[tool]; exists {
			return true
		}
	}
	return false
}

func assetDiagnostic(asset model.NormalizedAsset, message string) model.Diagnostic {
	diagnostic := model.Diagnostic{Code: "unsupported-capability", Severity: model.SeverityError, Asset: asset.Identity, Message: message}
	if asset.Hook != nil {
		location := asset.Hook.Location
		diagnostic.Location = &location
	}
	return diagnostic
}
