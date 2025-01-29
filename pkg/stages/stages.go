package stages

import (
	"fmt"
	semver "github.com/hashicorp/go-version"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/system"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/console"
	"github.com/mudler/yip/pkg/executor"
	"github.com/mudler/yip/pkg/schema"
	"github.com/twpayne/go-vfs/v5"
	"os"
	"sort"
	"strings"
)

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
				l.Logger.Debug().Err(err).Msgf("Failed to parse the version %s as semver", dir.Name())
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

func GetKairosReleaseStage(sis values.System, log types.KairosLogger) []schema.Stage {
	// TODO: Expand tis as this doesnt cover all the current fields
	// Current missing fields
	/*
			KAIROS_VERSION_ID="v3.2.4-36-g24ca209-v1.32.0-k3s1"
			KAIROS_REGISTRY_AND_ORG="quay.io/kairos"
			KAIROS_RELEASE="v3.2.4-36-g24ca209"
			KAIROS_IMAGE_LABEL="24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0-k3s1"
			KAIROS_GITHUB_REPO="kairos-io/kairos"
			KAIROS_SOFTWARE_VERSION_PREFIX="k3s"
			KAIROS_IMAGE_REPO="quay.io/kairos/ubuntu:24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0-k3s1"
			KAIROS_ARTIFACT="kairos-ubuntu-24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0+k3s1"
			KAIROS_SOFTWARE_VERSION="v1.32.0+k3s1"
			KAIROS_VERSION="v3.2.4-36-g24ca209-v1.32.0-k3s1"
			KAIROS_PRETTY_NAME="kairos-standard-ubuntu-24.04 v3.2.4-36-g24ca209-v1.32.0-k3s1"

		VERSION_ID and VERSION are the same
		RELEASE is the short version of VERSION and VERSION_ID, the version without the k3s version

		IMAGE_REPO is a mix of REGISTRY_AND_ORG and IMAGE_LABEL, useless?
		ARTIFACT is just the IMAGE_LABEL with the OS and OS VERSION in front, useless?
		IMAGE_LABEL is again a mix of all the others fields, useless ?

		IMHO, important fields here are:
		- RELEASE: Shows the version of KAIROS, wel already have this under VERSION field? Maybe we need to duplicate it, urgh
		- SOFTWARE_VERSION: Shows the version of the software (k3s for example)
		- REGISTRY_AND_ORG: Shows the registry for the image, useful for upgrades

		Thats it, the rest I would drop it. The rest is just a mix of the other fields and not really useful,
		if we have the original needed fields we can recreate the rest of the fields if needed so....
	*/

	idLike := fmt.Sprintf("kairos-%s-%s-%s", config.DefaultConfig.Variant, sis.Distro.String(), sis.Version)
	flavor := sis.Distro.String()
	flavorRelease := sis.Version

	// TODO: Check if this affects sles versions? I dont think so as they are set like registry.suse.com/bci/bci-micro:15.6
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
	// "24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0-k3s1"
	// We are not doing the k3s software version here
	imageLabel := fmt.Sprintf("%s-%s-%s-%s-%s", flavorRelease, config.DefaultConfig.Variant, sis.Arch.String(), config.DefaultConfig.Model, config.DefaultConfig.FrameworkVersion)

	return []schema.Stage{
		{
			Name: "Write kairos-release",
			Environment: map[string]string{
				"KAIROS_ID":               "kairos",                              // What for?
				"KAIROS_ID_LIKE":          idLike,                                // What for?
				"KAIROS_NAME":             idLike,                                // What for? Same as ID_LIKE
				"KAIROS_VERSION":          config.DefaultConfig.FrameworkVersion, // Move to use the framework version, bump framework to be in sync with Kairos
				"KAIROS_ARCH":             sis.Arch.String(),
				"KAIROS_TARGETARCH":       sis.Arch.String(), // What for? Same as ARCH
				"KAIROS_FLAVOR":           flavor,
				"KAIROS_FLAVOR_RELEASE":   flavorRelease,
				"KAIROS_FAMILY":           sis.Family.String(),
				"KAIROS_MODEL":            config.DefaultConfig.Model, // NEEDED or it breaks boot!
				"KAIROS_VARIANT":          config.DefaultConfig.Variant,
				"KAIROS_REGISTRY_AND_ORG": config.DefaultConfig.Registry, // Needed for upgrades to search for images
				"KAIROS_BUG_REPORT_URL":   "https://github.com/kairos-io/kairos/issues",
				"KAIROS_HOME_URL":         "https://github.com/kairos-io/kairos",
				"KAIROS_RELEASE":          config.DefaultConfig.FrameworkVersion, // Move to use the framework version, bump framework to be in sync with Kairos, used by upgrades
				"KAIROS_IMAGE_LABEL":      imageLabel,                            // Used by raw image creation...very bad
			},
			EnvironmentFile: "/etc/kairos-release",
		},
	}
}

func GetInstallStage(sis values.System, logger types.KairosLogger) ([]schema.Stage, error) {
	// Get the packages
	packages, err := values.GetPackages(sis, logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the packages: %s", err)
		return []schema.Stage{}, err
	}
	// Now parse the packages with the templating engine
	finalMergedPkgs, err := values.PackageListToTemplate(packages, values.GetTemplateParams(sis), logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to parse the packages: %s", err)
		return []schema.Stage{}, err
	}
	// TODO(rhel): Add zfs packages? Currently we add the repos to alma+rocky but we don't install the packages so?
	return []schema.Stage{
		{
			Name: "Install base packages",
			Packages: schema.Packages{
				Install: finalMergedPkgs,
				Refresh: true,
				Upgrade: true,
			},
		},
	}, nil
}

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
			Name: "Clean current kernel link",
			If:   "test -f /boot/Image",
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
			Name: "Link kernel",
			If:   fmt.Sprintf("test -f /boot/vmlinuz-%s", kernel),
			Commands: []string{
				fmt.Sprintf("depmod -a %s", kernel),
				fmt.Sprintf("ln -s /boot/vmlinuz-%s /boot/vmlinuz", kernel),
			},
		},
		{
			Name: "Link kernel",
			If:   fmt.Sprintf("test -f /boot/Image-%s", kernel), // On suse arm64 kernel starts with Image
			Commands: []string{
				fmt.Sprintf("depmod -a %s", kernel),
				fmt.Sprintf("ln -s /boot/Image-%s /boot/vmlinuz", kernel),
			},
		},
		{
			Name: "Link kernel for Alpine",
			If:   "test -f /boot/vmlinuz-lts",
			Commands: []string{
				fmt.Sprintf("depmod -a %s", kernel),
				"ln -s /boot/vmlinuz-lts /boot/vmlinuz",
			},
		},
	}, nil
}

func GetInitrdStage(_ values.System, logger types.KairosLogger) ([]schema.Stage, error) {
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

		stage = append(stage, []schema.Stage{
			{
				Name:     "Create new initrd",
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*|SLES.*|[O-o]penSUSE.*",
				Commands: []string{
					fmt.Sprintf("dracut -v -f /boot/initrd %s", kernel),
				},
			},
			{
				Name:     "Create new initrd for Alpine",
				OnlyIfOs: "Alpine.*",
				Commands: []string{
					fmt.Sprintf("mkinitfs -o /boot/initrd %s", kernel),
				},
			},
		}...)
	}

	return stage, nil
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

func GetCleanupStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
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
	}
}

func GetInstallFrameworkStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
		{
			Name: "Create kairos directory",
			If:   "test -d /etc/kairos",
			Directories: []schema.Directory{
				{
					Path:        "/etc/kairos",
					Permissions: 0755,
				},
			},
		},
		{
			Name: "Install framework",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: fmt.Sprintf("quay.io/kairos/framework:%s", config.DefaultConfig.FrameworkVersion),
					Target: "/",
				},
			},
		},
	}
}

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

// GetInstallStandardStage Returns the standard install stage so it installs the standard packages like k3s and provider
// Not used for now
func GetInstallStandardStage(sis values.System, logger types.KairosLogger) []schema.Stage {
	var data []schema.Stage
	/*
		if config.DefaultConfig.Variant == "standard" {
			data = append(data, schema.Stage{
				Name: "Install standard packages",
				Commands: []string{
					"luet install -y k9s-openrc/systemd/k3s",
					"luet install -y provider-kairos",
				},
			})
		}
	*/

	return data
}

// RunAllStages Runs all the stages in the correct order
func RunAllStages(logger types.KairosLogger) (schema.YipConfig, error) {
	fullYipConfig := schema.YipConfig{Stages: map[string][]schema.Stage{}}
	installStage, err := RunInstallStage(logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to run the install stage: %s", err)
		return installStage, err
	}

	fullYipConfig.Stages["install"] = installStage.Stages["install"]
	// Add packages install

	initStage, err := RunInitStage(logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to run the init stage: %s", err)
		return fullYipConfig, err
	}
	fullYipConfig.Stages["init"] = initStage.Stages["init"]

	return fullYipConfig, nil
}

// RunInstallStage Runs the install stage
// This is good if we are doing the init in layers as this will allow us to run the install stage and cache that then run
// the init stage later so we can cache the install stage which is usually the longest
func RunInstallStage(logger types.KairosLogger) (schema.YipConfig, error) {
	sis := system.DetectSystem(logger)
	initExecutor := executor.NewExecutor(executor.WithLogger(logger))
	yipConsole := console.NewStandardConsole(console.WithLogger(logger))

	data := schema.YipConfig{Stages: map[string][]schema.Stage{}}
	// Run things before we install packages and framework
	data.Stages["before-install"] = []schema.Stage{}
	// Add packages install
	installStage, err := GetInstallStage(sis, logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the install stage: %s", err)
		return data, err
	}
	data.Stages["install"] = installStage
	// Add the framework stage
	data.Stages["install"] = append(data.Stages["install"], GetInstallFrameworkStage(sis, logger)...)
	data.Stages["install"] = append(data.Stages["install"], GetInstallStandardStage(sis, logger)...)

	// Run things after we install packages and framework
	data.Stages["after-install"] = []schema.Stage{}

	// Run install first, as kernel and initrd resolution depend on the installed packages
	for _, st := range []string{"before-install", "install", "after-install"} {
		err = initExecutor.Run(st, vfs.OSFS, yipConsole, data.ToString())
		if err != nil {
			logger.Logger.Error().Msgf("Failed to run the %s stage: %s", st, err)
			return data, err
		}
	}
	return data, nil
}

// RunInitStage Runs the init stage
// This is good if we are doing the init in layers as this will allow us to run the install stage and cache that then run
// the init stage later so we can cache the install stage which is usually the longest
func RunInitStage(logger types.KairosLogger) (schema.YipConfig, error) {
	sis := system.DetectSystem(logger)
	initExecutor := executor.NewExecutor(executor.WithLogger(logger))
	yipConsole := console.NewStandardConsole(console.WithLogger(logger))

	data := schema.YipConfig{Stages: map[string][]schema.Stage{}}

	// Run things before we init the system
	data.Stages["before-init"] = []schema.Stage{}

	data.Stages["init"] = []schema.Stage{}
	data.Stages["init"] = append(data.Stages["init"], GetKairosReleaseStage(sis, logger)...)
	kernelStage, err := GetKernelStage(sis, logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the kernel stage: %s", err)
		return data, err
	}
	data.Stages["init"] = append(data.Stages["init"], kernelStage...)
	initrdStage, err := GetInitrdStage(sis, logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the initrd stage: %s", err)
		return data, err
	}
	data.Stages["init"] = append(data.Stages["init"], initrdStage...)
	data.Stages["init"] = append(data.Stages["init"], GetServicesStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetWorkaroundsStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetCleanupStage(sis, logger)...)

	// Run things after we init the system
	data.Stages["after-init"] = []schema.Stage{}

	for _, st := range []string{"before-init", "init", "after-init"} {
		err = initExecutor.Run(st, vfs.OSFS, yipConsole, data.ToString())
		if err != nil {
			logger.Logger.Error().Msgf("Failed to run the %s stage: %s", st, err)
			return data, err
		}
	}

	return data, nil
}
