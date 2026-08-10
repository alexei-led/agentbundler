// Package artifact validates and applies complete generated-output plans.
package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

	canonSource, err := canonPathOrTextual(sourceRoot)
	if err != nil {
		return WorkspaceLayoutGuard{}, err
	}
	canonOutput, err := canonPathOrTextual(outputRoot)
	if err != nil {
		return WorkspaceLayoutGuard{}, err
	}

	if err := checkDisjoint(canonSource, canonOutput); err != nil {
		return WorkspaceLayoutGuard{}, err
	}
	if err := checkExistingIdentity(sourceRoot, outputRoot); err != nil {
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
	current, err := canonPathOrTextual(output)
	if err != nil {
		return err
	}
	if !samePath(current, canonOutput) {
		return fmt.Errorf("output root %q changed after workspace layout validation", output)
	}
	return nil
}

// RevalidateArchiveDestination verifies the guarded roots and rejects an
// archive directory that overlaps either source or generated output.
func (g WorkspaceLayoutGuard) RevalidateArchiveDestination(destination string) error {
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return fmt.Errorf("archive destination must be an absolute cleaned path")
	}
	canonical, err := canonPathOrTextual(destination)
	if err != nil {
		return err
	}
	return g.revalidateBoundArchiveDestination(destination, canonical)
}

// revalidateBoundArchiveDestination validates the canonical path derived from
// a pinned existing parent. It deliberately does not resolve destination again:
// later creation remains bound to the same parent descriptor.
func (g WorkspaceLayoutGuard) revalidateBoundArchiveDestination(destination, canonical string) error {
	canonSource, canonOutput, err := g.revalidateRoots()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return fmt.Errorf("archive destination must be an absolute cleaned path")
	}
	if !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		return fmt.Errorf("canonical archive destination must be an absolute cleaned path")
	}
	if err := checkDisjoint(canonSource, canonical); err != nil {
		return fmt.Errorf("archive destination overlaps source root: %w", err)
	}
	if err := checkDisjoint(canonOutput, canonical); err != nil {
		return fmt.Errorf("archive destination overlaps output root: %w", err)
	}
	return nil
}

func (g WorkspaceLayoutGuard) revalidateRoots() (string, string, error) {
	if !g.valid {
		return "", "", fmt.Errorf("workspace layout guard was not constructed with NewWorkspaceLayoutGuard")
	}
	canonSource, err := canonPathOrTextual(g.sourcePath)
	if err != nil {
		return "", "", err
	}
	canonOutput, err := canonPathOrTextual(g.outputPath)
	if err != nil {
		return "", "", err
	}
	if !samePath(canonSource, g.sourceCanonical) {
		return "", "", fmt.Errorf("source root %q changed after workspace layout validation", g.sourcePath)
	}
	if !samePath(canonOutput, g.outputCanonical) {
		return "", "", fmt.Errorf("output root %q changed after workspace layout validation", g.outputPath)
	}
	if err := checkDisjoint(canonSource, canonOutput); err != nil {
		return "", "", err
	}
	if err := checkExistingIdentity(g.sourcePath, g.outputPath); err != nil {
		return "", "", err
	}
	return canonSource, canonOutput, nil
}

// checkDisjoint returns an error if source and output are equal or one is a
// parent of the other. Comparisons are folded on platforms whose path lookup
// is case-insensitive; existing roots are additionally checked by identity.
func checkDisjoint(source, output string) error {
	sep := string(filepath.Separator)
	equal := samePath(source, output)
	inside := func(child, parent string) bool {
		if samePath(child, parent) {
			return true
		}
		if caseInsensitivePaths() {
			return strings.HasPrefix(strings.ToLower(child), strings.ToLower(parent+sep))
		}
		return strings.HasPrefix(child, parent+sep)
	}
	switch {
	case equal:
		return fmt.Errorf("source root %q and output root %q are the same directory", source, output)
	case inside(output, source):
		return fmt.Errorf("output root %q is inside source root %q", output, source)
	case inside(source, output):
		return fmt.Errorf("source root %q is inside output root %q", source, output)
	}
	return nil
}

func samePath(left, right string) bool {
	if left == right || (caseInsensitivePaths() && strings.EqualFold(left, right)) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

func checkExistingIdentity(left, right string) error {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if errors.Is(leftErr, fs.ErrNotExist) || errors.Is(rightErr, fs.ErrNotExist) {
		return nil
	}
	if leftErr != nil {
		return fmt.Errorf("resolve path %q: %w", left, leftErr)
	}
	if rightErr != nil {
		return fmt.Errorf("resolve path %q: %w", right, rightErr)
	}
	if os.SameFile(leftInfo, rightInfo) {
		return fmt.Errorf("source root %q and output root %q identify the same directory", left, right)
	}
	return nil
}

func caseInsensitivePaths() bool {
	// Windows filesystems and the default macOS volumes fold path case. Folding
	// is conservative on case-sensitive macOS volumes: rejecting a possible
	// overlap is safer than allowing destructive output replacement.
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// canonPathOrTextual resolves symlinks in path. Missing paths use the longest
// canonicalized existing prefix plus their remaining textual suffix. Resolution
// failures other than fs.ErrNotExist are returned so layout validation fails
// closed on permission errors and symlink cycles.
func canonPathOrTextual(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved), nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	parent := filepath.Dir(path)
	if parent == path {
		return filepath.Clean(path), nil
	}
	canonicalParent, err := canonPathOrTextual(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(canonicalParent, filepath.Base(path)), nil
}
