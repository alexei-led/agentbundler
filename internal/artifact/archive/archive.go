// Package archive writes deterministic target-root release archives.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// WriteTargetRoots archives each generated target root with its native manifest at archive root.
func WriteTargetRoots(workspace string, manifest model.SourceManifest, plan model.BuildPlan, output string) ([]string, error) {
	if output == "" {
		return nil, fmt.Errorf("archive output directory is required")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("create archive output: %w", err)
	}
	name, ok := manifest.Distribution["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("distribution.name is required for release archives")
	}
	paths := make([]string, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		extension := ".tar.gz"
		if target.Target == model.TargetPi {
			extension = ".tgz"
		}
		archivePath := filepath.Join(output, name+"-"+string(target.Target)+extension)
		root := filepath.Join(workspace, filepath.FromSlash(string(manifest.Output)), string(target.Target))
		if err := writeTarGzip(archivePath, root); err != nil {
			return nil, fmt.Errorf("archive target %q: %w", target.Target, err)
		}
		paths = append(paths, archivePath)
	}
	sort.Strings(paths)
	return paths, nil
}

func writeTarGzip(destination, root string) (err error) {
	files := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(filepath.ToSlash(path[len(root):]), "/.agentbundler") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("generated output contains symlink %q", path)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("generated output contains non-regular file %q", path)
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("generated target root %q is empty", root)
	}
	sort.Strings(files)
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
	defer file.Close()
	gzipWriter, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, path := range files {
		info, statErr := os.Stat(path)
		if statErr != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return statErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return relErr
		}
		name := filepath.ToSlash(relative)
		if name == "." || strings.HasPrefix(name, "../") {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return fmt.Errorf("archive path escapes root: %q", name)
		}
		mode := int64(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		header := &tar.Header{Name: name, Mode: mode, Size: info.Size(), ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return err
		}
		input, openErr := os.Open(path)
		if openErr != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return openErr
		}
		_, copyErr := io.Copy(tarWriter, input)
		closeErr := input.Close()
		if copyErr != nil || closeErr != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, destination)
}
