// Package archive writes deterministic target-root release archives from plan bytes.
package archive

import (
	"archive/tar"
	"compress/gzip"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// writeTargetRoots is a test-only convenience wrapper. Production callers must
// validate the workspace layout and use OpenDestination plus
// WriteTargetRootsInRoot so archive creation is pinned to a guarded directory.
func writeTargetRoots(distribution model.DistributionMetadata, plan model.BuildPlan, output string) ([]string, error) {
	name, err := distributionName(distribution)
	if err != nil {
		return nil, err
	}
	writes, err := prepareArchiveWrites(name, plan, output)
	if err != nil {
		return nil, err
	}
	root, err := OpenDestination(output)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return writePreparedArchives(name, writes, output, root)
}

// OpenDestination creates and pins an archive destination directory. The
// returned root keeps later operations on the directory descriptor-relative,
// even if the destination pathname is replaced.
func OpenDestination(output string) (*os.Root, error) {
	if output == "" {
		return nil, fmt.Errorf("archive output directory is required")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("create archive output: %w", err)
	}
	pathInfo, err := os.Lstat(output)
	if err != nil {
		return nil, fmt.Errorf("lstat archive output: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("archive output directory must not be a symbolic link")
	}
	root, err := os.OpenRoot(output)
	if err != nil {
		return nil, fmt.Errorf("open archive output: %w", err)
	}
	if err := verifyDestinationIdentity(root, output); err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}

// WriteTargetRootsInRoot writes archives below the already pinned destination.
// output is retained only for returned display paths and identity checks.
func WriteTargetRootsInRoot(distribution model.DistributionMetadata, plan model.BuildPlan, output string, destinationRoot *os.Root) ([]string, error) {
	if output == "" {
		return nil, fmt.Errorf("archive output directory is required")
	}
	if destinationRoot == nil {
		return nil, fmt.Errorf("archive destination root is required")
	}
	name, err := distributionName(distribution)
	if err != nil {
		return nil, err
	}
	writes, err := prepareArchiveWrites(name, plan, output)
	if err != nil {
		return nil, err
	}
	return writePreparedArchives(name, writes, output, destinationRoot)
}

func distributionName(distribution model.DistributionMetadata) (string, error) {
	name, ok := distribution["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("distribution.name is required for release archives")
	}
	if err := validateArchiveName(name); err != nil {
		return "", err
	}
	return name, nil
}

func writePreparedArchives(name string, writes []archiveWrite, output string, destinationRoot *os.Root) ([]string, error) {
	paths := make([]string, 0, len(writes))
	for _, write := range writes {
		if err := writeTarGzipFromPlan(destinationRoot, output, filepath.Base(write.path), write.unit.Root, write.files); err != nil {
			return nil, fmt.Errorf("archive target %q unit %q: %w", write.target, write.unit.Root, err)
		}
		paths = append(paths, write.path)
	}
	sort.Strings(paths)
	return paths, nil
}

func verifyDestinationIdentity(root *os.Root, output string) error {
	pathInfo, err := os.Stat(output)
	if err != nil {
		return fmt.Errorf("stat archive output: %w", err)
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("stat pinned archive output: %w", err)
	}
	if !os.SameFile(pathInfo, rootInfo) {
		return fmt.Errorf("archive output directory changed while it was being opened")
	}
	return nil
}

type archiveWrite struct {
	target model.TargetID
	unit   model.ArchiveUnit
	path   string
	files  []model.PlannedFile
}

func prepareArchiveWrites(name string, plan model.BuildPlan, output string) ([]archiveWrite, error) {
	destinations := make(map[string]struct{})
	var writes []archiveWrite
	for _, targetPlan := range plan.Targets {
		if len(targetPlan.Files) != 0 && len(targetPlan.ArchiveUnits) == 0 {
			return nil, fmt.Errorf("target %q has files but no archive units", targetPlan.Target)
		}
		coverage := make([]int, len(targetPlan.Files))
		for _, unit := range targetPlan.ArchiveUnits {
			if unit.Root != "." {
				if _, err := model.NewRelativePath(unit.Root); err != nil {
					return nil, fmt.Errorf("archive unit for target %q: root %q: %w", targetPlan.Target, unit.Root, err)
				}
			}
			if err := validateArchiveName(unit.Stem); err != nil {
				return nil, fmt.Errorf("archive unit for target %q: stem %q: %w", targetPlan.Target, unit.Stem, err)
			}
			if unit.Suffix != ".tar.gz" && unit.Suffix != ".tgz" {
				return nil, fmt.Errorf("archive unit for target %q has invalid suffix %q", targetPlan.Target, unit.Suffix)
			}
			basename := name + "-" + unit.Stem + unit.Suffix
			archivePath := filepath.Join(output, basename)
			rel, err := filepath.Rel(output, archivePath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("archive name %q escapes the output directory", basename)
			}
			archivePathKey := strings.ToLower(archivePath)
			if _, exists := destinations[archivePathKey]; exists {
				return nil, fmt.Errorf("archive destination %q is duplicated", archivePath)
			}
			destinations[archivePathKey] = struct{}{}

			files := filterFiles(targetPlan.Files, unit.Root)
			if len(files) == 0 {
				return nil, fmt.Errorf("target %q archive unit root %q has no files", targetPlan.Target, unit.Root)
			}
			for index, file := range targetPlan.Files {
				if archiveUnitContains(unit.Root, string(file.Path)) {
					coverage[index]++
				}
			}
			writes = append(writes, archiveWrite{target: targetPlan.Target, unit: unit, path: archivePath, files: files})
		}
		for index, count := range coverage {
			switch {
			case count == 0:
				return nil, fmt.Errorf("target %q planned file %q is not covered by an archive unit", targetPlan.Target, targetPlan.Files[index].Path)
			case count > 1:
				return nil, fmt.Errorf("target %q planned file %q is covered by multiple archive units", targetPlan.Target, targetPlan.Files[index].Path)
			}
		}
	}
	return writes, nil
}

// filterFiles returns the subset of files whose path starts with root.
// If root is ".", all files are included. Otherwise only files with the
// root+"/" prefix are included.
func filterFiles(files []model.PlannedFile, root string) []model.PlannedFile {
	var result []model.PlannedFile
	for _, file := range files {
		if archiveUnitContains(root, string(file.Path)) {
			result = append(result, file)
		}
	}
	return result
}

func archiveUnitContains(root, filePath string) bool {
	return root == "." || strings.HasPrefix(filePath, root+"/")
}

// validateArchiveName checks that name is a safe cross-platform basename.
// Windows-invalid characters, controls, trailing dots/spaces, and reserved
// device names are rejected even when running on Unix.
func validateArchiveName(name string) error {
	if name == "" {
		return fmt.Errorf("archive name must not be empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("archive name %q contains a path separator", name)
	}
	if strings.ContainsAny(name, `:*?"<>|`) {
		return fmt.Errorf("archive name %q contains a Windows-invalid character", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("archive name %q contains a control character", name)
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("archive name %q has a trailing dot or space", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("archive name %q is a reserved path component", name)
	}
	// Reject Windows reserved device names for cross-platform portability.
	deviceBase := strings.SplitN(name, ".", 2)[0]
	upper := strings.ToUpper(strings.TrimRight(deviceBase, " ."))
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return fmt.Errorf("archive name %q uses a reserved device name", name)
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		if upper[3] >= '1' && upper[3] <= '9' {
			return fmt.Errorf("archive name %q uses a reserved device name", name)
		}
	}
	return nil
}

// writeTarGzipFromPlan writes a deterministic tar.gz archive from plan bytes.
// Files are sorted by path and written with zeroed timestamps. The root prefix
// is stripped from each file's archive name when root is not ".".
func writeTarGzipFromPlan(destinationRoot *os.Root, output, destination, root string, files []model.PlannedFile) (err error) {
	if err := verifyDestinationIdentity(destinationRoot, output); err != nil {
		return err
	}
	// Sort for determinism.
	sortedFiles := append([]model.PlannedFile(nil), files...)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	// Create a unique temporary file relative to the pinned destination. O_EXCL
	// prevents a pre-existing symlink from being followed.
	temporary, err := temporaryName(destination)
	if err != nil {
		return err
	}
	file, err := destinationRoot.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = destinationRoot.Remove(temporary)
		}
	}()
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
		format := tar.FormatUSTAR
		// USTAR's name and prefix fields cannot represent every valid imported
		// relative path. PAX carries the full path deterministically.
		if len([]byte(archiveName)) > 100 {
			format = tar.FormatPAX
		}
		header := &tar.Header{
			Name:    archiveName,
			Mode:    mode,
			Size:    int64(len(pf.Bytes)),
			ModTime: time.Unix(0, 0),
			Format:  format,
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

	// Check the pathname identity before committing, while all writes still use
	// the pinned root. A swapped pathname therefore fails closed rather than
	// redirecting the archive to another directory.
	if err := verifyDestinationIdentity(destinationRoot, output); err != nil {
		return err
	}
	unchanged, err := sameFileContents(destinationRoot, temporary, destination)
	if err != nil {
		return err
	}
	if unchanged {
		return destinationRoot.Remove(temporary)
	}
	return destinationRoot.Rename(temporary, destination)
}

func temporaryName(destination string) (string, error) {
	var random [16]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary archive name: %w", err)
	}
	return "." + destination + ".tmp-" + hex.EncodeToString(random[:]), nil
}

func sameFileContents(root *os.Root, left, right string) (bool, error) {
	leftInfo, err := root.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := root.Lstat(right)
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
	leftFile, err := root.Open(left)
	if err != nil {
		return false, err
	}
	defer func() { _ = leftFile.Close() }()
	rightFile, err := root.Open(right)
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
