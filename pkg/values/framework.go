package values

import "runtime"

var (
	version = "v0.0.1"
	// gitCommit is the git sha1 + dirty if build from a dirty git.
	gitCommit = "none"
	// This gets auto updated by renovate on github
	// renovate: datasource=docker depName=quay.io/kairos/framework versioning=semver
	frameWorkVersion = "v2.15.11"
)

func GetFrameworkVersion() string {
	return frameWorkVersion
}

func GetVersion() string {
	return version
}

// BuildInfo describes the compiled time information.
type BuildInfo struct {
	// Version is the current semver.
	Version string `json:"version,omitempty"`
	// GitCommit is the git sha1.
	GitCommit string `json:"git_commit,omitempty"`
	// GoVersion is the version of the Go compiler used.
	GoVersion string `json:"go_version,omitempty"`
}

// Get returns build info.
func GetFullVersion() BuildInfo {
	v := BuildInfo{
		Version:   GetVersion(),
		GitCommit: gitCommit,
		GoVersion: runtime.Version(),
	}

	return v
}
