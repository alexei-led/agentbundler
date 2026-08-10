package agentplugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// Diagnostic is a schema validation finding from the Agent Plugins wire contract.
type Diagnostic struct {
	// Code is a machine-readable error identifier.
	Code string
	// Path is the JSON path to the invalid field (empty for top-level errors).
	Path string
	// Message is the human-readable description.
	Message string
}

// DecodePluginManifest decodes and validates the JSON bytes of a plugin.json file.
//
// It rejects duplicate object keys, stores permitted unknown top-level members
// as raw values, and checks the schema selector before returning a decoded manifest.
// Returns the decoded manifest and any diagnostics; a non-empty diagnostic slice
// indicates the manifest is invalid or incomplete.
func DecodePluginManifest(data []byte) (PluginManifest, []Diagnostic) {
	raw, err := decodeLenientJSONObject(data)
	if err != nil {
		return PluginManifest{}, []Diagnostic{diag("invalid-json", "", err.Error())}
	}
	var diagnostics []Diagnostic
	var manifest PluginManifest

	// $schema — required, must be pinned URL
	schemaVal, hasSchemField := raw["$schema"]
	if !hasSchemField {
		diagnostics = append(diagnostics, diag("missing-required", "$schema", "$schema is required"))
	} else {
		if err := json.Unmarshal(schemaVal, &manifest.Schema); err != nil || manifest.Schema == "" {
			diagnostics = append(diagnostics, diag("invalid-field", "$schema", "$schema must be a non-empty string"))
		} else if manifest.Schema != PluginSchemaURL {
			diagnostics = append(diagnostics, diag("invalid-schema", "$schema",
				fmt.Sprintf("$schema %q is not the recognized selector for profile %s", manifest.Schema, ProfileID)))
		}
		delete(raw, "$schema")
	}

	// name — required
	if nameVal, ok := raw["name"]; ok {
		if err := json.Unmarshal(nameVal, &manifest.Name); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "name", "name must be a string"))
		}
		delete(raw, "name")
	} else {
		diagnostics = append(diagnostics, diag("missing-required", "name", "name is required"))
	}

	// version — optional string
	if v, ok := raw["version"]; ok {
		if err := json.Unmarshal(v, &manifest.Version); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "version", "version must be a string"))
		}
		delete(raw, "version")
	}

	// description — optional string
	if v, ok := raw["description"]; ok {
		if err := json.Unmarshal(v, &manifest.Description); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "description", "description must be a string"))
		}
		delete(raw, "description")
	}

	// author — optional closed object with string fields
	if v, ok := raw["author"]; ok {
		author, authorDiagnostics := decodePluginAuthor(v)
		manifest.Author = author
		diagnostics = append(diagnostics, authorDiagnostics...)
		delete(raw, "author")
	}

	// homepage — optional string (URL validated by ValidatePluginManifest)
	if v, ok := raw["homepage"]; ok {
		if err := json.Unmarshal(v, &manifest.Homepage); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "homepage", "homepage must be a string"))
		}
		delete(raw, "homepage")
	}

	// repository — optional string (URL validated by ValidatePluginManifest)
	if v, ok := raw["repository"]; ok {
		if err := json.Unmarshal(v, &manifest.Repository); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "repository", "repository must be a string"))
		}
		delete(raw, "repository")
	}

	// license — optional string
	if v, ok := raw["license"]; ok {
		if err := json.Unmarshal(v, &manifest.License); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "license", "license must be a string"))
		}
		delete(raw, "license")
	}

	// keywords — optional array of strings
	if v, ok := raw["keywords"]; ok {
		if err := json.Unmarshal(v, &manifest.Keywords); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "keywords", "keywords must be an array of strings"))
		}
		delete(raw, "keywords")
	}

	// extensions — optional object with opaque values
	if v, ok := raw["extensions"]; ok {
		rawExts, extErr := decodeLenientJSONObject(v)
		if extErr != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "extensions", "extensions must be a JSON object"))
		} else {
			manifest.Extensions = make(map[string]any, len(rawExts))
			for ns, extVal := range rawExts {
				decoded, err := decodeOpaqueJSON(extVal)
				if err != nil {
					diagnostics = append(diagnostics, diag("invalid-field", "extensions."+ns, "extension value must be valid JSON"))
					continue
				}
				manifest.Extensions[ns] = decoded
			}
		}
		delete(raw, "extensions")
	}

	// remaining keys are unknown (permitted for round-trip)
	if len(raw) > 0 {
		manifest.Unknown = make(map[string]any, len(raw))
		for key, val := range raw {
			decoded, err := decodeOpaqueJSON(val)
			if err != nil {
				diagnostics = append(diagnostics, diag("invalid-field", key, "field value must be valid JSON"))
				continue
			}
			manifest.Unknown[key] = decoded
		}
	}

	if len(diagnostics) > 0 {
		return manifest, diagnostics
	}
	return manifest, ValidatePluginManifest(manifest)
}

func decodePluginAuthor(data json.RawMessage) (*PluginAuthor, []Diagnostic) {
	raw, err := decodeLenientJSONObject(data)
	if err != nil {
		return nil, []Diagnostic{diag("invalid-field", "author", "author must be a JSON object")}
	}

	author := &PluginAuthor{}
	fields := map[string]*string{
		"name":  &author.Name,
		"email": &author.Email,
		"url":   &author.URL,
	}
	var diagnostics []Diagnostic
	for _, key := range sortedKeys(raw) {
		destination, ok := fields[key]
		if !ok {
			diagnostics = append(diagnostics, diag("invalid-field", "author."+key,
				"author may contain only name, email, and url"))
			continue
		}
		value := bytes.TrimSpace(raw[key])
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, destination) != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "author."+key,
				key+" must be a string"))
		}
	}
	return author, diagnostics
}

// DecodeMCPConfig decodes and validates the JSON bytes of a mcp.json file.
//
// It rejects duplicate object keys, stores permitted unknown members,
// and checks the schema selector. Each server entry is decoded into a typed
// MCPServer. Returns the decoded config and any diagnostics.
func DecodeMCPConfig(data []byte) (MCPConfig, []Diagnostic) {
	raw, err := decodeLenientJSONObject(data)
	if err != nil {
		return MCPConfig{}, []Diagnostic{diag("invalid-json", "", err.Error())}
	}
	var diagnostics []Diagnostic
	var config MCPConfig

	// $schema — required
	if schemaVal, ok := raw["$schema"]; ok {
		if err := json.Unmarshal(schemaVal, &config.Schema); err != nil || config.Schema == "" {
			diagnostics = append(diagnostics, diag("invalid-field", "$schema", "$schema must be a non-empty string"))
		} else if config.Schema != MCPSchemaURL {
			diagnostics = append(diagnostics, diag("invalid-schema", "$schema",
				fmt.Sprintf("$schema %q is not the recognized selector for profile %s", config.Schema, ProfileID)))
		}
		delete(raw, "$schema")
	} else {
		diagnostics = append(diagnostics, diag("missing-required", "$schema", "$schema is required"))
	}

	// mcpServers — required object
	if serversVal, ok := raw["mcpServers"]; ok {
		rawServers, serversErr := decodeLenientJSONObject(serversVal)
		if serversErr != nil {
			diagnostics = append(diagnostics, diag("invalid-field", "mcpServers", "mcpServers must be a JSON object"))
		} else {
			names := sortedKeys(rawServers)
			for _, name := range names {
				server, serverDiags := decodeServerEntry(name, rawServers[name])
				diagnostics = append(diagnostics, serverDiags...)
				config.Servers = append(config.Servers, server)
			}
		}
		delete(raw, "mcpServers")
	} else {
		diagnostics = append(diagnostics, diag("missing-required", "mcpServers", "mcpServers is required"))
	}

	// remaining keys are unknown top-level fields
	if len(raw) > 0 {
		config.Unknown = make(map[string]any, len(raw))
		for key, val := range raw {
			decoded, err := decodeOpaqueJSON(val)
			if err != nil {
				diagnostics = append(diagnostics, diag("invalid-field", key, "field value must be valid JSON"))
				continue
			}
			config.Unknown[key] = decoded
		}
	}

	if len(diagnostics) > 0 {
		return config, diagnostics
	}
	return config, ValidateMCPConfig(config)
}

// decodeServerEntry decodes one named MCP server entry.
func decodeServerEntry(name string, data json.RawMessage) (MCPServer, []Diagnostic) {
	prefix := "mcpServers." + name
	raw, err := decodeLenientJSONObject(data)
	if err != nil {
		return MCPServer{Name: name}, []Diagnostic{diag("invalid-json", prefix, err.Error())}
	}
	var diagnostics []Diagnostic
	server := MCPServer{Name: name}

	// type — required, determines transport
	typeVal, hasType := raw["type"]
	if !hasType {
		return server, []Diagnostic{diag("missing-required", prefix+".type", "type is required")}
	}
	var transportType string
	if err := json.Unmarshal(typeVal, &transportType); err != nil {
		return server, []Diagnostic{diag("invalid-field", prefix+".type", "type must be a string")}
	}
	delete(raw, "type")

	switch MCPTransportType(transportType) {
	case MCPTransportStdio:
		server.Transport = MCPTransportStdio
		stdio, diags := decodeStdioTransport(prefix, raw)
		server.Stdio = &stdio
		diagnostics = append(diagnostics, diags...)
	case MCPTransportStreamableHTTP, MCPTransportSSE:
		server.Transport = MCPTransportType(transportType)
		remote, diags := decodeRemoteTransport(prefix, raw)
		server.Remote = &remote
		diagnostics = append(diagnostics, diags...)
	default:
		diagnostics = append(diagnostics, diag("invalid-field", prefix+".type",
			fmt.Sprintf("type %q is not a recognized MCP transport (stdio, streamable-http, sse)", transportType)))
	}

	// remaining keys are unknown server-level fields
	if len(raw) > 0 {
		server.Unknown = make(map[string]any, len(raw))
		for key, val := range raw {
			decoded, err := decodeOpaqueJSON(val)
			if err != nil {
				diagnostics = append(diagnostics, diag("invalid-field", prefix+"."+key, "field value must be valid JSON"))
				continue
			}
			server.Unknown[key] = decoded
		}
	}
	return server, diagnostics
}

// decodeStdioTransport decodes the stdio-specific fields from the server entry map.
// The type key has already been removed from raw.
func decodeStdioTransport(prefix string, raw map[string]json.RawMessage) (StdioTransport, []Diagnostic) {
	var diagnostics []Diagnostic
	var t StdioTransport

	// command — required
	if cmdVal, ok := raw["command"]; ok {
		if err := json.Unmarshal(cmdVal, &t.Command); err != nil || t.Command == "" {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".command", "command must be a non-empty string"))
		}
		delete(raw, "command")
	} else {
		diagnostics = append(diagnostics, diag("missing-required", prefix+".command", "command is required for stdio transport"))
	}

	// args — optional array of strings
	if v, ok := raw["args"]; ok {
		if err := json.Unmarshal(v, &t.Args); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".args", "args must be an array of strings"))
		}
		delete(raw, "args")
	}

	// env — optional string-to-string object
	if v, ok := raw["env"]; ok {
		if err := json.Unmarshal(v, &t.Env); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".env", "env must be an object with string values"))
		}
		delete(raw, "env")
	}

	// cwd — optional string
	if v, ok := raw["cwd"]; ok {
		if err := json.Unmarshal(v, &t.Cwd); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".cwd", "cwd must be a string"))
		}
		delete(raw, "cwd")
	}

	return t, diagnostics
}

// decodeRemoteTransport decodes the streamable-http/sse fields from the server entry map.
// The type key has already been removed from raw.
func decodeRemoteTransport(prefix string, raw map[string]json.RawMessage) (RemoteTransport, []Diagnostic) {
	var diagnostics []Diagnostic
	var t RemoteTransport

	// url — required
	if urlVal, ok := raw["url"]; ok {
		if err := json.Unmarshal(urlVal, &t.URL); err != nil || t.URL == "" {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".url", "url must be a non-empty string"))
		}
		delete(raw, "url")
	} else {
		diagnostics = append(diagnostics, diag("missing-required", prefix+".url", "url is required for remote transport"))
	}

	// headers — optional string-to-string object
	if v, ok := raw["headers"]; ok {
		if err := json.Unmarshal(v, &t.Headers); err != nil {
			diagnostics = append(diagnostics, diag("invalid-field", prefix+".headers", "headers must be an object with string values"))
		}
		delete(raw, "headers")
	}

	return t, diagnostics
}

// decodeOpaqueJSON decodes a permitted unknown value without converting JSON
// numbers through float64. json.Number is preserved by json.Marshal, including
// integers outside float64's exact range.
func decodeOpaqueJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid JSON: multiple top-level values")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// decodeLenientJSONObject decodes a JSON object into a map of raw values.
// It rejects duplicate keys and non-object inputs, but accepts unknown keys.
func decodeLenientJSONObject(data []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || !utf8.Valid(trimmed) {
		return nil, fmt.Errorf("must be UTF-8 JSON")
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("must be a JSON object")
	}
	if err := rejectDuplicateJSONKeys(trimmed); err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// rejectDuplicateJSONKeys scans a JSON value and returns an error if any object
// has duplicate keys at any depth.
func rejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(dec); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid JSON: multiple top-level values")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func scanJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("invalid JSON object key")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	default:
		return fmt.Errorf("invalid JSON delimiter %q", delim)
	}
	return nil
}

// sortedKeys returns the keys of m in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func diag(code, path, message string) Diagnostic {
	return Diagnostic{Code: code, Path: path, Message: message}
}

// HasErrors reports whether any diagnostic has an error-level code.
func HasErrors(diagnostics []Diagnostic) bool {
	return len(diagnostics) > 0
}

// DiagnosticMessages returns all diagnostic messages joined by newline.
func DiagnosticMessages(diagnostics []Diagnostic) string {
	msgs := make([]string, len(diagnostics))
	for i, d := range diagnostics {
		if d.Path != "" {
			msgs[i] = d.Path + ": " + d.Message
		} else {
			msgs[i] = d.Message
		}
	}
	return strings.Join(msgs, "\n")
}
