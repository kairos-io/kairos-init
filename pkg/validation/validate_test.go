package validation_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/kairos-init/pkg/validation"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Use a real logger for testing
func createTestLogger() types.KairosLogger {
	return types.NewKairosLogger("test", "info", false)
}

var _ = Describe("Validator", func() {
	Describe("NewValidator", func() {
		It("should create a new validator with logger and system", func() {
			logger := createTestLogger()
			validator := validation.NewValidator(logger)

			Expect(validator).NotTo(BeNil())
			Expect(validator.Log).To(Equal(logger))
			Expect(validator.System).NotTo(BeNil())
		})
	})

	Describe("validateRHELServices", func() {
		Context("when system is not RHEL family", func() {
			It("should not validate services", func() {
				logger := createTestLogger()
				validator := &validation.Validator{
					Log: logger,
					System: values.System{
						Family: values.DebianFamily, // Non-RHEL family
					},
				}

				err := validator.ValidateRHELServices()
				Expect(err).NotTo(HaveOccurred(), "Should not validate services on non-RHEL family systems")
			})
		})

		Context("when system is RHEL family", func() {
			var (
				logger    types.KairosLogger
				validator *validation.Validator
				tempDir   string
			)

			BeforeEach(func() {
				logger = createTestLogger()
				validator = &validation.Validator{
					Log: logger,
					System: values.System{
						Family: values.RedHatFamily,
					},
				}
			})

			Context("with no masked services", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create regular service files (not masked)
					services := []string{"systemd-udevd", "systemd-logind"}
					for _, service := range services {
						servicePath := filepath.Join(tempDir, fmt.Sprintf("%s.service", service))
						err = os.WriteFile(servicePath, []byte("[Unit]\nDescription=Test Service"), 0644)
						Expect(err).NotTo(HaveOccurred())
					}
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should not error", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).NotTo(HaveOccurred(), "Should not error when services exist and are not masked")
				})
			})

			Context("with one masked service", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create a masked service file (symlink to /dev/null) for only one service
					maskedServicePath := filepath.Join(tempDir, "systemd-udevd.service")
					err = os.Symlink("/dev/null", maskedServicePath)
					Expect(err).NotTo(HaveOccurred())

					// Verify the symlink was created correctly
					target, err := os.Readlink(maskedServicePath)
					Expect(err).NotTo(HaveOccurred())
					Expect(target).To(Equal("/dev/null"), "Symlink should point to /dev/null")
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when a service is masked", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when a service is masked")
					Expect(err.Error()).To(ContainSubstring("systemd-udevd is masked on RHEL family system"))
				})
			})

			Context("with both services masked", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create masked service files (symlinks to /dev/null) for both services
					services := []string{"systemd-udevd", "systemd-logind"}
					for _, service := range services {
						maskedServicePath := filepath.Join(tempDir, fmt.Sprintf("%s.service", service))
						err = os.Symlink("/dev/null", maskedServicePath)
						Expect(err).NotTo(HaveOccurred())

						// Verify the symlink was created correctly
						target, err := os.Readlink(maskedServicePath)
						Expect(err).NotTo(HaveOccurred())
						Expect(target).To(Equal("/dev/null"), "Symlink should point to /dev/null")
					}
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when both services are masked", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when services are masked")
					Expect(err.Error()).To(ContainSubstring("systemd-udevd is masked on RHEL family system"))
					Expect(err.Error()).To(ContainSubstring("systemd-logind is masked on RHEL family system"))
				})
			})

			Context("with regular service files", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create regular service files (not masked) for both services
					services := []string{"systemd-udevd", "systemd-logind"}
					for _, service := range services {
						servicePath := filepath.Join(tempDir, fmt.Sprintf("%s.service", service))
						err = os.WriteFile(servicePath, []byte("[Unit]\nDescription=Test Service"), 0644)
						Expect(err).NotTo(HaveOccurred())
					}
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should not error when services are regular files", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).NotTo(HaveOccurred(), "Should not error when services are regular files")
				})
			})

			Context("with mixed services (one masked, one regular)", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create a masked service file
					maskedServicePath := filepath.Join(tempDir, "systemd-udevd.service")
					err = os.Symlink("/dev/null", maskedServicePath)
					Expect(err).NotTo(HaveOccurred())

					// Create a regular service file
					regularServicePath := filepath.Join(tempDir, "systemd-logind.service")
					err = os.WriteFile(regularServicePath, []byte("[Unit]\nDescription=Login Service"), 0644)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when any service is masked", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when any service is masked")
					Expect(err.Error()).To(ContainSubstring("systemd-udevd is masked on RHEL family system"))
				})
			})

			Context("with missing services", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())
					// Don't create any service files - they should be missing
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when services don't exist", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when services don't exist")
					Expect(err.Error()).To(ContainSubstring("systemd-udevd does not exist on RHEL family system"))
					Expect(err.Error()).To(ContainSubstring("systemd-logind does not exist on RHEL family system"))
				})
			})

			Context("with one missing service", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create only one service file
					servicePath := filepath.Join(tempDir, "systemd-udevd.service")
					err = os.WriteFile(servicePath, []byte("[Unit]\nDescription=Test Service"), 0644)
					Expect(err).NotTo(HaveOccurred())
					// systemd-logind.service is missing
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when one service is missing", func() {
					err := validator.ValidateRHELServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when one service is missing")
					Expect(err.Error()).To(ContainSubstring("systemd-logind does not exist on RHEL family system"))
					Expect(err.Error()).NotTo(ContainSubstring("systemd-udevd does not exist"))
				})
			})
		})
	})

	Describe("validateGettyServices", func() {
		Context("when system is Alpine family", func() {
			It("should not validate services", func() {
				logger := createTestLogger()
				validator := &validation.Validator{
					Log: logger,
					System: values.System{
						Family: values.AlpineFamily, // Alpine uses OpenRC, not systemd
					},
				}

				err := validator.ValidateGettyServices()
				Expect(err).NotTo(HaveOccurred(), "Should not validate services on Alpine family systems")
			})
		})

		Context("when system is systemd-based", func() {
			var (
				logger    types.KairosLogger
				validator *validation.Validator
				tempDir   string
			)

			BeforeEach(func() {
				logger = createTestLogger()
				validator = &validation.Validator{
					Log: logger,
					System: values.System{
						Family: values.DebianFamily, // Systemd-based family
					},
				}
			})

			Context("with getty.target not masked", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create a regular getty.target file (not masked)
					gettyPath := filepath.Join(tempDir, "getty.target")
					err = os.WriteFile(gettyPath, []byte("[Unit]\nDescription=Getty Target"), 0644)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should not error", func() {
					err := validator.ValidateGettyServicesWithPath(tempDir)
					Expect(err).NotTo(HaveOccurred(), "Should not error when getty.target is not masked")
				})
			})

			Context("with getty.target masked", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())

					// Create a masked getty.target file (symlink to /dev/null)
					gettyPath := filepath.Join(tempDir, "getty.target")
					err = os.Symlink("/dev/null", gettyPath)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when getty.target is masked", func() {
					err := validator.ValidateGettyServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when getty.target is masked")
					Expect(err.Error()).To(ContainSubstring("getty.target is masked on systemd-based system"))
				})
			})

			Context("with getty.target missing", func() {
				BeforeEach(func() {
					var err error
					tempDir, err = os.MkdirTemp("", "systemd-system")
					Expect(err).NotTo(HaveOccurred())
					// Don't create getty.target - it should be missing
				})

				AfterEach(func() {
					if tempDir != "" {
						os.RemoveAll(tempDir)
					}
				})

				It("should error when getty.target doesn't exist", func() {
					err := validator.ValidateGettyServicesWithPath(tempDir)
					Expect(err).To(HaveOccurred(), "Should error when getty.target doesn't exist")
					Expect(err.Error()).To(ContainSubstring("getty.target does not exist on systemd-based system"))
				})
			})
		})
	})

	Describe("Validate", func() {
		It("should run full validation without panicking", func() {
			logger := createTestLogger()
			validator := validation.NewValidator(logger)

			// This test will run the full validation on the current system
			// It's more of an integration test and may fail depending on the system state
			err := validator.Validate()

			// We don't assert on the result here since it depends on the actual system state
			// This test is mainly to ensure the validation doesn't panic
			GinkgoWriter.Printf("Validation result: %v\n", err)
		})
	})
})
