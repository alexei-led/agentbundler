# Agent Plugins Target Adapter

**Path**: `internal/target/agentplugins/`

## Purpose

Renders normalized agent-plugin packages to the standard Agent Plugins 1.0.0 wire format. This is the authoritative adapter for all portable agent-plugin capabilities.

## Functional Responsibilities

- Declare all seven portable agent-plugin capabilities as native.
- Reject aggregate package mode; accept only separate mode.
- Render plugin.json from `AgentPluginData.Manifest` + unknown fields using the pinned wire encoder.
- Render mcp.json when MCP servers or unknown MCP fields are present.
- Render skill assets as `<skill-name>/SKILL.md` plus support files.
- Render extension package files under `extensions/<namespace>/<path>`.
- Render regular package files at their declared path.
- Set one explicit archive unit per plugin package (root = package identity, suffix = .tar.gz).

## Constraints

- No filesystem, network, process, or environment access in Render.
- No aggregate mode.
- Output uses only plan bytes from AgentPluginData and normalized skill assets.
- Format revision bumps whenever the wire encoding or capability set changes.
