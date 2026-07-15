package marketplace

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestBuildOrdersEntriesAndAssignsPackageRoots(t *testing.T) {
	t.Parallel()

	input := catalogInput([]model.NormalizedPackage{
		publicationPackage("zeta", map[string]any{"author": map[string]any{"name": "Zeta Team", "email": "zeta@example.com"}, "keywords": []any{"tools", "agents"}}),
		publicationPackage("alpha", nil),
	})
	catalog, diagnostics := Build(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Build() diagnostics = %#v", diagnostics)
	}
	if catalog.Name != "team-tools" || catalog.Owner != (Person{Name: "Platform Team"}) || catalog.Description != "Shared developer tools" || catalog.Version != "2.0.0" {
		t.Fatalf("catalog metadata = %#v", catalog)
	}
	if got := []string{catalog.Entries[0].Name, catalog.Entries[1].Name}; !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("entry names = %#v", got)
	}
	if got := []string{catalog.Entries[0].Source, catalog.Entries[1].Source}; !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("entry sources = %#v", got)
	}
	if got := catalog.Entries[1].Keywords; !reflect.DeepEqual(got, []string{"agents", "tools"}) {
		t.Fatalf("sorted keywords = %#v", got)
	}
	if got := catalog.Entries[1].Author; got.Email != "zeta@example.com" || got.Name != "Zeta Team" {
		t.Fatalf("author = %#v", got)
	}
}

func TestBuildUsesDotForFlatSinglePackage(t *testing.T) {
	t.Parallel()

	input := catalogInput([]model.NormalizedPackage{publicationPackage("alpha", nil)})
	input.Distribution["owner"] = map[string]any{"name": "Platform Team", "email": "plugins@example.com"}
	catalog, diagnostics := Build(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Build() diagnostics = %#v", diagnostics)
	}
	if len(catalog.Entries) != 1 || catalog.Entries[0].Source != "." {
		t.Fatalf("entries = %#v", catalog.Entries)
	}
	if catalog.Owner != (Person{Name: "Platform Team", Email: "plugins@example.com"}) {
		t.Fatalf("owner = %#v", catalog.Owner)
	}
}

func TestBuildRejectsMissingAndMalformedPublicationMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*model.TargetRenderInput)
		want   string
	}{
		{name: "missing distribution", mutate: func(input *model.TargetRenderInput) { input.Distribution = nil }, want: "distribution requires name"},
		{name: "unknown distribution field", mutate: func(input *model.TargetRenderInput) { input.Distribution["publisher"] = "team" }, want: `distribution field "publisher" is not supported`},
		{name: "invalid distribution identity", mutate: func(input *model.TargetRenderInput) { input.Distribution["name"] = "Team Tools" }, want: "lowercase kebab-case"},
		{name: "invalid distribution version", mutate: func(input *model.TargetRenderInput) { input.Distribution["version"] = "latest" }, want: "semantic version"},
		{name: "invalid distribution owner", mutate: func(input *model.TargetRenderInput) {
			input.Distribution["owner"] = map[string]any{"name": "Platform Team", "url": "https://example.com/team"}
		}, want: `distribution owner field "url" is not supported`},
		{name: "missing package field", mutate: func(input *model.TargetRenderInput) { delete(input.Packages[0].Metadata, "license") }, want: "requires license"},
		{name: "invalid package identity", mutate: func(input *model.TargetRenderInput) { input.Packages[0].Identity = "Alpha" }, want: "lowercase kebab-case"},
		{name: "invalid author", mutate: func(input *model.TargetRenderInput) {
			input.Packages[0].Metadata["author"] = map[string]any{"email": "team@example.com"}
		}, want: "requires name"},
		{name: "invalid URL", mutate: func(input *model.TargetRenderInput) {
			input.Packages[0].Metadata["repository"] = "git@example.com:team/tools.git"
		}, want: "absolute HTTP or HTTPS URL"},
		{name: "invalid keywords", mutate: func(input *model.TargetRenderInput) { input.Packages[0].Metadata["keywords"] = []any{} }, want: "non-empty string array"},
		{name: "aggregate", mutate: func(input *model.TargetRenderInput) { input.PackageMode = model.TargetPackageModeAggregate }, want: "require separate package mode"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := catalogInput([]model.NormalizedPackage{publicationPackage("alpha", nil)})
			test.mutate(&input)
			catalog, diagnostics := Build(input)
			if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, test.want) {
				t.Fatalf("Build() = (%#v, %#v), want diagnostic containing %q", catalog, diagnostics, test.want)
			}
			if len(catalog.Entries) != 0 {
				t.Fatalf("catalog entries = %#v, want none", catalog.Entries)
			}
		})
	}
}

func TestBuildRejectsIdentityAndSourceCollisions(t *testing.T) {
	t.Parallel()

	input := catalogInput([]model.NormalizedPackage{publicationPackage("alpha", nil), publicationPackage("alpha", nil)})
	_, diagnostics := Build(input)
	if len(diagnostics) == 0 || diagnostics[0].Code != "catalog-identity-collision" {
		t.Fatalf("Build() diagnostics = %#v, want catalog-identity-collision", diagnostics)
	}
}

func TestBuildIsReproducible(t *testing.T) {
	t.Parallel()

	first, firstDiagnostics := Build(catalogInput([]model.NormalizedPackage{publicationPackage("zeta", nil), publicationPackage("alpha", nil)}))
	second, secondDiagnostics := Build(catalogInput([]model.NormalizedPackage{publicationPackage("alpha", nil), publicationPackage("zeta", nil)}))
	if len(firstDiagnostics) != 0 || len(secondDiagnostics) != 0 {
		t.Fatalf("Build() diagnostics = (%#v, %#v)", firstDiagnostics, secondDiagnostics)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("catalogs differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func catalogInput(packages []model.NormalizedPackage) model.TargetRenderInput {
	return model.TargetRenderInput{
		Packages: packages,
		Distribution: model.DistributionMetadata{
			"name": "team-tools", "owner": "Platform Team", "description": "Shared developer tools", "version": "2.0.0",
		},
		PackageMode: model.TargetPackageModeSeparate,
	}
}

func publicationPackage(identity model.PackageID, overrides map[string]any) model.NormalizedPackage {
	metadata := model.PackageMetadata{
		"description": "Developer workflows", "version": "1.2.3", "author": "Platform Team",
		"homepage": "https://example.com/tools", "repository": "https://github.com/example/tools",
		"license": "MIT", "keywords": []string{"agents", "tools"},
	}
	for key, value := range overrides {
		metadata[key] = value
	}
	return model.NormalizedPackage{Identity: identity, Metadata: metadata, Target: model.TargetClaude, Profile: model.TargetProfilePackage}
}
