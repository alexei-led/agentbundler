package agentplugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// EncodePluginManifest encodes a PluginManifest to deterministic JSON.
//
// Keys are emitted in a fixed order: $schema, name, then optional fields in
// spec definition order, then unknown fields sorted alphabetically. Extensions
// keys are sorted alphabetically within the extensions object. No indentation
// is added.
func EncodePluginManifest(manifest PluginManifest) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	comma := false

	write := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encoding field %q: %w", key, err)
		}
		if comma {
			buf.WriteByte(',')
		}
		comma = true
		keyBytes, _ := json.Marshal(key)
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.Write(encoded)
		return nil
	}

	// Required fields always present.
	if err := write("$schema", PluginSchemaURL); err != nil {
		return nil, err
	}
	if err := write("name", manifest.Name); err != nil {
		return nil, err
	}

	// Optional fields in spec definition order; omit empty values.
	if manifest.Version != "" {
		if err := write("version", manifest.Version); err != nil {
			return nil, err
		}
	}
	if manifest.Description != "" {
		if err := write("description", manifest.Description); err != nil {
			return nil, err
		}
	}
	if manifest.Author != "" {
		if err := write("author", manifest.Author); err != nil {
			return nil, err
		}
	}
	if manifest.Homepage != "" {
		if err := write("homepage", manifest.Homepage); err != nil {
			return nil, err
		}
	}
	if manifest.Repository != "" {
		if err := write("repository", manifest.Repository); err != nil {
			return nil, err
		}
	}
	if manifest.License != "" {
		if err := write("license", manifest.License); err != nil {
			return nil, err
		}
	}
	if len(manifest.Keywords) > 0 {
		if err := write("keywords", manifest.Keywords); err != nil {
			return nil, err
		}
	}

	// Extensions: encode as object with sorted namespace keys.
	if len(manifest.Extensions) > 0 {
		nsKeys := sortedStringSlice(manifest.Extensions)
		extBytes, err := encodeStringKeyedMap(nsKeys, manifest.Extensions)
		if err != nil {
			return nil, fmt.Errorf("encoding extensions: %w", err)
		}
		if comma {
			buf.WriteByte(',')
		}
		comma = true
		extKeyBytes, _ := json.Marshal("extensions")
		buf.Write(extKeyBytes)
		buf.WriteByte(':')
		buf.Write(extBytes)
	}

	// Unknown fields sorted alphabetically after all known fields.
	if len(manifest.Unknown) > 0 {
		unknownKeys := sortedStringSlice(manifest.Unknown)
		for _, key := range unknownKeys {
			if err := write(key, manifest.Unknown[key]); err != nil {
				return nil, err
			}
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// EncodeMCPConfig encodes a MCPConfig to deterministic JSON.
//
// Keys are emitted as: $schema, mcpServers (servers sorted by name), then
// unknown fields sorted alphabetically. Server entries emit type first, then
// transport-specific fields, then unknown server fields alphabetically.
func EncodeMCPConfig(config MCPConfig) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	comma := false

	write := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encoding field %q: %w", key, err)
		}
		if comma {
			buf.WriteByte(',')
		}
		comma = true
		keyBytes, _ := json.Marshal(key)
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.Write(encoded)
		return nil
	}

	if err := write("$schema", MCPSchemaURL); err != nil {
		return nil, err
	}

	// mcpServers — servers are already in lexical order from decoding.
	servers := make([]MCPServer, len(config.Servers))
	copy(servers, config.Servers)
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })

	serversBytes, err := encodeMCPServers(servers)
	if err != nil {
		return nil, fmt.Errorf("encoding mcpServers: %w", err)
	}
	if comma {
		buf.WriteByte(',')
	}
	comma = true
	keyBytes, _ := json.Marshal("mcpServers")
	buf.Write(keyBytes)
	buf.WriteByte(':')
	buf.Write(serversBytes)

	// Unknown top-level fields sorted.
	if len(config.Unknown) > 0 {
		unknownKeys := sortedStringSlice(config.Unknown)
		for _, key := range unknownKeys {
			if err := write(key, config.Unknown[key]); err != nil {
				return nil, err
			}
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encodeMCPServers encodes the mcpServers map as a JSON object.
func encodeMCPServers(servers []MCPServer) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, server := range servers {
		if i > 0 {
			buf.WriteByte(',')
		}
		nameBytes, _ := json.Marshal(server.Name)
		buf.Write(nameBytes)
		buf.WriteByte(':')
		entryBytes, err := encodeMCPServer(server)
		if err != nil {
			return nil, fmt.Errorf("encoding server %q: %w", server.Name, err)
		}
		buf.Write(entryBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encodeMCPServer encodes one MCPServer entry as a JSON object.
func encodeMCPServer(server MCPServer) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	comma := false

	writeField := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
		if comma {
			buf.WriteByte(',')
		}
		comma = true
		kBytes, _ := json.Marshal(key)
		buf.Write(kBytes)
		buf.WriteByte(':')
		buf.Write(encoded)
		return nil
	}

	// type first
	if err := writeField("type", string(server.Transport)); err != nil {
		return nil, err
	}

	switch server.Transport {
	case MCPTransportStdio:
		if server.Stdio != nil {
			if err := writeField("command", server.Stdio.Command); err != nil {
				return nil, err
			}
			if len(server.Stdio.Args) > 0 {
				if err := writeField("args", server.Stdio.Args); err != nil {
					return nil, err
				}
			}
			if len(server.Stdio.Env) > 0 {
				envKeys := make([]string, 0, len(server.Stdio.Env))
				for k := range server.Stdio.Env {
					envKeys = append(envKeys, k)
				}
				sort.Strings(envKeys)
				envBytes, err := encodeStringKeyedMapFromKeys(envKeys, server.Stdio.Env)
				if err != nil {
					return nil, fmt.Errorf("encoding env: %w", err)
				}
				if comma {
					buf.WriteByte(',')
				}
				comma = true
				keyB, _ := json.Marshal("env")
				buf.Write(keyB)
				buf.WriteByte(':')
				buf.Write(envBytes)
			}
			if server.Stdio.Cwd != "" {
				if err := writeField("cwd", server.Stdio.Cwd); err != nil {
					return nil, err
				}
			}
		}
	case MCPTransportStreamableHTTP, MCPTransportSSE:
		if server.Remote != nil {
			if err := writeField("url", server.Remote.URL); err != nil {
				return nil, err
			}
			if len(server.Remote.Headers) > 0 {
				hKeys := make([]string, 0, len(server.Remote.Headers))
				for k := range server.Remote.Headers {
					hKeys = append(hKeys, k)
				}
				sort.Strings(hKeys)
				hBytes, err := encodeStringKeyedMapFromKeys(hKeys, server.Remote.Headers)
				if err != nil {
					return nil, fmt.Errorf("encoding headers: %w", err)
				}
				if comma {
					buf.WriteByte(',')
				}
				comma = true
				keyB, _ := json.Marshal("headers")
				buf.Write(keyB)
				buf.WriteByte(':')
				buf.Write(hBytes)
			}
		}
	}

	// Unknown server fields sorted alphabetically.
	if len(server.Unknown) > 0 {
		unknownKeys := sortedStringSlice(server.Unknown)
		for _, key := range unknownKeys {
			if err := writeField(key, server.Unknown[key]); err != nil {
				return nil, err
			}
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encodeStringKeyedMap encodes a map[string]any as a JSON object with sorted keys.
func encodeStringKeyedMap(keys []string, m map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kBytes, _ := json.Marshal(key)
		buf.Write(kBytes)
		buf.WriteByte(':')
		vBytes, err := json.Marshal(m[key])
		if err != nil {
			return nil, fmt.Errorf("encoding key %q: %w", key, err)
		}
		buf.Write(vBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encodeStringKeyedMapFromKeys encodes a map[string]string as a JSON object.
func encodeStringKeyedMapFromKeys(keys []string, m map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kBytes, _ := json.Marshal(key)
		buf.Write(kBytes)
		buf.WriteByte(':')
		vBytes, err := json.Marshal(m[key])
		if err != nil {
			return nil, fmt.Errorf("encoding key %q: %w", key, err)
		}
		buf.Write(vBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// sortedStringSlice returns the keys of m sorted alphabetically.
func sortedStringSlice[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
