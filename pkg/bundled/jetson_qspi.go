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

# --- board gate ---------------------------------------------------------------
if [ ! -r "${NV_BOOT_CONTROL_CONF}" ]; then
	log "no ${NV_BOOT_CONTROL_CONF}, not a Jetson board with boot control; skipping"
	exit 0
fi

chipid="$(awk '/^CHIPID/ {print $2}' "${NV_BOOT_CONTROL_CONF}" | head -1)"
case "${chipid}" in
	0x26) payload_subdir="t26x" ;;
	*)    log "chip id ${chipid} is not a Thor board; skipping"; exit 0 ;;
esac

# --- current firmware version -------------------------------------------------
if [ ! -r "${ESRT_FW_VERSION_FILE}" ]; then
	fail "cannot read current firmware version from ${ESRT_FW_VERSION_FILE}"
fi
current_qspi_ver="$(tr -d '[:space:]' < "${ESRT_FW_VERSION_FILE}")"
if ! [ "${current_qspi_ver}" -eq "${current_qspi_ver}" ] 2>/dev/null; then
	fail "cannot read current firmware version from ${ESRT_FW_VERSION_FILE}"
fi

# --- expected version from the image ------------------------------------------
if [ -n "${KAIROS_QSPI_IMAGE_VERSION:-}" ]; then
	image_pkg_ver="${KAIROS_QSPI_IMAGE_VERSION}"
else
	image_pkg_ver="$(dpkg-query -W -f='${Version}' nvidia-l4t-bootloader 2>/dev/null | cut -d- -f1)"
fi
[ -n "${image_pkg_ver}" ] || fail "cannot determine the L4T bootloader version in this image"
image_ver="$(encode_version "${image_pkg_ver}")"

log "board firmware: ${current_qspi_ver}, image L4T: ${image_ver} (${image_pkg_ver})"

# --- decision -----------------------------------------------------------------
if [ "${current_qspi_ver}" -lt "${FACTORY_QSPI_VER}" ]; then
	fail "board firmware ${current_qspi_ver} is below the supported floor ${FACTORY_QSPI_VER} (38.0.0).
       This board cannot be updated from a running system and needs a USB host flash.
       See https://canonical-ubuntu-for-jetson.readthedocs-hosted.com/latest/how-to/flash/"
fi

if [ "${current_qspi_ver}" -gt "${image_ver}" ]; then
	fail "board firmware ${current_qspi_ver} is newer than this image (${image_ver}).
       UEFI capsule update cannot downgrade firmware, so this board would not boot.
       Use a Kairos image built for L4T ${current_qspi_ver}, or reflash the board.
       See https://github.com/kairos-io/kairos/issues/4228"
fi

if [ "${current_qspi_ver}" -eq "${image_ver}" ]; then
	log "board firmware already matches the image; nothing to do"
	exit 0
fi

log "board firmware is older than the image; staging capsule update"
`
