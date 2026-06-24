package tui

import (
	"os"
	"path/filepath"
	"testing"

	"owd-cli/bridge"
)

func TestIsNpmVersion(t *testing.T) {
	if !isNpmVersion("^1.2.0") {
		t.Fatal("semver should be npm")
	}
	if !isNpmVersion("latest") {
		t.Fatal("latest should be npm")
	}
	if isNpmVersion("workspace:*") {
		t.Fatal("workspace should not be npm")
	}
}

func TestReadDesktopPackageDeps(t *testing.T) {
	dir := t.TempDir()
	pkgPath := filepath.Join(dir, "package.json")
	content := `{
  "dependencies": {
    "@owdproject/app-about": "workspace:*",
    "@owdproject/theme-nova": "^1.0.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	deps, err := readDesktopPackageDeps(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	if deps["@owdproject/app-about"] != "workspace:*" {
		t.Fatalf("unexpected workspace dep: %v", deps)
	}
	if deps["@owdproject/theme-nova"] != "^1.0.0" {
		t.Fatalf("unexpected npm dep: %v", deps)
	}
}

func TestBuildQueueFromConfigWorkspaceAndNpm(t *testing.T) {
	dir := t.TempDir()
	desktopDir := filepath.Join(dir, "desktop")
	if err := os.MkdirAll(desktopDir, 0755); err != nil {
		t.Fatal(err)
	}
	pkgPath := filepath.Join(desktopDir, "package.json")
	content := `{
  "dependencies": {
    "@owdproject/app-about": "workspace:*",
    "@owdproject/theme-nova": "^1.2.0"
  }
}`
	if err := os.WriteFile(pkgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	theme := "@owdproject/theme-nova"
	ctx := &bridge.WorkspaceContext{
		Config: bridge.Config{
			Theme: &theme,
			Apps:  []string{"@owdproject/app-about"},
		},
		Paths: bridge.WorkspacePaths{PackageJson: pkgPath},
	}

	var logs []string
	q := BuildQueueFromConfig(dir, ctx, nil, func(s string) { logs = append(logs, s) })
	if q.TotalCount() != 2 {
		t.Fatalf("expected 2 queue items, got %d", q.TotalCount())
	}
	about := q.FindByName("@owdproject/app-about")
	if about == nil || about.Source != SourcePending {
		t.Fatalf("expected pending workspace item, got %+v", about)
	}
	nova := q.FindByName("@owdproject/theme-nova")
	if nova == nil || nova.Kind != KindNpm {
		t.Fatalf("expected npm item, got %+v", nova)
	}
}
