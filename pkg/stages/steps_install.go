package stages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/kairos-io/kairos-init/pkg/bundled"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
)

// This file contains the stages for the install process

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

// GetInstallProviderAndKubernetes will install the provider and kubernetes packages
func GetInstallProviderAndKubernetes(sis values.System, l types.KairosLogger) []schema.Stage {
	var data []schema.Stage

	// If its core we dont do anything here
	if config.DefaultConfig.Variant.String() == "core" {
		return data
	}
	err := os.MkdirAll("/system/providers", os.ModeDir|os.ModePerm)
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to create directory")
		return data
	}
	// write the embedded binaries to the system
	err = os.WriteFile("/system/providers/agent-provider-kairos", bundled.EmbeddedKairosProvider, 0755)
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to write agent-provider-kairos")
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
		}...)

		if sis.Family.String() == "alpine" {
			// Add openrc services
			data = append(data, []schema.Stage{
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
			data = append(data, []schema.Stage{
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
	// Install provider + k8s utils
	data = append(data, []schema.Stage{
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

// GetInstallOemCloudConfigs dumps the OEM files to the system from the embedded oem files
// TODO: Make them first class yip files in code and just dump them into the system?
// That way they can be set as a normal yip stage maybe? a yip stage that dumps the yip stage lol
func GetInstallOemCloudConfigs(l types.KairosLogger) error {
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
func GetInstallBrandingStage(_ values.System, _ types.KairosLogger) []schema.Stage {
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
func GetInstallGrubBootArgsStage(_ values.System, _ types.KairosLogger) []schema.Stage {
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

// GetInstallServicesStage returns the stage to create the services
// This installs some services that for some reason are not created by the configs
// TODO: Ideally this should be moved to be created on boot with cc instead of install
func GetInstallServicesStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	var data []schema.Stage

	data = append(data, []schema.Stage{
		{
			Name: "Create kairos services",
			If:   "[ ! -f \"/sbin/openrc\" ]",
			Files: []schema.File{
				{
					Path:        "/etc/systemd/system/kairos-agent.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.KairosAgentService,
				},
				{
					Path:        "/etc/systemd/system/kairos-recovery.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.KairosRecoveryService,
				},
				{
					Path:        "/etc/systemd/system/kairos-reset.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.KairosResetservice,
				},
				{
					Path:        "/etc/systemd/system/kairos-webui.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.KairosWebUIService,
				},
				{
					Path:        "/etc/systemd/system/kairos.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.KairosInstallerService,
				},
				{
					Path:        "/etc/systemd/system/kairos-interactive.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content:     bundled.KairosInteractiveService,
				},
			},
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
	}...)

	return data
}

// GetInstallKairosBinaries directly installs the kairos binaries from the remote location
// TODO: Ideally this should be able to be done with a yip plugin so it respects the rest of the process
// Something like InstallFromGithubRelease
// In some distros, this needs to run after the packages are installed due to the ca-certificates package
// being installed later, as we are using https
func GetInstallKairosBinaries(_ values.System, l types.KairosLogger) error {
	// TODO: If versions are provided, download and install those instead
	// TODO: Fips?

	// write the embedded binaries to the system
	err := os.WriteFile("/usr/bin/kairos-agent", bundled.EmbeddedAgent, 0755)
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to write kairos-agent")
		return err
	}

	err = os.WriteFile("/usr/bin/immucore", bundled.EmbeddedImmucore, 0755)
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to write immucore")
		return err
	}

	// Check if dir exists and create it if not
	if _, err = os.Stat("/system/discovery/"); os.IsNotExist(err) {
		err = os.MkdirAll("/system/discovery/", 0755)
		if err != nil {
			l.Logger.Error().Err(err).Msg("Failed to create directory")
			return err
		}
	}

	err = os.WriteFile("/system/discovery/kcrypt-discovery-challenger", bundled.EmbeddedKcryptChallenger, 0755)

	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to write kcrypt-discovery-challenger")
		return err
	}

	return nil
}

// InstallKairosMiscellaneousFilesStage installs the kairos miscellaneous files
// Like small scripts or other files that are not part of the main install process
func InstallKairosMiscellaneousFilesStage(sis values.System, l types.KairosLogger) ([]schema.Stage, error) {
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
	}

	return data, nil
}
