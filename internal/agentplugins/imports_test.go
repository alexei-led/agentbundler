package agentplugins_test

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// permittedImports is the complete allowlist for non-test agentplugins source files.
// This enforces the package purity contract: no filesystem, process, or network
// imports are permitted. Advancing the pinned profile or adding JSON/schema work
// may require adding new standard library packages; each addition requires a review
// to confirm it introduces no I/O capability.
var permittedImports = map[string]bool{
	"bytes":         true,
	"encoding/hex":  true,
	"encoding/json": true,
	"embed":         true,
	"fmt":           true,
	"net/url":       true,
	"regexp":        true,
	"sort":          true,
	"strings":       true,
	"unicode":       true,
	"unicode/utf8":  true,
	"io":            true,
}

// forbiddenPatterns is an explicit list of forbidden import path prefixes or
// exact paths. An import matching any of these fails even if it were somehow
// added to permittedImports.
var forbiddenPatterns = []string{
	"os",          // filesystem and process
	"io/fs",       // filesystem traversal
	"os/exec",     // process execution
	"net/http",    // network
	"syscall",     // raw OS calls
	"runtime/cgo", // FFI
	// internal compiler/target/artifact/cmd packages are caught by the
	// archfit forbidden_dependency rules; no need to duplicate them here.
}

// TestImportAllowlist parses every non-test Go source file in the agentplugins
// package and verifies that all imports are in the permitted set and that none
// match a forbidden pattern. This is the static proof that agentplugins is a
// pure package with no filesystem, process, or network capability.
func TestImportAllowlist(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading agentplugins directory: %v", err)
	}

	fset := token.NewFileSet()
	parsed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		parsed++

		pkgName := file.Name.Name
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Check forbidden patterns first (explicit deny).
			for _, forbidden := range forbiddenPatterns {
				if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
					t.Errorf("[%s] %s: forbidden import %q matches pattern %q",
						pkgName, name, importPath, forbidden)
				}
			}

			// Then check allowlist (must be present).
			if !permittedImports[importPath] {
				t.Errorf("[%s] %s: import %q is not in the permitted allowlist",
					pkgName, name, importPath)
			}
		}
	}

	if parsed == 0 {
		t.Fatal("no non-test Go source files found in current directory")
	}
}
