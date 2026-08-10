// Package compare detects exact drift between a build plan and generated output.
package compare

import (
	"bytes"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// DriftKind classifies how generated output differs from a build plan.
type DriftKind string

const (
	// DriftMissing means a planned file is absent from generated output.
	DriftMissing DriftKind = "missing"
	// DriftChanged means a planned file differs from generated output.
	DriftChanged DriftKind = "changed"
	// DriftExtra means generated output has no planned file at that path.
	DriftExtra DriftKind = "extra"
)

// Drift identifies one generated-output path that differs from a build plan.
type Drift struct {
	Kind DriftKind
	Path model.RelativePath
}

type expectedFile struct {
	bytes      []byte
	executable bool
}

// DetectDrift compares plan with the generated output under outputRoot without modifying it.
func DetectDrift(plan model.BuildPlan, outputRoot string) ([]Drift, error) {
	return detectDriftInternal(plan, outputRoot, nil)
}

// DetectDriftInRoot compares output relative to a pinned parent directory.
// All reads are descriptor-relative, so a pathname swap cannot redirect the
// observation into another tree.
func DetectDriftInRoot(plan model.BuildPlan, outputName string, parent *os.Root) ([]Drift, error) {
	return detectDriftInternal(plan, outputName, parent)
}

func detectDriftInternal(plan model.BuildPlan, outputRoot string, parent *os.Root) ([]Drift, error) {
	expected := expectedFiles(plan)
	if runtime.GOOS == "windows" {
		for _, destination := range sortedExecutableDestinations(expected) {
			return []Drift{{Kind: DriftChanged, Path: model.RelativePath(destination)}}, nil
		}
	}

	var actual map[string]os.FileInfo
	var err error
	if parent == nil {
		actual, err = outputEntries(outputRoot)
	} else {
		actual, err = outputEntriesInRoot(parent, outputRoot)
	}
	if err != nil {
		return nil, err
	}
	requiredDirectories := requiredDirectories(expected)
	var drift []Drift

	for destination, info := range actual {
		if _, planned := expected[destination]; planned {
			continue
		}
		if info.IsDir() && requiredDirectories[destination] {
			continue
		}
		drift = append(drift, Drift{Kind: DriftExtra, Path: model.RelativePath(destination)})
	}

	for destination, planned := range expected {
		info, exists := actual[destination]
		if !exists {
			drift = append(drift, Drift{Kind: DriftMissing, Path: model.RelativePath(destination)})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			drift = append(drift, Drift{Kind: DriftChanged, Path: model.RelativePath(destination)})
			continue
		}

		var contents []byte
		if parent == nil {
			contents, err = os.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(destination)))
		} else {
			contents, err = parent.ReadFile(filepath.Join(outputRoot, filepath.FromSlash(destination)))
		}
		if err != nil || !bytes.Equal(contents, planned.bytes) || hasExecutableIntent(info) != planned.executable {
			drift = append(drift, Drift{Kind: DriftChanged, Path: model.RelativePath(destination)})
		}
	}

	sortDrift(drift)
	return drift, nil
}

func expectedFiles(plan model.BuildPlan) map[string]expectedFile {
	expected := make(map[string]expectedFile)
	for _, targetPlan := range plan.Targets {
		for _, file := range targetPlan.Files {
			expected[path.Join(string(targetPlan.Target), string(file.Path))] = expectedFile{
				bytes:      file.Bytes,
				executable: file.Executable,
			}
		}
	}
	for _, file := range plan.CompilerFiles {
		expected[string(file.Path)] = expectedFile{
			bytes:      file.Bytes,
			executable: file.Executable,
		}
	}
	return expected
}

func sortedExecutableDestinations(expected map[string]expectedFile) []string {
	var destinations []string
	for destination, file := range expected {
		if file.executable {
			destinations = append(destinations, destination)
		}
	}
	sort.Strings(destinations)
	return destinations
}

func outputEntries(outputRoot string) (map[string]os.FileInfo, error) {
	entries := make(map[string]os.FileInfo)
	if err := walkOutput(outputRoot, "", entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func outputEntriesInRoot(parent *os.Root, outputName string) (map[string]os.FileInfo, error) {
	entries := make(map[string]os.FileInfo)
	info, err := parent.Lstat(outputName)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	entries[outputName] = info
	if !info.IsDir() {
		return entries, nil
	}
	if err := walkOutputInRoot(parent, outputName, "", entries); err != nil {
		return nil, err
	}
	delete(entries, outputName)
	return entries, nil
}

func walkOutputInRoot(parent *os.Root, directory, relativeDirectory string, entries map[string]os.FileInfo) error {
	file, err := parent.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	directoryEntries, err := file.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range directoryEntries {
		destination := entry.Name()
		if relativeDirectory != "" {
			destination = path.Join(relativeDirectory, destination)
		}
		fullPath := filepath.Join(directory, entry.Name())
		info, err := parent.Lstat(fullPath)
		if err != nil {
			return err
		}
		entries[destination] = info
		if info.IsDir() {
			if err := walkOutputInRoot(parent, fullPath, destination, entries); err != nil {
				return err
			}
		}
	}
	return nil
}

func walkOutput(directory, relativeDirectory string, entries map[string]os.FileInfo) error {
	directoryEntries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range directoryEntries {
		destination := entry.Name()
		if relativeDirectory != "" {
			destination = path.Join(relativeDirectory, destination)
		}
		info, err := os.Lstat(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		entries[destination] = info
		if info.IsDir() {
			if err := walkOutput(filepath.Join(directory, entry.Name()), destination, entries); err != nil {
				return err
			}
		}
	}
	return nil
}

func requiredDirectories(expected map[string]expectedFile) map[string]bool {
	directories := make(map[string]bool)
	for destination := range expected {
		for directory := path.Dir(destination); directory != "."; directory = path.Dir(directory) {
			directories[directory] = true
		}
	}
	return directories
}

func hasExecutableIntent(info os.FileInfo) bool {
	return info.Mode().Perm()&0o111 != 0
}

func sortDrift(drift []Drift) {
	sort.Slice(drift, func(i, j int) bool {
		if drift[i].Path == drift[j].Path {
			return drift[i].Kind < drift[j].Kind
		}
		return drift[i].Path < drift[j].Path
	})
}
