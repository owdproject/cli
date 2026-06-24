package tui

import "testing"

func TestWorkQueueProgress(t *testing.T) {
	q := NewWorkQueue()
	if q.CompletedCount() != 0 || q.TotalCount() != 0 {
		t.Fatalf("expected empty queue progress 0/0")
	}

	q.AppendUnique(WorkItem{Name: "a", ShortName: "a", Status: StatusPending})
	q.AppendUnique(WorkItem{Name: "b", ShortName: "b", Status: StatusPending})

	if c, total := q.Progress(); c != 0 || total != 2 {
		t.Fatalf("expected 0/2, got %d/%d", c, total)
	}

	q.MarkCompleted("a")
	if c, total := q.Progress(); c != 1 || total != 2 {
		t.Fatalf("expected 1/2, got %d/%d", c, total)
	}

	q.AppendUnique(WorkItem{Name: "c", ShortName: "c", Status: StatusPending})
	if c, total := q.Progress(); c != 1 || total != 3 {
		t.Fatalf("expected 1/3 after append, got %d/%d", c, total)
	}
}

func TestWorkQueueAppendUnique(t *testing.T) {
	q := NewWorkQueue()
	if !q.AppendUnique(WorkItem{Name: "@owdproject/app-about", ShortName: "app-about"}) {
		t.Fatal("first append should succeed")
	}
	if q.AppendUnique(WorkItem{Name: "@owdproject/app-about", ShortName: "app-about"}) {
		t.Fatal("duplicate append should fail")
	}
	if len(q.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(q.Items))
	}
}

func TestWorkQueueNextPending(t *testing.T) {
	q := NewWorkQueue()
	q.Items = []WorkItem{
		{Name: "a", Status: StatusCompleted},
		{Name: "b", Status: StatusPending, Source: SourcePending},
		{Name: "c", Status: StatusPending, Source: "git-ssh"},
	}

	if next := q.NextPending(); next == nil || next.Name != "b" {
		t.Fatalf("expected b, got %v", next)
	}
	if prompt := q.NextPendingPrompt(); prompt == nil || prompt.Name != "b" {
		t.Fatalf("expected pending prompt for b, got %v", prompt)
	}
}

func TestWorkQueueStatusTransitions(t *testing.T) {
	q := NewWorkQueue()
	q.AppendUnique(WorkItem{Name: "x", ShortName: "x", Status: StatusPending})

	q.MarkRunning("x")
	if item := q.FindByName("x"); item.Status != StatusRunning {
		t.Fatalf("expected RUNNING, got %s", item.Status)
	}

	q.MarkFailed("x", "clone failed")
	if item := q.FindByName("x"); item.Status != StatusFailed || item.Error != "clone failed" {
		t.Fatalf("expected FAILED with error, got %+v", item)
	}
}

func TestWorkItemStatusIcon(t *testing.T) {
	tests := []struct {
		status WorkItemStatus
		want   string
	}{
		{StatusCompleted, "✔"},
		{StatusRunning, "▶"},
		{StatusFailed, "✖"},
		{StatusSkipped, "⊘"},
		{StatusPending, "○"},
	}
	for _, tc := range tests {
		item := WorkItem{Status: tc.status}
		if got := item.StatusIcon(); got != tc.want {
			t.Fatalf("status %s: want %s, got %s", tc.status, tc.want, got)
		}
	}
}
