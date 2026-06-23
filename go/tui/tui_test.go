package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWipeDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "owd-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test structure:
	// - tmpDir/sub1 (dir)
	// - tmpDir/sub2 (dir)
	// - tmpDir/keep1 (dir)
	// - tmpDir/keep2 (dir)
	// - tmpDir/.gitkeep (file)
	// - tmpDir/file.txt (file)

	dirs := []string{"sub1", "sub2", "keep1", "keep2"}
	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitkeep"), []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("some text"), 0644); err != nil {
		t.Fatal(err)
	}

	preserve := map[string]bool{
		"keep1": true,
		"keep2": true,
	}

	if err := wipeDirectory(tmpDir, preserve); err != nil {
		t.Fatal(err)
	}

	// Verify results:
	// - sub1 and sub2 must be deleted.
	// - keep1 and keep2 must be preserved.
	// - .gitkeep and file.txt must be preserved (since they are files).
	checkNotExist := []string{"sub1", "sub2"}
	for _, d := range checkNotExist {
		if _, err := os.Stat(filepath.Join(tmpDir, d)); !os.IsNotExist(err) {
			t.Errorf("Expected %s to be deleted, but it exists", d)
		}
	}

	checkExist := []string{"keep1", "keep2", ".gitkeep", "file.txt"}
	for _, f := range checkExist {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); os.IsNotExist(err) {
			t.Errorf("Expected %s to exist, but it was deleted", f)
		}
	}
}
