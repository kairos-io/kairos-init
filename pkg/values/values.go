package values

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
	RedHat             Distro = "redhat"
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
