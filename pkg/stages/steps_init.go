package stages

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	semver "github.com/hashicorp/go-version"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
)

// This file contains the stages that are run during the init process

// GetInitrdStage Returns the initrd stage
// This stage cleans up any existing initrd files and creates a new one
// In the case of Trusted boot systems, we dont do anything but remove the initrd files as the initrd is created and
// signed during the build process
// If we have fips, we need to add the fips support to the initrd as well
func GetInitrdStage(sys values.System, logger types.KairosLogger) ([]schema.Stage, error) {
	stage := []schema.Stage{
		{
			Name: "Remove all initrds",
			Commands: []string{
				"rm -f /boot/initrd*",
				"rm -f /boot/initramfs*",
			},
		},
	}

	// If we are not using trusted boot we need to create a new initrd
	if !config.DefaultConfig.TrustedBoot {
		kernel, err := getLatestKernel(logger)
		if err != nil {
			logger.Logger.Error().Msgf("Failed to get the latest kernel: %s", err)
			return []schema.Stage{}, err
		}

		if config.DefaultConfig.Fips {
			// Add dracut fips support
			stage = append(stage, []schema.Stage{
				{
					Name:     "Add fips support to initramfs",
					OnlyIfOs: "Debian.*|Fedora.*|CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*|SLES.*|[O-o]penSUSE.*",
					Files: []schema.File{
						{
							Path:        "/etc/dracut.conf.d/kairos-fips.conf",
							Owner:       0,
							Group:       0,
							Permissions: 0644,
							Content:     "omit_dracutmodules+=\" iscsi iscsiroot \"\nadd_dracutmodules+=\" fips \"\n",
						},
					},
				},
			}...)
		}

		// Add proper network and systemd-sysext if needed
		// We default to systemd-networkd and sysext enbled and if its ubuntu <= 22.04 we need to use the plain network module and
		// disable sysext as they are not supported in those versions
		networkModule := "systemd-networkd"
		sysextModule := true

		if sys.Distro == values.Ubuntu {
			constraint, _ := semver.NewConstraint("<=22.04")
			ver, err := semver.NewVersion(sys.Version)
			if err != nil {
				logger.Logger.Error().Msgf("Failed to parse the version %s: %s", sys.Version, err)
				return []schema.Stage{}, err
			}
			// If its <= 22.04 we need to use the plain network module and disable sysext
			if constraint.Check(ver) {
				logger.Logger.Debug().Str("distro", string(sys.Distro)).Str("version", sys.Version).Msg("Using the plain network module and disabling sysext")
				networkModule = "network"
				sysextModule = false
			}
		}

		if sys.Distro == values.RockyLinux || sys.Distro == values.AlmaLinux || sys.Distro == values.RedHat {
			// On Rocky and AlmaLinux we need to use the plain network module
			logger.Logger.Debug().Str("distro", string(sys.Distro)).Str("version", sys.Version).Msg("Using the plain network module and disabling sysext")
			networkModule = "network"
		}

		stage = append(stage, []schema.Stage{
			{
				Name:     "Add proper network module to initramfs",
				OnlyIfOs: "Debian.*|Fedora.*|CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*|SLES.*|[O-o]penSUSE.*",
				Files: []schema.File{
					{
						Path:        "/etc/dracut.conf.d/kairos-network.conf",
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     fmt.Sprintf("add_dracutmodules+=\" %s \"\n", networkModule),
					},
				},
			},
		}...)

		if sysextModule {
			stage = append(stage, []schema.Stage{
				{
					Name:     "Add proper sysext module to initramfs",
					OnlyIfOs: "Debian.*|Fedora.*|CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*|SLES.*|[O-o]penSUSE.*",
					Files: []schema.File{
						{
							Path:        "/etc/dracut.conf.d/kairos-sysext.conf",
							Owner:       0,
							Group:       0,
							Permissions: 0644,
							Content:     fmt.Sprintf("add_dracutmodules+=\" systemd-sysext \"\n"),
						},
					},
				},
			}...)
		}

		stage = append(stage, []schema.Stage{
			{
				Name:     "Create new initrd",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*|SLES.*|[O-o]penSUSE.*",
				Commands: []string{
					fmt.Sprintf("depmod -a %s", kernel),
					fmt.Sprintf("dracut -v -f /boot/initrd %s", kernel),
				},
			},
			{
				Name:     "Create new initrd for Alpine",
				OnlyIfOs: "Alpine.*",
				Commands: []string{
					fmt.Sprintf("depmod -a %s", kernel),
					fmt.Sprintf("mkinitfs -o /boot/initrd %s", kernel),
				},
			},
		}...)
	}

	return stage, nil
}

// GetKairosReleaseStage Returns the kairos-release stage which creates the /etc/kairos-release file
// This file is very important as severals other pieces of Kairos refer to it.
// For example, for upgrading the version its taken from here
// During boot, grub checks this file to know things about the system and enable or disable stuff, like console for rpi images
func GetKairosReleaseStage(sis values.System, log types.KairosLogger) []schema.Stage {
	// TODO: Expand tis as this doesn't cover all the current fields
	// Current missing fields
	/*
			KAIROS_VERSION_ID="v3.2.4-36-g24ca209-v1.32.0-k3s1"
			KAIROS_GITHUB_REPO="kairos-io/kairos"
			KAIROS_IMAGE_REPO="quay.io/kairos/ubuntu:24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0-k3s1"
			KAIROS_ARTIFACT="kairos-ubuntu-24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0+k3s1"
			KAIROS_PRETTY_NAME="kairos-standard-ubuntu-24.04 v3.2.4-36-g24ca209-v1.32.0-k3s1"

		VERSION_ID and VERSION are the same, needed ?
		RELEASE is the short version of VERSION and VERSION_ID, the version without the k3s version needed?
		GITHUB_REPO is the repo where the image is stored, not really needed?
		PRETTY_NAME is the same as the ID_LIKE but different? needed?

	*/

	idLike := fmt.Sprintf("kairos-%s-%s-%s", config.DefaultConfig.Variant, sis.Distro.String(), sis.Version)
	flavor := sis.Distro.String()
	flavorRelease := sis.Version

	// TODO: Check if this affects sles versions? I don't think so as they are set like registry.suse.com/bci/bci-micro:15.6
	if strings.Contains(flavor, "opensuse") {
		// We store the suse version under the flavorRelease for some reason
		// So opensuse-leap:15.5 will be stored as `leap-15.5` with flavor being plain `opensuse`
		// Its a bit iffy IMHO but this is done so all opensuse stuff goes under the same repo instead of having
		// a repo for opensuse-leap and a repo for opensuse-tumbleweed
		flavorSplitted := strings.Split(flavor, "-")
		if len(flavorSplitted) == 2 {
			flavor = flavorSplitted[0]
			flavorRelease = fmt.Sprintf("%s-%s", flavorSplitted[1], sis.Version)
		} else {
			log.Debugf("Failed to split the flavor %s", flavor)
		}
	}

	// Back compat with old images
	// Before this we enforced the version to be vX.Y.Z
	// But now the version just cant be whatever semver version
	// The problem is that the upgrade checker uses a semver parses that marks anything without a v an invalid version :sob:
	// So now we need to enforce this forever
	release := config.DefaultConfig.KairosVersion.String()
	if release[0] != 'v' {
		release = fmt.Sprintf("v%s", release)
	}

	env := map[string]string{
		"KAIROS_ID":                "kairos", // What for?
		"KAIROS_ID_LIKE":           idLike,   // What for?
		"KAIROS_NAME":              idLike,   // What for? Same as ID_LIKE
		"KAIROS_VERSION":           release,
		"KAIROS_ARCH":              sis.Arch.String(),
		"KAIROS_TARGETARCH":        sis.Arch.String(), // What for? Same as ARCH
		"KAIROS_FLAVOR":            flavor,            // This should be in os-release as ID
		"KAIROS_FLAVOR_RELEASE":    flavorRelease,     // This should be in os-release as VERSION_ID
		"KAIROS_FAMILY":            sis.Family.String(),
		"KAIROS_MODEL":             config.DefaultConfig.Model,            // NEEDED or it breaks boot!
		"KAIROS_VARIANT":           config.DefaultConfig.Variant.String(), // TODO: Fully drop variant
		"KAIROS_REGISTRY_AND_ORG":  config.DefaultConfig.Registry,         // Needed for upgrades to search for images
		"KAIROS_BUG_REPORT_URL":    "https://github.com/kairos-io/kairos/issues",
		"KAIROS_HOME_URL":          "https://github.com/kairos-io/kairos",
		"KAIROS_RELEASE":           release,
		"KAIROS_FRAMEWORK_VERSION": config.DefaultConfig.FrameworkVersion,        // Just for info, could be dropped
		"KAIROS_FIPS":              fmt.Sprintf("%t", config.DefaultConfig.Fips), // Was the image built with FIPS support?
	}

	// Get SOFTWARE_VERSION from the k3s/k0s version
	if config.DefaultConfig.Variant == config.StandardVariant {
		log.Logger.Debug().Msg("Getting the k8s version for the kairos-release stage")
		var k8sVersion string

		switch config.DefaultConfig.KubernetesProvider {
		case config.K3sProvider:
			out, err := exec.Command("k3s", "--version").CombinedOutput()
			if err != nil {
				log.Logger.Error().Msgf("Failed to get the k3s version: %s", err)
			}
			// 2 lines in this format:
			// k3s version v1.21.4+k3s1 (3781f4b7)
			// go version go1.16.5
			// We need the first line
			re := regexp.MustCompile(`k3s version v(\d+\.\d+\.\d+\+k3s\d+)`)
			if re.MatchString(string(out)) {
				match := re.FindStringSubmatch(string(out))
				k8sVersion = match[1]
			} else {
				log.Logger.Error().Msgf("Failed to parse the k3s version: %s", string(out))
			}
		case config.K0sProvider:
			out, err := exec.Command("k0s", "version").CombinedOutput()
			if err != nil {
				log.Logger.Error().Msgf("Failed to get the k0s version: %s", err)
			}
			k8sVersion = strings.TrimSpace(string(out))
		}

		log.Logger.Debug().Str("k8sVersion", k8sVersion).Msg("Got the k8s version")
		env["KAIROS_SOFTWARE_VERSION"] = k8sVersion
		env["KAIROS_SOFTWARE_VERSION_PREFIX"] = string(config.DefaultConfig.KubernetesProvider)
	}

	log.Logger.Debug().Interface("env", env).Msg("Kairos release stage")

	return []schema.Stage{
		{
			Name:            "Write kairos-release",
			Environment:     env,
			EnvironmentFile: "/etc/kairos-release",
		},
	}
}

// GetWorkaroundsStage Returns the workarounds stage
// It applies some workarounds to the system to fix up inconsistent things or issues on the system
func GetWorkaroundsStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	stages := []schema.Stage{
		{
			Name: "Link grub-editenv to grub2-editenv",
			//OnlyIfOs: "Ubuntu.*|Alpine.*", // Maybe not needed and just checking if the file exists is enough
			If: "test -f /usr/bin/grub-editenv",
			Commands: []string{
				"ln -s /usr/bin/grub-editenv /usr/bin/grub2-editenv",
			},
		},
		{
			Name:     "Fixup sudo perms",
			OnlyIfOs: "Ubuntu.*|Debian.*",
			Commands: []string{
				"chown root:root /usr/bin/sudo",
				"chmod 4755 /usr/bin/sudo",
			},
		},
	}

	return stages
}

// GetCleanupStage Returns the cleanup stage
// This stage is mainly about cleaning up the system and removing unneeded packages
// As some of the software installed can mess with he system and we dont want to have it in an inconsistent state
// I also removes some packages that are no longer needed, like dracut and dependant packages as once
// we have build the initramfs we dont need them anymore
func GetCleanupStage(sis values.System, l types.KairosLogger) []schema.Stage {
	stages := []schema.Stage{
		{
			Name: "Remove dbus machine-id",
			If:   "test -f /var/lib/dbus/machine-id",
			Commands: []string{
				"rm -f /var/lib/dbus/machine-id",
			},
		},
		{
			Name: "truncate machine-id",
			If:   "test -f /etc/machine-id",
			Commands: []string{
				"truncate -s 0 /etc/machine-id",
			},
		},
		{
			Name: "truncate hostname",
			If:   "test -f /etc/hostname",
			Commands: []string{
				"truncate -s 0 /etc/hostname",
			},
		},
	}

	var pkgs []values.VersionMap

	if config.DefaultConfig.TrustedBoot {
		// Try to remove as many packages as possible that are not needed
		pkgs = append(pkgs, values.ImmucorePackages[sis.Distro][values.ArchCommon])
		pkgs = append(pkgs, values.ImmucorePackages[sis.Family][values.ArchCommon])
		pkgs = append(pkgs, values.ImmucorePackages[sis.Distro][sis.Arch])
		pkgs = append(pkgs, values.ImmucorePackages[sis.Family][sis.Arch])
		pkgs = append(pkgs, values.GrubPackages[sis.Distro][values.ArchCommon])
		pkgs = append(pkgs, values.GrubPackages[sis.Family][values.ArchCommon])
		pkgs = append(pkgs, values.GrubPackages[sis.Distro][sis.Arch])
		pkgs = append(pkgs, values.GrubPackages[sis.Family][sis.Arch])
	} else {
		// Now that initramfs is built we can drop those packages
		pkgs = append(pkgs, values.ImmucorePackages[sis.Distro][values.ArchCommon])
		pkgs = append(pkgs, values.ImmucorePackages[sis.Family][values.ArchCommon])
		pkgs = append(pkgs, values.ImmucorePackages[sis.Distro][sis.Arch])
		pkgs = append(pkgs, values.ImmucorePackages[sis.Family][sis.Arch])
	}

	filteredPkgs := values.FilterPackagesOnConstraint(sis, l, pkgs)
	stages = append(stages, []schema.Stage{
		{
			Name: "Remove unneeded packages",
			Packages: schema.Packages{
				Remove: filteredPkgs,
			},
		},
		{ // TODO: Send this upstream to the yip Packages plugin?
			Name:     "Auto remove packages in Debian family",
			OnlyIfOs: "Ubuntu.*|Debian.*",
			Commands: []string{
				"apt-get autoremove -y",
			},
		},
	}...)
	return stages
}

// GetServicesStage Returns the services stage
// This stage is about configuring the services to be run on the system. Either enabling or disabling them.
func GetServicesStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
		{
			Name:     "Enable services for Modern systems",
			OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"systemd-networkd", // Separate this and use ifOS to trigger it only on systemd systems? i.e. do a reverse regex match somehow
				},
			},
		},
		{
			Name:     "Enable services for Debian family",
			OnlyIfOs: "Ubuntu.*|Debian.*",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"ssh",
				},
			},
		},
		{
			Name:     "Enable services for RHEL family",
			OnlyIfOs: "Fedora.*|CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"sshd",
					"systemd-resolved",
				},
				Disable: []string{
					"dnf-makecache",
					"dnf-makecache.timer",
				},
			},
		},
		{
			Name:     "Enable services for Alpine family",
			OnlyIfOs: "Alpine.*",
			Commands: []string{
				"rc-update add sshd boot",
				"rc-update add connman boot ",
				"rc-update add acpid boot",
				"rc-update add hwclock boot",
				"rc-update add syslog boot",
				"rc-update add udev sysinit",
				"rc-update add udev-trigger sysinit",
				"rc-update add cgroups sysinit",
				"rc-update add ntpd boot",
				"rc-update add crond",
				"rc-update add fail2ban",
			},
		},
	}
}

// GetKernelStage Returns the kernel stage
// This stage is about configuring the kernel to be used on the system. Mainly we already have a kernel
// but all things kairos look for the /boot/vmlinuz file to be there
// So this creates a link to the actual kernel, no matter the version so we can boot the same everywhere
// This stage also cleans up the old kernels and initrd files that are no longer needed.
// This is a bit of a complex one, as every distro has its own way of doing things but we make it work here
func GetKernelStage(_ values.System, logger types.KairosLogger) ([]schema.Stage, error) {
	kernel, err := getLatestKernel(logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the latest kernel: %s", err)
		return []schema.Stage{}, err
	}

	return []schema.Stage{
		{
			Name: "Clean current kernel link",
			If:   "test -f /boot/vmlinuz",
			Commands: []string{
				"rm /boot/vmlinuz",
			},
		},
		{
			Name: "Clean current kernel link if its a symlink",
			If:   "test -L /boot/Image",
			Commands: []string{
				"rm /boot/Image",
			},
		},
		{
			Name: "Clean old kernel link",
			If:   "test -f /boot/vmlinuz.old",
			Commands: []string{
				"rm /boot/vmlinuz.old",
			},
		},
		{
			Name: "Clean debug kernel",
			If:   fmt.Sprintf("test -f /boot/vmlinux-%s", kernel),
			Commands: []string{
				fmt.Sprintf("rm /boot/vmlinux-%s", kernel),
			},
		},
		{
			Name: "Link kernel for Nvidia AGX Orin",              // Nvidia AGX Orin has the kernel in the Image file directly
			If:   "test -e /boot/Image && test ! -L /boot/Image", // If its not a symlink then its the kernel so link it to our expected location
			Commands: []string{
				"ln -s /boot/Image /boot/vmlinuz",
			},
		},
		{ // On Fedora, if we don't have grub2 installed, it wont copy the kernel and rename it to the /boot dir, so we need to do it manually
			// TODO: Check if this is needed on AlmaLinux/RockyLinux/RedHatLinux
			Name:     "Copy kernel for Fedora Trusted Boot",
			OnlyIfOs: "Fedora.*",
			If:       fmt.Sprintf("test ! -f /boot/vmlinuz-%s && test -f /usr/lib/modules/%s/vmlinuz", kernel, kernel),
			Commands: []string{
				fmt.Sprintf("cp /usr/lib/modules/%s/vmlinuz /boot/vmlinuz-%s", kernel, kernel),
			},
		},
		{
			Name: "Link kernel",
			If:   fmt.Sprintf("test -f /boot/vmlinuz-%s", kernel),
			Commands: []string{
				fmt.Sprintf("ln -s /boot/vmlinuz-%s /boot/vmlinuz", kernel),
			},
		},
		{
			Name: "Link kernel",
			If:   fmt.Sprintf("test -f /boot/Image-%s", kernel), // On suse arm64 kernel starts with Image
			Commands: []string{
				fmt.Sprintf("ln -s /boot/Image-%s /boot/vmlinuz", kernel),
			},
		},
		{
			Name: "Link kernel for Alpine",
			If:   "test -f /boot/vmlinuz-lts",
			Commands: []string{
				"ln -s /boot/vmlinuz-lts /boot/vmlinuz",
			},
		},
		{
			Name: "Link kernel for Alpine RPI",
			If:   "test -f /boot/vmlinuz-rpi",
			Commands: []string{
				"ln -s /boot/vmlinuz-rpi /boot/vmlinuz",
			},
		},
	}, nil
}

// getLatestKernel returns the latest kernel version installed on the system
func getLatestKernel(l types.KairosLogger) (string, error) {
	var kernelVersion string
	modulesPath := "/lib/modules"
	// Read the directories under /lib/modules
	dirs, err := os.ReadDir(modulesPath)
	if err != nil {
		l.Logger.Error().Msgf("Failed to read the directory %s: %s", modulesPath, err)
		return kernelVersion, err
	}

	var versions []*semver.Version
	var version *semver.Version
	for _, dir := range dirs {
		if dir.IsDir() {
			// Parse the directory name as a semver version
			version, err = semver.NewVersion(dir.Name())
			if err != nil {
				l.Logger.Debug().Err(err).Str("version", dir.Name()).Msg("Failed to parse the version as semver, will use the full name instead")
				continue
			}
			versions = append(versions, version)
		}
	}

	// We could have no semver version but custom versions like 5.4.0-101-generic.fc32.x86_64
	// In that case we need to just use the full name
	if len(versions) == 0 {
		if len(dirs) >= 1 {
			kernelVersion = dirs[0].Name()
		} else {
			return kernelVersion, fmt.Errorf("no kernel versions found")
		}
	} else {
		sort.Sort(semver.Collection(versions))
		kernelVersion = versions[0].String()
		if kernelVersion == "" {
			l.Logger.Error().Msgf("Failed to find the latest kernel version")
			return kernelVersion, fmt.Errorf("failed to find the latest kernel")
		}
	}

	return kernelVersion, nil
}
