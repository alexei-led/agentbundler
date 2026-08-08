package agentplugins

// PluginManifest is the decoded wire representation of a plugin.json file.
//
// Schema is always PluginSchemaURL. Unknown is the set of top-level JSON
// members beyond the portable 1.0.0 fields; clients treat these as
// non-fatal ignorable data. Extensions carries the client-specific
// reverse-domain namespace values from the extensions field.
type PluginManifest struct {
	// Schema is the $schema field value. Must equal PluginSchemaURL.
	Schema string

	// Name is the plugin identifier. Required.
	Name string

	// Version is the optional plugin release version.
	Version string

	// Description is the optional human-readable description.
	Description string

	// Author is the optional author name or contact.
	Author string

	// Homepage is the optional homepage URL string.
	Homepage string

	// Repository is the optional repository URL string.
	Repository string

	// License is the optional SPDX license identifier or expression.
	License string

	// Keywords is the optional list of search keywords.
	Keywords []string

	// Extensions holds opaque values from the extensions object, keyed by
	// reverse-domain namespace. Values are any JSON type; the extensions
	// field itself is optional.
	Extensions map[string]any

	// Unknown holds top-level JSON members not defined by the 1.0.0 portable
	// spec. Values are any JSON type. Preserved for round-trip reproduction.
	Unknown map[string]any
}
