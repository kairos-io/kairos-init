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

const (
	InitStage            = "init"             // Full init stage
	InstallStage         = "install"          // Full install stage
	InstallPackagesStep  = "installPackages"  // Installs the base system packages
	InitrdStep           = "initrd"           // Generates the initrd
	KairosReleaseStep    = "kairosRelease"    // Creates and fills the /etc/kairos-release file
	WorkaroundsStep      = "workarounds"      // Applies workarounds for known issues
	CleanupStep          = "cleanup"          // Cleans up the system of unneeded packages and files
	ServicesStep         = "services"         // Creates and enables required services
	KernelStep           = "kernel"           // Installs the kernel
	KubernetesStep       = "kubernetes"       // Installs the kubernetes provider
	CloudconfigsStep     = "cloudconfigs"     // Installs the cloud-configs for the system
	BrandingStep         = "branding"         // Applies the branding for the system
	GrubStep             = "grub"             // Configures the grub bootloader
	KairosBinariesStep   = "kairosBinaries"   // Installs the kairos binaries
	ProviderBinariesStep = "providerBinaries" // Installs the kairos provider binaries for k8s
	InitramfsConfigsStep = "initramfsConfigs" // Configures the initramfs for the system
	MiscellaneousStep    = "miscellaneous"    // Applies miscellaneous configurations
)

// StepsInfo returns a slice of StepInfo containing the steps and their descriptions
func StepsInfo() []StepInfo {
	steps := map[string]string{
		InitStage:            "The full init stage, which includes kairosRelease, kubernetes, initrd, services, workarounds and cleanup steps",
		InstallStage:         "The full install stage, which includes installPackages, kubernetes, cloudconfigs, branding, grub, services, kairosBinaries, providerBinaries, initramfsConfigs and miscellaneous steps",
		InstallPackagesStep:  "installs the base system packages",
		InitrdStep:           "generates the initrd",
		KairosReleaseStep:    "creates and fills the /etc/kairos-release file",
		WorkaroundsStep:      "applies workarounds for known issues",
		CleanupStep:          "cleans up the system of unneeded packages and files",
		ServicesStep:         "creates and enables required services",
		KernelStep:           "installs the kernel",
		KubernetesStep:       "installs the kubernetes provider",
		CloudconfigsStep:     "installs the cloud-configs for the system",
		BrandingStep:         "applies the branding for the system",
		GrubStep:             "configures the grub bootloader",
		KairosBinariesStep:   "installs the kairos binaries",
		ProviderBinariesStep: "installs the kairos provider binaries for k8s",
		InitramfsConfigsStep: "configures the initramfs for the system",
		MiscellaneousStep:    "applies miscellaneous configurations",
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
