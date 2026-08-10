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

// Destination binds an archive output path to its longest existing ancestor.
// Missing descendants are created only after the artifact facade validates the
// bound canonical path against its workspace guard.
type Destination struct {
	path          string
	canonicalPath string
	root          *os.Root
	missing       []string
	created       bool
}

// writeTargetRoots is a test-only convenience wrapper. Production callers must
// validate the workspace layout between OpenDestination and Create, then pass
// the same bound Destination to WriteTargetRootsInDestination.
func writeTargetRoots(distribution model.DistributionMetadata, plan model.BuildPlan, output string) ([]string, error) {
	name, err := distributionName(distribution)
	if err != nil {
		return nil, err
	}
	writes, err := prepareArchiveWrites(name, plan, output)
	if err != nil {
		return nil, err
	}
	destination, err := OpenDestination(output)
	if err != nil {
		return nil, err
	}
	defer func() { _ = destination.Close() }()
	if err := destination.Create(); err != nil {
		return nil, err
	}
	return writePreparedArchives(writes, destination)
}

// OpenDestination pins the longest existing ancestor of output without
// creating anything. Its canonical path is tied to the pinned directory
// identity, so a pathname swap cannot retarget later descendant creation.
func OpenDestination(output string) (*Destination, error) {
	if output == "" {
		return nil, fmt.Errorf("archive output directory is required")
	}
	if !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return nil, fmt.Errorf("archive output directory must be an absolute cleaned path")
	}

	existing, missing, err := longestExistingDirectory(output)
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return nil, fmt.Errorf("resolve archive output parent: %w", err)
	}
	canonical = filepath.Clean(canonical)
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, fmt.Errorf("pin archive output parent: %w", err)
	}
	if err := verifyPinnedDirectory(root, existing, canonical); err != nil {
		_ = root.Close()
		return nil, err
	}

	return &Destination{
		path:          output,
		canonicalPath: filepath.Join(append([]string{canonical}, missing...)...),
		root:          root,
		missing:       missing,
	}, nil
}

// CanonicalPath returns the destination path below its pinned existing parent.
// The path is valid only while the Destination remains open.
func (d *Destination) CanonicalPath() string {
	if d == nil {
		return ""
	}
	return d.canonicalPath
}

// Create creates missing destination components descriptor-relative. Each
// component is opened and identity-checked without accepting symbolic links.
func (d *Destination) Create() error {
	if d == nil || d.root == nil {
		return fmt.Errorf("archive destination is required")
	}
	if d.created {
		return d.verifyIdentity()
	}

	current := d.root
	opened := make([]*os.Root, 0, len(d.missing))
	defer func() {
		for _, root := range opened {
			if root != d.root {
				_ = root.Close()
			}
		}
	}()
	for _, component := range d.missing {
		info, err := current.Lstat(component)
		if errors.Is(err, os.ErrNotExist) {
			if err := current.Mkdir(component, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create archive output component %q: %w", component, err)
			}
			info, err = current.Lstat(component)
		}
		if err != nil {
			return fmt.Errorf("inspect archive output component %q: %w", component, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("archive output component %q must be a directory, not a symbolic link or file", component)
		}

		next, err := current.OpenRoot(component)
		if err != nil {
			return fmt.Errorf("open archive output component %q: %w", component, err)
		}
		opened = append(opened, next)
		currentInfo, err := current.Lstat(component)
		if err != nil || currentInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive output component %q changed while it was being opened", component)
		}
		nextInfo, err := next.Stat(".")
		if err != nil || !os.SameFile(currentInfo, nextInfo) {
			return fmt.Errorf("archive output component %q changed while it was being opened", component)
		}
		current = next
	}

	if len(opened) != 0 {
		oldRoot := d.root
		d.root = opened[len(opened)-1]
		_ = oldRoot.Close()
	}
	d.missing = nil
	d.created = true
	return d.verifyIdentity()
}

// Close releases the pinned destination handle.
func (d *Destination) Close() error {
	if d == nil || d.root == nil {
		return nil
	}
	err := d.root.Close()
	d.root = nil
	return err
}

// WriteTargetRootsInDestination writes archives below an already validated and
// created Destination. All filesystem operations use its pinned handle.
func WriteTargetRootsInDestination(distribution model.DistributionMetadata, plan model.BuildPlan, destination *Destination) ([]string, error) {
	if destination == nil || destination.root == nil || !destination.created {
		return nil, fmt.Errorf("created archive destination is required")
	}
	name, err := distributionName(distribution)
	if err != nil {
		return nil, err
	}
	writes, err := prepareArchiveWrites(name, plan, destination.path)
	if err != nil {
		return nil, err
	}
	return writePreparedArchives(writes, destination)
}

func longestExistingDirectory(output string) (string, []string, error) {
	candidate := output
	var reversed []string
	for {
		info, err := os.Lstat(candidate)
		if err == nil {
			if candidate == output && info.Mode()&os.ModeSymlink != 0 {
				return "", nil, fmt.Errorf("archive output directory must not be a symbolic link")
			}
			targetInfo, statErr := os.Stat(candidate)
			if statErr != nil {
				return "", nil, fmt.Errorf("stat archive output parent: %w", statErr)
			}
			if !targetInfo.IsDir() {
				return "", nil, fmt.Errorf("archive output parent %q is not a directory", candidate)
			}
			missing := make([]string, len(reversed))
			for index := range reversed {
				missing[len(reversed)-1-index] = reversed[index]
			}
			return candidate, missing, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", nil, fmt.Errorf("inspect archive output parent: %w", err)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", nil, fmt.Errorf("find existing archive output parent: %w", err)
		}
		reversed = append(reversed, filepath.Base(candidate))
		candidate = parent
	}
}

func verifyPinnedDirectory(root *os.Root, original, canonical string) error {
	originalInfo, err := os.Stat(original)
	if err != nil {
		return fmt.Errorf("stat archive output parent: %w", err)
	}
	canonicalInfo, err := os.Stat(canonical)
	if err != nil {
		return fmt.Errorf("stat canonical archive output parent: %w", err)
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("stat pinned archive output parent: %w", err)
	}
	if !os.SameFile(originalInfo, rootInfo) || !os.SameFile(canonicalInfo, rootInfo) {
		return fmt.Errorf("archive output parent changed while it was being pinned")
	}
	return nil
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

func writePreparedArchives(writes []archiveWrite, destination *Destination) ([]string, error) {
	paths := make([]string, 0, len(writes))
	for _, write := range writes {
		if err := writeTarGzipFromPlan(destination, filepath.Base(write.path), write.unit.Root, write.files); err != nil {
			return nil, fmt.Errorf("archive target %q unit %q: %w", write.target, write.unit.Root, err)
		}
		paths = append(paths, write.path)
	}
	sort.Strings(paths)
	return paths, nil
}

func (d *Destination) verifyIdentity() error {
	pathInfo, err := os.Stat(d.path)
	if err != nil {
		return fmt.Errorf("stat archive output: %w", err)
	}
	rootInfo, err := d.root.Stat(".")
	if err != nil {
		return fmt.Errorf("stat pinned archive output: %w", err)
	}
	if !os.SameFile(pathInfo, rootInfo) {
		return fmt.Errorf("archive output directory changed after it was pinned")
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
func writeTarGzipFromPlan(destinationRoot *Destination, destination, root string, files []model.PlannedFile) (err error) {
	if err := destinationRoot.verifyIdentity(); err != nil {
		return err
	}
	rootHandle := destinationRoot.root
	// Sort for determinism.
	sortedFiles := append([]model.PlannedFile(nil), files...)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	// Create a unique temporary file relative to the pinned destination. O_EXCL
	// prevents a pre-existing symlink from being followed.
	temporary, err := temporaryName()
	if err != nil {
		return err
	}
	file, err := rootHandle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = rootHandle.Remove(temporary)
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
	if err := destinationRoot.verifyIdentity(); err != nil {
		return err
	}
	unchanged, err := sameFileContents(rootHandle, temporary, destination)
	if err != nil {
		return err
	}
	if unchanged {
		return rootHandle.Remove(temporary)
	}
	return rootHandle.Rename(temporary, destination)
}

func temporaryName() (string, error) {
	var random [16]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary archive name: %w", err)
	}
	return ".agbun-tmp-" + hex.EncodeToString(random[:]), nil
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
