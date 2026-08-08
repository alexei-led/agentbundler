package agentplugins

// MCPTransportType identifies the transport type of an MCP server.
type MCPTransportType string

const (
	// MCPTransportStdio is the stdio transport.
	MCPTransportStdio MCPTransportType = "stdio"

	// MCPTransportStreamableHTTP is the Streamable HTTP transport.
	MCPTransportStreamableHTTP MCPTransportType = "streamable-http"

	// MCPTransportSSE is the SSE (legacy remote) transport.
	MCPTransportSSE MCPTransportType = "sse"
)

// MCPConfig is the decoded wire representation of a mcp.json file.
//
// Schema is always MCPSchemaURL. Servers holds each named server entry.
// Unknown holds mcp.json top-level members beyond $schema and mcpServers.
type MCPConfig struct {
	// Schema is the $schema field value. Must equal MCPSchemaURL.
	Schema string

	// Servers holds each named MCP server in lexical name order.
	Servers []MCPServer

	// Unknown holds top-level JSON members not defined by the 1.0.0 portable spec.
	Unknown map[string]any
}

// MCPServer is one decoded MCP server entry from mcpServers.
//
// Name is the key in the mcpServers object. Exactly one of Stdio or Remote
// is non-nil, consistent with Transport.
type MCPServer struct {
	// Name is the server's key in the mcpServers object.
	Name string

	// Transport identifies the active transport.
	Transport MCPTransportType

	// Stdio is non-nil for stdio transport.
	Stdio *StdioTransport

	// Remote is non-nil for streamable-http or sse transport.
	Remote *RemoteTransport

	// Unknown holds server-level JSON members beyond the defined transport fields.
	Unknown map[string]any
}

// StdioTransport carries the stdio-specific MCP server configuration.
//
// Command is one bare executable token or a plugin-relative ./path.
// Args, Env values, and Cwd may contain single-pass ${PLUGIN_ROOT} and
// ${PLUGIN_DATA} placeholders. The compiler validates their permitted forms
// but does not expand them. PLUGIN_ROOT and PLUGIN_DATA keys are reserved
// and must not appear in Env.
type StdioTransport struct {
	// Command is the bare executable name or plugin-relative ./path.
	Command string

	// Args is the ordered list of command arguments.
	Args []string

	// Env is the map of environment variable overrides.
	Env map[string]string

	// Cwd is the working directory. Empty means the plugin root.
	Cwd string
}

// RemoteTransport carries the configuration for streamable-http and sse servers.
type RemoteTransport struct {
	// URL is the HTTP or HTTPS endpoint URL.
	URL string

	// Headers is the map of fixed HTTP request headers.
	Headers map[string]string
}
