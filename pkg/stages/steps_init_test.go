package stages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types/logger"
)

func newTestLogger() logger.KairosLogger {
	return logger.NewKairosLogger("test", "info", false)
}

// mkdirs creates a set of sub-directories under base.
func mkdirs(t *testing.T, base string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.Mkdir(filepath.Join(base, n), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
	}
}

func TestGetLatestKernelFromPath(t *testing.T) {
	log := newTestLogger()

	tests := []struct {
		name        string
		model       string
		dirs        []string // kernel directories to create
		wantKernel  string   // exact expected return value ("" = don't check)
		wantErr     bool
		errContains string
	}{
		// ── generic model ──────────────────────────────────────────────────────
		{
			name:        "no kernel directories → error",
			model:       values.Generic.String(),
			dirs:        []string{},
			wantErr:     true,
			errContains: "no kernel versions found",
		},
		{
			// go-version parses "5.15.0-101-generic" and String() returns the
			// original version string as provided.
			name:       "single semver kernel",
			model:      values.Generic.String(),
			dirs:       []string{"5.15.0-101-generic"},
			wantKernel: "5.15.0-101-generic",
		},
		{
			name:  "multiple semver kernels → highest selected",
			model: values.Generic.String(),
			dirs:  []string{"5.15.0-100-generic", "5.15.0-102-generic", "5.15.0-101-generic"},
			// 102 > 101 > 100, so 5.15.0-102-generic is returned
			wantKernel: "5.15.0-102-generic",
		},
		{
			name:  "non-semver kernel name → returned as-is (first entry fallback)",
			model: values.Generic.String(),
			dirs:  []string{"5.4.0-101-generic.fc32.x86_64"},
			// non-semver: falls back to dirs[0].Name()
			wantKernel: "5.4.0-101-generic.fc32.x86_64",
		},
		{
			name:  "multiple non-semver kernels → first directory entry returned",
			model: values.Generic.String(),
			dirs:  []string{"alpha-kernel", "beta-kernel"},
			// os.ReadDir returns entries sorted by name, so "alpha-kernel" is first
			wantKernel: "alpha-kernel",
		},
		// ── RPi4 model ────────────────────────────────────────────────────────
		{
			name:       "rpi4: single raspi semver kernel",
			model:      values.Rpi4.String(),
			dirs:       []string{"5.15.0-1025-raspi"},
			wantKernel: "5.15.0-1025-raspi",
		},
		{
			name:  "rpi4: multiple raspi semver kernels → highest selected",
			model: values.Rpi4.String(),
			dirs:  []string{"5.15.0-1023-raspi", "5.15.0-1025-raspi", "5.15.0-1024-raspi"},
			// 1025 > 1024 > 1023
			wantKernel: "5.15.0-1025-raspi",
		},
		{
			name:  "rpi4: raspi preferred over generic even when generic is higher",
			model: values.Rpi4.String(),
			dirs:  []string{"6.8.0-51-generic", "5.15.0-1025-raspi"},
			// raspi wins regardless of version comparison with generic
			wantKernel: "5.15.0-1025-raspi",
		},
		{
			name:  "rpi4: raspi non-semver name → lexicographic last returned",
			model: values.Rpi4.String(),
			dirs:  []string{"custom-b-raspi", "custom-a-raspi"},
			// no semver parse succeeds; raspiFallback sorted → "custom-b-raspi" last
			wantKernel: "custom-b-raspi",
		},
		{
			name:       "rpi4: no raspi dir → falls through to generic semver selection",
			model:      values.Rpi4.String(),
			dirs:       []string{"5.15.0-101-generic", "5.15.0-102-generic"},
			wantKernel: "5.15.0-102-generic",
		},
		// ── RPi3 model (same logic as RPi4) ───────────────────────────────────
		{
			name:       "rpi3: raspi kernel preferred",
			model:      values.Rpi3.String(),
			dirs:       []string{"5.15.0-1025-raspi", "6.8.0-50-generic"},
			wantKernel: "5.15.0-1025-raspi",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			mkdirs(t, base, tc.dirs...)

			got, err := GetLatestKernelFromPath(base, tc.model, log)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (kernel=%q)", tc.errContains, got)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantKernel != "" && got != tc.wantKernel {
				t.Errorf("got kernel %q, want %q", got, tc.wantKernel)
			}
		})
	}
}
