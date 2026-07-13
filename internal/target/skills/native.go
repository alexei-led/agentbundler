// Package skills renders the common native skill subset for target adapters.
package skills

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Render emits one native skill-root tree. Native targets currently have no
// portable multi-package aggregation contract, so exactly one package is accepted.
func Render(target model.TargetID, root string, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
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
		if asset.Kind != model.AssetKindSkill {
			return empty(target), []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("target %q native skill output does not support %q", target, "asset."+string(asset.Kind)))}
		}
		for _, use := range asset.CapabilityUses {
			if use.Key != "asset.skill" {
				return empty(target), []model.Diagnostic{diagnostic("unsupported-capability", fmt.Sprintf("target %q native skill output does not support capability %q", target, use.Key))}
			}
		}
		name := strings.TrimPrefix(string(asset.Identity), string(model.AssetKindSkill)+"/")
		if name == "" || strings.Contains(name, "/") {
			return empty(target), []model.Diagnostic{diagnostic("invalid-skill-identity", fmt.Sprintf("skill identity %q cannot be rendered as a native skill directory", asset.Identity))}
		}
		base := strings.TrimSuffix(root, "/") + "/" + name
		content, err := markdown(asset.Content.Frontmatter, asset.Content.Body)
		if err != nil {
			return empty(target), []model.Diagnostic{diagnostic("invalid-skill-frontmatter", err.Error())}
		}
		if err := add(&files, paths, model.RelativePath(base+"/SKILL.md"), content); err != nil {
			return empty(target), []model.Diagnostic{diagnostic("duplicate-output-path", err.Error())}
		}
		supportPaths := make([]model.RelativePath, 0, len(asset.Content.Files))
		for path := range asset.Content.Files {
			supportPaths = append(supportPaths, path)
		}
		sort.Slice(supportPaths, func(i, j int) bool { return supportPaths[i] < supportPaths[j] })
		for _, path := range supportPaths {
			if err := add(&files, paths, model.RelativePath(base+"/"+string(path)), asset.Content.Files[path]); err != nil {
				return empty(target), []model.Diagnostic{diagnostic("duplicate-output-path", err.Error())}
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return model.TargetPlan{Target: target, Packages: []model.PackageID{pkg.Identity}, Files: files, NativeChecks: []model.NativeCheck{}}, nil
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
