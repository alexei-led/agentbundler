// Package source routes explicit source manifests to their topology importers.
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/compiler/source/bundle"
	"github.com/alexei-led/agentbundler/internal/compiler/source/claudeplugin"
	"github.com/alexei-led/agentbundler/internal/compiler/source/skillrepo"
)

const diagnosticCodeInvalidSourceImport = "invalid-source-import"

// Import validates a source manifest and workspace root, then imports the declared source layout.
func Import(manifest model.SourceManifest, workspaceRoot string) (model.SourceInventory, []model.Diagnostic) {
	if diagnostics := model.ValidateSourceManifest(manifest); hasErrors(diagnostics) {
		return model.SourceInventory{}, sortDiagnostics(diagnostics)
	}
	if diagnostics := validateWorkspaceRoot(workspaceRoot); len(diagnostics) != 0 {
		return model.SourceInventory{}, diagnostics
	}
	workspace, err := os.OpenRoot(workspaceRoot)
	if err != nil {
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", "workspace root: "+err.Error())}
	}
	defer func() { _ = workspace.Close() }()

	var inventory model.SourceInventory
	var diagnostics []model.Diagnostic
	switch manifest.Kind {
	case model.SourceKindBundle:
		inventory, diagnostics = bundle.InspectBundleRoot(manifest, workspaceRoot, workspace)
	case model.SourceKindClaudePlugin:
		inventory, diagnostics = claudeplugin.InspectClaudePluginRoot(manifest, workspaceRoot, workspace)
	case model.SourceKindSkillsRepository:
		inventory, diagnostics = skillrepo.InspectSkillRepoRoot(manifest, workspaceRoot, workspace)
	default:
		return model.SourceInventory{}, []model.Diagnostic{diagnostic("", fmt.Sprintf("unsupported source kind %q", manifest.Kind))}
	}
	if hasErrors(diagnostics) {
		return model.SourceInventory{}, sortDiagnostics(diagnostics)
	}
	return normalizeInventory(inventory), sortDiagnostics(diagnostics)
}

func validateWorkspaceRoot(workspaceRoot string) []model.Diagnostic {
	if !filepath.IsAbs(workspaceRoot) || filepath.Clean(workspaceRoot) != workspaceRoot {
		return []model.Diagnostic{diagnostic("", "workspace root must be a cleaned absolute path")}
	}
	info, err := os.Lstat(workspaceRoot)
	if err != nil {
		return []model.Diagnostic{diagnostic("", "workspace root: "+err.Error())}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return []model.Diagnostic{diagnostic("", "workspace root must be a non-symlink directory")}
	}
	return nil
}

func normalizeInventory(inventory model.SourceInventory) model.SourceInventory {
	sort.Slice(inventory.Packages, func(i, j int) bool {
		return inventory.Packages[i].Identity < inventory.Packages[j].Identity
	})
	for packageIndex := range inventory.Packages {
		sort.Slice(inventory.Packages[packageIndex].Assets, func(i, j int) bool {
			return inventory.Packages[packageIndex].Assets[i].Identity < inventory.Packages[packageIndex].Assets[j].Identity
		})
	}
	sort.Slice(inventory.NativeGaps, func(i, j int) bool {
		left, right := inventory.NativeGaps[i], inventory.NativeGaps[j]
		if left.Location.Path != right.Location.Path {
			return left.Location.Path < right.Location.Path
		}
		if left.Component != right.Component {
			return left.Component < right.Component
		}
		if optionalAssetID(left.Asset) != optionalAssetID(right.Asset) {
			return optionalAssetID(left.Asset) < optionalAssetID(right.Asset)
		}
		return optionalTargetID(left.Target) < optionalTargetID(right.Target)
	})
	sort.Slice(inventory.Inputs, func(i, j int) bool {
		return inventory.Inputs[i].Path < inventory.Inputs[j].Path
	})
	return inventory
}

func sortDiagnostics(diagnostics []model.Diagnostic) []model.Diagnostic {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Severity != right.Severity {
			return left.Severity < right.Severity
		}
		if diagnosticPath(left) != diagnosticPath(right) {
			return diagnosticPath(left) < diagnosticPath(right)
		}
		if diagnosticLine(left) != diagnosticLine(right) {
			return diagnosticLine(left) < diagnosticLine(right)
		}
		if diagnosticColumn(left) != diagnosticColumn(right) {
			return diagnosticColumn(left) < diagnosticColumn(right)
		}
		return left.Message < right.Message
	})
	return diagnostics
}

func diagnostic(path model.RelativePath, message string) model.Diagnostic {
	diagnostic := model.Diagnostic{Code: diagnosticCodeInvalidSourceImport, Severity: model.SeverityError, Message: message}
	if path != "" {
		diagnostic.Location = &model.SourceLocation{Path: path}
	}
	return diagnostic
}

func diagnosticPath(diagnostic model.Diagnostic) model.RelativePath {
	if diagnostic.Location == nil {
		return ""
	}
	return diagnostic.Location.Path
}

func diagnosticLine(diagnostic model.Diagnostic) int {
	if diagnostic.Location == nil || diagnostic.Location.Line == nil {
		return 0
	}
	return *diagnostic.Location.Line
}

func diagnosticColumn(diagnostic model.Diagnostic) int {
	if diagnostic.Location == nil || diagnostic.Location.Column == nil {
		return 0
	}
	return *diagnostic.Location.Column
}

func optionalAssetID(identity *model.AssetID) model.AssetID {
	if identity == nil {
		return ""
	}
	return *identity
}

func optionalTargetID(target *model.TargetID) model.TargetID {
	if target == nil {
		return ""
	}
	return *target
}

func hasErrors(diagnostics []model.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
