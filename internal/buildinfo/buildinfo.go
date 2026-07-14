// Package buildinfo reports the Agent Bundler build version.
package buildinfo

import "runtime/debug"

var releaseVersion string

// Version returns the injected release version, the module version, or a development marker.
func Version() string {
	if releaseVersion != "" {
		return releaseVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "agbun-dev"
}
