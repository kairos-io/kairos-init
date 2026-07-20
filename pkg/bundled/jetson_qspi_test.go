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
})
