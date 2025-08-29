package values_test

import (
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Create a test logger for testing
func createTestLogger() types.KairosLogger {
	return types.NewKairosLogger("test", "info", false)
}

var _ = Describe("Values Package", func() {
	Describe("String methods", func() {
		Context("Architecture", func() {
			It("should convert to string correctly", func() {
				arch := values.ArchAMD64
				Expect(arch.String()).To(Equal("amd64"))

				arch = values.ArchARM64
				Expect(arch.String()).To(Equal("arm64"))

				arch = values.ArchCommon
				Expect(arch.String()).To(Equal("common"))
			})
		})

		Context("Distro", func() {
			It("should convert to string correctly", func() {
				distro := values.Ubuntu
				Expect(distro.String()).To(Equal("ubuntu"))

				distro = values.Debian
				Expect(distro.String()).To(Equal("debian"))

				distro = values.RedHat
				Expect(distro.String()).To(Equal("rhel"))

				distro = values.Unknown
				Expect(distro.String()).To(Equal("unknown"))
			})
		})

		Context("Family", func() {
			It("should convert to string correctly", func() {
				family := values.DebianFamily
				Expect(family.String()).To(Equal("debian"))

				family = values.RedHatFamily
				Expect(family.String()).To(Equal("redhat"))

				family = values.UnknownFamily
				Expect(family.String()).To(Equal("unknown"))
			})
		})

		Context("Model", func() {
			It("should convert to string correctly", func() {
				model := values.Generic
				Expect(model.String()).To(Equal("generic"))

				model = values.Rpi3
				Expect(model.String()).To(Equal("rpi3"))

				model = values.Rpi4
				Expect(model.String()).To(Equal("rpi4"))

				model = values.AgxOrin
				Expect(model.String()).To(Equal("agx-orin"))
			})
		})
	})

	Describe("GetTemplateParams", func() {
		It("should return correct template parameters", func() {
			system := values.System{
				Name:    "test-system",
				Distro:  values.Ubuntu,
				Family:  values.DebianFamily,
				Version: "20.04",
				Arch:    values.ArchAMD64,
			}

			params := values.GetTemplateParams(system)

			Expect(params).To(HaveLen(4))
			Expect(params["distro"]).To(Equal("ubuntu"))
			Expect(params["version"]).To(Equal("20.04"))
			Expect(params["arch"]).To(Equal("amd64"))
			Expect(params["family"]).To(Equal("debian"))
		})

		It("should handle different system configurations", func() {
			system := values.System{
				Name:    "rhel-system",
				Distro:  values.Fedora,
				Family:  values.RedHatFamily,
				Version: "38",
				Arch:    values.ArchARM64,
			}

			params := values.GetTemplateParams(system)

			Expect(params["distro"]).To(Equal("fedora"))
			Expect(params["version"]).To(Equal("38"))
			Expect(params["arch"]).To(Equal("arm64"))
			Expect(params["family"]).To(Equal("redhat"))
		})
	})

	Describe("StepsInfo", func() {
		It("should return all steps in alphabetical order", func() {
			steps := values.StepsInfo()

			Expect(steps).NotTo(BeEmpty())
			Expect(len(steps)).To(BeNumerically(">=", 10)) // We have at least 10+ steps

			// Check that they are sorted
			for i := 1; i < len(steps); i++ {
				Expect(steps[i].Key >= steps[i-1].Key).To(BeTrue())
			}

			// Check that known steps are present
			stepKeys := make(map[string]bool)
			for _, step := range steps {
				stepKeys[step.Key] = true
			}

			Expect(stepKeys[values.InitStage]).To(BeTrue())
			Expect(stepKeys[values.InstallStage]).To(BeTrue())
			Expect(stepKeys[values.InstallPackagesStep]).To(BeTrue())
			Expect(stepKeys[values.KernelStep]).To(BeTrue())
		})

		It("should have descriptions for all steps", func() {
			steps := values.StepsInfo()

			for _, step := range steps {
				Expect(step.Key).NotTo(BeEmpty())
				Expect(step.Value).NotTo(BeEmpty())
			}
		})
	})

	Describe("GetStepNames", func() {
		It("should return all step names", func() {
			stepNames := values.GetStepNames()

			Expect(stepNames).NotTo(BeEmpty())
			Expect(len(stepNames)).To(BeNumerically(">=", 10))

			// Check that known steps are present
			Expect(stepNames).To(ContainElement(values.InitStage))
			Expect(stepNames).To(ContainElement(values.InstallStage))
			Expect(stepNames).To(ContainElement(values.KernelStep))
		})

		It("should return the same number of steps as StepsInfo", func() {
			stepNames := values.GetStepNames()
			stepsInfo := values.StepsInfo()

			Expect(len(stepNames)).To(Equal(len(stepsInfo)))
		})
	})

	Describe("PackageListToTemplate", func() {
		var logger types.KairosLogger

		BeforeEach(func() {
			logger = createTestLogger()
		})

		It("should process template packages correctly", func() {
			packages := []string{
				"linux-image-{{.arch}}",
				"linux-headers-{{.version}}",
				"{{.distro}}-special-package",
			}
			params := map[string]string{
				"arch":    "amd64",
				"version": "5.4.0",
				"distro":  "ubuntu",
			}

			result, err := values.PackageListToTemplate(packages, params, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(Equal("linux-image-amd64"))
			Expect(result[1]).To(Equal("linux-headers-5.4.0"))
			Expect(result[2]).To(Equal("ubuntu-special-package"))
		})

		It("should handle packages without templates", func() {
			packages := []string{"vim", "curl", "wget"}
			params := map[string]string{"arch": "amd64"}

			result, err := values.PackageListToTemplate(packages, params, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(packages))
		})

		It("should return error for invalid templates", func() {
			packages := []string{"package-{{.invalid"}
			params := map[string]string{}

			_, err := values.PackageListToTemplate(packages, params, logger)

			Expect(err).To(HaveOccurred())
		})

		It("should handle template execution that succeeds with empty result", func() {
			// Test a template that parses but results in empty output
			packages := []string{"{{if .nonexistent}}package{{end}}"}
			params := map[string]string{"arch": "amd64"}

			result, err := values.PackageListToTemplate(packages, params, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal("")) // Empty result from conditional template
		})
	})

	Describe("FilterPackagesOnConstraint", func() {
		var (
			logger types.KairosLogger
			system values.System
		)

		BeforeEach(func() {
			logger = createTestLogger()
			system = values.System{
				Distro:  values.Ubuntu,
				Family:  values.DebianFamily,
				Version: "20.04.1",
				Arch:    values.ArchAMD64,
			}
		})

		It("should filter packages based on version constraints", func() {
			versionMap := []values.VersionMap{
				{
					">=20.04": {"package-new"},
					"<18.04":  {"package-old"},
					values.Common: {"package-common"},
				},
			}

			result := values.FilterPackagesOnConstraint(system, logger, versionMap)

			Expect(result).To(ContainElement("package-common"))
			Expect(result).To(ContainElement("package-new"))
			Expect(result).NotTo(ContainElement("package-old"))
		})

		It("should handle OR constraints", func() {
			versionMap := []values.VersionMap{
				{
					">=20.04||<=18.04": {"package-either"},
					values.Common:      {"package-common"},
				},
			}

			result := values.FilterPackagesOnConstraint(system, logger, versionMap)

			Expect(result).To(ContainElement("package-common"))
			Expect(result).To(ContainElement("package-either"))
		})

		It("should handle invalid version strings gracefully", func() {
			systemBadVersion := values.System{
				Distro:  values.Ubuntu,
				Version: "invalid-version",
				Arch:    values.ArchAMD64,
			}

			versionMap := []values.VersionMap{
				{
					">=20.04": {"package-new"},
					values.Common: {"package-common"},
				},
			}

			result := values.FilterPackagesOnConstraint(systemBadVersion, logger, versionMap)

			// Should return empty result when version parsing fails
			Expect(result).To(BeEmpty())
		})

		It("should always include common packages", func() {
			versionMap := []values.VersionMap{
				{
					">=50.00":     {"package-future"}, // Won't match
					values.Common: {"package-common"},
				},
			}

			result := values.FilterPackagesOnConstraint(system, logger, versionMap)

			Expect(result).To(ContainElement("package-common"))
			Expect(result).NotTo(ContainElement("package-future"))
		})
	})

	Describe("GetPackages", func() {
		var logger types.KairosLogger

		BeforeEach(func() {
			logger = createTestLogger()
		})

		It("should return packages for Ubuntu system", func() {
			system := values.System{
				Distro:  values.Ubuntu,
				Family:  values.DebianFamily,
				Version: "20.04",
				Arch:    values.ArchAMD64,
			}

			result, err := values.GetPackages(system, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())

			// Should include common packages
			Expect(result).To(ContainElement("sudo"))
			Expect(result).To(ContainElement("rsync"))
			Expect(result).To(ContainElement("jq"))
		})

		It("should return packages for different architectures", func() {
			systemAMD64 := values.System{
				Distro:  values.Ubuntu,
				Family:  values.DebianFamily,
				Version: "20.04",
				Arch:    values.ArchAMD64,
			}

			systemARM64 := values.System{
				Distro:  values.Ubuntu,
				Family:  values.DebianFamily,
				Version: "20.04",
				Arch:    values.ArchARM64,
			}

			resultAMD64, err := values.GetPackages(systemAMD64, logger)
			Expect(err).NotTo(HaveOccurred())

			resultARM64, err := values.GetPackages(systemARM64, logger)
			Expect(err).NotTo(HaveOccurred())

			// Both should include common packages
			for _, pkg := range values.CommonPackages {
				Expect(resultAMD64).To(ContainElement(pkg))
				Expect(resultARM64).To(ContainElement(pkg))
			}
		})
	})

	Describe("GetKernelPackages", func() {
		var logger types.KairosLogger

		BeforeEach(func() {
			logger = createTestLogger()
		})

		It("should return kernel packages for supported systems", func() {
			system := values.System{
				Distro:  values.Ubuntu,
				Family:  values.DebianFamily,
				Version: "20.04",
				Arch:    values.ArchAMD64,
			}

			_, err := values.GetKernelPackages(system, logger)

			Expect(err).NotTo(HaveOccurred())
			// Result may be empty or contain packages depending on system configuration
			// The function should not error out
		})

		It("should handle different distro families", func() {
			systems := []values.System{
				{
					Distro:  values.Ubuntu,
					Family:  values.DebianFamily,
					Version: "20.04",
					Arch:    values.ArchAMD64,
				},
				{
					Distro:  values.Fedora,
					Family:  values.RedHatFamily,
					Version: "38",
					Arch:    values.ArchAMD64,
				},
			}

			for _, system := range systems {
				_, err := values.GetKernelPackages(system, logger)
				Expect(err).NotTo(HaveOccurred())
				// Function should succeed regardless of result content
			}
		})
	})
})