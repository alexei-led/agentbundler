package antigravity

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/packageoutput"
)

func TestManifestStrictFieldsAndBytes(t *testing.T) {
	tests := []struct {
		name     string
		identity model.PackageID
		metadata model.PackageMetadata
		want     string
		wantErr  string
	}{
		{name: "name only", identity: "demo", metadata: model.PackageMetadata{}, want: "{\"name\":\"demo\"}\n"},
		{name: "description", identity: "demo_plugin-1", metadata: model.PackageMetadata{"description": "Demo", "version": "9", "author": "ignored"}, want: "{\"description\":\"Demo\",\"name\":\"demo_plugin-1\"}\n"},
		{name: "empty description", identity: "demo", metadata: model.PackageMetadata{"description": ""}, want: "{\"description\":\"\",\"name\":\"demo\"}\n"},
		{name: "non-string description", identity: "demo", metadata: model.PackageMetadata{"description": 1}, wantErr: "description must be a string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := manifest(model.NormalizedPackage{Identity: test.identity, Metadata: test.metadata})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("manifest() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil || string(got) != test.want {
				t.Fatalf("manifest() = %q, %v, want %q", got, err, test.want)
			}
		})
	}
}

func TestValidatePackageName(t *testing.T) {
	for _, test := range []struct {
		name string
		ok   bool
	}{{"Alpha_1-beta", true}, {"bad.name", false}, {"bad name", false}, {"", false}} {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := validatePackage(model.NormalizedPackage{Identity: model.PackageID(test.name)})
			if (len(diagnostics) == 0) != test.ok {
				t.Fatalf("validatePackage(%q) = %#v", test.name, diagnostics)
			}
		})
	}
}

func TestMarkdownAgentStrictSubset(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter map[string]any
		body        string
		want        string
		wantErr     string
	}{
		{name: "valid", frontmatter: map[string]any{"description": "Review code", "name": "reviewer"}, body: "Review.\n", want: "---\n{\"description\":\"Review code\",\"name\":\"reviewer\"}\n---\nReview.\n"},
		{name: "multiline strings", frontmatter: map[string]any{"description": "Review\ncarefully", "name": "reviewer"}, body: "Body unchanged\n\n- item\n", want: "---\n{\"description\":\"Review\\ncarefully\",\"name\":\"reviewer\"}\n---\nBody unchanged\n\n- item\n"},
		{name: "missing name", frontmatter: map[string]any{"description": "Review"}, wantErr: "name"},
		{name: "empty description", frontmatter: map[string]any{"name": "reviewer", "description": ""}, wantErr: "description"},
		{name: "non-string name", frontmatter: map[string]any{"name": []any{"reviewer"}, "description": "Review"}, wantErr: "name"},
		{name: "extra field", frontmatter: map[string]any{"name": "reviewer", "description": "Review", "model": "fast"}, wantErr: "field \"model\""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asset := model.NormalizedAsset{Identity: "agent/reviewer", Content: model.AssetContent{Frontmatter: test.frontmatter, Body: test.body}}
			got, suffix, err := markdownAgent(asset)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("markdownAgent() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil || suffix != ".md" || string(got) != test.want {
				t.Fatalf("markdownAgent() = %q, %q, %v", got, suffix, err)
			}
		})
	}
}

func TestNativeResourceSortedDetachedExactFiles(t *testing.T) {
	origin := model.SourceLocation{Path: "source/native/rules/conductor.md"}
	asset := model.NormalizedAsset{Identity: "native-resource/conductor", Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{
		"rules/conductor.md":      {Bytes: []byte("rule\n"), Origin: []model.SourceLocation{origin}},
		"mcp_config.json":         {Bytes: []byte("{}\n")},
		"shared/scripts/check.sh": {Bytes: []byte("#!/bin/sh\n"), Executable: true},
	}}}
	got, err := nativeResource(asset)
	if err != nil {
		t.Fatal(err)
	}
	paths := []model.RelativePath{got[0].Path, got[1].Path, got[2].Path}
	want := []model.RelativePath{"mcp_config.json", "rules/conductor.md", "shared/scripts/check.sh"}
	if !reflect.DeepEqual(paths, want) || !got[2].Content.Executable || !reflect.DeepEqual(got[1].Content.Origin, []model.SourceLocation{origin}) {
		t.Fatalf("nativeResource() = %#v", got)
	}
	asset.Content.Files["mcp_config.json"] = model.FileContent{Bytes: []byte("changed")}
	if string(got[0].Content.Bytes) != "{}\n" {
		t.Fatal("nativeResource() returned aliased bytes")
	}
}

func TestNativeResourceRejectsEmptyAndPiExtensions(t *testing.T) {
	for _, asset := range []model.NormalizedAsset{
		{Identity: "native-resource/empty", Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{}}},
		{Identity: "native-resource/pi", Native: &model.NativeResourceOptions{PiExtensions: []model.RelativePath{"extension.ts"}}, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{"extension.ts": {}}}},
	} {
		if _, err := nativeResource(asset); err == nil {
			t.Fatalf("nativeResource(%q) succeeded", asset.Identity)
		}
	}
}

func TestNativeResourceTraversalAndCollisionsFailClosed(t *testing.T) {
	pkg := packageFixture()
	pkg.Assets = []model.NormalizedAsset{{Identity: "native-resource/bad", Kind: model.AssetKindNativeResource, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{"../escape": {}}}}}
	if _, diagnostics := Render(separate(pkg)); len(diagnostics) == 0 {
		t.Fatal("traversal rendered")
	}

	pkg = packageFixture()
	pkg.Assets = append(pkg.Assets, model.NormalizedAsset{Identity: "native-resource/collision", Kind: model.AssetKindNativeResource, Content: model.AssetContent{Files: map[model.RelativePath]model.FileContent{"plugin.json": {Bytes: []byte("native")}}}})
	if _, diagnostics := Render(separate(pkg)); len(diagnostics) != 1 || diagnostics[0].Code != "invalid-package-output" {
		t.Fatalf("collision diagnostics = %#v", diagnostics)
	}
}

func TestPackageCodecShape(t *testing.T) {
	codec := PackageCodec()
	if codec.Target != Target || codec.ManifestPath != "plugin.json" || codec.AgentRoot != "agents" || codec.Hooks != nil || codec.HookPayloadRoot != "" || codec.Catalog != nil || codec.Manifest == nil || codec.Agent == nil || codec.NativeResource == nil || codec.ValidatePackage == nil {
		t.Fatalf("PackageCodec() = %#v", codec)
	}
}

func separate(pkg model.NormalizedPackage) model.TargetRenderInput {
	return model.TargetRenderInput{Packages: []model.NormalizedPackage{pkg}, PackageMode: model.TargetPackageModeSeparate}
}

var _ packageoutput.Codec = PackageCodec()
