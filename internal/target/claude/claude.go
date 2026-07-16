// Package claude renders normalized packages for Claude Code.
package claude

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
	"github.com/alexei-led/agentbundler/internal/target/skills"
)

const (
	Target         = model.TargetClaude
	FormatRevision = 5
)

// Adapter renders Claude project skills and installable plugins.
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
		plan, diagnostics := packageoutput.RenderWithCodec(input, PackageCodec())
		if len(diagnostics) != 0 {
			return plan, diagnostics
		}
		plan.NativeChecks = nativeChecks(plan.Packages, input.Distribution != nil)
		return plan, nil
	}
	return skills.Render(adapter.Target(), ".claude/skills", input.Packages)
}
func Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(input)
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

func nativeChecks(packages []model.PackageID, catalog bool) []model.NativeCheck {
	if catalog {
		return []model.NativeCheck{{
			Program:   "claude",
			Arguments: []string{"plugin", "validate", "--strict", "."},
			Location:  model.SourceLocation{Path: "internal/target/claude/codec.go"},
		}}
	}
	checks := make([]model.NativeCheck, 0, len(packages))
	for _, identity := range packages {
		root := "."
		if len(packages) > 1 {
			root = string(identity)
		}
		checks = append(checks, model.NativeCheck{
			Program:   "claude",
			Arguments: []string{"plugin", "validate", "--strict", root},
			Location:  model.SourceLocation{Path: "internal/target/claude/codec.go"},
		})
	}
	return checks
}
