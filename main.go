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
	trusted      string
	version      string
	ksProvider   = newEnumFlag([]string{string(config.K3sProvider), string(config.K0sProvider)}, "")
	stageFlag    = newEnumFlag([]string{"init", "install", "all"}, "all")
	loglevelFlag = newEnumFlag([]string{"debug", "info", "warn", "error", "trace"}, "info")
)

// Fill the flags and set default configs for commands
func preRun(_ *cobra.Command, _ []string) {
	// Set the trusted boot flag to true
	if strings.ToLower(trusted) == "true" || strings.ToLower(trusted) == "1" {
		config.DefaultConfig.TrustedBoot = true
	}

	if ksProvider.Value != "" {
		// Try to load the kubernetes provider. As its an enum, there's no need to check if the value is valid
		_ = config.DefaultConfig.KubernetesProvider.FromString(ksProvider.Value)

		if config.DefaultConfig.KubernetesVersion == "latest" {
			// Set the kubernetes version to empty if latest is set so the latest is used
			config.DefaultConfig.KubernetesVersion = ""
		}
		config.DefaultConfig.Variant = config.StandardVariant
	} else {
		// If no provider is set, set the variant to core
		config.DefaultConfig.Variant = config.CoreVariant
	}
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the system",
	Long:  `Validate the system to ensure all required components are in place`,
	PreRun: func(cmd *cobra.Command, args []string) {
		preRun(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate always logs ant info level
		logger := types.NewKairosLogger("kairos-init", "info", false)
		logger.Infof("Starting kairos-init version %s", values.GetVersion())

		validator := validation.NewValidator(logger)
		return validator.Validate()
	},
}

var rootCmd = &cobra.Command{
	Use:   "kairos-init",
	Short: "Kairos init tool",
	Long:  `Kairos init tool for system initialization and configuration`,
	PreRun: func(cmd *cobra.Command, args []string) {
		preRun(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := types.NewKairosLogger("kairos-init", loglevelFlag.Value, false)
		logger.Infof("Starting kairos-init version %s", values.GetVersion())
		logger.Debug(litter.Sdump(values.GetFullVersion()))

		// Parse the version number
		sv, err := semver.NewSemver(version)
		if err != nil {
			return fmt.Errorf("error parsing version: %w", err)
		}

		config.DefaultConfig.KairosVersion = *sv
		litter.Config.HidePrivateFields = false
		logger.Debug(litter.Sdump(config.DefaultConfig))

		var runStages schema.YipConfig

		if stageFlag.Value != "" {
			logger.Infof("Running stage %s", stageFlag.Value)
			switch stageFlag.Value {
			case "install":
				runStages, err = stages.RunInstallStage(logger)
			case "init":
				runStages, err = stages.RunInitStage(logger)
			case "all":
				runStages, err = stages.RunAllStages(logger)
			default:
				return fmt.Errorf("unknown stage %s. Valid values are %s", stageFlag.Value, strings.Join(stageFlag.Allowed, ", "))
			}
		}

		if err != nil {
			logger.Error(err)
			return err
		}

		litter.Config.HideZeroValues = true
		litter.Config.HidePrivateFields = true
		// Save the stages to a file for debugging and future use
		if stageFlag.Value == "all" {
			_ = os.WriteFile("/etc/kairos/kairos-init-all-stage.yaml", []byte(runStages.ToString()), 0644)
		} else {
			_ = os.WriteFile(fmt.Sprintf("/etc/kairos/kairos-init-%s-stage.yaml", stageFlag.Value), []byte(runStages.ToString()), 0644)
		}

		return nil
	},
}

func init() {
	// enum flags
	rootCmd.Flags().VarP(stageFlag, "stage", "s", fmt.Sprintf("set the stage to run (%s)", strings.Join(stageFlag.Allowed, ", ")))
	rootCmd.Flags().VarP(loglevelFlag, "level", "l", fmt.Sprintf("set the log level (%s)", strings.Join(loglevelFlag.Allowed, ", ")))
	rootCmd.Flags().VarP(ksProvider, "kubernetes-provider", "k", fmt.Sprintf("Kubernetes provider (%s)", strings.Join(ksProvider.Allowed, ", ")))
	// rest of the flags
	rootCmd.Flags().StringVarP(&config.DefaultConfig.Model, "model", "m", "generic", "model to build for, like generic or rpi4")
	rootCmd.Flags().StringVar(&config.DefaultConfig.KubernetesVersion, "k8sversion", "latest", "Kubernetes version for provider")
	rootCmd.Flags().BoolVar(&config.DefaultConfig.Fips, "fips", false, "use fips kairos binary versions. For FIPS 140-2 compliance images")
	rootCmd.Flags().StringVarP(&version, "version", "v", "", "set a version number to use for the generated system. Its used to identify this system for upgrades and such. Required.")
	rootCmd.Flags().BoolVarP(&config.DefaultConfig.Extensions, "stage-extensions", "x", false, "enable stage extensions mode")
	rootCmd.Flags().BoolVar(&config.DefaultConfig.SkipInstallPackages, "skip-packages-install", false, "Skip the install of packages. This assumes that the needed packages are already installed in the base image.")
	rootCmd.Flags().BoolVar(&config.DefaultConfig.SkipInstallK8s, "skip-k8s-install", false, "Skip the install of k8s packages. This assumes that the needed packages are already installed in the base image.")

	// Mark required flags
	_ = rootCmd.MarkFlagRequired("version")

	addSharedFlags(rootCmd)
	addSharedFlags(validateCmd)

	rootCmd.AddCommand(validateCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// Shared flags are flags that are used in multiple commands
func addSharedFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&trusted, "trusted", "t", "false", "init the system for Trusted Boot, changes bootloader to systemd")
}

type enum struct {
	Allowed []string
	Value   string
}

// newEnum give a list of allowed flag parameters, where the second argument is the default
func newEnumFlag(allowed []string, d string) *enum {
	return &enum{
		Allowed: allowed,
		Value:   d,
	}
}

func (a *enum) String() string {
	return a.Value
}

func (a *enum) Set(p string) error {
	isIncluded := func(opts []string, val string) bool {
		for _, opt := range opts {
			if val == opt {
				return true
			}
		}
		return false
	}
	if !isIncluded(a.Allowed, p) {
		return fmt.Errorf("%s is not included in %s", p, strings.Join(a.Allowed, ","))
	}
	a.Value = p
	return nil
}

func (a *enum) Type() string {
	return "string"
}
