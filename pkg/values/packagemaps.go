package values

import (
	"bytes"
	"github.com/kairos-io/kairos-init/pkg/config"

	semver "github.com/hashicorp/go-version"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
)
import "text/template"

// packagemaps is a map of packages to install for each distro.
// so we can deal with stupid different names between distros.

// The format is usually a map[Distro]map[Architecture][]string
// So we can store the packages for each distro and architecture independently
// Except common packages, which are named the same across all distros
// Packages can be templated, so we can pass a map of parameters to replace in the package name
// So we can transform "linux-image-generic-hwe-{{.VERSION}}" into the proper version for each ubuntu release
// the params are not hardcoded or autogenerated anywhere yet.
// Ideally the System struct should have a method to generate the params for the packages automatically
// based on the distro and version, so we can pass them to the installer without anything from our side.
// Either we set also a Common key for the common packages, or we just duplicate them for both arches if needed
//

// CommonPackages are packages that are named the same across all distros and arches
var CommonPackages = []string{
	"curl",       // Basic tool. Also needed for netbooting as it is used to download the netboot artifacts
	"file",       // Basic tool.
	"gawk",       // Basic tool.
	"iptables",   // Basic tool.
	"less",       // Basic tool.
	"nano",       // Basic tool.
	"sudo",       // Basic tool. Needed for the user to be able to run commands as root
	"tar",        // Basic tool.
	"zstd",       // Compression support for zstd
	"rsync",      // Install, upgrade, reset use it to sync the files
	"systemd",    // Basic tool.
	"dbus",       // Basic tool.
	"lvm2",       // Seems to be used to support rpi3 only
	"jq",         // No idea why we need it, check if we can drop it?
	"dosfstools", // For the fat32 partition on EFI systems
	"e2fsprogs",  // mkfs support for ext2/3/4
	"parted",     // Partitioning support, check if we need it anymore
}

type PackageMap map[Distro]map[Architecture]VersionMap
type VersionMap map[string][]string

// ImmucorePackages are the minimum set of packages that immucore needs.
// Otherwise you wont be able to build the initrd with immucore on it.
var ImmucorePackages = PackageMap{
	Ubuntu: {
		ArchAMD64: {
			Common: {
				"dracut",            // To build the initrd
				"dracut-network",    // Network-legacy support for dracut
				"isc-dhcp-common",   // Network-legacy support for dracut, basic tools
				"isc-dhcp-client",   // Network-legacy support for dracut, basic tools
				"systemd-sysv",      // No idea, drop it?
				"cloud-guest-utils", // This brings growpart, so we can resize the partitions
			},
			">=22.04": {
				"dracut-live", // Livenet support for dracut, split into a separate package on 22.04
			},
		},
		ArchARM64: {},
	},
	Fedora: {
		ArchAMD64: {
			Common: {
				"dracut",
				"dracut-live",
				"dracut-network",
				"dracut-squash",
				"squashfs-tools",
				"dhcp-client",
			},
		},
		ArchARM64: {},
	},
}

// KernelPackages is a map of packages to install for each distro.
// No arch required here, maybe models will need different packages?
var KernelPackages = PackageMap{
	Ubuntu: {
		ArchAMD64: {
			">=20.04, != 24.10": {
				// This is a template, so we can replace the version with the actual version of the system
				"linux-image-generic-hwe-{{.version}}",
			},
			// Somehow 24.10 uses the 22.04 hwe kernel
			"24.10": {"linux-image-generic-hwe-24.04"},
		},
	},
	Fedora: {
		ArchCommon: {
			Common: {
				"kernel",
				"kernel-modules",
				"kernel-modules-extra",
			},
		},
	},
}

// BasePackages is a map of packages to install for each distro and architecture.
// This comprises the base packages that are needed for the system to work on a Kairos system
var BasePackages = PackageMap{
	Ubuntu: {
		ArchCommon: {
			Common: {
				"gdisk",           // Yip requires it for partitioning
				"fdisk",           // Yip requires it for partitioning
				"ca-certificates", // Basic certificates for secure communication
				"conntrack",
				"console-data",      // Console font support
				"cloud-guest-utils", // Yip requires it, this brings growpart, so we can resize the partitions
				"cryptsetup",        // For encrypted partitions support
				"debianutils",
				"gettext",
				"haveged",
				"iproute2",
				"iputils-ping",
				"krb5-locales",
				"nbd-client",
				"nfs-common",
				"open-iscsi",
				"open-vm-tools",  // For vmware support, probably move it to a bundle?
				"openssh-server", // Basic ssh server
				"systemd-timesyncd",
				"systemd-container",      // Not sure if needed?
				"ubuntu-advantage-tools", // For ubuntu advantage support, enablement of ubuntu services
				"xz-utils",               // Compression support for xz
				"tpm2-tools",             // For TPM support, mainly trusted boot
				"dmsetup",                // Device mapper support, needed for lvm and cryptsetup
				"mdadm",                  // For software raid support, not sure if needed?
				"ncurses-term",
				"networkd-dispatcher",
				"packagekit-tools",
				"publicsuffix",
				"xdg-user-dirs",
				"xxd",
				"zerofree",
			},
			">=24.04": {
				"systemd-resolved", // For systemd-resolved support, added as a separate package on 24.04
			},
		},
		ArchAMD64: {},
		ArchARM64: {},
	},
	RedHat: {},
	Fedora: {
		ArchCommon: {
			Common: {
				"gdisk",                // Yip requires it for partitioning, maybe BasePackages
				"audit",                // For audit support, check if needed?
				"cracklib-dicts",       // Password dictionary support
				"cloud-utils-growpart", // grow partition use. Check if yip still needs it?
				"device-mapper",        // Device mapper support, needed for lvm and cryptsetup
				"haveged",              // Random number generator, check if needed?
				"openssh-server",
				"openssh-clients",
				"polkit",
				"qemu-guest-agent",
				"systemd-networkd",
				"systemd-resolved",
				"which",      // Basic tool. Basepackages?
				"cryptsetup", // For encrypted partitions support, needed for trusted boot and dracut building
			},
		},
	},
	Alpine: {},
	Arch:   {},
	Debian: {},
}

// GrubPackages is a map of packages to install for each distro and architecture.
// TODO: Check why some packages we only install on amd64 and not on arm64?? Like neovim???
// Note: some of the packages seems to be onyl installed here as we dont have any size restraints
// And we dont want to have Trusted Boot have those packages, as we want it small.
// we should probably move those into a new PackageMap called ExtendedPackages or something like that
// instead of merging them with grub packages.
var GrubPackages = PackageMap{
	Ubuntu: {
		ArchAMD64: {
			Common: {
				"grub2",                 // Basic grub support
				"grub-efi-amd64-bin",    // Basic grub support for EFI
				"grub-efi-amd64-signed", // For secure boot support
				"grub-pc-bin",           // Basic grub support for BIOS, probably needed byt AuroraBoot to build hybrid isos?
				"coreutils",             // Basic tools, probably needs to be part of BasePackages?
				"grub2-common",          // Basic grub support
				"kbd",                   // Keyboard configuration
				"lldpd",                 // For lldp support, check if needed?
				"neovim",                // For neovim support, check if needed? Move to BasePackages if so?
				"shim-signed",           // For secure boot support
				"snmpd",                 // For snmp support, check if needed? Move to BasePackages if so?
				"squashfs-tools",        // For squashfs support, probably needs to be part of BasePackages
				"zfsutils-linux",        // For zfs tools (zfs and zpool), probably needs to be part of BasePackages
			},
		},
		ArchARM64: {
			Common: {
				"grub-efi-arm64",        // Basic grub support for EFI
				"grub-efi-arm64-bin",    // Basic grub support for EFI
				"grub-efi-arm64-signed", // For secure boot support
			},
		},
	},
	Fedora: {
		ArchCommon: {
			Common: {
				"grub2",
			},
		},
		ArchAMD64: {
			Common: {
				"grub2-efi-x64",
				"grub2-efi-x64-modules",
				"grub2-pc",
				"shim-x64",
			},
		},
		ArchARM64: {
			Common: {
				"grub2-efi-aa64",
				"grub2-efi-aa64-modules",
				"shim-aa64",
			},
		},
	},
}

// SystemdPackages is a map of packages to install for each distro and architecture for systemd-boot (trusted boot) variants
// TODO: Check why some packages we only install on amd64 and not on arm64?? Like kmod???
var SystemdPackages = PackageMap{
	Ubuntu: {
		ArchCommon: {
			Common: {
				"systemd",
			},
			">=24.04": {
				"iucode-tool",
				"kmod",
				"linux-base",
				"systemd-boot", // Trusted boot support, it was split as a package on 24.04
			},
		},
	},
}

// PackageListToTemplate takes a list of packages and a map of parameters to replace in the package name
// and returns a list of packages with the parameters replaced.
func PackageListToTemplate(packages []string, params map[string]string, l sdkTypes.KairosLogger) ([]string, error) {
	var finalPackages []string
	for _, pkg := range packages {
		var result bytes.Buffer
		tmpl, err := template.New("versionTemplate").Parse(pkg)
		if err != nil {
			l.Logger.Error().Err(err).Str("package", pkg).Msg("Error parsing template.")
			return []string{}, err
		}
		err = tmpl.Execute(&result, params)
		if err != nil {
			l.Logger.Error().Err(err).Str("package", pkg).Msg("Error executing template.")
			return []string{}, err
		}
		finalPackages = append(finalPackages, result.String())
	}
	return finalPackages, nil
}

func GetPackages(s System, l sdkTypes.KairosLogger) ([]string, error) {
	mergedPkgs := CommonPackages
	systemVersion, err := semver.NewVersion(s.Version)
	if err != nil {
		return nil, err
	}

	// Go over all packages maps
	filteredPackages := []VersionMap{
		BasePackages[s.Distro][ArchCommon],   // Common packages to both arches
		BasePackages[s.Distro][s.Arch],       // Specific packages for the arch
		KernelPackages[s.Distro][ArchCommon], // Common kernel packages to both arches
		KernelPackages[s.Distro][s.Arch],     // Specific kernel packages for the arch
	}

	if config.DefaultConfig.TrustedBoot {
		// Install only systemd-boot packages
		filteredPackages = append(filteredPackages, SystemdPackages[s.Distro][ArchCommon])
		filteredPackages = append(filteredPackages, SystemdPackages[s.Distro][s.Arch])
	} else {
		// install grub and immucore packages
		filteredPackages = append(filteredPackages, GrubPackages[s.Distro][ArchCommon])
		filteredPackages = append(filteredPackages, GrubPackages[s.Distro][s.Arch])
		filteredPackages = append(filteredPackages, ImmucorePackages[s.Distro][ArchCommon])
		filteredPackages = append(filteredPackages, ImmucorePackages[s.Distro][s.Arch])
	}

	// Go over each list of packages
	for _, packages := range filteredPackages {
		// for each package map, check if the version matches the constraint
		for constraint, values := range packages {
			// Add them if they are common
			l.Logger.Debug().Str("constraint", constraint).Str("version", systemVersion.String()).Msg("Checking constraint")
			if constraint == Common {
				l.Logger.Debug().Strs("packages", values).Msg("Adding common packages")
				mergedPkgs = append(mergedPkgs, values...)
				continue
			}
			semverConstraint, err := semver.NewConstraint(constraint)
			if err != nil {
				l.Logger.Error().Err(err).Str("constraint", constraint).Msg("Error parsing constraint.")
				continue
			}
			// Also add them if the constraint matches
			if semverConstraint.Check(systemVersion) {
				l.Logger.Debug().Strs("packages", values).Msg("Constraint matches, adding packages")
				mergedPkgs = append(mergedPkgs, values...)
			}
		}
	}

	return mergedPkgs, nil
}
