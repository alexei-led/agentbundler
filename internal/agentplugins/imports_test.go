package agentplugins_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
	"os",         // filesystem and process
	"io/fs",      // filesystem traversal
	"os/exec",    // process execution
	"net/http",   // network
	"syscall",    // raw OS calls
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

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		name := fi.Name()
		// Exclude test files and non-Go files.
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing agentplugins source: %v", err)
	}

	if len(pkgs) == 0 {
		t.Fatal("no Go packages found in current directory")
	}

	for pkgName, pkg := range pkgs {
		for filename, file := range pkg.Files {
			base := filepath.Base(filename)
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)

				// Check forbidden patterns first (explicit deny).
				for _, forbidden := range forbiddenPatterns {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
						t.Errorf("[%s] %s: forbidden import %q matches pattern %q",
							pkgName, base, importPath, forbidden)
					}
				}

				// Then check allowlist (must be present).
				if !permittedImports[importPath] {
					t.Errorf("[%s] %s: import %q is not in the permitted allowlist",
						pkgName, base, importPath)
				}
			}
		}
	}
}
