package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kairos-io/kairos-init/pkg/bundled"
	"gopkg.in/yaml.v3"

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
	trusted       string
	version       string
	stageFlag     = newEnumFlag([]string{"init", "install", "all"}, "all")
	loglevelFlag  = newEnumFlag([]string{"debug", "info", "warn", "error", "trace"}, "info")
	skipStepsFlag = newEnumSliceFlag(values.GetStepNames(), []string{})
)

// Fill the flags and set default configs for commands
func preRun(_ *cobra.Command, _ []string) {
	// Set the trusted boot flag to true
	if strings.ToLower(trusted) == "true" || strings.ToLower(trusted) == "1" {
		config.DefaultConfig.TrustedBoot = true
	}

	if config.DefaultConfig.ProviderName != "" {
		if config.DefaultConfig.ProviderVersion == "latest" {
			// Set the kubernetes version to empty if latest is set so the latest is used
			config.DefaultConfig.ProviderVersion = ""
		}
		config.DefaultConfig.Variant = config.StandardVariant
	} else {
		// If no provider is set, set the variant to core
		config.DefaultConfig.Variant = config.CoreVariant
	}

	config.DefaultConfig.SkipSteps = skipStepsFlag.Value
}

var stepsInfo = &cobra.Command{
	Use:   "steps-info",
	Short: "Get information about the steps",
	Long:  `Get information about the steps are run`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := types.NewKairosLogger("kairos-init", "info", false)
		logger.Infof("Starting kairos-init version %s", values.GetVersion())
		// Print the steps info in a human readable format
		stepsInfo := values.StepsInfo()
		logger.Infof("Step name & Description")
		logger.Infof("--------------------------------------------------------")
		for step, _ := range stepsInfo {
			logger.Infof("\"%s\": %s", stepsInfo[step].Key, stepsInfo[step].Value)
		}
		logger.Infof("--------------------------------------------------------")
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of kairos-init and bundled binaries",
	Long:  `Print the version of kairos-init and bundled binaries. If other binary versions are selected, those are not shown, only the embedded ones`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := types.NewKairosLogger("kairos-init", "info", false)
		logger.Infof("kairos-init version %s", values.GetVersion())
		logger.Debug(litter.Sdump(values.GetFullVersion()))

		// parse embeded version info for binaries
		versionInfo := map[string]string{}
		err := yaml.Unmarshal(bundled.EmbeddedVersionInfo, &versionInfo)
		if err != nil {
			logger.Errorf("Error parsing embedded version info: %v", err)
			return
		}
		logger.Infof("Embedded version info:")
		for key, value := range versionInfo {
			logger.Infof("%s: %s", key, value)
		}
	},
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
	// rest of the flags
	rootCmd.Flags().StringVarP(&config.DefaultConfig.Model, "model", "m", "generic", "model to build for, like generic or rpi4")
	rootCmd.Flags().StringVarP(&config.DefaultConfig.ProviderName, "provider", "k", "", fmt.Sprintf("Provider plugin"))
	rootCmd.Flags().StringVar(&config.DefaultConfig.ProviderVersion, "providerVersion", "latest", "Version for provider")
	rootCmd.Flags().StringVar(&config.DefaultConfig.ProviderConfigFile, "providerConfig", "", "Extra configuration for provider")
	rootCmd.Flags().BoolVar(&config.DefaultConfig.Fips, "fips", false, "use fips kairos binary versions. For FIPS 140-2 compliance images")
	rootCmd.Flags().StringVarP(&version, "version", "v", "", "set a version number to use for the generated system. Its used to identify this system for upgrades and such. Required.")
	rootCmd.Flags().BoolVarP(&config.DefaultConfig.Extensions, "stage-extensions", "x", false, "enable stage extensions mode")
	rootCmd.Flags().Var(skipStepsFlag, "skip-step", "Skip one or more steps. Valid values are: "+strings.Join(skipStepsFlag.Allowed, ", ")+". You can pass multiple values separated by commas, for example: --skip-step initrd,workarounds")
	// Mark required flags
	_ = rootCmd.MarkFlagRequired("version")

	addSharedFlags(rootCmd)
	addSharedFlags(validateCmd)

	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(stepsInfo)
	rootCmd.AddCommand(versionCmd)
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

type enumSlice struct {
	Allowed []string
	Value   []string
}

func (e *enumSlice) Type() string {
	return "string,string,..."
}

// newEnumSliceFlag give a list of allowed flag parameters, where the second argument is the default. Accepts more than one value
func newEnumSliceFlag(allowed []string, defaults []string) *enumSlice {
	return &enumSlice{
		Allowed: allowed,
		Value:   defaults,
	}
}

func (e *enumSlice) String() string {
	return strings.Join(e.Value, ",")
}

func (e *enumSlice) Set(val string) error {
	vals := strings.Split(val, ",")
	var newVals []string
	for _, v := range vals {
		v = strings.TrimSpace(v)
		found := false
		for _, a := range e.Allowed {
			if v == a {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s is not included in %s", v, strings.Join(e.Allowed, ","))
		}
		newVals = append(newVals, v)
	}
	e.Value = newVals
	return nil
}
