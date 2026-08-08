// Package artifact validates and applies complete generated-output plans.
package artifact

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/artifact/archive"
	"github.com/alexei-led/agentbundler/internal/artifact/compare"
	"github.com/alexei-led/agentbundler/internal/artifact/nativeverify"
	"github.com/alexei-led/agentbundler/internal/artifact/provenance"
	"github.com/alexei-led/agentbundler/internal/artifact/write"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	diagnosticInvalidPlan      = "invalid-model"
	diagnosticLayoutGuard      = "invalid-workspace-layout"
	diagnosticArchive          = "archive-write-failed"
	diagnosticExecutable       = "ARTIFACT_EXECUTABLE_INTENT_UNSUPPORTED"
	diagnosticProvenance       = "PROVENANCE_INVALID"
	diagnosticDriftMissing     = "DRIFT_MISSING"
	diagnosticDriftChanged     = "DRIFT_CHANGED"
	diagnosticDriftExtra       = "DRIFT_EXTRA"
	diagnosticDriftObservation = "DRIFT_OBSERVATION_FAILED"
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

// Write validates the layout guard and plan, then atomically replaces outputRoot
// with its generated output. The guard must have been constructed with
// NewWorkspaceLayoutGuard before source ingestion.
func Write(guard WorkspaceLayoutGuard, plan model.BuildPlan, outputRoot string) []model.Diagnostic {
	if err := guard.Revalidate(); err != nil {
		return []model.Diagnostic{layoutGuardDiagnostic(err.Error())}
	}
	if diagnostics := validatePlan(plan); len(diagnostics) != 0 {
		return diagnostics
	}
	if drift, err := compare.DetectDrift(plan, outputRoot); err == nil && len(drift) == 0 {
		return nil
	}
	return write.ReplaceOutput(plan, outputRoot)
}

// Archive validates the layout guard and plan, then writes deterministic archives
// from the plan's own bytes and ArchiveUnits. No filesystem traversal occurs;
// all archive content comes from TargetPlan.Files filtered by ArchiveUnits.
// The guard must have been constructed with NewWorkspaceLayoutGuard before
// source ingestion.
func Archive(guard WorkspaceLayoutGuard, distribution model.DistributionMetadata, plan model.BuildPlan, output string) ([]string, []model.Diagnostic) {
	if err := guard.Revalidate(); err != nil {
		return nil, []model.Diagnostic{layoutGuardDiagnostic(err.Error())}
	}
	if diagnostics := validatePlan(plan); len(diagnostics) != 0 {
		return nil, diagnostics
	}
	paths, err := archive.WriteTargetRoots(distribution, plan, output)
	if err != nil {
		return nil, []model.Diagnostic{{Code: diagnosticArchive, Severity: model.SeverityError, Message: err.Error()}}
	}
	return paths, nil
}

// Compare validates the layout guard and plan, then reports exact generated-output
// drift below outputRoot. The returned bool distinguishes observed drift from an
// output-observation failure. The guard must have been constructed with
// NewWorkspaceLayoutGuard before source ingestion.
func Compare(guard WorkspaceLayoutGuard, plan model.BuildPlan, outputRoot string) ([]model.Diagnostic, bool) {
	if err := guard.Revalidate(); err != nil {
		return []model.Diagnostic{layoutGuardDiagnostic(err.Error())}, false
	}
	if diagnostics := validatePlan(plan); len(diagnostics) != 0 {
		return diagnostics, false
	}

	drift, err := compare.DetectDrift(plan, outputRoot)
	if err != nil {
		return []model.Diagnostic{{Code: diagnosticDriftObservation, Severity: model.SeverityError, Message: fmt.Sprintf("inspect generated output: %v", err)}}, false
	}
	diagnostics := make([]model.Diagnostic, len(drift))
	for index, entry := range drift {
		diagnostics[index] = model.Diagnostic{
			Code:     driftDiagnosticCode(entry.Kind),
			Severity: model.SeverityError,
			Message:  fmt.Sprintf("generated output %s: %s", entry.Kind, entry.Path),
		}
	}
	return diagnostics, len(drift) != 0
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
func Verify(checks []model.NativeCheck, outputRoot string) nativeverify.Result {
	if diagnostics := model.ValidateNativeChecks(checks); len(diagnostics) != 0 {
		return nativeverify.Result{Diagnostics: diagnostics}
	}
	return nativeverify.RunNativeChecks(checks, outputRoot)
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
	type foldedDestination struct {
		raw, folded string
	}
	foldedSorted := make([]foldedDestination, len(sorted))
	for index, destination := range sorted {
		foldedSorted[index] = foldedDestination{raw: destination, folded: strings.ToLower(destination)}
	}
	sort.SliceStable(foldedSorted, func(left, right int) bool {
		if foldedSorted[left].folded == foldedSorted[right].folded {
			return foldedSorted[left].raw < foldedSorted[right].raw
		}
		return foldedSorted[left].folded < foldedSorted[right].folded
	})
	for index := 1; index < len(foldedSorted); index++ {
		if strings.HasPrefix(foldedSorted[index].folded, foldedSorted[index-1].folded+"/") {
			diagnostics = append(diagnostics, invalidPlanDiagnostic(
				fmt.Sprintf("planned destination %q prevents owning target path %q", foldedSorted[index-1].raw, foldedSorted[index].raw),
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

func layoutGuardDiagnostic(message string) model.Diagnostic {
	return model.Diagnostic{
		Code:     diagnosticLayoutGuard,
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
