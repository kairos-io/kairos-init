package bundled_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kairos-io/kairos-init/pkg/bundled"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// writeScript drops the bundled script into a temp dir and returns its path.
func writeScript() string {
	dir := GinkgoT().TempDir()
	path := filepath.Join(dir, "kairos-jetson-qspi-update")
	Expect(os.WriteFile(path, []byte(bundled.JetsonQSPIScript), 0o755)).To(Succeed())
	return path
}

var _ = Describe("Jetson QSPI script", func() {
	Describe("version encoding", func() {
		DescribeTable("encodes L4T versions as ESRT integers",
			func(version string, expected string) {
				out, err := exec.Command(writeScript(), "__encode_version", version).Output()
				Expect(err).ToNot(HaveOccurred())
				Expect(strings.TrimSpace(string(out))).To(Equal(expected))
			},
			Entry("floor version", "38.0.0", "2490368"),
			Entry("previous Thor pin", "38.4.0", "2491392"),
			Entry("current Thor pin", "39.2.0", "2556416"),
			Entry("two-component version", "39.2", "2556416"),
		)
	})

	Describe("decision logic", func() {
		// env builds a fixture environment: an ESRT file holding the board's current
		// firmware version, and an nv_boot_control.conf declaring a Thor chip id.
		env := func(currentQSPI, chipID string) (string, []string) {
			dir := GinkgoT().TempDir()
			esrt := filepath.Join(dir, "fw_version")
			Expect(os.WriteFile(esrt, []byte(currentQSPI+"\n"), 0o644)).To(Succeed())
			conf := filepath.Join(dir, "nv_boot_control.conf")
			Expect(os.WriteFile(conf, []byte("TNSPEC 3834-0008\nCHIPID "+chipID+"\n"), 0o644)).To(Succeed())
			return dir, append(os.Environ(),
				"ESRT_FW_VERSION_FILE="+esrt,
				"NV_BOOT_CONTROL_CONF="+conf,
				"KAIROS_QSPI_DRY_RUN=1",
			)
		}

		run := func(envv []string, imageVersion string) (string, error) {
			cmd := exec.Command(writeScript())
			cmd.Env = append(envv, "KAIROS_QSPI_IMAGE_VERSION="+imageVersion)
			out, err := cmd.CombinedOutput()
			return string(out), err
		}

		It("stages a capsule when the image is newer than the board", func() {
			_, envv := env("2490368", "0x26") // board 38.0.0
			out, err := run(envv, "39.2.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("staging capsule"))
		})

		It("does nothing when versions match", func() {
			_, envv := env("2556416", "0x26")
			out, err := run(envv, "39.2.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("already matches"))
			Expect(out).ToNot(ContainSubstring("staging capsule"))
		})

		It("aborts when the board firmware is newer than the image", func() {
			_, envv := env("2556416", "0x26") // board 39.2.0
			out, err := run(envv, "38.4.0")   // image 38.4.0
			Expect(err).To(HaveOccurred())
			Expect(out).To(ContainSubstring("newer than this image"))
			Expect(out).To(ContainSubstring("2556416"))
			Expect(out).To(ContainSubstring("2491392"))
		})

		It("aborts when the board is below the 38.0.0 floor", func() {
			_, envv := env("2424832", "0x26") // 37.0.0
			out, err := run(envv, "39.2.0")
			Expect(err).To(HaveOccurred())
			Expect(out).To(ContainSubstring("USB host flash"))
		})

		It("skips silently on non-Thor chip ids", func() {
			_, envv := env("2490368", "0x23") // Orin
			out, err := run(envv, "39.2.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(ContainSubstring("not a Thor board"))
		})

		It("aborts when ESRT is unreadable", func() {
			dir := GinkgoT().TempDir()
			conf := filepath.Join(dir, "nv_boot_control.conf")
			Expect(os.WriteFile(conf, []byte("CHIPID 0x26\n"), 0o644)).To(Succeed())
			cmd := exec.Command(writeScript())
			cmd.Env = append(os.Environ(),
				"ESRT_FW_VERSION_FILE="+filepath.Join(dir, "absent"),
				"NV_BOOT_CONTROL_CONF="+conf,
				"KAIROS_QSPI_DRY_RUN=1",
				"KAIROS_QSPI_IMAGE_VERSION=39.2.0",
			)
			out, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("cannot read current firmware version"))
		})

		// failingDpkgQueryDir writes a "dpkg-query" shim that always fails and
		// returns its directory, so prepending it to PATH shadows any real
		// dpkg-query without disturbing the other coreutils (awk, cut, tr, head)
		// the script depends on for the steps before the bootloader-version check.
		failingDpkgQueryDir := func() string {
			dir := GinkgoT().TempDir()
			shim := filepath.Join(dir, "dpkg-query")
			Expect(os.WriteFile(shim, []byte("#!/bin/sh\nexit 1\n"), 0o755)).To(Succeed())
			return dir
		}

		It("aborts with a clear message when the bootloader version cannot be determined on the real (non-test) path", func() {
			// Leave KAIROS_QSPI_IMAGE_VERSION unset so the script takes the
			// production dpkg-query branch, not the test seam. Shadow
			// dpkg-query with a failing shim so the command substitution fails
			// the way it would on an image without nvidia-l4t-bootloader installed.
			_, envv := env("2490368", "0x26")
			for i, e := range envv {
				if strings.HasPrefix(e, "PATH=") {
					envv[i] = "PATH=" + failingDpkgQueryDir() + string(os.PathListSeparator) + strings.TrimPrefix(e, "PATH=")
				}
			}

			cmd := exec.Command(writeScript())
			cmd.Env = envv
			out, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("cannot determine the L4T bootloader version in this image"))
		})

		It("aborts with a clear message when the image L4T version is malformed", func() {
			_, envv := env("2490368", "0x26")
			envv = append(envv, "KAIROS_QSPI_IMAGE_VERSION=not-a-version")
			cmd := exec.Command(writeScript())
			cmd.Env = envv
			out, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("cannot parse"))
			Expect(string(out)).To(ContainSubstring("not-a-version"))
		})
	})
})
