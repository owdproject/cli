package tui

import (
	"fmt"

	"owd-cli/bridge"
)

type WorkItemStatus string

const (
	StatusPending   WorkItemStatus = "PENDING"
	StatusRunning   WorkItemStatus = "RUNNING"
	StatusCompleted WorkItemStatus = "COMPLETED"
	StatusFailed    WorkItemStatus = "FAILED"
	StatusSkipped   WorkItemStatus = "SKIPPED"
)

type WorkItemKind string

const (
	KindClone  WorkItemKind = "clone"
	KindNpm    WorkItemKind = "npm"
	KindLocal  WorkItemKind = "local"
	KindPrompt WorkItemKind = "prompt"
)

const SourcePending = "pending"

type WorkItem struct {
	Name       string
	ShortName  string
	Type       string // app | module | theme
	Source     string // npm:version | workspace:* | git-ssh | git-https | pending
	Status     WorkItemStatus
	Kind       WorkItemKind
	Error      string
	TargetDir  string
	Discovered bool
	Entry      bridge.CatalogEntry
}

type WorkQueue struct {
	Items []WorkItem
}

func NewWorkQueue() *WorkQueue {
	return &WorkQueue{Items: []WorkItem{}}
}

func (q *WorkQueue) TotalCount() int {
	return len(q.Items)
}

func (q *WorkQueue) CompletedCount() int {
	n := 0
	for _, item := range q.Items {
		switch item.Status {
		case StatusCompleted, StatusSkipped, StatusFailed:
			n++
		}
	}
	return n
}

func (q *WorkQueue) Progress() (completed, total int) {
	return q.CompletedCount(), q.TotalCount()
}

func (q *WorkQueue) FindByName(name string) *WorkItem {
	for i := range q.Items {
		if q.Items[i].Name == name {
			return &q.Items[i]
		}
	}
	return nil
}

func (q *WorkQueue) NextPending() *WorkItem {
	for i := range q.Items {
		if q.Items[i].Status == StatusPending {
			return &q.Items[i]
		}
	}
	return nil
}

func (q *WorkQueue) HasPending() bool {
	return q.NextPending() != nil
}

func (q *WorkQueue) HasPendingPrompt() bool {
	for _, item := range q.Items {
		if item.Status == StatusPending && item.Source == SourcePending {
			return true
		}
	}
	return false
}

func (q *WorkQueue) NextPendingPrompt() *WorkItem {
	for i := range q.Items {
		if q.Items[i].Status == StatusPending && q.Items[i].Source == SourcePending {
			return &q.Items[i]
		}
	}
	return nil
}

func (q *WorkQueue) AppendUnique(item WorkItem) bool {
	if q.FindByName(item.Name) != nil {
		return false
	}
	q.Items = append(q.Items, item)
	return true
}

func (q *WorkQueue) MarkRunning(name string) {
	q.setStatus(name, StatusRunning, "")
}

func (q *WorkQueue) MarkCompleted(name string) {
	q.setStatus(name, StatusCompleted, "")
}

func (q *WorkQueue) MarkSkipped(name string) {
	q.setStatus(name, StatusSkipped, "")
}

func (q *WorkQueue) MarkFailed(name string, err string) {
	q.setStatus(name, StatusFailed, err)
}

func (q *WorkQueue) setStatus(name string, status WorkItemStatus, err string) {
	for i := range q.Items {
		if q.Items[i].Name == name {
			q.Items[i].Status = status
			if err != "" {
				q.Items[i].Error = err
			}
			return
		}
	}
}

func (q *WorkQueue) SetSource(name, source string, kind WorkItemKind) {
	for i := range q.Items {
		if q.Items[i].Name == name {
			q.Items[i].Source = source
			q.Items[i].Kind = kind
			return
		}
	}
}

func (q *WorkQueue) Clone() WorkQueue {
	dup := WorkQueue{Items: make([]WorkItem, len(q.Items))}
	copy(dup.Items, q.Items)
	return dup
}

func (item WorkItem) StatusIcon() string {
	switch item.Status {
	case StatusCompleted:
		return "✔"
	case StatusRunning:
		return "▶"
	case StatusFailed:
		return "✖"
	case StatusSkipped:
		return "⊘"
	default:
		return "○"
	}
}

func (item WorkItem) LogTransition(from, to WorkItemStatus) string {
	return fmt.Sprintf("ℹ Queue: %s %s → %s", item.ShortName, from, to)
}
