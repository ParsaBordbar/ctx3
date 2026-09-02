package version

import "runtime/debug"

var version string

const Module = "github.com/parsabordbar/ctx3"

func Version() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}
