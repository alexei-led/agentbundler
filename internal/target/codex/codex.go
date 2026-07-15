// Package codex renders normalized packages as Codex plugins.
package codex

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/plugin"
)

const (
	Target         = model.TargetCodex
	FormatRevision = 4
)

// Adapter renders Codex plugins and preserves the project-profile agent layout.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }
func (adapter Adapter) Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	if packagesHaveProfile(input.Packages, model.TargetProfilePackage) {
		return packageoutput.RenderWithCodec(input, PackageCodec())
	}
	return renderProject(adapter.Target(), input.Packages)
}
func Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(input)
}

func renderProject(target model.TargetID, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	if len(packages) != 1 {
		return plugin.Render(target, ".codex-plugin/plugin.json", packages, nil)
	}
	pkg := packages[0]
	if diagnostics := model.ValidateNormalizedPackage(pkg); len(diagnostics) != 0 {
		return model.TargetPlan{Target: target}, diagnostics
	}
	manifest := map[string]any{"name": pkg.Identity, "skills": "./skills/"}
	if value, ok := pkg.Metadata["version"].(string); ok {
		manifest["version"] = value
	}
	if value, ok := pkg.Metadata["description"].(string); ok {
		manifest["description"] = value
	}

	agents := make([]model.NormalizedAsset, 0)
	base := pkg
	base.Assets = make([]model.NormalizedAsset, 0, len(pkg.Assets))
	for _, asset := range pkg.Assets {
		if asset.Kind == model.AssetKindAgent {
			agents = append(agents, asset)
		} else {
			base.Assets = append(base.Assets, asset)
		}
	}
	plan, diagnostics := plugin.Render(target, ".codex-plugin/plugin.json", []model.NormalizedPackage{base}, manifest)
	if len(diagnostics) != 0 {
		return plan, diagnostics
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Identity < agents[j].Identity })
	paths := make(map[model.RelativePath]struct{}, len(plan.Files)+len(agents))
	for _, file := range plan.Files {
		paths[file.Path] = struct{}{}
	}
	for _, asset := range agents {
		name := strings.TrimPrefix(string(asset.Identity), "agent/")
		if name == "" || strings.Contains(name, "/") {
			return model.TargetPlan{Target: target}, []model.Diagnostic{{Code: "invalid-asset-identity", Severity: model.SeverityError, Asset: asset.Identity, Message: fmt.Sprintf("asset identity %q cannot be rendered as a Codex project agent", asset.Identity)}}
		}
		data, extension, err := codexAgent(asset)
		if err != nil {
			return model.TargetPlan{Target: target}, []model.Diagnostic{{Code: "invalid-agent", Severity: model.SeverityError, Asset: asset.Identity, Message: err.Error()}}
		}
		path := model.RelativePath(".codex/agents/" + name + extension)
		if _, exists := paths[path]; exists {
			return model.TargetPlan{Target: target}, []model.Diagnostic{{Code: "duplicate-output-path", Severity: model.SeverityError, Asset: asset.Identity, Message: fmt.Sprintf("generated output path %q is duplicated", path)}}
		}
		paths[path] = struct{}{}
		plan.Files = append(plan.Files, model.PlannedFile{Path: path, Bytes: data})
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

func packagesHaveProfile(packages []model.NormalizedPackage, profile model.TargetProfile) bool {
	if len(packages) == 0 {
		return false
	}
	for _, pkg := range packages {
		if pkg.Profile != profile {
			return false
		}
	}
	return true
}
