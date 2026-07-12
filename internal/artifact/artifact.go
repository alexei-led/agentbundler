// Package artifact validates and applies complete generated-output plans.
package artifact

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/artifact/compare"
	"github.com/alexei-led/agentbundler/internal/artifact/nativeverify"
	"github.com/alexei-led/agentbundler/internal/artifact/provenance"
	"github.com/alexei-led/agentbundler/internal/artifact/write"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	diagnosticInvalidPlan  = "invalid-model"
	diagnosticExecutable   = "ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED"
	diagnosticProvenance   = "PROVENANCE_INVALID"
	diagnosticDriftMissing = "DRIFT_MISSING"
	diagnosticDriftChanged = "DRIFT_CHANGED"
	diagnosticDriftExtra   = "DRIFT_EXTRA"
)

// ProvenanceInputFile identifies an input included in build provenance.
type ProvenanceInputFile struct {
	Path   model.RelativePath
	SHA256 string
}

// ProvenanceAcknowledgment records accepted advisory capability behavior.
type ProvenanceAcknowledgment struct {
	Asset  string
	Target model.TargetID
	Key    string
	Reason string
}

// AdapterRevision identifies an output format revision for a target.
type AdapterRevision struct {
	Target   model.TargetID
	Revision int
}

// ProvenanceInput contains the evidence recorded in compiler provenance.
type ProvenanceInput struct {
	CompilerVersion  string
	Configuration    []byte
	Inputs           []ProvenanceInputFile
	Acknowledgments  []ProvenanceAcknowledgment
	AdapterRevisions []AdapterRevision
}

// Write validates plan and atomically replaces outputRoot with its generated output.
func Write(plan model.BuildPlan, outputRoot string) []model.Diagnostic {
	if diagnostics := validatePlan(plan); len(diagnostics) != 0 {
		return diagnostics
	}
	return write.ReplaceOutput(plan, outputRoot)
}

// Compare validates plan and reports exact generated-output drift below outputRoot.
func Compare(plan model.BuildPlan, outputRoot string) []model.Diagnostic {
	if diagnostics := validatePlan(plan); len(diagnostics) != 0 {
		return diagnostics
	}

	drift := compare.DetectDrift(plan, outputRoot)
	diagnostics := make([]model.Diagnostic, len(drift))
	for index, entry := range drift {
		diagnostics[index] = model.Diagnostic{
			Code:     driftDiagnosticCode(entry.Kind),
			Severity: model.SeverityError,
			Message:  fmt.Sprintf("generated output %s: %s", entry.Kind, entry.Path),
		}
	}
	return diagnostics
}

// Provenance validates plan and appends deterministic compiler-owned provenance.
func Provenance(plan model.BuildPlan, input ProvenanceInput) (model.BuildPlan, []model.Diagnostic) {
	if diagnostics := validatePlan(plan); len(diagnostics) != 0 {
		return plan, diagnostics
	}

	result, err := provenance.Append(plan, provenanceInput(input))
	if err != nil {
		return plan, []model.Diagnostic{{
			Code:     diagnosticProvenance,
			Severity: model.SeverityError,
			Message:  err.Error(),
		}}
	}
	return result, nil
}

// Verify runs declared native checks against outputRoot after a current comparison.
func Verify(checks []model.NativeCheck, outputRoot string) []model.Diagnostic {
	return nativeverify.RunNativeChecks(checks, outputRoot).Diagnostics
}

func validatePlan(plan model.BuildPlan) []model.Diagnostic {
	diagnostics := model.ValidateBuildPlan(plan)
	if len(diagnostics) != 0 {
		return diagnostics
	}

	var validation []model.Diagnostic
	for _, destination := range planDestinations(plan) {
		if reservedPath(destination) {
			validation = append(validation, invalidPlanDiagnostic(
				fmt.Sprintf("planned destination %q contains a reserved platform name", destination),
			))
		}
	}
	validation = append(validation, destinationConflicts(planDestinations(plan))...)
	if runtime.GOOS == "windows" && hasExecutableFile(plan) {
		validation = append(validation, model.Diagnostic{
			Code:     diagnosticExecutable,
			Severity: model.SeverityError,
			Message:  "executable file intent is unsupported on Windows",
		})
	}
	sort.SliceStable(validation, func(left, right int) bool {
		return validation[left].Message < validation[right].Message
	})
	return validation
}

func planDestinations(plan model.BuildPlan) []string {
	destinations := make([]string, 0, len(plan.CompilerFiles))
	for _, target := range plan.Targets {
		for _, file := range target.Files {
			destinations = append(destinations, string(target.Target)+"/"+string(file.Path))
		}
	}
	for _, file := range plan.CompilerFiles {
		destinations = append(destinations, string(file.Path))
	}
	return destinations
}

func destinationConflicts(destinations []string) []model.Diagnostic {
	sorted := append([]string(nil), destinations...)
	sort.Strings(sorted)

	caseFolded := make(map[string]string, len(sorted))
	var diagnostics []model.Diagnostic
	for _, destination := range sorted {
		folded := strings.ToLower(destination)
		if previous, exists := caseFolded[folded]; exists && previous != destination {
			diagnostics = append(diagnostics, invalidPlanDiagnostic(
				fmt.Sprintf("planned destinations %q and %q collide after case folding", previous, destination),
			))
			continue
		}
		caseFolded[folded] = destination
	}
	for index := 1; index < len(sorted); index++ {
		if strings.HasPrefix(sorted[index], sorted[index-1]+"/") {
			diagnostics = append(diagnostics, invalidPlanDiagnostic(
				fmt.Sprintf("planned destination %q prevents owning target path %q", sorted[index-1], sorted[index]),
			))
		}
	}
	return diagnostics
}

func reservedPath(destination string) bool {
	for _, component := range strings.Split(destination, "/") {
		name := strings.TrimRight(strings.ToUpper(component), ". ")
		base, _, _ := strings.Cut(name, ".")
		switch base {
		case "CON", "PRN", "AUX", "NUL", "CLOCK$":
			return true
		}
		if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return true
		}
	}
	return false
}

func hasExecutableFile(plan model.BuildPlan) bool {
	for _, target := range plan.Targets {
		for _, file := range target.Files {
			if file.Executable {
				return true
			}
		}
	}
	for _, file := range plan.CompilerFiles {
		if file.Executable {
			return true
		}
	}
	return false
}

func invalidPlanDiagnostic(message string) model.Diagnostic {
	return model.Diagnostic{
		Code:     diagnosticInvalidPlan,
		Severity: model.SeverityError,
		Message:  message,
	}
}

func driftDiagnosticCode(kind compare.DriftKind) string {
	switch kind {
	case compare.DriftMissing:
		return diagnosticDriftMissing
	case compare.DriftChanged:
		return diagnosticDriftChanged
	case compare.DriftExtra:
		return diagnosticDriftExtra
	default:
		return diagnosticDriftChanged
	}
}

func provenanceInput(input ProvenanceInput) provenance.Input {
	result := provenance.Input{
		CompilerVersion:  input.CompilerVersion,
		Configuration:    json.RawMessage(input.Configuration),
		Inputs:           make([]provenance.InputFile, len(input.Inputs)),
		Acknowledgments:  make([]provenance.Acknowledgment, len(input.Acknowledgments)),
		AdapterRevisions: make([]provenance.AdapterRevision, len(input.AdapterRevisions)),
	}
	for index, file := range input.Inputs {
		result.Inputs[index] = provenance.InputFile{Path: file.Path, SHA256: file.SHA256}
	}
	for index, acknowledgment := range input.Acknowledgments {
		result.Acknowledgments[index] = provenance.Acknowledgment{
			Asset:  acknowledgment.Asset,
			Target: acknowledgment.Target,
			Key:    acknowledgment.Key,
			Reason: acknowledgment.Reason,
		}
	}
	for index, revision := range input.AdapterRevisions {
		result.AdapterRevisions[index] = provenance.AdapterRevision{
			Target:   revision.Target,
			Revision: revision.Revision,
		}
	}
	return result
}
