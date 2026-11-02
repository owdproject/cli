package tui

import (
	"testing"

	"owd-cli/bridge"
)

func TestResolveMethodLabel(t *testing.T) {
	if resolveMethodLabel("git-ssh") != "Git SSH" {
		t.Fatal("unexpected label for git-ssh")
	}
	if resolveMethodLabel("npm") != "NPM Package" {
		t.Fatal("unexpected label for npm")
	}
}

func TestBuildConfigFromPendingRemovals(t *testing.T) {
	ctx := &bridge.WorkspaceContext{
		Config: bridge.Config{
			Apps:    []string{"@owdproject/nuxt-app-about", "@owdproject/nuxt-app-todo"},
			Modules: []string{},
		},
	}
	catalog := &bridge.CatalogResponse{
		Entries: []bridge.CatalogEntry{
			{Name: "@owdproject/nuxt-app-todo", ShortName: "app-todo", Kind: "app"},
		},
	}
	pending := map[string]bool{
		"@owdproject/nuxt-app-todo": false,
	}
	config, removes := buildConfigFromPending(ctx, catalog, pending, nil)
	if len(removes) != 1 || removes[0] != "@owdproject/nuxt-app-todo" {
		t.Fatalf("expected removal, got %v", removes)
	}
	if containsStr(config.Apps, "@owdproject/nuxt-app-todo") {
		t.Fatalf("app-todo should be removed from config, got %v", config.Apps)
	}
}

func TestFrameworkSelectionsRejectOtherFrameworksBeforeSaving(t *testing.T) {
	if err := validateFrameworkSelections("nuxt", map[string]bool{"@owdproject/react-app-example": true}, nil); err == nil {
		t.Fatal("a React app must not be added to a Nuxt desktop")
	}
	if err := validateFrameworkSelections("nuxt", map[string]bool{"@owdproject/react-app-example": false}, nil); err != nil {
		t.Fatalf("an existing incompatible app must remain removable: %v", err)
	}
	theme := "@owdproject/react-theme-win95"
	if err := validateFrameworkSelections("nuxt", nil, &theme); err == nil {
		t.Fatal("a React theme must not be selected for a Nuxt desktop")
	}
	if err := validateFrameworkSelections("nuxt", map[string]bool{"@owdproject/nuxt-app-todo": true}, nil); err != nil {
		t.Fatalf("a Nuxt app should remain selectable: %v", err)
	}
	// The same checks are symmetric for a React desktop.
	if err := validateFrameworkSelections("react", map[string]bool{"@owdproject/react-app-example": true}, nil); err != nil {
		t.Fatalf("a React app should be selectable in a React desktop: %v", err)
	}
	if err := validateFrameworkSelections("react", map[string]bool{"@owdproject/nuxt-app-todo": true}, nil); err == nil {
		t.Fatal("a Nuxt app must not be added to a React desktop")
	}
	// No profile defaults to nuxt, preserving legacy behavior.
	if err := validateFrameworkSelections("", map[string]bool{"@owdproject/react-app-example": true}, nil); err == nil {
		t.Fatal("an empty framework must default to nuxt")
	}
}

func TestDirectInstallRejectsOtherFrameworkBeforeWriting(t *testing.T) {
	model := NewModel(t.TempDir())
	_, command := model.triggerInstall(&bridge.CatalogEntry{Name: "@owdproject/react-app-example", Kind: "app"}, "npm")
	if model.activeTask != TaskNone || command != nil {
		t.Fatal("incompatible install must stop before starting a task or writing configuration")
	}
}
