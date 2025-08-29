package stages

import (
	"testing"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
	"github.com/rs/zerolog"
	"os"
)

func TestFilterMultipathDependencies(t *testing.T) {
	logger := types.KairosLogger{Logger: zerolog.New(os.Stdout).With().Logger()}
	
	sis := values.System{
		Distro: values.Ubuntu,
		Family: values.DebianFamily,
		Version: "22.04",
		Arch: values.ArchAMD64,
	}
	
	// Test packages that include dracut packages that should be preserved
	inputPackages := []string{
		"dracut",           // Should be preserved
		"dracut-network",   // Should be preserved
		"dracut-live",      // Should be preserved
		"isc-dhcp-common",  // Should be removed
		"isc-dhcp-client",  // Should be removed
		"cloud-guest-utils", // Should be removed
	}
	
	expectedPreserved := []string{
		"isc-dhcp-common",
		"isc-dhcp-client", 
		"cloud-guest-utils",
	}
	
	result := filterMultipathDependencies(inputPackages, sis, logger)
	
	// Check that dracut packages were filtered out (preserved)
	if len(result) != len(expectedPreserved) {
		t.Errorf("Expected %d packages after filtering, got %d", len(expectedPreserved), len(result))
	}
	
	// Check that the right packages remain
	for _, expected := range expectedPreserved {
		found := false
		for _, actual := range result {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected package %s to remain after filtering, but it was removed", expected)
		}
	}
	
	// Check that dracut packages were indeed filtered out
	for _, pkg := range []string{"dracut", "dracut-network", "dracut-live"} {
		for _, actual := range result {
			if actual == pkg {
				t.Errorf("Package %s should have been preserved (filtered out), but it's still in the removal list", pkg)
			}
		}
	}
}

func TestFilterMultipathDependenciesEmptyInput(t *testing.T) {
	logger := types.KairosLogger{Logger: zerolog.New(os.Stdout).With().Logger()}
	
	sis := values.System{
		Distro: values.Ubuntu,
		Family: values.DebianFamily,
		Version: "22.04",
		Arch: values.ArchAMD64,
	}
	
	result := filterMultipathDependencies([]string{}, sis, logger)
	
	if len(result) != 0 {
		t.Errorf("Expected empty result for empty input, got %v", result)
	}
}