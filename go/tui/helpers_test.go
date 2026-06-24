package tui

import (
	"testing"

	"owd-cli/bridge"
)

func TestCatalogSourceLabel(t *testing.T) {
	tests := []struct {
		name         string
		item         bridge.CatalogEntry
		localGitDirs map[string]bool
		want         string
	}{
		{
			name: "not installed",
			item: bridge.CatalogEntry{
				ShortName: "module-foo",
				Installed: false,
			},
			want: "—",
		},
		{
			name: "installed via npm",
			item: bridge.CatalogEntry{
				ShortName:     "module-foo",
				Installed:     true,
				InPackageJson: true,
			},
			want: "npm",
		},
		{
			name: "local source with git",
			item: bridge.CatalogEntry{
				ShortName:   "module-foo",
				Installed:   true,
				LocalSource: true,
			},
			localGitDirs: map[string]bool{"module-foo": true},
			want:         "git",
		},
		{
			name: "local source without git",
			item: bridge.CatalogEntry{
				ShortName:   "module-foo",
				Installed:   true,
				LocalSource: true,
			},
			localGitDirs: map[string]bool{},
			want:         "dev",
		},
		{
			name: "installed in config but missing folder",
			item: bridge.CatalogEntry{
				ShortName:     "module-foo",
				Installed:     true,
				LocalSource:   false,
				InPackageJson: false,
			},
			want: "—",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := catalogSourceLabel(tt.item, tt.localGitDirs)
			if got != tt.want {
				t.Fatalf("catalogSourceLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
