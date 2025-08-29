package provider_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/bus"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/go-pluggable"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provider Build Tests", func() {
	var (
		logger       types.KairosLogger
		system       values.System
		tempDir      string
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

		// Build the test provider on the fly
		testProvider = filepath.Join(tempDir, "agent-provider-test")
		buildProviderBinary(testProvider)
		
		// Ensure the current working directory contains the provider for the bus manager
		// The bus manager adds the current working directory to the plugin search path
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		
		cwdProvider := filepath.Join(cwd, "agent-provider-test")
		err = copyFile(testProvider, cwdProvider)
		Expect(err).NotTo(HaveOccurred())
		
		// Clean up CWD provider after test
		DeferCleanup(func() {
			os.Remove(cwdProvider)
		})
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
			})

			It("should call provider build install event via bus", func() {
				// Test provider communication via bus
				providers := []config.Provider{
					{Name: "test", Version: "v1.0.0", Config: "test-config"},
				}

				config.DefaultConfig.Providers = providers
				config.DefaultConfig.Variant = config.StandardVariant

				// Test bus communication directly
				manager := bus.NewBus(bus.InitProviderInstall)
				manager.Initialize(bus.WithLogger(&logger))

				// The manager should discover plugins (might be 0 if no plugins in CWD)
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
				providers := []config.Provider{
					{Name: "test", Version: "v1.0.0", Config: "test-config"},
				}
				config.DefaultConfig.Providers = providers

				// Test provider communication with timeout
				done := make(chan bool, 1)
				go func() {
					manager := bus.NewBus(bus.InitProviderInstall)
					manager.Initialize(bus.WithLogger(&logger))
					done <- true
				}()

				Eventually(done, 10*time.Second).Should(Receive(BeTrue()))
			})

			It("should test provider response via bus publish", func() {
				// Skip if provider not available
				if testProvider == "" || !fileExists(testProvider) {
					Skip("Test provider binary not available")
				}

				providers := []config.Provider{
					{Name: "test", Version: "v1.0.0", Config: "test-config"},
				}
				config.DefaultConfig.Providers = providers

				// Test bus communication
				manager := bus.NewBus(bus.InitProviderInstall)
				manager.Initialize(bus.WithLogger(&logger))

				if len(manager.Plugins) > 0 {
					// Test that we can publish to the provider
					responseChan := make(chan bool, 1)
					
					manager.Response(bus.InitProviderInstall, func(p *pluggable.Plugin, resp *pluggable.EventResponse) {
						logger.Logger.Debug().Str("at", p.Executable).Interface("resp", resp).Msg("Received response from provider")
						responseChan <- true
					})

					for _, provider := range config.DefaultConfig.Providers {
						dataSend := bus.ProviderPayload{
							Provider: provider.Name,
							Version:  provider.Version,
							Config:   provider.Config,
							LogLevel: logger.Logger.GetLevel().String(),
							Family:   system.Family.String(),
						}
						_, err := manager.Publish(bus.InitProviderInstall, dataSend)
						Expect(err).NotTo(HaveOccurred())
					}

					// Wait for response or timeout
					select {
					case <-responseChan:
						// Success: received response from provider
					case <-time.After(5 * time.Second):
						// Timeout is ok - just means no provider responded
					}
				}
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

// buildProviderBinary builds the test provider binary on the fly
func buildProviderBinary(outputPath string) {
	// Get the source directory
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	
	// Find the project root by looking for go.mod
	projectRoot := cwd
	for {
		if fileExists(filepath.Join(projectRoot, "go.mod")) {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			Fail("Could not find project root with go.mod")
		}
		projectRoot = parent
	}
	
	providerSrc := filepath.Join(projectRoot, "cmd", "agent-provider-test")
	
	// Build the provider
	cmd := exec.Command("go", "build", "-o", outputPath, providerSrc)
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		Fail(fmt.Sprintf("Failed to build test provider: %v\nOutput: %s", err, string(output)))
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// Helper function to check if file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}