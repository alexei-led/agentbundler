package artifact

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewWorkspaceLayoutGuardAcceptsDisjointPaths(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "generated")
	guard, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err != nil {
		t.Fatalf("NewWorkspaceLayoutGuard() = %v", err)
	}
	if err := guard.Revalidate(); err != nil {
		t.Fatalf("Revalidate() = %v for disjoint paths", err)
	}
}

func TestNewWorkspaceLayoutGuardAcceptsAbsentOutput(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	// output does not exist on disk
	output := filepath.Join(ws, "generated")
	if _, err := NewWorkspaceLayoutGuard(ws, source, output); err != nil {
		t.Fatalf("NewWorkspaceLayoutGuard() = %v for absent output", err)
	}
}

func TestNewWorkspaceLayoutGuardRejectsCaseFoldedOverlapOnCaseInsensitivePlatforms(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("case-folded path behavior is platform-specific")
	}
	ws := t.TempDir()
	source := filepath.Join(ws, "Source")
	output := filepath.Join(ws, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceLayoutGuard(ws, source, output); err == nil {
		t.Fatal("NewWorkspaceLayoutGuard() accepted case-folded roots")
	}
}

func TestNewWorkspaceLayoutGuardRejectsEqualPaths(t *testing.T) {
	ws := t.TempDir()
	root := filepath.Join(ws, "source")
	_, err := NewWorkspaceLayoutGuard(ws, root, root)
	if err == nil {
		t.Fatal("NewWorkspaceLayoutGuard() accepted equal source and output")
	}
	if !strings.Contains(err.Error(), root) {
		t.Fatalf("error does not name conflicting path %q: %v", root, err)
	}
}

func TestNewWorkspaceLayoutGuardRejectsOutputInsideSource(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "source", "generated")
	_, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err == nil {
		t.Fatal("NewWorkspaceLayoutGuard() accepted output inside source")
	}
	if !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), output) {
		t.Fatalf("error does not name both conflicting paths: %v", err)
	}
}

func TestNewWorkspaceLayoutGuardRejectsSourceInsideOutput(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "generated", "source")
	output := filepath.Join(ws, "generated")
	_, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err == nil {
		t.Fatal("NewWorkspaceLayoutGuard() accepted source inside output")
	}
	if !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), output) {
		t.Fatalf("error does not name both conflicting paths: %v", err)
	}
}

func TestNewWorkspaceLayoutGuardRejectsTextualAlias(t *testing.T) {
	// After filepath.Clean, paths that differ only by trailing slash are equal.
	ws := t.TempDir()
	root := filepath.Join(ws, "shared")
	// Both source and output resolve to the same cleaned path.
	_, err := NewWorkspaceLayoutGuard(ws, root, root)
	if err == nil {
		t.Fatal("NewWorkspaceLayoutGuard() accepted textual alias")
	}
}

func TestNewWorkspaceLayoutGuardRejectsSymlinkAlias(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "output")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make output a symlink pointing to source.
	if err := os.Symlink(source, output); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err == nil {
		t.Fatal("NewWorkspaceLayoutGuard() accepted symlink alias (output→source)")
	}
}

func TestRevalidateDetectsPostConstructionSymlinkAlias(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "output")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err != nil {
		t.Fatalf("NewWorkspaceLayoutGuard() = %v", err)
	}
	if err := guard.Revalidate(); err != nil {
		t.Fatalf("Revalidate() = %v before mutation", err)
	}
	// Replace output with a symlink to source.
	if err := os.RemoveAll(output); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, output); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := guard.Revalidate(); err == nil {
		t.Fatal("Revalidate() accepted post-construction symlink alias")
	}
}

func TestRevalidateDetectsSourceAliasSwap(t *testing.T) {
	ws := t.TempDir()
	realSource := filepath.Join(ws, "real-source")
	sourceAlias := filepath.Join(ws, "source")
	output := filepath.Join(ws, "output")
	for _, directory := range []string{realSource, output} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(realSource, sourceAlias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	guard, err := NewWorkspaceLayoutGuard(ws, sourceAlias, output)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourceAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(output, sourceAlias); err != nil {
		t.Fatal(err)
	}
	if err := guard.Revalidate(); err == nil {
		t.Fatal("Revalidate() accepted a swapped source alias")
	}
}

func TestRevalidateOutputRootRequiresGuardedPath(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "generated")
	guard, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{filepath.Join(ws, "other"), "relative/output"} {
		if err := guard.RevalidateOutputRoot(candidate); err == nil {
			t.Errorf("RevalidateOutputRoot(%q) accepted an unbound path", candidate)
		}
	}
	if err := guard.RevalidateOutputRoot(output); err != nil {
		t.Fatalf("RevalidateOutputRoot(%q) = %v", output, err)
	}
}

func TestRevalidateArchiveDestinationRejectsWorkspaceOverlap(t *testing.T) {
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "generated")
	for _, directory := range []string{source, output} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	guard, err := NewWorkspaceLayoutGuard(ws, source, output)
	if err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{source, filepath.Join(source, "release"), output, filepath.Join(output, "release"), ws} {
		if err := guard.RevalidateArchiveDestination(destination); err == nil {
			t.Errorf("RevalidateArchiveDestination(%q) accepted an overlapping path", destination)
		}
	}
	if destination := filepath.Join(ws, "release"); guard.RevalidateArchiveDestination(destination) != nil {
		t.Fatalf("RevalidateArchiveDestination(%q) rejected a disjoint path", destination)
	}
}

func TestZeroValueGuardIsRejectedByRevalidate(t *testing.T) {
	var guard WorkspaceLayoutGuard
	if err := guard.Revalidate(); err == nil {
		t.Fatal("Revalidate() accepted zero-value guard")
	}
}

func TestNewWorkspaceLayoutGuardRequiresAbsolutePaths(t *testing.T) {
	ws := t.TempDir()
	for _, test := range []struct {
		name                    string
		workspaceRoot, src, out string
	}{
		{name: "relative workspace", workspaceRoot: "relative", src: filepath.Join(ws, "s"), out: filepath.Join(ws, "o")},
		{name: "relative source", workspaceRoot: ws, src: "relative/source", out: filepath.Join(ws, "o")},
		{name: "relative output", workspaceRoot: ws, src: filepath.Join(ws, "s"), out: "relative/output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewWorkspaceLayoutGuard(test.workspaceRoot, test.src, test.out); err == nil {
				t.Fatal("NewWorkspaceLayoutGuard() accepted non-absolute path")
			}
		})
	}
}

func TestCheckDisjointPreventsDirectoryPrefixFalseMatch(t *testing.T) {
	// "source" and "sourcefoo" share a prefix but are disjoint.
	ws := t.TempDir()
	source := filepath.Join(ws, "source")
	output := filepath.Join(ws, "sourcefoo")
	if _, err := NewWorkspaceLayoutGuard(ws, source, output); err != nil {
		t.Fatalf("NewWorkspaceLayoutGuard() = %v for disjoint prefix-sharing paths: %v", source, err)
	}
}
