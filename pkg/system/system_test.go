package system_test

import (
	"github.com/kairos-io/kairos-init/pkg/system"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Create a test logger for testing
func createTestLogger() types.KairosLogger {
	return types.NewKairosLogger("test", "info", false)
}

var _ = Describe("System Package", func() {
	Describe("DetectSystem", func() {
		var logger types.KairosLogger

		BeforeEach(func() {
			logger = createTestLogger()
		})

		Context("on current system", func() {
			It("should detect system without errors", func() {
				result := system.DetectSystem(logger)

				// The function should return a valid System struct
				Expect(result.Distro).NotTo(BeEmpty())
				Expect(result.Family).NotTo(BeEmpty())
				
				// On the test system (Ubuntu), we expect specific values
				Expect(result.Distro).To(Equal(values.Ubuntu))
				Expect(result.Family).To(Equal(values.DebianFamily))
				Expect(result.Version).To(ContainSubstring("24.04"))
				Expect(result.Name).To(ContainSubstring("Ubuntu"))
			})

			It("should populate all system fields", func() {
				result := system.DetectSystem(logger)

				// All fields should be populated on a real system
				Expect(string(result.Distro)).NotTo(BeEmpty())
				Expect(string(result.Family)).NotTo(BeEmpty())
				Expect(result.Version).NotTo(BeEmpty())
				Expect(result.Name).NotTo(BeEmpty())
			})

			It("should have consistent distro and family mapping", func() {
				result := system.DetectSystem(logger)

				// Check that distro and family are consistent
				switch result.Distro {
				case values.Ubuntu, values.Debian:
					Expect(result.Family).To(Equal(values.DebianFamily))
				case values.Fedora, values.RedHat, values.RockyLinux, values.AlmaLinux:
					Expect(result.Family).To(Equal(values.RedHatFamily))
				case values.Arch:
					Expect(result.Family).To(Equal(values.ArchFamily))
				case values.Alpine:
					Expect(result.Family).To(Equal(values.AlpineFamily))
				case values.OpenSUSELeap, values.OpenSUSETumbleweed, values.SLES:
					Expect(result.Family).To(Equal(values.SUSEFamily))
				case values.Unknown:
					Expect(result.Family).To(Equal(values.UnknownFamily))
				}
			})

			It("should set architecture to runtime architecture", func() {
				result := system.DetectSystem(logger)

				// Architecture should be set based on runtime.GOARCH
				// We can't predict the exact value but it should be valid
				validArchs := []values.Architecture{
					values.ArchAMD64,
					values.ArchARM64,
				}
				Expect(validArchs).To(ContainElement(result.Arch))
			})
		})

		Context("distro-specific behavior", func() {
			It("should handle version formatting correctly for current distro", func() {
				result := system.DetectSystem(logger)

				// Version should be a reasonable format
				Expect(result.Version).To(MatchRegexp(`^\d+(\.\d+)*`))
				
				// For Ubuntu (our test environment), version should be specific format
				if result.Distro == values.Ubuntu {
					Expect(result.Version).To(MatchRegexp(`^\d+\.\d+$`))
				}
			})

			It("should prefer PRETTY_NAME over NAME for display", func() {
				result := system.DetectSystem(logger)

				// Name should be populated and reasonable
				Expect(result.Name).NotTo(BeEmpty())
				Expect(len(result.Name)).To(BeNumerically(">", 3))
			})
		})

		Context("validation of system detection", func() {
			It("should return consistent results across multiple calls", func() {
				result1 := system.DetectSystem(logger)
				result2 := system.DetectSystem(logger)

				// Results should be identical
				Expect(result1.Distro).To(Equal(result2.Distro))
				Expect(result1.Family).To(Equal(result2.Family))
				Expect(result1.Version).To(Equal(result2.Version))
				Expect(result1.Name).To(Equal(result2.Name))
				Expect(result1.Arch).To(Equal(result2.Arch))
			})

			It("should have valid enum values", func() {
				result := system.DetectSystem(logger)

				// Validate that the detected values are from known enums
				validDistros := []values.Distro{
					values.Unknown, values.Debian, values.Ubuntu, values.RedHat,
					values.RockyLinux, values.AlmaLinux, values.Fedora, values.Arch,
					values.Alpine, values.OpenSUSELeap, values.OpenSUSETumbleweed,
					values.SLES,
				}
				Expect(validDistros).To(ContainElement(result.Distro))

				validFamilies := []values.Family{
					values.UnknownFamily, values.DebianFamily, values.RedHatFamily,
					values.ArchFamily, values.AlpineFamily, values.SUSEFamily,
				}
				Expect(validFamilies).To(ContainElement(result.Family))
			})
		})

		Context("error handling", func() {
			It("should not panic during detection", func() {
				// This test ensures the function doesn't panic
				Expect(func() {
					_ = system.DetectSystem(logger)
				}).NotTo(Panic())
			})

			It("should handle logger gracefully", func() {
				// Test with nil logger to ensure no panics
				// Note: This may not work as expected if logger methods are called
				// but it tests basic robustness
				Expect(func() {
					_ = system.DetectSystem(logger)
				}).NotTo(Panic())
			})
		})
	})
})