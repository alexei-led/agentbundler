// Package model defines the target-neutral data exchanged by compiler subsystems.
package model

// RelativePath is a normalized, non-empty path below a declared root.
type RelativePath string

// PackageID is a stable package identity.
type PackageID string

// AssetID is a stable asset identity in kind/name form.
type AssetID string

// CapabilityKey is a canonical capability identifier.
type CapabilityKey string

// SourceKind identifies a supported source layout.
type SourceKind string

const (
	SourceKindBundle           SourceKind = "bundle"
	SourceKindClaudePlugin     SourceKind = "claude-plugin"
	SourceKindSkillsRepository SourceKind = "skills-repository"
)

// TargetID identifies a supported output target.
type TargetID string

const (
	TargetClaude  TargetID = "claude"
	TargetCodex   TargetID = "codex"
	TargetPi      TargetID = "pi"
	TargetCopilot TargetID = "copilot"
	TargetGrok    TargetID = "grok"
	TargetCursor  TargetID = "cursor"
)

// AssetKind classifies normalized assets.
type AssetKind string

const (
	AssetKindSkill          AssetKind = "skill"
	AssetKindAgent          AssetKind = "agent"
	AssetKindHook           AssetKind = "hook"
	AssetKindResource       AssetKind = "resource"
	AssetKindNativeResource AssetKind = "native-resource"
)

// CapabilityState describes how a target handles a capability.
type CapabilityState string

const (
	CapabilityStateNative      CapabilityState = "native"
	CapabilityStateEquivalent  CapabilityState = "equivalent"
	CapabilityStateAdvisory    CapabilityState = "advisory"
	CapabilityStateUnsupported CapabilityState = "unsupported"
)

// Severity classifies a diagnostic.
type Severity string

const (
	SeverityError       Severity = "error"
	SeverityWarning     Severity = "warning"
	SeverityInformation Severity = "information"
)

// BodyMode selects the active BodyPatch payload.
type BodyMode string

const (
	BodyModeReplace  BodyMode = "replace"
	BodyModeSections BodyMode = "sections"
)

// NativeGapAction specifies how composition handles a native-only gap.
type NativeGapAction string

const (
	NativeGapActionReplace    NativeGapAction = "replace"
	NativeGapActionExclude    NativeGapAction = "exclude"
	NativeGapActionSourceOnly NativeGapAction = "source-only"
)

// SourceLocation identifies a position in a source file.
type SourceLocation struct {
	Path   RelativePath `json:"path"`
	Line   *int         `json:"line"`
	Column *int         `json:"column"`
}

// InputFile identifies a source input by path and SHA-256 digest.
type InputFile struct {
	Path   RelativePath `json:"path"`
	SHA256 string       `json:"sha256"`
}

// PackageMetadata stores source package JSON metadata.
type PackageMetadata map[string]any

// FileContent is one source payload file with its mode and source evidence.
type FileContent struct {
	Bytes      []byte           `json:"bytes"`
	Executable bool             `json:"executable"`
	Origin     []SourceLocation `json:"origin"`
}

// AssetContent is an asset's frontmatter, body, and sidecar files.
type AssetContent struct {
	Frontmatter map[string]any               `json:"frontmatter"`
	Body        string                       `json:"body"`
	Files       map[RelativePath]FileContent `json:"files"`
}

// SectionPatch replaces the body under a heading path.
type SectionPatch struct {
	HeadingPath []string `json:"headingPath"`
	Body        string   `json:"body"`
}

// BodyPatch changes an asset body.
type BodyPatch struct {
	Mode     BodyMode       `json:"mode"`
	Text     *string        `json:"text,omitempty"`
	Sections []SectionPatch `json:"sections"`
}

// FilePatch replaces one asset sidecar file.
type FilePatch struct {
	Path    RelativePath `json:"path"`
	Content FileContent  `json:"content"`
}

// HookEvent is a portable lifecycle event.
type HookEvent string

const (
	HookEventSessionStart    HookEvent = "session-start"
	HookEventSessionEnd      HookEvent = "session-end"
	HookEventPromptSubmit    HookEvent = "prompt-submit"
	HookEventPreTool         HookEvent = "pre-tool"
	HookEventPostTool        HookEvent = "post-tool"
	HookEventPostToolFailure HookEvent = "post-tool-failure"
	HookEventStop            HookEvent = "stop"
	HookEventNotification    HookEvent = "notification"
	HookEventPreCompact      HookEvent = "pre-compact"
	HookEventPostCompact     HookEvent = "post-compact"
)

// HookToolCategory is a canonical portable tool category.
type HookToolCategory string

const (
	HookToolCategoryCommand HookToolCategory = "command"
	HookToolCategoryRead    HookToolCategory = "read"
	HookToolCategoryWrite   HookToolCategory = "write"
	HookToolCategoryEdit    HookToolCategory = "edit"
	HookToolCategorySearch  HookToolCategory = "search"
	HookToolCategoryWeb     HookToolCategory = "web"
	HookToolCategoryTask    HookToolCategory = "task"
	HookToolCategoryMCP     HookToolCategory = "mcp"
	HookToolCategoryOther   HookToolCategory = "other"
)

// HookMatcher restricts a tool event to canonical tool categories.
type HookMatcher struct {
	Tools []HookToolCategory `json:"tools"`
}

// HookHandlerMode selects the active command representation.
type HookHandlerMode string

const (
	HookHandlerModeExec  HookHandlerMode = "exec"
	HookHandlerModeShell HookHandlerMode = "shell"
)

// HookArgument is exactly one literal value or package-file reference.
type HookArgument struct {
	Literal     *string       `json:"literal,omitempty"`
	PackageFile *RelativePath `json:"packageFile,omitempty"`
}

// HookCommand describes an exec or explicit shell handler.
type HookCommand struct {
	Mode         HookHandlerMode `json:"mode"`
	Program      *string         `json:"program,omitempty"`
	Arguments    []HookArgument  `json:"arguments"`
	ShellCommand *string         `json:"shellCommand,omitempty"`
}

// HookFailurePolicy controls handler failure behavior.
type HookFailurePolicy string

const (
	HookFailurePolicyOpen   HookFailurePolicy = "open"
	HookFailurePolicyClosed HookFailurePolicy = "closed"
)

// MaxHookTimeoutMilliseconds is the portable hook timeout ceiling.
const MaxHookTimeoutMilliseconds = 600_000

// HookDescriptor is one typed portable command hook.
type HookDescriptor struct {
	Identity            AssetID           `json:"identity"`
	Location            SourceLocation    `json:"location"`
	Event               HookEvent         `json:"event"`
	Matcher             *HookMatcher      `json:"matcher,omitempty"`
	Handler             HookCommand       `json:"handler"`
	TimeoutMilliseconds int               `json:"timeoutMilliseconds"`
	Asynchronous        bool              `json:"asynchronous"`
	FailurePolicy       HookFailurePolicy `json:"failurePolicy"`
	Order               int               `json:"order"`
}

// TargetOverlay describes target-specific asset changes.
type TargetOverlay struct {
	Target           TargetID         `json:"target"`
	FrontmatterPatch *map[string]any  `json:"frontmatterPatch,omitempty"`
	BodyPatch        *BodyPatch       `json:"bodyPatch,omitempty"`
	Files            []FilePatch      `json:"files"`
	DeletedFiles     []RelativePath   `json:"deletedFiles"`
	Acknowledgments  []Acknowledgment `json:"acknowledgments"`
}

// NativeGap identifies a source component with target-native behavior.
type NativeGap struct {
	Component string         `json:"component"`
	Asset     *AssetID       `json:"asset,omitempty"`
	Location  SourceLocation `json:"location"`
	Target    *TargetID      `json:"target,omitempty"`
}

// Acknowledgment records an accepted capability loss.
type Acknowledgment struct {
	Asset  AssetID       `json:"asset"`
	Target TargetID      `json:"target"`
	Key    CapabilityKey `json:"key"`
	Reason string        `json:"reason"`
}

// CapabilityUse identifies an asset capability use.
type CapabilityUse struct {
	Key      CapabilityKey  `json:"key"`
	Location SourceLocation `json:"location"`
}

// CapabilityRule assigns a target state to a capability.
type CapabilityRule struct {
	Key   CapabilityKey   `json:"key"`
	State CapabilityState `json:"state"`
}

// NativeGapPolicy describes how one native gap is handled for a target.
type NativeGapPolicy struct {
	Component   string          `json:"component"`
	Action      NativeGapAction `json:"action"`
	Replacement *AssetID        `json:"replacement,omitempty"`
}

// TargetProfile selects the output contract for a target.
type TargetProfile string

const (
	TargetProfileProject TargetProfile = "project"
	TargetProfilePackage TargetProfile = "package"
)

// TargetComposition contains target-specific composition policy.
type TargetComposition struct {
	Target        TargetID          `json:"target"`
	Profile       TargetProfile     `json:"profile,omitempty"`
	SkillPreamble *string           `json:"skillPreamble,omitempty"`
	Capabilities  []CapabilityRule  `json:"capabilities"`
	NativeGaps    []NativeGapPolicy `json:"nativeGaps"`
}

// BundleSourceConfig configures a bundle source.
type BundleSourceConfig struct {
	Packages []RelativePath `json:"packages"`
}

// ClaudePluginSourceConfig configures a Claude plugin source.
type ClaudePluginSourceConfig struct {
	PluginRoot RelativePath `json:"pluginRoot"`
}

// SkillsRepositorySourceConfig configures a skills repository source.
type SkillsRepositorySourceConfig struct {
	Package  PackageID       `json:"package"`
	Roots    []RelativePath  `json:"roots"`
	Metadata PackageMetadata `json:"metadata"`
}

// SourceManifest describes one compiler source.
type SourceManifest struct {
	Version          int                           `json:"version"`
	Kind             SourceKind                    `json:"kind"`
	Root             RelativePath                  `json:"root"`
	Targets          []TargetID                    `json:"targets"`
	Output           RelativePath                  `json:"output"`
	Composition      []TargetComposition           `json:"composition"`
	Bundle           *BundleSourceConfig           `json:"bundle,omitempty"`
	ClaudePlugin     *ClaudePluginSourceConfig     `json:"claudePlugin,omitempty"`
	SkillsRepository *SkillsRepositorySourceConfig `json:"skillsRepository,omitempty"`
}

// SourceAsset is an uncomposed source asset.
type SourceAsset struct {
	Identity       AssetID         `json:"identity"`
	Kind           AssetKind       `json:"kind"`
	Targets        []TargetID      `json:"targets,omitempty"`
	Base           AssetContent    `json:"base"`
	Hook           *HookDescriptor `json:"hook,omitempty"`
	CapabilityUses []CapabilityUse `json:"capabilityUses"`
	Overlays       []TargetOverlay `json:"overlays"`
}

// SourcePackage is an uncomposed source package.
type SourcePackage struct {
	Identity PackageID       `json:"identity"`
	Metadata PackageMetadata `json:"metadata"`
	Assets   []SourceAsset   `json:"assets"`
}

// SourceInventory is the discovered source input.
type SourceInventory struct {
	Packages   []SourcePackage `json:"packages"`
	NativeGaps []NativeGap     `json:"nativeGaps"`
	Inputs     []InputFile     `json:"inputs"`
}

// NormalizedAsset is a composed target-neutral asset.
type NormalizedAsset struct {
	Identity       AssetID         `json:"identity"`
	Kind           AssetKind       `json:"kind"`
	Content        AssetContent    `json:"content"`
	Hook           *HookDescriptor `json:"hook,omitempty"`
	CapabilityUses []CapabilityUse `json:"capabilityUses"`
}

// NormalizedPackage is a target-specific composed package.
type NormalizedPackage struct {
	Identity        PackageID         `json:"identity"`
	Metadata        PackageMetadata   `json:"metadata"`
	Target          TargetID          `json:"target"`
	Profile         TargetProfile     `json:"profile,omitempty"`
	Assets          []NormalizedAsset `json:"assets"`
	Acknowledgments []Acknowledgment  `json:"acknowledgments"`
}

// Diagnostic reports a model validation problem.
type Diagnostic struct {
	Code     string          `json:"code"`
	Severity Severity        `json:"severity"`
	Location *SourceLocation `json:"location,omitempty"`
	Message  string          `json:"message"`
	Hint     string          `json:"hint,omitempty"`
	Asset    AssetID         `json:"asset,omitempty"`
	Field    string          `json:"field,omitempty"`
	Targets  []TargetID      `json:"targets,omitempty"`
}

// PlannedFile describes one generated file.
type PlannedFile struct {
	Path       RelativePath     `json:"path"`
	Bytes      []byte           `json:"bytes"`
	Executable bool             `json:"executable"`
	Origin     []SourceLocation `json:"origin"`
}

// NativeCheck describes an external verification command.
type NativeCheck struct {
	Program          string         `json:"program"`
	Arguments        []string       `json:"arguments"`
	WorkingDirectory *RelativePath  `json:"workingDirectory,omitempty"`
	Location         SourceLocation `json:"location"`
}

// TargetPlan describes all generated files and checks for one target.
type TargetPlan struct {
	Target       TargetID      `json:"target"`
	Packages     []PackageID   `json:"packages"`
	Files        []PlannedFile `json:"files"`
	NativeChecks []NativeCheck `json:"nativeChecks"`
}

// BuildPlan is the complete write and verification transaction.
type BuildPlan struct {
	Targets       []TargetPlan  `json:"targets"`
	CompilerFiles []PlannedFile `json:"compilerFiles"`
}
