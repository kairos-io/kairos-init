package stages

import (
	"fmt"
	"os"
	"sort"

	semver "github.com/hashicorp/go-version"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/mudler/yip/pkg/schema"
	"kairos-init-yip/pkg/values"
)

func getLatestKernel(l types.KairosLogger) (string, error) {
	var kernelVersion string
	modulesPath := "/lib/modules"
	// Read the directories under /lib/modules
	dirs, err := os.ReadDir(modulesPath)
	if err != nil {
		l.Logger.Error().Msgf("Failed to read the directory %s: %s", modulesPath, err)
		return kernelVersion, err

	}

	var versions []*semver.Version

	for _, dir := range dirs {
		if dir.IsDir() {
			// Parse the directory name as a semver version
			version, err := semver.NewVersion(dir.Name())
			if err != nil {
				l.Logger.Error().Msgf("Failed to parse the version %s: %s", dir.Name(), err)
				continue
			}
			versions = append(versions, version)
		}
	}

	sort.Sort(semver.Collection(versions))
	kernelVersion = versions[0].String()
	return kernelVersion, nil
}

func GetKairosReleaseStage(sis values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
		{
			Name: "Write kairos-release",
			Environment: map[string]string{
				"KAIROS_VERSION": sis.Version,
				"KAIROS_ARCH":    sis.Arch.String(),
				"KAIROS_FLAVOR":  sis.Distro.String(),
				"KAIROS_FAMILY":  sis.Family.String(),
				"KAIROS_MODEL":   "generic", // NEEDED or it breaks boot!
				"KAIROS_VARIANT": "core",    // Maybe needed?
			},
			EnvironmentFile: "/etc/kairos-release",
		},
	}
}

func GetInstallStage(sis values.System, logger types.KairosLogger) []schema.Stage {
	// Get the packages
	packages, _ := values.GetPackages(sis, logger)
	// Now parse the packages with the templating engine
	finalMergedPkgs, _ := values.PackageListToTemplate(packages, values.GetTemplateParams(sis), logger)
	return []schema.Stage{
		{
			Name: "Install base packages",
			Packages: schema.Packages{
				Install: finalMergedPkgs,
				Refresh: true,
			},
		},
	}
}

func GetKernelStage(_ values.System, logger types.KairosLogger) []schema.Stage {
	kernel, _ := getLatestKernel(logger)

	return []schema.Stage{
		{
			Name: "Clean current kernel link",
			If:   "test -f /boot/vmlinuz",
			Commands: []string{
				"rm /boot/vmlinuz",
			},
		},
		{
			Name: "Clean old kernel link",
			If:   "test -f /boot/vmlinuz.old",
			Commands: []string{
				"rm /boot/vmlinuz.old",
			},
		},
		{
			Name: "Link kernel",
			Commands: []string{
				fmt.Sprintf("depmod -a %s", kernel),
				fmt.Sprintf("ln -s /boot/vmlinuz-%s /boot/vmlinuz", kernel),
			},
		},
	}
}

func GetInitrdStage(_ values.System, logger types.KairosLogger) []schema.Stage {
	kernel, _ := getLatestKernel(logger)

	return []schema.Stage{
		{
			Name: "Remove all initrds",
			Commands: []string{
				"rm -f /boot/initrd*",
			},
		},
		{
			Name: "Create new initrd",
			Commands: []string{
				fmt.Sprintf("dracut -v -f /boot/initrd %s", kernel),
			},
		},
	}
}

func GetCleanupStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
		{
			Name: "Remove dbus machine-id",
			If:   "test -f /var/lib/dbus/machine-id",
			Commands: []string{
				"rm -f /var/lib/dbus/machine-id",
			},
		},
		{
			Name: "truncate machine-id",
			If:   "test -f /etc/machine-id",
			Commands: []string{
				"truncate -s 0 /etc/machine-id",
			},
		},
	}
}

func GetInstallFrameworkStage(_ values.System, _ types.KairosLogger) []schema.Stage {
	return []schema.Stage{
		{
			Name: "Create kairos directory",
			If:   "test -d /etc/kairos",
			Directories: []schema.Directory{
				{
					Path:        "/etc/kairos",
					Permissions: 0755,
				},
			},
		},
		{
			Name: "Install framework",
			UnpackImages: []schema.UnpackImageConf{
				{
					Source: fmt.Sprintf("quay.io/kairos/framework:%s", values.GetFrameworkVersion()),
					Target: "/",
				},
			},
		},
	}
}

// GetAllStages Returns all the stages in the correct order and in the init stage
// TODO: other stages should be able to return an error so we stop
func GetAllStages(sis values.System, logger types.KairosLogger) schema.YipConfig {
	data := schema.YipConfig{Stages: map[string][]schema.Stage{}}
	data.Stages["init"] = []schema.Stage{}

	data.Stages["init"] = append(data.Stages["init"], GetKairosReleaseStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetInstallStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetKernelStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetInstallFrameworkStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetInitrdStage(sis, logger)...)
	data.Stages["init"] = append(data.Stages["init"], GetCleanupStage(sis, logger)...)

	return data
}
