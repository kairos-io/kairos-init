package main

import (
	"flag"
	"fmt"
	"github.com/mudler/yip/pkg/schema"
	"os"

	"github.com/kairos-io/kairos-init/pkg/stages"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/sanity-io/litter"
)

func main() {
	level := flag.String("level", "info", "set the log level (shorthand: -l)")
	flag.StringVar(level, "l", "info", "set the log level (shorthand: -l)")
	stage := flag.String("stage", "", "set the stage to run (shorthand: -s)")
	flag.StringVar(stage, "s", "", "set the stage to run (shorthand: -s)")
	flag.Parse()

	logger := types.NewKairosLogger("kairos-init", *level, false)
	logger.Infof("Starting kairos-init version %s", values.GetVersion())
	logger.Debug(values.GetFullVersion())

	var err error
	var runStages schema.YipConfig

	if *stage != "" {
		logger.Infof("Running stage %s", *stage)
		switch *stage {
		case "install":
			runStages, err = stages.RunInstallStage(logger)
		case "init":
			runStages, err = stages.RunInitStage(logger)
		default:
			logger.Errorf("Unknown stage %s", *stage)
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
	if *stage == "" {
		_ = os.WriteFile(fmt.Sprintf("/etc/kairos/kairos-init-%s.yaml", *stage), []byte(runStages.ToString()), 0644)
	} else {
		_ = os.WriteFile("/etc/kairos/kairos-init.yaml", []byte(runStages.ToString()), 0644)
	}

}
