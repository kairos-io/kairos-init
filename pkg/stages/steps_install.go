package stages

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	semver "github.com/hashicorp/go-version"
	"github.com/kairos-io/kairos-init/pkg/bundled"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
)

// This file contains the stages for the install process

func GetInstallStage(sis values.System, logger types.KairosLogger) ([]schema.Stage, error) {
	if config.ContainsSkipStep(values.InstallPackagesStep) {
		logger.Logger.Warn().Msg("Skipping install packages stage")
		return []schema.Stage{}, nil
	}
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
			OnlyIfOs: "CentOS.*|Rocky.*|AlmaLinux.*",
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

// GetInstallKubernetesStage returns the the kubernetes install stage
func GetInstallKubernetesStage(sis values.System, logger types.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.KubernetesStep) {
		logger.Logger.Warn().Msg("Skipping installing kubernetes stage")
		return []schema.Stage{}
	}
	var stages []schema.Stage

	// If its core we dont do anything here
	if config.DefaultConfig.Variant.String() == "core" {
		return stages
	}

	switch config.DefaultConfig.KubernetesProvider {
	case config.K3sProvider:
		cmd := "INSTALL_K3S_BIN_DIR=/usr/bin INSTALL_K3S_SKIP_ENABLE=true INSTALL_K3S_SKIP_SELINUX_RPM=true"
		// Append version if any, otherwise default to latest
		if config.DefaultConfig.KubernetesVersion != "" {
			cmd = fmt.Sprintf("INSTALL_K3S_VERSION=%s %s", config.DefaultConfig.KubernetesVersion, cmd)
		}
		stages = append(stages, []schema.Stage{
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
		stages = append(stages, []schema.Stage{
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
		}...)

		if sis.Family.String() == "alpine" {
			// Add openrc services
			stages = append(stages, []schema.Stage{
				{
					Name: "Create k0s services for openrc",
					Files: []schema.File{
						{
							Path:        "/etc/init.d/k0scontroller",
							Permissions: 0755,
							Owner:       0,
							Group:       0,
							Content:     bundled.K0sControllerOpenrc,
						},
						{
							Path:        "/etc/init.d/k0sworker",
							Permissions: 0755,
							Owner:       0,
							Group:       0,
							Content:     bundled.K0sWorkerOpenrc,
						},
					},
				},
			}...)
		} else {
			// Add systemd services
			stages = append(stages, []schema.Stage{
				{
					Name: "Create k0s services for systemd",
					Files: []schema.File{
						{
							Path:        "/etc/systemd/system/k0scontroller.service",
							Permissions: 0644,
							Owner:       0,
							Group:       0,
							Content:     bundled.K0sControllerSystemd,
						},
						{
							Path:        "/etc/systemd/system/k0sworker.service",
							Permissions: 0644,
							Owner:       0,
							Group:       0,
							Content:     bundled.K0sWorkerSystemd,
						},
					},
				},
			}...)
		}
	}
	return stages
}

// GetInstallOemCloudConfigs dumps the OEM files to the system from the embedded oem files
// TODO: Make them first class yip files in code and just dump them into the system?
// That way they can be set as a normal yip stage maybe? a yip stage that dumps the yip stage lol
func GetInstallOemCloudConfigs(l types.KairosLogger) error {
	if config.ContainsSkipStep(values.CloudconfigsStep) {
		l.Logger.Warn().Msg("Skipping installing cloudconfigs stage")
		return nil
	}
	files, err := bundled.EmbeddedConfigs.ReadDir("cloudconfigs")
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to read embedded files")
		return err
	}

	// Extract each file
	for _, file := range files {
		if !file.IsDir() {
			data, err := bundled.EmbeddedConfigs.ReadFile(filepath.Join("cloudconfigs", file.Name()))
			if err != nil {
				l.Logger.Error().Err(err).Str("file", file.Name()).Msg("Failed to read embedded file")
				continue
			}

			// check if /system/oem exists and create it if not
			if _, err = os.Stat("/system/oem"); os.IsNotExist(err) {
				err = os.MkdirAll("/system/oem", 0755)
				if err != nil {
					l.Logger.Error().Err(err).Str("dir", "/system/oem").Msg("Failed to create directory")
					continue
				}
			}
			outputPath := filepath.Join("/system/oem/", file.Name())
			err = os.WriteFile(outputPath, data, 0644)
			if err != nil {
				fmt.Printf("Failed to write file %s: %v\n", outputPath, err)
				continue
			}

			l.Logger.Debug().Str("file", outputPath).Msg("Wrote cloud config")
		}
	}
	return nil
}

// GetInstallBrandingStage returns the branding stage
// This stage takes care of creating the default branding files that are used by the system
// Thinks like interactive install or recoivery welcome text or grubmenu configs
func GetInstallBrandingStage(_ values.System, l types.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.BrandingStep) {
		l.Logger.Warn().Msg("Skipping installing branding stage")
		return []schema.Stage{}
	}
	var data []schema.Stage

	data = append(data, []schema.Stage{
		{
			Name: "Create branding files",
			Files: []schema.File{
				{
					Path:        "/etc/kairos/branding/grubmenu.cfg",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.ExtraGrubCfg,
				},
				{
					Path:        "/etc/kairos/branding/interactive_install_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.InteractiveText,
				},
				{
					Path:        "/etc/kairos/branding/recovery_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.RecoveryText,
				},
				{
					Path:        "/etc/kairos/branding/reset_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.ResetText,
				},
				{
					Path:        "/etc/kairos/branding/install_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.InstallText,
				},
			},
		},
	}...)
	return data
}

// GetInstallGrubBootArgsStage returns the stage to write the grub configs
// This stage takes create of creating the /etc/cos/bootargs.cfg and /etc/cos/grub.cfg
func GetInstallGrubBootArgsStage(_ values.System, l types.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.GrubStep) {
		l.Logger.Warn().Msg("Skipping installing grub stage")
		return []schema.Stage{}
	}
	var data []schema.Stage
	// On trusted boot this is useless
	if config.DefaultConfig.TrustedBoot {
		return data
	}

	data = append(data, []schema.Stage{
		{
			Name: "Install grub configs",
			Files: []schema.File{
				{
					Path:        "/etc/cos/grub.cfg",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.GrubCfg,
				},
				{
					Path:        "/etc/cos/bootargs.cfg",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.BootArgsCfg,
				},
			},
		},
	}...)

	return data
}

// GetInstallKairosBinaries directly installs the kairos binaries from bundled binaries
func GetInstallKairosBinaries(sis values.System, l types.KairosLogger) error {
	if config.ContainsSkipStep(values.KairosBinariesStep) {
		l.Logger.Warn().Msg("Skipping installing Kairos binaries stage")
		return nil
	}
	//  If versions are provided, download and install those instead? i.e. Allow online install versions?

	binaries := map[string]string{
		"/usr/bin/kairos-agent":                         config.DefaultConfig.VersionOverrides.Agent,
		"/usr/bin/immucore":                             config.DefaultConfig.VersionOverrides.Immucore,
		"/system/discovery/kcrypt-discovery-challenger": config.DefaultConfig.VersionOverrides.KcryptChallenger,
	}

	for dest, version := range binaries {
		if version != "" {
			// Create the directory if it doesn't exist
			if _, err := os.Stat(filepath.Dir(dest)); os.IsNotExist(err) {
				err := os.MkdirAll(filepath.Dir(dest), 0755)
				if err != nil {
					l.Logger.Error().Err(err).Str("dir", filepath.Dir(dest)).Msg("Failed to create directory")
				}
			}

			reponame := filepath.Base(dest)
			url := fmt.Sprintf("https://github.com/kairos-io/%[1]s/releases/download/%[2]s/%[1]s-%[2]s-Linux-%[3]s", reponame, version, sis.Arch)
			// Append -fips to the url if fips is enabled
			if config.DefaultConfig.Fips {
				url = fmt.Sprintf("%s-fips", url)
			}
			// Add the .tar.gz to the url
			url = fmt.Sprintf("%s.tar.gz", url)
			l.Logger.Info().Str("url", url).Msg("Downloading binary")
			err := DownloadAndExtract(url, dest)
			if err != nil {
				l.Logger.Error().Err(err).Str("binary", dest).Msg("Failed to download and extract binary")
				return err
			}
		} else {
			// Use embedded binaries
			var data []byte
			switch dest {
			case "/usr/bin/kairos-agent":
				data = bundled.EmbeddedAgent
			case "/usr/bin/immucore":
				data = bundled.EmbeddedImmucore
			case "/system/discovery/kcrypt-discovery-challenger":
				data = bundled.EmbeddedKcryptChallenger
			}

			// Create the directory if it doesn't exist
			if _, err := os.Stat(filepath.Dir(dest)); os.IsNotExist(err) {
				err := os.MkdirAll(filepath.Dir(dest), 0755)
				if err != nil {
					l.Logger.Error().Err(err).Str("dir", filepath.Dir(dest)).Msg("Failed to create directory")
				}
			}

			err := os.WriteFile(dest, data, 0755)
			if err != nil {
				l.Logger.Error().Err(err).Str("binary", dest).Msg("Failed to write embedded binary")
				return err
			}
		}
	}

	return nil
}

// GetInstallProviderBinaries installs the provider and edgevpn binaries
func GetInstallProviderBinaries(sis values.System, l types.KairosLogger) error {
	if config.ContainsSkipStep(values.ProviderBinariesStep) {
		l.Logger.Warn().Msg("Skipping installing Kairos k8s provider binaries stage")
		return nil
	}
	// If its core we dont do anything here
	if config.DefaultConfig.Variant.String() == "core" {
		return nil
	}

	err := os.MkdirAll("/system/providers", os.ModeDir|os.ModePerm)
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to create directory")
		return err
	}

	binaries := map[string]string{
		"/system/providers/agent-provider-kairos": config.DefaultConfig.VersionOverrides.Provider,
		"/usr/bin/edgevpn":                        config.DefaultConfig.VersionOverrides.EdgeVpn,
	}

	for dest, version := range binaries {
		if version != "" {
			// Create the directory if it doesn't exist
			if _, err := os.Stat(filepath.Dir(dest)); os.IsNotExist(err) {
				err := os.MkdirAll(filepath.Dir(dest), 0755)
				if err != nil {
					l.Logger.Error().Err(err).Str("dir", filepath.Dir(dest)).Msg("Failed to create directory")
					return err
				}
			}

			org := "kairos-io"
			arch := sis.Arch
			// Check if the destination is edgevpn, if so we need to use mudler as the org
			// And change the arch to x86_64 if its amd64
			if dest == "/usr/bin/edgevpn" {
				org = "mudler"
				if arch == "amd64" {
					arch = "x86_64"
				}
			}
			// Binary destination has the prefix agent- so we need to remove it as the repo does not have it, nor the file
			binaryName := strings.Replace(filepath.Base(dest), "agent-", "", 1)
			url := fmt.Sprintf("https://github.com/%[4]s/%[1]s/releases/download/%[2]s/%[1]s-%[2]s-Linux-%[3]s", binaryName, version, arch, org)

			// Append -fips to the url if fips is enabled for provider only
			if config.DefaultConfig.Fips && dest != "/usr/bin/edgevpn" {
				url = fmt.Sprintf("%s-fips", url)
			}
			// Add the .tar.gz to the url
			url = fmt.Sprintf("%s.tar.gz", url)
			l.Logger.Info().Str("url", url).Msg("Downloading binary")
			err := DownloadAndExtract(url, dest, binaryName)
			if err != nil {
				l.Logger.Error().Err(err).Str("binary", dest).Msg("Failed to download and extract binary")
				return err
			}
		} else {
			// Use embedded binaries
			var data []byte
			switch dest {
			case "/system/providers/agent-provider-kairos":
				if config.DefaultConfig.Fips {
					data = bundled.EmbeddedKairosProviderFips
				} else {
					data = bundled.EmbeddedKairosProvider
				}
			case "/usr/bin/edgevpn":
				data = bundled.EmbeddedEdgeVPN
			}

			// Create the directory if it doesn't exist
			if _, err := os.Stat(filepath.Dir(dest)); os.IsNotExist(err) {
				err := os.MkdirAll(filepath.Dir(dest), 0755)
				if err != nil {
					l.Logger.Error().Err(err).Str("dir", filepath.Dir(dest)).Msg("Failed to create directory")
				}
			}

			err := os.WriteFile(dest, data, 0755)
			if err != nil {
				l.Logger.Error().Err(err).Str("binary", dest).Msg("Failed to write embedded binary")
				return err
			}
		}
	}

	// Link /system/providers/agent-provider-kairos to /usr/bin/kairos, not sure what uses it?
	// TODO: Check if this is needed, maybe we can remove it?
	err = os.Symlink("/system/providers/agent-provider-kairos", "/usr/bin/kairos")
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to create symlink")
		return err
	}
	return nil
}

// GetKairosInitramfsFilesStage installs the kairos initramfs files
// This stage is used to install the initramfs files that are needed for the system to boot
func GetKairosInitramfsFilesStage(sis values.System, l types.KairosLogger) ([]schema.Stage, error) {
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
		// otherwise systemd-networkd doest not trigger the dracut hooks to let it know that its up and running
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

		if sis.Distro == values.RedHat {
			// On redhat we drop the systemd-networkd module as there is no systemd-networkd on rh9
			networkModule = "network-legacy"
		}

		if sis.Distro == values.Fedora {
			// On Fedora we drop the network-legacy module
			networkModule = "systemd-networkd"
		}

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
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*[O-o]penSUSE.*",
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
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*|[O-o]penSUSE.*",
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
				OnlyIfOs: "Ubuntu.*|Debian.*|Fedora.*|CentOS.*|Red\\sHat.*|Rocky.*|AlmaLinux.*|openSUSE.*|SUSE.*|[O-o]penSUSE.*",
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

// GetKairosMiscellaneousFilesStage installs the kairos miscellaneous files
// Like small scripts or other files that are not part of the main install process
func GetKairosMiscellaneousFilesStage(sis values.System, l types.KairosLogger) []schema.Stage {
	if config.ContainsSkipStep(values.MiscellaneousStep) {
		l.Logger.Warn().Msg("Skipping installing miscellaneous configs stage")
		return []schema.Stage{}
	}

	var data []schema.Stage

	data = append(data, []schema.Stage{
		{
			Name: "Create kairos welcome message",
			Files: []schema.File{
				{
					Path:        "/etc/issue.d/01-KAIROS",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.Issue,
				},
				{
					Path:        "/etc/motd",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.MOTD,
				},
			},
		},
		{
			Name: "Install suc-upgrade script",
			Files: []schema.File{
				{
					Path:        "/usr/sbin/suc-upgrade",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.SucUpgrade,
				},
			},
		},
		{
			Name: "Install logrotate config",
			Files: []schema.File{
				{
					Path:        "/etc/logrotate.d/kairos",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     bundled.LogRotateConfig,
				},
			},
		},
	}...)

	return data
}

// DownloadAndExtract downloads a tar.gz file from the specified URL, extracts its contents,
// and searches for a binary file to move to the destination path. If a binary name is provided
// as an optional parameter, it uses that name to locate the binary in the archive; otherwise,
// it defaults to using the base name of the destination path. The function returns an error
// if the download, extraction, or file operations fail, or if the binary is not found in the archive.
func DownloadAndExtract(url, dest string, binaryName ...string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tarReader := tar.NewReader(gzr)
	targetBinary := filepath.Base(dest)
	if len(binaryName) > 0 {
		targetBinary = binaryName[0]
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar file: %w", err)
		}

		if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, targetBinary) {
			outFile, err := os.Create(dest)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, tarReader)
			if err != nil {
				return fmt.Errorf("failed to copy file content: %w", err)
			}
			// Set the file permissions

			err = outFile.Chmod(0755)
			if err != nil {
				return fmt.Errorf("failed to set file permissions: %w", err)
			}

			return nil
		}
	}
	return fmt.Errorf("binary not found in archive")
}
