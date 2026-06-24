package tui

import (
	"strings"

	"owd-cli/bridge"
)

type pendingDecision struct {
	PkgName   string
	ShortName string
	Action    string // "install" or "uninstall"
	Kind      string // "app", "module", "theme"
	Entry     bridge.CatalogEntry
}

type Wizard struct {
	Queue            []pendingDecision
	Index            int
	FinalizedAdds    map[string]string // pkgName -> method
	FinalizedRemoves []string          // pkgNames
	FinalizedTheme   *string           // theme name to activate
}

func NewWizard(decisions []pendingDecision) *Wizard {
	return &Wizard{
		Queue:            decisions,
		Index:            0,
		FinalizedAdds:    make(map[string]string),
		FinalizedRemoves: []string{},
		FinalizedTheme:   nil,
	}
}

func (w *Wizard) IsComplete() bool {
	return w.Index >= len(w.Queue)
}

func (w *Wizard) Current() *pendingDecision {
	if w.IsComplete() {
		return nil
	}
	return &w.Queue[w.Index]
}

func (w *Wizard) Next() {
	if !w.IsComplete() {
		w.Index++
	}
}

func (w *Wizard) ResolveCurrentInstall(method string) {
	curr := w.Current()
	if curr == nil || curr.Action != "install" {
		return
	}
	if curr.Kind == "theme" {
		w.FinalizedTheme = &curr.PkgName
	}
	w.FinalizedAdds[curr.PkgName] = method
}

func (w *Wizard) ResolveCurrentUninstall(confirm bool) {
	curr := w.Current()
	if curr == nil || curr.Action != "uninstall" {
		return
	}
	if confirm {
		w.FinalizedRemoves = append(w.FinalizedRemoves, curr.PkgName)
	}
}

func (w *Wizard) AddInstall(pkgName string, entry bridge.CatalogEntry) bool {
	// Skip if already in the queue to avoid duplicates
	for _, dec := range w.Queue {
		if dec.PkgName == pkgName {
			return false
		}
	}

	short := pkgName
	if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
		short = pkgName[idx+1:]
	}

	w.Queue = append(w.Queue, pendingDecision{
		PkgName:   pkgName,
		ShortName: short,
		Action:    "install",
		Kind:      entry.Kind,
		Entry:     entry,
	})
	return true
}
