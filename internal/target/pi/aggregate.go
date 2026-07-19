package pi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

const (
	hookDescriptorPath = model.RelativePath("hooks/hooks.v1.json")
	hookAdapterPath    = model.RelativePath("extensions/agentbundler-hooks.ts")
)

var hookAdapter = []byte(`import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createPiHookRuntime, type PiExtensionAPI } from "./_agentbundler-hooks/index.js";

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const config = JSON.parse(readFileSync(resolve(packageRoot, "hooks/hooks.v1.json"), "utf8")) as unknown;

export default function agentBundlerHooks(pi: PiExtensionAPI): void {
  createPiHookRuntime(pi, config, { packageRoot });
}
`)

func renderAggregate(input model.TargetRenderInput) (model.TargetPlan, []model.Diagnostic) {
	input.Packages = append([]model.NormalizedPackage(nil), input.Packages...)
	model.SortTargetRenderInput(&input)
	if diagnostics := model.ValidateTargetRenderInput(input); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}
	if diagnostics := validateAggregateAssetNames(input.Packages); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}
	if diagnostics := validateAggregateOutputPaths(input.Packages); len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}

	pkg, err := mergeAggregatePackage(input)
	if err != nil {
		return emptyPlan(), []model.Diagnostic{piDiagnostic("invalid-aggregate-package", err.Error())}
	}
	plan, diagnostics := packageoutput.RenderWithCodec(model.TargetRenderInput{
		Packages:    []model.NormalizedPackage{pkg},
		PackageMode: model.TargetPackageModeSeparate,
	}, aggregatePackageCodec())
	if len(diagnostics) != 0 {
		return emptyPlan(), diagnostics
	}

	paths := make(map[model.RelativePath]struct{}, len(plan.Files))
	for _, file := range plan.Files {
		paths[file.Path] = struct{}{}
	}
	if _, exists := paths[hookDescriptorPath]; !exists {
		data, err := json.Marshal(hookConfigV1{Version: 1, Hooks: []model.HookDescriptor{}})
		if err != nil {
			return emptyPlan(), []model.Diagnostic{piDiagnostic("invalid-hook-manifest", err.Error())}
		}
		if err := addAggregateFile(&plan, paths, hookDescriptorPath, append(data, '\n')); err != nil {
			return emptyPlan(), []model.Diagnostic{piDiagnostic("invalid-package-output", err.Error())}
		}
	}
	if err := addAggregateFile(&plan, paths, hookAdapterPath, hookAdapter); err != nil {
		return emptyPlan(), []model.Diagnostic{piDiagnostic("invalid-package-output", err.Error())}
	}
	runtime, err := runtimeFiles()
	if err != nil {
		return emptyPlan(), []model.Diagnostic{piDiagnostic("invalid-embedded-runtime", err.Error())}
	}
	for _, file := range runtime {
		path := model.RelativePath(embeddedRuntimeRoot + "/" + file.name)
		if err := addAggregateRuntimeFile(&plan, paths, path, file); err != nil {
			return emptyPlan(), []model.Diagnostic{piDiagnostic("invalid-package-output", err.Error())}
		}
	}
	sort.Slice(plan.Files, func(left, right int) bool { return plan.Files[left].Path < plan.Files[right].Path })
	return plan, nil
}

func addAggregateRuntimeFile(plan *model.TargetPlan, paths map[model.RelativePath]struct{}, path model.RelativePath, file runtimeFile) error {
	if _, exists := paths[path]; exists {
		return fmt.Errorf("aggregate file %q collides with generated output", path)
	}
	paths[path] = struct{}{}
	plan.Files = append(plan.Files, model.PlannedFile{Path: path, Bytes: append([]byte(nil), file.bytes...), Executable: file.executable})
	return nil
}

func mergeAggregatePackage(input model.TargetRenderInput) (model.NormalizedPackage, error) {
	metadata := make(model.PackageMetadata, len(input.Aggregate.Metadata))
	for key, value := range input.Aggregate.Metadata {
		if key != "dependencies" {
			metadata[key] = value
		}
	}
	mergedDependencies := make(map[string]any)
	merge := func(owner string, source model.PackageMetadata) error {
		value, exists := source["dependencies"]
		if !exists {
			return nil
		}
		values, err := dependencies(value)
		if err != nil {
			return fmt.Errorf("%s dependencies: %w", owner, err)
		}
		for name, version := range values {
			if previous, exists := mergedDependencies[name]; exists && previous != version {
				return fmt.Errorf("dependency %q conflicts in aggregate package", name)
			}
			mergedDependencies[name] = version
		}
		return nil
	}
	if err := merge("aggregate package", input.Aggregate.Metadata); err != nil {
		return model.NormalizedPackage{}, err
	}

	assets := make([]model.NormalizedAsset, 0)
	acknowledgments := make([]model.Acknowledgment, 0)
	for _, pkg := range input.Packages {
		if err := merge(fmt.Sprintf("package %q", pkg.Identity), pkg.Metadata); err != nil {
			return model.NormalizedPackage{}, err
		}
		assets = append(assets, pkg.Assets...)
		acknowledgments = append(acknowledgments, pkg.Acknowledgments...)
	}
	if len(mergedDependencies) != 0 {
		metadata["dependencies"] = mergedDependencies
	}
	return model.NormalizedPackage{
		Identity:        input.Aggregate.Identity,
		Metadata:        metadata,
		Target:          Target,
		Profile:         model.TargetProfilePackage,
		Assets:          assets,
		Acknowledgments: acknowledgments,
	}, nil
}

type aggregateAssetOwner struct {
	packageID model.PackageID
	asset     model.NormalizedAsset
}

func validateAggregateAssetNames(packages []model.NormalizedPackage) []model.Diagnostic {
	owners := make(map[model.AssetID]aggregateAssetOwner)
	for _, pkg := range packages {
		assets := append([]model.NormalizedAsset(nil), pkg.Assets...)
		sort.Slice(assets, func(left, right int) bool { return assets[left].Identity < assets[right].Identity })
		for _, asset := range assets {
			if previous, exists := owners[asset.Identity]; exists {
				return []model.Diagnostic{piDiagnostic("aggregate-asset-conflict", fmt.Sprintf(
					"aggregate asset name %q conflicts between %s and %s",
					asset.Identity, formatAggregateAssetOwner(previous), formatAggregateAssetOwner(aggregateAssetOwner{packageID: pkg.Identity, asset: asset}),
				))}
			}
			owners[asset.Identity] = aggregateAssetOwner{packageID: pkg.Identity, asset: asset}
		}
	}
	return nil
}

func formatAggregateAssetOwner(owner aggregateAssetOwner) string {
	value := fmt.Sprintf("package %q asset %q", owner.packageID, owner.asset.Identity)
	if location, ok := aggregateAssetLocation(owner.asset); ok {
		value += " (source " + formatPiLocation(location) + ")"
	}
	return value
}

type aggregatePathOwner struct {
	description string
	locations   []model.SourceLocation
}

func validateAggregateOutputPaths(packages []model.NormalizedPackage) []model.Diagnostic {
	paths := make(map[model.RelativePath]aggregatePathOwner)
	add := func(path model.RelativePath, owner aggregatePathOwner) []model.Diagnostic {
		if previous, exists := paths[path]; exists {
			return []model.Diagnostic{piDiagnostic("aggregate-path-conflict", fmt.Sprintf(
				"aggregate output path %q conflicts between %s%s and %s%s",
				path, previous.description, formatPiOrigins(previous.locations), owner.description, formatPiOrigins(owner.locations),
			))}
		}
		paths[path] = owner
		return nil
	}
	for _, pkg := range packages {
		assets := append([]model.NormalizedAsset(nil), pkg.Assets...)
		sort.Slice(assets, func(left, right int) bool { return assets[left].Identity < assets[right].Identity })
		for _, asset := range assets {
			name := strings.TrimPrefix(string(asset.Identity), string(asset.Kind)+"/")
			location, hasLocation := aggregateAssetLocation(asset)
			assetOrigins := []model.SourceLocation(nil)
			if hasLocation {
				assetOrigins = []model.SourceLocation{location}
			}
			description := fmt.Sprintf("package %q asset %q", pkg.Identity, asset.Identity)
			switch asset.Kind {
			case model.AssetKindSkill:
				if diagnostics := add(model.RelativePath("skills/"+name+"/SKILL.md"), aggregatePathOwner{description: description, locations: assetOrigins}); len(diagnostics) != 0 {
					return diagnostics
				}
			case model.AssetKindAgent:
				if diagnostics := add(model.RelativePath("agents/"+name+".md"), aggregatePathOwner{description: description, locations: assetOrigins}); len(diagnostics) != 0 {
					return diagnostics
				}
			case model.AssetKindNativeResource:
				for _, path := range sortedNativeResourcePaths(asset.Content.Files) {
					content := asset.Content.Files[path]
					if diagnostics := add(path, aggregatePathOwner{description: fmt.Sprintf("%s payload %q", description, path), locations: content.Origin}); len(diagnostics) != 0 {
						return diagnostics
					}
				}
			}
			var root string
			switch asset.Kind {
			case model.AssetKindSkill:
				root = "skills/" + name
			case model.AssetKindResource:
				root = "resources/" + name
			case model.AssetKindHook:
				root = "hooks/payloads/" + name
			}
			if root == "" {
				continue
			}
			filePaths := make([]model.RelativePath, 0, len(asset.Content.Files))
			for path := range asset.Content.Files {
				filePaths = append(filePaths, path)
			}
			sort.Slice(filePaths, func(left, right int) bool { return filePaths[left] < filePaths[right] })
			for _, filePath := range filePaths {
				origins := asset.Content.Files[filePath].Origin
				if len(origins) == 0 {
					origins = assetOrigins
				}
				owner := aggregatePathOwner{description: fmt.Sprintf("%s payload %q", description, filePath), locations: origins}
				if diagnostics := add(model.RelativePath(root+"/"+string(filePath)), owner); len(diagnostics) != 0 {
					return diagnostics
				}
			}
		}
	}
	return nil
}

func aggregateAssetLocation(asset model.NormalizedAsset) (model.SourceLocation, bool) {
	if asset.Hook != nil {
		return asset.Hook.Location, true
	}
	if len(asset.CapabilityUses) != 0 {
		return asset.CapabilityUses[0].Location, true
	}
	return model.SourceLocation{}, false
}

func formatPiOrigins(locations []model.SourceLocation) string {
	if len(locations) == 0 {
		return ""
	}
	values := make([]string, len(locations))
	for index, location := range locations {
		values[index] = formatPiLocation(location)
	}
	return " (source " + strings.Join(values, ", ") + ")"
}

func formatPiLocation(location model.SourceLocation) string {
	value := string(location.Path)
	if location.Line != nil {
		value += fmt.Sprintf(":%d", *location.Line)
		if location.Column != nil {
			value += fmt.Sprintf(":%d", *location.Column)
		}
	}
	return value
}

func addAggregateFile(plan *model.TargetPlan, paths map[model.RelativePath]struct{}, path model.RelativePath, data []byte) error {
	if _, err := model.NewRelativePath(string(path)); err != nil {
		return fmt.Errorf("generated output path: %w", err)
	}
	if _, exists := paths[path]; exists {
		return fmt.Errorf("generated output path %q is duplicated by Pi runtime output", path)
	}
	paths[path] = struct{}{}
	plan.Files = append(plan.Files, model.PlannedFile{Path: path, Bytes: append([]byte(nil), data...)})
	return nil
}

func emptyPlan() model.TargetPlan {
	return model.TargetPlan{Target: Target, Packages: []model.PackageID{}, Files: []model.PlannedFile{}, NativeChecks: []model.NativeCheck{}}
}

func piDiagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}
