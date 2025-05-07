package bundled

import (
	"embed"
)

//go:embed binaries/kairos-agent
var EmbeddedAgent []byte

//go:embed binaries/fips/kairos-agent
var EmbeddedAgentFips []byte

//go:embed binaries/immucore
var EmbeddedImmucore []byte

//go:embed binaries/fips/immucore
var EmbeddedImmucoreFips []byte

//go:embed binaries/kcrypt-discovery-challenger
var EmbeddedKcryptChallenger []byte

//go:embed binaries/fips/kcrypt-discovery-challenger
var EmbeddedKcryptChallengerFips []byte

//go:embed binaries/kairos-cli
var EmbeddedKairosProvider []byte

//go:embed binaries/fips/kairos-cli
var EmbeddedKairosProviderFips []byte

//go:embed binaries/edgevpn
var EmbeddedEdgeVPN []byte

// EmbeddedConfigs contains the cloudconfigs that go into /system/oem
//
//go:embed cloudconfigs/*
var EmbeddedConfigs embed.FS

// EmbeddedAlpineInit contains the alpine initramfs config and scripts used to build the alpine initramfs
//
//go:embed alpineInit/*
var EmbeddedAlpineInit embed.FS

// SucUpgrade /usr/sbin/suc-upgrade is a script that is used to upgrade the system via k8s
// This has to be in the rootfs, cant be generated dynamically as upgrades use this
const SucUpgrade = `#!/bin/bash
set -x -e
HOST_DIR="${HOST_DIR:-/host}"
SUC_VERSION="0.0.0"

echo "SUC_VERSION: $SUC_VERSION"

get_version() {
    local file_path="$1"
    # shellcheck disable=SC1090
    source "$file_path"

    echo "${KAIROS_VERSION}-${KAIROS_SOFTWARE_VERSION_PREFIX}${KAIROS_SOFTWARE_VERSION}"
}

if [ "$FORCE" != "true" ]; then
    if [ -f "/etc/kairos-release" ]; then
      UPDATE_VERSION=$(get_version "/etc/kairos-release")
    else
      # shellcheck disable=SC1091
      UPDATE_VERSION=$(get_version "/etc/os-release" )
    fi

    if [ -f "${HOST_DIR}/etc/kairos-release" ]; then
      # shellcheck disable=SC1091
      CURRENT_VERSION=$(get_version "${HOST_DIR}/etc/kairos-release" )
    else
      # shellcheck disable=SC1091
      CURRENT_VERSION=$(get_version "${HOST_DIR}/etc/os-release" )
    fi

    if [ "$CURRENT_VERSION" == "$UPDATE_VERSION" ]; then
      echo Up to date
      echo "Current version: ${CURRENT_VERSION}"
      echo "Update version: ${UPDATE_VERSION}"
      exit 0
    fi
fi

mount --rbind "$HOST_DIR"/dev /dev
mount --rbind "$HOST_DIR"/run /run

recovery_mode=false
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --recovery) recovery_mode=true;;
    esac
    shift
done
if [ "$recovery_mode" = true ]; then
    kairos-agent upgrade --recovery --source dir:/
    exit 0 # no need to reboot when upgrading recovery
else
    kairos-agent upgrade --source dir:/
    nsenter -i -m -t 1 -- reboot
    exit 1
fi
`

// ReconcileScript /usr/bin/cos-setup-reconcile is a script that is used to run the kairos-agent in a loop for the reconcile service
// Mainly for alpine, for systemd distros we user a simple timer unit
// TODO: Check if we can move this into a supervisord script that runs every 300 seconds???
const ReconcileScript = `#!/bin/sh

SLEEP_TIME=${SLEEP_TIME:-360}

while :
do
    kairos-agent run-stage "reconcile"
    sleep "$SLEEP_TIME"
done`

// LogRotateConfig contains the logrotate config for the kairos-agent logs and such
const LogRotateConfig = `/var/log/kairos/*.log {
    create
    daily
    compress
    copytruncate
    missingok
    rotate 3
}`

// GrubCfg /etc/cos/grub.cfg is the default grub config that is used for the system boot
const GrubCfg = `set timeout=10

# set default values for kernel, initramfs
# Other files can still override this
set kernel=/boot/vmlinuz
set initramfs=/boot/initrd

# Load custom env file
set env_file="/grubenv"
search --no-floppy --file --set=env_blk "${env_file}"

# Load custom env file
set oem_env_file="/grub_oem_env"
search --no-floppy --file --set=oem_blk "${oem_env_file}"

# Load custom config file
set custom_menu="/grubmenu"
search --no-floppy --file --set=menu_blk "${custom_menu}"

# Load custom config file
set custom="/grubcustom"
search --no-floppy --file --set=custom_blk "${custom}"

if [ "${oem_blk}" ] ; then
  load_env -f "(${oem_blk})${oem_env_file}"
fi

if [ "${env_blk}" ] ; then
  load_env -f "(${env_blk})${env_file}"
fi

# Save default
if [ "${next_entry}" ]; then
  set default="${next_entry}"
  set selected_entry="${next_entry}"
  set next_entry=
  save_env -f "(${env_blk})${env_file}" next_entry
else
  set default="${saved_entry}"
fi

## Display a default menu entry if set
if [ "${default_menu_entry}" ]; then
  set display_name="${default_menu_entry}"
else
  set display_name="Kairos"
fi

## Set a default fallback if set
if [ "${default_fallback}" ]; then
  set fallback="${default_fallback}"
else
  set fallback="0 1 2"
fi

insmod all_video
insmod loopback
insmod squash4
insmod serial

loadfont unicode
if [ "${grub_platform}" = "efi" ]; then
    ## workaround for grub2-efi bug: https://bugs.launchpad.net/ubuntu/+source/grub2/+bug/1851311
    rmmod tpm
fi

menuentry "${display_name}" --id cos {
  search --no-floppy --label --set=root COS_STATE
  set img=/cOS/active.img
  set label=COS_ACTIVE
  loopback loop0 /$img
  set root=($root)
  source (loop0)/etc/cos/bootargs.cfg
  linux (loop0)$kernel $kernelcmd ${extra_cmdline} ${extra_active_cmdline}
  initrd (loop0)$initramfs
}

menuentry "${display_name} (fallback)" --id fallback {
  search --no-floppy --label --set=root COS_STATE
  set img=/cOS/passive.img
  set label=COS_PASSIVE
  loopback loop0 /$img
  set root=($root)
  source (loop0)/etc/cos/bootargs.cfg
  linux (loop0)$kernel $kernelcmd ${extra_cmdline} ${extra_passive_cmdline}
  initrd (loop0)$initramfs
}

menuentry "${display_name} recovery" --id recovery {
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
  linux (loop0)$kernel $kernelcmd ${extra_cmdline} ${extra_recovery_cmdline}
  initrd (loop0)$initramfs
}

menuentry "${display_name} state reset (auto)" --id statereset {
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
}

if [ "${menu_blk}" ]; then
  source "(${menu_blk})${custom_menu}"
fi

if [ "${custom_blk}" ]; then
  source "(${custom_blk})${custom}"
fi
`

// BootArgsCfg /etc/cos/bootargs.cfg is the script run under grub that chooses what cmdline to construct based on the distro
const BootArgsCfg = `function setSelinux {
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
`

// MOTD /etc/motd is the message of the day that is displayed when the user logs in
const MOTD = `Welcome to Kairos!

Refer to https://kairos.io for documentation.
`

// Issue /etc/issue.d/01-KAIROS is the issue that is displayed when the user logs in
const Issue = `                                                          
    _/    _/            _/                                
   _/  _/      _/_/_/      _/  _/_/    _/_/      _/_/_/   
  _/_/      _/    _/  _/  _/_/      _/    _/  _/_/        
 _/  _/    _/    _/  _/  _/        _/    _/      _/_/     
_/    _/    _/_/_/  _/  _/          _/_/    _/_/_/        
                                                          
                         
`

// Branding starts here

// ExtraGrubCfg /etc/kairos/branding/grubmenu.cfg is the extra grub config that is used for the system that can be
// overridden by the user to provide its own entries in grub
const ExtraGrubCfg = `
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
`

const InstallText = `Welcome to Kairos!
P2P device installation enrollment is starting.
A QR code will be displayed below.
In another machine, run "kairos register" with the QR code visible on screen,
or "kairos register <file>" to register the machine from a photo.
IF the qrcode is not displaying correctly,
try booting with another vga option from the boot cmdline (e.g. vga=791).

Press any key to abort pairing. To restart run 'kairos install'.

Starting in 5 seconds...
`

const ResetText = `Welcome to kairos!
The node will automatically reset its state in a few.

Press any key to abort this process. To restart run 'kairos reset'.

Starting in 60 seconds...
`

const RecoveryText = `Welcome to kairos recovery mode!
P2P device recovery mode is starting.
A QR code with a generated network token will be displayed below that can be used to connect 
over with "kairos bridge --qr-code-image /path/to/image.jpg" from another machine, 
further instruction will appear on the bridge CLI to connect over via SSH.
IF the qrcode is not displaying correctly,
try booting with another vga option from the boot cmdline (e.g. vga=791).

Press any key to abort recovery. To restart the process run 'kairos recovery'.
`

const InteractiveText = `Welcome to the Interactive installation.
Documentation is available at https://kairos.io.
`

// Branding ends here

// Services start here

// SystemdNetworkOnlineWaitOverride contains the service that is used to wait for the network to be online
// This makes the service wait for ANY interface to be online instead of the default which is to wait for all interfaces
const SystemdNetworkOnlineWaitOverride = `[Service]
ExecStart=
ExecStart=/usr/lib/systemd/systemd-networkd-wait-online --any`

// KairosAgentService contains the service that is used to run the kairos agent which waits for events
const KairosAgentService = `[Unit]
Description=kairos agent
After=cos-setup-network.service
Wants=network.target
[Service]
Restart=on-failure
RestartSec=5s
ExecStart=/usr/bin/kairos-agent start
[Install]
WantedBy=multi-user.target
`

// KairosRecoveryService contains the service that is used to run the kairos agent in recovery mode
const KairosRecoveryService = `[Unit]
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
`

// KairosResetService contains the service that is used to run the kairos agent in reset mode
const KairosResetService = `[Unit]
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
`

// KairosWebUIService contains the service that is used to run the kairos agent webui for web installs
const KairosWebUIService = `[Unit]
Description=kairos webui installer
After=sysinit.target
[Service]
ExecStart=/usr/bin/kairos-agent webui
TimeoutStopSec=10s
[Install]
WantedBy=multi-user.target
`

// KairosInstallerService contains the service that is used to run the kairos agent automated installer via cloudconfig/datasources
const KairosInstallerService = `[Unit]
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
WantedBy=multi-user.target
`

// KairosInteractiveService contains the service that is used to run the kairos agent interactive installer
const KairosInteractiveService = `[Unit]
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
WantedBy=multi-user.target
`

// Services end here

// K0s Services start here

const K0sControllerSystemd = `[Unit]
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
WantedBy=multi-user.target`

const K0sWorkerSystemd = `[Unit]
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
WantedBy=multi-user.target`

const K0sControllerOpenrc = `#!/sbin/openrc-run
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
}`

const K0sWorkerOpenrc = `#!/sbin/openrc-run
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
}`

// K0s Services end here
