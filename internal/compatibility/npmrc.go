package compatibility

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// This file exists only for v0.5.1 root-compatibility cleanup. Remove it when
// that migration path is no longer supported.
const (
	npmrcPath                  = model.RelativePath(".npmrc")
	legacyPeerDepsKey          = "legacy-peer-deps"
	legacyPeerDepsMarker       = "# agentbundler: repository-root Pi compatibility\n"
	legacyPeerDepsSetting      = "legacy-peer-deps=true\n"
	ownedLegacyPeerDepsSetting = legacyPeerDepsMarker + legacyPeerDepsSetting
)

func validateLegacyPiOwnership(workspace string, ownership *piOwnership) (bool, []model.Diagnostic) {
	legacy := ownership.LegacyPeerDeps || containsString(ownership.Extensions, legacyPiSubagentExtension)
	if !legacy {
		return false, nil
	}
	if !ownership.LegacyPeerDeps || !containsString(ownership.Extensions, legacyPiSubagentExtension) {
		return false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "legacy Pi runtime ownership requires the v0.5.1 extension and .npmrc ownership markers")}
	}
	data, exists, err := readOptionalRootRegularFile(workspace, npmrcPath)
	if err != nil {
		return false, []model.Diagnostic{diagnostic("compatibility-npmrc-invalid", "read repository .npmrc: "+err.Error())}
	}
	if !exists {
		return false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "legacy Pi runtime ownership requires the v0.5.1 .npmrc marker")}
	}
	_, found, err := removeOwnedLegacyPeerDeps(data)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("agent Bundler marker is missing")
		}
		return false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "legacy Pi runtime .npmrc ownership is invalid: "+err.Error())}
	}
	return true, nil
}

func cleanupPiNPMRC(workspace string, ownership *piOwnership) (File, bool, bool, []model.Diagnostic) {
	if ownership == nil || !ownership.LegacyPeerDeps {
		return File{}, false, false, nil
	}
	data, exists, err := readOptionalRootRegularFile(workspace, npmrcPath)
	if err != nil {
		return File{}, false, false, []model.Diagnostic{diagnostic("compatibility-npmrc-invalid", "read repository .npmrc during stale cleanup: "+err.Error())}
	}
	if !exists {
		return File{}, false, false, nil
	}
	cleaned, removed, err := removeOwnedLegacyPeerDeps(data)
	if err != nil {
		return File{}, false, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", err.Error())}
	}
	if !removed {
		return File{}, false, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "ownership state claims .npmrc legacy-peer-deps, but the Agent Bundler marker is missing")}
	}
	if ownership.LegacyPeerDepsSeparator && len(cleaned) != 0 && cleaned[len(cleaned)-1] == '\n' {
		cleaned = cleaned[:len(cleaned)-1]
	}
	if len(cleaned) == 0 {
		return File{}, false, true, nil
	}
	return File{Path: npmrcPath, Bytes: cleaned}, true, false, nil
}

func removeOwnedLegacyPeerDeps(data []byte) ([]byte, bool, error) {
	lines := npmrcLines(data)
	marker := -1
	for index, line := range lines {
		if string(line) != legacyPeerDepsMarker {
			continue
		}
		if marker >= 0 {
			return nil, false, fmt.Errorf("repository .npmrc contains the Agent Bundler legacy-peer-deps marker more than once")
		}
		marker = index
	}
	if marker < 0 {
		return data, false, nil
	}
	if marker+1 >= len(lines) {
		return nil, false, fmt.Errorf("repository .npmrc Agent Bundler marker is not followed by legacy-peer-deps=true")
	}
	key, value, active := npmrcAssignment(lines[marker+1])
	if !active || key != legacyPeerDepsKey || strings.ToLower(value) != "true" {
		return nil, false, fmt.Errorf("repository .npmrc Agent Bundler marker is not followed by legacy-peer-deps=true")
	}
	result := make([]byte, 0, len(data))
	for index, line := range lines {
		if index == marker || index == marker+1 {
			continue
		}
		result = append(result, line...)
	}
	return result, true, nil
}

func npmrcLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	var lines [][]byte
	for len(data) != 0 {
		index := strings.IndexByte(string(data), '\n')
		if index < 0 {
			lines = append(lines, data)
			break
		}
		index++
		lines = append(lines, data[:index])
		data = data[index:]
	}
	return lines
}

func npmrcAssignment(raw []byte) (string, string, bool) {
	line := strings.TrimSpace(strings.TrimSuffix(string(raw), "\n"))
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false
	}
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	return key, strings.TrimSpace(value), true
}

func readOptionalRootRegularFile(workspace string, relative model.RelativePath) ([]byte, bool, error) {
	data, err := readRootRegularFile(workspace, relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}
