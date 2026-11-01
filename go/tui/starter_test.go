package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStarterPickerSelectionAndCancel(t *testing.T) {
	initial := starterPicker{}
	if !strings.Contains(initial.View(), "Nuxt") || !strings.Contains(initial.View(), "React") {
		t.Fatal("picker must show both starter choices")
	}
	for _, label := range []string{"Vue", "Svelte", "Angular", "Astro"} {
		if !strings.Contains(initial.View(), label) {
			t.Fatalf("picker must show planned %s host", label)
		}
	}
	model, _ := initial.Update(tea.KeyMsg{Type: tea.KeyDown})
	model, quit := model.(starterPicker).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.(starterPicker).choice != "react" || quit == nil {
		t.Fatal("down and enter must select React and finish")
	}
	model, quit = initial.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.(starterPicker).choice != "" || quit == nil {
		t.Fatal("escape must cancel without choosing a starter")
	}
	planned := starterPicker{selected: 3}
	model, quit = planned.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.(starterPicker).choice != "" || quit != nil || !strings.Contains(model.(starterPicker).notice, "not available") {
		t.Fatal("planned Svelte host must explain its availability without exiting")
	}
}

func TestStarterDirectorySafety(t *testing.T) {
	dir := t.TempDir()
	empty := starterForDirectory(dir)
	if empty.blocked || !strings.Contains(empty.View(), "Empty folder") || !strings.Contains(empty.View(), dir) {
		t.Fatal("empty current folder should show its path and readiness")
	}
	if err := os.WriteFile(filepath.Join(dir, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	occupied := starterForDirectory(dir)
	updated, quit := occupied.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !occupied.blocked || quit != nil || updated.(starterPicker).choice != "" {
		t.Fatal("nonempty folder must not be initialized")
	}
}
