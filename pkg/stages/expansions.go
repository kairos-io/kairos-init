package stages

// This allows to load stage expansions from a dir in the filesystem
// The structure is as follows:
// We got a base dir which is /tmp/kairos-init/
// (this is the default, but you can override it using the KAIROS_INIT_EXPANSIONS_DIR env var)
// Inside this dir we have the files we want to add to the stage
// This are simple yip files (https://github.com/mudler/yip)
// as usual, they will be loaded and executed in lexicographic order
// so for example, if we have:
// /tmp/kairos-init/10-foo.yip
// /tmp/kairos-init/20-bar.yip
// /tmp/kairos-init/30-baz.yip
// The files will be executed in the following order:
// 10-foo.yip
// 20-bar.yip
// 30-baz.yip
// The files are loaded using the yip library, so you can use all the
// features of yip
// The current stages available are:
// - before-install: Good for adding extra repos and such.
// - install: Good for installing packages and such.
// - after-install: Do some cleanup of packages, add extra packages, add different kernels and remove the kairos default one, etc.
// - before-init: Good for adding some dracut modules for example to be added to the initramfs.
// - init: Anything that configures the system, security hardening for example.
// - after-init: Good for rebuilding the initramfs, or adding a different initramfs like a kdump one, add grub configs or branding, etc.

import (
	"os"
	"path/filepath"

	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
	"github.com/sanity-io/litter"
	"github.com/twpayne/go-vfs/v5"
)

// GetStageExpansions returns the expansions for a given stage
// It loads the expansions from a dir in the filesystem, loads all files and only selects the proper stage to be returned
func GetStageExpansions(stage string, logger types.KairosLogger) []schema.Stage {
	var data []schema.Stage
	// If extensions are not enabled, return empty
	if !config.DefaultConfig.Extensions {
		return data
	}

	dir := os.Getenv("KAIROS_INIT_EXPANSIONS_DIR")
	if dir == "" {
		dir = "/tmp/kairos-init"
	}

	logger.Logger.Debug().Str("stage", stage).Str("dir", dir).Msg("getting stage")

	// Go over all the files in order
	// and load them
	litter.Config.HideZeroValues = true
	litter.Config.HidePrivateFields = false
	_ = vfs.Walk(vfs.OSFS, dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}
		if filepath.Ext(info.Name()) != ".yaml" && filepath.Ext(info.Name()) != ".yml" {
			logger.Logger.Debug().Str("file", path).Msg("skipping file due to extension")
			return nil
		}

		logger.Logger.Debug().Str("file", path).Msg("loading file")
		d, err := schema.Load(path, vfs.OSFS, schema.FromFile, nil)
		if err != nil {
			logger.Logger.Error().Str("file", path).Err(err).Msg("error loading file")
			return nil
		}
		for _, s := range d.Stages[stage] {
			logger.Logger.Debug().Str("file", path).Msg("found stage, appending data")
			data = append(data, s)
		}
		return nil
	})

	return data
}
