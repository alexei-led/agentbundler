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
	sourcePath      string // original source alias used by source import
	outputPath      string // original output alias used by artifact operations
	sourceCanonical string // canonical source root at construction
	outputCanonical string // canonical output root at construction; may not yet exist
	valid           bool   // false for the zero value
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
		sourcePath:      sourceRoot,
		outputPath:      outputRoot,
		sourceCanonical: canonSource,
		outputCanonical: canonOutput,
		valid:           true,
	}, nil
}

// Revalidate re-resolves the original source and output aliases, verifies that
// neither canonical root changed, and confirms that they remain disjoint.
func (g WorkspaceLayoutGuard) Revalidate() error {
	_, _, err := g.revalidateRoots()
	return err
}

// RevalidateOutputRoot verifies that output is the exact path bound to this
// guard, then revalidates both guarded roots.
func (g WorkspaceLayoutGuard) RevalidateOutputRoot(output string) error {
	_, canonOutput, err := g.revalidateRoots()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return fmt.Errorf("output root must be an absolute cleaned path")
	}
	if output != g.outputPath {
		return fmt.Errorf("output root %q does not match guarded output root %q", output, g.outputPath)
	}
	if canonPathOrTextual(output) != canonOutput {
		return fmt.Errorf("output root %q changed after workspace layout validation", output)
	}
	return nil
}

// RevalidateArchiveDestination verifies the guarded roots and rejects an
// archive directory that overlaps either source or generated output.
func (g WorkspaceLayoutGuard) RevalidateArchiveDestination(destination string) error {
	canonSource, canonOutput, err := g.revalidateRoots()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return fmt.Errorf("archive destination must be an absolute cleaned path")
	}
	canonDestination := canonPathOrTextual(destination)
	if err := checkDisjoint(canonSource, canonDestination); err != nil {
		return fmt.Errorf("archive destination overlaps source root: %w", err)
	}
	if err := checkDisjoint(canonOutput, canonDestination); err != nil {
		return fmt.Errorf("archive destination overlaps output root: %w", err)
	}
	return nil
}

func (g WorkspaceLayoutGuard) revalidateRoots() (string, string, error) {
	if !g.valid {
		return "", "", fmt.Errorf("workspace layout guard was not constructed with NewWorkspaceLayoutGuard")
	}
	canonSource := canonPathOrTextual(g.sourcePath)
	canonOutput := canonPathOrTextual(g.outputPath)
	if canonSource != g.sourceCanonical {
		return "", "", fmt.Errorf("source root %q changed after workspace layout validation", g.sourcePath)
	}
	if canonOutput != g.outputCanonical {
		return "", "", fmt.Errorf("output root %q changed after workspace layout validation", g.outputPath)
	}
	if err := checkDisjoint(canonSource, canonOutput); err != nil {
		return "", "", err
	}
	return canonSource, canonOutput, nil
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
