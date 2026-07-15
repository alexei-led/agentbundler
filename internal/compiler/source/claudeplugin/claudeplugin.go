package claudeplugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const diagnosticCode = "invalid-claude-plugin"

// InspectClaudePlugin discovers portable assets and Claude-native gaps in one plugin root.
func InspectClaudePlugin(manifest model.SourceManifest, workspaceRoot string) (model.SourceInventory, []model.Diagnostic) {
	workspace, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return model.SourceInventory{}, []model.Diagnostic{{Code: diagnosticCode, Severity: model.SeverityError, Message: "workspace root: " + err.Error()}}
	}
	defer func() { _ = workspace.Close() }()
	return InspectClaudePluginRoot(manifest, workspaceRoot, workspace)
}

// InspectClaudePluginRoot imports a Claude plugin using a workspace-bounded filesystem.
func InspectClaudePluginRoot(manifest model.SourceManifest, workspaceRoot string, workspace *os.Root) (model.SourceInventory, []model.Diagnostic) {
	inspector := claudeInspector{workspaceRoot: workspaceRoot, filesystem: workspace, inputs: make(map[model.RelativePath]string)}
	inspector.diagnostics = append(inspector.diagnostics, model.ValidateSourceManifest(manifest)...)
	if manifest.Kind != model.SourceKindClaudePlugin {
		inspector.error("", "manifest kind must be claude-plugin")
	}
	if hasErrors(inspector.diagnostics) {
		return model.SourceInventory{}, inspector.diagnostics
	}
	if !filepath.IsAbs(workspaceRoot) || filepath.Clean(workspaceRoot) != workspaceRoot {
		inspector.error("", "workspace root must be a cleaned absolute path")
		return model.SourceInventory{}, inspector.diagnostics
	}
	var err error
	inspector.sourceRoot, err = containedPath(workspaceRoot, string(manifest.Root))
	if err != nil {
		inspector.error(string(manifest.Root), "manifest root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	pluginRoot, err := containedPath(inspector.sourceRoot, string(manifest.ClaudePlugin.PluginRoot))
	if err != nil {
		inspector.error(string(manifest.ClaudePlugin.PluginRoot), "plugin root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	if err := inspector.noSymlinkComponents(workspaceRoot, pluginRoot); err != nil {
		inspector.error(inspector.relativePath(pluginRoot), "plugin root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	if err := inspector.requireDirectory(pluginRoot); err != nil {
		inspector.error(inspector.relativePath(pluginRoot), "plugin root: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	inspector.sourceRoot = pluginRoot

	pluginPath := filepath.Join(pluginRoot, ".claude-plugin", "plugin.json")
	data, ok := inspector.optionalRegularInput(pluginPath)
	if !ok {
		if !hasErrors(inspector.diagnostics) {
			inspector.error(inspector.relativePath(pluginPath), "plugin manifest is required")
		}
		return model.SourceInventory{}, inspector.diagnostics
	}
	plugin, err := parsePluginManifest(data)
	if err != nil {
		inspector.error(inspector.relativePath(pluginPath), "plugin manifest: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}
	packageID, err := model.NewPackageID(plugin.Name)
	if err != nil {
		inspector.error(inspector.relativePath(pluginPath), "plugin name: "+err.Error())
		return model.SourceInventory{}, inspector.diagnostics
	}

	metadata := model.PackageMetadata{}
	if plugin.Description != nil {
		metadata["description"] = *plugin.Description
	}
	if plugin.Version != nil {
		metadata["version"] = *plugin.Version
	}
	inspector.marketplaceMetadata(filepath.Join(pluginRoot, ".claude-plugin", "marketplace.json"), pluginRoot, metadata)

	assets := make(map[model.AssetID]model.SourceAsset)
	recognized := map[string]struct{}{pluginPath: {}}
	marketplacePath := filepath.Join(pluginRoot, ".claude-plugin", "marketplace.json")
	if _, exists := inspector.inputs[model.RelativePath(inspector.relativePath(marketplacePath))]; exists {
		recognized[marketplacePath] = struct{}{}
	}
	inspector.skills(pluginRoot, assets, recognized)
	inspector.agents(pluginRoot, assets, recognized)
	inspector.hooks(plugin.Hooks, assets, pluginPath)
	inspector.nativeGaps(pluginRoot, recognized)
	if hasErrors(inspector.diagnostics) {
		return model.SourceInventory{}, inspector.diagnostics
	}

	assetList := make([]model.SourceAsset, 0, len(assets))
	for _, asset := range assets {
		assetList = append(assetList, asset)
	}
	sort.Slice(assetList, func(a, b int) bool { return assetList[a].Identity < assetList[b].Identity })
	inputs := make([]model.InputFile, 0, len(inspector.inputs))
	for path, digest := range inspector.inputs {
		inputs = append(inputs, model.InputFile{Path: path, SHA256: digest})
	}
	sort.Slice(inputs, func(a, b int) bool { return inputs[a].Path < inputs[b].Path })
	inventory := model.SourceInventory{Packages: []model.SourcePackage{{Identity: packageID, Metadata: metadata, Assets: assetList}}, NativeGaps: inspector.native, Inputs: inputs}
	if diagnostics := model.ValidateSourceInventory(inventory); len(diagnostics) != 0 {
		return model.SourceInventory{}, append(inspector.diagnostics, diagnostics...)
	}
	return inventory, inspector.diagnostics
}

type pluginManifest struct {
	Name        string
	Description *string
	Version     *string
	Hooks       map[string][]hookSpec
}
type hookSpec struct {
	Matcher *string
	Command string
	Timeout *int
}

func parsePluginManifest(data []byte) (pluginManifest, error) {
	var raw struct {
		Name        *string                    `json:"name"`
		Description *string                    `json:"description"`
		Version     *string                    `json:"version"`
		Hooks       map[string]json.RawMessage `json:"hooks"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return pluginManifest{}, err
	}
	if raw.Name == nil || *raw.Name == "" {
		return pluginManifest{}, fmt.Errorf("name is required")
	}
	result := pluginManifest{Name: *raw.Name, Description: raw.Description, Version: raw.Version, Hooks: make(map[string][]hookSpec)}
	for event, hooks := range raw.Hooks {
		if event == "" {
			return pluginManifest{}, fmt.Errorf("hook event must not be empty")
		}
		var entries []json.RawMessage
		if err := decodeStrictJSON(hooks, &entries); err != nil {
			return pluginManifest{}, fmt.Errorf("hooks.%s: %w", event, err)
		}
		for index, entry := range entries {
			var hook struct {
				Matcher *string `json:"matcher"`
				Command *string `json:"command"`
				Timeout *int    `json:"timeout"`
			}
			if err := decodeStrictJSONObject(entry, &hook); err != nil {
				return pluginManifest{}, fmt.Errorf("hooks.%s[%d]: %w", event, index, err)
			}
			if hook.Command == nil || *hook.Command == "" {
				return pluginManifest{}, fmt.Errorf("hooks.%s[%d].command is required", event, index)
			}
			result.Hooks[event] = append(result.Hooks[event], hookSpec{Matcher: hook.Matcher, Command: *hook.Command, Timeout: hook.Timeout})
		}
	}
	return result, nil
}

func resolveMarketplaceSource(root, source string) (string, error) {
	if filepath.IsAbs(source) {
		return "", fmt.Errorf("source must be relative")
	}
	return filepath.Clean(filepath.Join(root, source)), nil
}

func (i *claudeInspector) marketplaceMetadata(path, pluginRoot string, metadata model.PackageMetadata) {
	data, exists := i.optionalRegularInput(path)
	if !exists {
		return
	}
	var market map[string]json.RawMessage
	if err := decodeStrictJSONObject(data, &market); err != nil {
		i.error(i.relativePath(path), "marketplace: "+err.Error())
		return
	}
	plugins, ok := market["plugins"]
	if !ok {
		i.error(i.relativePath(path), "marketplace plugins is required")
		return
	}
	var entries []json.RawMessage
	if err := decodeStrictJSON(plugins, &entries); err != nil {
		i.error(i.relativePath(path), "marketplace plugins: "+err.Error())
		return
	}
	if len(entries) != 1 {
		i.error(i.relativePath(path), "marketplace plugins must contain exactly one entry")
		return
	}
	var entry map[string]json.RawMessage
	if err := decodeStrictJSONObject(entries[0], &entry); err != nil {
		i.error(i.relativePath(path), "marketplace plugin: "+err.Error())
		return
	}
	source, ok := entry["source"]
	if !ok {
		i.error(i.relativePath(path), "marketplace plugin source is required")
		return
	}
	var sourcePath string
	if err := decodeStrictJSON(source, &sourcePath); err != nil {
		i.error(i.relativePath(path), "marketplace plugin source: "+err.Error())
		return
	}
	resolved, err := resolveMarketplaceSource(filepath.Dir(path), sourcePath)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(pluginRoot) {
		i.error(i.relativePath(path), "marketplace plugin source must resolve to the plugin root")
		return
	}
	for key, value := range market {
		if key == "plugins" {
			continue
		}
		var decoded any
		if err := decodeStrictJSON(value, &decoded); err != nil {
			i.error(i.relativePath(path), "marketplace metadata: "+err.Error())
			continue
		}
		metadata[key] = decoded
	}
	for key, value := range entry {
		if key == "source" {
			continue
		}
		var decoded any
		if err := decodeStrictJSON(value, &decoded); err != nil {
			i.error(i.relativePath(path), "marketplace plugin metadata: "+err.Error())
			continue
		}
		metadata[key] = decoded
	}
}

func (i *claudeInspector) skills(root string, assets map[model.AssetID]model.SourceAsset, recognized map[string]struct{}) {
	skillsRoot := filepath.Join(root, "skills")
	if _, err := i.lstat(skillsRoot); err != nil {
		if !os.IsNotExist(err) {
			i.error(i.relativePath(skillsRoot), "inspect skills: "+err.Error())
		}
		return
	}
	for _, skillFile := range i.findSkillFiles(skillsRoot) {
		asset, ok := i.inspectSkill(skillFile)
		if !ok {
			continue
		}
		if _, exists := assets[asset.Identity]; exists {
			i.error(i.relativePath(skillFile), fmt.Sprintf("duplicate asset %q", asset.Identity))
			continue
		}
		assets[asset.Identity] = asset
		dir := filepath.Dir(skillFile)
		_ = i.walkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				recognized[path] = struct{}{}
			}
			return nil
		})
	}
}

func (i *claudeInspector) agents(root string, assets map[model.AssetID]model.SourceAsset, recognized map[string]struct{}) {
	entries, err := i.readDir(filepath.Join(root, "agents"))
	if err != nil {
		if !os.IsNotExist(err) {
			i.error(i.relativePath(filepath.Join(root, "agents")), "read agents: "+err.Error())
		}
		return
	}
	for _, entry := range entries {
		path := filepath.Join(root, "agents", entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			i.error(i.relativePath(path), "source symlinks are not allowed")
			continue
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		identity, err := model.NewAssetID("agent/" + name)
		if err != nil {
			i.error(i.relativePath(path), "agent identity: "+err.Error())
			continue
		}
		data, ok := i.readInput(path)
		if !ok {
			continue
		}
		frontmatter, body, err := parseFrontmatter(data)
		if err != nil {
			i.error(i.relativePath(path), "agent frontmatter: "+err.Error())
			continue
		}
		caps, overlays := i.sidecars(identity, model.AssetKindAgent, name)
		assets[identity] = model.SourceAsset{Identity: identity, Kind: model.AssetKindAgent, Base: model.AssetContent{Frontmatter: frontmatter, Body: body, Files: map[model.RelativePath]model.FileContent{}}, CapabilityUses: caps, Overlays: overlays}
		recognized[path] = struct{}{}
	}
}

func (i *claudeInspector) hooks(hooks map[string][]hookSpec, assets map[model.AssetID]model.SourceAsset, pluginPath string) {
	events := make([]string, 0, len(hooks))
	for event := range hooks {
		events = append(events, event)
	}
	sort.Strings(events)
	for _, nativeEvent := range events {
		event, ok := portableClaudeEvent(nativeEvent)
		if !ok {
			i.error(i.relativePath(pluginPath), fmt.Sprintf("hook event %q is not a portable event", nativeEvent))
			continue
		}
		for ordinal, hook := range hooks[nativeEvent] {
			name := fmt.Sprintf("%s-%d", nativeEvent, ordinal+1)
			identity, err := model.NewAssetID("hook/" + name)
			if err != nil {
				i.error(i.relativePath(pluginPath), "hook identity: "+err.Error())
				continue
			}
			matcher, err := portableClaudeMatcher(hook.Matcher)
			if err != nil {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("hook %q matcher: %v", identity, err))
				continue
			}
			timeout, err := portableClaudeTimeout(event, hook.Timeout)
			if err != nil {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("hook %q timeout: %v", identity, err))
				continue
			}
			frontmatter := map[string]any{"event": nativeEvent}
			if hook.Matcher != nil {
				frontmatter["matcher"] = *hook.Matcher
			}
			if hook.Timeout != nil {
				frontmatter["timeout"] = *hook.Timeout
			}
			command := hook.Command
			descriptor := model.HookDescriptor{
				Identity:            identity,
				Location:            model.SourceLocation{Path: model.RelativePath(i.relativePath(pluginPath))},
				Event:               event,
				Matcher:             matcher,
				Handler:             model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command},
				TimeoutMilliseconds: timeout,
				FailurePolicy:       model.HookFailurePolicyOpen,
				Order:               ordinal,
			}
			caps, overlays := i.sidecars(identity, model.AssetKindHook, name)
			assets[identity] = model.SourceAsset{
				Identity: identity,
				Kind:     model.AssetKindHook,
				Base: model.AssetContent{
					Frontmatter: frontmatter,
					Body:        hook.Command,
					Files:       map[model.RelativePath]model.FileContent{},
				},
				Hook:           &descriptor,
				CapabilityUses: caps,
				Overlays:       overlays,
			}
		}
	}
}

func portableClaudeEvent(event string) (model.HookEvent, bool) {
	events := map[string]model.HookEvent{
		"SessionStart":       model.HookEventSessionStart,
		"SessionEnd":         model.HookEventSessionEnd,
		"UserPromptSubmit":   model.HookEventPromptSubmit,
		"PreToolUse":         model.HookEventPreTool,
		"PostToolUse":        model.HookEventPostTool,
		"PostToolUseFailure": model.HookEventPostToolFailure,
		"Stop":               model.HookEventStop,
		"Notification":       model.HookEventNotification,
		"PreCompact":         model.HookEventPreCompact,
		"PostCompact":        model.HookEventPostCompact,
	}
	portable, ok := events[event]
	return portable, ok
}

func portableClaudeMatcher(matcher *string) (*model.HookMatcher, error) {
	if matcher == nil {
		return nil, nil
	}
	categories := map[string]model.HookToolCategory{
		"Bash":         model.HookToolCategoryCommand,
		"Read":         model.HookToolCategoryRead,
		"Write":        model.HookToolCategoryWrite,
		"Edit":         model.HookToolCategoryEdit,
		"NotebookEdit": model.HookToolCategoryEdit,
		"Glob":         model.HookToolCategorySearch,
		"Grep":         model.HookToolCategorySearch,
		"WebFetch":     model.HookToolCategoryWeb,
		"WebSearch":    model.HookToolCategoryWeb,
		"Task":         model.HookToolCategoryTask,
	}
	category, ok := categories[*matcher]
	if !ok {
		return nil, fmt.Errorf("%q is not an exact known native tool name", *matcher)
	}
	return &model.HookMatcher{Tools: []model.HookToolCategory{category}}, nil
}

func portableClaudeTimeout(event model.HookEvent, seconds *int) (int, error) {
	if seconds == nil {
		if event == model.HookEventPromptSubmit {
			return 30_000, nil
		}
		return model.MaxHookTimeoutMilliseconds, nil
	}
	if *seconds < 1 || *seconds > model.MaxHookTimeoutMilliseconds/1_000 {
		return 0, fmt.Errorf("seconds must be between 1 and %d", model.MaxHookTimeoutMilliseconds/1_000)
	}
	return *seconds * 1_000, nil
}

func (i *claudeInspector) nativeGaps(root string, recognized map[string]struct{}) {
	claude := model.TargetClaude
	_ = i.walkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			i.error(i.relativePath(path), "walk source: "+err.Error())
			return nil
		}
		if path != root && entry.Type()&os.ModeSymlink != 0 {
			i.error(i.relativePath(path), "source symlinks are not allowed")
			return filepath.SkipDir
		}
		if entry.IsDir() {
			if entry.Name() == ".agentbundler" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			i.error(i.relativePath(path), "source entries must be regular files or directories")
			return nil
		}
		if _, ok := recognized[path]; ok {
			return nil
		}
		i.readInput(path)
		i.native = append(i.native, model.NativeGap{Component: i.relativePath(path), Location: model.SourceLocation{Path: model.RelativePath(i.relativePath(path))}, Target: &claude})
		return nil
	})
	sort.Slice(i.native, func(a, b int) bool { return i.native[a].Component < i.native[b].Component })
}

func containedPath(root, relative string) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	path, err := filepath.Rel(root, candidate)
	if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes its declared root")
	}
	return candidate, nil
}
