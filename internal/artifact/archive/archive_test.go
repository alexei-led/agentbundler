package archive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestWriteTargetRootsCreatesDeterministicNativeRootArchives(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, workspace, "dist/antigravity/plugin.json", "{\"name\":\"demo\"}\n", 0o644)
	writeFile(t, workspace, "dist/antigravity/skills/demo/SKILL.md", "# Demo\n", 0o644)
	writeFile(t, workspace, "dist/claude/.claude-plugin/marketplace.json", "{}\n", 0o644)
	writeFile(t, workspace, "dist/claude/hooks/run.sh", "#!/bin/sh\n", 0o755)
	writeFile(t, workspace, "dist/claude/.agentbundler/build.json", "private\n", 0o644)
	writeFile(t, workspace, "dist/pi/package.json", "{}\n", 0o644)
	writeFile(t, workspace, ".claude-plugin/marketplace.json", "root compatibility\n", 0o644)
	writeFile(t, workspace, ".agentbundler/compatibility.json", "root ownership\n", 0o644)
	writeFile(t, workspace, "package.json", "root Pi compatibility\n", 0o644)
	manifest := model.SourceManifest{Output: "dist", Distribution: model.DistributionMetadata{"name": "demo"}}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetAntigravity}, {Target: model.TargetClaude}, {Target: model.TargetPi}}}
	output := filepath.Join(workspace, "release")

	first, err := WriteTargetRoots(workspace, manifest, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	firstHashes := archiveHashes(t, first)
	firstInfo, err := os.Stat(first[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := WriteTargetRoots(workspace, manifest, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	if got := archiveHashes(t, second); !reflect.DeepEqual(got, firstHashes) {
		t.Fatalf("archive hashes changed: first=%x second=%x", firstHashes, got)
	}
	secondInfo, err := os.Stat(second[0])
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(firstInfo, secondInfo) {
		t.Fatal("second archive write replaced an unchanged archive")
	}
	if !firstInfo.ModTime().Equal(secondInfo.ModTime()) {
		t.Fatalf("second archive write changed timestamp: before=%v after=%v", firstInfo.ModTime(), secondInfo.ModTime())
	}
	if got, want := tarEntries(t, filepath.Join(output, "demo-antigravity.tar.gz")), []string{"plugin.json", "skills/demo/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Antigravity entries = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(filepath.Join(output, "demo-antigravity.tgz")); !os.IsNotExist(err) {
		t.Fatalf("Antigravity unexpectedly used Pi archive suffix: %v", err)
	}
	if got, want := tarEntries(t, filepath.Join(output, "demo-claude.tar.gz")), []string{".claude-plugin/marketplace.json", "hooks/run.sh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Claude entries = %#v, want %#v", got, want)
	}
	if got, want := tarEntries(t, filepath.Join(output, "demo-pi.tgz")), []string{"package.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Pi entries = %#v, want %#v", got, want)
	}
}

func TestValidateArchiveNameRejectsUnsafeBasenames(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "forward slash", input: "foo/bar"},
		{name: "backslash", input: `foo\bar`},
		{name: "traversal", input: "../evil"},
		{name: "null byte", input: "foo\x00bar"},
		{name: "dot", input: "."},
		{name: "double dot", input: ".."},
		{name: "reserved CON", input: "CON"},
		{name: "reserved NUL", input: "NUL"},
		{name: "reserved COM1", input: "COM1"},
		{name: "reserved LPT9", input: "lpt9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateArchiveName(test.input); err == nil {
				t.Fatalf("validateArchiveName(%q) accepted unsafe name", test.input)
			}
		})
	}
}

func TestValidateArchiveNameAcceptsSafeBasenames(t *testing.T) {
	for _, name := range []string{
		"my-project", "demo", "cc-thingz", "v1.2.3", "tools_2024", ".hidden",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateArchiveName(name); err != nil {
				t.Fatalf("validateArchiveName(%q) = %v", name, err)
			}
		})
	}
}

func TestWriteTargetRootsRejectsDistributionNameWithSeparator(t *testing.T) {
	workspace := t.TempDir()
	manifest := model.SourceManifest{Output: "dist", Distribution: model.DistributionMetadata{"name": "../evil"}}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude}}}
	output := filepath.Join(workspace, "release")

	_, err := WriteTargetRoots(workspace, manifest, plan, output)
	if err == nil {
		t.Fatal("WriteTargetRoots() accepted a path-traversal distribution name")
	}
	// Verify no archive files were created outside the output directory.
	if _, statErr := os.Stat(filepath.Join(workspace, "evil-claude.tar.gz")); !os.IsNotExist(statErr) {
		t.Fatal("archive escaped the output directory")
	}
	// The output directory must not have been created before the name was validated.
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatal("output directory was created before name validation")
	}
}

func TestWriteTargetRootsRejectsReservedDistributionName(t *testing.T) {
	workspace := t.TempDir()
	manifest := model.SourceManifest{Output: "dist", Distribution: model.DistributionMetadata{"name": "CON"}}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{Target: model.TargetClaude}}}
	_, err := WriteTargetRoots(workspace, manifest, plan, filepath.Join(workspace, "release"))
	if err == nil {
		t.Fatal("WriteTargetRoots() accepted a reserved device name")
	}
}

func writeFile(t *testing.T, root, path, content string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func archiveHashes(t *testing.T, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	result := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[filepath.Base(path)] = sha256.Sum256(data)
	}
	return result
}

func tarEntries(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gzipReader.Close() }()
	reader := tar.NewReader(gzipReader)
	var entries []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, header.Name)
	}
	return entries
}
