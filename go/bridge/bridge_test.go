package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBridgePathForInstalledCli(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "node_modules", "@owdproject", "cli")
	executable := filepath.Join(packageDir, "go", "desktop-go")
	bridgePath := filepath.Join(packageDir, "bin", "bridge.js")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(bridgePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bridgePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := bridgePathFor(executable, filepath.Join(root, "consumer")); got != bridgePath {
		t.Fatalf("bridgePathFor() = %q, want installed bridge %q", got, bridgePath)
	}
}

func TestBridgePathForMonorepoDevelopment(t *testing.T) {
	root := t.TempDir()
	bridgePath := filepath.Join(root, "packages", "cli", "bin", "bridge.js")
	if err := os.MkdirAll(filepath.Dir(bridgePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bridgePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := bridgePathFor(filepath.Join(root, "go-build", "desktop-go"), root); got != bridgePath {
		t.Fatalf("bridgePathFor() = %q, want workspace bridge %q", got, bridgePath)
	}
}
