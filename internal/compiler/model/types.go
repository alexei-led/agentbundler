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
	SourceKindAgentPlugin      SourceKind = "agent-plugin"
)

// TargetID identifies a supported output target.
type TargetID string

const (
	TargetAntigravity  TargetID = "antigravity"
	TargetClaude       TargetID = "claude"
	TargetCodex        TargetID = "codex"
	TargetPi           TargetID = "pi"
	TargetCopilot      TargetID = "copilot"
	TargetGrok         TargetID = "grok"
	TargetCursor       TargetID = "cursor"
	TargetAgentPlugins TargetID = "agent-plugins"
)

// AssetKind classifies normalized assets.
type AssetKind string

const (
	AssetKindSkill          AssetKind = "skill"
	AssetKindAgent          AssetKind = "agent"
	AssetKindHook           AssetKind = "hook"
	AssetKindCommand        AssetKind = "command"
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

// DistributionMetadata stores target-wide distribution JSON metadata.
type DistributionMetadata map[string]any

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
	Environment         []string          `json:"environment,omitempty"`
	Order               int               `json:"order"`
}

// CommandDescriptor describes one portable user-invoked command.
type CommandDescriptor struct {
	Identity    AssetID        `json:"identity"`
	Location    SourceLocation `json:"location"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
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

// NativeResourceOptions declares target-native resource behavior that cannot be
// inferred from the copied file tree. Pi extension paths are package-relative.
type NativeResourceOptions struct {
	PiExtensions []RelativePath `json:"piExtensions,omitempty"`
}

// NativeGap identifies a package-owned source component with target-native behavior.
type NativeGap struct {
	Package   PackageID      `json:"package"`
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

// TargetPackageMode selects separate package roots or one explicit aggregate package.
type TargetPackageMode string

const (
	TargetPackageModeSeparate  TargetPackageMode = "separate"
	TargetPackageModeAggregate TargetPackageMode = "aggregate"
)

// AggregatePackage declares the identity and metadata for one aggregate package.
type AggregatePackage struct {
	Identity PackageID       `json:"identity"`
	Metadata PackageMetadata `json:"metadata"`
}

// TargetComposition contains target-specific composition policy.
type TargetComposition struct {
	Target        TargetID          `json:"target"`
	Profile       TargetProfile     `json:"profile,omitempty"`
	PackageMode   TargetPackageMode `json:"packageMode,omitempty"`
	Aggregate     *AggregatePackage `json:"aggregate,omitempty"`
	SkillPreamble *string           `json:"skillPreamble,omitempty"`
	Capabilities  []CapabilityRule  `json:"capabilities"`
	NativeGaps    []NativeGapPolicy `json:"nativeGaps"`
}

// AgentPluginSourceConfig configures an agent-plugin source.
// Plugins is a non-empty, duplicate-free list of plugin roots relative to
// SourceManifest.Root. Each root must contain a valid plugin.json.
type AgentPluginSourceConfig struct {
	Plugins []RelativePath `json:"plugins"`
}

// MCPTransport identifies the transport type of an MCP server.
type MCPTransport string

const (
	// MCPTransportStdio is the stdio MCP transport.
	MCPTransportStdio MCPTransport = "stdio"
	// MCPTransportStreamableHTTP is the Streamable HTTP MCP transport.
	MCPTransportStreamableHTTP MCPTransport = "streamable-http"
	// MCPTransportSSE is the SSE (legacy remote) MCP transport.
	MCPTransportSSE MCPTransport = "sse"
)

// StdioMCPServer carries the stdio-specific MCP server configuration.
//
// Command is one bare executable token or a plugin-relative ./path.
// Args, Env values, and Cwd may contain single-pass ${PLUGIN_ROOT} and
// ${PLUGIN_DATA} placeholders. The compiler validates permitted forms but
// does not expand them. PLUGIN_ROOT and PLUGIN_DATA keys are reserved.
type StdioMCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
}

// RemoteMCPServer carries configuration for streamable-http and sse MCP servers.
type RemoteMCPServer struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPServer is one semantic MCP server configuration entry in a plugin.
// Exactly one of Stdio or Remote is non-nil, consistent with Transport.
type MCPServer struct {
	Name      string           `json:"name"`
	Transport MCPTransport     `json:"transport"`
	Stdio     *StdioMCPServer  `json:"stdio,omitempty"`
	Remote    *RemoteMCPServer `json:"remote,omitempty"`
	Unknown   map[string]any   `json:"unknown,omitempty"`
}

// PackageFile is one regular file inventoried from an agent plugin package root.
type PackageFile struct {
	Path       RelativePath     `json:"path"`
	Bytes      []byte           `json:"bytes"`
	Executable bool             `json:"executable"`
	SHA256     string           `json:"sha256"`
	Origin     []SourceLocation `json:"origin"`
}

// ClientExtension is one client-specific reverse-domain extension namespace.
type ClientExtension struct {
	Namespace    string        `json:"namespace"`
	Manifest     any           `json:"manifest"`
	PackageFiles []PackageFile `json:"packageFiles"`
}

// AgentPluginManifest holds the typed portable fields from a plugin.json manifest.
type AgentPluginManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

// AgentPluginData carries all decoded plugin data for one agent plugin package.
// It travels from SourcePackage through NormalizedPackage to TargetRenderInput
// unchanged; composition copies it exactly without merging.
type AgentPluginData struct {
	// Profile is the compatibility profile ID used to decode this plugin.
	Profile string `json:"profile"`
	// Manifest holds the typed portable manifest fields.
	Manifest AgentPluginManifest `json:"manifest"`
	// MCPServers holds decoded MCP server configurations in lexical name order.
	MCPServers []MCPServer `json:"mcpServers,omitempty"`
	// Extensions holds client-specific extension namespaces with their manifests
	// and namespaced package files.
	Extensions []ClientExtension `json:"extensions,omitempty"`
	// PackageFiles holds regular package files not owned by manifest, MCP, skills,
	// or extension contracts.
	PackageFiles []PackageFile `json:"packageFiles,omitempty"`
	// UnknownManifest holds top-level plugin.json fields beyond the portable spec.
	// Preserved as semantic JSON values for deterministic round-trip reproduction.
	UnknownManifest map[string]any `json:"unknownManifest,omitempty"`
	// UnknownMCP holds top-level mcp.json fields beyond $schema and mcpServers.
	// Preserved as semantic JSON values for deterministic round-trip reproduction.
	UnknownMCP map[string]any `json:"unknownMCP,omitempty"`
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

// Portable capability keys for agent-plugin components.
// Target adapters declare support for each key through their capability catalog.
// Missing keys are normalized to unsupported; no adapter opts in by default.
const (
	// CapabilityKeyAgentPluginSkills identifies Agent Skills component support.
	CapabilityKeyAgentPluginSkills CapabilityKey = "agent-plugin.skills"
	// CapabilityKeyAgentPluginMCPStdio identifies stdio MCP server transport support.
	CapabilityKeyAgentPluginMCPStdio CapabilityKey = "agent-plugin.mcp.stdio"
	// CapabilityKeyAgentPluginMCPStreamableHTTP identifies Streamable HTTP MCP transport support.
	CapabilityKeyAgentPluginMCPStreamableHTTP CapabilityKey = "agent-plugin.mcp.streamable-http"
	// CapabilityKeyAgentPluginMCPSSE identifies SSE MCP server transport support.
	CapabilityKeyAgentPluginMCPSSE CapabilityKey = "agent-plugin.mcp.sse"
	// CapabilityKeyAgentPluginExtensions identifies client extension namespace support.
	CapabilityKeyAgentPluginExtensions CapabilityKey = "agent-plugin.extensions"
	// CapabilityKeyAgentPluginUnknownJSON identifies permitted-unknown JSON value preservation.
	CapabilityKeyAgentPluginUnknownJSON CapabilityKey = "agent-plugin.unknown-json"
	// CapabilityKeyAgentPluginPackageFiles identifies regular package file support.
	CapabilityKeyAgentPluginPackageFiles CapabilityKey = "agent-plugin.package-files"
)

// CompatibilityConfig opts generated vendor discovery files into the repository root.
type CompatibilityConfig struct {
	RootManifests []TargetID `json:"rootManifests"`
}

// SourceManifest describes one compiler source.
type SourceManifest struct {
	Version          int                           `json:"version"`
	Kind             SourceKind                    `json:"kind"`
	Root             RelativePath                  `json:"root"`
	Targets          []TargetID                    `json:"targets"`
	Output           RelativePath                  `json:"output"`
	Distribution     DistributionMetadata          `json:"distribution,omitempty"`
	Compatibility    *CompatibilityConfig          `json:"compatibility,omitempty"`
	Composition      []TargetComposition           `json:"composition"`
	Bundle           *BundleSourceConfig           `json:"bundle,omitempty"`
	ClaudePlugin     *ClaudePluginSourceConfig     `json:"claudePlugin,omitempty"`
	SkillsRepository *SkillsRepositorySourceConfig `json:"skillsRepository,omitempty"`
	AgentPlugin      *AgentPluginSourceConfig      `json:"agentPlugin,omitempty"`
}

// SourceAsset is an uncomposed source asset.
type SourceAsset struct {
	Identity       AssetID                `json:"identity"`
	Kind           AssetKind              `json:"kind"`
	Targets        []TargetID             `json:"targets,omitempty"`
	Base           AssetContent           `json:"base"`
	Hook           *HookDescriptor        `json:"hook,omitempty"`
	Command        *CommandDescriptor     `json:"command,omitempty"`
	Native         *NativeResourceOptions `json:"native,omitempty"`
	CapabilityUses []CapabilityUse        `json:"capabilityUses"`
	Overlays       []TargetOverlay        `json:"overlays"`
}

// SourcePackage is an uncomposed source package.
type SourcePackage struct {
	Identity    PackageID        `json:"identity"`
	Metadata    PackageMetadata  `json:"metadata"`
	Assets      []SourceAsset    `json:"assets"`
	AgentPlugin *AgentPluginData `json:"agentPlugin,omitempty"`
}

// SourceInventory is the discovered source input.
type SourceInventory struct {
	Packages   []SourcePackage `json:"packages"`
	NativeGaps []NativeGap     `json:"nativeGaps"`
	Inputs     []InputFile     `json:"inputs"`
}

// NormalizedAsset is a composed target-neutral asset.
type NormalizedAsset struct {
	Identity       AssetID                `json:"identity"`
	Kind           AssetKind              `json:"kind"`
	Content        AssetContent           `json:"content"`
	Hook           *HookDescriptor        `json:"hook,omitempty"`
	Command        *CommandDescriptor     `json:"command,omitempty"`
	Native         *NativeResourceOptions `json:"native,omitempty"`
	CapabilityUses []CapabilityUse        `json:"capabilityUses"`
}

// NormalizedPackage is a target-specific composed package.
type NormalizedPackage struct {
	Identity        PackageID         `json:"identity"`
	Metadata        PackageMetadata   `json:"metadata"`
	Target          TargetID          `json:"target"`
	Profile         TargetProfile     `json:"profile,omitempty"`
	Assets          []NormalizedAsset `json:"assets"`
	Acknowledgments []Acknowledgment  `json:"acknowledgments"`
	AgentPlugin     *AgentPluginData  `json:"agentPlugin,omitempty"`
}

// TargetRenderInput contains all target-neutral inputs for one adapter render.
type TargetRenderInput struct {
	Packages     []NormalizedPackage  `json:"packages"`
	Distribution DistributionMetadata `json:"distribution"`
	PackageMode  TargetPackageMode    `json:"packageMode"`
	Aggregate    *AggregatePackage    `json:"aggregate,omitempty"`
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

// ArchiveUnit describes one planned release archive produced from a target's output.
// Root is the plan-relative directory prefix; "." means all target files.
// Stem is combined with the distribution name to form the archive basename.
// Suffix is the archive file extension (".tar.gz" or ".tgz").
type ArchiveUnit struct {
	Root   string `json:"root"`
	Stem   string `json:"stem"`
	Suffix string `json:"suffix"`
}

// TargetPlan describes all generated files and checks for one target.
type TargetPlan struct {
	Target       TargetID      `json:"target"`
	Packages     []PackageID   `json:"packages"`
	Files        []PlannedFile `json:"files"`
	NativeChecks []NativeCheck `json:"nativeChecks"`
	ArchiveUnits []ArchiveUnit `json:"archiveUnits,omitempty"`
}

// BuildPlan is the complete write and verification transaction.
type BuildPlan struct {
	Targets       []TargetPlan  `json:"targets"`
	CompilerFiles []PlannedFile `json:"compilerFiles"`
}
