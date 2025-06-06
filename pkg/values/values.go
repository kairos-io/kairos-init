package values

import (
	"sort"
)

// Common Used for packages that are common to whatever key
const Common = "common"

type Architecture string

func (a Architecture) String() string {
	return string(a)
}

const (
	ArchAMD64  Architecture = "amd64"
	ArchARM64  Architecture = "arm64"
	ArchCommon Architecture = "common"
)

type Distro string

func (d Distro) String() string {
	return string(d)
}

// Individual distros for when we need to be specific
const (
	Unknown            Distro = "unknown"
	Debian             Distro = "debian"
	Ubuntu             Distro = "ubuntu"
	RedHat             Distro = "rhel"
	RockyLinux         Distro = "rocky"
	AlmaLinux          Distro = "almalinux"
	Fedora             Distro = "fedora"
	Arch               Distro = "arch"
	Alpine             Distro = "alpine"
	OpenSUSELeap       Distro = "opensuse-leap"
	OpenSUSETumbleweed Distro = "opensuse-tumbleweed"
	SLES               Distro = "sles"
)

type Family string

func (f Family) String() string {
	return string(f)
}

// generic families that have things in common and we can apply to all of them
const (
	UnknownFamily Family = "unknown"
	DebianFamily  Family = "debian"
	RedHatFamily  Family = "redhat"
	ArchFamily    Family = "arch"
	AlpineFamily  Family = "alpine"
	SUSEFamily    Family = "suse"
)

type Model string              // Model is the type of the system
func (m Model) String() string { return string(m) }

const (
	Generic Model = "generic"
	Rpi3    Model = "rpi3"
	Rpi4    Model = "rpi4"
	AgxOrin Model = "agx-orin"
)

type System struct {
	Name    string
	Distro  Distro
	Family  Family
	Version string
	Arch    Architecture
}

// GetTemplateParams returns a map of parameters that can be used in a template
func GetTemplateParams(s System) map[string]string {
	return map[string]string{
		"distro":  s.Distro.String(),
		"version": s.Version,
		"arch":    s.Arch.String(),
		"family":  s.Family.String(),
	}
}

type StepInfo struct {
	Key   string
	Value string
}

// StepsInfo returns a slice of StepInfo containing the steps and their descriptions
func StepsInfo() []StepInfo {
	steps := map[string]string{
		"init":             "The full init stage, which includes kairosRelease, kubernetes, initrd, services, workarounds and cleanup steps",
		"install":          "The full install stage, which includes installPackages, kubernetes, cloudconfigs, branding, grub, services, kairosBinaries, providerBinaries, initramfsConfigs and miscellaneous steps",
		"installPackages":  "installs the base system packages",
		"initrd":           "generates the initrd",
		"kairosRelease":    "creates and fills the /etc/kairos-release file",
		"workarounds":      "applies workarounds for known issues",
		"cleanup":          "cleans up the system of unneeded packages and files",
		"services":         "creates and enables required services",
		"kernel":           "installs the kernel",
		"kubernetes":       "installs the kubernetes provider",
		"cloudconfigs":     "installs the cloud-configs for the system",
		"branding":         "applies the branding for the system",
		"grub":             "configures the grub bootloader",
		"kairosBinaries":   "installs the kairos binaries",
		"providerBinaries": "installs the kairos provider binaries for k8s",
		"initramfsConfigs": "configures the initramfs for the system",
		"miscellaneous":    "applies miscellaneous configurations",
	}
	keys := make([]string, 0, len(steps))
	for k := range steps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([]StepInfo, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, StepInfo{Key: k, Value: steps[k]})
	}
	return ordered
}

// GetStepNames returns a slice of step names
func GetStepNames() []string {
	stepsInfo := StepsInfo()
	steps := make([]string, 0, len(stepsInfo))
	for step := range stepsInfo {
		steps = append(steps, stepsInfo[step].Key)
	}
	return steps

}
