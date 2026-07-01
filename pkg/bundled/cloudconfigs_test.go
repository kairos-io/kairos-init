package bundled_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kairos-io/kairos-init/pkg/bundled"
	"github.com/kairos-io/kairos-sdk/constants"
)

// agentExecPathRe matches absolute-path kairos-agent invocations in bundled
// cloud-config systemd units and inittab entries (ExecStart, respawn, etc.).
var agentExecPathRe = regexp.MustCompile(`(?:ExecStart=|respawn:|systemd-inhibit )(/[^\s"']*kairos-agent)`)

func TestBundledCloudConfigsUseAgentDefaultPath(t *testing.T) {
	t.Helper()

	entries, err := bundled.EmbeddedConfigs.ReadDir("cloudconfigs")
	if err != nil {
		t.Fatalf("read embedded cloudconfigs: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		content, err := bundled.EmbeddedConfigs.ReadFile(filepath.Join("cloudconfigs", name))
		if err != nil {
			t.Fatalf("read cloudconfig %s: %v", name, err)
		}

		for lineNum, line := range strings.Split(string(content), "\n") {
			matches := agentExecPathRe.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) < 2 {
					continue
				}
				path := match[1]
				if path != constants.AgentDefaultPath {
					t.Errorf("%s:%d: absolute kairos-agent path %q must be %q",
						name, lineNum+1, path, constants.AgentDefaultPath)
				}
			}
		}
	}
}
