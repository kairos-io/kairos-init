package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExisting(t *testing.T) {
	tests := []struct {
		name string
		// seed lists installer paths (relative to root) to create before the check
		seed      []string
		wantFound bool
		// wantPath is the path relative to root that Existing should return
		wantPath string
	}{
		{
			name:      "no installer present",
			seed:      nil,
			wantFound: false,
		},
		{
			name:      "only default present",
			seed:      []string{DefaultPath},
			wantFound: true,
			wantPath:  DefaultPath,
		},
		{
			name:      "only override present",
			seed:      []string{OverridePath},
			wantFound: true,
			wantPath:  OverridePath,
		},
		{
			name:      "both present, override wins",
			seed:      []string{DefaultPath, OverridePath},
			wantFound: true,
			wantPath:  OverridePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()

			for _, p := range tt.seed {
				full := filepath.Join(root, p)
				if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
					t.Fatalf("failed to create dir for %s: %v", p, err)
				}
				if err := os.WriteFile(full, []byte("installer"), 0755); err != nil {
					t.Fatalf("failed to seed installer %s: %v", p, err)
				}
			}

			got, found := Existing(root)

			if found != tt.wantFound {
				t.Fatalf("Existing() found = %v, want %v", found, tt.wantFound)
			}

			if !tt.wantFound {
				if got != "" {
					t.Fatalf("Existing() path = %q, want empty when not found", got)
				}
				return
			}

			want := filepath.Join(root, tt.wantPath)
			if got != want {
				t.Fatalf("Existing() path = %q, want %q", got, want)
			}
		})
	}
}
