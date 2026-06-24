package tui

import (
	"testing"

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
	theme := "@owdproject/theme-nova"
	ctx := &bridge.WorkspaceContext{
		Config: bridge.Config{
			Theme:   &theme,
			Apps:    []string{"@owdproject/app-about"},
			Modules: []string{},
		},
	}
	catalog := &bridge.CatalogResponse{
		Entries: []bridge.CatalogEntry{
			{Name: "@owdproject/app-todo", ShortName: "app-todo", Kind: "app"},
		},
	}
	pending := map[string]bool{
		"@owdproject/app-todo": true,
	}
	config, removes := buildConfigFromPending(ctx, catalog, pending, nil)
	if len(removes) != 0 {
		t.Fatalf("expected no removals, got %v", removes)
	}
	if !containsStr(config.Apps, "@owdproject/app-todo") {
		t.Fatalf("expected app-todo in apps, got %v", config.Apps)
	}
}
