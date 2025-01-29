package main

import (
	"flag"
	"fmt"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/stages"
	"github.com/kairos-io/kairos-init/pkg/validation"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
	"github.com/sanity-io/litter"
	"os"
	"strings"
)

func main() {
	var trusted string
	var validate bool
	flag.StringVar(&config.DefaultConfig.Level, "l", "info", "set the log level")
	flag.StringVar(&config.DefaultConfig.Stage, "s", "all", "set the stage to run")
	flag.StringVar(&config.DefaultConfig.Model, "m", "generic", "model to build for, like generic or rpi4")
	flag.StringVar(&config.DefaultConfig.Variant, "v", "core", "variant to build (core or standard for k3s flavor) (shorthand: -v)")
	flag.StringVar(&config.DefaultConfig.Registry, "r", "quay.io/kairos", "registry and org where the image is gonna be pushed. This is mainly used on upgrades to search for available images to upgrade to")
	flag.StringVar(&trusted, "t", "false", "init the system for Trusted Boot, changes bootloader to systemd")
	flag.StringVar(&config.DefaultConfig.FrameworkVersion, "f", values.GetFrameworkVersion(), "set the framework version to use")
	flag.BoolVar(&validate, "validate", false, "validate the running os to see if it all the pieces are in place")
	showHelp := flag.Bool("help", false, "show help")

	// Custom usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.VisitAll(func(f *flag.Flag) {
			if f.Name != "cpuprofile" && f.Name != "memprofile" && f.Name != "stubs" && f.Name != "help" && f.Name != "pkg" && f.Name != "log" && f.Name != "e" && f.Name != "out" {
				fmt.Fprintf(os.Stderr, "  -%s: %s (default: %s)\n", f.Name, f.Usage, f.DefValue)
			}
		})
	}

	flag.Parse()

	// Set the trusted boot flag to true
	if strings.ToLower(trusted) == "true" || strings.ToLower(trusted) == "1" {
		config.DefaultConfig.TrustedBoot = true
	}

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	logger := types.NewKairosLogger("kairos-init", config.DefaultConfig.Level, false)
	logger.Infof("Starting kairos-init version %s", values.GetVersion())
	logger.Debug(litter.Sdump(values.GetFullVersion()))
	logger.Debug(litter.Sdump(config.DefaultConfig))

	var err error
	var runStages schema.YipConfig

	if validate {
		validator := validation.NewValidator(logger)
		err = validator.Validate()
		if err != nil {
			logger.Error(err)
			os.Exit(1)
		}
		os.Exit(0)
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
			logger.Errorf("Unknown stage %s. Valid values are install, init and all", config.DefaultConfig.Stage)
			os.Exit(1)
		}
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
