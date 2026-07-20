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

chipid="$(awk '/^CHIPID/ {print $2}' "${NV_BOOT_CONTROL_CONF}" | head -1)" ||
	fail "cannot determine the chip id from ${NV_BOOT_CONTROL_CONF}"
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
	image_pkg_ver="$(dpkg-query -W -f='${Version}' nvidia-l4t-bootloader 2>/dev/null | cut -d- -f1)" || image_pkg_ver=""
fi
[ -n "${image_pkg_ver}" ] || fail "cannot determine the L4T bootloader version in this image"
image_ver="$(encode_version "${image_pkg_ver}")" ||
	fail "cannot parse the L4T bootloader version '${image_pkg_ver}' found in this image"

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

# A dry run must exit before anything that requires the real image layout
# (e.g. OTA_PACKAGE_DIR), since the decision-logic tests exercise this seam
# without staging fixtures in place.
if [ "${KAIROS_QSPI_DRY_RUN:-0}" = "1" ]; then
	log "dry run; not staging"
	exit 0
fi

# --- locate the payload -------------------------------------------------------
capsule_src="${OTA_PACKAGE_DIR}/${payload_subdir}/TEGRA_BL_3834_agx.Cap"
[ -r "${capsule_src}" ] || fail "capsule payload not found at ${capsule_src}"
capsule_size="$(stat -c %s "${capsule_src}")" || fail "cannot stat ${capsule_src}"
capsule_kb=$(( (capsule_size + 1023) / 1024 ))

# --- mount the ESP ------------------------------------------------------------
# The after-install-chroot hook does not bind-mount the ESP into the chroot, so we
# mount it ourselves by label. /dev is bind-mounted, so the device is resolvable.
mkdir -p "${ESP_MOUNT_DIR}" || fail "cannot create ${ESP_MOUNT_DIR}"
if [ "${KAIROS_QSPI_SKIP_MOUNT:-0}" != "1" ]; then
	esp_dev="$(blkid -L "${ESP_LABEL}")" || fail "cannot find an ESP labelled ${ESP_LABEL}"
	mount "${esp_dev}" "${ESP_MOUNT_DIR}" || fail "cannot mount ${esp_dev} at ${ESP_MOUNT_DIR}"
	trap 'umount "${ESP_MOUNT_DIR}" || true' EXIT
fi

# --- capacity check -----------------------------------------------------------
if [ -n "${KAIROS_QSPI_FORCE_FREE_KB:-}" ]; then
	# Test hook: let tests simulate a full ESP without needing real block devices.
	free_kb="${KAIROS_QSPI_FORCE_FREE_KB}"
else
	free_kb="$(df -Pk "${ESP_MOUNT_DIR}" | awk 'NR==2 {print $4}')" ||
		fail "cannot determine free space on ${ESP_MOUNT_DIR}"
fi
# Use <= rather than < : filesystem/directory-entry overhead means free space
# exactly equal to the payload size is not a safe margin to write into.
if [ "${free_kb}" -lt "${capsule_kb}" ]; then
	fail "not enough free space on the ESP: need ${capsule_kb} KiB, have ${free_kb} KiB.
       The Kairos ESP may need to be larger for this board."
fi

# --- stage --------------------------------------------------------------------
capsule_dir="${ESP_MOUNT_DIR}/EFI/UpdateCapsule"
mkdir -p "${capsule_dir}" || fail "cannot create ${capsule_dir}"
cp "${capsule_src}" "${capsule_dir}/" || fail "cannot copy ${capsule_src} to ${capsule_dir}"
log "staged $(basename "${capsule_src}") on the ESP"

# Set bit 2 of OsIndications (EFI_OS_INDICATIONS_FILE_CAPSULE_DELIVERY_SUPPORTED).
# UEFI applies the capsule on the next boot, before Linux starts.
osind="${EFIVARS_DIR}/OsIndications-8be4df61-93ca-11d2-aa0d-00e098032b8c"
mkdir -p "${EFIVARS_DIR}" || fail "cannot create ${EFIVARS_DIR}"
printf '\x07\x00\x00\x00\x04\x00\x00\x00\x00\x00\x00\x00' > "${osind}" ||
	fail "cannot write ${osind}"

log "firmware will be updated on the next boot"
`
