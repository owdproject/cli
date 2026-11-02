package tui

import (
	"os"
	"path/filepath"
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

func TestActiveThemeName(t *testing.T) {
	nova := "@owdproject/nuxt-theme-nova"
	gnome := "@owdproject/theme-gnome"
	ctx := &bridge.WorkspaceContext{
		Config: bridge.Config{Theme: &nova},
	}

	if got := activeThemeName(ctx, nil); got != nova {
		t.Fatalf("expected saved theme %q, got %q", nova, got)
	}

	if got := activeThemeName(ctx, &gnome); got != gnome {
		t.Fatalf("expected pending theme %q, got %q", gnome, got)
	}

	if got := activeThemeName(nil, &gnome); got != gnome {
		t.Fatalf("expected pending theme without ctx %q, got %q", gnome, got)
	}

	if got := activeThemeName(nil, nil); got != "" {
		t.Fatalf("expected empty theme, got %q", got)
	}
}

func TestLogScrollClampsToWindow(t *testing.T) {
	m := &TuiModel{logLines: []string{"a", "b", "c", "d", "e"}, logViewHeight: 2}

	if got := m.maxLogScroll(); got != 3 {
		t.Fatalf("maxLogScroll = %d, want 3", got)
	}

	m.scrollLogs(100)
	if m.logScrollOffset != 3 {
		t.Fatalf("scroll up clamp = %d, want 3", m.logScrollOffset)
	}

	m.scrollLogs(-100)
	if m.logScrollOffset != 0 {
		t.Fatalf("scroll down clamp = %d, want 0", m.logScrollOffset)
	}

	// Empty buffer and taller viewport never produce a negative max.
	empty := &TuiModel{logLines: nil, logViewHeight: 10}
	if got := empty.maxLogScroll(); got != 0 {
		t.Fatalf("empty maxLogScroll = %d, want 0", got)
	}
}

func TestGetVersionsReadsProjectFramework(t *testing.T) {
	dir := t.TempDir()
	pkg := "{\n  \"packageManager\": \"pnpm@11.9.0\",\n  \"devDependencies\": {\n    \"react\": \"^19.0.0\"\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}
	vers := getVersions(dir, dir, "react")
	if vers.Framework != "19.0.0" {
		t.Fatalf("Framework = %q, want 19.0.0", vers.Framework)
	}
	if vers.Pnpm != "11.9.0" {
		t.Fatalf("Pnpm = %q, want 11.9.0", vers.Pnpm)
	}
	if frameworkLabel("react") != "React" {
		t.Fatalf("frameworkLabel = %q, want React", frameworkLabel("react"))
	}
}
