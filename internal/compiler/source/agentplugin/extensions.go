package agentplugin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// buildExtensions collects client-specific extension entries from the plugin
// manifest extensions map and the traversal files under extensions/<namespace>/.
//
// Any namespace declared in the manifest gets an entry (even with no files).
// Any files found under extensions/<namespace>/ are added to the corresponding
// entry (even if the namespace is not in the manifest, with nil Manifest value).
// File paths within each ClientExtension are relative to the extension root
// extensions/<namespace>/.
//
// It returns the extension entries in lexical namespace order and the set of
// plugin-relative file paths consumed by extensions.
func buildExtensions(
	manifestExtensions map[string]any,
	files []traversedFile,
	workspacePrefix string,
) ([]model.ClientExtension, map[string]bool, []model.Diagnostic) {
	type extEntry struct {
		manifest any
		files    []traversedFile
	}
	entries := make(map[string]*extEntry)

	// Seed from manifest extension namespaces.
	for ns, val := range manifestExtensions {
		entries[ns] = &extEntry{manifest: val}
	}

	// Collect filesystem files under extensions/<namespace>/.
	usedPaths := make(map[string]bool)
	for _, f := range files {
		parts := strings.SplitN(f.relPath, "/", 3)
		if len(parts) < 3 {
			continue
		}
		if parts[0] != "extensions" {
			continue
		}
		ns := parts[1]
		if ns == "" {
			continue
		}
		usedPaths[f.relPath] = true
		if _, ok := entries[ns]; !ok {
			entries[ns] = &extEntry{}
		}
		entries[ns].files = append(entries[ns].files, f)
	}

	if len(entries) == 0 {
		return nil, usedPaths, nil
	}

	// Build result in sorted namespace order.
	namespaces := make([]string, 0, len(entries))
	for ns := range entries {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	result := make([]model.ClientExtension, 0, len(namespaces))
	var diagnostics []model.Diagnostic
	for _, ns := range namespaces {
		entry := entries[ns]
		// Prefix for files within this extension: "extensions/<ns>/"
		prefix := "extensions/" + ns + "/"

		var pkgFiles []model.PackageFile
		// Sort by relPath for determinism.
		sortedFiles := append([]traversedFile(nil), entry.files...)
		sort.Slice(sortedFiles, func(i, j int) bool {
			return sortedFiles[i].relPath < sortedFiles[j].relPath
		})
		for _, f := range sortedFiles {
			// Path relative to extensions/<ns>/ root.
			relToExt := strings.TrimPrefix(f.relPath, prefix)
			rp, err := model.NewRelativePath(relToExt)
			if err != nil {
				diagnostics = append(diagnostics, diag("", fmt.Sprintf(
					"extension %q package file path %q is not portable: %v", ns, relToExt, err)))
				continue
			}
			origin := workspaceOrigin(workspacePrefix, f.relPath)
			pkgFiles = append(pkgFiles, model.PackageFile{
				Path:       rp,
				Bytes:      f.bytes,
				Executable: f.executable,
				SHA256:     f.sha256,
				Origin:     []model.SourceLocation{{Path: origin}},
			})
		}
		result = append(result, model.ClientExtension{
			Namespace:    ns,
			Manifest:     entry.manifest,
			PackageFiles: pkgFiles,
		})
	}
	return result, usedPaths, diagnostics
}
