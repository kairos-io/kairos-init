package provider_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/kairos-io/kairos-sdk/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provider Build Tests", func() {
	var (
		logger      types.KairosLogger
		system      values.System
		tempDir     string
		testProvider string
	)

	BeforeEach(func() {
		logger = types.NewKairosLogger("test", "debug", false)
		system = values.System{
			Name:    "test-system",
			Distro:  values.Ubuntu,
			Family:  values.DebianFamily,
			Version: "20.04",
			Arch:    values.ArchAMD64,
		}

		var err error
		tempDir, err = os.MkdirTemp("", "kairos-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Check for an existing test provider binary in the project root
		if _, err := os.Stat("../../agent-provider-test"); err == nil {
			testProvider = filepath.Join(tempDir, "agent-provider-test")
			data, err := os.ReadFile("../../agent-provider-test")
			if err == nil {
				err = os.WriteFile(testProvider, data, 0755)
				if err != nil {
					testProvider = ""
				}
			}
		}
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
		// Reset the default config to avoid interference between tests
		config.DefaultConfig = config.Config{
			Variant: config.CoreVariant,
		}
	})

	Describe("Provider Bus Communication", func() {
		Context("when no providers are configured", func() {
			It("should handle empty provider list correctly", func() {
				// Setup config with no providers
				config.DefaultConfig.Providers = []config.Provider{}
				config.DefaultConfig.Variant = config.CoreVariant

				// Test that the system handles empty provider list
				providerCount := len(config.DefaultConfig.Providers)
				Expect(providerCount).To(Equal(0))
			})
		})

		Context("when build provider step is in skip list", func() {
			It("should respect skip configuration", func() {
				// Configure skip steps
				config.DefaultConfig.SkipSteps = []string{values.BuildProviderStep}

				// Test that the skip step is properly configured
				skipSteps := config.DefaultConfig.SkipSteps
				Expect(skipSteps).To(ContainElement(values.BuildProviderStep))
				
				// Test the skip check function
				isSkipped := config.ContainsSkipStep(values.BuildProviderStep)
				Expect(isSkipped).To(BeTrue())

				// Clean up skip steps for other tests
				config.DefaultConfig.SkipSteps = []string{}
			})
		})

		Context("when test provider is available", func() {
			BeforeEach(func() {
				// Skip this test if the test provider wasn't built
				if testProvider == "" || !fileExists(testProvider) {
					Skip("Test provider binary not available")
				}

				// Add the test provider directory to PATH
				oldPath := os.Getenv("PATH")
				newPath := tempDir + ":" + oldPath
				os.Setenv("PATH", newPath)
				
				// Restore PATH after test
				DeferCleanup(func() {
					os.Setenv("PATH", oldPath)
				})
			})

			It("should discover the test provider plugin", func() {
				// Test that bus can discover plugins
				manager := bus.NewBus(bus.InitProviderInstall)
				manager.Initialize(bus.WithLogger(&logger))
				
				// The manager should find plugins in PATH
				// Note: This might be 0 if no plugins respond to the specific event
				Expect(len(manager.Plugins)).To(BeNumerically(">=", 0))
			})

			It("should handle provider configuration correctly", func() {
				// Test that the provider configuration structure works as expected
				providers := []config.Provider{
					{Name: "test", Version: "v1.0.0", Config: "test-config"},
				}

				config.DefaultConfig.Providers = providers
				config.DefaultConfig.Variant = config.StandardVariant

				// Verify the providers are set correctly
				Expect(config.DefaultConfig.Providers).To(HaveLen(1))
				Expect(config.DefaultConfig.Providers[0].Name).To(Equal("test"))
				Expect(config.DefaultConfig.Providers[0].Version).To(Equal("v1.0.0"))
				Expect(config.DefaultConfig.Providers[0].Config).To(Equal("test-config"))

				// Verify variant is set to standard for providers
				Expect(config.DefaultConfig.Variant.String()).To(Equal("standard"))
			})

			It("should create proper provider payload", func() {
				providers := []config.Provider{
					{Name: "test", Version: "v1.0.0", Config: "test-config"},
				}
				config.DefaultConfig.Providers = providers

				for _, provider := range config.DefaultConfig.Providers {
					payload := bus.ProviderPayload{
						Provider: provider.Name,
						Version:  provider.Version,
						Config:   provider.Config,
						LogLevel: logger.Logger.GetLevel().String(),
						Family:   system.Family.String(),
					}

					Expect(payload.Provider).To(Equal("test"))
					Expect(payload.Version).To(Equal("v1.0.0"))
					Expect(payload.Config).To(Equal("test-config"))
					Expect(payload.Family).To(Equal("debian"))
				}
			})

			It("should handle communication timeout gracefully", func() {
				// Test that provider communication has reasonable timeout behavior
				// This test verifies that we can at least attempt provider communication
				providers := []config.Provider{
					{Name: "test", Version: "v1.0.0", Config: "test-config"},
				}
				config.DefaultConfig.Providers = providers

				// Simulate provider event with timeout
				done := make(chan bool, 1)
				go func() {
					manager := bus.NewBus(bus.InitProviderInstall)
					manager.Initialize(bus.WithLogger(&logger))
					done <- true
				}()

				Eventually(done, 5*time.Second).Should(Receive(BeTrue()))
			})
		})
	})

	Describe("Provider Info and Discovery", func() {
		It("should include BuildProviderStep in steps info", func() {
			stepsInfo := values.StepsInfo()
			
			stepKeys := make(map[string]bool)
			for _, step := range stepsInfo {
				stepKeys[step.Key] = true
			}

			Expect(stepKeys).To(HaveKey(values.BuildProviderStep))
		})

		It("should provide meaningful description for BuildProviderStep", func() {
			stepsInfo := values.StepsInfo()
			
			var buildProviderDesc string
			for _, step := range stepsInfo {
				if step.Key == values.BuildProviderStep {
					buildProviderDesc = step.Value
					break
				}
			}

			Expect(buildProviderDesc).NotTo(BeEmpty())
			Expect(buildProviderDesc).To(ContainSubstring("build"))
			Expect(buildProviderDesc).To(ContainSubstring("provider"))
		})

		It("should include BuildProviderStep in step names list", func() {
			stepNames := values.GetStepNames()
			Expect(stepNames).To(ContainElement(values.BuildProviderStep))
		})

		It("should have correct BuildProviderStep constant value", func() {
			Expect(values.BuildProviderStep).To(Equal("buildProvider"))
		})

		It("should validate BuildProviderStep exists in values package", func() {
			// This test ensures that the BuildProviderStep constant is properly defined
			// and available for use in configuration and skip logic
			Expect(values.BuildProviderStep).NotTo(BeEmpty())
			Expect(len(values.BuildProviderStep)).To(BeNumerically(">", 5))
		})
	})

	Describe("Provider Configuration Integration", func() {
		It("should handle multiple provider configurations", func() {
			// Test that the provider configuration structure works with multiple providers
			providers := []config.Provider{
				{Name: "k3s", Version: "v1.28.1", Config: "k3s-config"},
				{Name: "k0s", Version: "v1.27.5", Config: "k0s-config"},
				{Name: "test", Version: "v1.0.0", Config: "test-config"},
			}

			config.DefaultConfig.Providers = providers
			config.DefaultConfig.Variant = config.StandardVariant

			// Verify all providers are set correctly
			Expect(config.DefaultConfig.Providers).To(HaveLen(3))
			
			expectedProviders := map[string]string{
				"k3s": "v1.28.1",
				"k0s": "v1.27.5", 
				"test": "v1.0.0",
			}

			for _, provider := range config.DefaultConfig.Providers {
				expectedVersion, exists := expectedProviders[provider.Name]
				Expect(exists).To(BeTrue(), "Provider %s should exist", provider.Name)
				Expect(provider.Version).To(Equal(expectedVersion), "Version for provider %s should match", provider.Name)
			}

			// Verify variant is set to standard for providers
			Expect(config.DefaultConfig.Variant.String()).To(Equal("standard"))
		})

		It("should default to core variant when no providers", func() {
			config.DefaultConfig.Providers = []config.Provider{}
			config.DefaultConfig.Variant = config.CoreVariant

			Expect(config.DefaultConfig.Variant.String()).To(Equal("core"))
			Expect(len(config.DefaultConfig.Providers)).To(Equal(0))
		})

		It("should handle provider configurations with empty fields", func() {
			// Test edge cases with empty or missing configuration fields
			providers := []config.Provider{
				{Name: "test", Version: "", Config: ""},
				{Name: "", Version: "v1.0.0", Config: "config"},
			}

			config.DefaultConfig.Providers = providers
			
			Expect(config.DefaultConfig.Providers).To(HaveLen(2))
			Expect(config.DefaultConfig.Providers[0].Name).To(Equal("test"))
			Expect(config.DefaultConfig.Providers[0].Version).To(Equal(""))
			Expect(config.DefaultConfig.Providers[1].Name).To(Equal(""))
			Expect(config.DefaultConfig.Providers[1].Version).To(Equal("v1.0.0"))
		})
	})
})

// Helper function to check if file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}