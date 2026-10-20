// Package version reports the build version of strig.
package version

import "runtime/debug"

// Version is set at build time via -ldflags "-X .../internal/version.Version=...".
// When unset, the module version recorded by `go install` is used instead.
var Version = ""

// String returns the best available version string.
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
