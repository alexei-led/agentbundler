package target

import (
	"encoding/json"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestPackageTargetsRenderCatalogGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		target model.TargetID
		path   model.RelativePath
	}{
		{target: model.TargetClaude, path: ".claude-plugin/marketplace.json"},
		{target: model.TargetCodex, path: ".agents/plugins/marketplace.json"},
		{target: model.TargetCopilot, path: ".github/plugin/marketplace.json"},
		{target: model.TargetCursor, path: ".cursor-plugin/marketplace.json"},
		{target: model.TargetGrok, path: ".claude-plugin/marketplace.json"},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.target), func(t *testing.T) {
			t.Parallel()
			plan, diagnostics := renderCatalogTarget(test.target, []model.NormalizedPackage{catalogPackage(test.target, "alpha")})
			if len(diagnostics) != 0 {
				t.Fatalf("Render() diagnostics = %#v", diagnostics)
			}
			file, exists := plannedFile(plan, test.path)
			if !exists {
				t.Fatalf("catalog %q is absent; paths = %#v", test.path, plannedPaths(plan))
			}
			want, err := os.ReadFile("testdata/catalog/" + string(test.target) + ".json")
			if err != nil {
				t.Fatal(err)
			}
			if string(file.Bytes) != string(want) {
				t.Fatalf("catalog differs:\ngot:  %s\nwant: %s", file.Bytes, want)
			}
			if file.Executable || len(file.Origin) != 0 {
				t.Fatalf("catalog file metadata = %#v", file)
			}
			wantSource := "./"
			if test.target == model.TargetClaude || test.target == model.TargetGrok {
				wantSource = ".."
			}
			assertCatalogSources(t, test.target, file.Bytes, []string{"alpha"}, []string{wantSource})
			assertCatalogRequiredFields(t, test.target, file.Bytes)
		})
	}
}

func TestPackageTargetCatalogsOrderMultiPackageRootsAndAreReproducible(t *testing.T) {
	t.Parallel()

	for _, targetID := range []model.TargetID{model.TargetClaude, model.TargetCodex, model.TargetCopilot, model.TargetCursor, model.TargetGrok} {
		targetID := targetID
		t.Run(string(targetID), func(t *testing.T) {
			t.Parallel()
			zeta := catalogPackage(targetID, "zeta")
			alpha := catalogPackage(targetID, "alpha")
			first, diagnostics := renderCatalogTarget(targetID, []model.NormalizedPackage{zeta, alpha})
			if len(diagnostics) != 0 {
				t.Fatalf("first Render() diagnostics = %#v", diagnostics)
			}
			second, diagnostics := renderCatalogTarget(targetID, []model.NormalizedPackage{alpha, zeta})
			if len(diagnostics) != 0 {
				t.Fatalf("second Render() diagnostics = %#v", diagnostics)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("catalog plan depends on package input order")
			}
			catalog, exists := catalogFile(first)
			if !exists {
				t.Fatalf("catalog is absent; paths = %#v", plannedPaths(first))
			}
			wantSources := []string{"./alpha", "./zeta"}
			if targetID == model.TargetClaude || targetID == model.TargetGrok {
				wantSources = []string{"../alpha", "../zeta"}
			}
			assertCatalogSources(t, targetID, catalog.Bytes, []string{"alpha", "zeta"}, wantSources)
			if targetID == model.TargetClaude {
				want := []model.NativeCheck{{Program: "claude", Arguments: []string{"plugin", "validate", "--strict", "."}, Location: model.SourceLocation{Path: "internal/target/claude/codec.go"}}}
				if !reflect.DeepEqual(first.NativeChecks, want) {
					t.Fatalf("Claude catalog NativeChecks = %#v, want %#v", first.NativeChecks, want)
				}
			}
			for _, root := range []string{"alpha/", "zeta/"} {
				found := false
				for _, file := range first.Files {
					if strings.HasPrefix(string(file.Path), root) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("no package files below %q; paths = %#v", root, plannedPaths(first))
				}
			}
		})
	}
}

func TestClaudeAndGrokCatalogSourcesResolveFromCatalogDirectory(t *testing.T) {
	t.Parallel()

	for _, targetID := range []model.TargetID{model.TargetClaude, model.TargetGrok} {
		targetID := targetID
		for _, test := range []struct {
			name     string
			packages []model.PackageID
		}{
			{name: "flat", packages: []model.PackageID{"alpha"}},
			{name: "separate", packages: []model.PackageID{"alpha", "zeta"}},
		} {
			test := test
			t.Run(string(targetID)+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				packages := make([]model.NormalizedPackage, 0, len(test.packages))
				for _, packageID := range test.packages {
					packages = append(packages, catalogPackage(targetID, packageID))
				}
				plan, diagnostics := renderCatalogTarget(targetID, packages)
				if len(diagnostics) != 0 {
					t.Fatalf("Render() diagnostics = %#v", diagnostics)
				}
				catalog, exists := catalogFile(plan)
				if !exists {
					t.Fatalf("catalog is absent; paths = %#v", plannedPaths(plan))
				}
				var document struct {
					Plugins []struct {
						Name   string `json:"name"`
						Source string `json:"source"`
					} `json:"plugins"`
				}
				if err := json.Unmarshal(catalog.Bytes, &document); err != nil {
					t.Fatal(err)
				}
				if len(document.Plugins) != len(test.packages) {
					t.Fatalf("catalog plugins = %#v", document.Plugins)
				}
				catalogDirectory := path.Dir(string(catalog.Path))
				for _, plugin := range document.Plugins {
					wantRoot := "."
					if len(test.packages) > 1 {
						wantRoot = plugin.Name
					}
					resolvedRoot := path.Clean(path.Join(catalogDirectory, plugin.Source))
					if resolvedRoot != wantRoot {
						t.Fatalf("source %q from %q resolves to %q, want %q", plugin.Source, catalogDirectory, resolvedRoot, wantRoot)
					}
					manifestPath := model.RelativePath(path.Join(resolvedRoot, ".claude-plugin/plugin.json"))
					if _, exists := plannedFile(plan, manifestPath); !exists {
						t.Fatalf("source %q resolves to root without manifest %q; paths = %#v", plugin.Source, manifestPath, plannedPaths(plan))
					}
				}
			})
		}
	}
}

func TestPackageTargetCatalogsRejectMissingMetadata(t *testing.T) {
	t.Parallel()

	for _, targetID := range []model.TargetID{model.TargetClaude, model.TargetCodex, model.TargetCopilot, model.TargetCursor, model.TargetGrok} {
		targetID := targetID
		for _, test := range []struct {
			name       string
			distribute func(model.DistributionMetadata)
			publish    func(model.PackageMetadata)
			code       string
			message    string
		}{
			{name: "distribution", distribute: func(metadata model.DistributionMetadata) { delete(metadata, "version") }, code: "invalid-distribution-metadata", message: "requires version"},
			{name: "package", publish: func(metadata model.PackageMetadata) { delete(metadata, "repository") }, code: "invalid-package-publication-metadata", message: "requires repository"},
		} {
			t.Run(string(targetID)+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				pkg := catalogPackage(targetID, "alpha")
				if test.publish != nil {
					test.publish(pkg.Metadata)
				}
				distribution := catalogDistribution()
				if test.distribute != nil {
					test.distribute(distribution)
				}
				adapter, diagnostics := Resolve(targetID)
				if len(diagnostics) != 0 {
					t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
				}
				plan, diagnostics := Render(adapter, model.TargetRenderInput{Packages: []model.NormalizedPackage{pkg}, Distribution: distribution, PackageMode: model.TargetPackageModeSeparate})
				if len(diagnostics) == 0 || diagnostics[0].Code != test.code || !strings.Contains(diagnostics[0].Message, test.message) {
					t.Fatalf("Render() = (%#v, %#v), want %s containing %q", plan, diagnostics, test.code, test.message)
				}
				if len(plan.Files) != 0 {
					t.Fatalf("failed plan files = %#v, want none", plan.Files)
				}
			})
		}
	}
}

func TestAggregateModeIsPiOnlyAndPiEmitsNoCatalog(t *testing.T) {
	t.Parallel()

	for _, targetID := range []model.TargetID{model.TargetClaude, model.TargetCodex, model.TargetCopilot, model.TargetCursor, model.TargetGrok} {
		input := model.TargetRenderInput{
			Packages:     []model.NormalizedPackage{{Identity: "alpha", Target: targetID, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{}}},
			Distribution: catalogDistribution(), PackageMode: model.TargetPackageModeAggregate,
			Aggregate: &model.AggregatePackage{Identity: "team-tools", Metadata: model.PackageMetadata{}},
		}
		adapter, diagnostics := Resolve(targetID)
		if len(diagnostics) != 0 {
			t.Fatalf("Resolve(%q) diagnostics = %#v", targetID, diagnostics)
		}
		plan, diagnostics := Render(adapter, input)
		if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "does not support aggregate package mode") {
			t.Fatalf("Render(%q) = (%#v, %#v), want aggregate rejection", targetID, plan, diagnostics)
		}
	}

	piInput := model.TargetRenderInput{
		Packages:     []model.NormalizedPackage{{Identity: "alpha", Target: model.TargetPi, Profile: model.TargetProfilePackage, Metadata: model.PackageMetadata{}}},
		Distribution: catalogDistribution(), PackageMode: model.TargetPackageModeAggregate,
		Aggregate: &model.AggregatePackage{Identity: "team-tools", Metadata: model.PackageMetadata{"version": "1.0.0"}},
	}
	adapter, diagnostics := Resolve(model.TargetPi)
	if len(diagnostics) != 0 {
		t.Fatalf("Resolve(pi) diagnostics = %#v", diagnostics)
	}
	plan, diagnostics := Render(adapter, piInput)
	if len(diagnostics) != 0 {
		t.Fatalf("Render(pi) diagnostics = %#v", diagnostics)
	}
	if _, exists := catalogFile(plan); exists {
		t.Fatalf("Pi plan unexpectedly contains marketplace output: %#v", plannedPaths(plan))
	}
}

func renderCatalogTarget(targetID model.TargetID, packages []model.NormalizedPackage) (model.TargetPlan, []model.Diagnostic) {
	adapter, diagnostics := Resolve(targetID)
	if len(diagnostics) != 0 {
		return model.TargetPlan{Target: targetID}, diagnostics
	}
	return Render(adapter, model.TargetRenderInput{
		Packages: packages, Distribution: catalogDistribution(), PackageMode: model.TargetPackageModeSeparate,
	})
}

func catalogDistribution() model.DistributionMetadata {
	return model.DistributionMetadata{
		"name": "team-tools", "owner": "Platform Team", "description": "Shared developer tools", "version": "2.0.0",
	}
}

func catalogPackage(targetID model.TargetID, identity model.PackageID) model.NormalizedPackage {
	return model.NormalizedPackage{
		Identity: identity, Target: targetID, Profile: model.TargetProfilePackage,
		Metadata: model.PackageMetadata{
			"description": strings.ToUpper(string(identity[:1])) + string(identity[1:]) + " workflows",
			"version":     "1.2.3",
			"author":      map[string]any{"name": "Platform Team", "email": "plugins@example.com"},
			"homepage":    "https://example.com/" + string(identity),
			"repository":  "https://github.com/example/" + string(identity),
			"license":     "MIT", "keywords": []any{"tools", "agents"},
		},
	}
}

func assertCatalogRequiredFields(t *testing.T, targetID model.TargetID, data []byte) {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["name"] != "team-tools" {
		t.Fatalf("catalog name = %#v", document["name"])
	}
	plugins, ok := document["plugins"].([]any)
	if !ok || len(plugins) != 1 {
		t.Fatalf("catalog plugins = %#v", document["plugins"])
	}
	entry, ok := plugins[0].(map[string]any)
	if !ok || entry["name"] != "alpha" || entry["source"] == nil {
		t.Fatalf("catalog entry = %#v", plugins[0])
	}
	if targetID == model.TargetCodex {
		presentation, _ := document["interface"].(map[string]any)
		policy, _ := entry["policy"].(map[string]any)
		if presentation["displayName"] != "team-tools" || policy["installation"] != "AVAILABLE" || policy["authentication"] != "ON_INSTALL" || entry["category"] != "Productivity" {
			t.Fatalf("Codex catalog fields = document %#v, entry %#v", document, entry)
		}
		return
	}
	owner, _ := document["owner"].(map[string]any)
	if owner["name"] != "Platform Team" {
		t.Fatalf("catalog owner = %#v", document["owner"])
	}
	if targetID == model.TargetClaude || targetID == model.TargetGrok {
		if document["description"] != "Shared developer tools" || document["version"] != "2.0.0" {
			t.Fatalf("catalog metadata = %#v", document)
		}
	} else {
		metadata, _ := document["metadata"].(map[string]any)
		if metadata["description"] != "Shared developer tools" || metadata["version"] != "2.0.0" {
			t.Fatalf("catalog metadata = %#v", document["metadata"])
		}
	}
	for _, field := range []string{"description", "version", "author", "homepage", "repository", "license", "keywords"} {
		if entry[field] == nil {
			t.Fatalf("catalog entry field %q is missing: %#v", field, entry)
		}
	}
}

func assertCatalogSources(t *testing.T, targetID model.TargetID, data []byte, wantNames, wantSources []string) {
	t.Helper()
	var document struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source any    `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var names, sources []string
	for _, plugin := range document.Plugins {
		names = append(names, plugin.Name)
		switch source := plugin.Source.(type) {
		case string:
			sources = append(sources, source)
		case map[string]any:
			if targetID != model.TargetCodex || source["source"] != "local" {
				t.Fatalf("source = %#v", source)
			}
			path, _ := source["path"].(string)
			sources = append(sources, path)
		default:
			t.Fatalf("source = %#v", plugin.Source)
		}
	}
	if !reflect.DeepEqual(names, wantNames) || !reflect.DeepEqual(sources, wantSources) {
		t.Fatalf("catalog entries = names %#v, sources %#v; want %#v, %#v", names, sources, wantNames, wantSources)
	}
}

func catalogFile(plan model.TargetPlan) (model.PlannedFile, bool) {
	for _, file := range plan.Files {
		if strings.HasSuffix(string(file.Path), "/marketplace.json") {
			return file, true
		}
	}
	return model.PlannedFile{}, false
}

func plannedFile(plan model.TargetPlan, path model.RelativePath) (model.PlannedFile, bool) {
	for _, file := range plan.Files {
		if file.Path == path {
			return file, true
		}
	}
	return model.PlannedFile{}, false
}

func plannedPaths(plan model.TargetPlan) []model.RelativePath {
	paths := make([]model.RelativePath, 0, len(plan.Files))
	for _, file := range plan.Files {
		paths = append(paths, file.Path)
	}
	return paths
}
