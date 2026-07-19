package compatibility

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func preparePiPackage(workspace string, output model.RelativePath, plan model.TargetPlan, previous *piOwnership) (File, *piOwnership, []model.Diagnostic) {
	generated, diagnostics := piGeneratedPackages(plan)
	if len(diagnostics) != 0 {
		return File{}, nil, diagnostics
	}
	rootBytes, err := readRootRegularFile(workspace, "package.json")
	if err != nil {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-pi-package-missing", "read repository package.json: "+err.Error())}
	}
	root, err := decodePackageJSON(rootBytes, "repository package.json")
	if err != nil {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
	}
	if previous != nil {
		if err := stripPiOwnership(root, previous); err != nil {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-pi-ownership-invalid", err.Error())}
		}
	}

	ownership := &piOwnership{}
	for _, pkg := range generated {
		if err := mergePiDependencies(root, pkg.manifest, ownership); err != nil {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-dependency-collision", err.Error())}
		}
		if err := mergePiManifest(root, pkg.manifest, output, pkg.root, ownership); err != nil {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
		}
	}
	data, err := marshalPackageJSON(root, rootBytes)
	if err != nil {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
	}
	return File{Path: "package.json", Bytes: data}, ownership, nil
}

type piGeneratedPackage struct {
	root     string
	manifest map[string]any
}

func piGeneratedPackages(plan model.TargetPlan) ([]piGeneratedPackage, []model.Diagnostic) {
	if generatedFile, exists := targetPlanFile(plan, "package.json"); exists {
		generated, err := decodePackageJSON(generatedFile.Bytes, "generated Pi package.json")
		if err != nil {
			return nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
		}
		return []piGeneratedPackage{{manifest: generated}}, nil
	}
	packages := append([]model.PackageID(nil), plan.Packages...)
	sort.Slice(packages, func(left, right int) bool { return packages[left] < packages[right] })
	result := make([]piGeneratedPackage, 0, len(packages))
	for _, packageID := range packages {
		manifestPath := model.RelativePath(path.Join(string(packageID), "package.json"))
		generatedFile, exists := targetPlanFile(plan, manifestPath)
		if !exists {
			return nil, []model.Diagnostic{diagnostic("compatibility-pi-layout-unsupported", fmt.Sprintf("Pi package %q has no generated manifest %q", packageID, manifestPath))}
		}
		generated, err := decodePackageJSON(generatedFile.Bytes, fmt.Sprintf("generated Pi package %q package.json", packageID))
		if err != nil {
			return nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
		}
		result = append(result, piGeneratedPackage{root: string(packageID), manifest: generated})
	}
	if len(result) == 0 {
		return nil, []model.Diagnostic{diagnostic("compatibility-pi-layout-unsupported", "Pi target generated no package.json")}
	}
	return result, nil
}

func canonicalPiOwnership(output model.RelativePath, plan model.TargetPlan) (*piOwnership, []model.Diagnostic) {
	generated, diagnostics := piGeneratedPackages(plan)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	root := make(map[string]any)
	ownership := &piOwnership{}
	for _, pkg := range generated {
		if err := mergePiDependencies(root, pkg.manifest, ownership); err != nil {
			return nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
		}
		if err := mergePiManifest(root, pkg.manifest, output, pkg.root, ownership); err != nil {
			return nil, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
		}
	}
	return ownership, nil
}

func cleanupPiPackage(workspace string, previous *piOwnership) (File, bool, []model.Diagnostic) {
	rootBytes, err := readRootRegularFile(workspace, "package.json")
	if err != nil {
		return File{}, false, []model.Diagnostic{diagnostic("compatibility-pi-package-missing", "read repository package.json during stale cleanup: "+err.Error())}
	}
	root, err := decodePackageJSON(rootBytes, "repository package.json")
	if err != nil {
		return File{}, false, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
	}
	if err := stripPiOwnership(root, previous); err != nil {
		return File{}, false, []model.Diagnostic{diagnostic("compatibility-pi-ownership-invalid", err.Error())}
	}
	data, err := marshalPackageJSON(root, rootBytes)
	if err != nil {
		return File{}, false, []model.Diagnostic{diagnostic("compatibility-pi-manifest-invalid", err.Error())}
	}
	return File{Path: "package.json", Bytes: data}, !bytes.Equal(data, rootBytes), nil
}

func decodePackageJSON(data []byte, owner string) (map[string]any, error) {
	var result map[string]any
	if err := model.DecodeStrictJSONObject(data, &result); err != nil {
		return nil, fmt.Errorf("%s: %w", owner, err)
	}
	return result, nil
}

func marshalPackageJSON(document map[string]any, original []byte) ([]byte, error) {
	order, rawFields, err := orderedJSONObject(original)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(order))
	keys := make([]string, 0, len(document))
	for _, key := range order {
		if _, exists := document[key]; exists {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	for _, key := range []string{"dependencies", "pi"} {
		if _, exists := document[key]; exists {
			if _, already := seen[key]; !already {
				keys = append(keys, key)
				seen[key] = struct{}{}
			}
		}
	}
	var additions []string
	for key := range document {
		if _, exists := seen[key]; !exists {
			additions = append(additions, key)
		}
	}
	sort.Strings(additions)
	keys = append(keys, additions...)

	var result bytes.Buffer
	result.WriteString("{\n")
	for index, key := range keys {
		if index != 0 {
			result.WriteString(",\n")
		}
		encodedKey, _ := json.Marshal(key)
		result.WriteString("  ")
		result.Write(encodedKey)
		result.WriteString(": ")
		encodedValue, err := packageJSONField(document[key], rawFields[key])
		if err != nil {
			return nil, err
		}
		result.Write(encodedValue)
	}
	result.WriteString("\n}\n")
	return result.Bytes(), nil
}

func orderedJSONObject(data []byte) ([]string, map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := model.DecodeStrictJSONObject(data, &fields); err != nil {
		return nil, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, fmt.Errorf("package.json must be an object")
	}
	order := make([]string, 0, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, nil, fmt.Errorf("package.json has an invalid object key")
		}
		order = append(order, key)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, err
	}
	return order, fields, nil
}

func packageJSONField(value any, original json.RawMessage) ([]byte, error) {
	if original != nil {
		var originalValue any
		if err := model.DecodeStrictJSON(original, &originalValue); err != nil {
			return nil, err
		}
		if reflect.DeepEqual(value, originalValue) {
			var preserved bytes.Buffer
			if err := json.Indent(&preserved, original, "  ", "  "); err != nil {
				return nil, err
			}
			return preserved.Bytes(), nil
		}
	}
	return json.MarshalIndent(value, "  ", "  ")
}

func mergePiDependencies(root, generated map[string]any, ownership *piOwnership) error {
	generatedDependencies, err := objectField(generated, "dependencies", false)
	if err != nil {
		return fmt.Errorf("generated Pi package dependencies: %w", err)
	}
	if len(generatedDependencies) == 0 {
		return nil
	}
	rootDependencies, err := objectField(root, "dependencies", true)
	if err != nil {
		return fmt.Errorf("repository package dependencies: %w", err)
	}
	names := sortedKeys(generatedDependencies)
	for _, name := range names {
		version, ok := generatedDependencies[name].(string)
		if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return fmt.Errorf("generated dependency %q must have a non-empty string version", name)
		}
		if current, exists := rootDependencies[name]; exists {
			if current != version {
				return fmt.Errorf("repository dependency %q is %#v, but generated Pi runtime requires %q", name, current, version)
			}
			continue
		}
		rootDependencies[name] = version
		ownership.Dependencies = append(ownership.Dependencies, name)
	}
	return nil
}

func mergePiManifest(root, generated map[string]any, output model.RelativePath, packageRoot string, ownership *piOwnership) error {
	generatedPi, err := objectField(generated, "pi", false)
	if err != nil || generatedPi == nil {
		if err == nil {
			err = fmt.Errorf("field is required")
		}
		return fmt.Errorf("generated Pi package pi: %w", err)
	}
	rootPi, err := objectField(root, "pi", true)
	if err != nil {
		return fmt.Errorf("repository package pi: %w", err)
	}

	for _, field := range []struct {
		name      string
		ownership *[]string
		rebase    func(model.RelativePath, string, string) (string, error)
	}{
		{name: "extensions", ownership: &ownership.Extensions, rebase: rebasePiExtension},
		{name: "skills", ownership: &ownership.Skills, rebase: rebasePiResource},
	} {
		generatedValues, err := stringArrayField(generatedPi, field.name, false)
		if err != nil {
			return fmt.Errorf("generated Pi package pi.%s: %w", field.name, err)
		}
		rootValues, err := stringArrayField(rootPi, field.name, false)
		if err != nil {
			return fmt.Errorf("repository package pi.%s: %w", field.name, err)
		}
		for _, value := range generatedValues {
			rebased, err := field.rebase(output, packageRoot, value)
			if err != nil {
				return fmt.Errorf("generated Pi package pi.%s path %q: %w", field.name, value, err)
			}
			if !containsString(rootValues, rebased) {
				rootValues = append(rootValues, rebased)
				*field.ownership = append(*field.ownership, rebased)
			}
		}
		if len(rootValues) != 0 {
			rootPi[field.name] = stringsToAny(rootValues)
		}
	}

	generatedSubagents, err := objectField(generatedPi, "subagents", false)
	if err != nil {
		return fmt.Errorf("generated Pi package pi.subagents: %w", err)
	}
	if generatedSubagents != nil {
		generatedAgents, err := stringArrayField(generatedSubagents, "agents", false)
		if err != nil {
			return fmt.Errorf("generated Pi package pi.subagents.agents: %w", err)
		}
		rootSubagents, err := objectField(rootPi, "subagents", true)
		if err != nil {
			return fmt.Errorf("repository package pi.subagents: %w", err)
		}
		rootAgents, err := stringArrayField(rootSubagents, "agents", false)
		if err != nil {
			return fmt.Errorf("repository package pi.subagents.agents: %w", err)
		}
		for _, value := range generatedAgents {
			rebased, err := rebasePiResource(output, packageRoot, value)
			if err != nil {
				return fmt.Errorf("generated Pi package pi.subagents.agents path %q: %w", value, err)
			}
			if !containsString(rootAgents, rebased) {
				rootAgents = append(rootAgents, rebased)
				ownership.Agents = append(ownership.Agents, rebased)
			}
		}
		if len(rootAgents) != 0 {
			rootSubagents["agents"] = stringsToAny(rootAgents)
		}
	}
	return nil
}

func stripPiOwnership(root map[string]any, ownership *piOwnership) error {
	dependencies, err := objectField(root, "dependencies", false)
	if err != nil {
		return fmt.Errorf("repository package dependencies: %w", err)
	}
	for _, name := range ownership.Dependencies {
		delete(dependencies, name)
	}
	if len(dependencies) == 0 {
		delete(root, "dependencies")
	}

	pi, err := objectField(root, "pi", false)
	if err != nil {
		return fmt.Errorf("repository package pi: %w", err)
	}
	if pi == nil {
		return nil
	}
	for field, owned := range map[string][]string{"extensions": ownership.Extensions, "skills": ownership.Skills} {
		values, err := stringArrayField(pi, field, false)
		if err != nil {
			return fmt.Errorf("repository package pi.%s: %w", field, err)
		}
		values = removeStrings(values, owned)
		if len(values) == 0 {
			delete(pi, field)
		} else {
			pi[field] = stringsToAny(values)
		}
	}
	subagents, err := objectField(pi, "subagents", false)
	if err != nil {
		return fmt.Errorf("repository package pi.subagents: %w", err)
	}
	if subagents != nil {
		agents, err := stringArrayField(subagents, "agents", false)
		if err != nil {
			return fmt.Errorf("repository package pi.subagents.agents: %w", err)
		}
		agents = removeStrings(agents, ownership.Agents)
		if len(agents) == 0 {
			delete(subagents, "agents")
		} else {
			subagents["agents"] = stringsToAny(agents)
		}
		if len(subagents) == 0 {
			delete(pi, "subagents")
		}
	}
	if len(pi) == 0 {
		delete(root, "pi")
	}
	return nil
}

func rebasePiExtension(output model.RelativePath, packageRoot, value string) (string, error) {
	local, err := piPackagePath(value)
	if err != nil {
		return "", err
	}
	if local == "node_modules" || strings.HasPrefix(local, "node_modules/") {
		return "./" + local, nil
	}
	return "./" + path.Join(string(output), string(model.TargetPi), packageRoot, local), nil
}

func rebasePiResource(output model.RelativePath, packageRoot, value string) (string, error) {
	local, err := piPackagePath(value)
	if err != nil {
		return "", err
	}
	return "./" + path.Join(string(output), string(model.TargetPi), packageRoot, local), nil
}

func piPackagePath(value string) (string, error) {
	if !strings.HasPrefix(value, "./") {
		return "", fmt.Errorf("must start with ./")
	}
	local, err := model.NewRelativePath(strings.TrimPrefix(value, "./"))
	if err != nil {
		return "", err
	}
	return string(local), nil
}

func objectField(document map[string]any, key string, create bool) (map[string]any, error) {
	value, exists := document[key]
	if !exists {
		if !create {
			return nil, nil
		}
		result := make(map[string]any)
		document[key] = result
		return result, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	return result, nil
}

func stringArrayField(document map[string]any, key string, create bool) ([]string, error) {
	value, exists := document[key]
	if !exists {
		if create {
			document[key] = []any{}
		}
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("must be an array")
	}
	result := make([]string, len(values))
	for index, item := range values {
		text, ok := item.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("must contain only non-empty strings")
		}
		result[index] = text
	}
	return result, nil
}

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func sortedKeys(values map[string]any) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func removeStrings(values, removed []string) []string {
	set := make(map[string]struct{}, len(removed))
	for _, value := range removed {
		set[value] = struct{}{}
	}
	result := values[:0]
	for _, value := range values {
		if _, remove := set[value]; !remove {
			result = append(result, value)
		}
	}
	return result
}

func readRootRegularFile(workspace string, relative model.RelativePath) ([]byte, error) {
	if _, err := model.NewRelativePath(string(relative)); err != nil {
		return nil, err
	}
	full := filepath.Join(workspace, filepath.FromSlash(string(relative)))
	if err := rejectSymlinkComponents(workspace, full); err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("must be a regular file")
	}
	return os.ReadFile(full)
}
