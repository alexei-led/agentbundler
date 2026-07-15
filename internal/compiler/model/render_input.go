package model

import (
	"fmt"
	"sort"
	"strings"
)

// SortTargetRenderInput orders packages by stable package identity.
func SortTargetRenderInput(input *TargetRenderInput) {
	sort.SliceStable(input.Packages, func(left, right int) bool {
		return input.Packages[left].Identity < input.Packages[right].Identity
	})
}

// ValidateTargetRenderInput validates one complete target adapter request.
func ValidateTargetRenderInput(input TargetRenderInput) []Diagnostic {
	var diagnostics []Diagnostic
	if len(input.Packages) == 0 {
		diagnostics = appendInvalid(diagnostics, "target render input packages must not be empty")
	}
	if err := validateJSONValue(input.Distribution); err != nil {
		diagnostics = appendInvalid(diagnostics, "target render input distribution: "+err.Error())
	}

	packages := make(map[PackageID]struct{}, len(input.Packages))
	var target TargetID
	for index, pkg := range input.Packages {
		diagnostics = append(diagnostics, ValidateNormalizedPackage(pkg)...)
		if _, exists := packages[pkg.Identity]; exists {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target render input package %q is duplicated", pkg.Identity))
		}
		packages[pkg.Identity] = struct{}{}
		if index > 0 && input.Packages[index-1].Identity > pkg.Identity {
			diagnostics = appendInvalid(diagnostics, "target render input packages must be ordered by identity")
		}
		if target == "" {
			target = pkg.Target
		} else if pkg.Target != target {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target render input package %q targets %q, not %q", pkg.Identity, pkg.Target, target))
		}
	}

	switch input.PackageMode {
	case TargetPackageModeSeparate:
		if input.Aggregate != nil {
			diagnostics = appendInvalid(diagnostics, "separate target render input forbids aggregate configuration")
		}
	case TargetPackageModeAggregate:
		if target != TargetPi {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target %q does not support aggregate package mode", target))
		}
		for _, pkg := range input.Packages {
			if pkg.Profile != TargetProfilePackage {
				diagnostics = appendInvalid(diagnostics, fmt.Sprintf("aggregate target render input package %q requires package profile", pkg.Identity))
			}
		}
		if input.Aggregate == nil {
			diagnostics = appendInvalid(diagnostics, "aggregate target render input requires aggregate configuration")
		} else {
			diagnostics = append(diagnostics, validateAggregatePackage(*input.Aggregate)...)
			diagnostics = append(diagnostics, validateAggregateDependencies(input)...)
		}
	default:
		diagnostics = appendInvalid(diagnostics, fmt.Sprintf("target render input package mode %q is invalid", input.PackageMode))
		if input.Aggregate != nil {
			diagnostics = append(diagnostics, validateAggregatePackage(*input.Aggregate)...)
		}
	}
	return diagnostics
}

func validateAggregateDependencies(input TargetRenderInput) []Diagnostic {
	versions := make(map[string]string)
	owners := make(map[string]string)
	var diagnostics []Diagnostic
	if input.Aggregate != nil {
		diagnostics = append(diagnostics, mergeAggregateDependencies(versions, owners, "aggregate package", input.Aggregate.Metadata)...)
	}
	for _, pkg := range input.Packages {
		diagnostics = append(diagnostics, mergeAggregateDependencies(versions, owners, fmt.Sprintf("package %q", pkg.Identity), pkg.Metadata)...)
	}
	return diagnostics
}

func mergeAggregateDependencies(versions, owners map[string]string, owner string, metadata PackageMetadata) []Diagnostic {
	value, exists := metadata["dependencies"]
	if !exists {
		return nil
	}
	dependencies, ok := value.(map[string]any)
	if !ok {
		return appendInvalid(nil, owner+" dependencies must be an object")
	}

	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	var diagnostics []Diagnostic
	for _, name := range names {
		version, ok := dependencies[name].(string)
		if strings.TrimSpace(name) == "" || strings.ContainsRune(name, '\x00') {
			diagnostics = appendInvalid(diagnostics, owner+" dependency name must not be empty or contain NUL")
			continue
		}
		if !ok || strings.TrimSpace(version) == "" || strings.ContainsRune(version, '\x00') {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("%s dependency %q must have a non-empty string version", owner, name))
			continue
		}
		previous, found := versions[name]
		if found && previous != version {
			diagnostics = appendInvalid(diagnostics, fmt.Sprintf("aggregate dependency %q conflicts between %s (%q) and %s (%q)", name, owners[name], previous, owner, version))
			continue
		}
		if !found {
			versions[name] = version
			owners[name] = owner
		}
	}
	return diagnostics
}
