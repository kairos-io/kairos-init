package system

import (
	"github.com/sanity-io/litter"
	"os"
	"runtime"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kairos-io/kairos-init/pkg/values"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
)

// DetectSystem detects the system based on the os-release file
// and returns a values.System struct
// This could probably be implemented in a different way, or use a lib but its helpful
// in conjunction with the values packagemaps to determine the packages to install
func DetectSystem(l sdkTypes.KairosLogger) values.System {
	// Detects the system
	s := values.System{
		Distro: values.Unknown,
		Family: values.UnknownFamily,
	}

	osReleasePath := os.Getenv("KAIROS_OS_RELEASE_PATH")
	if osReleasePath == "" {
		osReleasePath = "/etc/os-release"
	}
	file, err := os.Open(osReleasePath)
	if err != nil {
		return s
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)
	val, err := godotenv.Parse(file)
	if err != nil {
		return s
	}
	l.Logger.Trace().Interface("values", val).Msg("Read values from os-release")
	// Match values to distros
	switch values.Distro(val["ID"]) {
	case values.Debian:
		s.Distro = values.Debian
		s.Family = values.DebianFamily
	case values.Ubuntu:
		s.Distro = values.Ubuntu
		s.Family = values.DebianFamily
	case values.Fedora:
		s.Distro = values.Fedora
		s.Family = values.RedHatFamily
	case values.RockyLinux:
		s.Distro = values.RockyLinux
		s.Family = values.RedHatFamily
	case values.AlmaLinux:
		s.Distro = values.AlmaLinux
		s.Family = values.RedHatFamily
	case values.RedHat:
		s.Distro = values.RedHat
		s.Family = values.RedHatFamily
	case values.Arch:
		s.Distro = values.Arch
		s.Family = values.ArchFamily
	case values.Alpine:
		s.Distro = values.Alpine
		s.Family = values.AlpineFamily
	case values.OpenSUSELeap:
		s.Distro = values.OpenSUSELeap
		s.Family = values.SUSEFamily
	case values.OpenSUSETumbleweed:
		s.Distro = values.OpenSUSETumbleweed
		s.Family = values.SUSEFamily
	case values.SLES:
		s.Distro = values.SLES
		s.Family = values.SUSEFamily
	}

	// Match architecture
	switch values.Architecture(runtime.GOARCH) {
	case values.ArchAMD64:
		s.Arch = values.ArchAMD64
	case values.ArchARM64:
		s.Arch = values.ArchARM64
	}

	// Check if we are still unknown
	if s.Distro == values.Unknown {
		// Check ID_LIKE value
		// For some derivatives they ID will be their own but the ID_LIKE will be the parent
		// So we may be able to detect the parent and use the same family and such
		switch values.Family(val["ID_LIKE"]) {
		case values.DebianFamily:
			s.Distro = values.Debian
			s.Family = values.DebianFamily
		case values.RedHatFamily, values.Family(values.Fedora):
			s.Distro = values.Fedora
			s.Family = values.RedHatFamily
		case values.ArchFamily:
			s.Distro = values.Arch
			s.Family = values.ArchFamily
		case values.SUSEFamily:
			s.Distro = values.OpenSUSELeap
			s.Family = values.SUSEFamily
		}
	}

	// Store the version
	s.Version = val["VERSION_ID"]
	if s.Distro == values.Alpine {
		// We currently only do major.minor for alpine, even if os-release reports also the patch
		// So for backwards compatibility we will only store the major.minor
		splittedVersion := strings.Split(s.Version, ".")
		if len(splittedVersion) == 3 {
			s.Version = splittedVersion[0] + "." + splittedVersion[1]
		} else {
			l.Debugf("Could not split version for alpine, using default as is: %s", s.Version)
		}
	}

	// Store the name
	s.Name = val["PRETTY_NAME"]
	// Fallback to normal name
	if s.Name == "" {
		s.Name = val["NAME"]
	}

	l.Debugf("Detected system: %s", litter.Sdump(s))
	return s
}
