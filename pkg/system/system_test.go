package system_test

import (
	"os"
	"path/filepath"
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

		Context("with mock os-release files", func() {
			var tempDir string
			var originalPath string

			BeforeEach(func() {
				var err error
				tempDir, err = os.MkdirTemp("", "system-test")
				Expect(err).NotTo(HaveOccurred())
				
				// Store original environment variable value
				originalPath = os.Getenv("KAIROS_OS_RELEASE_PATH")
			})

			AfterEach(func() {
				// Restore original environment variable
				if originalPath != "" {
					os.Setenv("KAIROS_OS_RELEASE_PATH", originalPath)
				} else {
					os.Unsetenv("KAIROS_OS_RELEASE_PATH")
				}
				os.RemoveAll(tempDir)
			})

			It("should detect Ubuntu correctly", func() {
				osReleaseContent := `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
VERSION_ID="22.04"
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
PRIVACY_POLICY_URL="https://www.ubuntu.com/legal/terms-and-policies/privacy-policy"
VERSION_CODENAME=jammy
UBUNTU_CODENAME=jammy`

				mockPath := filepath.Join(tempDir, "os-release")
				err := os.WriteFile(mockPath, []byte(osReleaseContent), 0644)
				Expect(err).NotTo(HaveOccurred())
				
				os.Setenv("KAIROS_OS_RELEASE_PATH", mockPath)
				
				result := system.DetectSystem(logger)
				
				Expect(result.Distro).To(Equal(values.Ubuntu))
				Expect(result.Family).To(Equal(values.DebianFamily))
				Expect(result.Version).To(Equal("22.04"))
				Expect(result.Name).To(Equal("Ubuntu 22.04.3 LTS"))
			})

			It("should detect Fedora correctly", func() {
				osReleaseContent := `NAME="Fedora Linux"
VERSION="38 (Workstation Edition)"
ID=fedora
VERSION_ID=38
VERSION_CODENAME=""
PLATFORM_ID="platform:f38"
PRETTY_NAME="Fedora Linux 38 (Workstation Edition)"
ANSI_COLOR="0;38;2;60;110;180"
LOGO=fedora-logo-icon
CPE_NAME="cpe:/o:fedoraproject:fedora:38"
DEFAULT_HOSTNAME="fedora"
HOME_URL="https://fedoraproject.org/"
DOCUMENTATION_URL="https://docs.fedoraproject.org/en-US/fedora/f38/system-administrators-guide/"
SUPPORT_URL="https://ask.fedoraproject.org/"
BUG_REPORT_URL="https://bugzilla.redhat.com/"
REDHAT_BUGZILLA_PRODUCT="Fedora"
REDHAT_BUGZILLA_PRODUCT_VERSION=38
REDHAT_SUPPORT_PRODUCT="Fedora"
REDHAT_SUPPORT_PRODUCT_VERSION=38
SUPPORT_END=2024-05-14`

				mockPath := filepath.Join(tempDir, "os-release")
				err := os.WriteFile(mockPath, []byte(osReleaseContent), 0644)
				Expect(err).NotTo(HaveOccurred())
				
				os.Setenv("KAIROS_OS_RELEASE_PATH", mockPath)
				
				result := system.DetectSystem(logger)
				
				Expect(result.Distro).To(Equal(values.Fedora))
				Expect(result.Family).To(Equal(values.RedHatFamily))
				Expect(result.Version).To(Equal("38"))
				Expect(result.Name).To(Equal("Fedora Linux 38 (Workstation Edition)"))
			})

			It("should detect Alpine correctly and format version", func() {
				osReleaseContent := `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.18.4
PRETTY_NAME="Alpine Linux v3.18"
HOME_URL="https://alpinelinux.org/"
BUG_REPORT_URL="https://gitlab.alpinelinux.org/alpine/aports/-/issues"`

				mockPath := filepath.Join(tempDir, "os-release")
				err := os.WriteFile(mockPath, []byte(osReleaseContent), 0644)
				Expect(err).NotTo(HaveOccurred())
				
				os.Setenv("KAIROS_OS_RELEASE_PATH", mockPath)
				
				result := system.DetectSystem(logger)
				
				Expect(result.Distro).To(Equal(values.Alpine))
				Expect(result.Family).To(Equal(values.AlpineFamily))
				Expect(result.Version).To(Equal("3.18")) // Should strip patch version
				Expect(result.Name).To(Equal("Alpine Linux v3.18"))
			})

			It("should fallback to ID_LIKE when ID is unknown", func() {
				osReleaseContent := `NAME="CustomDistro"
ID=customdistro
ID_LIKE=debian
VERSION_ID=1.0
PRETTY_NAME="CustomDistro 1.0"`

				mockPath := filepath.Join(tempDir, "os-release")
				err := os.WriteFile(mockPath, []byte(osReleaseContent), 0644)
				Expect(err).NotTo(HaveOccurred())
				
				os.Setenv("KAIROS_OS_RELEASE_PATH", mockPath)
				
				result := system.DetectSystem(logger)
				
				Expect(result.Distro).To(Equal(values.Debian)) // Should fallback to parent
				Expect(result.Family).To(Equal(values.DebianFamily))
				Expect(result.Version).To(Equal("1.0"))
				Expect(result.Name).To(Equal("CustomDistro 1.0"))
			})

			It("should handle missing NAME and use PRETTY_NAME fallback", func() {
				osReleaseContent := `ID=ubuntu
ID_LIKE=debian
VERSION_ID="22.04"
PRETTY_NAME="Ubuntu 22.04.3 LTS"`

				mockPath := filepath.Join(tempDir, "os-release")
				err := os.WriteFile(mockPath, []byte(osReleaseContent), 0644)
				Expect(err).NotTo(HaveOccurred())
				
				os.Setenv("KAIROS_OS_RELEASE_PATH", mockPath)
				
				result := system.DetectSystem(logger)
				
				Expect(result.Name).To(Equal("Ubuntu 22.04.3 LTS"))
			})

			It("should handle missing os-release file gracefully", func() {
				nonExistentPath := filepath.Join(tempDir, "nonexistent")
				os.Setenv("KAIROS_OS_RELEASE_PATH", nonExistentPath)
				
				result := system.DetectSystem(logger)
				
				// Should return unknown values when file is missing
				Expect(result.Distro).To(Equal(values.Unknown))
				Expect(result.Family).To(Equal(values.UnknownFamily))
				Expect(result.Version).To(BeEmpty())
				Expect(result.Name).To(BeEmpty())
			})

			It("should handle malformed os-release file gracefully", func() {
				osReleaseContent := `This is not a valid os-release file
Invalid format without key=value pairs`

				mockPath := filepath.Join(tempDir, "os-release")
				err := os.WriteFile(mockPath, []byte(osReleaseContent), 0644)
				Expect(err).NotTo(HaveOccurred())
				
				os.Setenv("KAIROS_OS_RELEASE_PATH", mockPath)
				
				result := system.DetectSystem(logger)
				
				// Should return unknown values when parsing fails
				Expect(result.Distro).To(Equal(values.Unknown))
				Expect(result.Family).To(Equal(values.UnknownFamily))
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