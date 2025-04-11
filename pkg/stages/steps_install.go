package stages

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
)

//go:embed cloudconfigs/*
var embeddedConfigs embed.FS

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

// GetInstallOemCloudConfigs dumps the OEM files to the system from the embedded oem files
// TODO: Make them first class yip files in code and just dump them into the system?
// That way they can be set as a normal yip stage maybe? a yip stage that dumps the yip stage lol
func GetInstallOemCloudConfigs(l types.KairosLogger) error {
	files, err := embeddedConfigs.ReadDir("cloudconfigs")
	if err != nil {
		l.Logger.Error().Err(err).Msg("Failed to read embedded files")
		return err
	}

	// Extract each file
	for _, file := range files {
		if !file.IsDir() {
			data, err := embeddedConfigs.ReadFile(filepath.Join("cloudconfigs", file.Name()))
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
					Content: `
menuentry "Kairos remote recovery" --id remoterecovery {
    search --no-floppy --label --set=root COS_RECOVERY
    if [ test -s /cOS/recovery.squashfs ]; then
        set img=/cOS/recovery.squashfs
        set recoverylabel=COS_RECOVERY
    else
        set img=/cOS/recovery.img
    fi
    set label=COS_SYSTEM
    loopback loop0 /$img
    set root=($root)
    source (loop0)/etc/cos/bootargs.cfg
    linux (loop0)$kernel $kernelcmd ${extra_cmdline} ${extra_recovery_cmdline} vga=795 nomodeset kairos.remote_recovery_mode
    initrd (loop0)$initramfs
}

menuentry "Kairos state reset (auto)" --id statereset {
    search --no-floppy --label --set=root COS_RECOVERY
    if [ test -s /cOS/recovery.squashfs ]; then
        set img=/cOS/recovery.squashfs
        set recoverylabel=COS_RECOVERY
    else
        set img=/cOS/recovery.img
    fi
    set label=COS_SYSTEM
    loopback loop0 /$img
    set root=($root)
    source (loop0)/etc/cos/bootargs.cfg
    linux (loop0)$kernel $kernelcmd ${extra_cmdline} ${extra_recovery_cmdline} vga=795 nomodeset kairos.reset
    initrd (loop0)$initramfs
}`,
				},
				{
					Path:        "/etc/kairos/branding/interactive_install",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content:     "Interactive installation. Documentation is available at https://kairos.io.",
				},
				{
					Path:        "/etc/kairos/branding/recovery_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content: `Welcome to kairos recovery mode!
P2P device recovery mode is starting.
A QR code with a generated network token will be displayed below that can be used to connect 
over with "kairos bridge --qr-code-image /path/to/image.jpg" from another machine, 
further instruction will appear on the bridge CLI to connect over via SSH.
IF the qrcode is not displaying correctly,
try booting with another vga option from the boot cmdline (e.g. vga=791).

Press any key to abort recovery. To restart the process run 'kairos recovery'.`,
				},
				{
					Path:        "/etc/kairos/branding/reset_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content: `Welcome to kairos!
The node will automatically reset its state in a few.

Press any key to abort this process. To restart run 'kairos reset'.

Starting in 60 seconds...`,
				},
				{
					Path:        "/etc/kairos/branding/install_text",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content: `Welcome to Kairos!
P2P device installation enrollment is starting.
A QR code will be displayed below.
In another machine, run "kairos register" with the QR code visible on screen,
or "kairos register <file>" to register the machine from a photo.
IF the qrcode is not displaying correctly,
try booting with another vga option from the boot cmdline (e.g. vga=791).

Press any key to abort pairing. To restart run 'kairos install'.

Starting in 5 seconds...`,
				},
			},
		},
	}...)
	return data
}

// GetInstallGrubBootArgsStage returns the stage to write the grub boot args
// This stage takes create of creating the /etc/cos/bootargs.cfg that grub reads on boot in order to
// set ad more dynamic kernel cmdline
func GetInstallGrubBootArgsStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	var data []schema.Stage
	// On trusted boot this is useless
	if config.DefaultConfig.TrustedBoot {
		return data
	}

	data = append(data, []schema.Stage{
		{
			Name: "Create bootargs.cfg",
			Files: []schema.File{
				{
					Path:        "/etc/cos/bootargs.cfg",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content: `function setSelinux {
    source (loop0)/etc/os-release
    if [ -f (loop0)/etc/kairos-release ]; then
        source (loop0)/etc/kairos-release
    fi

    # Disable selinux for all distros. Supporting selinux requires more than
    # just enabling it like this.
    set baseSelinuxCmd="selinux=0"

    #if test $KAIROS_FAMILY == "rhel" -o test $ID == "opensuse-tumbleweed" -o test $ID == "opensuse-leap"; then
    #    set baseSelinuxCmd="selinux=0"
    #else
    #    # if not in recovery
    #    if [ -z "$recoverylabel" ];then
    #        set baseSelinuxCmd="security=selinux selinux=1"
    #    fi
    #fi
}

function setExtraConsole {
    source (loop0)/etc/os-release
    if [ -f (loop0)/etc/kairos-release ]; then
        source (loop0)/etc/kairos-release
    fi
    set baseExtraConsole="console=ttyS0"
    # rpi
    if test $KAIROS_MODEL == "rpi3" -o test $KAIROS_MODEL == "rpi4"; then
        set baseExtraConsole="console=ttyS0,115200"
    fi
    # nvidia orin
    if test $KAIROS_MODEL == "nvidia-jetson-agx-orin"; then
        set baseExtraConsole="console=ttyTCU0,115200"
    fi
}

function setExtraArgs {
    source (loop0)/etc/os-release
    if [ -f (loop0)/etc/kairos-release ]; then
        source (loop0)/etc/kairos-release
    fi
    set baseExtraArgs=""
    # rpi
    if test $KAIROS_MODEL == "rpi3" -o test $KAIROS_MODEL == "rpi4"; then
        # on rpi we need to enable memory cgroup for docker/k3s to work
        set baseExtraArgs="modprobe.blacklist=vc4 8250.nr_uarts=1 cgroup_enable=memory"
    fi
}

function setKernelCmd {
    # At this point we have the system mounted under (loop0)
    #
    # baseCmd -> Shared between all entries
    # baseRootCmd -> specific bits that immucore uses to mount the boot devices and identify the image to mount
    # baseSelinuxCmd -> selinux enabled/disabled
    # baseExtraConsole -> extra console to set
    # baseExtraArgs -> extra needed args
    set baseCmd="console=tty1 net.ifnames=1 rd.cos.oemlabel=COS_OEM rd.cos.oemtimeout=10 panic=5 rd.emergency=reboot rd.shell=0 systemd.crash_reboot=yes"
    if [ -n "$recoverylabel" ]; then
        set baseRootCmd="root=live:LABEL=$recoverylabel rd.live.dir=/ rd.live.squashimg=$img"
    else
        set baseRootCmd="root=LABEL=$label cos-img/filename=$img"
    fi
    setSelinux
    setExtraConsole
    setExtraArgs
    # finally set the full cmdline
    set kernelcmd="$baseExtraConsole $baseCmd $baseRootCmd $baseSelinuxCmd $baseExtraArgs"
}

# grub.cfg now ships this but during upgrades we do not update the COS_GRUB partition, so no new grub.cfg is copied over there
# We need to keep it for upgrades to work.
# TODO: Deprecate in v2.8-v3.0
set kernel=/boot/vmlinuz
set initramfs=/boot/initrd
# set the kernelcmd dynamically
setKernelCmd
`,
				},
			},
		},
	}...)

	return data
}

// GetInstallMiscellaneousFilesStage returns the stage to create the miscellaneous files
// Some files that do not fall into any other category are created here
// TODO: Ideally this should all be moved to be created on boot with cc instead of install
func GetInstallMiscellaneousFilesStage(_ values.System, _ types.KairosLogger) []schema.Stage {
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
					Content: `                                                          
    _/    _/            _/                                
   _/  _/      _/_/_/      _/  _/_/    _/_/      _/_/_/   
  _/_/      _/    _/  _/  _/_/      _/    _/  _/_/        
 _/  _/    _/    _/  _/  _/        _/    _/      _/_/     
_/    _/    _/_/_/  _/  _/          _/_/    _/_/_/        
                                                          
                         
`,
				},
				{
					Path:        "/etc/motd",
					Permissions: 0644,
					Owner:       0,
					Group:       0,
					Content: `Welcome to Kairos!

Refer to https://kairos.io for documentation.
`,
				},
			},
		},
		{
			Name: "Create miscellaneous binaries",
			Files: []schema.File{
				{
					Path:        "/usr/bin/cos-setup-reconcile",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `#!/bin/sh

SLEEP_TIME=${SLEEP_TIME:-360}

while :
do
    kairos-agent run-stage "reconcile"
    sleep "$SLEEP_TIME"
done`,
				},
				{
					Path:        "/usr/bin/fix-home-dir-ownership",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `#!/bin/bash

set -e

SENTINEL_FILE="/usr/local/.kairos/skip-home-directory-ownership-fix"

if [ -f $SENTINEL_FILE ]; then
    echo "Skipping ownership fix because sentinel file was found: $SENTINEL_FILE"
    exit 0
fi

# Iterate over users in /etc/passwd and chown their directories
awk -F: '$3 >= 1000 && $6 ~ /^\/home\// {print $1, $6}' /etc/passwd | while read -r user homedir; do
    if [ -d "$homedir" ]; then  # Check if the home directory exists
        echo "Changing ownership of $homedir to $user"
        chown -R "$user":"$user" "$homedir"
    else
        echo "Directory $homedir does not exist for user $user"
    fi
done

# Write the sentinel file
mkdir -p "$(dirname $SENTINEL_FILE)"
echo "https://github.com/kairos-io/kairos/issues/2843" > $SENTINEL_FILE
`,
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
			Name: "Create system services dir",
			If:   "test -d /etc/systemd/system && [ ! -f \"/sbin/openrc\" ]",
			Directories: []schema.Directory{
				{
					Path:        "/etc/systemd/system",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
				},
			},
		},
		{
			Name: "Create kairos services",
			If:   "[ ! -f \"/sbin/openrc\" ]",
			Files: []schema.File{
				{
					Path:        "/etc/systemd/system/kairos-agent.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `[Unit]
Description=kairos agent
After=cos-setup-network.service
Wants=network.target
[Service]
Restart=on-failure
RestartSec=5s
ExecStart=/usr/bin/kairos-agent start
[Install]
WantedBy=multi-user.target`,
				},
				{
					Path:        "/etc/systemd/system/kairos-recovery.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `[Unit]
Description=kairos recovery
After=multi-user.target
[Service]
Type=simple
StandardInput=tty
StandardOutput=tty
LimitNOFILE=49152
ExecStartPre=-/bin/sh -c "dmesg -D"
# This source explains why we are using this number
# https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes/a3327deff89d2428d48596ce0e643531f9944f99
ExecStartPre=-/bin/sh -c "sysctl -w net.core.rmem_max=7500000"
# Stop systemd messages on tty
ExecStartPre=-/usr/bin/kill -SIGRTMIN+21 1
TTYPath=/dev/tty1
RemainAfterExit=yes
ExecStart=/usr/bin/kairos-agent recovery
# Start systemd messages on tty
ExecStartPost=-/usr/bin/kill -SIGRTMIN+20 1
[Install]
WantedBy=multi-user.target
`,
				},
				{
					Path:        "/etc/systemd/system/kairos-reset.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `[Unit]
Description=kairos reset
After=sysinit.target
[Service]
Type=oneshot
StandardInput=tty
StandardOutput=tty
LimitNOFILE=49152
TTYPath=/dev/tty1
RemainAfterExit=yes
# Stop systemd messages on tty
ExecStartPre=-/usr/bin/kill -SIGRTMIN+21 1
ExecStart=/usr/bin/kairos-agent reset --unattended --reboot
# Start systemd messages on tty
ExecStartPost=-/usr/bin/kill -SIGRTMIN+20 1
TimeoutStopSec=10s
[Install]
WantedBy=multi-user.target
`,
				},
				{
					Path:        "/etc/systemd/system/kairos-webui.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `[Unit]
Description=kairos installer
After=sysinit.target
[Service]
ExecStart=/usr/bin/kairos-agent webui
TimeoutStopSec=10s
[Install]
WantedBy=multi-user.target`,
				},
				{
					Path:        "/etc/systemd/system/kairos.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `[Unit]
Description=kairos installer
After=multi-user.target
[Service]
Type=simple
StandardInput=tty
StandardOutput=tty
LimitNOFILE=49152
ExecStartPre=-/bin/sh -c "dmesg -D"
TTYPath=/dev/tty1
RemainAfterExit=yes
# Stop systemd messages on tty
ExecStartPre=-/usr/bin/kill -SIGRTMIN+21 1
ExecStart=/usr/bin/kairos-agent install
# Start systemd messages on tty
ExecStartPost=-/usr/bin/kill -SIGRTMIN+20 1
TimeoutStopSec=10s
[Install]
WantedBy=multi-user.target`,
				},
				{
					Path:        "/etc/systemd/system/kairos-interactive.service",
					Permissions: 0755,
					Owner:       0,
					Group:       0,
					Content: `[Unit]
Description=kairos interactive-installer
After=multi-user.target
[Service]
## Dont mark it as running until it finishes
Type=oneshot
# input/output to tty as its interactive
# otherwise it will be silent and with no input
StandardInput=tty
StandardOutput=tty
LimitNOFILE=49152
ExecStartPre=-/bin/sh -c "dmesg -D"
TTYPath=/dev/tty1
RemainAfterExit=yes
# Stop systemd messages on tty
ExecStartPre=-/usr/bin/kill -SIGRTMIN+21 1
ExecStart=/usr/bin/kairos-agent interactive-install --shell
# Start systemd messages on tty
ExecStartPost=-/usr/bin/kill -SIGRTMIN+20 1
TimeoutStopSec=10s
# Restart if it fails, like user doing control+c
Restart=on-failure
[Install]
WantedBy=multi-user.target`,
				},
			},
		},
	}...)

	return data
}

// GetInstallKairosBinariesStage directly installs the kairos binaries from the remote location
// TODO: Ideally this should be able to be done wiht a yip plugin
// Something like InstallFromGithubRelease
// In some distros, this needs to run after the packages are installed due to the ca-certificates package
// being installed later, as we are using https
func GetInstallKairosBinariesStage(sis values.System, l types.KairosLogger) error {
	var fips string
	targetDir := "/usr/bin"
	arch := sis.Arch.String()

	if config.DefaultConfig.Fips {
		fips = "-fips"
	}

	// Download the kairos-agent binary
	url := fmt.Sprintf("https://github.com/kairos-io/kairos-agent/releases/download/%s/kairos-agent-%s-Linux-%s%s.tar.gz", values.GetAgentVersion(), values.GetAgentVersion(), arch, fips)
	binaryName := "kairos-agent"

	err := downloadAndExtractBinaryInMemory(l, url, binaryName, targetDir)
	if err != nil {
		return err
	}
	// Download the immucore binary
	url = fmt.Sprintf("https://github.com/kairos-io/immucore/releases/download/%s/immucore-%s-Linux-%s%s.tar.gz", values.GetImmucoreVersion(), values.GetImmucoreVersion(), arch, fips)
	binaryName = "immucore"
	err = downloadAndExtractBinaryInMemory(l, url, binaryName, targetDir)
	if err != nil {
		return err
	}

	return nil
}

func downloadAndExtractBinaryInMemory(l types.KairosLogger, url, binaryName, targetDir string) error {
	// Perform the HTTP GET request
	l.Logger.Debug().Str("url", url).Msg("Downloading file")
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: status code %d", resp.StatusCode)
	}

	// Create a gzip reader directly from the response body
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create a tar reader from the gzip reader
	tarReader := tar.NewReader(gzReader)

	// Extract the binary
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar archive: %w", err)
		}

		// Check if the current file is the binary we want
		if filepath.Base(header.Name) == binaryName {
			targetPath := filepath.Join(targetDir, binaryName)

			// Create the target file
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create target file: %w", err)
			}
			defer outFile.Close()

			// Copy the binary content to the target file
			_, err = io.Copy(outFile, tarReader)
			if err != nil {
				return fmt.Errorf("failed to extract binary: %w", err)
			}

			// Make the binary executable
			err = os.Chmod(targetPath, 0755)
			if err != nil {
				return fmt.Errorf("failed to set executable permissions: %w", err)
			}

			l.Logger.Debug().Str("target", targetPath).Str("name", binaryName).Msg("Binary extracted")
			return nil
		}
	}

	return fmt.Errorf("binary %s not found in archive", binaryName)
}
