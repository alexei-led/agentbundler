// Package compatibility derives and maintains opt-in repository-root vendor discovery files.
package compatibility

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const (
	statePath    = model.RelativePath(".agentbundler/compatibility.json")
	stateVersion = 1
)

// Request contains the canonical target plan and repository-root context.
type Request struct {
	WorkspaceRoot string
	Output        model.RelativePath
	Config        *model.CompatibilityConfig
	Plan          model.BuildPlan
}

// File is one expected repository-root compatibility file.
type File struct {
	Path  model.RelativePath
	Bytes []byte
}

// Plan describes expected compatibility files and stale owned files to remove.
type Plan struct {
	Files  []File
	Remove []model.RelativePath
}

type ownershipState struct {
	Version int                  `json:"version"`
	Files   []model.RelativePath `json:"files"`
	Pi      *piOwnership         `json:"pi,omitempty"`
}

type piOwnership struct {
	Dependencies            []string `json:"dependencies,omitempty"`
	Extensions              []string `json:"extensions,omitempty"`
	Skills                  []string `json:"skills,omitempty"`
	Agents                  []string `json:"agents,omitempty"`
	LegacyPeerDeps          bool     `json:"legacyPeerDeps,omitempty"`
	LegacyPeerDepsSeparator bool     `json:"legacyPeerDepsSeparator,omitempty"`
}

// Prepare derives root files only from target-generated manifests plus the
// existing root package.json fields that Agent Bundler does not own.
func Prepare(request Request) (Plan, []model.Diagnostic) {
	previous, present, diagnostics := loadState(request.WorkspaceRoot)
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}

	selected := make(map[model.TargetID]model.TargetPlan, len(request.Plan.Targets))
	for _, targetPlan := range request.Plan.Targets {
		selected[targetPlan.Target] = targetPlan
	}
	if diagnostics := validateOwnershipAgainstPlan(request.WorkspaceRoot, previous, request.Output, selected); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	enabled := make(map[model.TargetID]struct{})
	if request.Config != nil {
		for _, targetID := range request.Config.RootManifests {
			if _, exists := selected[targetID]; !exists {
				return Plan{}, []model.Diagnostic{diagnostic("compatibility-incomplete-selection", fmt.Sprintf("repository-root compatibility target %q was not selected; select every compatibility target", targetID))}
			}
			enabled[targetID] = struct{}{}
		}
	}

	desired := ownershipState{Version: stateVersion, Files: []model.RelativePath{}}
	files := make([]File, 0)
	remove := make([]model.RelativePath, 0)
	for _, targetID := range sortedTargets(enabled) {
		targetPlan := selected[targetID]
		switch targetID {
		case model.TargetClaude, model.TargetCodex, model.TargetCopilot, model.TargetCursor, model.TargetGrok:
			file, extra, targetDiagnostics := prepareMarketplace(request.Output, targetPlan)
			diagnostics = append(diagnostics, targetDiagnostics...)
			if len(targetDiagnostics) != 0 {
				continue
			}
			files = append(files, file)
			desired.Files = append(desired.Files, file.Path)
			for _, extraFile := range extra {
				files = append(files, extraFile)
				desired.Files = append(desired.Files, extraFile.Path)
			}
		case model.TargetPi:
			file, ownership, targetDiagnostics := preparePiPackage(request.WorkspaceRoot, request.Output, targetPlan, previous.Pi)
			diagnostics = append(diagnostics, targetDiagnostics...)
			if len(targetDiagnostics) == 0 {
				files = append(files, file)
				desired.Pi = ownership
				if previous.Pi != nil && previous.Pi.LegacyPeerDeps {
					npmrc, changed, removeNPMRC, npmrcDiagnostics := cleanupPiNPMRC(request.WorkspaceRoot, previous.Pi)
					diagnostics = append(diagnostics, npmrcDiagnostics...)
					if changed {
						files = append(files, npmrc)
					}
					if removeNPMRC {
						remove = append(remove, npmrcPath)
					}
				}
			}
		}
	}
	if len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}

	if _, piEnabled := enabled[model.TargetPi]; !piEnabled && previous.Pi != nil {
		file, changed, cleanupDiagnostics := cleanupPiPackage(request.WorkspaceRoot, previous.Pi)
		if len(cleanupDiagnostics) != 0 {
			return Plan{}, cleanupDiagnostics
		}
		if changed {
			files = append(files, file)
		}
		npmrc, npmrcChanged, npmrcRemove, npmrcDiagnostics := cleanupPiNPMRC(request.WorkspaceRoot, previous.Pi)
		if len(npmrcDiagnostics) != 0 {
			return Plan{}, npmrcDiagnostics
		}
		if npmrcChanged {
			files = append(files, npmrc)
		}
		if npmrcRemove {
			remove = append(remove, ".npmrc")
		}
	}

	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	sort.Slice(desired.Files, func(left, right int) bool { return desired.Files[left] < desired.Files[right] })
	if len(desired.Files) != 0 || desired.Pi != nil {
		data, err := json.Marshal(desired)
		if err != nil {
			return Plan{}, []model.Diagnostic{diagnostic("compatibility-state-invalid", err.Error())}
		}
		files = append(files, File{Path: statePath, Bytes: append(data, '\n')})
		sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	}

	desiredFiles := make(map[model.RelativePath]struct{}, len(desired.Files))
	for _, file := range desired.Files {
		desiredFiles[file] = struct{}{}
	}
	if present {
		for _, file := range previous.Files {
			if _, exists := desiredFiles[file]; !exists {
				remove = append(remove, file)
			}
		}
		if len(desired.Files) == 0 && desired.Pi == nil {
			remove = append(remove, statePath)
		}
	}
	sort.Slice(remove, func(left, right int) bool { return remove[left] < remove[right] })
	if diagnostics := validateRootPaths(request.WorkspaceRoot, files, remove); len(diagnostics) != 0 {
		return Plan{}, diagnostics
	}
	return Plan{Files: files, Remove: remove}, nil
}

func validateRootPaths(workspace string, files []File, remove []model.RelativePath) []model.Diagnostic {
	paths := make([]model.RelativePath, 0, len(files)+len(remove))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	paths = append(paths, remove...)
	seen := make(map[model.RelativePath]struct{}, len(paths))
	for _, relative := range paths {
		if _, duplicate := seen[relative]; duplicate {
			return []model.Diagnostic{diagnostic("compatibility-path-collision", fmt.Sprintf("repository compatibility path %q is planned more than once", relative))}
		}
		seen[relative] = struct{}{}
		full := filepath.Join(workspace, filepath.FromSlash(string(relative)))
		if err := rejectSymlinkComponents(workspace, full); err != nil {
			return []model.Diagnostic{diagnostic("compatibility-path-unsafe", fmt.Sprintf("repository compatibility path %q: %v", relative, err))}
		}
		info, err := os.Lstat(full)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return []model.Diagnostic{diagnostic("compatibility-path-unsafe", fmt.Sprintf("inspect repository compatibility path %q: %v", relative, err))}
		}
		if !info.Mode().IsRegular() {
			return []model.Diagnostic{diagnostic("compatibility-path-unsafe", fmt.Sprintf("repository compatibility path %q is not a regular file", relative))}
		}
	}
	return nil
}

func loadState(workspace string) (ownershipState, bool, []model.Diagnostic) {
	data, err := readRootRegularFile(workspace, statePath)
	if errors.Is(err, os.ErrNotExist) {
		return ownershipState{Version: stateVersion}, false, nil
	}
	if err != nil {
		return ownershipState{}, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "read compatibility ownership state: "+err.Error())}
	}
	var state ownershipState
	if err := model.DecodeStrictJSONObject(data, &state); err != nil {
		return ownershipState{}, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "decode compatibility ownership state: "+err.Error())}
	}
	if state.Version != stateVersion {
		return ownershipState{}, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("compatibility ownership state version must be %d", stateVersion))}
	}
	if diagnostics := validatePiOwnership(state.Pi); len(diagnostics) != 0 {
		return ownershipState{}, false, diagnostics
	}
	seen := make(map[model.RelativePath]struct{}, len(state.Files))
	for _, file := range state.Files {
		if _, err := model.NewRelativePath(string(file)); err != nil {
			return ownershipState{}, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned path %q: %v", file, err))}
		}
		if !fixedMarketplacePath(file) && !codexAgentPath(file) {
			return ownershipState{}, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned path %q is not a repository compatibility output", file))}
		}
		if _, duplicate := seen[file]; duplicate {
			return ownershipState{}, false, []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned path %q is duplicated", file))}
		}
		seen[file] = struct{}{}
	}
	return state, true, nil
}

func fixedMarketplacePath(file model.RelativePath) bool {
	switch file {
	case ".claude-plugin/marketplace.json", ".agents/plugins/marketplace.json", ".github/plugin/marketplace.json", ".cursor-plugin/marketplace.json":
		return true
	default:
		return false
	}
}

func codexAgentPath(file model.RelativePath) bool {
	const prefix = ".codex/agents/"
	name := strings.TrimPrefix(string(file), prefix)
	return name != string(file) && name != "" && !strings.Contains(name, "/") && strings.HasSuffix(name, ".toml") && name != ".toml"
}

func validateOwnershipAgainstPlan(workspace string, state ownershipState, output model.RelativePath, selected map[model.TargetID]model.TargetPlan) []model.Diagnostic {
	for _, file := range state.Files {
		if !codexAgentPath(file) {
			continue
		}
		codexPlan, exists := selected[model.TargetCodex]
		if !exists || !targetPlanHasFile(codexPlan, string(file)) {
			return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned Codex agent %q does not match a canonical generated profile", file))}
		}
	}
	if state.Pi == nil {
		return nil
	}
	legacyOwnership, legacyDiagnostics := validateLegacyPiOwnership(workspace, state.Pi)
	if len(legacyDiagnostics) != 0 {
		return legacyDiagnostics
	}
	piPlan, exists := selected[model.TargetPi]
	if !exists {
		return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "owned Pi fields cannot be validated without canonical generated Pi output")}
	}
	expected, diagnostics := canonicalPiOwnership(output, piPlan)
	if len(diagnostics) != 0 {
		return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "validate owned Pi fields against canonical output: "+diagnostics[0].Message)}
	}
	for _, values := range []struct {
		name     string
		actual   []string
		expected []string
	}{
		{name: "dependency", actual: state.Pi.Dependencies, expected: expected.Dependencies},
		{name: "extension", actual: state.Pi.Extensions, expected: expected.Extensions},
		{name: "skill", actual: state.Pi.Skills, expected: expected.Skills},
		{name: "agent", actual: state.Pi.Agents, expected: expected.Agents},
	} {
		for _, value := range values.actual {
			if !containsString(values.expected, value) && (!legacyOwnership || !legacyPiOwnershipValue(values.name, value)) {
				return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned Pi %s %q does not match canonical generated output", values.name, value))}
			}
		}
	}
	return nil
}

func validatePiOwnership(ownership *piOwnership) []model.Diagnostic {
	if ownership == nil {
		return nil
	}
	if ownership.LegacyPeerDepsSeparator && !ownership.LegacyPeerDeps {
		return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "legacyPeerDepsSeparator requires owned legacyPeerDeps")}
	}
	seen := make(map[string]struct{})
	for _, dependency := range ownership.Dependencies {
		if strings.TrimSpace(dependency) == "" || strings.ContainsRune(dependency, '\x00') {
			return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", "owned Pi dependency names must be non-empty strings without NUL")}
		}
		if _, duplicate := seen["dependency:"+dependency]; duplicate {
			return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned Pi dependency %q is duplicated", dependency))}
		}
		seen["dependency:"+dependency] = struct{}{}
	}
	for field, values := range map[string][]string{
		"extensions": ownership.Extensions,
		"skills":     ownership.Skills,
		"agents":     ownership.Agents,
	} {
		for _, value := range values {
			if _, err := piPackagePath(value); err != nil {
				return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned Pi %s path %q: %v", field, value, err))}
			}
			key := field + ":" + value
			if _, duplicate := seen[key]; duplicate {
				return []model.Diagnostic{diagnostic("compatibility-ownership-invalid", fmt.Sprintf("owned Pi %s path %q is duplicated", field, value))}
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

func sortedTargets(values map[model.TargetID]struct{}) []model.TargetID {
	result := make([]model.TargetID, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func targetPlanFile(plan model.TargetPlan, filePath model.RelativePath) (model.PlannedFile, bool) {
	for _, file := range plan.Files {
		if file.Path == filePath {
			return file, true
		}
	}
	return model.PlannedFile{}, false
}

func targetPlanHasFile(plan model.TargetPlan, filePath string) bool {
	for _, file := range plan.Files {
		if string(file.Path) == filePath {
			return true
		}
	}
	return false
}

func safeLocalSource(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") {
		return "", fmt.Errorf("local source must be a non-empty slash-separated path")
	}
	if path.IsAbs(value) || strings.HasPrefix(value, "/") || strings.Contains(value, ":") {
		return "", fmt.Errorf("local source must not be absolute or contain a volume separator")
	}
	local := strings.TrimPrefix(value, "./")
	if local == "" || local == "." {
		return ".", nil
	}
	for _, segment := range strings.Split(local, "/") {
		if segment == ".." {
			return "", fmt.Errorf("local source contains traversal")
		}
	}
	if _, err := model.NewRelativePath(local); err != nil {
		return "", err
	}
	return local, nil
}

func rootRoute(output model.RelativePath, target model.TargetID, source string) string {
	root := path.Join(string(output), string(target))
	if source != "." {
		root = path.Join(root, source)
	}
	return "./" + root
}

func diagnostic(code, message string) model.Diagnostic {
	return model.Diagnostic{Code: code, Severity: model.SeverityError, Message: message}
}
