// Package antigravity renders normalized packages as Antigravity CLI plugins.
package antigravity

import (
	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

const (
	Target         = model.TargetAntigravity
	FormatRevision = 1
)

// Adapter renders installable Antigravity plugins.
type Adapter struct{}

func New() Adapter                     { return Adapter{} }
func (Adapter) Target() model.TargetID { return Target }
func (Adapter) FormatRevision() int    { return FormatRevision }
func Capabilities() []model.CapabilityRule {
	return append([]model.CapabilityRule(nil), capabilityRules...)
}
func (Adapter) Capabilities() []model.CapabilityRule { return Capabilities() }

func (Adapter) Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	plan, diagnostics := packageoutput.RenderWithCodec(input, PackageCodec())
	if len(diagnostics) != 0 {
		return plan, diagnostics
	}
	plan.NativeChecks = nativeChecks(plan.Packages)
	return plan, nil
}

func Render(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	return New().Render(input)
}

func nativeChecks(packages []model.PackageID) []model.NativeCheck {
	checks := make([]model.NativeCheck, 0, len(packages))
	for _, identity := range packages {
		var workingDirectory *model.RelativePath
		if len(packages) > 1 {
			root := model.RelativePath(identity)
			workingDirectory = &root
		}
		checks = append(checks, model.NativeCheck{
			Program:          "agy",
			Arguments:        []string{"plugin", "validate", "."},
			WorkingDirectory: workingDirectory,
			Location:         model.SourceLocation{Path: "internal/target/antigravity/antigravity.go"},
		})
	}
	return checks
}
