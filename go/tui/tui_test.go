package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"owd-cli/bridge"
)

func TestNewModelEngineState(t *testing.T) {
	model := NewModel("test-root")
	if model.workQueue == nil {
		t.Error("Expected workQueue to be initialized")
	}
	if model.enginePhase != PhaseIdle {
		t.Errorf("Expected enginePhase idle, got %s", model.enginePhase)
	}
	if model.runtime.engineResume == nil {
		t.Error("Expected engineResume channel to be initialized")
	}
}

func TestFileServiceQuestFindsFrameworkFirstModuleWithoutChangingConfig(t *testing.T) {
	model := NewModel(t.TempDir())
	theme := "@owdproject/nuxt-theme-paper"
	model.ctx = &bridge.WorkspaceContext{
		Config:   bridge.Config{Theme: &theme, Apps: []string{"@owdproject/nuxt-app-about"}},
		Settings: bridge.Settings{DevPort: 4321},
	}
	model.catalog = &bridge.CatalogResponse{Entries: []bridge.CatalogEntry{
		{Name: "@owdproject/nuxt-kit-primevue", ShortName: "nuxt-kit-primevue", Kind: "module"},
		{Name: nuxtFileService, ShortName: "nuxt-module-fs", Kind: "module"},
	}}
	if status := model.fileServiceStatus(); !strings.Contains(status, "not configured") {
		t.Fatalf("unexpected filesystem status: %q", status)
	}
	if !strings.Contains(model.renderClientPanel(80, 7), "localhost:4321") {
		t.Fatal("panel should display the configured dev port")
	}
	model.termWidth, model.termHeight = 100, 30
	view := model.View()
	if !strings.Contains(view, "Files") || !strings.Contains(view, "not configured") || !strings.Contains(view, "Start your desktop") {
		t.Fatalf("the normal terminal view must show file status and the next step: %s", view)
	}
	if quest := model.nextQuest(); !strings.Contains(quest, "Start") {
		t.Fatalf("expected start quest, got %q", quest)
	}
	_, cmd := (&model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd != nil || model.activeTab != 2 || len(model.getActiveItems()) != 1 || model.getActiveItems()[model.selectedIndex].Name != nuxtFileService {
		t.Fatal("f must focus the filesystem in Modules without running a task")
	}
	if model.hasPendingChanges() || len(model.ctx.Config.Modules) != 0 {
		t.Fatal("browsing files must not install or modify desktop configuration")
	}
	model.pendingPackages[nuxtFileService] = true
	if model.fileServiceStatus() != "add queued" || !strings.Contains(model.nextQuest(), "Apply") {
		t.Fatal("pending file service should show an actionable save quest")
	}
	delete(model.pendingPackages, nuxtFileService)
	model.ctx.Config.Modules = []string{nuxtFileService}
	if model.fileServiceStatus() != "configured" {
		t.Fatal("configured file service must not be called runtime ready")
	}
}

func TestFileServiceQuestWorksWhenCatalogIsUnavailable(t *testing.T) {
	model := NewModel(t.TempDir())
	model.ctx = &bridge.WorkspaceContext{}
	_, cmd := (&model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd != nil || !strings.Contains(model.statusMsg, "desktop add nuxt-module-fs") {
		t.Fatalf("expected an actionable fallback without side effects, got %q", model.statusMsg)
	}
	if !strings.Contains(model.renderCatalogPanel(70, 15, false), "desktop add nuxt-module-fs") {
		t.Fatal("the complete command must remain visible when the catalog is unavailable")
	}
	_, cmd = (&model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd != nil || model.activeTask != TaskNone {
		t.Fatal("save with no pending changes must not install packages")
	}
}

func TestEngineQueueMessages(t *testing.T) {
	model := NewModel("test-root")

	q := NewWorkQueue()
	q.AppendUnique(WorkItem{Name: "a", ShortName: "a", Status: StatusPending})
	q.AppendUnique(WorkItem{Name: "b", ShortName: "b", Status: StatusCompleted})

	m2, _ := (&model).Update(engineQueueUpdatedMsg{Queue: q.Clone()})
	updated := m2.(*TuiModel)
	if updated.workQueue == nil || len(updated.workQueue.Items) != 2 {
		t.Fatalf("expected queue with 2 items, got %+v", updated.workQueue)
	}
	if updated.setupStep != 1 || updated.setupTotalSteps != 2 {
		t.Errorf("expected progress 1/2, got %d/%d", updated.setupStep, updated.setupTotalSteps)
	}

	m3, _ := updated.Update(enginePhaseMsg{Phase: PhaseProcess})
	phaseModel := m3.(*TuiModel)
	if phaseModel.enginePhase != PhaseProcess {
		t.Errorf("expected PhaseProcess, got %s", phaseModel.enginePhase)
	}
}

func TestBuildConfigFromPending(t *testing.T) {
	theme := "@owdproject/nuxt-theme-nova"
	ctx := &bridge.WorkspaceContext{
		Config: bridge.Config{
			Theme:   &theme,
			Apps:    []string{"@owdproject/nuxt-app-about"},
			Modules: []string{},
		},
	}
	catalog := &bridge.CatalogResponse{
		Entries: []bridge.CatalogEntry{
			{Name: "@owdproject/nuxt-app-todo", ShortName: "app-todo", Kind: "app"},
		},
	}
	pending := map[string]bool{
		"@owdproject/nuxt-app-todo": true,
	}
	config, removes := buildConfigFromPending(ctx, catalog, pending, nil)
	if len(removes) != 0 {
		t.Fatalf("expected no removals, got %v", removes)
	}
	if !containsStr(config.Apps, "@owdproject/nuxt-app-todo") {
		t.Fatalf("expected app-todo in apps, got %v", config.Apps)
	}
}
