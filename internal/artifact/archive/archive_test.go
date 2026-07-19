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
	second, err := WriteTargetRoots(workspace, manifest, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	if got := archiveHashes(t, second); !reflect.DeepEqual(got, firstHashes) {
		t.Fatalf("archive hashes changed: first=%x second=%x", firstHashes, got)
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
