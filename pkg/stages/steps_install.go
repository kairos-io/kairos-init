package stages

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/mudler/go-pluggable"

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

	if sis.Distro == values.Hadron {
		logger.Logger.Info().Msg("Hadron Linux does not require package installation")
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

	// Get the full version from the system info parsed so we can use the major version
	fullVersion, err := semver.NewSemver(sis.Version)
	if err != nil {
		logger.Logger.Error().Msgf("Failed to parse the version %s: %s", sis.Version, err)
		return []schema.Stage{}, err
	}

	// Read the NVIDIA env variables, use defaults if not set
	nvidiaRelease := os.Getenv("NVIDIA_RELEASE")
	if nvidiaRelease == "" {
		// This was just introduced in PR #211, however if you check the
		// Dockerfile.nvidia-orin-nx it says 36 :shrug:, do we actually need a
		// default or should the user always set it? if we have a default, should it
		// ever change?
		nvidiaRelease = "35"
	}

	nvidiaVersion := os.Getenv("NVIDIA_VERSION")
	if nvidiaVersion == "" {
		// This was just introduced in PR #211, however if you check the
		// Dockerfile.nvidia-orin-nx it says 4.4 :shrug:, do we actually need a
		// default or should the user always set it? if we have a default, should it
		// ever change?
		nvidiaVersion = "3.1"
	}

	l4tVersion := os.Getenv("L4T_VERSION")
	if l4tVersion == "" {
		l4tVersion = "36.4"
	}

	// Get board model from environment or config
	boardModel := os.Getenv("BOARD_MODEL")
	if boardModel == "" {
		// Does it make sense that both AGX Orin and Orin NX use the same board model?
		boardModel = "t234"
	}

	// Prepare NVIDIA L4T extraction script
	l4tScript := fmt.Sprintf(`#!/bin/bash
		set -e

		NVIDIA_RELEASE="%s"
		NVIDIA_VERSION="%s"
		NVIDIA_ARCHIVE_URI="https://developer.nvidia.com/downloads/embedded/l4t/r${NVIDIA_RELEASE}_release_v${NVIDIA_VERSION}/release"
		TEGRA_ARCHIVE="jetson_linux_r${NVIDIA_RELEASE}.${NVIDIA_VERSION}_aarch64.tbz2"
		ROOTFS_ARCHIVE="tegra_linux_sample-root-filesystem_r${NVIDIA_RELEASE}.${NVIDIA_VERSION}_aarch64.tbz2"
		TEGRA_DIR="Linux_for_Tegra"

		echo "Downloading NVIDIA L4T archives..."
		wget "${NVIDIA_ARCHIVE_URI}/${TEGRA_ARCHIVE}" -O "$TEGRA_ARCHIVE"
		wget "${NVIDIA_ARCHIVE_URI}/${ROOTFS_ARCHIVE}" -O "$ROOTFS_ARCHIVE"

		echo "Extracting Jetson Linux..."
		tar -xjf "$TEGRA_ARCHIVE"
		tar -xjf "$ROOTFS_ARCHIVE" -C "$TEGRA_DIR/rootfs"
		`, nvidiaRelease, nvidiaVersion)

	stage := []schema.Stage{
		{
			Name:     "Install epel repository",
			OnlyIfOs: "AlmaLinux.*|Rocky.*|CentOS.*",
			Packages: schema.Packages{
				Install: []string{"epel-release"},
			},
		},
		{
			Name:     "Install epel repository for Red Hat",
			OnlyIfOs: "Red\\sHat.*",
			Commands: []string{
				fmt.Sprintf("dnf install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-%d.noarch.rpm", fullVersion.Segments()[0]),
			},
		},
		{
			Name:     "Disable grub2 probe trigger", // Grub2 will try to probe the system on install, which fails due it being on a container
			OnlyIfOs: "Alpine.*",
			Files: []schema.File{
				{
					Path:    "/etc/update-grub.conf",
					Content: "disable_trigger=1",
				},
			},
		},
		{
			Name:     "Disable mkinitfs trigger", // mkinitfs will generate an initramfs file but we dont need it
			OnlyIfOs: "Alpine.*",
			Files: []schema.File{
				{
					Path:    "/etc/mkinitfs/mkinitfs.conf",
					Content: "disable_trigger=true",
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
		{
			Name:     "Restore mkinitfs default config", // restore the mkinitfs default config to its proper place
			OnlyIfOs: "Alpine.*",
			If:       "test -f /etc/mkinitfs/mkinitfs.conf.apk-new",
			Commands: []string{
				"mv /etc/mkinitfs/mkinitfs.conf.apk-new /etc/mkinitfs/mkinitfs.conf",
			},
		},
		{
			Name: "Fetch Linux for Tegra (L4T)",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model),
			Commands: []string{
				l4tScript,
			},
		},
		{
			Name: "Setup NVIDIA L4T repositories",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-agx-orin" ] || [ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model, config.DefaultConfig.Model),
			Commands: []string{
				// Clean up existing NVIDIA repository files
				"rm -rf /etc/apt/sources.list.d/nvidia-l4t-apt-source.list",
				// Create NVIDIA L4T packages directory
				"mkdir -p /opt/nvidia/l4t-packages",
				"touch /opt/nvidia/l4t-packages/.nv-l4t-disable-boot-fw-update-in-preinstall",
				// Add NVIDIA GPG keys
				"curl -fSsL https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/3bf863cc.pub | gpg --dearmor | tee /usr/share/keyrings/nvidia-drivers-2004.gpg > /dev/null 2>&1",
				"curl -fSsL https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/3bf863cc.pub | gpg --dearmor | tee /usr/share/keyrings/nvidia-drivers-2204.gpg > /dev/null 2>&1",
				"curl -fSsL https://repo.download.nvidia.com/jetson/jetson-ota-public.asc | gpg --dearmor | tee /usr/share/keyrings/jetson-ota.gpg > /dev/null 2>&1",
				// Add NVIDIA repositories
				"echo 'deb [signed-by=/usr/share/keyrings/nvidia-drivers-2204.gpg] https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/ /' | tee -a /etc/apt/sources.list.d/nvidia-drivers.list",
				"echo 'deb [signed-by=/usr/share/keyrings/nvidia-drivers-2004.gpg] https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/ /' | tee -a /etc/apt/sources.list.d/nvidia-drivers.list",
				fmt.Sprintf("echo 'deb [signed-by=/usr/share/keyrings/jetson-ota.gpg] https://repo.download.nvidia.com/jetson/common/ r%s main' | tee -a /etc/apt/sources.list.d/nvidia-drivers.list", l4tVersion),
				fmt.Sprintf("echo 'deb [signed-by=/usr/share/keyrings/jetson-ota.gpg] https://repo.download.nvidia.com/jetson/%s/ r%s main' | tee -a /etc/apt/sources.list.d/nvidia-drivers.list", boardModel, l4tVersion),
			},
		},
		{
			Name: "Setup OpenCV symlink for NVIDIA devices",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-agx-orin" ] || [ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model, config.DefaultConfig.Model),
			Commands: []string{
				"ln -s /usr/include/opencv4/opencv2 /usr/include/opencv2",
			},
		},
		{
			Name: "Configure CUDA paths for NVIDIA devices",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-agx-orin" ] || [ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model, config.DefaultConfig.Model),
			Commands: []string{
				// Move CUDA out of the way to /opt so kairos can occupy /usr/local without workarounds
				"update-alternatives --remove-all cuda || true",
				"update-alternatives --remove-all cuda-12 || true",
				"mv /usr/local/cuda-12.6 /opt/cuda-12.6 || true",
				"update-alternatives --install /opt/cuda cuda /opt/cuda-12.6 1 || true",
				"update-alternatives --install /opt/cuda-12 cuda-12 /opt/cuda-12.6 1 || true",
			},
		},
		{
			Name: "Configure NVIDIA L4T USB device mode for NVIDIA devices",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-agx-orin" ] || [ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model, config.DefaultConfig.Model),
			Commands: []string{
				// Change mountpoint for l4t usb device mode, as rootfs is mounted ro
				// /srv/data is made through cloud-config
				"sed -i -e 's|mntpoint=\"/mnt|mntpoint=\"/srv/data|' /opt/nvidia/l4t-usb-device-mode/nv-l4t-usb-device-mode-start.sh || true",
			},
		},
		{
			Name: "Disable ISCSI for NVIDIA devices",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-agx-orin" ] || [ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model, config.DefaultConfig.Model),
			Files: []schema.File{
				{
					Path:    "/etc/dracut.conf.d/iscsi.conf",
					Content: "omit_dracutmodules+=\" iscsi \"",
				},
			},
		},
		{
			Name: "Disable ISCSI services for NVIDIA devices",
			If:   fmt.Sprintf(`[ "%s" = "nvidia-jetson-agx-orin" ] || [ "%s" = "nvidia-jetson-orin-nx" ]`, config.DefaultConfig.Model, config.DefaultConfig.Model),
			Commands: []string{
				// iscsid causes delays on the login shell, and we don't need it, so we'll disable it
				"systemctl disable iscsi open-iscsi iscsid.socket || true",
			},
		},
	}
	return stage, nil
}

func GetInstallKernelStage(sis values.System, logger types.KairosLogger) ([]schema.Stage, error) {
	if config.ContainsSkipStep(values.InstallKernelStep) {
		logger.Logger.Warn().Msg("Skipping install kernel stage")
		return []schema.Stage{}, nil
	}

	if sis.Distro == values.Hadron {
		logger.Logger.Info().Msg("Hadron Linux does not require kernel installation")
		return []schema.Stage{}, nil
	}

	// Get the packages
	packages, err := values.GetKernelPackages(sis, logger)
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

	stage := []schema.Stage{
		{
			Name: "Install kernel packages",
			Packages: schema.Packages{
				Install: finalMergedPkgs,
				Refresh: true,
				Upgrade: true,
			},
		},
	}

	return stage, nil
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

func ProviderBuildInstallEvent(sis values.System, logger types.KairosLogger) error {
	if config.ContainsSkipStep(values.BuildProviderStep) {
		logger.Logger.Warn().Msg("Skipping calling build for providers stage")
		return nil
	}

	providerCount := len(config.DefaultConfig.Providers)
	if providerCount == 0 {
		logger.Logger.Info().Msg("No providers configured, skipping provider install event")
		return nil
	}

	logger.Logger.Info().Msg("Triggering provider install event")
	// Trigger provider build-install event
	manager := bus.NewBus(bus.InitProviderInstall)
	manager.Initialize(bus.WithLogger(&logger))
	if len(manager.Plugins) == 0 {
		logger.Logger.Info().Msg("No plugins found, skipping provider install event")
		return nil
	}

	errChan := make(chan error, providerCount)
	var wg sync.WaitGroup
	wg.Add(providerCount)

	manager.Response(bus.InitProviderInstall, func(p *pluggable.Plugin, resp *pluggable.EventResponse) {
		logger.Logger.Debug().Str("at", p.Executable).Interface("resp", resp).Msg("Received build-install event from provider")
		if resp.Errored() {
			errChan <- fmt.Errorf("provider build-install event failed: %s", resp.Error)
			wg.Done()
			return
		}
		if resp.State == bus.EventResponseNotApplicable {
			logger.Logger.Info().Msg("Provider build-install event is non-applicable, skipping")
			errChan <- nil
			wg.Done()
			return
		}
		if resp.State == bus.EventResponseSuccess {
			logger.Logger.Info().Msg("Provider install event succeeded")
			errChan <- nil
			wg.Done()
			return
		}
		// If none of the above, still mark as done
		errChan <- nil
		wg.Done()
	})

	logger.Logger.Debug().Msg("Publishing provider build-install event")
	for _, provider := range config.DefaultConfig.Providers {
		dataSend := bus.ProviderPayload{
			Provider: provider.Name,
			Version:  provider.Version,
			Config:   provider.Config,
			LogLevel: logger.Logger.GetLevel().String(),
			Family:   sis.Family.String(),
		}
		_, err := manager.Publish(bus.InitProviderInstall, dataSend)
		if err != nil {
			logger.Logger.Error().Msgf("Failed to publish provider build-install event: %s", err)
			return err
		}
	}

	wg.Wait()
	var combinedErr error
	for i := 0; i < providerCount; i++ {
		err := <-errChan
		if err != nil {
			if combinedErr == nil {
				combinedErr = fmt.Errorf("provider build-install errors")
			}
			combinedErr = fmt.Errorf("%w; %v", combinedErr, err)
		}
	}
	if combinedErr != nil {
		return combinedErr
	}
	return nil
}
