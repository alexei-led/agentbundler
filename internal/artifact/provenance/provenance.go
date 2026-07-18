// Package provenance adds deterministic build evidence to compiler plans.
package provenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

const provenancePath = ".agentbundler/build.json"

// InputFile identifies an input included in build provenance.
type InputFile struct {
	Path   model.RelativePath
	SHA256 string
}

// Acknowledgment records an accepted advisory capability in build provenance.
type Acknowledgment struct {
	Asset  string
	Target model.TargetID
	Key    string
	Reason string
}

// AdapterRevision identifies the output format revision for a target.
type AdapterRevision struct {
	Target   model.TargetID
	Revision int
}

// Input contains the evidence recorded in build provenance.
type Input struct {
	CompilerVersion  string
	Configuration    json.RawMessage
	Inputs           []InputFile
	Acknowledgments  []Acknowledgment
	AdapterRevisions []AdapterRevision
}

// Append validates input and adds deterministic provenance to a deep copy of plan.
func Append(plan model.BuildPlan, input Input) (model.BuildPlan, error) {
	if diagnostics := model.ValidateBuildPlan(plan); len(diagnostics) != 0 {
		return model.BuildPlan{}, fmt.Errorf("invalid build plan: %s", diagnostics[0].Message)
	}
	if err := rejectProvenanceCollision(plan); err != nil {
		return model.BuildPlan{}, err
	}

	configuration, err := compactConfiguration(input.Configuration)
	if err != nil {
		return model.BuildPlan{}, fmt.Errorf("invalid configuration: %w", err)
	}
	if err := validateInput(input, plan.Targets); err != nil {
		return model.BuildPlan{}, err
	}

	provenance, err := marshalProvenance(plan, input, configuration)
	if err != nil {
		return model.BuildPlan{}, fmt.Errorf("marshal provenance: %w", err)
	}

	result := cloneBuildPlan(plan)
	result.CompilerFiles = append(result.CompilerFiles, model.PlannedFile{
		Path:  model.RelativePath(provenancePath),
		Bytes: provenance,
	})
	return result, nil
}

func rejectProvenanceCollision(plan model.BuildPlan) error {
	for _, target := range plan.Targets {
		for _, file := range target.Files {
			if file.Path == provenancePath {
				return fmt.Errorf("provenance path %q already exists in target %q", provenancePath, target.Target)
			}
		}
	}
	for _, file := range plan.CompilerFiles {
		if file.Path == provenancePath {
			return fmt.Errorf("provenance path %q already exists in compiler files", provenancePath)
		}
	}
	return nil
}

func validateInput(input Input, targets []model.TargetPlan) error {
	if input.CompilerVersion == "" || !utf8.ValidString(input.CompilerVersion) {
		return fmt.Errorf("compiler version must be non-empty valid UTF-8")
	}

	inputPaths := make(map[model.RelativePath]struct{}, len(input.Inputs))
	for _, file := range input.Inputs {
		if _, err := model.NewRelativePath(string(file.Path)); err != nil {
			return fmt.Errorf("input path %q: %w", file.Path, err)
		}
		if !validSHA256(file.SHA256) {
			return fmt.Errorf("input %q has invalid SHA-256", file.Path)
		}
		if _, exists := inputPaths[file.Path]; exists {
			return fmt.Errorf("input path %q is duplicated", file.Path)
		}
		inputPaths[file.Path] = struct{}{}
	}

	for _, acknowledgment := range input.Acknowledgments {
		if !validString(acknowledgment.Asset) || !validString(acknowledgment.Key) || !validString(acknowledgment.Reason) {
			return fmt.Errorf("acknowledgment fields must be non-empty valid UTF-8")
		}
		if !validTarget(acknowledgment.Target) {
			return fmt.Errorf("acknowledgment target %q is invalid", acknowledgment.Target)
		}
	}

	revisions := make(map[model.TargetID]struct{}, len(input.AdapterRevisions))
	for _, revision := range input.AdapterRevisions {
		if !validTarget(revision.Target) {
			return fmt.Errorf("adapter revision target %q is invalid", revision.Target)
		}
		if _, exists := revisions[revision.Target]; exists {
			return fmt.Errorf("adapter revision target %q is duplicated", revision.Target)
		}
		revisions[revision.Target] = struct{}{}
	}
	if len(revisions) != len(targets) {
		return fmt.Errorf("adapter revisions must cover every selected target exactly once")
	}
	for _, target := range targets {
		if _, exists := revisions[target.Target]; !exists {
			return fmt.Errorf("adapter revision for target %q is missing", target.Target)
		}
	}
	return nil
}

func compactConfiguration(configuration json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(configuration))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("must be a JSON object")
	}
	nonEmpty, err := scanJSONObject(decoder)
	if err != nil {
		return nil, err
	}
	if !nonEmpty {
		return nil, fmt.Errorf("must be a non-empty JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("must contain exactly one JSON value")
		}
		return nil, err
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, configuration); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}

func scanJSONObject(decoder *json.Decoder) (bool, error) {
	keys := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false, err
		}
		key, ok := token.(string)
		if !ok {
			return false, fmt.Errorf("invalid JSON object key")
		}
		if _, exists := keys[key]; exists {
			return false, fmt.Errorf("duplicate JSON key %q", key)
		}
		keys[key] = struct{}{}
		if err := scanJSONValue(decoder); err != nil {
			return false, err
		}
	}
	if token, err := decoder.Token(); err != nil {
		return false, err
	} else if delimiter, ok := token.(json.Delim); !ok || delimiter != '}' {
		return false, fmt.Errorf("invalid JSON object")
	}
	return len(keys) != 0, nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		_, err := scanJSONObject(decoder)
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != ']' {
			return fmt.Errorf("invalid JSON array")
		}
		return nil
	default:
		return fmt.Errorf("invalid JSON delimiter %q", delimiter)
	}
}

func marshalProvenance(plan model.BuildPlan, input Input, configuration []byte) ([]byte, error) {
	inputs := make([]provenanceInputFile, len(input.Inputs))
	for index, file := range input.Inputs {
		inputs[index] = provenanceInputFile{Path: string(file.Path), SHA256: file.SHA256}
	}
	sort.Slice(inputs, func(left, right int) bool {
		return inputs[left].Path < inputs[right].Path
	})

	acknowledgments := make([]provenanceAcknowledgment, len(input.Acknowledgments))
	for index, acknowledgment := range input.Acknowledgments {
		acknowledgments[index] = provenanceAcknowledgment{
			Asset:  acknowledgment.Asset,
			Target: string(acknowledgment.Target),
			Key:    acknowledgment.Key,
			Reason: acknowledgment.Reason,
		}
	}
	sort.Slice(acknowledgments, func(left, right int) bool {
		return compareAcknowledgments(acknowledgments[left], acknowledgments[right]) < 0
	})

	revisions := make(map[model.TargetID]int, len(input.AdapterRevisions))
	for _, revision := range input.AdapterRevisions {
		revisions[revision.Target] = revision.Revision
	}
	outputs := make([]provenanceOutput, len(plan.Targets))
	for index, target := range plan.Targets {
		files := make([]provenanceOutputFile, len(target.Files))
		for fileIndex, file := range target.Files {
			files[fileIndex] = provenanceOutputFile{
				Path:       string(file.Path),
				SHA256:     digest(file.Bytes),
				Executable: file.Executable,
			}
		}
		sort.Slice(files, func(left, right int) bool {
			return files[left].Path < files[right].Path
		})
		outputs[index] = provenanceOutput{
			Target:          string(target.Target),
			AdapterRevision: revisions[target.Target],
			Files:           files,
		}
	}
	sort.Slice(outputs, func(left, right int) bool {
		return outputs[left].Target < outputs[right].Target
	})

	return json.Marshal(provenanceDocument{
		SchemaVersion:   1,
		Compiler:        provenanceCompiler{Version: input.CompilerVersion},
		Configuration:   provenanceDigest{SHA256: digest(configuration)},
		Inputs:          inputs,
		Acknowledgments: acknowledgments,
		Outputs:         outputs,
	})
}

func compareAcknowledgments(left, right provenanceAcknowledgment) int {
	for _, values := range [][2]string{
		{left.Asset, right.Asset},
		{left.Target, right.Target},
		{left.Key, right.Key},
		{left.Reason, right.Reason},
	} {
		if values[0] < values[1] {
			return -1
		}
		if values[0] > values[1] {
			return 1
		}
	}
	return 0
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		digit := character >= '0' && character <= '9'
		lowerHex := character >= 'a' && character <= 'f'
		if !digit && !lowerHex {
			return false
		}
	}
	return true
}

func validString(value string) bool {
	return value != "" && utf8.ValidString(value)
}

func validTarget(target model.TargetID) bool {
	switch target {
	case model.TargetAntigravity, model.TargetClaude, model.TargetCodex, model.TargetPi, model.TargetCopilot, model.TargetGrok, model.TargetCursor:
		return true
	default:
		return false
	}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func cloneBuildPlan(plan model.BuildPlan) model.BuildPlan {
	result := model.BuildPlan{
		Targets:       make([]model.TargetPlan, len(plan.Targets)),
		CompilerFiles: clonePlannedFiles(plan.CompilerFiles),
	}
	for index, target := range plan.Targets {
		result.Targets[index] = model.TargetPlan{
			Target:       target.Target,
			Packages:     append([]model.PackageID(nil), target.Packages...),
			Files:        clonePlannedFiles(target.Files),
			NativeChecks: cloneNativeChecks(target.NativeChecks),
		}
	}
	return result
}

func clonePlannedFiles(files []model.PlannedFile) []model.PlannedFile {
	if files == nil {
		return nil
	}
	result := make([]model.PlannedFile, len(files))
	for index, file := range files {
		result[index] = model.PlannedFile{
			Path:       file.Path,
			Bytes:      cloneBytes(file.Bytes),
			Executable: file.Executable,
			Origin:     cloneLocations(file.Origin),
		}
	}
	return result
}

func cloneNativeChecks(checks []model.NativeCheck) []model.NativeCheck {
	if checks == nil {
		return nil
	}
	result := make([]model.NativeCheck, len(checks))
	for index, check := range checks {
		result[index] = model.NativeCheck{
			Program:          check.Program,
			Arguments:        append([]string(nil), check.Arguments...),
			WorkingDirectory: cloneRelativePath(check.WorkingDirectory),
			Location:         cloneLocation(check.Location),
		}
	}
	return result
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	result := make([]byte, len(value))
	copy(result, value)
	return result
}

func cloneLocations(locations []model.SourceLocation) []model.SourceLocation {
	if locations == nil {
		return nil
	}
	result := make([]model.SourceLocation, len(locations))
	for index, location := range locations {
		result[index] = cloneLocation(location)
	}
	return result
}

func cloneLocation(location model.SourceLocation) model.SourceLocation {
	return model.SourceLocation{
		Path:   location.Path,
		Line:   cloneInt(location.Line),
		Column: cloneInt(location.Column),
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneRelativePath(value *model.RelativePath) *model.RelativePath {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

type provenanceDocument struct {
	SchemaVersion   int                        `json:"schemaVersion"`
	Compiler        provenanceCompiler         `json:"compiler"`
	Configuration   provenanceDigest           `json:"configuration"`
	Inputs          []provenanceInputFile      `json:"inputs"`
	Acknowledgments []provenanceAcknowledgment `json:"acknowledgments"`
	Outputs         []provenanceOutput         `json:"outputs"`
}

type provenanceCompiler struct {
	Version string `json:"version"`
}

type provenanceDigest struct {
	SHA256 string `json:"sha256"`
}

type provenanceInputFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type provenanceAcknowledgment struct {
	Asset  string `json:"asset"`
	Target string `json:"target"`
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

type provenanceOutput struct {
	Target          string                 `json:"target"`
	AdapterRevision int                    `json:"adapterRevision"`
	Files           []provenanceOutputFile `json:"files"`
}

type provenanceOutputFile struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Executable bool   `json:"executable"`
}
