// Package archive writes deterministic target-root release archives from plan bytes.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// WriteTargetRoots archives each target's planned files using the target's
// declared ArchiveUnits. Files are read from PlannedFile.Bytes; no filesystem
// walk is performed. Each archive is written atomically: a .tmp file is renamed
// over the destination only if bytes differ from an existing archive.
func WriteTargetRoots(distribution model.DistributionMetadata, plan model.BuildPlan, output string) ([]string, error) {
	if output == "" {
		return nil, fmt.Errorf("archive output directory is required")
	}
	name, ok := distribution["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("distribution.name is required for release archives")
	}
	if err := validateArchiveName(name); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("create archive output: %w", err)
	}

	var paths []string
	for _, targetPlan := range plan.Targets {
		for _, unit := range targetPlan.ArchiveUnits {
			if err := validateArchiveName(unit.Stem); err != nil {
				return nil, fmt.Errorf("archive unit for target %q: stem %q: %w", targetPlan.Target, unit.Stem, err)
			}
			basename := name + "-" + unit.Stem + unit.Suffix
			archivePath := filepath.Join(output, basename)
			// Verify the computed path stays within the requested output directory.
			rel, err := filepath.Rel(output, archivePath)
			if err != nil || strings.HasPrefix(rel, "..") {
				return nil, fmt.Errorf("archive name %q escapes the output directory", basename)
			}
			// Collect files for this unit.
			files := filterFiles(targetPlan.Files, unit.Root)
			if len(files) == 0 {
				return nil, fmt.Errorf("target %q archive unit root %q has no files", targetPlan.Target, unit.Root)
			}
			if err := writeTarGzipFromPlan(archivePath, unit.Root, files); err != nil {
				return nil, fmt.Errorf("archive target %q unit %q: %w", targetPlan.Target, unit.Root, err)
			}
			paths = append(paths, archivePath)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// filterFiles returns the subset of files whose path starts with root.
// If root is ".", all files are included. Otherwise only files with the
// root+"/" prefix are included.
func filterFiles(files []model.PlannedFile, root string) []model.PlannedFile {
	if root == "." {
		return append([]model.PlannedFile(nil), files...)
	}
	prefix := root + "/"
	var result []model.PlannedFile
	for _, f := range files {
		if strings.HasPrefix(string(f.Path), prefix) {
			result = append(result, f)
		}
	}
	return result
}

// validateArchiveName checks that name is a safe filename component for use in
// archive paths. It must be non-empty, contain no path separators or null
// bytes, and not use platform-reserved device names.
func validateArchiveName(name string) error {
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("distribution name %q contains a path separator", name)
	}
	if strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("distribution name %q contains a null byte", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("distribution name %q is a reserved path component", name)
	}
	// Reject Windows reserved device names for cross-platform portability.
	upper := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return fmt.Errorf("distribution name %q uses a reserved device name", name)
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		if upper[3] >= '1' && upper[3] <= '9' {
			return fmt.Errorf("distribution name %q uses a reserved device name", name)
		}
	}
	return nil
}

// writeTarGzipFromPlan writes a deterministic tar.gz archive from plan bytes.
// Files are sorted by path and written with zeroed timestamps. The root prefix
// is stripped from each file's archive name when root is not ".".
func writeTarGzipFromPlan(destination, root string, files []model.PlannedFile) (err error) {
	// Sort for determinism.
	sortedFiles := append([]model.PlannedFile(nil), files...)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	temporary := destination + ".tmp"
	defer func() {
		if err != nil {
			_ = os.Remove(temporary)
		}
	}()

	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzipWriter, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.ModTime = time.Unix(0, 0)
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	for _, pf := range sortedFiles {
		// Strip the root prefix to get the in-archive path.
		archiveName := string(pf.Path)
		if root != "." {
			archiveName = strings.TrimPrefix(archiveName, root+"/")
		}
		mode := int64(0o644)
		if pf.Executable {
			mode = 0o755
		}
		header := &tar.Header{
			Name:    archiveName,
			Mode:    mode,
			Size:    int64(len(pf.Bytes)),
			ModTime: time.Unix(0, 0),
			Format:  tar.FormatUSTAR,
		}
		if writeErr := tarWriter.WriteHeader(header); writeErr != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return writeErr
		}
		if _, writeErr := tarWriter.Write(pf.Bytes); writeErr != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return writeErr
		}
	}

	if closeErr := tarWriter.Close(); closeErr != nil {
		_ = gzipWriter.Close()
		return closeErr
	}
	if closeErr := gzipWriter.Close(); closeErr != nil {
		return closeErr
	}
	if closeErr := file.Close(); closeErr != nil {
		return closeErr
	}

	unchanged, err := sameFileContents(temporary, destination)
	if err != nil {
		return err
	}
	if unchanged {
		return os.Remove(temporary)
	}
	return os.Rename(temporary, destination)
}

func sameFileContents(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Lstat(right)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if rightInfo.Mode()&os.ModeSymlink != 0 || !rightInfo.Mode().IsRegular() {
		return false, nil
	}
	if leftInfo.Size() != rightInfo.Size() {
		return false, nil
	}
	leftFile, err := os.Open(left)
	if err != nil {
		return false, err
	}
	defer func() { _ = leftFile.Close() }()
	rightFile, err := os.Open(right)
	if err != nil {
		return false, err
	}
	defer func() { _ = rightFile.Close() }()
	leftHash := sha256.New()
	if _, err := io.Copy(leftHash, leftFile); err != nil {
		return false, err
	}
	rightHash := sha256.New()
	if _, err := io.Copy(rightHash, rightFile); err != nil {
		return false, err
	}
	return string(leftHash.Sum(nil)) == string(rightHash.Sum(nil)), nil
}
