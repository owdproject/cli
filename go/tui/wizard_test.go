package tui

import (
	"testing"

	"owd-cli/bridge"
)

func TestWizardInitialization(t *testing.T) {
	// 1. Empty wizard
	wEmpty := NewWizard(nil)
	if !wEmpty.IsComplete() {
		t.Error("Expected empty wizard to be complete initially")
	}
	if wEmpty.Current() != nil {
		t.Error("Expected Current() to return nil for empty wizard")
	}

	// 2. Non-empty wizard
	decisions := []pendingDecision{
		{
			PkgName:   "pkg-a",
			ShortName: "pkg-a",
			Action:    "install",
			Kind:      "app",
		},
		{
			PkgName:   "pkg-b",
			ShortName: "pkg-b",
			Action:    "uninstall",
			Kind:      "module",
		},
	}
	w := NewWizard(decisions)
	if w.IsComplete() {
		t.Error("Expected wizard with elements not to be complete initially")
	}
	curr := w.Current()
	if curr == nil || curr.PkgName != "pkg-a" {
		t.Errorf("Expected current to be pkg-a, got %v", curr)
	}
	if w.Index != 0 {
		t.Errorf("Expected index 0, got %d", w.Index)
	}
}

func TestWizardNextAndCurrent(t *testing.T) {
	decisions := []pendingDecision{
		{PkgName: "pkg-a", Action: "install"},
		{PkgName: "pkg-b", Action: "uninstall"},
	}
	w := NewWizard(decisions)

	// Step 1: pkg-a
	if w.IsComplete() {
		t.Error("Should not be complete")
	}
	if w.Current().PkgName != "pkg-a" {
		t.Errorf("Expected pkg-a, got %s", w.Current().PkgName)
	}

	// Move to step 2
	w.Next()
	if w.IsComplete() {
		t.Error("Should not be complete yet")
	}
	if w.Current().PkgName != "pkg-b" {
		t.Errorf("Expected pkg-b, got %s", w.Current().PkgName)
	}

	// Move past end
	w.Next()
	if !w.IsComplete() {
		t.Error("Should be complete")
	}
	if w.Current() != nil {
		t.Errorf("Expected Current() to be nil, got %v", w.Current())
	}

	// Extra Next should do nothing
	w.Next()
	if w.Index != 2 {
		t.Errorf("Expected index to remain at 2, got %d", w.Index)
	}
}

func TestWizardResolveCurrentInstall(t *testing.T) {
	decisions := []pendingDecision{
		{PkgName: "pkg-app", Action: "install", Kind: "app"},
		{PkgName: "pkg-theme", Action: "install", Kind: "theme"},
		{PkgName: "pkg-uninstall", Action: "uninstall", Kind: "app"}, // not an install action
	}
	w := NewWizard(decisions)

	// 1. Resolve first install
	w.ResolveCurrentInstall("ssh")
	if w.FinalizedAdds["pkg-app"] != "ssh" {
		t.Errorf("Expected FinalizedAdds for pkg-app to be 'ssh', got %s", w.FinalizedAdds["pkg-app"])
	}
	if w.FinalizedTheme != nil {
		t.Errorf("Expected FinalizedTheme to be nil, got %v", w.FinalizedTheme)
	}

	w.Next()

	// 2. Resolve theme install
	w.ResolveCurrentInstall("https")
	if w.FinalizedAdds["pkg-theme"] != "https" {
		t.Errorf("Expected FinalizedAdds for pkg-theme to be 'https', got %s", w.FinalizedAdds["pkg-theme"])
	}
	if w.FinalizedTheme == nil || *w.FinalizedTheme != "pkg-theme" {
		t.Errorf("Expected FinalizedTheme to be 'pkg-theme', got %v", w.FinalizedTheme)
	}

	w.Next()

	// 3. Trying to resolve installation on an uninstall action (should do nothing)
	w.ResolveCurrentInstall("https")
	if _, ok := w.FinalizedAdds["pkg-uninstall"]; ok {
		t.Error("Expected pkg-uninstall NOT to be resolved as an install")
	}
}

func TestWizardResolveCurrentUninstall(t *testing.T) {
	decisions := []pendingDecision{
		{PkgName: "pkg-remove-1", Action: "uninstall"},
		{PkgName: "pkg-remove-2", Action: "uninstall"},
		{PkgName: "pkg-install", Action: "install"}, // not an uninstall action
	}
	w := NewWizard(decisions)

	// 1. Reject uninstall (confirm = false)
	w.ResolveCurrentUninstall(false)
	if len(w.FinalizedRemoves) != 0 {
		t.Errorf("Expected no removals, got %v", w.FinalizedRemoves)
	}

	w.Next()

	// 2. Confirm uninstall (confirm = true)
	w.ResolveCurrentUninstall(true)
	if len(w.FinalizedRemoves) != 1 || w.FinalizedRemoves[0] != "pkg-remove-2" {
		t.Errorf("Expected final removal of pkg-remove-2, got %v", w.FinalizedRemoves)
	}

	w.Next()

	// 3. Trying to resolve uninstall on an install action (should do nothing)
	w.ResolveCurrentUninstall(true)
	if len(w.FinalizedRemoves) != 1 {
		t.Errorf("Expected removals count to remain 1, got %d", len(w.FinalizedRemoves))
	}
}

func TestWizardAddInstall(t *testing.T) {
	w := NewWizard(nil)

	// Add new install
	added := w.AddInstall("my-org/kit-tailwind", bridge.CatalogEntry{Kind: "module"})
	if !added {
		t.Error("Expected AddInstall to succeed and return true")
	}
	if len(w.Queue) != 1 {
		t.Fatalf("Expected Queue length to be 1, got %d", len(w.Queue))
	}
	dec := w.Queue[0]
	if dec.PkgName != "my-org/kit-tailwind" {
		t.Errorf("Expected PkgName 'my-org/kit-tailwind', got '%s'", dec.PkgName)
	}
	if dec.ShortName != "kit-tailwind" {
		t.Errorf("Expected ShortName 'kit-tailwind', got '%s'", dec.ShortName)
	}
	if dec.Kind != "module" {
		t.Errorf("Expected Kind 'module', got '%s'", dec.Kind)
	}
	if dec.Action != "install" {
		t.Errorf("Expected Action 'install', got '%s'", dec.Action)
	}

	// Try to add duplicate - should be ignored
	added = w.AddInstall("my-org/kit-tailwind", bridge.CatalogEntry{Kind: "module"})
	if added {
		t.Error("Expected AddInstall of duplicate to fail and return false")
	}
	if len(w.Queue) != 1 {
		t.Errorf("Expected Queue length to remain 1, got %d", len(w.Queue))
	}

	// Add another package that has no slash in its name
	added = w.AddInstall("standalone-theme", bridge.CatalogEntry{Kind: "theme"})
	if !added {
		t.Error("Expected AddInstall of standalone package to succeed")
	}
	if len(w.Queue) != 2 {
		t.Fatalf("Expected Queue length to be 2, got %d", len(w.Queue))
	}
	if w.Queue[1].ShortName != "standalone-theme" {
		t.Errorf("Expected ShortName 'standalone-theme', got '%s'", w.Queue[1].ShortName)
	}
}

func TestWizardDynamicDependenciesFlow(t *testing.T) {
	// A user starts the wizard with 1 uninstall decision
	initial := []pendingDecision{
		{PkgName: "old-app", ShortName: "old-app", Action: "uninstall", Kind: "app"},
	}
	w := NewWizard(initial)

	if w.IsComplete() {
		t.Fatal("Wizard should not be complete initially")
	}

	// Step 1: User confirms uninstall of old-app
	w.ResolveCurrentUninstall(true)
	w.Next()

	// Wizard is complete now
	if !w.IsComplete() {
		t.Fatal("Wizard should be complete after resolving the only item")
	}

	// Mid-way (or at any point), the system detects a missing dependency (e.g. kit-tailwind) and adds it dynamically
	added := w.AddInstall("org/kit-tailwind", bridge.CatalogEntry{Kind: "module"})
	if !added {
		t.Fatal("Should have successfully added dynamic install dependency")
	}

	// Now the wizard should NOT be complete anymore, since the queue grew
	if w.IsComplete() {
		t.Fatal("Wizard should no longer be complete after adding a dependency")
	}

	// Current item should now be the new dynamic dependency
	curr := w.Current()
	if curr == nil || curr.PkgName != "org/kit-tailwind" {
		t.Fatalf("Expected current to be org/kit-tailwind, got %v", curr)
	}

	// Resolve the dynamic dependency with 'https'
	w.ResolveCurrentInstall("https")
	w.Next()

	// Now it should be complete
	if !w.IsComplete() {
		t.Fatal("Wizard should be complete after resolving all items")
	}

	// Check final outputs
	if len(w.FinalizedRemoves) != 1 || w.FinalizedRemoves[0] != "old-app" {
		t.Errorf("Expected FinalizedRemoves to be ['old-app'], got %v", w.FinalizedRemoves)
	}
	if len(w.FinalizedAdds) != 1 || w.FinalizedAdds["org/kit-tailwind"] != "https" {
		t.Errorf("Expected FinalizedAdds to map 'org/kit-tailwind' to 'https', got %v", w.FinalizedAdds)
	}
	if w.FinalizedTheme != nil {
		t.Errorf("Expected FinalizedTheme to be nil, got %v", w.FinalizedTheme)
	}
}
