// Package packageoutput renders installable target package roots.
package packageoutput

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Render emits one or more self-contained installable package roots for a supported target.
// A single package keeps the target's historical flat layout. Multiple packages are
// namespaced by package ID so each root can be installed independently.
func Render(target model.TargetID, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if len(packages) == 0 {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-package-aggregation", "installable target output requires at least one package")}
	}
	for _, pkg := range packages {
		if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
			return empty(target), diagnostics
		}
		if pkg.Target != target {
			return empty(target), []model.Diagnostic{diagnostic("target-mismatch", fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, target))}
		}
		if pkg.Profile != model.TargetProfilePackage {
			return empty(target), []model.Diagnostic{diagnostic("invalid-target-profile", fmt.Sprintf("target %q requires package profile", target))}
		}
	}
	if target != model.TargetClaude && target != model.TargetCodex && target != model.TargetPi && target != model.TargetCopilot && target != model.TargetCursor {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-target-profile", fmt.Sprintf("target %q has no installable package layout", target))}
	}

	orderedPackages := append([]model.NormalizedPackage(nil), packages...)
	sort.Slice(orderedPackages, func(left, right int) bool { return orderedPackages[left].Identity < orderedPackages[right].Identity })
	packageIDs := make([]model.PackageID, 0, len(orderedPackages))
	for _, pkg := range orderedPackages {
		packageIDs = append(packageIDs, pkg.Identity)
	}
	plan := model.TargetPlan{Target: target, Packages: packageIDs, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
	paths := make(map[model.RelativePath]struct{})
	for _, pkg := range orderedPackages {
		root := packageRoot(len(packages), pkg.Identity)
		for _, asset := range sortedAssets(pkg.Assets) {
			if err := renderAsset(&plan.Files, paths, target, root, asset); err != nil {
				if fieldError, ok := err.(*unsupportedAgentFieldError); ok {
					return empty(target), []model.Diagnostic{fieldError.diagnostic()}
				}
				return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
			}
		}
		manifestBytes, err := manifest(target, pkg)
		if err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-manifest", err.Error())}
		}
		if err := add(&plan.Files, paths, rootedPath(root, "README.md"), packageReadme(pkg)); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
		if err := add(&plan.Files, paths, rootedPath(root, manifestPath(target)), manifestBytes); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
	}
	sort.Slice(plan.Files, func(left, right int) bool { return plan.Files[left].Path < plan.Files[right].Path })
	return plan, nil
}

func packageRoot(packageCount int, packageID model.PackageID) string {
	if packageCount == 1 {
		return ""
	}
	return string(packageID)
}

func rootedPath(root, value string) model.RelativePath {
	if root == "" {
		return model.RelativePath(value)
	}
	return model.RelativePath(root + "/" + value)
}

func manifestPath(target model.TargetID) string {
	switch target {
	case model.TargetCodex:
		return ".codex-plugin/plugin.json"
	case model.TargetPi:
		return "package.json"
	case model.TargetCopilot:
		return "plugin.json"
	case model.TargetCursor:
		return ".cursor-plugin/plugin.json"
	default:
		return ".claude-plugin/plugin.json"
	}
}

func renderAsset(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, target model.TargetID, root string, asset model.NormalizedAsset) error {
	name := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("asset identity %q cannot be rendered in a package root", asset.Identity)
	}
	switch asset.Kind {
	case model.AssetKindSkill:
		return renderSkill(files, paths, root, asset, "skills/"+name)
	case model.AssetKindResource:
		for path, data := range asset.Content.Files {
			if err := add(files, paths, rootedPath(root, "resources/"+name+"/"+string(path)), data); err != nil {
				return err
			}
		}
		return nil
	case model.AssetKindAgent:
		if target == model.TargetClaude || target == model.TargetCopilot || target == model.TargetCursor {
			frontmatter, err := agentFrontmatter(target, asset.Content.Frontmatter)
			if err != nil {
				setUnsupportedAgentFieldAsset(err, asset.Identity)
				return err
			}
			data, err := markdown(frontmatter, asset.Content.Body)
			if err != nil {
				return err
			}
			extension := ".md"
			if target == model.TargetCopilot {
				extension = ".agent.md"
			}
			return add(files, paths, rootedPath(root, "agents/"+name+extension), data)
		}
		if target == model.TargetPi {
			frontmatter, err := agentFrontmatter(target, asset.Content.Frontmatter)
			if err != nil {
				setUnsupportedAgentFieldAsset(err, asset.Identity)
				return err
			}
			data, err := piSubagent(frontmatter, asset.Content.Body)
			if err != nil {
				return err
			}
			return add(files, paths, rootedPath(root, "agents/"+name+".md"), data)
		}
		if target == model.TargetCodex {
			data, err := codexAgent(asset)
			if err != nil {
				return err
			}
			return add(files, paths, rootedPath(root, "agents/"+name+".toml"), data)
		}
		return fmt.Errorf("target %q does not support package agent asset %q", target, asset.Identity)
	default:
		return fmt.Errorf("target %q does not support package asset %q", target, asset.Identity)
	}
}

func agentFrontmatter(target model.TargetID, source map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(source))
	for key, value := range source {
		if key == "sandbox_mode" && target != model.TargetCodex {
			return nil, &unsupportedAgentFieldError{Target: target, Field: key}
		}
		result[key] = value
	}
	return result, nil
}

type unsupportedAgentFieldError struct {
	Target model.TargetID
	Asset  model.AssetID
	Field  string
}

func setUnsupportedAgentFieldAsset(err error, asset model.AssetID) {
	if fieldError, ok := err.(*unsupportedAgentFieldError); ok {
		fieldError.Asset = asset
	}
}

func (e *unsupportedAgentFieldError) Error() string {
	return fmt.Sprintf("agent %q field %q is unsupported by target %q", e.Asset, e.Field, e.Target)
}

func (e *unsupportedAgentFieldError) diagnostic() model.Diagnostic {
	return model.Diagnostic{
		Code:     "unsupported-agent-field",
		Severity: model.SeverityError,
		Message:  e.Error(),
		Hint:     `move "sandbox_mode" from base agent frontmatter to <agent-directory>/.agentbundler/targets/codex.json: {"frontmatterPatch":{"sandbox_mode":"read-only"}}`,
		Asset:    e.Asset,
		Field:    e.Field,
		Targets:  []model.TargetID{e.Target},
	}
}

func renderSkill(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, packageRoot string, asset model.NormalizedAsset, root string) error {
	data, err := markdown(asset.Content.Frontmatter, asset.Content.Body)
	if err != nil {
		return err
	}
	if err := add(files, paths, rootedPath(packageRoot, root+"/SKILL.md"), data); err != nil {
		return err
	}
	for path, data := range asset.Content.Files {
		if err := add(files, paths, rootedPath(packageRoot, root+"/"+string(path)), data); err != nil {
			return err
		}
	}
	return nil
}

func packageReadme(pkg model.NormalizedPackage) []byte {
	title := string(pkg.Identity)
	if value, ok := pkg.Metadata["displayName"].(string); ok && value != "" {
		title = value
	}
	lines := []string{"# " + title, ""}
	if description, ok := pkg.Metadata["description"].(string); ok && description != "" {
		lines = append(lines, description, "")
	}
	lines = append(lines, "Generated by **Agent Bundler**.")
	if homepage, ok := pkg.Metadata["homepage"].(string); ok && homepage != "" {
		lines = append(lines, "", "More information: "+homepage)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func manifest(target model.TargetID, pkg model.NormalizedPackage) ([]byte, error) {
	base := map[string]any{"name": pkg.Identity}
	switch target {
	case model.TargetClaude:
		copyMetadata(base, pkg.Metadata, "version", "description", "author", "license", "homepage", "repository", "keywords")
		if value, ok := base["author"]; ok {
			base["author"] = personMetadata(value)
		}
		base["$schema"] = "https://json.schemastore.org/claude-code-plugin-manifest.json"
	case model.TargetCodex:
		copyMetadata(base, pkg.Metadata, "version", "description", "author", "license", "homepage", "repository", "keywords", "interface")
		if value, ok := base["author"]; ok {
			base["author"] = personMetadata(value)
		}
		base["skills"] = "./skills"
	case model.TargetPi:
		copyMetadata(base, pkg.Metadata, "version", "description", "keywords", "license", "homepage", "repository")
		if value, ok := pkg.Metadata["dependencies"]; ok {
			dependencies, err := dependencyMetadata(value)
			if err != nil {
				return nil, fmt.Errorf("dependencies: %w", err)
			}
			base["dependencies"] = dependencies
		}
		base["keywords"] = appendKeywords(base["keywords"])
		pi := map[string]any{"skills": []string{"./skills"}}
		if packageHasAsset(pkg, model.AssetKindAgent) {
			pi["subagents"] = map[string]any{"agents": []string{"./agents"}}
		}
		base["pi"] = pi
	case model.TargetCopilot:
		copyMetadata(base, pkg.Metadata, "version", "description", "author", "license", "homepage", "repository", "keywords")
		if value, ok := base["author"]; ok {
			base["author"] = personMetadata(value)
		}
		base["skills"] = []string{"skills/"}
		if packageHasAsset(pkg, model.AssetKindAgent) {
			base["agents"] = "agents/"
		}
	case model.TargetCursor:
		copyMetadata(base, pkg.Metadata, "version", "description", "displayName", "homepage", "repository", "license", "keywords")
		base["skills"] = "./skills/"
	}
	data, err := json.Marshal(orderedManifest(base, target))
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func copyMetadata(destination map[string]any, metadata model.PackageMetadata, keys ...string) {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			destination[key] = value
		}
	}
}

func personMetadata(value any) any {
	switch value := value.(type) {
	case string:
		if value != "" {
			return map[string]string{"name": value}
		}
	case map[string]any:
		return value
	}
	return value
}

func orderedManifest(values map[string]any, target model.TargetID) map[string]any {
	// encoding/json sorts map keys, but this helper keeps the target-specific
	// manifest shape explicit and makes future schema review local to this file.
	return values
}

func packageHasAsset(pkg model.NormalizedPackage, kind model.AssetKind) bool {
	for _, asset := range pkg.Assets {
		if asset.Kind == kind {
			return true
		}
	}
	return false
}

func dependencyMetadata(value any) (map[string]string, error) {
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

func appendKeywords(value any) []string {
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

func markdown(frontmatter map[string]any, body string) ([]byte, error) {
	if len(frontmatter) == 0 {
		return []byte(body), nil
	}
	encoded, err := json.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(encoded) + "\n---\n" + body), nil
}

func piSubagent(frontmatter map[string]any, body string) ([]byte, error) {
	keys := make([]string, 0, len(frontmatter))
	for key := range frontmatter {
		if key == "sandbox_mode" {
			continue
		}
		if !validPiSubagentKey(key) {
			return nil, fmt.Errorf("pi subagent frontmatter key %q is invalid", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	keys = prioritizePiSubagentKeys(keys)

	lines := make([]string, 0, len(keys)+2)
	lines = append(lines, "---")
	for _, key := range keys {
		value, err := piSubagentValue(frontmatter[key])
		if err != nil {
			return nil, fmt.Errorf("pi subagent frontmatter %q: %w", key, err)
		}
		lines = append(lines, key+": "+value)
	}
	lines = append(lines, "---")
	return []byte(strings.Join(lines, "\n") + "\n" + body), nil
}

func validPiSubagentKey(value string) bool {
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

func prioritizePiSubagentKeys(keys []string) []string {
	result := make([]string, 0, len(keys))
	for _, priority := range []string{"name", "description"} {
		for _, key := range keys {
			if key == priority {
				result = append(result, key)
			}
		}
	}
	for _, key := range keys {
		if key != "name" && key != "description" {
			result = append(result, key)
		}
	}
	return result
}

func piSubagentValue(value any) (string, error) {
	var text string
	switch value := value.(type) {
	case string:
		text = value
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		text = fmt.Sprint(value)
	default:
		return "", fmt.Errorf("must be a scalar")
	}
	if strings.ContainsAny(text, "\r\n") {
		return "", fmt.Errorf("must be single-line")
	}
	return text, nil
}

func codexAgent(asset model.NormalizedAsset) ([]byte, error) {
	name := strings.TrimPrefix(string(asset.Identity), "agent/")
	if value, ok := asset.Content.Frontmatter["name"].(string); ok && value != "" {
		name = value
	}
	description, _ := asset.Content.Frontmatter["description"].(string)
	if description == "" {
		return nil, fmt.Errorf("agent %q requires string description frontmatter", asset.Identity)
	}
	lines := []string{fmt.Sprintf("name = %s", tomlString(name)), fmt.Sprintf("description = %s", tomlString(description))}
	if value, ok := asset.Content.Frontmatter["sandbox_mode"].(string); ok && value != "" {
		lines = append(lines, fmt.Sprintf("sandbox_mode = %s", tomlString(value)))
	}
	lines = append(lines, "developer_instructions = "+tomlMultiline(asset.Content.Body), "")
	return []byte(strings.Join(lines, "\n")), nil
}

func tomlString(value string) string {
	value = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\b", "\\b", "\t", "\\t", "\n", "\\n", "\f", "\\f", "\r", "\\r").Replace(value)
	return `"` + value + `"`
}

func tomlMultiline(value string) string {
	return "\"\"\"\n" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"\"\"", "\\\"\\\"\\\"") + "\n\"\"\""
}

func sortedAssets(assets []model.NormalizedAsset) []model.NormalizedAsset {
	result := append([]model.NormalizedAsset(nil), assets...)
	sort.Slice(result, func(left, right int) bool { return result[left].Identity < result[right].Identity })
	return result
}

func add(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, path model.RelativePath, data []byte) error {
	if _, exists := paths[path]; exists {
		return fmt.Errorf("generated output path %q is duplicated", path)
	}
	paths[path] = struct{}{}
	*files = append(*files, model.PlannedFile{Path: path, Bytes: append([]byte(nil), data...)})
	return nil
}

func empty(target model.TargetID) model.TargetPlan {
	return model.TargetPlan{Target: target, Packages: []model.PackageID{}, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
}

func diagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}
