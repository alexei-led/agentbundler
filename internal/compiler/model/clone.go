package model

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
