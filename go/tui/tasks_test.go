package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOptionalPrepareModules(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "called")
	if err := os.WriteFile(filepath.Join(binDir, "pnpm"), []byte("#!/bin/sh\nprintf called > \"$OWD_PREPARE_MARKER\"\nexit 9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("OWD_PREPARE_MARKER", marker)
	runtime := &RuntimeState{msgChan: make(chan tea.Msg, 20)}
	manifest := filepath.Join(root, "package.json")
	if err := os.WriteFile(manifest, []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.runOptionalPrepareModules(root); err != nil {
		t.Fatalf("starter without prepare script must succeed: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("pnpm should not be called without the script")
	}
	if err := os.WriteFile(manifest, []byte(`{"scripts":{"prepare:modules":"echo prepare"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.runOptionalPrepareModules(root); err == nil {
		t.Fatal("a failing declared prepare script must remain a failure")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected pnpm to be called: %v", err)
	}
}
