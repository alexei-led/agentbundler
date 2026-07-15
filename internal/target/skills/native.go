// Package skills renders the common native skill subset for target adapters.
package skills

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Render emits one native skill-root tree. Project output remains single-package;
// installable multi-package output is handled by packageoutput.
func Render(target model.TargetID, root string, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	return render(target, root, "", packages)
}

// RenderProject emits skills and declared portable resources beneath one native project root.
func RenderProject(target model.TargetID, skillRoot, resourceRoot string, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if resourceRoot == "" {
		return empty(target), []model.Diagnostic{diagnostic("invalid-resource-root", "native project resource root must not be empty")}
	}
	return render(target, skillRoot, resourceRoot, packages)
}

func render(target model.TargetID, root, resourceRoot string, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if len(packages) != 1 {
		return empty(target), []model.Diagnostic{diagnostic("unsupported-package-aggregation", "target-native skill output requires exactly one package")}
	}
	pkg := packages[0]
	if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		return empty(target), diagnostics
	}
	if pkg.Target != target {
		return empty(target), []model.Diagnostic{diagnostic("target-mismatch", fmt.Sprintf("package %q targets %q, not %q", pkg.Identity, pkg.Target, target))}
	}

	assets := append([]model.NormalizedAsset(nil), pkg.Assets...)
	sort.Slice(assets, func(i, j int) bool { return assets[i].Identity < assets[j].Identity })
	files := make([]model.PlannedFile, 0, len(assets))
	paths := make(map[model.RelativePath]struct{})
	for _, asset := range assets {
		expectedCapability := model.CapabilityKey("asset." + string(asset.Kind))
		for _, use := range asset.CapabilityUses {
			if use.Key != expectedCapability {
				return empty(target), []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("target %q native skill output does not support capability %q", target, use.Key))}
			}
		}
		name := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
		if name == "" || strings.Contains(name, "/") {
			return empty(target), []model.Diagnostic{diagnostic("invalid-asset-identity", fmt.Sprintf("asset identity %q cannot be rendered as a native directory", asset.Identity))}
		}

		var base string
		switch asset.Kind {
		case model.AssetKindSkill:
			base = strings.TrimSuffix(root, "/") + "/" + name
			content, err := markdown(asset.Content.Frontmatter, asset.Content.Body)
			if err != nil {
				return empty(target), []model.Diagnostic{diagnostic("invalid-skill-frontmatter", err.Error())}
			}
			if err := add(&files, paths, model.RelativePath(base+"/SKILL.md"), content); err != nil {
				return empty(target), []model.Diagnostic{diagnostic("duplicate-output-path", err.Error())}
			}
		case model.AssetKindResource:
			if resourceRoot == "" {
				return empty(target), []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("target %q native skill output does not support %q", target, expectedCapability))}
			}
			base = strings.TrimSuffix(resourceRoot, "/") + "/" + name
		default:
			return empty(target), []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("target %q native skill output does not support %q", target, expectedCapability))}
		}
		if err := addSupportFiles(&files, paths, base, asset.Content.Files); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("duplicate-output-path", err.Error())}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return model.TargetPlan{Target: target, Packages: []model.PackageID{pkg.Identity}, Files: files, NativeChecks: []model.NativeCheck{}}, nil
}

func addSupportFiles(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, base string, supportFiles map[model.RelativePath]model.FileContent) error {
	supportPaths := make([]model.RelativePath, 0, len(supportFiles))
	for path := range supportFiles {
		supportPaths = append(supportPaths, path)
	}
	sort.Slice(supportPaths, func(i, j int) bool { return supportPaths[i] < supportPaths[j] })
	for _, path := range supportPaths {
		if err := add(files, paths, model.RelativePath(base+"/"+string(path)), supportFiles[path].Bytes); err != nil {
			return err
		}
	}
	return nil
}

func markdown(frontmatter map[string]any, body string) ([]byte, error) {
	if len(frontmatter) == 0 {
		return []byte(body), nil
	}
	encoded, err := json.Marshal(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("encode skill frontmatter: %w", err)
	}
	return []byte("---\n" + string(encoded) + "\n---\n" + body), nil
}

func add(files *[]model.PlannedFile, paths map[model.RelativePath]struct{}, path model.RelativePath, bytes []byte) error {
	if _, exists := paths[path]; exists {
		return fmt.Errorf("generated output path %q is duplicated", path)
	}
	paths[path] = struct{}{}
	*files = append(*files, model.PlannedFile{Path: path, Bytes: append([]byte(nil), bytes...)})
	return nil
}

func empty(target model.TargetID) model.TargetPlan {
	return model.TargetPlan{Target: target, NativeChecks: []model.NativeCheck{}}
}

func diagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}
