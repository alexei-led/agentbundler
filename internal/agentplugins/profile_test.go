package agentplugins_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/alexei-led/agentbundler/internal/agentplugins"
)

func TestProfileID(t *testing.T) {
	t.Parallel()
	if agentplugins.ProfileID == "" {
		t.Fatal("ProfileID is empty")
	}
	if agentplugins.ProfileID != "agent-plugins/1.0.0-bd383552" {
		t.Fatalf("ProfileID = %q; want agent-plugins/1.0.0-bd383552", agentplugins.ProfileID)
	}
}

func TestUpstreamCommit(t *testing.T) {
	t.Parallel()
	if len(agentplugins.UpstreamCommit) != 40 {
		t.Fatalf("UpstreamCommit length = %d; want 40 (full SHA-1)", len(agentplugins.UpstreamCommit))
	}
	if agentplugins.UpstreamCommit != "bd383552095128f6effe895b9257cfd580a6d179" {
		t.Fatalf("UpstreamCommit = %q; want bd383552095128f6effe895b9257cfd580a6d179", agentplugins.UpstreamCommit)
	}
}

func TestSchemaURLs(t *testing.T) {
	t.Parallel()
	if agentplugins.PluginSchemaURL == "" {
		t.Fatal("PluginSchemaURL is empty")
	}
	if agentplugins.MCPSchemaURL == "" {
		t.Fatal("MCPSchemaURL is empty")
	}
	if agentplugins.PluginSchemaURL == agentplugins.MCPSchemaURL {
		t.Fatal("PluginSchemaURL and MCPSchemaURL must be distinct")
	}
}

// TestEmbeddedSchemaDigests verifies that the embedded schema bytes hash to
// the digest constants declared in profile.go. These digests reflect the
// vendored schema files; to advance the pinned profile, replace both the
// schema bytes and the digest constants together.
func TestEmbeddedSchemaDigests(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		schema   []byte
		declared string
		approved string
	}{
		{
			name:     "plugin-schema",
			schema:   agentplugins.PluginSchemaBytes(),
			declared: agentplugins.PluginSchemaSHA256,
			approved: "0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883",
		},
		{
			name:     "mcp-schema",
			schema:   agentplugins.MCPSchemaBytes(),
			declared: agentplugins.MCPSchemaSHA256,
			approved: "6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.declared != test.approved {
				t.Fatalf("declared schema SHA-256 = %s; want approved profile digest %s", test.declared, test.approved)
			}
			if len(test.schema) == 0 {
				t.Fatal("embedded schema is empty")
			}
			sum := sha256.Sum256(test.schema)
			if got := hex.EncodeToString(sum[:]); got != test.approved {
				t.Fatalf("embedded schema SHA-256 = %s; want approved profile digest %s", got, test.approved)
			}
		})
	}
}

// TestSchemaSelectorFunctions verifies the IsPluginSchemaURL and IsMCPSchemaURL helpers.
func TestSchemaSelectorFunctions(t *testing.T) {
	t.Parallel()

	if !agentplugins.IsPluginSchemaURL(agentplugins.PluginSchemaURL) {
		t.Errorf("IsPluginSchemaURL(PluginSchemaURL) = false")
	}
	if agentplugins.IsPluginSchemaURL(agentplugins.MCPSchemaURL) {
		t.Errorf("IsPluginSchemaURL(MCPSchemaURL) = true, want false")
	}
	if agentplugins.IsPluginSchemaURL("https://example.com/schema") {
		t.Errorf("IsPluginSchemaURL(unknown) = true, want false")
	}

	if !agentplugins.IsMCPSchemaURL(agentplugins.MCPSchemaURL) {
		t.Errorf("IsMCPSchemaURL(MCPSchemaURL) = false")
	}
	if agentplugins.IsMCPSchemaURL(agentplugins.PluginSchemaURL) {
		t.Errorf("IsMCPSchemaURL(PluginSchemaURL) = true, want false")
	}
}

// TestOfflineSchemaAccess verifies that the embedded schema bytes are accessible
// without any network I/O and are valid JSON.
func TestOfflineSchemaAccess(t *testing.T) {
	t.Parallel()

	t.Run("plugin-schema-is-json", func(t *testing.T) {
		t.Parallel()
		data := agentplugins.PluginSchemaBytes()
		if len(data) == 0 {
			t.Fatal("plugin schema bytes are empty")
		}
		// Verify it starts and ends with JSON object markers.
		trimmed := trimJSON(data)
		if len(trimmed) == 0 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
			t.Fatalf("plugin schema does not appear to be a JSON object")
		}
	})

	t.Run("mcp-schema-is-json", func(t *testing.T) {
		t.Parallel()
		data := agentplugins.MCPSchemaBytes()
		if len(data) == 0 {
			t.Fatal("MCP schema bytes are empty")
		}
		trimmed := trimJSON(data)
		if len(trimmed) == 0 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
			t.Fatalf("MCP schema does not appear to be a JSON object")
		}
	})
}

// trimJSON trims whitespace from both ends of a byte slice.
func trimJSON(data []byte) []byte {
	start, end := 0, len(data)-1
	for start <= end && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n' || data[start] == '\r') {
		start++
	}
	for end >= start && (data[end] == ' ' || data[end] == '\t' || data[end] == '\n' || data[end] == '\r') {
		end--
	}
	if start > end {
		return nil
	}
	return data[start : end+1]
}
