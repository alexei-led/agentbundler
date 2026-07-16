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
	inspector.hooks(pluginRoot, pluginPath, plugin.Hooks, assets, recognized)
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
	Hooks       json.RawMessage
}

type hookSpec struct {
	Event        string
	Matcher      *string
	Command      string
	Arguments    []string
	Exec         bool
	Timeout      *int
	Asynchronous bool
	Location     model.SourceLocation
	Component    string
	NativeOnly   bool
}

func parsePluginManifest(data []byte) (pluginManifest, error) {
	var raw struct {
		Schema       json.RawMessage `json:"$schema"`
		Name         *string         `json:"name"`
		Version      *string         `json:"version"`
		Description  *string         `json:"description"`
		Author       json.RawMessage `json:"author"`
		Homepage     json.RawMessage `json:"homepage"`
		Repository   json.RawMessage `json:"repository"`
		License      json.RawMessage `json:"license"`
		Keywords     json.RawMessage `json:"keywords"`
		Dependencies json.RawMessage `json:"dependencies"`
		Hooks        json.RawMessage `json:"hooks"`
		Commands     json.RawMessage `json:"commands"`
		Agents       json.RawMessage `json:"agents"`
		Skills       json.RawMessage `json:"skills"`
		OutputStyles json.RawMessage `json:"outputStyles"`
		Themes       json.RawMessage `json:"themes"`
		Channels     json.RawMessage `json:"channels"`
		MCPServers   json.RawMessage `json:"mcpServers"`
		LSPServers   json.RawMessage `json:"lspServers"`
		Monitors     json.RawMessage `json:"monitors"`
		Settings     json.RawMessage `json:"settings"`
		UserConfig   json.RawMessage `json:"userConfig"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return pluginManifest{}, err
	}
	if raw.Name == nil || *raw.Name == "" {
		return pluginManifest{}, fmt.Errorf("name is required")
	}
	return pluginManifest{Name: *raw.Name, Description: raw.Description, Version: raw.Version, Hooks: raw.Hooks}, nil
}

func parseHookFile(data []byte) ([]hookSpec, error) {
	var raw struct {
		Description *string                    `json:"description"`
		Hooks       map[string]json.RawMessage `json:"hooks"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return nil, err
	}
	if raw.Hooks == nil {
		return nil, fmt.Errorf("hooks is required")
	}
	return parseHookEvents(raw.Hooks)
}

func parseInlineHooks(data []byte) ([]hookSpec, error) {
	var events map[string]json.RawMessage
	if err := decodeStrictJSONObject(data, &events); err != nil {
		return nil, err
	}
	if _, wrapped := events["hooks"]; wrapped {
		return parseHookFile(data)
	}
	return parseHookEvents(events)
}

func parseHookEvents(events map[string]json.RawMessage) ([]hookSpec, error) {
	names := make([]string, 0, len(events))
	for event := range events {
		if event == "" {
			return nil, fmt.Errorf("hook event must not be empty")
		}
		names = append(names, event)
	}
	sort.Strings(names)

	var result []hookSpec
	for _, event := range names {
		var entries []json.RawMessage
		if err := decodeStrictJSON(events[event], &entries); err != nil {
			return nil, fmt.Errorf("hooks.%s: %w", event, err)
		}
		for groupIndex, entry := range entries {
			var fields map[string]json.RawMessage
			if err := decodeStrictJSONObject(entry, &fields); err != nil {
				return nil, fmt.Errorf("hooks.%s[%d]: %w", event, groupIndex, err)
			}
			if _, official := fields["hooks"]; !official {
				hook, err := parseLegacyHook(entry)
				if err != nil {
					return nil, fmt.Errorf("hooks.%s[%d]: %w", event, groupIndex, err)
				}
				hook.Event = event
				hook.Component = fmt.Sprintf("hooks.%s[%d]", event, groupIndex)
				result = append(result, hook)
				continue
			}

			var group struct {
				Matcher *string           `json:"matcher"`
				Hooks   []json.RawMessage `json:"hooks"`
			}
			if err := decodeStrictJSONObject(entry, &group); err != nil {
				return nil, fmt.Errorf("hooks.%s[%d]: %w", event, groupIndex, err)
			}
			if len(group.Hooks) == 0 {
				return nil, fmt.Errorf("hooks.%s[%d].hooks must not be empty", event, groupIndex)
			}
			for handlerIndex, handler := range group.Hooks {
				hook, err := parseCommandHook(handler)
				if err != nil {
					return nil, fmt.Errorf("hooks.%s[%d].hooks[%d]: %w", event, groupIndex, handlerIndex, err)
				}
				hook.Event = event
				hook.Matcher = group.Matcher
				hook.Component = fmt.Sprintf("hooks.%s[%d].hooks[%d]", event, groupIndex, handlerIndex)
				result = append(result, hook)
			}
		}
	}
	return result, nil
}

func parseLegacyHook(data []byte) (hookSpec, error) {
	var raw struct {
		Matcher      *string         `json:"matcher"`
		Type         *string         `json:"type"`
		Command      *string         `json:"command"`
		Arguments    json.RawMessage `json:"args"`
		Timeout      *int            `json:"timeout"`
		Asynchronous *bool           `json:"async"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return hookSpec{}, err
	}
	hook, err := commandHook(raw.Command, raw.Arguments, raw.Timeout, raw.Asynchronous)
	hook.Matcher = raw.Matcher
	return hook, err
}

func parseCommandHook(data []byte) (hookSpec, error) {
	var fields map[string]json.RawMessage
	if err := decodeStrictJSONObject(data, &fields); err != nil {
		return hookSpec{}, err
	}
	if encodedType, exists := fields["type"]; exists {
		var hookType string
		if err := decodeStrictJSON(encodedType, &hookType); err != nil {
			return hookSpec{}, fmt.Errorf("type: %w", err)
		}
		if hookType != "command" {
			if err := validateUnsupportedHook(data, hookType); err != nil {
				return hookSpec{}, err
			}
			return hookSpec{NativeOnly: true}, nil
		}
	}

	var raw struct {
		Type         *string         `json:"type"`
		If           *string         `json:"if"`
		Timeout      *int            `json:"timeout"`
		Status       *string         `json:"statusMessage"`
		Once         *bool           `json:"once"`
		Command      *string         `json:"command"`
		Arguments    json.RawMessage `json:"args"`
		Asynchronous *bool           `json:"async"`
		AsyncRewake  *bool           `json:"asyncRewake"`
		Shell        *string         `json:"shell"`
	}
	if err := decodeStrictJSONObject(data, &raw); err != nil {
		return hookSpec{}, err
	}
	if raw.Type == nil {
		return hookSpec{}, fmt.Errorf("type is required")
	}
	if raw.If != nil || raw.Status != nil || (raw.Once != nil && *raw.Once) || (raw.AsyncRewake != nil && *raw.AsyncRewake) || raw.Shell != nil {
		return hookSpec{NativeOnly: true}, nil
	}
	return commandHook(raw.Command, raw.Arguments, raw.Timeout, raw.Asynchronous)
}

func validateUnsupportedHook(data []byte, hookType string) error {
	switch hookType {
	case "http":
		var raw struct {
			Type           string            `json:"type"`
			If             *string           `json:"if"`
			Timeout        *int              `json:"timeout"`
			Status         *string           `json:"statusMessage"`
			Once           *bool             `json:"once"`
			URL            *string           `json:"url"`
			Headers        map[string]string `json:"headers"`
			AllowedEnvVars []string          `json:"allowedEnvVars"`
		}
		if err := decodeStrictJSONObject(data, &raw); err != nil {
			return err
		}
		if raw.URL == nil || *raw.URL == "" {
			return fmt.Errorf("url is required")
		}
		return nil
	case "mcp_tool":
		var raw struct {
			Type    string         `json:"type"`
			If      *string        `json:"if"`
			Timeout *int           `json:"timeout"`
			Status  *string        `json:"statusMessage"`
			Once    *bool          `json:"once"`
			Server  *string        `json:"server"`
			Tool    *string        `json:"tool"`
			Input   map[string]any `json:"input"`
		}
		if err := decodeStrictJSONObject(data, &raw); err != nil {
			return err
		}
		if raw.Server == nil || *raw.Server == "" || raw.Tool == nil || *raw.Tool == "" {
			return fmt.Errorf("server and tool are required")
		}
		return nil
	case "prompt", "agent":
		var raw struct {
			Type    string  `json:"type"`
			If      *string `json:"if"`
			Timeout *int    `json:"timeout"`
			Status  *string `json:"statusMessage"`
			Once    *bool   `json:"once"`
			Prompt  *string `json:"prompt"`
			Model   *string `json:"model"`
		}
		if err := decodeStrictJSONObject(data, &raw); err != nil {
			return err
		}
		if raw.Prompt == nil || *raw.Prompt == "" {
			return fmt.Errorf("prompt is required")
		}
		return nil
	default:
		return fmt.Errorf("type %q is invalid", hookType)
	}
}

func commandHook(command *string, arguments json.RawMessage, timeout *int, asynchronous *bool) (hookSpec, error) {
	if command == nil || *command == "" {
		return hookSpec{}, fmt.Errorf("command is required")
	}
	hook := hookSpec{Command: *command, Timeout: timeout}
	if asynchronous != nil {
		hook.Asynchronous = *asynchronous
	}
	if arguments == nil {
		return hook, nil
	}
	if string(arguments) == "null" {
		return hookSpec{}, fmt.Errorf("args must be an array")
	}
	if err := decodeStrictJSON(arguments, &hook.Arguments); err != nil {
		return hookSpec{}, fmt.Errorf("args: %w", err)
	}
	hook.Exec = true
	return hook, nil
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
	resolved, err := resolveMarketplaceSource(pluginRoot, sourcePath)
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

func (i *claudeInspector) hooks(pluginRoot, pluginPath string, source json.RawMessage, assets map[model.AssetID]model.SourceAsset, recognized map[string]struct{}) {
	hooks := i.loadHookSpecs(pluginRoot, pluginPath, source, recognized)
	ordinals := make(map[string]int)
	for _, hook := range hooks {
		event, ok := portableClaudeEvent(hook.Event)
		if !ok {
			i.error(string(hook.Location.Path), fmt.Sprintf("hook event %q is not a portable event", hook.Event))
			continue
		}
		ordinal := ordinals[hook.Event]
		ordinals[hook.Event]++
		if hook.NativeOnly {
			i.native = append(i.native, model.NativeGap{
				Component: string(hook.Location.Path) + "#" + hook.Component,
				Location:  hook.Location,
			})
			continue
		}

		name := fmt.Sprintf("%s-%d", hook.Event, ordinal+1)
		identity, err := model.NewAssetID("hook/" + name)
		if err != nil {
			i.error(string(hook.Location.Path), "hook identity: "+err.Error())
			continue
		}
		matcher, err := portableClaudeMatcher(event, hook.Matcher)
		if err != nil {
			i.error(string(hook.Location.Path), fmt.Sprintf("hook %q matcher: %v", identity, err))
			continue
		}
		timeout, err := portableClaudeTimeout(event, hook.Timeout)
		if err != nil {
			i.error(string(hook.Location.Path), fmt.Sprintf("hook %q timeout: %v", identity, err))
			continue
		}
		handler, files, err := i.portableClaudeCommand(pluginRoot, hook, recognized)
		if err != nil {
			i.error(string(hook.Location.Path), fmt.Sprintf("hook %q command: %v", identity, err))
			continue
		}

		frontmatter := map[string]any{"event": hook.Event}
		if hook.Matcher != nil {
			frontmatter["matcher"] = *hook.Matcher
		}
		if hook.Timeout != nil {
			frontmatter["timeout"] = *hook.Timeout
		}
		if hook.Asynchronous {
			frontmatter["async"] = true
		}
		descriptor := model.HookDescriptor{
			Identity:            identity,
			Location:            hook.Location,
			Event:               event,
			Matcher:             matcher,
			Handler:             handler,
			TimeoutMilliseconds: timeout,
			Asynchronous:        hook.Asynchronous,
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
				Files:       files,
			},
			Hook:           &descriptor,
			CapabilityUses: mergeHookCapabilities(descriptor, caps),
			Overlays:       overlays,
		}
	}
}

func (i *claudeInspector) loadHookSpecs(pluginRoot, pluginPath string, source json.RawMessage, recognized map[string]struct{}) []hookSpec {
	if source == nil {
		return i.loadHookFile(pluginRoot, filepath.Join(pluginRoot, "hooks", "hooks.json"), false, recognized)
	}
	source = json.RawMessage(strings.TrimSpace(string(source)))
	var sources []json.RawMessage
	if len(source) != 0 && source[0] == '[' {
		if err := decodeStrictJSON(source, &sources); err != nil {
			i.error(i.relativePath(pluginPath), "plugin manifest hooks: "+err.Error())
			return nil
		}
		if len(sources) == 0 {
			i.error(i.relativePath(pluginPath), "plugin manifest hooks must not be empty")
			return nil
		}
	} else {
		sources = []json.RawMessage{source}
	}

	var result []hookSpec
	for index, raw := range sources {
		raw = json.RawMessage(strings.TrimSpace(string(raw)))
		if len(raw) == 0 {
			i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest hooks[%d] must be a path or inline object", index))
			continue
		}
		switch raw[0] {
		case '"':
			var declared string
			if err := decodeStrictJSON(raw, &declared); err != nil {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest hooks[%d]: %v", index, err))
				continue
			}
			if !strings.HasPrefix(declared, "./") {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest hooks[%d] path must start with ./", index))
				continue
			}
			relative := strings.TrimPrefix(declared, "./")
			if _, err := model.NewRelativePath(relative); err != nil {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest hooks[%d] path: %v", index, err))
				continue
			}
			path, err := containedPath(pluginRoot, relative)
			if err != nil {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest hooks[%d] path: %v", index, err))
				continue
			}
			result = append(result, i.loadHookFile(pluginRoot, path, true, recognized)...)
		case '{':
			hooks, err := parseInlineHooks(raw)
			if err != nil {
				i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest inline hooks[%d]: %v", index, err))
				continue
			}
			location := model.SourceLocation{Path: model.RelativePath(i.relativePath(pluginPath))}
			for hookIndex := range hooks {
				hooks[hookIndex].Location = location
			}
			result = append(result, hooks...)
		default:
			i.error(i.relativePath(pluginPath), fmt.Sprintf("plugin manifest hooks[%d] must be a path or inline object", index))
		}
	}
	return result
}

func (i *claudeInspector) loadHookFile(pluginRoot, path string, required bool, recognized map[string]struct{}) []hookSpec {
	info, err := i.lstat(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil
		}
		i.error(i.relativePath(path), "hook file: "+err.Error())
		return nil
	}
	if err := i.noSymlinkComponents(pluginRoot, path); err != nil {
		i.error(i.relativePath(path), "hook file: "+err.Error())
		return nil
	}
	if !info.Mode().IsRegular() {
		i.error(i.relativePath(path), "hook file must be a regular file")
		return nil
	}
	data, ok := i.readInput(path)
	if !ok {
		return nil
	}
	recognized[path] = struct{}{}
	hooks, err := parseHookFile(data)
	if err != nil {
		i.error(i.relativePath(path), "hook file: "+err.Error())
		return nil
	}
	location := model.SourceLocation{Path: model.RelativePath(i.relativePath(path))}
	for index := range hooks {
		hooks[index].Location = location
	}
	return hooks
}

func (i *claudeInspector) portableClaudeCommand(pluginRoot string, hook hookSpec, recognized map[string]struct{}) (model.HookCommand, map[model.RelativePath]model.FileContent, error) {
	files := make(map[model.RelativePath]model.FileContent)
	if hook.Exec {
		if !portableClaudeProgram(hook.Command) || containsClaudeVariable(hook.Command) {
			return model.HookCommand{}, nil, fmt.Errorf("exec-form command must be a portable executable name")
		}
		program := hook.Command
		arguments := make([]model.HookArgument, 0, len(hook.Arguments))
		for index, argument := range hook.Arguments {
			path, packageFile, err := claudePackageFileArgument(argument)
			if err != nil {
				return model.HookCommand{}, nil, fmt.Errorf("argument %d: %w", index, err)
			}
			if !packageFile {
				literal := argument
				arguments = append(arguments, model.HookArgument{Literal: &literal})
				continue
			}
			content, err := i.readHookPayload(pluginRoot, path, recognized)
			if err != nil {
				return model.HookCommand{}, nil, fmt.Errorf("argument %d package file %q: %w", index, path, err)
			}
			files[path] = content
			packagePath := path
			arguments = append(arguments, model.HookArgument{PackageFile: &packagePath})
		}
		return model.HookCommand{Mode: model.HookHandlerModeExec, Program: &program, Arguments: arguments}, files, nil
	}

	if program, path, ok := simpleClaudeShellCommand(hook.Command); ok {
		content, err := i.readHookPayload(pluginRoot, path, recognized)
		if err != nil {
			return model.HookCommand{}, nil, fmt.Errorf("package file %q: %w", path, err)
		}
		files[path] = content
		packagePath := path
		return model.HookCommand{
			Mode:      model.HookHandlerModeExec,
			Program:   &program,
			Arguments: []model.HookArgument{{PackageFile: &packagePath}},
		}, files, nil
	}
	if containsClaudeVariable(hook.Command) {
		return model.HookCommand{}, nil, fmt.Errorf("shell command contains a package or project path reference that is not statically provable")
	}
	command := hook.Command
	return model.HookCommand{Mode: model.HookHandlerModeShell, ShellCommand: &command}, files, nil
}

func (i *claudeInspector) readHookPayload(pluginRoot string, relative model.RelativePath, recognized map[string]struct{}) (model.FileContent, error) {
	path, err := containedPath(pluginRoot, string(relative))
	if err != nil {
		return model.FileContent{}, err
	}
	info, err := i.lstat(path)
	if err != nil {
		return model.FileContent{}, err
	}
	if err := i.noSymlinkComponents(pluginRoot, path); err != nil {
		return model.FileContent{}, err
	}
	if !info.Mode().IsRegular() {
		return model.FileContent{}, fmt.Errorf("must be a regular file")
	}
	data, ok := i.readInput(path)
	if !ok {
		return model.FileContent{}, fmt.Errorf("could not read payload")
	}
	recognized[path] = struct{}{}
	return model.FileContent{
		Bytes:      data,
		Executable: info.Mode().Perm()&0o111 != 0,
		Origin:     []model.SourceLocation{{Path: model.RelativePath(i.relativePath(path))}},
	}, nil
}

func claudePackageFileArgument(value string) (model.RelativePath, bool, error) {
	const prefix = "${CLAUDE_PLUGIN_ROOT}/"
	if strings.HasPrefix(value, prefix) {
		path, err := model.NewRelativePath(strings.TrimPrefix(value, prefix))
		return path, true, err
	}
	if containsClaudeVariable(value) {
		return "", false, fmt.Errorf("path reference is not an exact plugin-root package file")
	}
	return "", false, nil
}

func simpleClaudeShellCommand(command string) (string, model.RelativePath, bool) {
	separator := strings.IndexByte(command, ' ')
	if separator < 1 {
		return "", "", false
	}
	program, reference := command[:separator], command[separator+1:]
	if !simpleClaudeProgram(program) || reference == "" {
		return "", "", false
	}
	const variable = "${CLAUDE_PLUGIN_ROOT}"
	var relative string
	switch {
	case strings.HasPrefix(reference, variable+"/"):
		relative = strings.TrimPrefix(reference, variable+"/")
	case strings.HasPrefix(reference, `"`+variable+`/`) && strings.HasSuffix(reference, `"`):
		relative = strings.TrimSuffix(strings.TrimPrefix(reference, `"`+variable+`/`), `"`)
	case strings.HasPrefix(reference, `"`+variable+`"/`):
		relative = strings.TrimPrefix(reference, `"`+variable+`"/`)
	default:
		return "", "", false
	}
	path, err := model.NewRelativePath(relative)
	if err != nil {
		return "", "", false
	}
	return program, path, true
}

func portableClaudeProgram(program string) bool {
	return program != "" && strings.TrimSpace(program) == program && !strings.ContainsAny(program, "\x00/\\")
}

func simpleClaudeProgram(program string) bool {
	if !portableClaudeProgram(program) {
		return false
	}
	for _, character := range program {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._+-", character) {
			continue
		}
		return false
	}
	return true
}

func containsClaudeVariable(value string) bool {
	return strings.Contains(value, "CLAUDE_PLUGIN_ROOT") || strings.Contains(value, "CLAUDE_PLUGIN_DATA") || strings.Contains(value, "CLAUDE_PROJECT_DIR") || strings.Contains(value, "${user_config.")
}

func mergeHookCapabilities(descriptor model.HookDescriptor, declared []model.CapabilityUse) []model.CapabilityUse {
	location := descriptor.Location
	uses := []model.CapabilityUse{
		{Key: "asset.hook", Location: location},
		{Key: model.CapabilityKey("hook.command." + string(descriptor.Handler.Mode)), Location: location},
		{Key: model.CapabilityKey("hook.event." + string(descriptor.Event)), Location: location},
	}
	if descriptor.Matcher != nil {
		uses = append(uses, model.CapabilityUse{Key: "hook.matcher.tool-category", Location: location})
	}
	if descriptor.Asynchronous {
		uses = append(uses, model.CapabilityUse{Key: "hook.async", Location: location})
	}
	if descriptor.FailurePolicy == model.HookFailurePolicyClosed {
		uses = append(uses, model.CapabilityUse{Key: "hook.failure.closed", Location: location})
	}
	uses = append(uses, declared...)
	byKey := make(map[model.CapabilityKey]model.CapabilityUse, len(uses))
	for _, use := range uses {
		if _, exists := byKey[use.Key]; !exists {
			byKey[use.Key] = use
		}
	}
	keys := make([]model.CapabilityKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	result := make([]model.CapabilityUse, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
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

func portableClaudeMatcher(event model.HookEvent, matcher *string) (*model.HookMatcher, error) {
	if matcher == nil || *matcher == "" || *matcher == "*" {
		return nil, nil
	}
	if event != model.HookEventPreTool && event != model.HookEventPostTool && event != model.HookEventPostToolFailure {
		return nil, fmt.Errorf("event %q does not have a portable tool-category matcher", event)
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
		"Agent":        model.HookToolCategoryTask,
	}
	completeCategoryNames := map[model.HookToolCategory][]string{
		model.HookToolCategoryEdit:   {"Edit", "NotebookEdit"},
		model.HookToolCategorySearch: {"Glob", "Grep"},
		model.HookToolCategoryWeb:    {"WebFetch", "WebSearch"},
		model.HookToolCategoryTask:   {"Task", "Agent"},
	}
	values := strings.FieldsFunc(*matcher, func(character rune) bool { return character == '|' || character == ',' })
	if len(values) == 0 {
		return nil, fmt.Errorf("%q is not an exact known native tool name or list", *matcher)
	}
	seenCategories := make(map[model.HookToolCategory]bool)
	seenNames := make(map[string]bool)
	tools := make([]model.HookToolCategory, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		category, ok := categories[value]
		if !ok {
			return nil, fmt.Errorf("%q is not an exact known native tool name or list", *matcher)
		}
		seenNames[value] = true
		if !seenCategories[category] {
			seenCategories[category] = true
			tools = append(tools, category)
		}
	}
	for _, category := range tools {
		for _, name := range completeCategoryNames[category] {
			if !seenNames[name] {
				return nil, fmt.Errorf("%q does not contain the complete native expansion for portable category %q (%s)", *matcher, category, strings.Join(completeCategoryNames[category], "|"))
			}
		}
	}
	return &model.HookMatcher{Tools: tools}, nil
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
