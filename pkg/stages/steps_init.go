package stages

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/kairos-io/kairos-init/pkg/bundled"

	semver "github.com/hashicorp/go-version"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/kairos-io/kairos-sdk/types/logger"
	"github.com/mudler/go-pluggable"
	"github.com/mudler/yip/pkg/schema"
)

// This file contains the stages that are run during the init process

// GetInitrdStage Returns the initrd stage
// This stage cleans up any existing initrd files and creates a new one
// In the case of Trusted boot systems, we dont do anything but remove the initrd files as the initrd is created and
// signed during the build process
// If we have fips, we need to add the fips support to the initrd as well
func GetInitrdStage(_ values.System, logger logger.KairosLogger) ([]schema.Stage, error) {
	if config.ContainsSkipStep(values.InitrdStep) {
		logger.Logger.Warn().Msg("Skipping initrd generation stage")
		return []schema.Stage{}, nil
	}

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

		dracutCmd := fmt.Sprintf("dracut -f /boot/initrd %s", kernel)

		if logger.GetLevel() == 0 {
			dracutCmd = fmt.Sprintf("dracut -v -f /boot/initrd %s", kernel)
		}

		stage = append(stage, []schema.Stage{
			{
				Name:     "Create new initrd",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|SLES.*|[O-o]penSUSE.*|SUSE.*|Hadron.*",
				Commands: []string{
					fmt.Sprintf("depmod -a %s", kernel),
					dracutCmd,
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
func GetKairosReleaseStage(sis values.System, log logger.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.KairosReleaseStep) {
		log.Logger.Warn().Msg("Skipping /etc/kairos-release generation stage")
		return []schema.Stage{}
	}
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
		"KAIROS_ID":             "kairos", // What for?
		"KAIROS_ID_LIKE":        idLike,   // What for?
		"KAIROS_NAME":           idLike,   // What for? Same as ID_LIKE
		"KAIROS_VERSION":        release,
		"KAIROS_ARCH":           sis.Arch.String(),
		"KAIROS_TARGETARCH":     sis.Arch.String(), // What for? Same as ARCH
		"KAIROS_FLAVOR":         flavor,            // This should be in os-release as ID
		"KAIROS_FLAVOR_RELEASE": flavorRelease,     // This should be in os-release as VERSION_ID
		"KAIROS_FAMILY":         sis.Family.String(),
		"KAIROS_MODEL":          config.DefaultConfig.Model,            // NEEDED or it breaks boot!
		"KAIROS_VARIANT":        config.DefaultConfig.Variant.String(), // TODO: Fully drop variant
		"KAIROS_BUG_REPORT_URL": "https://github.com/kairos-io/kairos/issues",
		"KAIROS_HOME_URL":       "https://github.com/kairos-io/kairos",
		"KAIROS_RELEASE":        release,
		"KAIROS_FIPS":           fmt.Sprintf("%t", config.DefaultConfig.Fips),        // Was the image built with FIPS support?
		"KAIROS_TRUSTED_BOOT":   fmt.Sprintf("%t", config.DefaultConfig.TrustedBoot), // Was the image built with Trusted Boot support?
		"KAIROS_INIT_VERSION":   values.GetVersion(),                                 // The version of the kairos-init binary
	}

	// Todo: Change this to allow getting info from several providers? No idea how to store it, the current below is only for k3s/k0s I think
	versionInfo, err := getProviderInfo(log)
	if err == nil && versionInfo.Provider != "" && versionInfo.Version != "" {
		env["KAIROS_SOFTWARE_VERSION"] = versionInfo.Version
		env["KAIROS_SOFTWARE_VERSION_PREFIX"] = versionInfo.Provider
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

func getProviderInfo(logger logger.KairosLogger) (bus.ProviderInstalledVersionPayload, error) {
	logger.Logger.Info().Msg("Triggering provider info event")
	versionInfo := bus.ProviderInstalledVersionPayload{}
	manager := bus.NewBus(bus.InitProviderInfo)
	manager.Initialize(bus.WithLogger(&logger))

	if len(manager.Plugins) == 0 {
		logger.Logger.Info().Msg("No plugins found, skipping provider install event")
		return versionInfo, nil
	}

	manager.Response(bus.InitProviderInfo, func(p *pluggable.Plugin, resp *pluggable.EventResponse) {
		logger.Logger.Debug().Str("at", p.Executable).Interface("resp", resp).Msg("Received info event from provider")
		if resp.Errored() {
			logger.Logger.Error().Msgf("Provider info event failed: %s", resp.Error)
			return
		}
		if resp.State == bus.EventResponseNotApplicable {
			logger.Logger.Info().Msg("Provider info event is non-applicable, skipping")
			return
		}
		if err := json.Unmarshal([]byte(resp.Data), &versionInfo); err != nil {
			logger.Logger.Error().Msgf("Failed to unmarshal provider info event: %s", err)
			return
		}
		if resp.State == bus.EventResponseSuccess {
			logger.Logger.Info().Msg("Provider info event succeeded")
		}
	})
	_, err := manager.Publish(bus.InitProviderInfo, nil)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to publish provider info event: %s", err)
		return versionInfo, err
	}
	return versionInfo, nil
}

// GetWorkaroundsStage Returns the workarounds stage
// It applies some workarounds to the system to fix up inconsistent things or issues on the system
// For ubuntu + trusted boot we need to download the linux-modules-extra package, save the nvdimm modules
// and then clean it up so http uki boot works out of the box. By default the nvdimm modules needed are in that package
// We could just install the package but its a 100+MB  package and we need just 4 or 5 modules.
func GetWorkaroundsStage(_ values.System, l logger.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.WorkaroundsStep) {
		l.Logger.Warn().Msg("Skipping workarounds stage")
		return []schema.Stage{}
	}
	stages := []schema.Stage{
		{
			Name: "Link grub-editenv to grub2-editenv",
			//OnlyIfOs: "Ubuntu.*|Alpine.*", // Maybe not needed and just checking if the file exists is enough
			// test if the file exists and if the link does not exist
			If: "test -f /usr/bin/grub-editenv && ! test -e /usr/bin/grub2-editenv",
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
		{
			Name:     "Create snap dir in rootfs", // Very special as its on teh rootfs so we need to create it now just in case
			OnlyIfOs: "Ubuntu.*|Debian.*",
			Directories: []schema.Directory{
				{
					Path:        "/snap",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
				},
			},
		},
	}

	if config.DefaultConfig.TrustedBoot {
		// This looks like its out of its place as we would expect this modules to be in the initrd but this is for Trusted Boot
		// so the initrd is creating during artifact build and contains the rootfs, so this is ok to be in here
		kernel, err := getLatestKernel(l)
		if err != nil {
			l.Logger.Error().Msgf("Failed to get the latest kernel: %s", err)
			return stages
		}
		// 25.10 is the first version where this workaround is not needed
		stages = append(stages, []schema.Stage{
			{
				Name:            "Download linux-modules-extra for nvdimm modules",
				OnlyIfOs:        "Ubuntu.*",
				OnlyIfOsVersion: `2[0-4]\..*`,
				Commands: []string{
					fmt.Sprintf("apt-get download linux-modules-extra-%s", kernel),
					fmt.Sprintf("dpkg-deb -x linux-modules-extra-%s_*.deb /tmp/modules", kernel),
					fmt.Sprintf("mkdir -p /usr/lib/modules/%s/kernel/drivers/nvdimm", kernel),
					fmt.Sprintf("mv /tmp/modules/lib/modules/%[1]s/kernel/drivers/nvdimm/* /usr/lib/modules/%[1]s/kernel/drivers/nvdimm/", kernel),
					fmt.Sprintf("depmod -a %s", kernel),
					"rm -rf /tmp/modules",
					"rm /*.deb",
				},
			},
		}...)
	}

	return stages
}

// GetCleanupStage Returns the cleanup stage
// This stage is mainly about cleaning up the system and removing unneeded packages
// As some of the software installed can mess with he system and we dont want to have it in an inconsistent state
// I also removes some packages that are no longer needed, like dracut and dependant packages as once
// we have build the initramfs we dont need them anymore
// TODO: Remove package cache for all distros
func GetCleanupStage(_ values.System, l logger.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.CleanupStep) {
		l.Logger.Warn().Msg("Skipping cleanup stage")
		return []schema.Stage{}
	}

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
		{
			Name: "Remove host ssh keys",
			If:   "test -d /etc/ssh",
			Commands: []string{
				"rm -f /etc/ssh/ssh_host_*_key*",
			},
		},
		{
			Name:     "Cleanup",
			OnlyIfOs: "Ubuntu.*|Debian.*",
			Commands: []string{
				"apt-get clean",
				"rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*",
			},
		},
		{
			Name:     "Cleanup",
			OnlyIfOs: "Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*",
			Commands: []string{
				"dnf clean all",
				"rm -rf /var/cache/dnf/* /tmp/* /var/tmp/*",
			},
		},
		{
			Name:     "Cleanup",
			OnlyIfOs: "Alpine.*",
			Commands: []string{
				"rm -rf /var/cache/apk/* /tmp/* /var/tmp/*",
			},
		},
		{
			Name:     "Cleanup",
			OnlyIfOs: "openSUSE.*|SUSE.*",
			Commands: []string{
				"zypper clean -a",
				"rm -rf /var/cache/zypp/* /tmp/* /var/tmp/*",
			},
		},
	}

	return stages
}

// GetServicesStage Returns the services stage
// This stage is about configuring the services to be run on the system. Either enabling or disabling them.
func GetServicesStage(_ values.System, l logger.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.ServicesStep) {
		l.Logger.Warn().Msg("Skipping services stage")
		return []schema.Stage{}
	}
	return []schema.Stage{
		{
			Name:                 "Configure default systemd services",
			OnlyIfServiceManager: "systemd",
			Systemctl: schema.Systemctl{
				Mask: []string{
					"systemd-firstboot.service",
				},
				Overrides: []schema.SystemctlOverride{
					{
						Service: "systemd-networkd-wait-online",
						Content: bundled.SystemdNetworkOnlineWaitOverride,
					},
				},
			},
		},
		{
			Name:                 "Enable fail2ban service for RHEL family",
			OnlyIfServiceManager: "systemd",
			If:                   "test -f /usr/bin/fail2ban-server",
			OnlyIfOs:             "CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"fail2ban",
				},
			},
		},
		{
			Name:                 "Enable fail2ban service",
			OnlyIfServiceManager: "systemd",
			OnlyIfOs:             "Ubuntu.*|Debian.*|SLES.*|openSUSE.*|Fedora.*", // RHEL family has it optinally installed
			Systemctl: schema.Systemctl{
				Enable: []string{
					"fail2ban",
				},
			},
		},
		{
			Name:                 "Enable timesyncd service",
			OnlyIfServiceManager: "systemd",
			OnlyIfOs:             "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|SLES.*|[O-o]penSUSE.*|Hadron.*", // RHEL family has it optinally installed
			Systemctl: schema.Systemctl{
				Enable: []string{
					"systemd-timesyncd",
				},
			},
		},
		{
			Name:                 "Enable services for Debian family",
			OnlyIfOs:             "Ubuntu.*|Debian.*",
			OnlyIfServiceManager: "systemd",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"ssh",
					"systemd-networkd",
				},
			},
		},
		{
			Name:                 "Disable Wicked for SUSE family", // Collides with systemd-networkd
			OnlyIfOs:             "SLES.*|openSUSE.*|SUSE.*",
			OnlyIfServiceManager: "systemd",
			Systemctl: schema.Systemctl{
				Disable: []string{
					"wicked",
				},
				Mask: []string{
					"wicked",
				},
			},
		},
		{
			Name:                 "Enable services for SUSE family",
			OnlyIfOs:             "SLES.*|openSUSE.*|SUSE.*",
			OnlyIfServiceManager: "systemd",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"sshd",
					"systemd-networkd",
					"systemd-resolved",
				},
			},
		},
		{
			Name:                 "Enable services for RHEL family",
			OnlyIfOs:             "Fedora.*|CentOS.*|Rocky.*|AlmaLinux.*",
			OnlyIfServiceManager: "systemd",
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
			Commands: []string{
				"systemctl unmask getty.target",   // Unmask getty.target to allow login on ttys as it comes masked by default
				"systemctl unmask systemd-udevd",  // Unmask systemd-udevd as it comes masked by default
				"systemctl unmask systemd-logind", // Unmask systemd-logind as it comes masked by default
			},
		},
		{
			Name:                 "Enable services for RHEL",
			OnlyIfOs:             "Red\\sHat.*",
			OnlyIfServiceManager: "systemd",
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
			Commands: []string{
				"systemctl unmask getty.target",   // Unmask getty.target to allow login on ttys as it comes masked by default
				"systemctl unmask systemd-udevd",  // Unmask systemd-udevd as it comes masked by default
				"systemctl unmask systemd-logind", // Unmask systemd-logind as it comes masked by default
			},
		},
		{
			Name:                 "Enable networkd for RHEL family if binary is available",
			OnlyIfOs:             "Fedora.*|CentOS.*|Rocky.*|AlmaLinux.*|Red\\sHat.*",
			OnlyIfServiceManager: "systemd",
			If:                   "test -f /usr/lib/systemd/systemd-networkd",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"systemd-networkd",
				},
			},
		},
		{
			Name:                 "Enable NetworkManager for RHEL if binary is available",
			OnlyIfOs:             "Fedora.*|CentOS.*|Rocky.*|AlmaLinux.*|Red\\sHat.*",
			OnlyIfServiceManager: "systemd",
			If:                   "test -f /usr/sbin/NetworkManager",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"NetworkManager",
				},
			},
		},
		{
			Name:                 "Enable services for Alpine family",
			OnlyIfOs:             "Alpine.*",
			OnlyIfServiceManager: "openrc",
			Commands: []string{
				"rc-update add sshd boot",
				"rc-update add fail2ban boot",
				"rc-update add connman boot",
				"rc-update add acpid boot",
				"rc-update add hwclock boot",
				"rc-update add syslog boot",
				"rc-update add udev sysinit",
				"rc-update add udev-trigger sysinit",
				"rc-update add cgroups sysinit",
				"rc-update add ntpd boot",
				"rc-update add crond",
			},
		},
		{
			Name:                 "Enable services for Hadron",
			OnlyIfOs:             "Hadron.*",
			OnlyIfServiceManager: "systemd",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"sshd",
					"systemd-networkd",
					"systemd-resolved",
				},
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
func GetKernelStage(_ values.System, logger logger.KairosLogger) ([]schema.Stage, error) {
	if config.ContainsSkipStep(values.KernelStep) {
		logger.Logger.Warn().Msg("Skipping kernel stage")
		return []schema.Stage{}, nil
	}
	kernel, err := getLatestKernel(logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the latest kernel: %s", err)
		return []schema.Stage{}, err
	}

	return []schema.Stage{
		{
			Name: "Create dir if not exists",
			If:   "test ! -d /boot",
			Directories: []schema.Directory{
				{
					Path:        "/boot",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
				},
			},
		},
		{
			Name: "Clean current kernel link",
			If:   "test -L /boot/vmlinuz",
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
		{ // On RHEL family, if we don't have grub2 installed, it wont copy the kernel and rename it to the /boot dir, so we need to do it manually
			Name:     "Copy kernel for Trusted Boot",
			OnlyIfOs: "Fedora.*|Red\\sHat.*|Rocky.*|AlmaLinux.*",
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
func getLatestKernel(l logger.KairosLogger) (string, error) {
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
		kernelVersion = versions[len(versions)-1].String()
		if kernelVersion == "" {
			l.Logger.Error().Msgf("Failed to find the latest kernel version")
			return kernelVersion, fmt.Errorf("failed to find the latest kernel")
		}
	}

	return kernelVersion, nil
}

// GetKairosInitramfsFilesStage installs the kairos initramfs files
// This stage is used to install the initramfs files that are needed for the system to boot
func GetKairosInitramfsFilesStage(sis values.System, l logger.KairosLogger) ([]schema.Stage, error) {
	if config.ContainsSkipStep(values.InitramfsConfigsStep) {
		l.Logger.Warn().Msg("Skipping installing initramfs configs stage")
		return []schema.Stage{}, nil
	}
	var data []schema.Stage
	if config.DefaultConfig.TrustedBoot {
		l.Logger.Info().Msg("Skipping installing initramfs files stage for trusted boot")
		return data, nil
	}

	if sis.Family.String() == "alpine" {
		immucoreFiles, err := bundled.EmbeddedAlpineInit.ReadFile("alpineInit/immucore.files")
		if err != nil {
			l.Logger.Error().Err(err).Str("file", "immucore.files").Msg("Failed to read embedded file")
			return nil, err
		}
		initramfsInit, err := bundled.EmbeddedAlpineInit.ReadFile("alpineInit/initramfs-init")
		if err != nil {
			l.Logger.Error().Err(err).Str("file", "initramfs-init").Msg("Failed to read embedded file")
			return nil, err
		}
		mkinitfsConf, err := bundled.EmbeddedAlpineInit.ReadFile("alpineInit/mkinitfs.conf")
		if err != nil {
			l.Logger.Error().Err(err).Str("file", "mkinitfs.conf").Msg("Failed to read embedded file")
			return nil, err
		}
		tpmModules, err := bundled.EmbeddedAlpineInit.ReadFile("alpineInit/tpm.modules")
		if err != nil {
			l.Logger.Error().Err(err).Str("file", "tpm.modules").Msg("Failed to read embedded file")
			return nil, err
		}

		data = append(data, []schema.Stage{
			{
				Name: "Install reconcile script",
				Files: []schema.File{
					{
						Path:        "/usr/sbin/cos-setup-reconcile",
						Permissions: 0755,
						Owner:       0,
						Group:       0,
						Content:     bundled.ReconcileScript,
					},
				},
			},
			{
				Name: "Install Alpine initrd scripts",
				Files: []schema.File{
					{
						Path:        "/etc/mkinitfs/features.d/immucore.files",
						Permissions: 0644,
						Owner:       0,
						Group:       0,
						Content:     string(immucoreFiles),
					},
					{
						Path:        "/etc/mkinitfs/features.d/tpm.modules",
						Permissions: 0644,
						Owner:       0,
						Group:       0,
						Content:     string(tpmModules),
					},
					{
						Path:        "/etc/mkinitfs/mkinitfs.conf",
						Permissions: 0644,
						Owner:       0,
						Group:       0,
						Content:     string(mkinitfsConf),
					},
					{
						Path:        "/usr/share/mkinitfs/initramfs-init",
						Permissions: 0755,
						Owner:       0,
						Group:       0,
						Content:     string(initramfsInit),
					},
				},
			},
		}...)
	} else {
		// Add proper network and systemd-sysext if needed
		// We default to systemd-networkd+network-legacy and sysext enabled
		// If its ubuntu <= 22.04 we need to disable sysext
		// If its ubuntu <= 20.04 we need to use the plain network module
		// network-legacy is needed for ipxe as it comes up very fast which makes the livenet stuff work properly
		// otherwise systemd-networkd does not trigger the dracut hooks to let it know that its up and running
		// https://github.com/dracutdevs/dracut/issues/1822
		networkModule := "systemd-networkd network-legacy"
		sysextModule := true

		if sis.Distro == values.Ubuntu {
			ver, err := semver.NewVersion(sis.Version)
			if err != nil {
				l.Logger.Error().Msgf("Failed to parse the version %s: %s", sis.Version, err)
				return []schema.Stage{}, err
			}
			constraint, _ := semver.NewConstraint("<=22.04")
			// If its <= 22.04 we need to use the plain network module and disable sysext
			if constraint.Check(ver) {
				l.Logger.Debug().Str("distro", string(sis.Distro)).Str("version", sis.Version).Msg("Disabling sysext")
				sysextModule = false
				constraint, _ = semver.NewConstraint("<=20.04")
				// If its <= 20.04 we need to use the plain network module
				if constraint.Check(ver) {
					l.Logger.Debug().Str("distro", string(sis.Distro)).Str("version", sis.Version).Msg("Using the plain network module")
					networkModule = "network"
				}
			}
		}

		if sis.Family == values.RedHatFamily {
			// Check sysext first
			ver, err := semver.NewVersion(sis.Version)
			if err != nil {
				l.Logger.Error().Msgf("Failed to parse the version %s: %s", sis.Version, err)
				return []schema.Stage{}, err
			}
			constraint, _ := semver.NewConstraint("<9.0")
			// If its < 9.0 we need to disable sysext
			if constraint.Check(ver) {
				l.Logger.Debug().Str("distro", string(sis.Distro)).Str("version", sis.Version).Msg("Disabling sysext")
				sysextModule = false
			}

			// Now network
			// we default to networkmanager
			// if systemd-network is available we use it instead
			// depending on the version we might add network-legacy
			// Start from scratch
			networkModule = ""

			// Do we have networkmanmager?
			if _, err := os.Stat("/usr/sbin/NetworkManager"); err == nil {
				networkModule = "network-manager"
			}

			// Do we have systemd-networkd?
			if _, err := os.Stat("/usr/lib/systemd/systemd-networkd"); err == nil {
				networkModule = "systemd-networkd"
			}

			constraint, _ = semver.NewConstraint("<10")
			// If its > 9.0 we cant add network-legacy
			if constraint.Check(ver) {
				networkModule += " network-legacy"
			} else {
				networkModule += " network"
			}

		}

		if sis.Distro == values.Hadron {
			networkModule = "systemd-networkd "
		}

		l.Logger.Debug().Str("networkModule", networkModule).Bool("sysextModule", sysextModule).Msg("Adding dracut modules to initramfs")

		// Add support for pmem modules to support HTTP EFI boot automatically mounting the served ISO as a livecd
		// This means the UEFI firmware will expose the loaded HTTP Iso memory as a block device for the kernel
		// to find it and mount it as if it was a regular disk
		// Then dracut will find the label and mount it in the proper places
		// Add the dmsquash-live module to the initramfs so we can use it
		// Add network module to the initramfs so we can use it
		// Add immucore module to the initramfs so we can use it
		data = append(data, []schema.Stage{
			{
				Name:     "Add pmem modules to initramfs",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*",
				Files: []schema.File{
					{
						Path:        bundled.DracutPmemPath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     bundled.DracutPmemConfig,
					},
				},
			},
			{
				Name:     "Add sysext module to initramfs",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*[O-o]penSUSE.*|Hadron.*",
				If:       strconv.FormatBool(sysextModule),
				Files: []schema.File{
					{
						Path:        bundled.DracutSysextPath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     bundled.DracutSysextConfig,
					},
				},
			},
			{
				Name:     "Add network module to initramfs",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*|[O-o]penSUSE.*|Hadron.*",
				Files: []schema.File{
					{
						Path:        bundled.DracutNetworkPath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     fmt.Sprintf(bundled.DracutNetworkConfig, networkModule),
					},
				},
			},
			{
				Name:     "Add immucore module to initramfs",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*|[O-o]penSUSE.*|Hadron.*",
				Files: []schema.File{
					{
						Path:        bundled.DracutConfigPath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     bundled.ImmucoreConfigDracut,
					},
					{
						Path:        bundled.DracutImmucoreModuleSetupPath,
						Owner:       0,
						Group:       0,
						Permissions: 0755,
						Content:     bundled.ImmucoreModuleSetupDracut,
					},
					{
						Path:        bundled.DracutImmucoreGeneratorPath,
						Owner:       0,
						Group:       0,
						Permissions: 0755,
						Content:     bundled.ImmucoreGeneratorDracut,
					},
					{
						Path:        bundled.DracutImmucoreServicePath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     bundled.ImmucoreServiceDracut,
					},
				},
			},
			// Ubuntu 20.04 does not support the dracut multipath module
			// therefore we don't support multipath for Ubuntu 20.04 and below
			{
				Name:     "Add Multipath module to initramfs for Ubuntu 21.04 and above",
				OnlyIfOs: "Ubuntu.*",
				// Skips the multipath module for Ubuntu 20.04 and below
				// This uses a regex expression and Go does not support lookaheads
				// so we have to hackily use a or condition
				OnlyIfOsVersion: bundled.UbuntuSupportedMultipathVersions,
				Files: []schema.File{
					{
						Path:        bundled.DracutMultipathPath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     bundled.DracutMultipathConfig,
					},
				},
			},
			{
				Name:     "Add Multipath module to initramfs",
				OnlyIfOs: "Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*|[O-o]penSUSE.*|Hadron.*",
				Files: []schema.File{
					{
						Path:        bundled.DracutMultipathPath,
						Owner:       0,
						Group:       0,
						Permissions: 0644,
						Content:     bundled.DracutMultipathConfig,
					},
				},
			},
		}...)

		if config.DefaultConfig.Fips {
			// Add dracut fips support
			data = append(data, []schema.Stage{
				{
					Name:     "Add fips support to initramfs",
					OnlyIfOs: "Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*|[O-o]penSUSE.*",
					Files: []schema.File{
						{
							Path:        bundled.DracutFipsPath,
							Owner:       0,
							Group:       0,
							Permissions: 0644,
							Content:     bundled.DracutFipsConfig,
						},
					},
				},
			}...)
		}

	}

	return data, nil
}
