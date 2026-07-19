package compatibility

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	npmrcPath                  = model.RelativePath(".npmrc")
	legacyPeerDepsKey          = "legacy-peer-deps"
	legacyPeerDepsMarker       = "# agentbundler: repository-root Pi compatibility\n"
	legacyPeerDepsSetting      = "legacy-peer-deps=true\n"
	ownedLegacyPeerDepsSetting = legacyPeerDepsMarker + legacyPeerDepsSetting
)

func preparePiNPMRC(workspace string, previous, ownership *piOwnership) (File, []model.Diagnostic) {
	data, _, err := readOptionalRootRegularFile(workspace, npmrcPath)
	if err != nil {
		return File{}, []model.Diagnostic{diagnostic("compatibility-npmrc-invalid", "read repository .npmrc: "+err.Error())}
	}
	if previous != nil && previous.LegacyPeerDeps {
		var removed bool
		data, removed, err = removeOwnedLegacyPeerDeps(data)
		if err != nil {
			return File{}, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", err.Error())}
		}
		if !removed {
			return File{}, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "ownership state claims .npmrc legacy-peer-deps, but the Agent Bundler marker is missing")}
		}
		if previous.LegacyPeerDepsSeparator && len(data) != 0 && data[len(data)-1] == '\n' {
			data = data[:len(data)-1]
		}
	}

	value, present, err := legacyPeerDepsValue(data)
	if err != nil {
		return File{}, []model.Diagnostic{diagnostic("compatibility-npmrc-conflict", err.Error())}
	}
	if present {
		if value != "true" {
			return File{}, []model.Diagnostic{diagnostic("compatibility-npmrc-conflict", fmt.Sprintf("repository .npmrc %s is %q; Pi Git installs require true", legacyPeerDepsKey, value))}
		}
		return File{Path: npmrcPath, Bytes: data}, nil
	}

	separator := len(data) != 0 && data[len(data)-1] != '\n'
	if separator {
		data = append(data, '\n')
	}
	data = append(data, ownedLegacyPeerDepsSetting...)
	ownership.LegacyPeerDeps = true
	ownership.LegacyPeerDepsSeparator = separator
	return File{Path: npmrcPath, Bytes: data}, nil
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

func legacyPeerDepsValue(data []byte) (string, bool, error) {
	var value string
	found := false
	for _, line := range npmrcLines(data) {
		key, candidate, active := npmrcAssignment(line)
		if !active || key != legacyPeerDepsKey {
			continue
		}
		if found {
			return "", false, fmt.Errorf("repository .npmrc defines %s more than once", legacyPeerDepsKey)
		}
		found = true
		value = strings.ToLower(candidate)
	}
	return value, found, nil
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
