package agentplugins

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// reservedEnvKeys is the set of environment variable keys that plugins cannot
// override. Clients inject these at runtime.
var reservedEnvKeys = map[string]bool{
	"PLUGIN_ROOT": true,
	"PLUGIN_DATA": true,
}

// ValidatePluginManifest validates the semantic field rules for a decoded manifest.
// It does not access the filesystem or network.
func ValidatePluginManifest(manifest PluginManifest) []Diagnostic {
	var diagnostics []Diagnostic

	// Schema selector must be present and correct.
	if manifest.Schema != PluginSchemaURL {
		diagnostics = append(diagnostics, diag("invalid-schema", "$schema",
			fmt.Sprintf("$schema must be %q", PluginSchemaURL)))
	}

	// Name is required and must conform to the naming rules.
	diagnostics = append(diagnostics, validatePluginName(manifest.Name)...)

	// Optional URL fields.
	if manifest.Homepage != "" {
		diagnostics = append(diagnostics, validateURIField("homepage", manifest.Homepage)...)
	}
	if manifest.Repository != "" {
		diagnostics = append(diagnostics, validateURIField("repository", manifest.Repository)...)
	}

	// Extension namespaces must be valid reverse-domain identifiers.
	for ns := range manifest.Extensions {
		if err := validateReverseDomainNamespace(ns); err != nil {
			diagnostics = append(diagnostics, diag("invalid-namespace", "extensions."+ns, err.Error()))
		}
	}

	return diagnostics
}

// ValidateMCPConfig validates the semantic field rules for a decoded MCP config.
// It does not access the filesystem or network.
func ValidateMCPConfig(config MCPConfig) []Diagnostic {
	var diagnostics []Diagnostic

	if config.Schema != MCPSchemaURL {
		diagnostics = append(diagnostics, diag("invalid-schema", "$schema",
			fmt.Sprintf("$schema must be %q", MCPSchemaURL)))
	}

	if len(config.Servers) == 0 {
		diagnostics = append(diagnostics, diag("missing-required", "mcpServers", "mcpServers must contain at least one server"))
	}

	for _, server := range config.Servers {
		diagnostics = append(diagnostics, validateMCPServer(server)...)
	}

	return diagnostics
}

// validateMCPServer validates one decoded server entry.
func validateMCPServer(server MCPServer) []Diagnostic {
	prefix := "mcpServers." + server.Name
	var diagnostics []Diagnostic

	if strings.TrimSpace(server.Name) == "" {
		diagnostics = append(diagnostics, diag("invalid-field", "mcpServers", "server name must not be empty"))
		return diagnostics
	}

	switch server.Transport {
	case MCPTransportStdio:
		if server.Stdio == nil {
			diagnostics = append(diagnostics, diag("invalid-field", prefix, "missing stdio transport data"))
			return diagnostics
		}
		diagnostics = append(diagnostics, validateStdioTransport(prefix, *server.Stdio)...)
	case MCPTransportStreamableHTTP, MCPTransportSSE:
		if server.Remote == nil {
			diagnostics = append(diagnostics, diag("invalid-field", prefix, "missing remote transport data"))
			return diagnostics
		}
		diagnostics = append(diagnostics, validateRemoteTransport(prefix, *server.Remote)...)
	default:
		diagnostics = append(diagnostics, diag("invalid-field", prefix+".type",
			fmt.Sprintf("transport %q is not recognized", server.Transport)))
	}

	return diagnostics
}

// validateStdioTransport validates stdio-specific fields.
func validateStdioTransport(prefix string, t StdioTransport) []Diagnostic {
	var diagnostics []Diagnostic

	// command: one bare token (no path separators, no leading ./),
	// or a plugin-relative path starting with "./"
	if t.Command == "" {
		diagnostics = append(diagnostics, diag("missing-required", prefix+".command", "command must not be empty"))
	} else if !isValidStdioCommand(t.Command) {
		diagnostics = append(diagnostics, diag("invalid-field", prefix+".command",
			"command must be a bare executable name or a plugin-relative path starting with ./"))
	}

	// args: check for NUL bytes
	for i, arg := range t.Args {
		if strings.ContainsRune(arg, '\x00') {
			diagnostics = append(diagnostics, diag("invalid-field",
				fmt.Sprintf("%s.args[%d]", prefix, i), "argument must not contain NUL"))
		}
	}

	// env: check for reserved keys and NUL bytes
	for key, value := range t.Env {
		if reservedEnvKeys[key] {
			diagnostics = append(diagnostics, diag("reserved-env-key", prefix+".env."+key,
				fmt.Sprintf("environment key %q is reserved and cannot be overridden", key)))
		}
		if strings.ContainsRune(key, '\x00') || strings.ContainsRune(value, '\x00') {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".env."+key,
				"environment key and value must not contain NUL"))
		}
	}

	// cwd: if set, must be a valid non-NUL string
	if t.Cwd != "" && strings.ContainsRune(t.Cwd, '\x00') {
		diagnostics = append(diagnostics, diag("invalid-field", prefix+".cwd", "cwd must not contain NUL"))
	}

	return diagnostics
}

// validateRemoteTransport validates remote (streamable-http/sse) transport fields.
func validateRemoteTransport(prefix string, t RemoteTransport) []Diagnostic {
	var diagnostics []Diagnostic

	if t.URL == "" {
		diagnostics = append(diagnostics, diag("missing-required", prefix+".url", "url must not be empty"))
	} else {
		diagnostics = append(diagnostics, validateURIField(prefix+".url", t.URL)...)
	}

	for key, value := range t.Headers {
		if strings.ContainsRune(key, '\x00') || strings.ContainsRune(value, '\x00') {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".headers."+key,
				"header name and value must not contain NUL"))
		}
	}

	return diagnostics
}

// validatePluginName checks the Agent Plugins 1.0.0 name rules:
//   - 1–64 characters
//   - only lowercase ASCII letters, digits, hyphens, periods
//   - begins and ends alphanumeric
//   - no consecutive hyphens (--), consecutive periods (..), or mixed separator sequences (-. or .-)
func validatePluginName(name string) []Diagnostic {
	if name == "" {
		return []Diagnostic{diag("missing-required", "name", "name is required")}
	}

	var diagnostics []Diagnostic
	runes := []rune(name)
	length := len(runes)

	if length > 64 {
		diagnostics = append(diagnostics, diag("invalid-name", "name",
			fmt.Sprintf("name must be at most 64 characters; got %d", length)))
	}

	for _, r := range runes {
		if !isPluginNameRune(r) {
			diagnostics = append(diagnostics, diag("invalid-name", "name",
				fmt.Sprintf("name %q contains invalid character %q; only lowercase letters, digits, hyphens, and periods are allowed", name, r)))
			break
		}
	}

	if length > 0 {
		first, _ := utf8.DecodeRuneInString(name)
		last, _ := utf8.DecodeLastRuneInString(name)
		if !isAlphanumeric(first) || !isAlphanumeric(last) {
			diagnostics = append(diagnostics, diag("invalid-name", "name",
				fmt.Sprintf("name %q must begin and end with a letter or digit", name)))
		}
	}

	// Reject consecutive separators and mixed separator sequences.
	if strings.Contains(name, "--") || strings.Contains(name, "..") ||
		strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		diagnostics = append(diagnostics, diag("invalid-name", "name",
			fmt.Sprintf("name %q must not contain consecutive hyphens, consecutive periods, or mixed separator sequences", name)))
	}

	return diagnostics
}

// validateURIField validates a URI string field.
func validateURIField(field, value string) []Diagnostic {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return []Diagnostic{diag("invalid-url", field,
			fmt.Sprintf("%s must be an absolute http or https URL", field))}
	}
	return nil
}

// validateReverseDomainNamespace checks that a namespace is a valid reverse-domain
// identifier (e.g. com.example.client). Each segment must start with a letter
// and contain only letters, digits, and hyphens.
func validateReverseDomainNamespace(ns string) error {
	if ns == "" {
		return fmt.Errorf("extension namespace must not be empty")
	}
	segments := strings.Split(ns, ".")
	if len(segments) < 2 {
		return fmt.Errorf("extension namespace %q must contain at least two domain segments", ns)
	}
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("extension namespace %q contains an empty segment", ns)
		}
		r, _ := utf8.DecodeRuneInString(seg)
		if !unicode.IsLetter(r) {
			return fmt.Errorf("extension namespace segment %q must start with a letter", seg)
		}
		for _, c := range seg {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' {
				return fmt.Errorf("extension namespace segment %q contains invalid character %q", seg, c)
			}
		}
	}
	return nil
}

// isPluginNameRune reports whether r is a character allowed in a plugin name.
func isPluginNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.'
}

// isAlphanumeric reports whether r is an ASCII letter or digit.
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// isValidStdioCommand reports whether command is a valid stdio MCP command value.
// It must be a bare executable name (no separators) or a plugin-relative path
// starting with "./".
func isValidStdioCommand(command string) bool {
	if command == "" {
		return false
	}
	if strings.HasPrefix(command, "./") {
		// plugin-relative path; must not escape via ..
		rest := command[2:]
		return rest != "" && !strings.Contains(rest, "..") && !strings.ContainsRune(command, '\x00')
	}
	// Bare executable: no path separators, no NUL
	return !strings.ContainsAny(command, "/\\\x00")
}
