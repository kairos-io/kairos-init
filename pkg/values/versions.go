package values

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	version = "v0.0.1"
	// gitCommit is the git sha1 + dirty if build from a dirty git.
	gitCommit = "none"
	// The packages below get auto updated by renovate on github
	// We built those under github.com/kairos/packages
	// renovate: datasource=docker depName=quay.io/kairos/framework
	frameWorkVersion = "v2.22.0"
	// renovate: datasource=docker
	providerPackage = "quay.io/kairos/packages:provider-kairos-system-2.10.4"
	// renovate: datasource=docker
	edgeVpnPackage = "quay.io/kairos/packages:edgevpn-utils-0.30.2"
	// renovate: datasource=docker
	k9sPackage = "quay.io/kairos/packages:k9s-utils-0.50.4"
	// renovate: datasource=docker
	nerdctlPackage = "quay.io/kairos/packages:nerdctl-utils-2.0.4"
	// renovate: datasource=docker
	kubeVipPackage = "quay.io/kairos/packages:kube-vip-utils-0.9.0"
)

func GetFrameworkVersion() string {
	return frameWorkVersion
}

// setProperRepo sets the proper repo for arm64
// As we are not pushing the luet packages to the same repo and have a different repo for arm64
// we need to check if the arch is arm64 and set the proper repo to a different address
// as this is the same for all packages, its easier to track just one repo as versions should be the same
func setProperRepo(arch string, url string) string {
	data := url
	if arch == "arm64" {
		splitted := strings.Split(url, ":")
		if len(splitted) > 1 {
			data = fmt.Sprintf("%s-arm64:%s", splitted[0], splitted[1])
		}
	}
	return data
}

func GetProviderPackage(arch string) string {
	return setProperRepo(arch, providerPackage)
}

func GetEdgeVPNPackage(arch string) string {
	return setProperRepo(arch, edgeVpnPackage)
}

func GetK9sPackage(arch string) string {
	return setProperRepo(arch, k9sPackage)
}

func GetNerdctlPackage(arch string) string {
	return setProperRepo(arch, nerdctlPackage)
}

func GetKubeVipPackage(arch string) string {
	return setProperRepo(arch, kubeVipPackage)
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

// GetFullVersion returns the full build info.
func GetFullVersion() BuildInfo {
	v := BuildInfo{
		Version:   GetVersion(),
		GitCommit: gitCommit,
		GoVersion: runtime.Version(),
	}

	return v
}
