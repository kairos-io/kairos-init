package main

import (
	"flag"
	"fmt"
	"github.com/mudler/yip/pkg/schema"
	"os"

	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/stages"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/sanity-io/litter"
)

func main() {
	flag.StringVar(&config.DefaultConfig.Level, "level", "info", "set the log level (shorthand: -l)")
	flag.StringVar(&config.DefaultConfig.Level, "l", "info", "set the log level (shorthand: -l)")
	flag.StringVar(&config.DefaultConfig.Stage, "stage", "", "set the stage to run (shorthand: -s)")
	flag.StringVar(&config.DefaultConfig.Stage, "s", "", "set the stage to run (shorthand: -s)")
	flag.StringVar(&config.DefaultConfig.Model, "model", "generic", "model to build for, like generic or rpi4 (shorthand: -m)")
	flag.StringVar(&config.DefaultConfig.Model, "m", "generic", "model to build for, like generic or rpi4 (shorthand: -m)")
	flag.BoolVar(&config.DefaultConfig.TrustedBoot, "trustedboot", false, "init the system for Trusted Boot, changes bootloader to systemd (shorthand: -t)")
	flag.BoolVar(&config.DefaultConfig.TrustedBoot, "t", false, "init the system for Trusted Boot, changes bootloader to systemd (shorthand: -t)")
	flag.Parse()

	logger := types.NewKairosLogger("kairos-init", config.DefaultConfig.Level, false)
	logger.Infof("Starting kairos-init version %s", values.GetVersion())
	logger.Debug(litter.Sdump(values.GetFullVersion()))
	logger.Debug(litter.Sdump(config.DefaultConfig))

	var err error
	var runStages schema.YipConfig

	if config.DefaultConfig.Stage != "" {
		logger.Infof("Running stage %s", config.DefaultConfig.Stage)
		switch config.DefaultConfig.Stage {
		case "install":
			runStages, err = stages.RunInstallStage(logger)
		case "init":
			runStages, err = stages.RunInitStage(logger)
		default:
			logger.Errorf("Unknown stage %s", config.DefaultConfig.Stage)
			os.Exit(1)
		}
	} else {
		runStages, err = stages.RunAllStages(logger)
	}

	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

	litter.Config.HideZeroValues = true
	litter.Config.HidePrivateFields = true
	// I would say lets save the stages to a file for debugging and future use
	// we don't fail if we cant write the file
	if config.DefaultConfig.Stage == "" {
		_ = os.WriteFile(fmt.Sprintf("/etc/kairos/kairos-init-%s.yaml", config.DefaultConfig.Stage), []byte(runStages.ToString()), 0644)
	} else {
		_ = os.WriteFile("/etc/kairos/kairos-init.yaml", []byte(runStages.ToString()), 0644)
	}

}
