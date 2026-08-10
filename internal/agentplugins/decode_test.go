package agentplugins_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/agentbundler/internal/agentplugins"
)

// --- Plugin manifest decode tests ---

func TestDecodePluginManifestMinimal(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/minimal.json")
	manifest, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) != 0 {
		t.Fatalf("minimal plugin diagnostics = %v", diags)
	}
	if manifest.Name != "my-plugin" {
		t.Errorf("Name = %q; want my-plugin", manifest.Name)
	}
	if manifest.Schema != agentplugins.PluginSchemaURL {
		t.Errorf("Schema = %q; want %q", manifest.Schema, agentplugins.PluginSchemaURL)
	}
	if manifest.Version != "" || manifest.Description != "" || manifest.Author != "" {
		t.Errorf("optional fields non-empty in minimal manifest: %+v", manifest)
	}
}

func TestDecodePluginManifestFull(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/full.json")
	manifest, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) != 0 {
		t.Fatalf("full plugin diagnostics = %v", diags)
	}
	if manifest.Name != "full-plugin" {
		t.Errorf("Name = %q", manifest.Name)
	}
	if manifest.Version != "1.2.3" {
		t.Errorf("Version = %q", manifest.Version)
	}
	if manifest.Description != "A complete plugin for testing." {
		t.Errorf("Description = %q", manifest.Description)
	}
	if manifest.License != "MIT" {
		t.Errorf("License = %q", manifest.License)
	}
	if len(manifest.Keywords) != 3 {
		t.Errorf("Keywords len = %d; want 3", len(manifest.Keywords))
	}
	if len(manifest.Extensions) != 2 {
		t.Errorf("Extensions len = %d; want 2", len(manifest.Extensions))
	}
	if _, ok := manifest.Extensions["com.example.client"]; !ok {
		t.Error("missing extension com.example.client")
	}
}

func TestDecodePluginManifestUnknownFields(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/unknown-fields.json")
	manifest, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) != 0 {
		t.Fatalf("unknown-fields plugin diagnostics = %v", diags)
	}
	if manifest.Name != "unknown-plugin" {
		t.Errorf("Name = %q", manifest.Name)
	}
	// Unknown fields must be preserved with their values.
	if len(manifest.Unknown) != 2 {
		t.Fatalf("Unknown len = %d; want 2; Unknown = %v", len(manifest.Unknown), manifest.Unknown)
	}
	if _, ok := manifest.Unknown["futureFeature"]; !ok {
		t.Error("missing unknown field futureFeature")
	}
	if _, ok := manifest.Unknown["anotherUnknown"]; !ok {
		t.Error("missing unknown field anotherUnknown")
	}
	// The unknown value must not be assigned compiler semantics.
	if manifest.Unknown["futureFeature"] != "some-value" {
		t.Errorf("futureFeature = %v; want some-value", manifest.Unknown["futureFeature"])
	}
}

func TestDecodeOpaqueNumbersPreservePrecision(t *testing.T) {
	t.Parallel()
	manifestJSON := []byte(`{"$schema":"` + agentplugins.PluginSchemaURL + `","name":"number-plugin","extensions":{"com.example.ext":{"value":9007199254740993}},"future":{"value":9007199254740993}}`)
	manifest, diags := agentplugins.DecodePluginManifest(manifestJSON)
	if len(diags) != 0 {
		t.Fatalf("manifest diagnostics = %v", diags)
	}
	encoded, err := agentplugins.EncodePluginManifest(manifest)
	if err != nil {
		t.Fatalf("EncodePluginManifest() error = %v", err)
	}
	if got, want := string(encoded), `{"$schema":"`+agentplugins.PluginSchemaURL+`","name":"number-plugin","extensions":{"com.example.ext":{"value":9007199254740993}},"future":{"value":9007199254740993}}`; got != want {
		t.Fatalf("encoded manifest = %s; want %s", got, want)
	}

	mcpJSON := []byte(`{"$schema":"` + agentplugins.MCPSchemaURL + `","mcpServers":{"local":{"type":"stdio","command":"server","future":9007199254740993}},"future":9007199254740993}`)
	config, diags := agentplugins.DecodeMCPConfig(mcpJSON)
	if len(diags) != 0 {
		t.Fatalf("MCP diagnostics = %v", diags)
	}
	encoded, err = agentplugins.EncodeMCPConfig(config)
	if err != nil {
		t.Fatalf("EncodeMCPConfig() error = %v", err)
	}
	if !bytes.Contains(encoded, []byte("9007199254740993")) {
		t.Fatalf("encoded MCP config lost opaque integer: %s", encoded)
	}
}

func TestDecodePluginManifestDuplicateKeyFails(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/duplicate-key.json")
	_, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) == 0 {
		t.Fatal("duplicate-key: expected diagnostics, got none")
	}
	for _, d := range diags {
		if d.Code == "invalid-json" {
			return
		}
	}
	t.Fatalf("expected invalid-json diagnostic; got %v", diags)
}

func TestDecodePluginManifestWrongSchemaFails(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/wrong-schema.json")
	_, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) == 0 {
		t.Fatal("wrong-schema: expected diagnostics, got none")
	}
	for _, d := range diags {
		if d.Code == "invalid-schema" {
			return
		}
	}
	t.Fatalf("expected invalid-schema diagnostic; got %v", diags)
}

func TestDecodePluginManifestMissingNameFails(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/missing-name.json")
	_, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) == 0 {
		t.Fatal("missing-name: expected diagnostics, got none")
	}
}

// --- Plugin name validation tests ---

func TestPluginNameValidation(t *testing.T) {
	t.Parallel()

	valid := []string{
		"my-plugin", "plugin1", "a", "ab", "my.plugin", "a0", "0a",
		"hello-world", "a-b-c", "a.b.c",
	}
	for _, name := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			t.Parallel()
			data := pluginJSON(name)
			_, diags := agentplugins.DecodePluginManifest(data)
			if len(diags) != 0 {
				t.Fatalf("valid name %q got diagnostics: %v", name, diags)
			}
		})
	}

	invalid := []string{
		"",
		"UPPER",
		"-starts-with-hyphen",
		"ends-with-hyphen-",
		".starts-with-period",
		"ends-with-period.",
		"has space",
		"has/slash",
		"has--double-hyphen",
		"has..double.period",
		"has-.mixed",
		"has.-mixed",
		"a_underscore",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong", // 65 chars
	}
	for _, name := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			t.Parallel()
			data := pluginJSON(name)
			_, diags := agentplugins.DecodePluginManifest(data)
			if len(diags) == 0 {
				t.Fatalf("invalid name %q: expected diagnostics, got none", name)
			}
		})
	}
}

// --- MCP config decode tests ---

func TestDecodeMCPConfigMinimal(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "mcp/minimal.json")
	config, diags := agentplugins.DecodeMCPConfig(data)
	if len(diags) != 0 {
		t.Fatalf("minimal mcp diagnostics = %v", diags)
	}
	if len(config.Servers) != 1 {
		t.Fatalf("Servers len = %d; want 1", len(config.Servers))
	}
	server := config.Servers[0]
	if server.Name != "my-server" {
		t.Errorf("server Name = %q; want my-server", server.Name)
	}
	if server.Transport != agentplugins.MCPTransportStdio {
		t.Errorf("transport = %q; want stdio", server.Transport)
	}
	if server.Stdio == nil {
		t.Fatal("Stdio is nil")
	}
	if server.Stdio.Command != "my-mcp-server" {
		t.Errorf("command = %q; want my-mcp-server", server.Stdio.Command)
	}
}

func TestDecodeMCPConfigFull(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "mcp/full.json")
	config, diags := agentplugins.DecodeMCPConfig(data)
	if len(diags) != 0 {
		t.Fatalf("full mcp diagnostics = %v", diags)
	}
	if len(config.Servers) != 3 {
		t.Fatalf("Servers len = %d; want 3", len(config.Servers))
	}

	// Servers are decoded in lexical order.
	byName := make(map[string]agentplugins.MCPServer, len(config.Servers))
	for _, s := range config.Servers {
		byName[s.Name] = s
	}

	local := byName["local-server"]
	if local.Transport != agentplugins.MCPTransportStdio {
		t.Errorf("local-server transport = %q; want stdio", local.Transport)
	}
	if local.Stdio.Command != "./bin/server" {
		t.Errorf("local-server command = %q", local.Stdio.Command)
	}
	if len(local.Stdio.Args) != 4 {
		t.Errorf("local-server args len = %d; want 4", len(local.Stdio.Args))
	}
	if local.Stdio.Env["LOG_LEVEL"] != "info" {
		t.Errorf("local-server env LOG_LEVEL = %q", local.Stdio.Env["LOG_LEVEL"])
	}
	if local.Stdio.Cwd != "${PLUGIN_ROOT}" {
		t.Errorf("local-server cwd = %q", local.Stdio.Cwd)
	}

	remote := byName["remote-server"]
	if remote.Transport != agentplugins.MCPTransportStreamableHTTP {
		t.Errorf("remote-server transport = %q; want streamable-http", remote.Transport)
	}
	if remote.Remote == nil {
		t.Fatal("remote-server Remote is nil")
	}
	if remote.Remote.URL != "https://api.example.com/mcp" {
		t.Errorf("remote-server URL = %q", remote.Remote.URL)
	}

	legacy := byName["legacy-server"]
	if legacy.Transport != agentplugins.MCPTransportSSE {
		t.Errorf("legacy-server transport = %q; want sse", legacy.Transport)
	}
}

func TestDecodeMCPConfigUnknownFields(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "mcp/unknown-fields.json")
	config, diags := agentplugins.DecodeMCPConfig(data)
	if len(diags) != 0 {
		t.Fatalf("unknown-fields mcp diagnostics = %v", diags)
	}
	// Top-level unknown field must be preserved.
	if _, ok := config.Unknown["topLevelUnknown"]; !ok {
		t.Errorf("missing unknown top-level field topLevelUnknown; Unknown = %v", config.Unknown)
	}
	// Server-level unknown field must be preserved.
	if len(config.Servers) != 1 {
		t.Fatalf("Servers len = %d; want 1", len(config.Servers))
	}
	if _, ok := config.Servers[0].Unknown["futureTransportField"]; !ok {
		t.Errorf("missing server unknown field futureTransportField; Unknown = %v", config.Servers[0].Unknown)
	}
}

func TestDecodeMCPConfigDuplicateKeyFails(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "mcp/duplicate-key.json")
	_, diags := agentplugins.DecodeMCPConfig(data)
	if len(diags) == 0 {
		t.Fatal("duplicate-key mcp: expected diagnostics, got none")
	}
}

// --- Encode tests ---

func TestEncodePluginManifestDeterministic(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/full.json")
	manifest, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) != 0 {
		t.Fatalf("decode diagnostics = %v", diags)
	}

	encoded1, err := agentplugins.EncodePluginManifest(manifest)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	encoded2, err := agentplugins.EncodePluginManifest(manifest)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if string(encoded1) != string(encoded2) {
		t.Fatal("encode is not deterministic")
	}

	// Verify the result is valid JSON.
	var decoded any
	if err := json.Unmarshal(encoded1, &decoded); err != nil {
		t.Fatalf("encoded result is not valid JSON: %v", err)
	}
}

func TestEncodePluginManifestSchemaFirst(t *testing.T) {
	t.Parallel()
	manifest := agentplugins.PluginManifest{
		Schema:  agentplugins.PluginSchemaURL,
		Name:    "test-plugin",
		Version: "1.0.0",
		Unknown: map[string]any{"z-unknown": 1, "a-unknown": 2},
	}
	encoded, err := agentplugins.EncodePluginManifest(manifest)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	// $schema must appear before name, unknown fields after known fields.
	schemaPos := findJSON(encoded, `"$schema"`)
	namePos := findJSON(encoded, `"name"`)
	aUnknownPos := findJSON(encoded, `"a-unknown"`)
	zUnknownPos := findJSON(encoded, `"z-unknown"`)

	if schemaPos >= namePos {
		t.Error("$schema must appear before name")
	}
	if aUnknownPos >= zUnknownPos {
		t.Error("a-unknown must appear before z-unknown (alphabetical order)")
	}
	if namePos >= aUnknownPos {
		t.Error("known field name must appear before unknown fields")
	}
}

func TestEncodeMCPConfigDeterministic(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "mcp/full.json")
	config, diags := agentplugins.DecodeMCPConfig(data)
	if len(diags) != 0 {
		t.Fatalf("decode diagnostics = %v", diags)
	}

	encoded1, err := agentplugins.EncodeMCPConfig(config)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	encoded2, err := agentplugins.EncodeMCPConfig(config)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if string(encoded1) != string(encoded2) {
		t.Fatal("MCP encode is not deterministic")
	}

	var decoded any
	if err := json.Unmarshal(encoded1, &decoded); err != nil {
		t.Fatalf("encoded MCP result is not valid JSON: %v", err)
	}
}

func TestEncodePluginManifestUnknownPreserved(t *testing.T) {
	t.Parallel()
	data := readTestData(t, "plugin/unknown-fields.json")
	manifest, diags := agentplugins.DecodePluginManifest(data)
	if len(diags) != 0 {
		t.Fatalf("decode diagnostics = %v", diags)
	}
	encoded, err := agentplugins.EncodePluginManifest(manifest)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// Both unknown fields must appear in the encoded output.
	if findJSON(encoded, `"futureFeature"`) < 0 {
		t.Error("encoded output missing futureFeature unknown field")
	}
	if findJSON(encoded, `"anotherUnknown"`) < 0 {
		t.Error("encoded output missing anotherUnknown unknown field")
	}
}

// --- Reserved env key test ---

func TestReservedEnvKeyFails(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"PLUGIN_ROOT", "PLUGIN_DATA"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			data := []byte(`{
				"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
				"mcpServers": {
					"srv": {
						"type": "stdio",
						"command": "server",
						"env": {"` + key + `": "override-attempt"}
					}
				}
			}`)
			_, diags := agentplugins.DecodeMCPConfig(data)
			if len(diags) == 0 {
				t.Fatalf("reserved env key %q: expected diagnostics, got none", key)
			}
			for _, d := range diags {
				if d.Code == "reserved-env-key" {
					return
				}
			}
			t.Fatalf("expected reserved-env-key diagnostic; got %v", diags)
		})
	}
}

// --- Helpers ---

func readTestData(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}

// pluginJSON constructs minimal plugin.json bytes with the given name.
func pluginJSON(name string) []byte {
	encoded, _ := json.Marshal(name)
	return []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":` + string(encoded) + `}`)
}

// findJSON finds the byte offset of needle in data; returns -1 if not found.
func findJSON(data []byte, needle string) int {
	nb := []byte(needle)
	for i := 0; i <= len(data)-len(nb); i++ {
		if string(data[i:i+len(nb)]) == needle {
			return i
		}
	}
	return -1
}
