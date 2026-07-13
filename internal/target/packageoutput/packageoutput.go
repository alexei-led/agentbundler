// Package packageoutput renders installable target package roots.
package packageoutput

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Render emits one installable package root for a supported target.
func Render(target model.TargetID, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if len(packages) != 1 {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-package-aggregation", "installable target output requires exactly one package")}
	}
	pkg := packages[0]
	if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		return empty(target), diagnostics
	}
	if pkg.Target != target {
		return empty(target), []model.Diagnostic{diagnostic("target-mismatch", fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, target))}
	}
	if pkg.Profile != model.TargetProfilePackage {
		return empty(target), []model.Diagnostic{diagnostic("invalid-target-profile", fmt.Sprintf("target %q requires package profile", target))}
	}
	if target != model.TargetClaude && target != model.TargetCodex && target != model.TargetPi && target != model.TargetCursor {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-target-profile", fmt.Sprintf("target %q has no installable package layout", target))}
	}

	plan := model.TargetPlan{Target: target, Packages: []model.PackageID{pkg.Identity}, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
	paths := make(map[model.RelativePath]struct{})
	for _, asset := range sortedAssets(pkg.Assets) {
		if err := renderAsset(&plan.Files, paths, target, asset); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
		}
	}
	manifest, err := manifest(target, pkg)
	if err != nil {
		return empty(target), []model.Diagnostic{diagnostic("invalid-package-manifest", err.Error())}
	}
	manifestPath := model.RelativePath(".claude-plugin/plugin.json")
	if target == model.TargetCodex {
		manifestPath = ".codex-plugin/plugin.json"
	} else if target == model.TargetPi {
		manifestPath = "package.json"
	} else if target == model.TargetCursor {
		manifestPath = ".cursor-plugin/plugin.json"
	}
	if err := add(&plan.Files, paths, manifestPath, manifest); err != nil {
		return empty(target), []model.Diagnostic{diagnostic("invalid-package-output", err.Error())}
	}
	sort.Slice(plan.Files, func(left, right int) bool { return plan.Files[left].Path < plan.Files[right].Path })
	return plan, nil
}

func renderAsset(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, target model.TargetID, asset model.NormalizedAsset) error {
	name := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("asset identity %q cannot be rendered in a package root", asset.Identity)
	}
	switch asset.Kind {
	case model.AssetKindSkill:
		return renderSkill(files, paths, asset, "skills/"+name)
	case model.AssetKindResource:
		for path, data := range asset.Content.Files {
			if err := add(files, paths, model.RelativePath("resources/"+name+"/"+string(path)), data); err != nil {
				return err
			}
		}
		return nil
	case model.AssetKindAgent:
		if target == model.TargetClaude {
			data, err := markdown(agentFrontmatter(target, asset.Content.Frontmatter), asset.Content.Body)
			if err != nil {
				return err
			}
			return add(files, paths, model.RelativePath("agents/"+name+".md"), data)
		}
		if target == model.TargetCodex {
			data, err := codexAgent(asset)
			if err != nil {
				return err
			}
			return add(files, paths, model.RelativePath("agents/"+name+".toml"), data)
		}
		return fmt.Errorf("target %q does not support package agent asset %q", target, asset.Identity)
	default:
		return fmt.Errorf("target %q does not support package asset %q", target, asset.Identity)
	}
}

func agentFrontmatter(target model.TargetID, source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		if target == model.TargetClaude && key == "sandbox_mode" {
			continue
		}
		result[key] = value
	}
	return result
}

func renderSkill(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, asset model.NormalizedAsset, root string) error {
	data, err := markdown(asset.Content.Frontmatter, asset.Content.Body)
	if err != nil {
		return err
	}
	if err := add(files, paths, model.RelativePath(root+"/SKILL.md"), data); err != nil {
		return err
	}
	for path, data := range asset.Content.Files {
		if err := add(files, paths, model.RelativePath(root+"/"+string(path)), data); err != nil {
			return err
		}
	}
	return nil
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
		base["keywords"] = appendKeywords(base["keywords"])
		base["pi"] = map[string]any{"skills": []string{"./skills"}}
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
