package stages

import (
	"fmt"
	"os/exec"
	"regexp"

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

// GetInstallFrameworkStage This returns the Stage to install the framework image
// It uses the framework version from the config, defaulting to the latest found in the versions.go file
// If we enable fips, we append -fips to the framework version
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
