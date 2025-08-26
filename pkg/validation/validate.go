package validation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/joho/godotenv"
	"github.com/kairos-io/kairos-init/pkg/config"
	"github.com/kairos-io/kairos-init/pkg/system"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
)

type Validator struct {
	Log    types.KairosLogger
	System values.System
}

func NewValidator(logger types.KairosLogger) *Validator {
	sis := system.DetectSystem(logger)
	return &Validator{Log: logger, System: sis}
}

// TODO: Validate fips, if enabled, check go binaries for boringcrypto

func (v *Validator) Validate() error {
	var multi *multierror.Error

	binaries := []string{
		"immucore",
		"kairos-agent",
		"sudo",
		"less",
		"kcrypt-discovery-challenger",
		"mount.nfs",
	}

	if config.DefaultConfig.Variant == "standard" {
		binaries = append(binaries, "agent-provider-kairos", "kairos", "edgevpn")
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
			multi = multierror.Append(multi, fmt.Errorf("[BINARIES] could not find binary %s", binary))
		} else {
			v.Log.Logger.Info().Str("path", path).Str("binary", binary).Msg("[BINARIES] Found binary")
			// Check if the binary is executable
			info, err := os.Stat(path)
			if err != nil {
				multi = multierror.Append(multi, fmt.Errorf("[BINARIES] could not stat binary %s: %s", binary, err))
			}
			if info.Mode()&0111 == 0 {
				multi = multierror.Append(multi, fmt.Errorf("[BINARIES] binary %s is not executable", binary))
			} else {
				v.Log.Logger.Info().Str("binary", binary).Msg("[BINARIES] Binary is executable")
			}
		}
	}

	// Restore the path
	_ = os.Setenv("PATH", originalPath)

	checkFiles := []string{"/boot/vmlinuz"}
	if !config.DefaultConfig.TrustedBoot {
		checkFiles = append(checkFiles, "/boot/initrd")
	}
	for _, f := range checkFiles {
		s, err := os.Lstat(f)
		if err != nil {
			multi = multierror.Append(multi, fmt.Errorf("[FILES] file missing %s", f))
			continue
		}
		v.Log.Logger.Info().Str("file", f).Msg("Found file")
		// Check if its a symlink in the vmlinuz case
		if s != nil && s.Mode()&os.ModeSymlink != 0 && f == "/boot/vmlinuz" {
			// check if it resolves correctly
			target, err := os.Readlink(f)
			if err != nil {
				multi = multierror.Append(multi, fmt.Errorf("%s symlink is not a valid symlink", f))
				continue
			} else {
				v.Log.Logger.Info().Str("file", f).Msg("File is a symlink and resolves as expected")
			}
			if _, err = os.Stat(target); os.IsNotExist(err) {
				multi = multierror.Append(multi, fmt.Errorf("[FILES] symlink %s points to a non-existent file %s", f, target))
			} else {
				v.Log.Logger.Info().Str("target", target).Msg("Symlink points to a valid file")
			}

		} else {
			v.Log.Logger.Info().Str("file", f).Msg("File is not a symlink")
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
		"KAIROS_BUG_REPORT_URL", // Not critical
		"KAIROS_HOME_URL",       // Not critical
		"KAIROS_RELEASE",
	}

	vals, err := godotenv.Read("/etc/kairos-release")
	if err != nil {
		multi = multierror.Append(multi, fmt.Errorf("[RELEASE] could not open kairos-release file"))
	} else {
		for _, key := range keys {
			if vals[key] == "" {
				multi = multierror.Append(multi, fmt.Errorf("[RELEASE] key %s not found or empty in kairos-release", key))
			}
		}
	}

	if config.DefaultConfig.Variant == "standard" {
		if vals["KAIROS_VARIANT"] != "standard" {
			multi = multierror.Append(multi, fmt.Errorf("[RELEASE] KAIROS_VARIANT is not standard"))
		}
		if vals["KAIROS_SOFTWARE_VERSION"] == "" {
			multi = multierror.Append(multi, fmt.Errorf("[RELEASE] KAIROS_SOFTWARE_VERSION is empty"))
		}
		if vals["KAIROS_SOFTWARE_VERSION_PREFIX"] == "" {
			multi = multierror.Append(multi, fmt.Errorf("[RELEASE] KAIROS_SOFTWARE_VERSION_PREFIX is empty"))
		}
	}

	ExpectedDirs := []string{"/var/lock"}

	for _, dir := range ExpectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			multi = multierror.Append(multi, fmt.Errorf("[DIRS] directory %s does not exist", dir))
		}
	}

	// Check if initrd contains the necessary binaries
	// Do it at the ends as its the slowest check
	if !config.DefaultConfig.TrustedBoot {
		// check dracut
		if _, err := exec.LookPath("lsinitrd"); err != nil {
			v.Log.Logger.Warn().Msg("[INITRD] lsinitrd not found, cannot check initrd contents")
		} else {
			v.Log.Logger.Info().Msg("Checking initrd contents")
			out, err := exec.Command("lsinitrd", "/boot/initrd").CombinedOutput()
			if err != nil {
				multi = multierror.Append(multi, fmt.Errorf("[INITRD] failed checking initrd contents: %s", err))
			}
			for _, binary := range []string{"immucore", "kairos-agent"} {
				if !strings.Contains(string(out), binary) {
					multi = multierror.Append(multi, fmt.Errorf("[INITRD] did not find %s in the initrd", binary))
				} else {
					v.Log.Logger.Info().Str("binary", binary).Msg("Found binary in the initrd")
				}
			}
		}
	}

	// Check if there are any ssh host keys in /etc/ssh
	matches, err := filepath.Glob("/etc/ssh/ssh_host_*_key")
	if err != nil {
		multi = multierror.Append(multi, fmt.Errorf("[SSH] error checking for SSH host keys: %s", err))
	}
	if len(matches) > 0 {
		multi = multierror.Append(multi, fmt.Errorf("[SSH] found SSH host keys in the system: %v", matches))
	} else {
		v.Log.Logger.Info().Msg("No SSH host keys found bundled in the system")
	}

	// Check RHEL family specific service validations
	if err := v.ValidateRHELServices(); err != nil {
		multi = multierror.Append(multi, err)
	}

	if multi.ErrorOrNil() == nil {
		v.Log.Logger.Info().Msg("System validation passed")
	}

	return multi.ErrorOrNil()
}

// ValidateRHELServices checks that critical systemd services are not masked on RHEL family systems
func (v *Validator) ValidateRHELServices() error {
	return v.ValidateRHELServicesWithPath("/etc/systemd/system")
}

// ValidateRHELServicesWithPath checks that critical systemd services exist and are not masked on RHEL family systems
// This method is used for testing by allowing a custom systemd system directory path
func (v *Validator) ValidateRHELServicesWithPath(systemdSystemPath string) error {
	var multi *multierror.Error

	if v.System.Family != values.RedHatFamily {
		// Not a RHEL family system, skip validation
		return nil
	}

	v.Log.Logger.Info().Msg("Checking RHEL family service validations")
	services := []string{"systemd-udevd", "systemd-logind"}

	for _, service := range services {
		servicePath := filepath.Join(systemdSystemPath, fmt.Sprintf("%s.service", service))

		// Check if service file exists
		if _, err := os.Lstat(servicePath); os.IsNotExist(err) {
			// Service doesn't exist at all - this is an error
			multi = multierror.Append(multi, fmt.Errorf("[SERVICES] service %s does not exist on RHEL family system", service))
			continue
		} else if err != nil {
			// Some other error occurred
			multi = multierror.Append(multi, fmt.Errorf("[SERVICES] error checking service %s: %s", service, err))
			continue
		}

		// Service exists, now check if it's masked (symlink to /dev/null)
		if target, err := os.Readlink(servicePath); err == nil && target == "/dev/null" {
			multi = multierror.Append(multi, fmt.Errorf("[SERVICES] service %s is masked on RHEL family system", service))
		} else {
			v.Log.Logger.Info().Str("service", service).Msg("Service exists and is not masked")
		}
	}

	return multi.ErrorOrNil()
}
