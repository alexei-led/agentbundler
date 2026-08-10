// Package agentplugin imports agent plugin packages using the pinned Agent
// Plugins 1.0.0 wire format. It is a pure source adapter: no composition,
// target, artifact, or network imports.
package agentplugin

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"
	"unicode/utf8"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// Traversal resource limits per plugin root.
const (
	maxEntries    = 10_000
	maxFileSizeB  = 64 << 20  // 64 MiB
	maxTotalBytes = 256 << 20 // 256 MiB
	maxDepth      = 64
	maxPathBytes  = 1024 // UTF-8 bytes per relative path
)

// traversalLimits bounds one plugin root traversal. The production limits are
// supplied by defaultTraversalLimits; tests use the helper with smaller limits
// to exercise each failure path without creating large fixtures.
type traversalLimits struct {
	maxEntries    int
	maxFileSizeB  int64
	maxTotalBytes int64
	maxDepth      int
	maxPathBytes  int
}

func defaultTraversalLimits() traversalLimits {
	return traversalLimits{
		maxEntries:    maxEntries,
		maxFileSizeB:  maxFileSizeB,
		maxTotalBytes: maxTotalBytes,
		maxDepth:      maxDepth,
		maxPathBytes:  maxPathBytes,
	}
}

// traversedFile is one regular file found during bounded plugin root traversal.
type traversedFile struct {
	// relPath is slash-separated, relative to the plugin root.
	relPath string
	// bytes is the file content.
	bytes []byte
	// executable reports whether the file has any execute permission bit set.
	executable bool
	// sha256 is the hex-encoded SHA-256 digest of bytes.
	sha256 string
}

// traversalState holds mutable counters during one plugin root walk.
type traversalState struct {
	pluginRoot *os.Root
	limits     traversalLimits
	entryCount int
	totalBytes int64
	files      []traversedFile
	diags      []model.Diagnostic
}

// traversePluginRoot walks all files under pluginRoot within the bounds
// defined by the traversal limits. It materializes contained symlinks and
// rejects external symlinks, cycles, and special files. Returned files are in
// lexical relPath order. On any quota or boundary violation, diagnostics
// contain an error and the returned files may be partial.
func traversePluginRoot(pluginRoot *os.Root) ([]traversedFile, []model.Diagnostic) {
	return traversePluginRootWithLimits(pluginRoot, defaultTraversalLimits())
}

func traversePluginRootWithLimits(pluginRoot *os.Root, limits traversalLimits) ([]traversedFile, []model.Diagnostic) {
	s := &traversalState{pluginRoot: pluginRoot, limits: limits}
	s.walk(".", 0, nil)
	return s.files, s.diags
}

// walk recurses into dirPath (slash-separated relative to pluginRoot),
// passing visited as the set of directories on the current ancestor stack.
// visited is passed by value so each child branch has its own copy.
func (s *traversalState) walk(dirPath string, depth int, visited []os.FileInfo) {
	if depth > s.limits.maxDepth {
		s.err(dirPath, "traversal depth limit exceeded")
		return
	}
	if s.hasErrors() {
		return
	}

	f, err := s.pluginRoot.Open(dirPath)
	if err != nil {
		s.err(dirPath, "open directory: "+err.Error())
		return
	}
	defer func() { _ = f.Close() }()

	// Real inode for cycle detection (follows symlinks for the final component).
	dirStat, err := f.Stat()
	if err != nil {
		s.err(dirPath, "stat directory: "+err.Error())
		return
	}
	for _, v := range visited {
		if os.SameFile(dirStat, v) {
			s.err(dirPath, "directory cycle detected via symlink")
			return
		}
	}
	visited = append(visited, dirStat)

	remainingEntries := s.limits.maxEntries - s.entryCount
	if remainingEntries < 0 {
		s.err(dirPath, "traversal entry limit exceeded")
		return
	}
	entries, err := f.ReadDir(remainingEntries + 1)
	if len(entries) > remainingEntries {
		s.err(dirPath, "traversal entry limit exceeded")
		return
	}
	if err != nil && err != io.EOF {
		s.err(dirPath, "read directory: "+err.Error())
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if s.hasErrors() {
			return
		}
		s.entryCount++
		if s.entryCount > s.limits.maxEntries {
			s.err(dirPath, "traversal entry limit exceeded")
			return
		}

		childPath := childRelPath(dirPath, entry.Name())
		if len([]byte(childPath)) > s.limits.maxPathBytes || !utf8.ValidString(childPath) {
			s.err(childPath, "path exceeds byte limit or contains invalid UTF-8")
			continue
		}

		// Lstat info for the entry (does not follow the final symlink).
		lInfo, err := entry.Info()
		if err != nil {
			s.err(childPath, "entry info: "+err.Error())
			continue
		}
		mode := lInfo.Mode()

		switch {
		case mode&os.ModeSymlink != 0:
			s.processSymlink(childPath, depth, visited)
		case mode.IsDir():
			s.walk(childPath, depth+1, visited)
		case mode.IsRegular():
			s.processFile(childPath)
		default:
			s.err(childPath, "special file not allowed in plugin root")
		}
	}
}

// processSymlink follows a symlink (os.Root rejects external ones) and either
// recurses into a directory target or materializes a regular file target.
func (s *traversalState) processSymlink(childPath string, depth int, visited []os.FileInfo) {
	// Open follows the symlink; os.Root returns an error for external links.
	target, err := s.pluginRoot.Open(childPath)
	if err != nil {
		s.err(childPath, "symlink target: "+err.Error())
		return
	}
	defer func() { _ = target.Close() }()

	targetStat, err := target.Stat()
	if err != nil {
		s.err(childPath, "symlink target stat: "+err.Error())
		return
	}

	switch {
	case targetStat.IsDir():
		// Recurse; cycle detection happens inside walk via dirStat check.
		s.walk(childPath, depth+1, visited)
	case targetStat.Mode().IsRegular():
		s.processFile(childPath)
	default:
		s.err(childPath, "symlink to special file not allowed")
	}
}

// processFile reads and records a regular file (or regular file via symlink).
// The opened descriptor's stat is authoritative for size, type, and mode.
func (s *traversalState) processFile(relPath string) {
	file, err := s.pluginRoot.Open(relPath)
	if err != nil {
		s.err(relPath, "open file: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		s.err(relPath, "stat file: "+err.Error())
		return
	}
	if !info.Mode().IsRegular() {
		s.err(relPath, "file changed to a non-regular file during traversal")
		return
	}
	if info.Size() > s.limits.maxFileSizeB {
		s.err(relPath, "file exceeds 64 MiB size limit")
		return
	}
	remaining := s.limits.maxTotalBytes - s.totalBytes
	if info.Size() > remaining {
		s.err(relPath, "total file bytes exceed 256 MiB limit")
		return
	}
	readLimit := s.limits.maxFileSizeB
	if remaining < readLimit {
		readLimit = remaining
	}
	content, err := io.ReadAll(io.LimitReader(file, readLimit+1))
	if err != nil {
		s.err(relPath, "read file: "+err.Error())
		return
	}
	if int64(len(content)) > s.limits.maxFileSizeB {
		s.err(relPath, "file exceeds 64 MiB size limit")
		return
	}
	if int64(len(content)) > remaining {
		s.err(relPath, "total file bytes exceed 256 MiB limit")
		return
	}
	s.totalBytes += int64(len(content))
	sum := sha256.Sum256(content)
	s.files = append(s.files, traversedFile{
		relPath:    relPath,
		bytes:      content,
		executable: info.Mode().Perm()&0o111 != 0,
		sha256:     hex.EncodeToString(sum[:]),
	})
}

// childRelPath builds a slash-separated child path.
func childRelPath(parent, name string) string {
	if parent == "." {
		return name
	}
	return parent + "/" + name
}

func (s *traversalState) err(relPath, message string) {
	diag := model.Diagnostic{
		Code:     diagnosticCode,
		Severity: model.SeverityError,
		Message:  message,
	}
	if relPath != "" {
		diag.Location = &model.SourceLocation{Path: model.RelativePath(relPath)}
	}
	s.diags = append(s.diags, diag)
}

func (s *traversalState) hasErrors() bool {
	for _, d := range s.diags {
		if d.Severity == model.SeverityError {
			return true
		}
	}
	return false
}
