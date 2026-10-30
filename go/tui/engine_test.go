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
