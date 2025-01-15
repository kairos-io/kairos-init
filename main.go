package main

import (
	"flag"
	"os"

	"github.com/kairos-io/kairos-init/pkg/stages"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/sanity-io/litter"
)

func main() {
	level := flag.String("level", "info", "set the log level (shorthand: -l)")
	flag.StringVar(level, "l", "info", "set the log level (shorthand: -l)")
	flag.Parse()

	logger := types.NewKairosLogger("kairos-init", *level, false)
	logger.Infof("Starting kairos-init version %s", values.GetVersion())
	logger.Debug(values.GetFullVersion())

	runStages, err := stages.RunAllStages(logger)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

	litter.Config.HideZeroValues = true
	litter.Config.HidePrivateFields = true
	logger.Logger.Debug().Str("stages", litter.Sdump(runStages)).Msg("Done")
	// I would say lets save the stages to a file for debugging and future use
	// we don't fail if we cant write the file
	_ = os.WriteFile("/etc/kairos/kairos-init.yaml", []byte(runStages.ToString()), 0644)
}
