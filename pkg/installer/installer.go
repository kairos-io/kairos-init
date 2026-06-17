// Package installer holds the contract for where the kairos installer binary
// lives in an image. It is intentionally dependency-free (no embedded binaries)
// so the resolution logic can be unit tested in isolation.
package installer

import (
	"os"
	"path/filepath"
)

// DefaultPath is where kairos-init bundles its embedded installer.
const DefaultPath = "/system/installer/kairos-installer"

// OverridePath is the canonical drop-in slot where the base image or the user
// can provide their own installer. It takes precedence over the bundled default,
// matching kairos-installer's resolution contract
// (see github.com/kairos-io/kairos-installer).
const OverridePath = "/system/installer/installer"

// Existing reports whether an installer is already present under root, returning
// its path. It checks the override slot first and then the default path, mirroring
// the installer's own resolution order. When one is found, kairos-init should skip
// bundling its embedded copy to keep the image surface small.
//
// root is prefixed to the candidate paths so callers can point it at a test
// directory; production callers pass "/".
func Existing(root string) (string, bool) {
	for _, p := range []string{OverridePath, DefaultPath} {
		full := filepath.Join(root, p)
		if _, err := os.Stat(full); err == nil {
			return full, true
		}
	}
	return "", false
}
