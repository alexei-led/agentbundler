package compatibility

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Compare reports exact drift for owned or merged compatibility files without writing.
func Compare(plan Plan, workspace string) ([]model.Diagnostic, bool) {
	var diagnostics []model.Diagnostic
	for _, file := range plan.Files {
		full := filepath.Join(workspace, filepath.FromSlash(string(file.Path)))
		if err := rejectSymlinkComponents(workspace, full); err != nil {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_CHANGED", fmt.Sprintf("repository compatibility changed: %s (%v)", file.Path, err)))
			continue
		}
		info, err := os.Lstat(full)
		if errors.Is(err, os.ErrNotExist) {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_MISSING", "repository compatibility missing: "+string(file.Path)))
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_CHANGED", "repository compatibility changed: "+string(file.Path)))
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil || !bytes.Equal(data, file.Bytes) {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_CHANGED", "repository compatibility changed: "+string(file.Path)))
		}
	}
	for _, file := range plan.Remove {
		full := filepath.Join(workspace, filepath.FromSlash(string(file)))
		if err := rejectSymlinkComponents(workspace, full); err != nil {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_CHANGED", fmt.Sprintf("repository compatibility changed: %s (%v)", file, err)))
			continue
		}
		if _, err := os.Lstat(full); err == nil {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_EXTRA", "repository compatibility stale: "+string(file)))
		} else if !errors.Is(err, os.ErrNotExist) {
			diagnostics = append(diagnostics, diagnostic("COMPATIBILITY_DRIFT_CHANGED", fmt.Sprintf("repository compatibility changed: %s (%v)", file, err)))
		}
	}
	sort.Slice(diagnostics, func(left, right int) bool {
		if diagnostics[left].Message == diagnostics[right].Message {
			return diagnostics[left].Code < diagnostics[right].Code
		}
		return diagnostics[left].Message < diagnostics[right].Message
	})
	return diagnostics, len(diagnostics) != 0
}

// Write applies the prepared root plan. Fully owned stale files are removed;
// package.json is always merged before this function receives it.
func Write(plan Plan, workspace string) []model.Diagnostic {
	files := append([]File(nil), plan.Files...)
	sort.Slice(files, func(left, right int) bool {
		if files[left].Path == statePath {
			return false
		}
		if files[right].Path == statePath {
			return true
		}
		return files[left].Path < files[right].Path
	})
	for _, file := range files {
		if err := writeRootFile(workspace, file); err != nil {
			return []model.Diagnostic{diagnostic("COMPATIBILITY_WRITE_FAILED", fmt.Sprintf("write repository compatibility %s: %v", file.Path, err))}
		}
	}
	remove := append([]model.RelativePath(nil), plan.Remove...)
	sort.Slice(remove, func(left, right int) bool {
		if remove[left] == statePath {
			return false
		}
		if remove[right] == statePath {
			return true
		}
		return remove[left] < remove[right]
	})
	for _, file := range remove {
		if err := removeRootFile(workspace, file); err != nil {
			return []model.Diagnostic{diagnostic("COMPATIBILITY_WRITE_FAILED", fmt.Sprintf("remove stale repository compatibility %s: %v", file, err))}
		}
	}
	return nil
}

func writeRootFile(workspace string, file File) error {
	if _, err := model.NewRelativePath(string(file.Path)); err != nil {
		return err
	}
	full := filepath.Join(workspace, filepath.FromSlash(string(file.Path)))
	if err := rejectSymlinkComponents(workspace, full); err != nil {
		return err
	}
	if info, err := os.Lstat(full); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destination is not a regular file")
		}
		current, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if bytes.Equal(current, file.Bytes) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(full)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(workspace, parent); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".agbun-compatibility-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(file.Bytes); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, full)
}

func removeRootFile(workspace string, relative model.RelativePath) error {
	if _, err := model.NewRelativePath(string(relative)); err != nil {
		return err
	}
	full := filepath.Join(workspace, filepath.FromSlash(string(relative)))
	if err := rejectSymlinkComponents(workspace, full); err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("owned path is not a regular file")
	}
	if err := os.Remove(full); err != nil {
		return err
	}
	for directory := filepath.Dir(full); directory != workspace && directory != filepath.Dir(directory); directory = filepath.Dir(directory) {
		if err := os.Remove(directory); err != nil {
			if errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "not empty") || strings.Contains(strings.ToLower(err.Error()), "directory not empty") {
				break
			}
			break
		}
	}
	return nil
}

func rejectSymlinkComponents(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	current := root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains a symbolic link")
		}
	}
	return nil
}
