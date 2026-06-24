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

func TestNewModelSetupState(t *testing.T) {
	model := NewModel("test-root")
	if model.setupInProgress {
		t.Error("Expected setupInProgress to be false initially")
	}
	if model.setupAdds == nil {
		t.Error("Expected setupAdds to be initialized")
	}
	if model.setupCloned == nil {
		t.Error("Expected setupCloned to be initialized")
	}
}

func TestSetupMessages(t *testing.T) {
	model := NewModel("test-root")

	// Test setupClonedMsg updates setupCloned map and setupStep
	m2, _ := (&model).Update(setupClonedMsg{Cloned: []string{"pkg-a", "pkg-b"}, Step: 3})
	updatedModel := m2.(*TuiModel)
	if updatedModel.setupStep != 3 {
		t.Errorf("Expected setupStep to be 3, got %d", updatedModel.setupStep)
	}
	if !updatedModel.setupCloned["pkg-a"] || !updatedModel.setupCloned["pkg-b"] {
		t.Error("Expected cloned packages to be registered in setupCloned")
	}

	// Test depsDetectedMsg pauses setup task and populates promptedModel.wizard
	m3, _ := updatedModel.Update(depsDetectedMsg{Deps: []string{"@owdproject/kit-tailwind"}})
	promptedModel := m3.(*TuiModel)
	if promptedModel.activeTask != TaskNone {
		t.Errorf("Expected activeTask to be TaskNone, got %v", promptedModel.activeTask)
	}
	if !promptedModel.setupInProgress {
		t.Error("Expected setupInProgress to remain true during prompt")
	}
	if promptedModel.wizard == nil {
		t.Fatal("Expected promptedModel.wizard to be initialized")
	}
	if len(promptedModel.wizard.Queue) != 1 {
		t.Errorf("Expected wizard queue size to be 1, got %d", len(promptedModel.wizard.Queue))
	}
	if promptedModel.wizard.Queue[0].PkgName != "@owdproject/kit-tailwind" {
		t.Errorf("Expected queued package name to be @owdproject/kit-tailwind, got %s", promptedModel.wizard.Queue[0].PkgName)
	}
}

