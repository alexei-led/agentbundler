// Package artifact validates and applies complete generated-output plans.
package artifact

import (
	"fmt"
	"path/filepath"
	"strings"
)

// WorkspaceLayoutGuard proves that the workspace source and output roots are
// disjoint before source ingestion. Construct with NewWorkspaceLayoutGuard
// before any source import, then pass to every mutating artifact operation.
//
// The zero value is invalid and is rejected by all artifact operations.
// Callers must not compare, copy, or embed guard values across construction
// boundaries; always construct through NewWorkspaceLayoutGuard.
type WorkspaceLayoutGuard struct {
	source string // canonical source root (resolved at construction)
	output string // canonical output root (resolved at construction; may not yet exist)
	valid  bool   // false for the zero value
}

// NewWorkspaceLayoutGuard constructs and validates the layout guard.
// workspaceRoot, sourceRoot, and outputRoot must be absolute cleaned paths.
// sourceRoot is workspace + manifest.Root; outputRoot is workspace + manifest.Output.
// outputRoot need not exist on disk yet.
//
// Returns an error naming both conflicting paths when they are equal or when
// one is a parent of the other, including via symlink or junction aliases.
func NewWorkspaceLayoutGuard(workspaceRoot, sourceRoot, outputRoot string) (WorkspaceLayoutGuard, error) {
	for _, check := range []struct{ name, value string }{
		{"workspace root", workspaceRoot},
		{"source root", sourceRoot},
		{"output root", outputRoot},
	} {
		if !filepath.IsAbs(check.value) || filepath.Clean(check.value) != check.value {
			return WorkspaceLayoutGuard{}, fmt.Errorf("%s must be an absolute cleaned path", check.name)
		}
	}

	canonSource := canonPathOrTextual(sourceRoot)
	canonOutput := canonPathOrTextual(outputRoot)

	if err := checkDisjoint(canonSource, canonOutput); err != nil {
		return WorkspaceLayoutGuard{}, err
	}

	return WorkspaceLayoutGuard{
		source: canonSource,
		output: canonOutput,
		valid:  true,
	}, nil
}

// Revalidate re-checks that the physical source and output roots remain
// disjoint. Call before each mutating artifact operation to catch
// post-construction symlink or junction changes (TOCTOU protection).
func (g WorkspaceLayoutGuard) Revalidate() error {
	if !g.valid {
		return fmt.Errorf("workspace layout guard was not constructed with NewWorkspaceLayoutGuard")
	}
	canonSource := canonPathOrTextual(g.source)
	canonOutput := canonPathOrTextual(g.output)
	return checkDisjoint(canonSource, canonOutput)
}

// checkDisjoint returns an error if source and output are equal or one is a
// parent of the other. Separators are appended to prevent false matches
// between paths that share a common prefix but are not parent/child.
func checkDisjoint(source, output string) error {
	sep := string(filepath.Separator)
	switch {
	case source == output:
		return fmt.Errorf("source root %q and output root %q are the same directory", source, output)
	case strings.HasPrefix(output, source+sep):
		return fmt.Errorf("output root %q is inside source root %q", output, source)
	case strings.HasPrefix(source, output+sep):
		return fmt.Errorf("source root %q is inside output root %q", source, output)
	}
	return nil
}

// canonPathOrTextual resolves symlinks in path. When a component cannot be
// resolved (e.g., the path or an ancestor does not yet exist), it returns the
// longest canonicalized prefix joined with the remaining textual components.
// This allows the guard to validate paths for output directories that have not
// been created yet, while still catching existing symlink aliases.
func canonPathOrTextual(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	parent := filepath.Dir(path)
	if parent == path {
		// Filesystem root: cannot go higher.
		return filepath.Clean(path)
	}
	return filepath.Join(canonPathOrTextual(parent), filepath.Base(path))
}
