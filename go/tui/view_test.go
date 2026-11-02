package tui

import (
	"strings"
	"testing"

	"owd-cli/bridge"
)

func TestRenderLogsPanelFollowsTailByDefault(t *testing.T) {
	m := &TuiModel{
		logLines:      []string{"l1", "l2", "l3", "l4", "l5", "l6"},
		logViewHeight: 3,
		termWidth:     160,
		activeTask:    TaskNone,
	}
	// h=4 -> viewport of 2 log rows (one leading blank line + one reserved row).
	out := m.renderLogsPanel(80, 4)

	if !strings.Contains(out, "l5") || !strings.Contains(out, "l6") {
		t.Fatalf("expected tail lines l5/l6 in output:\n%s", out)
	}
	if strings.Contains(out, "l1") {
		t.Fatalf("tail view should not show the oldest line l1:\n%s", out)
	}
}

func TestRenderLogsPanelScrollsIntoOlderLines(t *testing.T) {
	m := &TuiModel{
		logLines:        []string{"l1", "l2", "l3", "l4", "l5", "l6"},
		logViewHeight:   3,
		termWidth:       160,
		activeTask:      TaskNone,
		logScrollOffset: 2,
	}
	out := m.renderLogsPanel(80, 4)

	if !strings.Contains(out, "older") {
		t.Fatalf("scrolled view should show the offset hint:\n%s", out)
	}
	if strings.Contains(out, "l6") {
		t.Fatalf("scrolled view should not show the newest line l6:\n%s", out)
	}
}
func TestGetActiveItemsFiltersByProjectFramework(t *testing.T) {
	entries := []bridge.CatalogEntry{
		{Name: "@owdproject/nuxt-app-todo", ShortName: "nuxt-app-todo", Kind: "app"},
		{Name: "@owdproject/react-app-notepad", ShortName: "react-app-notepad", Kind: "app"},
		{Name: "@owdproject/nuxt-theme-win95", ShortName: "nuxt-theme-win95", Kind: "theme"},
		{Name: "@owdproject/react-theme-win95", ShortName: "react-theme-win95", Kind: "theme"},
		{Name: "@owdproject/core", ShortName: "core", Kind: "module"},
	}

	nuxt := &TuiModel{
		catalog:   &bridge.CatalogResponse{Entries: entries},
		activeTab: 1,
		ctx:       &bridge.WorkspaceContext{Framework: "nuxt"},
	}
	if got := nuxt.getActiveItems(); len(got) != 1 || got[0].ShortName != "nuxt-app-todo" {
		t.Fatalf("nuxt Apps tab = %+v, want only nuxt-app-todo", got)
	}

	react := &TuiModel{
		catalog:   &bridge.CatalogResponse{Entries: entries},
		activeTab: 3,
		ctx:       &bridge.WorkspaceContext{Framework: "react"},
	}
	if got := react.getActiveItems(); len(got) != 1 || got[0].ShortName != "react-theme-win95" {
		t.Fatalf("react Themes tab = %+v, want only react-theme-win95", got)
	}

	// Entries without a framework segment are never filtered out.
	if !entryMatchesFramework("react", "core") {
		t.Fatal("framework-less entries must stay visible")
	}
}
