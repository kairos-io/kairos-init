package validation_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var qemuProductPatternRe = regexp.MustCompile(`grep -iE "([^"]+)" /sys/class/dmi/id/product_name`)

var _ = Describe("Bundled VM cloudconfig", func() {
	It("recognizes QEMU default product names", func() {
		content, err := os.ReadFile(filepath.Join("..", "bundled", "cloudconfigs", "26_vm.yaml"))
		Expect(err).NotTo(HaveOccurred())

		var qemuPatterns []string
		for _, match := range qemuProductPatternRe.FindAllStringSubmatch(string(content), -1) {
			if strings.Contains(match[1], "qemu") {
				qemuPatterns = append(qemuPatterns, match[1])
			}
		}
		Expect(qemuPatterns).To(HaveLen(2), "systemd and OpenRC must use the QEMU detector")

		for _, pattern := range qemuPatterns {
			detector, err := regexp.Compile("(?i)" + pattern)
			Expect(err).NotTo(HaveOccurred())
			Expect(detector.MatchString("Standard PC (Q35 + ICH9, 2009)")).To(BeTrue())
			Expect(detector.MatchString("Standard PC (i440FX + PIIX, 1996)")).To(BeTrue())
			Expect(detector.MatchString("To Be Filled By O.E.M.")).To(BeFalse())
		}
	})
})
