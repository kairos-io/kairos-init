package config_test

import (
	"os"

	"github.com/kairos-io/kairos-init/pkg/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config Package", func() {
	Describe("Variant", func() {
		Context("String method", func() {
			It("should convert to string correctly", func() {
				variant := config.CoreVariant
				Expect(variant.String()).To(Equal("core"))

				variant = config.StandardVariant
				Expect(variant.String()).To(Equal("standard"))
			})
		})

		Context("Equal method", func() {
			It("should compare correctly", func() {
				variant := config.CoreVariant
				Expect(variant.Equal("core")).To(BeTrue())
				Expect(variant.Equal("standard")).To(BeFalse())
				Expect(variant.Equal("invalid")).To(BeFalse())
			})
		})

		Context("FromString method", func() {
			It("should parse valid variants", func() {
				var variant config.Variant

				err := variant.FromString("core")
				Expect(err).NotTo(HaveOccurred())
				Expect(variant).To(Equal(config.CoreVariant))

				err = variant.FromString("standard")
				Expect(err).NotTo(HaveOccurred())
				Expect(variant).To(Equal(config.StandardVariant))
			})

			It("should return error for invalid variants", func() {
				var variant config.Variant

				err := variant.FromString("invalid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid variant"))
			})
		})
	})

	Describe("Provider", func() {
		Context("Provider struct", func() {
			It("should create Provider with all fields", func() {
				provider := config.Provider{
					Name:    "k3s",
					Version: "v1.28.1",
					Config:  "some-config",
				}

				Expect(provider.Name).To(Equal("k3s"))
				Expect(provider.Version).To(Equal("v1.28.1"))
				Expect(provider.Config).To(Equal("some-config"))
			})

			It("should work with empty fields", func() {
				provider := config.Provider{
					Name: "k0s",
				}

				Expect(provider.Name).To(Equal("k0s"))
				Expect(provider.Version).To(BeEmpty())
				Expect(provider.Config).To(BeEmpty())
			})
		})
	})

	Describe("Config", func() {
		Context("LoadVersionOverrides", func() {
			var tempDir string

			BeforeEach(func() {
				var err error
				tempDir, err = os.MkdirTemp("", "config-test")
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if tempDir != "" {
					os.RemoveAll(tempDir)
				}
			})

			It("should load version overrides from file", func() {
				Skip("LoadVersionOverrides uses hardcoded path /etc/kairos/.init_versions.yaml")
				// This test is skipped because the function uses a hardcoded path
				// which cannot be easily mocked without changing the implementation
			})

			It("should handle missing file gracefully", func() {
				cfg := &config.Config{}
				// This should not panic or error
				cfg.LoadVersionOverrides()

				// Overrides should remain empty/default
				Expect(cfg.VersionOverrides.Agent).To(BeEmpty())
				Expect(cfg.VersionOverrides.Immucore).To(BeEmpty())
			})

			It("should handle invalid YAML gracefully", func() {
				Skip("LoadVersionOverrides uses hardcoded path, cannot test invalid YAML without affecting system")
				// This test is skipped because we cannot mock the file system easily
			})
		})
	})

	Describe("ContainsSkipStep", func() {
		var originalSkipSteps []string

		BeforeEach(func() {
			// Save original skip steps to restore later
			originalSkipSteps = config.DefaultConfig.SkipSteps
		})

		AfterEach(func() {
			// Restore original skip steps
			config.DefaultConfig.SkipSteps = originalSkipSteps
		})

		It("should return true when step is in skip list", func() {
			config.DefaultConfig.SkipSteps = []string{"step1", "step2", "step3"}

			Expect(config.ContainsSkipStep("step1")).To(BeTrue())
			Expect(config.ContainsSkipStep("step2")).To(BeTrue())
			Expect(config.ContainsSkipStep("step3")).To(BeTrue())
		})

		It("should return false when step is not in skip list", func() {
			config.DefaultConfig.SkipSteps = []string{"step1", "step2"}

			Expect(config.ContainsSkipStep("step3")).To(BeFalse())
			Expect(config.ContainsSkipStep("nonexistent")).To(BeFalse())
		})

		It("should be case insensitive", func() {
			config.DefaultConfig.SkipSteps = []string{"Step1", "STEP2", "step3"}

			Expect(config.ContainsSkipStep("step1")).To(BeTrue())
			Expect(config.ContainsSkipStep("step2")).To(BeTrue())
			Expect(config.ContainsSkipStep("STEP3")).To(BeTrue())
			Expect(config.ContainsSkipStep("Step2")).To(BeTrue())
		})

		It("should handle empty skip list", func() {
			config.DefaultConfig.SkipSteps = []string{}

			Expect(config.ContainsSkipStep("anystep")).To(BeFalse())
		})

		It("should handle nil skip list", func() {
			config.DefaultConfig.SkipSteps = nil

			Expect(config.ContainsSkipStep("anystep")).To(BeFalse())
		})
	})

	Describe("Constants and Variables", func() {
		It("should have correct variant constants", func() {
			Expect(string(config.CoreVariant)).To(Equal("core"))
			Expect(string(config.StandardVariant)).To(Equal("standard"))
		})

		It("should have valid variants list", func() {
			Expect(config.ValidVariants).To(ContainElement(config.CoreVariant))
			Expect(config.ValidVariants).To(ContainElement(config.StandardVariant))
			Expect(len(config.ValidVariants)).To(Equal(2))
		})

		It("should initialize DefaultConfig with empty providers", func() {
			Expect(config.DefaultConfig.Providers).NotTo(BeNil())
			Expect(len(config.DefaultConfig.Providers)).To(Equal(0))
		})
	})
})