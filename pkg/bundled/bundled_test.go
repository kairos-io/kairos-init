package bundled

import (
	"strings"
	"testing"
)

func TestImmucoreDracutConfigEnablesInitramfsNetworking(t *testing.T) {
	const expected = `kernel_cmdline+=" rd.neednet=1 "`

	for line := range strings.Lines(ImmucoreConfigDracut) {
		if strings.TrimSpace(line) == expected {
			return
		}
	}

	t.Fatalf("ImmucoreConfigDracut does not include %q", expected)
}
