package validation

import (
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/joho/godotenv"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/system"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"os"
	"os/exec"
	"strings"
)

type Validator struct {
	Log    types.KairosLogger
	System values.System
}

func NewValidator(logger types.KairosLogger) *Validator {
	sis := system.DetectSystem(logger)
	return &Validator{Log: logger, System: sis}
}

func (v *Validator) Validate() error {
	var multi *multierror.Error

	binaries := []string{
		"immucore",
		"kairos-agent",
		"sudo",
		"less",
		"kcrypt",
		"kcrypt-discovery-challenger",
	}

	if config.DefaultConfig.Variant == "standard" {
		binaries = append(binaries, "agent-provider-kairos", "kairos")
		if config.DefaultConfig.KubernetesProvider == config.K3sProvider {
			binaries = append(binaries, "k3s")
		}
		if config.DefaultConfig.KubernetesProvider == config.K0sProvider {
			binaries = append(binaries, "k0s")
		}
	}

	// Alter path to include our providers path
	originalPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", fmt.Sprintf("%s:%s:%s", "/system/providers/", "/system/discovery/", originalPath))
	// Check binaries
	for _, binary := range binaries {
		path, err := exec.LookPath(binary)
		if err != nil {
			multi = multierror.Append(multi, fmt.Errorf("could not find binary %s", binary))
		}
		v.Log.Logger.Info().Str("path", path).Str("binary", binary).Msg("Found binary")
	}

	// Restore the path
	_ = os.Setenv("PATH", originalPath)

	checkfiles := []string{"/boot/vmlinuz"}
	if !config.DefaultConfig.TrustedBoot {
		checkfiles = append(checkfiles, "/boot/initrd")
	}
	for _, f := range checkfiles {
		s, err := os.Lstat(f)
		if err != nil {
			multi = multierror.Append(multi, fmt.Errorf("file missing %s", f))
			continue
		}
		v.Log.Logger.Info().Str("file", f).Msg("Found file")
		// Check if its a symlink in the vmlinuz case
		if s != nil && s.Mode()&os.ModeSymlink != 0 && f == "/boot/vmlinuz" {
			// check if it resolves correctly
			_, err = os.Readlink(f)
			if err != nil {
				multi = multierror.Append(multi, fmt.Errorf("%s symlink is not a valid symlink", f))
				continue
			} else {
				v.Log.Logger.Info().Str("file", f).Msg("File is a symlink and resolves as expected")
			}
		} else {
			v.Log.Logger.Info().Str("file", f).Msg("File is not a symlink")
		}
	}

	// Check services are there
	if v.System.Family != values.AlpineFamily {
		services := []string{
			"kairos-agent",
			"kairos-interactive",
			"kairos-recovery",
			"kairos-reset",
			"kairos-webui",
			"kairos",
		}

		if config.DefaultConfig.Variant == "standard" {
			switch config.DefaultConfig.KubernetesProvider {
			case config.K3sProvider:
				services = append(services, "k3s", "k3s-agent")
			case config.K0sProvider:
				services = append(services, "k0scontroller", "k0sworker")
			}
		}
		for _, service := range services {
			_, err := os.Stat(fmt.Sprintf("/etc/systemd/system/%s.service", service))
			if err != nil {
				multi = multierror.Append(multi, fmt.Errorf("service %s not found", service))
			} else {
				v.Log.Logger.Info().Str("service", service).Msg("Found service")
			}

		}
	}

	// Validate all needed keys are stored in kairos-release
	keys := []string{
		"KAIROS_ID",
		"KAIROS_ID_LIKE", // Maybe not critical? Same as name below
		"KAIROS_NAME",
		"KAIROS_VERSION",
		"KAIROS_ARCH",
		"KAIROS_TARGETARCH", // Not critical, same as ARCH above
		"KAIROS_FLAVOR",
		"KAIROS_FLAVOR_RELEASE",
		"KAIROS_FAMILY",
		"KAIROS_MODEL",
		"KAIROS_VARIANT",
		"KAIROS_REGISTRY_AND_ORG",
		"KAIROS_BUG_REPORT_URL", // Not critical
		"KAIROS_HOME_URL",       // Not critical
		"KAIROS_RELEASE",
		"KAIROS_IMAGE_LABEL",
	}

	vals, err := godotenv.Read("/etc/kairos-release")
	if err != nil {
		multi = multierror.Append(multi, fmt.Errorf("could not open kairos-release file"))
	} else {
		for _, key := range keys {
			if vals[key] == "" {
				multi = multierror.Append(multi, fmt.Errorf("key %s not found or empty in kairos-release", key))
			}
		}
	}

	if config.DefaultConfig.Variant == "standard" {
		if vals["KAIROS_VARIANT"] != "standard" {
			multi = multierror.Append(multi, fmt.Errorf("KAIROS_VARIANT is not standard"))
		}
		if vals["KAIROS_SOFTWARE_VERSION"] == "" {
			multi = multierror.Append(multi, fmt.Errorf("KAIROS_SOFTWARE_VERSION is empty"))
		}
		if vals["KAIROS_SOFTWARE_VERSION_PREFIX"] == "" {
			multi = multierror.Append(multi, fmt.Errorf("KAIROS_SOFTWARE_VERSION_PREFIX is empty"))
		}
	}

	// Check if initrd contains the necessary binaries
	// Do it at the ends as its the slowest check
	if !config.DefaultConfig.TrustedBoot {
		// check dracut
		if _, err := exec.LookPath("lsinitrd"); err != nil {
			v.Log.Logger.Warn().Msg("lsinitrd not found, cannot check initrd contents")
		} else {
			v.Log.Logger.Info().Msg("Checking initrd contents")
			out, err := exec.Command("lsinitrd", "/boot/initrd").CombinedOutput()
			if err != nil {
				return err
			}
			for _, binary := range []string{"immucore", "kairos-agent"} {
				if !strings.Contains(string(out), binary) {
					multi = multierror.Append(multi, fmt.Errorf("did not found %s in the initrd", binary))
				} else {
					v.Log.Logger.Info().Str("binary", binary).Msg("Found binary in the initrd")
				}
			}
		}
	}

	return multi.ErrorOrNil()
}
