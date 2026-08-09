package model

// CloneAgentPluginData returns a fully detached deep copy of AgentPluginData.
// All slices, maps, and byte slices are copied; pointer-typed fields get new
// allocations. Returns nil when data is nil.
func CloneAgentPluginData(data *AgentPluginData) *AgentPluginData {
	if data == nil {
		return nil
	}
	clone := &AgentPluginData{
		Profile:  data.Profile,
		Manifest: cloneAgentPluginManifest(data.Manifest),
	}
	if data.MCPServers != nil {
		clone.MCPServers = make([]MCPServer, len(data.MCPServers))
		for i, srv := range data.MCPServers {
			clone.MCPServers[i] = cloneMCPServer(srv)
		}
	}
	if data.Extensions != nil {
		clone.Extensions = make([]ClientExtension, len(data.Extensions))
		for i, ext := range data.Extensions {
			clone.Extensions[i] = cloneClientExtension(ext)
		}
	}
	if data.PackageFiles != nil {
		clone.PackageFiles = make([]PackageFile, len(data.PackageFiles))
		for i, pf := range data.PackageFiles {
			clone.PackageFiles[i] = clonePackageFile(pf)
		}
	}
	if data.UnknownManifest != nil {
		clone.UnknownManifest = CloneJSONMap(data.UnknownManifest)
	}
	if data.UnknownMCP != nil {
		clone.UnknownMCP = CloneJSONMap(data.UnknownMCP)
	}
	return clone
}

// CloneJSONMap returns a detached copy of a map containing JSON values.
func CloneJSONMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	clone := make(map[string]any, len(m))
	for k, v := range m {
		clone[k] = cloneJSONValue(v)
	}
	return clone
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return CloneJSONMap(typed)
	case []any:
		if typed == nil {
			return []any(nil)
		}
		clone := make([]any, len(typed))
		for i, item := range typed {
			clone[i] = cloneJSONValue(item)
		}
		return clone
	default:
		return value
	}
}

func cloneAgentPluginManifest(m AgentPluginManifest) AgentPluginManifest {
	clone := m
	if m.Keywords != nil {
		clone.Keywords = append([]string(nil), m.Keywords...)
	}
	return clone
}

func cloneMCPServer(srv MCPServer) MCPServer {
	clone := MCPServer{
		Name:      srv.Name,
		Transport: srv.Transport,
	}
	if srv.Stdio != nil {
		st := *srv.Stdio
		if srv.Stdio.Args != nil {
			st.Args = append([]string(nil), srv.Stdio.Args...)
		}
		if srv.Stdio.Env != nil {
			st.Env = make(map[string]string, len(srv.Stdio.Env))
			for k, v := range srv.Stdio.Env {
				st.Env[k] = v
			}
		}
		clone.Stdio = &st
	}
	if srv.Remote != nil {
		rt := *srv.Remote
		if srv.Remote.Headers != nil {
			rt.Headers = make(map[string]string, len(srv.Remote.Headers))
			for k, v := range srv.Remote.Headers {
				rt.Headers[k] = v
			}
		}
		clone.Remote = &rt
	}
	if srv.Unknown != nil {
		clone.Unknown = CloneJSONMap(srv.Unknown)
	}
	return clone
}

func clonePackageFile(pf PackageFile) PackageFile {
	clone := PackageFile{
		Path:       pf.Path,
		Executable: pf.Executable,
		SHA256:     pf.SHA256,
	}
	if pf.Bytes != nil {
		clone.Bytes = append([]byte(nil), pf.Bytes...)
	}
	clone.Origin = CloneSourceLocations(pf.Origin)
	return clone
}

func cloneClientExtension(ext ClientExtension) ClientExtension {
	clone := ClientExtension{
		Namespace: ext.Namespace,
		Manifest:  cloneJSONValue(ext.Manifest),
	}
	if ext.PackageFiles != nil {
		clone.PackageFiles = make([]PackageFile, len(ext.PackageFiles))
		for i, pf := range ext.PackageFiles {
			clone.PackageFiles[i] = clonePackageFile(pf)
		}
	}
	return clone
}

// CloneHookDescriptor returns a detached hook descriptor value.
func CloneHookDescriptor(descriptor HookDescriptor) HookDescriptor {
	clone := descriptor
	clone.Location = CloneSourceLocation(descriptor.Location)
	if descriptor.Matcher != nil {
		clone.Matcher = &HookMatcher{Tools: append([]HookToolCategory(nil), descriptor.Matcher.Tools...)}
	}
	clone.Handler.Program = cloneStringPointer(descriptor.Handler.Program)
	clone.Handler.ShellCommand = cloneStringPointer(descriptor.Handler.ShellCommand)
	clone.Environment = append([]string(nil), descriptor.Environment...)
	if descriptor.Handler.Arguments != nil {
		clone.Handler.Arguments = make([]HookArgument, len(descriptor.Handler.Arguments))
		for index, argument := range descriptor.Handler.Arguments {
			clone.Handler.Arguments[index] = HookArgument{
				Literal:     cloneStringPointer(argument.Literal),
				PackageFile: cloneRelativePathPointer(argument.PackageFile),
			}
		}
	}
	return clone
}

// CloneCommandDescriptor returns a detached command descriptor value.
func CloneCommandDescriptor(descriptor *CommandDescriptor) *CommandDescriptor {
	if descriptor == nil {
		return nil
	}
	return &CommandDescriptor{
		Identity: descriptor.Identity, Location: CloneSourceLocation(descriptor.Location),
		Name: descriptor.Name, Description: descriptor.Description,
	}
}

// CloneNativeResourceOptions returns a detached native resource configuration.
func CloneNativeResourceOptions(options *NativeResourceOptions) *NativeResourceOptions {
	if options == nil {
		return nil
	}
	return &NativeResourceOptions{PiExtensions: append([]RelativePath(nil), options.PiExtensions...)}
}

// CloneCapabilityUses returns detached capability use values.
func CloneCapabilityUses(uses []CapabilityUse) []CapabilityUse {
	if uses == nil {
		return nil
	}
	clones := make([]CapabilityUse, len(uses))
	for index, use := range uses {
		clones[index] = CapabilityUse{Key: use.Key, Location: CloneSourceLocation(use.Location)}
	}
	return clones
}

// CloneSourceLocations returns detached source location values.
func CloneSourceLocations(locations []SourceLocation) []SourceLocation {
	if locations == nil {
		return nil
	}
	clones := make([]SourceLocation, len(locations))
	for index, location := range locations {
		clones[index] = CloneSourceLocation(location)
	}
	return clones
}

// CloneSourceLocation returns a detached source location value.
func CloneSourceLocation(location SourceLocation) SourceLocation {
	return SourceLocation{Path: location.Path, Line: cloneIntPointer(location.Line), Column: cloneIntPointer(location.Column)}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneRelativePathPointer(value *RelativePath) *RelativePath {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
