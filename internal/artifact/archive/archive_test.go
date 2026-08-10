package archive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// planForTest builds a minimal BuildPlan with ArchiveUnits and PlannedFile bytes.
func planForTest() model.BuildPlan {
	return model.BuildPlan{Targets: []model.TargetPlan{
		{
			Target: model.TargetAntigravity,
			ArchiveUnits: []model.ArchiveUnit{
				{Root: ".", Stem: "antigravity", Suffix: ".tar.gz"},
			},
			Files: []model.PlannedFile{
				{Path: "plugin.json", Bytes: []byte("{\"name\":\"demo\"}\n")},
				{Path: "skills/demo/SKILL.md", Bytes: []byte("# Demo\n")},
			},
		},
		{
			Target: model.TargetClaude,
			ArchiveUnits: []model.ArchiveUnit{
				{Root: ".", Stem: "claude", Suffix: ".tar.gz"},
			},
			Files: []model.PlannedFile{
				{Path: ".claude-plugin/marketplace.json", Bytes: []byte("{}\n")},
				{Path: "hooks/run.sh", Bytes: []byte("#!/bin/sh\n"), Executable: true},
			},
		},
		{
			Target: model.TargetPi,
			ArchiveUnits: []model.ArchiveUnit{
				{Root: ".", Stem: "pi", Suffix: ".tgz"},
			},
			Files: []model.PlannedFile{
				{Path: "package.json", Bytes: []byte("{}\n")},
			},
		},
	}}
}

func TestWriteTargetRootsCreatesDeterministicPlanOwnedArchives(t *testing.T) {
	distribution := model.DistributionMetadata{"name": "demo"}
	plan := planForTest()
	output := filepath.Join(t.TempDir(), "release")

	first, err := writeTargetRoots(distribution, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	firstHashes := archiveHashes(t, first)
	firstInfo, err := os.Stat(first[0])
	if err != nil {
		t.Fatal(err)
	}

	second, err := writeTargetRoots(distribution, plan, output)
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

	// Verify archive contents and suffixes.
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

func TestWriteTargetRootsUsesFixedWidthTemporaryNameForNearLimitBasename(t *testing.T) {
	distributionName := strings.Repeat("d", 245)
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target:       model.TargetPi,
		ArchiveUnits: []model.ArchiveUnit{{Root: ".", Stem: "p", Suffix: ".tar.gz"}},
		Files:        []model.PlannedFile{{Path: "plugin.json", Bytes: []byte("{}\n")}},
	}}}
	output := filepath.Join(t.TempDir(), "release")
	paths, err := writeTargetRoots(model.DistributionMetadata{"name": distributionName}, plan, output)
	if err != nil {
		t.Fatalf("writeTargetRoots() with %d-byte final basename: %v", len(distributionName)+len("-p.tar.gz"), err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths = %v, want one archive", paths)
	}
	if info, err := os.Stat(paths[0]); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("near-limit archive: info=%v err=%v", info, err)
	}
}

func TestWriteTargetRootsUsesPAXForLongPlannedPaths(t *testing.T) {
	longPath := filepath.ToSlash(filepath.Join(strings.Repeat("a", 80), strings.Repeat("b", 80)+".txt"))
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target:       model.TargetAntigravity,
		ArchiveUnits: []model.ArchiveUnit{{Root: ".", Stem: "long", Suffix: ".tar.gz"}},
		Files:        []model.PlannedFile{{Path: model.RelativePath(longPath), Bytes: []byte("long")}},
	}}}
	output := filepath.Join(t.TempDir(), "release")
	paths, err := writeTargetRoots(model.DistributionMetadata{"name": "demo"}, plan, output)
	if err != nil {
		t.Fatalf("writeTargetRoots() error = %v", err)
	}
	if got := tarEntries(t, paths[0]); !reflect.DeepEqual(got, []string{longPath}) {
		t.Fatalf("archive entries = %#v; want long path", got)
	}
}

func TestWriteTargetRootsPinnedDestinationRejectsPathSwap(t *testing.T) {
	outputParent := t.TempDir()
	output := filepath.Join(outputParent, "release")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	destination, err := OpenDestination(output)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = destination.Close() }()
	if err := destination.Create(); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(outputParent, "moved")
	if err := os.Rename(output, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, output); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := WriteTargetRootsInDestination(model.DistributionMetadata{"name": "demo"}, planForTest(), destination); err == nil {
		t.Fatal("WriteTargetRootsInDestination accepted a swapped destination path")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("swapped destination received archive files: %v", entries)
	}
}

func TestOpenDestinationPinsExistingParentBeforeCreatingDescendants(t *testing.T) {
	parentPath := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(parentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(parentPath, "nested", "release")
	destination, err := OpenDestination(output)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = destination.Close() }()
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("OpenDestination created output before validation: %v", err)
	}

	movedParent := parentPath + "-moved"
	if err := os.Rename(parentPath, movedParent); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, parentPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := destination.Create(); err == nil {
		t.Fatal("Create accepted a swapped destination pathname")
	}
	if _, err := os.Stat(filepath.Join(outside, "nested")); !os.IsNotExist(err) {
		t.Fatalf("swapped parent received destination descendants: %v", err)
	}
	if info, err := os.Stat(filepath.Join(movedParent, "nested", "release")); err != nil || !info.IsDir() {
		t.Fatalf("destination descendants were not created below pinned parent: info=%v err=%v", info, err)
	}
}

func TestDestinationCreateRejectsSymlinkedMissingDescendant(t *testing.T) {
	parentPath := t.TempDir()
	output := filepath.Join(parentPath, "nested", "release")
	destination, err := OpenDestination(output)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = destination.Close() }()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(parentPath, "nested")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := destination.Create(); err == nil {
		t.Fatal("Create followed a symlinked missing destination component")
	}
	if _, err := os.Stat(filepath.Join(outside, "release")); !os.IsNotExist(err) {
		t.Fatalf("symlink target received destination descendants: %v", err)
	}
}

func TestWriteTargetRootsDoesNotFollowPredictableTemporarySymlink(t *testing.T) {
	output := filepath.Join(t.TempDir(), "release")
	destination := filepath.Join(output, "demo-antigravity.tar.gz")
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, destination+".tmp"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := writeTargetRoots(model.DistributionMetadata{"name": "demo"}, planForTest(), output); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(sentinel); err != nil {
		t.Fatal(err)
	} else if string(got) != "sentinel" {
		t.Fatalf("predictable temporary symlink target changed to %q", got)
	}
	if info, err := os.Lstat(destination); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("archive destination is not a regular file: %v", info.Mode())
	}
}

func TestWriteTargetRootsArchiveUnitsFilterByRoot(t *testing.T) {
	// agent-plugins style: two plugins with separate roots.
	distribution := model.DistributionMetadata{"name": "test"}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetAgentPlugins,
		ArchiveUnits: []model.ArchiveUnit{
			{Root: "alpha", Stem: "alpha", Suffix: ".tar.gz"},
			{Root: "beta", Stem: "beta", Suffix: ".tar.gz"},
		},
		Files: []model.PlannedFile{
			{Path: "alpha/plugin.json", Bytes: []byte("{\"name\":\"alpha\"}\n")},
			{Path: "alpha/mcp.json", Bytes: []byte("{}\n")},
			{Path: "beta/plugin.json", Bytes: []byte("{\"name\":\"beta\"}\n")},
		},
	}}}
	output := filepath.Join(t.TempDir(), "release")

	paths, err := writeTargetRoots(distribution, plan, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want 2", paths)
	}

	// alpha archive: plugin.json and mcp.json, with alpha/ prefix stripped.
	if got, want := tarEntries(t, filepath.Join(output, "test-alpha.tar.gz")), []string{"mcp.json", "plugin.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("alpha entries = %#v, want %#v", got, want)
	}
	// beta archive: only beta/plugin.json, prefix stripped.
	if got, want := tarEntries(t, filepath.Join(output, "test-beta.tar.gz")), []string{"plugin.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("beta entries = %#v, want %#v", got, want)
	}
}

func TestWriteTargetRootsStripsRootPrefixFromArchiveEntries(t *testing.T) {
	distribution := model.DistributionMetadata{"name": "strip-test"}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetAgentPlugins,
		ArchiveUnits: []model.ArchiveUnit{
			{Root: "myplugin", Stem: "myplugin", Suffix: ".tar.gz"},
		},
		Files: []model.PlannedFile{
			{Path: "myplugin/plugin.json", Bytes: []byte("{\"name\":\"myplugin\"}\n")},
			{Path: "myplugin/skills/my-skill/SKILL.md", Bytes: []byte("# Skill\n")},
		},
	}}}
	output := filepath.Join(t.TempDir(), "release")
	if _, err := writeTargetRoots(distribution, plan, output); err != nil {
		t.Fatal(err)
	}
	entries := tarEntries(t, filepath.Join(output, "strip-test-myplugin.tar.gz"))
	want := []string{"plugin.json", "skills/my-skill/SKILL.md"}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v (prefix should be stripped)", entries, want)
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
		{name: "colon", input: "foo:bar"},
		{name: "asterisk", input: "foo*bar"},
		{name: "question mark", input: "foo?bar"},
		{name: "quote", input: `foo"bar`},
		{name: "less than", input: "foo<bar"},
		{name: "greater than", input: "foo>bar"},
		{name: "pipe", input: "foo|bar"},
		{name: "control", input: "foo\x1fbar"},
		{name: "trailing dot", input: "foo."},
		{name: "trailing space", input: "foo "},
		{name: "empty", input: ""},
		{name: "dot", input: "."},
		{name: "double dot", input: ".."},
		{name: "reserved CON", input: "CON"},
		{name: "reserved CON with space", input: "CON .txt"},
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
	distribution := model.DistributionMetadata{"name": "../evil"}
	plan := planForTest()
	output := filepath.Join(t.TempDir(), "release")

	_, err := writeTargetRoots(distribution, plan, output)
	if err == nil {
		t.Fatal("writeTargetRoots() accepted a path-traversal distribution name")
	}
	// No archive must have escaped the output directory.
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(output), "evil-claude.tar.gz")); !os.IsNotExist(statErr) {
		t.Fatal("archive escaped the output directory")
	}
	// The output directory must not have been created before the name was validated.
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatal("output directory was created before name validation")
	}
}

func TestWriteTargetRootsRejectsReservedDistributionName(t *testing.T) {
	distribution := model.DistributionMetadata{"name": "CON"}
	plan := planForTest()
	_, err := writeTargetRoots(distribution, plan, filepath.Join(t.TempDir(), "release"))
	if err == nil {
		t.Fatal("writeTargetRoots() accepted a reserved device name")
	}
}

func TestWriteTargetRootsRejectsEmptyArchiveUnit(t *testing.T) {
	// A unit whose root matches no plan files must fail.
	distribution := model.DistributionMetadata{"name": "demo"}
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetAgentPlugins,
		ArchiveUnits: []model.ArchiveUnit{
			{Root: "missing-plugin", Stem: "missing-plugin", Suffix: ".tar.gz"},
		},
		Files: []model.PlannedFile{
			{Path: "other-plugin/plugin.json", Bytes: []byte(`{}`)},
		},
	}}}
	output := filepath.Join(t.TempDir(), "release")
	if _, err := writeTargetRoots(distribution, plan, output); err == nil {
		t.Fatal("writeTargetRoots() accepted unit with no matching files")
	}
}

func TestWriteTargetRootsRejectsInvalidArchivePartitionsBeforeMutation(t *testing.T) {
	files := []model.PlannedFile{{Path: "foo/a", Bytes: []byte("a")}, {Path: "bar/b", Bytes: []byte("b")}}
	for _, test := range []struct {
		name  string
		units []model.ArchiveUnit
	}{
		{
			name:  "uncovered",
			units: []model.ArchiveUnit{{Root: "foo", Stem: "foo", Suffix: ".tar.gz"}},
		},
		{
			name:  "overlap",
			units: []model.ArchiveUnit{{Root: ".", Stem: "all", Suffix: ".tar.gz"}, {Root: "foo", Stem: "foo", Suffix: ".tar.gz"}},
		},
		{
			name:  "duplicate destination",
			units: []model.ArchiveUnit{{Root: "foo", Stem: "same", Suffix: ".tar.gz"}, {Root: "bar", Stem: "same", Suffix: ".tar.gz"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "release")
			plan := model.BuildPlan{Targets: []model.TargetPlan{{
				Target: model.TargetAgentPlugins, Files: files, ArchiveUnits: test.units,
			}}}
			if _, err := writeTargetRoots(model.DistributionMetadata{"name": "demo"}, plan, output); err == nil {
				t.Fatal("writeTargetRoots() accepted invalid archive partition")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("archive output was mutated before complete validation: %v", err)
			}
		})
	}
}

func TestFilterFilesWithDotRootIncludesAll(t *testing.T) {
	files := []model.PlannedFile{
		{Path: "a.txt"},
		{Path: "b/c.txt"},
	}
	result := filterFiles(files, ".")
	if len(result) != len(files) {
		t.Fatalf("filterFiles(., ...) = %v, want all %d files", result, len(files))
	}
}

func TestFilterFilesWithSubdirRootFiltersCorrectly(t *testing.T) {
	files := []model.PlannedFile{
		{Path: "alpha/plugin.json"},
		{Path: "alpha/mcp.json"},
		{Path: "beta/plugin.json"},
	}
	result := filterFiles(files, "alpha")
	if len(result) != 2 {
		t.Fatalf("filterFiles(alpha) = %v, want 2 files", result)
	}
	for _, f := range result {
		if !hasPrefix(string(f.Path), "alpha/") {
			t.Errorf("filtered file %q does not start with alpha/", f.Path)
		}
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
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
