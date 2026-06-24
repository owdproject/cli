package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"owd-cli/bridge"

	tea "github.com/charmbracelet/bubbletea"
)

type WizardMode int

const (
	WizardSave WizardMode = iota
	WizardSaveAndServe
	WizardStartupCheck
)

type EnginePhase string

const (
	PhaseIdle        EnginePhase = "idle"
	PhaseWriteConfig EnginePhase = "write_config"
	PhaseResolve     EnginePhase = "resolve"
	PhasePrompt      EnginePhase = "prompt"
	PhaseProcess     EnginePhase = "process"
	PhaseInstall     EnginePhase = "install"
	PhasePrepare     EnginePhase = "prepare"
	PhaseDone        EnginePhase = "done"
	PhaseFailed      EnginePhase = "failed"
)

// buildConfigFromPending applies catalog toggles to a copy of the current config.
func buildConfigFromPending(
	ctx *bridge.WorkspaceContext,
	catalog *bridge.CatalogResponse,
	pending map[string]bool,
	pendingTheme *string,
) (*bridge.Config, []string) {
	if ctx == nil {
		return nil, nil
	}

	config := bridge.Config{
		Apps:    append([]string{}, ctx.Config.Apps...),
		Modules: append([]string{}, ctx.Config.Modules...),
	}
	if ctx.Config.Theme != nil {
		t := *ctx.Config.Theme
		config.Theme = &t
	}

	var depsToRemove []string

	for name, install := range pending {
		entry := catalogEntryForName(catalog, name)
		if install {
			switch entry.Kind {
			case "app":
				if !containsStr(config.Apps, name) {
					config.Apps = append(config.Apps, name)
				}
			case "module":
				if !containsStr(config.Modules, name) {
					config.Modules = append(config.Modules, name)
				}
			case "theme":
				config.Theme = &name
			}
		} else {
			depsToRemove = append(depsToRemove, name)
			switch entry.Kind {
			case "app":
				config.Apps = removeStr(config.Apps, name)
			case "module":
				config.Modules = removeStr(config.Modules, name)
			case "theme":
				if config.Theme != nil && *config.Theme == name {
					empty := ""
					config.Theme = &empty
				}
			}
		}
	}

	if pendingTheme != nil {
		config.Theme = pendingTheme
	}

	return &config, depsToRemove
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func removeStr(list []string, v string) []string {
	var out []string
	for _, s := range list {
		if s != v {
			out = append(out, s)
		}
	}
	return out
}

func (m *TuiModel) startWizard(mode WizardMode) tea.Cmd {
	if mode == WizardStartupCheck {
		q := BuildStartupCheckQueue(m.workspaceRoot, m.ctx, m.catalog, func(string) {})
		hasWork := false
		for _, item := range q.Items {
			if item.Status == StatusPending {
				hasWork = true
				break
			}
		}
		if !hasWork {
			m.addLog("ℹ Startup check: all packages are available")
			return nil
		}
	}

	m.activeTask = TaskSetup
	m.enginePhase = PhaseWriteConfig
	m.workQueue = NewWorkQueue()
	m.statusMsg = "Starting wizard…"
	m.addLog("ℹ Starting wizard workflow…")

	if mode == WizardSaveAndServe {
		m.startServerAfterSetup = true
	}

	var config *bridge.Config
	var depsToRemove []string

	if mode == WizardStartupCheck {
		m.addLog("ℹ Running startup check wizard…")
	} else {
		config, depsToRemove = buildConfigFromPending(m.ctx, m.catalog, m.pendingPackages, m.pendingTheme)
		m.pendingPackages = make(map[string]bool)
		m.pendingTheme = nil
	}

	catalogCopy := m.catalog
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime
	lastMethod := m.lastInstallMethod

	return tea.Batch(func() tea.Msg {
		m.RunWizardEngine(mode, config, depsToRemove, catalogCopy, workspaceRoot, runtime, lastMethod)
		return nil
	}, m.listenToChannel())
}

func (m *TuiModel) RunWizardEngine(
	mode WizardMode,
	config *bridge.Config,
	depsToRemove []string,
	catalog *bridge.CatalogResponse,
	workspaceRoot string,
	runtime *RuntimeState,
	lastInstallMethod string,
) {
	go func() {
		msgChan := runtime.msgChan
		log := func(line string) { msgChan <- logLineMsg(line) }
		emitQueue := func(q *WorkQueue) {
			msgChan <- engineQueueUpdatedMsg{Queue: q.Clone()}
		}
		emitPhase := func(p EnginePhase) { msgChan <- enginePhaseMsg{Phase: p} }

		var ctx *bridge.WorkspaceContext
		var err error

		if mode != WizardStartupCheck {
			emitPhase(PhaseWriteConfig)
			log("ℹ Updating desktop.config.ts")
			if err = bridge.WriteConfigOnly(workspaceRoot, config); err != nil {
				log("✖ Failed to update desktop.config.ts")
				emitPhase(PhaseFailed)
				msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
			log("✔ desktop.config.ts updated")

			if len(depsToRemove) > 0 {
				log(fmt.Sprintf("ℹ Removing %d package(s) from package.json", len(depsToRemove)))
				if err = bridge.WritePackageJsonDeps(workspaceRoot, nil, depsToRemove); err != nil {
					log("✖ Failed to update package.json removals")
					emitPhase(PhaseFailed)
					msgChan <- taskFinishedMsg{Success: false, Err: err}
					return
				}
				log("✔ package.json updated")
			}
		}

		emitPhase(PhaseResolve)
		ctx, err = bridge.ReadWorkspaceContext(workspaceRoot)
		if err != nil {
			log("✖ Failed to read workspace context")
			emitPhase(PhaseFailed)
			msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		var q *WorkQueue
		if mode == WizardStartupCheck {
			q = BuildStartupCheckQueue(workspaceRoot, ctx, catalog, log)
		} else {
			q = BuildQueueFromConfig(workspaceRoot, ctx, catalog, log)
		}
		emitQueue(q)

		settings := ctx.Settings

		for {
			if promptItem := q.NextPendingPrompt(); promptItem != nil {
				emitPhase(PhasePrompt)
				msgChan <- engineNeedsPromptMsg{Item: *promptItem}
				method := <-runtime.engineResume

				if method == "abort" {
					log("✖ Wizard aborted by user")
					emitPhase(PhaseFailed)
					msgChan <- taskFinishedMsg{Success: false, Err: fmt.Errorf("wizard aborted")}
					return
				}

				log("ℹ Download method selected: " + resolveMethodLabel(method))
				if err := applyResolution(q, promptItem, method, workspaceRoot, catalog, &settings, log); err != nil {
					emitPhase(PhaseFailed)
					msgChan <- taskFinishedMsg{Success: false, Err: err}
					return
				}
				emitQueue(q)
				continue
			}

			item := q.NextPending()
			if item == nil {
				break
			}

			if item.Kind == KindNpm || item.Kind == KindLocal {
				prev := item.Status
				q.MarkSkipped(item.Name)
				log(item.LogTransition(prev, StatusSkipped))
				emitQueue(q)
				continue
			}

			emitPhase(PhaseProcess)
			if err := processCloneItem(q, item, workspaceRoot, catalog, runtime, log); err != nil {
				emitPhase(PhaseFailed)
				msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
			emitQueue(q)

			updated := q.FindByName(item.Name)
			if updated != nil {
				DiscoverNestedDeps(workspaceRoot, *updated, catalog, q, log)
				emitQueue(q)
			}
		}

		emitPhase(PhaseInstall)
		log("ℹ Running pnpm install")
		result := runtime.runProcessStream(workspaceRoot, "pnpm", []string{"install"})
		if result.Err != nil {
			log("✖ pnpm install failed")
			emitPhase(PhaseFailed)
			msgChan <- taskFinishedMsg{Success: false, Err: result.Err}
			return
		}
		log("✔ pnpm install completed")

		emitPhase(PhasePrepare)
		log("ℹ Running pnpm run prepare:modules")
		result = runtime.runProcessStream(workspaceRoot, "pnpm", []string{"run", "prepare:modules"})
		if result.Err != nil {
			log("✖ prepare:modules failed")
			emitPhase(PhaseFailed)
			msgChan <- taskFinishedMsg{Success: false, Err: result.Err}
			return
		}
		log("✔ prepare:modules completed")

		if settings.LastInstallChoices != nil {
			_ = bridge.WriteChanges(workspaceRoot, &bridge.WritePayload{Settings: &settings})
		}

		emitPhase(PhaseDone)
		log("✔ Wizard completed successfully")
		msgChan <- taskFinishedMsg{Success: true}
	}()
}

func resolveMethodLabel(method string) string {
	switch method {
	case "git-ssh":
		return "Git SSH"
	case "git-https":
		return "Git HTTPS"
	case "npm":
		return "NPM Package"
	default:
		return method
	}
}

func applyResolution(
	q *WorkQueue,
	item *WorkItem,
	method string,
	workspaceRoot string,
	catalog *bridge.CatalogResponse,
	settings *bridge.Settings,
	log func(string),
) error {
	name := item.Name
	short := item.ShortName

	switch method {
	case "git-ssh", "git-https":
		owner := resolveOwner(name, catalogEntriesSlice(catalog))
		var gitURL string
		if method == "git-ssh" {
			gitURL = fmt.Sprintf("git@github.com:%s/%s.git", owner, short)
		} else {
			gitURL = fmt.Sprintf("https://github.com/%s/%s.git", owner, short)
		}
		if err := bridge.WritePackageJsonDeps(workspaceRoot, map[string]string{name: "workspace:*"}, nil); err != nil {
			log("✖ Failed to update package.json for " + short)
			return err
		}
		if settings.LastInstallChoices == nil {
			settings.LastInstallChoices = make(map[string]interface{})
		}
		settings.LastInstallChoices[name] = map[string]string{"type": "git", "gitUrl": gitURL}
		q.SetSource(name, method, KindClone)
		if item.TargetDir == "" {
			item.TargetDir = filepath.Join(workspaceRoot, kindDirForKind(item.Type), short)
		}
		log("✔ " + short + " queued for clone (" + resolveMethodLabel(method) + ")")
		return nil

	case "npm":
		version := "latest"
		if item.Entry.Version != nil {
			version = *item.Entry.Version
		}
		if err := bridge.WritePackageJsonDeps(workspaceRoot, map[string]string{name: version}, nil); err != nil {
			log("✖ Failed to add " + short + " to package.json")
			return err
		}
		q.SetSource(name, "npm:"+version, KindNpm)
		q.MarkSkipped(name)
		log("✔ " + short + " added to package.json")
		log("ℹ " + short + " will be installed through pnpm install")
		return nil

	default:
		return fmt.Errorf("unknown resolution method: %s", method)
	}
}

func catalogEntriesSlice(catalog *bridge.CatalogResponse) []bridge.CatalogEntry {
	if catalog == nil {
		return nil
	}
	return catalog.Entries
}

func processCloneItem(
	q *WorkQueue,
	item *WorkItem,
	workspaceRoot string,
	catalog *bridge.CatalogResponse,
	runtime *RuntimeState,
	log func(string),
) error {
	prev := item.Status
	q.MarkRunning(item.Name)
	log(item.LogTransition(prev, StatusRunning))

	short := item.ShortName
	targetDir := item.TargetDir
	if targetDir == "" {
		targetDir = filepath.Join(workspaceRoot, kindDirForKind(item.Type), short)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "package.json")); err == nil {
		log("ℹ " + short + " already cloned — skipping")
		q.MarkCompleted(item.Name)
		log("✔ Directory verified: " + relWorkspacePath(workspaceRoot, targetDir))
		return nil
	}

	method := item.Source
	if method == SourcePending {
		method = "git-https"
	}

	owner := resolveOwner(item.Name, catalogEntriesSlice(catalog))
	var gitURL string
	if method == "git-ssh" {
		gitURL = fmt.Sprintf("git@github.com:%s/%s.git", owner, short)
	} else {
		gitURL = fmt.Sprintf("https://github.com/%s/%s.git", owner, short)
	}

	log("ℹ Cloning " + short + "…")
	result := runtime.runProcessStream(workspaceRoot, "git", []string{"clone", gitURL, targetDir})
	if result.Err != nil {
		q.MarkFailed(item.Name, result.Err.Error())
		log("✖ Clone failed for " + short)
		return result.Err
	}
	log("✔ Clone completed")

	if _, err := os.Stat(filepath.Join(targetDir, "package.json")); err != nil {
		errMsg := "directory not found after clone"
		q.MarkFailed(item.Name, errMsg)
		log("✖ Clone reported success but directory not found")
		return fmt.Errorf("%s: %s", short, errMsg)
	}

	q.MarkCompleted(item.Name)
	log("✔ Directory verified: " + relWorkspacePath(workspaceRoot, targetDir))
	return nil
}

func relWorkspacePath(workspaceRoot, absPath string) string {
	if rel, err := filepath.Rel(workspaceRoot, absPath); err == nil {
		return rel
	}
	return absPath
}

func (m *TuiModel) resolveEnginePrompt(method string) {
	if m.runtime.engineResume != nil {
		m.runtime.engineResume <- method
	}
}
