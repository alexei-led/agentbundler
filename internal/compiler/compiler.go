// Package compiler coordinates source import, composition, target rendering, and artifacts.
package compiler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/alexei-led/agentbundler/internal/artifact"
	"github.com/alexei-led/agentbundler/internal/compiler/composition"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/compiler/source"
	"github.com/alexei-led/agentbundler/internal/target"
)

// BuildMode selects whether compilation writes or checks generated output.
type BuildMode string

const (
	BuildModeBuild BuildMode = "build"
	BuildModeCheck BuildMode = "check"
)

// CompileRequest contains operational context and normalized compiler inputs.
type CompileRequest struct {
	WorkspaceRoot string
	Manifest      model.SourceManifest
	Targets       []model.TargetID
	Packages      []model.PackageID
	Mode          BuildMode
	NativeVerify  bool
}

// CompilationResult is the complete result of one compilation.
type CompilationResult struct {
	Plan                     model.BuildPlan
	Diagnostics              []model.Diagnostic
	Drift                    bool
	NativeVerificationFailed bool
}

// Compile runs one deterministic compilation transaction.
func Compile(request CompileRequest) CompilationResult {
	result := CompilationResult{}
	if request.Mode != BuildModeBuild && request.Mode != BuildModeCheck {
		result.Diagnostics = append(result.Diagnostics, errorDiagnostic("invalid-build-mode", fmt.Sprintf("unsupported build mode %q", request.Mode)))
		return result
	}
	if request.Mode == BuildModeBuild && request.NativeVerify {
		result.Diagnostics = append(result.Diagnostics, errorDiagnostic("invalid-native-verify", "native verification is valid only for check"))
		return result
	}
	if err := validateWorkspace(request.WorkspaceRoot); err != nil {
		result.Diagnostics = append(result.Diagnostics, errorDiagnostic("invalid-workspace", err.Error()))
		return result
	}
	if diagnostics := model.ValidateSourceManifest(request.Manifest); hasErrors(diagnostics) {
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		return result
	}

	selectedTargets, diagnostics := selectTargets(request.Manifest, request.Targets)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasErrors(result.Diagnostics) {
		return result
	}

	inventory, diagnostics := source.Import(request.Manifest, request.WorkspaceRoot)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasErrors(result.Diagnostics) {
		return result
	}
	inventory = selectPackages(inventory, request.Packages, &result.Diagnostics)
	if hasErrors(result.Diagnostics) {
		return result
	}

	for _, targetID := range selectedTargets {
		adapter, adapterDiagnostics := target.Resolve(targetID)
		result.Diagnostics = append(result.Diagnostics, adapterDiagnostics...)
		if hasErrors(adapterDiagnostics) {
			continue
		}
		policy := compositionPolicy(request.Manifest, targetID, target.Capabilities(adapter))
		packages, composeDiagnostics := composition.Compose(inventory, policy)
		result.Diagnostics = append(result.Diagnostics, composeDiagnostics...)
		if hasErrors(composeDiagnostics) {
			continue
		}
		targetPlan, renderDiagnostics := target.Render(adapter, packages)
		result.Diagnostics = append(result.Diagnostics, renderDiagnostics...)
		if hasErrors(renderDiagnostics) {
			continue
		}
		result.Plan.Targets = append(result.Plan.Targets, targetPlan)
	}
	if hasErrors(result.Diagnostics) {
		return result
	}
	sort.Slice(result.Plan.Targets, func(i, j int) bool { return result.Plan.Targets[i].Target < result.Plan.Targets[j].Target })
	provenancePlan, provenanceDiagnostics := artifact.Provenance(result.Plan, buildProvenance(request.Manifest, inventory, result.Plan))
	result.Diagnostics = append(result.Diagnostics, provenanceDiagnostics...)
	if hasErrors(provenanceDiagnostics) {
		return result
	}
	result.Plan = provenancePlan
	outputRoot := filepath.Join(request.WorkspaceRoot, filepath.FromSlash(string(request.Manifest.Output)))
	if request.Mode == BuildModeBuild {
		result.Diagnostics = append(result.Diagnostics, artifact.Write(result.Plan, outputRoot)...)
		return result
	}
	driftDiagnostics := artifact.Compare(result.Plan, outputRoot)
	result.Diagnostics = append(result.Diagnostics, driftDiagnostics...)
	result.Drift = len(driftDiagnostics) != 0
	if !result.Drift && request.NativeVerify {
		checks := nativeChecks(result.Plan)
		nativeDiagnostics := artifact.Verify(checks, outputRoot)
		result.Diagnostics = append(result.Diagnostics, nativeDiagnostics...)
		result.NativeVerificationFailed = len(nativeDiagnostics) != 0
	}
	return result
}

func validateWorkspace(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("workspace root must be a cleaned absolute path")
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("workspace root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root must be a directory")
	}
	return nil
}

func selectTargets(manifest model.SourceManifest, requested []model.TargetID) ([]model.TargetID, []model.Diagnostic) {
	allowed := make(map[model.TargetID]bool, len(manifest.Targets))
	for _, targetID := range manifest.Targets {
		allowed[targetID] = true
	}
	selected := append([]model.TargetID(nil), requested...)
	if len(selected) == 0 {
		selected = append(selected, manifest.Targets...)
	}
	seen := make(map[model.TargetID]bool, len(selected))
	var diagnostics []model.Diagnostic
	for _, targetID := range selected {
		if !allowed[targetID] {
			diagnostics = append(diagnostics, errorDiagnostic("invalid-target-selector", fmt.Sprintf("target %q is not declared by the manifest", targetID)))
		}
		if seen[targetID] {
			diagnostics = append(diagnostics, errorDiagnostic("duplicate-target-selector", fmt.Sprintf("target %q is selected more than once", targetID)))
		}
		seen[targetID] = true
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i] < selected[j] })
	return selected, diagnostics
}

func selectPackages(inventory model.SourceInventory, requested []model.PackageID, diagnostics *[]model.Diagnostic) model.SourceInventory {
	if len(requested) == 0 {
		return inventory
	}
	allowed := make(map[model.PackageID]bool, len(inventory.Packages))
	for _, pkg := range inventory.Packages {
		allowed[pkg.Identity] = true
	}
	seen := make(map[model.PackageID]bool, len(requested))
	selected := make(map[model.PackageID]bool, len(requested))
	for _, pkg := range requested {
		if !allowed[pkg] {
			*diagnostics = append(*diagnostics, errorDiagnostic("invalid-package-selector", fmt.Sprintf("package %q is not declared by the source", pkg)))
		}
		if seen[pkg] {
			*diagnostics = append(*diagnostics, errorDiagnostic("duplicate-package-selector", fmt.Sprintf("package %q is selected more than once", pkg)))
		}
		seen[pkg] = true
		selected[pkg] = true
	}
	filtered := inventory
	filtered.Packages = nil
	for _, pkg := range inventory.Packages {
		if selected[pkg.Identity] {
			filtered.Packages = append(filtered.Packages, pkg)
		}
	}
	return filtered
}

func compositionPolicy(manifest model.SourceManifest, targetID model.TargetID, capabilities []model.CapabilityRule) model.TargetComposition {
	for _, policy := range manifest.Composition {
		if policy.Target == targetID {
			return policy
		}
	}
	return model.TargetComposition{Target: targetID, Capabilities: capabilities}
}

func nativeChecks(plan model.BuildPlan) []model.NativeCheck {
	var checks []model.NativeCheck
	for _, targetPlan := range plan.Targets {
		checks = append(checks, targetPlan.NativeChecks...)
	}
	return checks
}

func buildProvenance(manifest model.SourceManifest, inventory model.SourceInventory, plan model.BuildPlan) artifact.ProvenanceInput {
	configuration, _ := json.Marshal(manifest)
	input := artifact.ProvenanceInput{CompilerVersion: "agentbundler-dev", Configuration: configuration}
	for _, file := range inventory.Inputs {
		input.Inputs = append(input.Inputs, artifact.ProvenanceInputFile{Path: file.Path, SHA256: file.SHA256})
	}
	for _, targetPlan := range plan.Targets {
		input.AdapterRevisions = append(input.AdapterRevisions, artifact.AdapterRevision{Target: targetPlan.Target, Revision: 1})
		for _, pkg := range inventory.Packages {
			for _, asset := range pkg.Assets {
				for _, overlay := range asset.Overlays {
					for _, ack := range overlay.Acknowledgments {
						if ack.Target == targetPlan.Target {
							input.Acknowledgments = append(input.Acknowledgments, artifact.ProvenanceAcknowledgment{Asset: string(ack.Asset), Target: ack.Target, Key: string(ack.Key), Reason: ack.Reason})
						}
					}
				}
			}
		}
	}
	return input
}

func hasErrors(diagnostics []model.Diagnostic) bool {
	for _, d := range diagnostics {
		if d.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
func errorDiagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}
