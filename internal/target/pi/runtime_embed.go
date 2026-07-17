package pi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const embeddedRuntimeRoot = "extensions/_agentbundler-hooks"

var embeddedRuntimePaths = []string{
	"runtime/src/index.ts",
	"runtime/src/matcher.ts",
	"runtime/src/process.ts",
	"runtime/src/runtime.ts",
	"runtime/src/schema.ts",
}

//go:embed runtime/src/index.ts runtime/src/matcher.ts runtime/src/process.ts runtime/src/runtime.ts runtime/src/schema.ts
var embeddedRuntime embed.FS

//go:embed runtime/vendor/pi-subagents-runtime-0.34.0.tgz
var piSubagentsRuntime []byte

type runtimeFile struct {
	name       string
	bytes      []byte
	executable bool
}

func runtimeFiles() ([]runtimeFile, error) {
	files := make([]runtimeFile, 0, len(embeddedRuntimePaths))
	for _, path := range embeddedRuntimePaths {
		data, err := embeddedRuntime.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, runtimeFile{name: path[len("runtime/src/"):], bytes: data})
	}
	return files, nil
}

func piSubagentRuntimeFiles() ([]runtimeFile, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(piSubagentsRuntime))
	if err != nil {
		return nil, err
	}
	defer func() { _ = compressed.Close() }()
	reader := tar.NewReader(compressed)
	var files []runtimeFile
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeDir || strings.HasPrefix(header.Name, "node_modules/.bin/") || header.Name == "node_modules/.package-lock.json" {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("vendored Pi runtime contains unsupported entry %q", header.Name)
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if _, err := model.NewRelativePath(name); err != nil || !strings.HasPrefix(name, "node_modules/") {
			return nil, fmt.Errorf("vendored Pi runtime path %q is invalid", header.Name)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		files = append(files, runtimeFile{name: name, bytes: data, executable: header.FileInfo().Mode().Perm()&0o111 != 0})
	}
	sort.Slice(files, func(left, right int) bool { return files[left].name < files[right].name })
	return files, nil
}
