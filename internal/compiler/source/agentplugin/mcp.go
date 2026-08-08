package agentplugin

import (
	"github.com/alexei-led/agentbundler/internal/agentplugins"
	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

// mapMCPConfig maps an agentplugins wire MCPConfig to []model.MCPServer.
// Servers are already in lexical name order from decode.
// The compiler does not execute, resolve, or validate MCP endpoints.
func mapMCPConfig(config agentplugins.MCPConfig) []model.MCPServer {
	if len(config.Servers) == 0 {
		return nil
	}
	servers := make([]model.MCPServer, 0, len(config.Servers))
	for _, ws := range config.Servers {
		servers = append(servers, mapMCPServer(ws))
	}
	return servers
}

func mapMCPServer(ws agentplugins.MCPServer) model.MCPServer {
	srv := model.MCPServer{
		Name:      ws.Name,
		Transport: mapTransport(ws.Transport),
	}
	if ws.Stdio != nil {
		stdio := &model.StdioMCPServer{
			Command: ws.Stdio.Command,
		}
		if ws.Stdio.Args != nil {
			stdio.Args = append([]string(nil), ws.Stdio.Args...)
		}
		if ws.Stdio.Env != nil {
			stdio.Env = make(map[string]string, len(ws.Stdio.Env))
			for k, v := range ws.Stdio.Env {
				stdio.Env[k] = v
			}
		}
		stdio.Cwd = ws.Stdio.Cwd
		srv.Stdio = stdio
	}
	if ws.Remote != nil {
		remote := &model.RemoteMCPServer{
			URL: ws.Remote.URL,
		}
		if ws.Remote.Headers != nil {
			remote.Headers = make(map[string]string, len(ws.Remote.Headers))
			for k, v := range ws.Remote.Headers {
				remote.Headers[k] = v
			}
		}
		srv.Remote = remote
	}
	if ws.Unknown != nil {
		srv.Unknown = cloneAnyMap(ws.Unknown)
	}
	return srv
}

func mapTransport(t agentplugins.MCPTransportType) model.MCPTransport {
	switch t {
	case agentplugins.MCPTransportStdio:
		return model.MCPTransportStdio
	case agentplugins.MCPTransportStreamableHTTP:
		return model.MCPTransportStreamableHTTP
	case agentplugins.MCPTransportSSE:
		return model.MCPTransportSSE
	default:
		return model.MCPTransport(t)
	}
}

// cloneAnyMap returns a shallow copy of map[string]any.
func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	clone := make(map[string]any, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}
