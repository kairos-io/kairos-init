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
	"github.com/kairos-io/kairos-init/pkg/system"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/console"
	"github.com/mudler/yip/pkg/executor"
	"github.com/mudler/yip/pkg/schema"
	"github.com/twpayne/go-vfs/v5"
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
		ARTIFACT is just the IMAGE_LABEL with the OS and OS VERSION in front, useless?
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

	// "24.04-standard-amd64-generic-v3.2.4-36-g24ca209-k3sv1.32.0-k3s1"
	// We are not doing the k3s software version here
	imageLabel := fmt.Sprintf("%s-%s-%s-%s-%s", flavorRelease, config.DefaultConfig.Variant, sis.Arch.String(), config.DefaultConfig.Model, release)

	env := map[string]string{
		"KAIROS_ID":                "kairos", // What for?
		"KAIROS_ID_LIKE":           idLike,   // What for?
		"KAIROS_NAME":              idLike,   // What for? Same as ID_LIKE
		"KAIROS_VERSION":           release,
		"KAIROS_ARCH":              sis.Arch.String(),
		"KAIROS_TARGETARCH":        sis.Arch.String(), // What for? Same as ARCH
		"KAIROS_FLAVOR":            flavor,
		"KAIROS_FLAVOR_RELEASE":    flavorRelease,
		"KAIROS_FAMILY":            sis.Family.String(),
		"KAIROS_MODEL":             config.DefaultConfig.Model, // NEEDED or it breaks boot!
		"KAIROS_VARIANT":           config.DefaultConfig.Variant.String(),
		"KAIROS_REGISTRY_AND_ORG":  config.DefaultConfig.Registry, // Needed for upgrades to search for images
		"KAIROS_BUG_REPORT_URL":    "https://github.com/kairos-io/kairos/issues",
		"KAIROS_HOME_URL":          "https://github.com/kairos-io/kairos",
		"KAIROS_RELEASE":           release,
		"KAIROS_IMAGE_LABEL":       imageLabel,                                   // Used by raw image creation...very bad
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
			re := regexp.MustCompile(`k3s version (v\d+\.\d+\.\d+\+k3s\d+)`)
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

func GetInstallStage(sis values.System, logger types.KairosLogger) ([]schema.Stage, error) {
	// Fips + ubuntu fails early and redirect to our Example
	if sis.Distro == values.Ubuntu && config.DefaultConfig.Fips {
		return nil, fmt.Errorf("FIPS is not supported on Ubuntu without a PRO account and extra packages.\n" +
			"See https://github.com/kairos-io/kairos/blob/master/examples/builds/ubuntu-fips/Dockerfile for an example on how to build it")
	}

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

	// For trusted boot we need to select the correct kernel packages manually
	// TODO: Have a flag in the config to add the full linux-firmware package?
	if config.DefaultConfig.TrustedBoot {
		// TODO: Check for other distros/families
		if sis.Distro == values.Ubuntu {
			// First update the package list so we can search for the kernel packages properly
			err = exec.Command("apt-get", "update").Run()
			if err != nil {
				logger.Logger.Error().Msgf("Failed to update the package list: %s", err)
				return []schema.Stage{}, err
			}

			out, err := exec.Command("apt-cache", "search", "linux-image").CombinedOutput()
			if err != nil {
				logger.Logger.Error().Msgf("Failed to get the kernel packages: %s", err)
				return []schema.Stage{}, err
			}
			// Get the latest kernel image and modules version
			// package is in format linux-image-5.4.0-104-generic
			// modules are in format linux-modules-5.4.0-104-generic
			// we need to extract the number only
			re, _ := regexp.Compile(`linux-image-(\d+\.\d+\.\d+-\d+)-generic`)
			if re.Match(out) {
				match := re.FindStringSubmatch(string(out))
				logger.Logger.Debug().Str("kernel", match[1]).Msg("Found the kernel package")
				finalMergedPkgs = append(finalMergedPkgs, fmt.Sprintf("linux-image-%s-generic", match[1]))
				finalMergedPkgs = append(finalMergedPkgs, fmt.Sprintf("linux-modules-%s-generic", match[1]))
			} else {
				logger.Logger.Error().Err(err).Msgf("Failed to get the kernel packages")
				logger.Logger.Debug().Str("output", string(out)).Msgf("Failed to get the kernel packages")
				return []schema.Stage{}, err
			}
		}
	}

	// TODO(rhel): Add zfs packages? Currently we add the repos to alma+rocky but we don't install the packages so?
	stage := []schema.Stage{
		{
			Name:     "Install epel-release",
			OnlyIfOs: "CentOS.*|RedHat.*|Rocky.*|AlmaLinux.*",
			Packages: schema.Packages{
				Install: []string{
					"epel-release",
				},
			},
		},
		{
			Name: "Install base packages",
			Packages: schema.Packages{
				Install: finalMergedPkgs,
				Refresh: true,
				Upgrade: true,
			},
		},
	}
	return stage, nil
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
		{
			Name: "Remove host ssh keys",
			If:   "test -d /etc/ssh",
			Commands: []string{
				"rm -f /etc/ssh/ssh_host_*_key*",
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

func GetInstallFrameworkStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	framework := config.DefaultConfig.FrameworkVersion
	if config.DefaultConfig.Fips {
		framework = fmt.Sprintf("%s-fips", framework)
	}
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
					Source: fmt.Sprintf("quay.io/kairos/framework:%s", framework),
					Target: "/",
				},
			},
		},
	}
}

// GetInstallProviderAndKubernetes will install the provider and kubernetes packages
func GetInstallProviderAndKubernetes(sis values.System, _ types.KairosLogger) []schema.Stage {
	var data []schema.Stage

	// If its core we dont do anything here
	if config.DefaultConfig.Variant.String() == "core" {
		return data
	}

	switch config.DefaultConfig.KubernetesProvider {
	case config.K3sProvider:
		cmd := "INSTALL_K3S_BIN_DIR=/usr/bin INSTALL_K3S_SKIP_ENABLE=true INSTALL_K3S_SKIP_SELINUX_RPM=true"
		// Append version if any, otherwise default to latest
		if config.DefaultConfig.KubernetesVersion != "" {
			cmd = fmt.Sprintf("INSTALL_K3S_VERSION=%s %s", config.DefaultConfig.KubernetesVersion, cmd)
		}
		data = append(data, []schema.Stage{
			{
				Name: "Install Kubernetes packages",
				Commands: []string{
					"curl -sfL https://get.k3s.io > installer.sh",
					"chmod +x installer.sh",
					fmt.Sprintf("%s sh installer.sh", cmd),
					fmt.Sprintf("%s sh installer.sh agent", cmd),
				},
			},
		}...)
	case config.K0sProvider:
		cmd := "sh installer.sh"
		// Append version if any, otherwise default to latest
		if config.DefaultConfig.KubernetesVersion != "" {
			cmd = fmt.Sprintf("K0S_VERSION=%s %s", config.DefaultConfig.KubernetesVersion, cmd)
		}
		data = append(data, []schema.Stage{
			{
				Name: "Install Kubernetes packages",
				Commands: []string{
					"curl -sfL https://get.k0s.sh > installer.sh",
					"chmod +x installer.sh",
					cmd,
					"rm installer.sh",
					"mv /usr/local/bin/k0s /usr/bin/k0s",
				},
			},
			{
				Name: "Create k0s services for systemd",
				If:   `[ -e "/sbin/systemctl" ] || [ -e "/usr/bin/systemctl" ] || [ -e "/usr/sbin/systemctl" ] || [ -e "/usr/bin/systemctl" ]`,
				Files: []schema.File{
					{
						Path:        "/etc/systemd/system/k0scontroller.service",
						Permissions: 0644,
						Owner:       0,
						Group:       0,
						Content: `[Unit]
Description=k0s - Zero Friction Kubernetes
Documentation=https://docs.k0sproject.io
ConditionFileIsExecutable=/usr/bin/k0s

After=network-online.target 
Wants=network-online.target 

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/bin/k0s controller

RestartSec=10
Delegate=yes
KillMode=process
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
LimitNOFILE=999999
Restart=always

[Install]
WantedBy=multi-user.target`,
					},
					{
						Path:        "/etc/systemd/system/k0sworker.service",
						Permissions: 0644,
						Owner:       0,
						Group:       0,
						Content: `[Unit]
Description=k0s - Zero Friction Kubernetes
Documentation=https://docs.k0sproject.io
ConditionFileIsExecutable=/usr/bin/k0s

After=network-online.target 
Wants=network-online.target 

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/bin/k0s worker

RestartSec=10
Delegate=yes
KillMode=process
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
LimitNOFILE=999999
Restart=always

[Install]
WantedBy=multi-user.target`,
					},
				},
			},
			{
				Name: "Create k0s services for openrc",
				If:   `[ -f "/sbin/openrc" ]`,
				Files: []schema.File{
					{
						Path:        "/etc/init.d/k0scontroller",
						Permissions: 0755,
						Owner:       0,
						Group:       0,
						Content: `#!/sbin/openrc-run
supervisor=supervise-daemon
description="k0s - Zero Friction Kubernetes"
command=/usr/bin/k0s
command_args="'controller' "
name=$(basename $(readlink -f $command))
supervise_daemon_args="--stdout /var/log/${name}.log --stderr /var/log/${name}.err"

: "${rc_ulimit=-n 1048576 -u unlimited}"
depend() { 
	need cgroups 
	need net 
	use dns 
	after firewall
}`,
					},
					{
						Path:        "/etc/init.d/k0sworker",
						Permissions: 0755,
						Owner:       0,
						Group:       0,
						Content: `#!/sbin/openrc-run
supervisor=supervise-daemon
description="k0s - Zero Friction Kubernetes"
command=/usr/bin/k0s
command_args="'worker' "
name=$(basename $(readlink -f $command))
supervise_daemon_args="--stdout /var/log/${name}.log --stderr /var/log/${name}.err"

: "${rc_ulimit=-n 1048576 -u unlimited}"
depend() { 
	need cgroups 
	need net 
	use dns 
	after firewall
}`,
					},
				},
			},
		}...)
	}

	// Install provider + k8s utils
	data = append(data, []schema.Stage{
		{
			Name: "Install Provider packages",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: values.GetProviderPackage(sis.Arch.String()),
					Target: "/",
				},
			},
		},
		{
			Name: "Install Edgevpn packages",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: values.GetEdgeVPNPackage(sis.Arch.String()),
					Target: "/",
				},
			},
		},
		{
			Name: "Install K9s packages",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: values.GetK9sPackage(sis.Arch.String()),
					Target: "/",
				},
			},
		},
		{
			Name: "Install Nerdctl packages",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: values.GetNerdctlPackage(sis.Arch.String()),
					Target: "/",
				},
			},
		},
		{
			Name: "Install Kube-vip packages",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: values.GetKubeVipPackage(sis.Arch.String()),
					Target: "/",
				},
			},
		},
	}...)

	return data
}

func GetServicesStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
		{
			Name:     "Enable services for Debian family",
			OnlyIfOs: "Ubuntu.*|Debian.*",
			Systemctl: schema.Systemctl{
				Enable: []string{
					"ssh",
					"systemd-networkd",
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
					"systemd-networkd",
				},
				Disable: []string{
					"dnf-makecache",
					"dnf-makecache.timer",
				},
			},
			Commands: []string{
				"systemctl unmask getty.target", // Unmask getty.target to allow login on ttys as it comes masked by default
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

// RunAllStages Runs all the stages in the correct order
func RunAllStages(logger types.KairosLogger) (schema.YipConfig, error) {
	fullYipConfig := schema.YipConfig{Stages: map[string][]schema.Stage{}}
	installStage, err := RunInstallStage(logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to run the install stage: %s", err)
		return installStage, err
	}

	// Add all stages to the full yip config
	for stageName, stages := range installStage.Stages {
		fullYipConfig.Stages[stageName] = append(fullYipConfig.Stages[stageName], stages...)
	}

	initStage, err := RunInitStage(logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to run the init stage: %s", err)
		return fullYipConfig, err
	}

	// Add all stages to the full yip config
	for stageName, stages := range initStage.Stages {
		fullYipConfig.Stages[stageName] = append(fullYipConfig.Stages[stageName], stages...)
	}

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

	// On Rpi3 and Rpi4 we need to enable the non-free repository for Debian to get the firmware
	if config.DefaultConfig.Model == values.Rpi3.String() || config.DefaultConfig.Model == values.Rpi4.String() {
		data.Stages["before-install"] = append(data.Stages["before-install"], []schema.Stage{
			{
				Name:     "Enable non-free repository",
				OnlyIfOs: "Debian.*",
				Commands: []string{
					"sed -i 's/^Components: main.*$/& non-free-firmware/' /etc/apt/sources.list.d/debian.sources",
				},
			},
		}...)
	}
	// Add extensions from disk
	data.Stages["before-install"] = append(data.Stages["before-install"], GetStageExtensions("before-install", logger)...)

	// Add packages install
	installStage, err := GetInstallStage(sis, logger)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to get the install stage: %s", err)
		return data, err
	}
	data.Stages["install"] = installStage
	// Add the framework stage
	data.Stages["install"] = append(data.Stages["install"], GetInstallFrameworkStage(sis, logger)...)
	data.Stages["install"] = append(data.Stages["install"], GetInstallProviderAndKubernetes(sis, logger)...)

	// Add extensions from disk
	data.Stages["install"] = append(data.Stages["install"], GetStageExtensions("install", logger)...)

	// Run things after we install packages and framework
	data.Stages["after-install"] = []schema.Stage{}

	// Add extensions from disk
	data.Stages["after-install"] = append(data.Stages["after-install"], GetStageExtensions("after-install", logger)...)

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

	// Add extensions from disk
	data.Stages["before-init"] = append(data.Stages["before-init"], GetStageExtensions("before-init", logger)...)

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

	// Add extensions from disk
	data.Stages["init"] = append(data.Stages["init"], GetStageExtensions("init", logger)...)

	// Run things after we init the system
	data.Stages["after-init"] = []schema.Stage{}

	// Add extensions from disk
	data.Stages["after-init"] = append(data.Stages["after-init"], GetStageExtensions("after-init", logger)...)

	for _, st := range []string{"before-init", "init", "after-init"} {
		err = initExecutor.Run(st, vfs.OSFS, yipConsole, data.ToString())
		if err != nil {
			logger.Logger.Error().Msgf("Failed to run the %s stage: %s", st, err)
			return data, err
		}
	}

	return data, nil
}
