package bundled

// JetsonQSPIScriptPath is where the QSPI update script is installed in the image.
const JetsonQSPIScriptPath = "/usr/sbin/kairos-jetson-qspi-update"

// JetsonQSPIScript checks the board's QSPI boot firmware version against the L4T version
// baked into the image, and stages a UEFI capsule update when the image is newer.
//
// It deliberately does not use `dpkg-reconfigure nvidia-l4t-bootloader`: that package's
// postinst also overwrites the ESP's EFI/BOOT/BOOTAA64.efi with NVIDIA's L4TLauncher
// (which would replace Kairos's own bootloader) and rewrites /boot/extlinux/extlinux.conf.
// We reproduce only the two operations that actually stage the capsule.
//
// See kairos-io/kairos#4228.
const JetsonQSPIScript = `#!/bin/bash
set -euo pipefail

# All roots are overridable so the logic can be tested without hardware.
ESRT_FW_VERSION_FILE="${ESRT_FW_VERSION_FILE:-/sys/firmware/efi/esrt/entries/entry0/fw_version}"
NV_BOOT_CONTROL_CONF="${NV_BOOT_CONTROL_CONF:-/etc/nv_boot_control.conf}"
OTA_PACKAGE_DIR="${OTA_PACKAGE_DIR:-/opt/ota_package}"
EFIVARS_DIR="${EFIVARS_DIR:-/sys/firmware/efi/efivars}"
ESP_MOUNT_DIR="${ESP_MOUNT_DIR:-/run/kairos-esp}"
ESP_LABEL="${ESP_LABEL:-COS_GRUB}"

# Below this the board cannot be updated in-band and needs a USB host flash.
FACTORY_QSPI_VER="2490368" # 38.0.0

log()  { echo "[kairos-qspi] $*"; }
fail() { echo "[kairos-qspi] ERROR: $*" >&2; exit 1; }

# encode_version 39.2.0 -> 2556416
encode_version() {
	local v="$1" branch major minor
	branch="$(echo "${v}" | cut -d. -f1)"
	major="$(echo "${v}" | cut -d. -f2)"
	minor="$(echo "${v}" | cut -d. -f3)"
	[ -n "${minor}" ] || minor=0
	echo $(( (branch << 16) | (major << 8) | minor ))
}

# Hidden entrypoint for tests.
if [ "${1:-}" = "__encode_version" ]; then
	encode_version "$2"
	exit 0
fi
`
