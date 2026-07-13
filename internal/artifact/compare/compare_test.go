package compare

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func TestDetectDriftClassifiesExactMissingChangedAndExtra(t *testing.T) {
	root := t.TempDir()
	plan := model.BuildPlan{
		Targets: []model.TargetPlan{{
			Target: model.TargetClaude,
			Files: []model.PlannedFile{
				{Path: "missing.txt", Bytes: []byte("missing")},
				{Path: "plugin.json", Bytes: []byte("{\"name\":\"example\"}\n")},
				{Path: "current.txt", Bytes: []byte("current")},
			},
		}},
		CompilerFiles: []model.PlannedFile{{Path: "compiler.txt", Bytes: []byte("compiler")}},
	}

	writeFile(t, root, "claude/plugin.json", []byte("{ \"name\": \"example\" }\n"), 0o644)
	writeFile(t, root, "claude/current.txt", []byte("current"), 0o644)
	writeFile(t, root, "claude/extra.txt", []byte("extra"), 0o644)
	writeFile(t, root, "compiler.txt", []byte("compiler"), 0o644)

	assertDrift(t, detectDrift(t, plan, root), []Drift{
		{Kind: DriftExtra, Path: "claude/extra.txt"},
		{Kind: DriftMissing, Path: "claude/missing.txt"},
		{Kind: DriftChanged, Path: "claude/plugin.json"},
	})
}

func TestDetectDriftOrdersFindingsByPath(t *testing.T) {
	root := t.TempDir()
	plan := model.BuildPlan{Targets: []model.TargetPlan{
		{
			Target: model.TargetPi,
			Files:  []model.PlannedFile{{Path: "z-missing.txt", Bytes: []byte("z")}},
		},
		{
			Target: model.TargetClaude,
			Files:  []model.PlannedFile{{Path: "a-missing.txt", Bytes: []byte("a")}},
		},
	}}

	writeFile(t, root, "z-extra.txt", []byte("z"), 0o644)
	writeFile(t, root, "a-extra.txt", []byte("a"), 0o644)

	assertDrift(t, detectDrift(t, plan, root), []Drift{
		{Kind: DriftExtra, Path: "a-extra.txt"},
		{Kind: DriftMissing, Path: "claude/a-missing.txt"},
		{Kind: DriftMissing, Path: "pi/z-missing.txt"},
		{Kind: DriftExtra, Path: "z-extra.txt"},
	})
}

func TestDetectDriftDoesNotMutateOutput(t *testing.T) {
	root := t.TempDir()
	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetClaude,
		Files: []model.PlannedFile{
			{Path: "current.txt", Bytes: []byte("current")},
			{Path: "missing.txt", Bytes: []byte("missing")},
		},
	}}}
	writeFile(t, root, "claude/current.txt", []byte("current"), 0o644)
	writeFile(t, root, "extra.txt", []byte("extra"), 0o644)

	before := snapshotTree(t, root)
	_ = detectDrift(t, plan, root)
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("output changed:\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestDetectDriftClassifiesSymlinksWithoutTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", filepath.Join(root, "claude", "expected-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("missing-directory", filepath.Join(root, "claude", "blocked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("missing-target", filepath.Join(root, "unplanned-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	plan := model.BuildPlan{Targets: []model.TargetPlan{{
		Target: model.TargetClaude,
		Files: []model.PlannedFile{
			{Path: "blocked/child.txt", Bytes: []byte("child")},
			{Path: "expected-link", Bytes: []byte("link")},
		},
	}}}

	assertDrift(t, detectDrift(t, plan, root), []Drift{
		{Kind: DriftExtra, Path: "claude/blocked"},
		{Kind: DriftMissing, Path: "claude/blocked/child.txt"},
		{Kind: DriftChanged, Path: "claude/expected-link"},
		{Kind: DriftExtra, Path: "unplanned-link"},
	})
}

func TestDetectDriftComparesExecutableIntent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executable files are rejected before comparison")
	}

	root := t.TempDir()
	plan := model.BuildPlan{CompilerFiles: []model.PlannedFile{{
		Path:       "tool",
		Bytes:      []byte("#!/bin/sh\nexit 0\n"),
		Executable: true,
	}}}
	writeFile(t, root, "tool", []byte("#!/bin/sh\nexit 0\n"), 0o644)

	assertDrift(t, detectDrift(t, plan, root), []Drift{{Kind: DriftChanged, Path: "tool"}})
	if err := os.Chmod(filepath.Join(root, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertDrift(t, detectDrift(t, plan, root), nil)
}

func writeFile(t *testing.T, root, relativePath string, contents []byte, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func detectDrift(t *testing.T, plan model.BuildPlan, root string) []Drift {
	t.Helper()
	drift, err := DetectDrift(plan, root)
	if err != nil {
		t.Fatalf("DetectDrift() error = %v", err)
	}
	return drift
}

func assertDrift(t *testing.T, got, want []Drift) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DetectDrift() = %#v, want %#v", got, want)
	}
}

type treeEntry struct {
	mode    os.FileMode
	modTime time.Time
	size    int64
}

func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	snapshot := make(map[string]treeEntry)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relativePath)] = treeEntry{
			mode:    info.Mode(),
			modTime: info.ModTime(),
			size:    info.Size(),
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
