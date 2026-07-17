package packageoutput

import (
	"fmt"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/target/marketplace"
)

// Codec isolates target-specific package rules and serialization from package
// aggregation and filesystem layout.
type Codec struct {
	Target          model.TargetID
	ManifestPath    string
	AgentRoot       string
	HookPayloadRoot string
	Capabilities    []model.CapabilityRule
	Manifest        func(model.NormalizedPackage) ([]byte, error)
	Agent           func(model.NormalizedAsset) ([]byte, string, error)
	NativeResource  func(model.NormalizedAsset) ([]NativeResourceFile, error)
	Hooks           func(HookRenderInput) (HookManifest, error)
	Catalog         func(marketplace.Catalog) (CatalogManifest, error)
	ValidatePackage func(model.NormalizedPackage) []model.Diagnostic
}

// NativeResourceFile is one package-relative native-resource output file.
type NativeResourceFile struct {
	Path    model.RelativePath
	Content model.FileContent
}

// HookRenderInput is an immutable package hook view for target-owned serialization.
type HookRenderInput struct {
	packageID model.PackageID
	hooks     []HookInput
}

// PackageID returns the package that owns these hooks.
func (input HookRenderInput) PackageID() model.PackageID { return input.packageID }

// Hooks returns detached immutable views in portable hook order.
func (input HookRenderInput) Hooks() []HookInput {
	return cloneHookInputs(input.hooks)
}

// HookInput is an immutable hook descriptor and payload view.
type HookInput struct {
	descriptor   model.HookDescriptor
	capabilities []model.CapabilityUse
	payloadRoot  model.RelativePath
	payload      []HookPayloadFile
}

// Descriptor returns a detached copy of the portable hook descriptor.
func (input HookInput) Descriptor() model.HookDescriptor {
	return cloneHookDescriptor(input.descriptor)
}

// CapabilityUses returns the detached capability uses declared by this hook.
func (input HookInput) CapabilityUses() []model.CapabilityUse {
	return model.CloneCapabilityUses(input.capabilities)
}

// PayloadRoot returns the package-relative directory containing this hook's payload.
func (input HookInput) PayloadRoot() model.RelativePath { return input.payloadRoot }

// PayloadFiles returns detached payload file views ordered by package-relative path.
func (input HookInput) PayloadFiles() []HookPayloadFile {
	return cloneHookPayloadFiles(input.payload)
}

// HookPayloadFile is an immutable hook payload file view.
type HookPayloadFile struct {
	path        model.RelativePath
	packagePath model.RelativePath
	bytes       []byte
	executable  bool
	origin      []model.SourceLocation
}

// Path returns the path relative to the hook payload root.
func (file HookPayloadFile) Path() model.RelativePath { return file.path }

// PackagePath returns the path relative to the installable package root.
func (file HookPayloadFile) PackagePath() model.RelativePath { return file.packagePath }

// Bytes returns a detached copy of the payload bytes.
func (file HookPayloadFile) Bytes() []byte { return append([]byte(nil), file.bytes...) }

// Executable reports the payload file's explicit executable intent.
func (file HookPayloadFile) Executable() bool { return file.executable }

// Origin returns detached source evidence for the payload file.
func (file HookPayloadFile) Origin() []model.SourceLocation {
	return cloneSourceLocations(file.origin)
}

// HookManifest is one target-owned native hook manifest.
type HookManifest struct {
	Path  model.RelativePath
	Bytes []byte
}

// CatalogManifest is one target-owned native marketplace manifest.
type CatalogManifest struct {
	Path  model.RelativePath
	Bytes []byte
}

func cloneHookInputs(inputs []HookInput) []HookInput {
	if inputs == nil {
		return nil
	}
	clones := make([]HookInput, len(inputs))
	for index, input := range inputs {
		clones[index] = HookInput{
			descriptor:   cloneHookDescriptor(input.descriptor),
			capabilities: model.CloneCapabilityUses(input.capabilities),
			payloadRoot:  input.payloadRoot,
			payload:      cloneHookPayloadFiles(input.payload),
		}
	}
	return clones
}

func cloneHookPayloadFiles(files []HookPayloadFile) []HookPayloadFile {
	if files == nil {
		return nil
	}
	clones := make([]HookPayloadFile, len(files))
	for index, file := range files {
		clones[index] = HookPayloadFile{
			path:        file.path,
			packagePath: file.packagePath,
			bytes:       append([]byte(nil), file.bytes...),
			executable:  file.executable,
			origin:      cloneSourceLocations(file.origin),
		}
	}
	return clones
}

func cloneHookDescriptor(descriptor model.HookDescriptor) model.HookDescriptor {
	return model.CloneHookDescriptor(descriptor)
}

func cloneSourceLocations(locations []model.SourceLocation) []model.SourceLocation {
	return model.CloneSourceLocations(locations)
}

func cloneSourceLocation(location model.SourceLocation) model.SourceLocation {
	return model.CloneSourceLocation(location)
}

// UnsupportedAgentFieldError reports an agent field a target codec cannot encode.
type UnsupportedAgentFieldError struct {
	Target model.TargetID
	Asset  model.AssetID
	Field  string
}

func (e *UnsupportedAgentFieldError) Error() string {
	return fmt.Sprintf("agent %q field %q is unsupported by target %q", e.Asset, e.Field, e.Target)
}

func (e *UnsupportedAgentFieldError) Diagnostic() model.Diagnostic {
	return model.Diagnostic{
		Code:     "unsupported-agent-field",
		Severity: model.SeverityError,
		Message:  e.Error(),
		Hint:     `move target-only agent fields into <agent-directory>/.agentbundler/targets/<target>.json using frontmatterPatch`,
		Asset:    e.Asset,
		Field:    e.Field,
		Targets:  []model.TargetID{e.Target},
	}
}
