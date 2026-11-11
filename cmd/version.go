package cmd

import "runtime/debug"

// version is stamped at build time with
// -ldflags "-X github.com/dcadolph/ferry/cmd.version=v1.2.3".
var version = "dev"

// resolveVersion returns the stamped build version. A binary installed with go install carries
// no stamp, so it falls back to the module version from the build info, and to dev when there
// is none.
func resolveVersion() string {
	if version != "dev" && version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
