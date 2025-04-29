package main

import (
	"fmt"
	"os"
	"strings"

	semver "github.com/hashicorp/go-version"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/stages"
	"github.com/kairos-io/kairos-init/pkg/validation"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"
)

var (
	trusted    string
	validate   bool
	variant    string
	ksProvider string
	version    string
)

// runValidation performs system validation and returns an error if validation fails
func runValidation(logger types.KairosLogger) error {
	logger.Infof("Starting kairos-init version %s", values.GetVersion())
	logger.Debug(litter.Sdump(values.GetFullVersion()))

	validator := validation.NewValidator(logger)
	err := validator.Validate()
	if err != nil {
		logger.Error(err)
		return err
	}
	logger.Info("System is valid")
	return nil
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the system",
	Long:  `Validate the system to ensure all required components are in place`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := types.NewKairosLogger("kairos-init", config.DefaultConfig.Level, false)
		return runValidation(logger)
	},
}

var rootCmd = &cobra.Command{
	Use:   "kairos-init",
	Short: "Kairos init tool",
	Long:  `Kairos init tool for system initialization and configuration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set the trusted boot flag to true
		if strings.ToLower(trusted) == "true" || strings.ToLower(trusted) == "1" {
			config.DefaultConfig.TrustedBoot = true
		}

		// Try to load the variant
		err := config.DefaultConfig.Variant.FromString(variant)
		if err != nil {
			return fmt.Errorf("error loading variant: %w", err)
		}

		// Try to load the kubernetes provider
		err = config.DefaultConfig.KubernetesProvider.FromString(ksProvider)
		if err != nil {
			return fmt.Errorf("error loading kubernetes provider: %w", err)
		}

		if config.DefaultConfig.KubernetesVersion == "latest" {
			config.DefaultConfig.KubernetesVersion = ""
		}

		logger := types.NewKairosLogger("kairos-init", config.DefaultConfig.Level, false)
		logger.Infof("Starting kairos-init version %s", values.GetVersion())
		logger.Debug(litter.Sdump(values.GetFullVersion()))

		// Parse the version number
		sv, err := semver.NewSemver(version)
		if err != nil {
			return fmt.Errorf("error parsing version: %w", err)
		}

		config.DefaultConfig.KairosVersion = *sv
		litter.Config.HideZeroValues = true
		litter.Config.HidePrivateFields = false
		logger.Debug(litter.Sdump(config.DefaultConfig))

		var runStages schema.YipConfig

		if validate {
			return runValidation(logger)
		}

		if config.DefaultConfig.Stage != "" {
			logger.Infof("Running stage %s", config.DefaultConfig.Stage)
			switch config.DefaultConfig.Stage {
			case "install":
				runStages, err = stages.RunInstallStage(logger)
			case "init":
				runStages, err = stages.RunInitStage(logger)
			case "all":
				runStages, err = stages.RunAllStages(logger)
			default:
				return fmt.Errorf("unknown stage %s. Valid values are install, init and all", config.DefaultConfig.Stage)
			}
		}

		if err != nil {
			logger.Error(err)
			return err
		}

		litter.Config.HideZeroValues = true
		litter.Config.HidePrivateFields = true
		// Save the stages to a file for debugging and future use
		if config.DefaultConfig.Stage == "all" {
			_ = os.WriteFile("/etc/kairos/kairos-init-all-stage.yaml", []byte(runStages.ToString()), 0644)
		} else {
			_ = os.WriteFile(fmt.Sprintf("/etc/kairos/kairos-init-%s-stage.yaml", config.DefaultConfig.Stage), []byte(runStages.ToString()), 0644)
		}

		return nil
	},
}

func init() {
	// Global flags
	rootCmd.Flags().StringVarP(&config.DefaultConfig.Level, "level", "l", "info", "set the log level")
	rootCmd.Flags().StringVarP(&config.DefaultConfig.Stage, "stage", "s", "all", "set the stage to run")
	rootCmd.Flags().StringVarP(&config.DefaultConfig.Model, "model", "m", "generic", "model to build for, like generic or rpi4")
	rootCmd.Flags().StringVarP(&variant, "variant", "v", config.CoreVariant.String(), "variant to build (core or standard for k3s flavor)")
	rootCmd.Flags().StringVarP(&ksProvider, "kubernetes-provider", "k", string(config.K3sProvider), "Kubernetes provider")
	rootCmd.Flags().StringVar(&config.DefaultConfig.KubernetesVersion, "k8sversion", "latest", "Kubernetes version for provider")
	rootCmd.Flags().StringVarP(&config.DefaultConfig.Registry, "registry", "r", "quay.io/kairos", "registry and org where the image is gonna be pushed. This is mainly used on upgrades to search for available images to upgrade to")
	rootCmd.Flags().StringVarP(&trusted, "trusted", "t", "false", "init the system for Trusted Boot, changes bootloader to systemd")
	rootCmd.Flags().StringVarP(&config.DefaultConfig.FrameworkVersion, "framework", "f", values.GetFrameworkVersion(), "set the framework version to use")
	rootCmd.Flags().BoolVar(&validate, "validate", false, "validate the running os to see if it all the pieces are in place")
	rootCmd.Flags().BoolVar(&config.DefaultConfig.Fips, "fips", false, "use fips framework. For FIPS 140-2 compliance images")
	rootCmd.Flags().StringVar(&version, "version", "", "set a version number to use for the generated system. Its used to identify this system for upgrades and such. Required.")
	rootCmd.Flags().BoolVar(&config.DefaultConfig.Extensions, "stage-extensions", false, "enable stage extensions mode")

	// Mark required flags
	rootCmd.MarkFlagRequired("version")

	rootCmd.AddCommand(validateCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
