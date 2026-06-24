package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"owd-cli/bridge"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func NewModel(root string) TuiModel {
	return TuiModel{
		workspaceRoot:    root,
		activeTab:        1, // Default to Apps (index 1), Internal (index 0) is secret
		loading:          true,
		statusMsg:        "Initializing OWD Control Panel…",
		logLines:         []string{},
		memHistory:       []int{},
		gitChangesMap:    make(map[string]GitChanges),
		updatesMap:       make(map[string]UpdateInfo),
		localGitDirs:     make(map[string]bool),
		settingsSel:      0,
		termWidth:        160,
		termHeight:       40,
		lastInstallMethod: "npm",
		logTailerStarted:  false,
		pendingPackages:  make(map[string]bool),
		pendingTheme:     nil,
		wizard:           nil,
		runtime: &RuntimeState{
			msgChan: make(chan tea.Msg, 500),
		},
		startupCheckDone:    false,
		startServerAfterSetup: false,
		setupStep:           0,
		setupTotalSteps:     0,
		setupLabel:          "",
		activeTask:          TaskNone,
		workspaceBranch:     "—",
		workspaceChanges:    "clean",
		justInstalledAdds:   make(map[string]string),
		setupInProgress:     false,
		setupAdds:           make(map[string]string),
		setupCloned:         make(map[string]bool),
	}
}

func (m *TuiModel) listenToChannel() tea.Cmd {
	return func() tea.Msg {
		return <-m.runtime.msgChan
	}
}

func (m *TuiModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadContextCmd(),
		m.loadCatalogCmd(false),
		m.checkServerStatusCmd(),
		m.updateWorkspaceGitStatusCmd(),
		m.listenToChannel(),
		tickCmd(),
	)
}

// ─────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────

func tickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func memSampleCmd() tea.Cmd {
	return func() tea.Msg {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		mb := int(ms.Sys / 1024 / 1024)
		return memTickMsg{MemMB: mb}
	}
}

func (m *TuiModel) loadContextCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, err := bridge.ReadWorkspaceContext(m.workspaceRoot)
		return contextLoadedMsg{Ctx: ctx, Err: err}
	}
}

func (m *TuiModel) loadCatalogCmd(force bool) tea.Cmd {
	return func() tea.Msg {
		cat, err := bridge.ReadCatalog(m.workspaceRoot, force)
		return catalogLoadedMsg{Cat: cat, Err: err}
	}
}

func (m *TuiModel) rebootServerCmd() tea.Cmd {
	return func() tea.Msg {
		m.runtime.msgChan <- logLineMsg("ℹ Theme change detected. Rebooting dev server…")
		var metaDir string
		if m.ctx != nil {
			metaDir = m.ctx.Paths.MetaDir
		}
		stopServeTaskSync(m.workspaceRoot, metaDir, m.runtime)
		time.Sleep(1500 * time.Millisecond)
		m.RunServeTask()
		return nil
	}
}

func (m *TuiModel) checkServerStatusCmd() tea.Cmd {
	return func() tea.Msg {
		if m.ctx == nil {
			return serverStatusMsg{Running: false}
		}

		port := m.ctx.Settings.DevPort
		if port == 0 {
			port = 3000
		}

		open := isPortOpen(port)
		if !open {
			pidPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.pid")
			_ = os.Remove(pidPath)
			return serverStatusMsg{Running: false}
		}

		pid := findProcessPID(port)
		if pid > 0 {
			pidPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.pid")
			_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
		}

		return serverStatusMsg{Running: true}
	}
}

func (m *TuiModel) updateWorkspaceGitStatusCmd() tea.Cmd {
	return func() tea.Msg {
		branch := gitBranch(m.workspaceRoot)
		changes := gitChanges(m.workspaceRoot)
		return workspaceGitStatusMsg{Branch: branch, Changes: changes}
	}
}

func (m *TuiModel) hasPendingChanges() bool {
	if len(m.pendingPackages) > 0 {
		return true
	}
	if m.pendingTheme != nil {
		if m.ctx != nil && m.ctx.Config.Theme != nil {
			return *m.ctx.Config.Theme != *m.pendingTheme
		}
		return true
	}
	return false
}

func (m *TuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			if m.isClearLogsClick(msg) {
				m.logLines = []string{}
				m.statusMsg = "Logs cleared."
				return m, nil
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tickMsg:
		m.tickCount++
		if m.tickCount%4 == 0 {
			m.blink = !m.blink
		}

		var cmds []tea.Cmd
		cmds = append(cmds, tickCmd())

		// Every ~1.5s: sample memory + check server status
		if m.tickCount%10 == 0 {
			var mb int
			if m.ctx != nil {
				pidPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.pid")
				if data, err := os.ReadFile(pidPath); err == nil {
					var pid int
					if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err == nil {
						mb = getProcessMemoryMB(pid)
					}
				}
			}
			m.memHistory = append(m.memHistory, mb)
			if len(m.memHistory) > 30 {
				m.memHistory = m.memHistory[1:]
			}
			if m.ctx != nil {
				cmds = append(cmds, m.checkServerStatusCmd())
			}
		}

		// Every ~15s: fast local git status check (no network)
		if m.tickCount%100 == 0 && !m.loading && m.activeTask == TaskNone && !m.checkingUpdates {
			m.checkingUpdates = true
			cmds = append(cmds, m.checkLocalChangesCmd(), m.updateWorkspaceGitStatusCmd())
		}

		// Every ~5min: slow remote check (git behind without fetch + npm versions)
		if m.tickCount%2000 == 0 && !m.loading && m.activeTask == TaskNone {
			cmds = append(cmds, m.checkRemoteUpdatesCmd())
		}

		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		promptToShow := m.activePrompt
		if (m.activeTask == TaskSetup || m.activeTask == TaskWipe) && (promptToShow == PromptNone || promptToShow == PromptUninstallConfirm || promptToShow == PromptInstallMethod || promptToShow == PromptWipeWorkspaceConfirm) {
			promptToShow = PromptSetupProgress
		}
		if promptToShow != PromptNone {
			return m.handlePromptKeys(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "0":
			m.activeTab, m.selectedIndex, m.scrollOffset = 0, 0, 0
		case "1":
			m.activeTab, m.selectedIndex, m.scrollOffset = 1, 0, 0
		case "2":
			m.activeTab, m.selectedIndex, m.scrollOffset = 2, 0, 0
		case "3":
			m.activeTab, m.selectedIndex, m.scrollOffset = 3, 0, 0
		case "left":
			if m.activeTab == 0 {
				m.activeTab = 3
			} else {
				m.activeTab--
			}
			m.selectedIndex, m.scrollOffset = 0, 0
		case "right":
			if m.activeTab == 3 {
				m.activeTab = 1
			} else {
				m.activeTab++
			}
			m.selectedIndex, m.scrollOffset = 0, 0
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				if m.selectedIndex < m.scrollOffset {
					m.scrollOffset = m.selectedIndex
				}
			}
		case "down", "j":
			items := m.getActiveItems()
			if m.selectedIndex < len(items)-1 {
				m.selectedIndex++
			}
		case "r":
			m.loading = true
			m.statusMsg = "Refreshing package catalog…"
			return m, m.loadCatalogCmd(true)
		case "s":
			if m.hasPendingChanges() {
				cmd := m.startQueueReview()
				return m, cmd
			} else {
				cmd := m.startStartupCheck()
				if cmd != nil {
					return m, cmd
				}
			}
		case "d":
			if !m.serverRunning && m.activeTask == TaskNone {
				if m.hasPendingChanges() {
					m.startServerAfterSetup = true
					cmd := m.startQueueReview()
					return m, cmd
				} else {
					cmd := m.startStartupCheck()
					if m.activePrompt != PromptNone || cmd != nil {
						m.startServerAfterSetup = true
						return m, cmd
					}
				}

				m.statusMsg = "Starting dev server…"
				m.activeTask = TaskServe
				m.RunServeTask()
			}
		case "x":
			if m.serverRunning {
				m.statusMsg = "Stopping dev server…"
				m.activeTask = TaskServe
				m.StopServeTask()
			}
		case "c":
			items := m.getActiveItems()
			if len(items) > 0 && m.selectedIndex < len(items) {
				pkg := items[m.selectedIndex]
				m.promptPkg = &pkg
				m.loadPackageDetails(&pkg)
				m.activePrompt = PromptManagePackage
				m.promptSel = 0
			}
		case "enter", " ":
			items := m.getActiveItems()
			if len(items) > 0 && m.selectedIndex < len(items) {
				pkg := items[m.selectedIndex]
				if m.activeTab == 3 { // Themes (radiobox)
					active := false
					if m.ctx != nil && m.ctx.Config.Theme != nil && *m.ctx.Config.Theme == pkg.Name {
						active = true
					}
					if active {
						m.pendingTheme = nil
					} else {
						themeName := pkg.Name
						m.pendingTheme = &themeName
					}
				} else { // Apps or Modules (checkbox)
					if pkg.Installed {
						if _, exists := m.pendingPackages[pkg.Name]; exists {
							delete(m.pendingPackages, pkg.Name)
						} else {
							m.pendingPackages[pkg.Name] = false
						}
					} else {
						if _, exists := m.pendingPackages[pkg.Name]; exists {
							delete(m.pendingPackages, pkg.Name)
						} else {
							m.pendingPackages[pkg.Name] = true
						}
					}
				}
			}
		case "u":
			items := m.getActiveItems()
			if len(items) > 0 && m.selectedIndex < len(items) {
				pkg := items[m.selectedIndex]
				if pkg.Installed && m.activeTask == TaskNone {
					m.activeTask = TaskSetup
					m.statusMsg = fmt.Sprintf("Updating %s…", pkg.ShortName)
					m.RunUpdatePackageTask(pkg.Name, pkg.ShortName, pkg.Kind, pkg.LocalSource)
				}
			}
		case "g":
			if m.ctx != nil {
				m.settingsInstallMode = m.ctx.Settings.InstallMode
				m.settingsCatalogSort = m.ctx.Settings.CatalogSort
				m.settingsSel = 0
				m.activePrompt = PromptSettings

				// Initialize text inputs
				tiUser := textinput.New()
				tiUser.Placeholder = "github_user"
				tiUser.CharLimit = 64
				tiUser.Width = 30
				if m.ctx.Settings.GithubUser != nil {
					tiUser.SetValue(*m.ctx.Settings.GithubUser)
				}
				m.settingsUserInput = tiUser

				tiOrgs := textinput.New()
				tiOrgs.Placeholder = "org1, org2"
				tiOrgs.CharLimit = 256
				tiOrgs.Width = 40
				if len(m.ctx.Settings.GithubOrgs) > 0 {
					tiOrgs.SetValue(strings.Join(m.ctx.Settings.GithubOrgs, ", "))
				} else {
					tiOrgs.SetValue("owdproject, atproto-os")
				}
				m.settingsOrgsInput = tiOrgs

				m.updateSettingsFocus()
			}
		case "n":
			m.ExitCode = 10
			return m, tea.Quit
		}
		return m, nil

	case contextLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.statusMsg = fmt.Sprintf("Error loading workspace: %v", msg.Err)
		} else {
			themeChanged := false
			if m.ctx != nil && m.ctx.Config.Theme != nil && msg.Ctx.Config.Theme != nil {
				if *m.ctx.Config.Theme != *msg.Ctx.Config.Theme {
					themeChanged = true
				}
			} else if (m.ctx != nil && m.ctx.Config.Theme == nil && msg.Ctx.Config.Theme != nil) || (m.ctx != nil && m.ctx.Config.Theme != nil && msg.Ctx.Config.Theme == nil) {
				themeChanged = true
			}

			m.ctx = msg.Ctx
			m.statusMsg = "Workspace loaded."

			if !m.logTailerStarted && m.ctx != nil {
				m.logTailerStarted = true
				logPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.log")
				m.StartLogTailer(logPath)
			}

			if themeChanged && m.serverRunning {
				m.statusMsg = "Rebooting server for theme change…"
				m.activeTask = TaskServe
				return m, m.rebootServerCmd()
			}

			return m, m.checkServerStatusCmd()
		}

	case catalogLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.statusMsg = fmt.Sprintf("Error loading catalog: %v", msg.Err)
		} else {
			m.catalog = msg.Cat
			m.statusMsg = fmt.Sprintf("Catalog loaded (%s).", msg.Cat.CacheAge)

			// Run fast local check immediately; schedule remote check too
			m.checkingUpdates = true
			return m, tea.Batch(m.checkLocalChangesCmd(), m.checkRemoteUpdatesCmd())
		}

	case localChangesMsg:
		m.gitChangesMap = msg.GitChanges
		m.localGitDirs = msg.LocalGitDirs
		m.checkingUpdates = false
		m.loading = false

	case remoteUpdatesMsg:
		m.updatesMap = msg.Updates
		if msg.Err == nil {
			count := 0
			for _, up := range msg.Updates {
				if up.LocalGit || up.Npm {
					count++
				}
			}
			if count > 0 {
				m.statusMsg = fmt.Sprintf("%d update(s) available.", count)
			}
		}

	case setupProgressMsg:
		m.setupStep = msg.Step
		m.setupTotalSteps = msg.Total
		m.setupLabel = msg.Label
		return m, m.listenToChannel()
	case setupClonedMsg:
		m.setupStep = msg.Step
		for _, pkg := range msg.Cloned {
			m.setupCloned[pkg] = true
		}
		return m, m.listenToChannel()
	case depsDetectedMsg:
		m.activeTask = TaskNone
		m.setupInProgress = true
		m.addLog(fmt.Sprintf("ℹ Mid-setup: Missing dependencies detected: %v", msg.Deps))
		if m.wizard == nil {
			m.wizard = NewWizard([]pendingDecision{})
		}

		for _, dep := range msg.Deps {
			var depEntry bridge.CatalogEntry
			found := false
			if m.catalog != nil {
				for _, e := range m.catalog.Entries {
					if e.Name == dep {
						depEntry = e
						found = true
						break
					}
				}
			}
			if !found {
				short := dep
				if idx := strings.LastIndex(dep, "/"); idx >= 0 {
					short = dep[idx+1:]
				}
				depEntry = bridge.CatalogEntry{
					Name:      dep,
					ShortName: short,
					Kind:      "module",
				}
			}

			m.wizard.AddInstall(dep, depEntry)
		}

		if !m.wizard.IsComplete() {
			m.statusMsg = "Found additional dependencies. Choose install method…"
			return m, m.processNextQueueDecision()
		}
		return m, m.listenToChannel()
	case promptThemeDepMsg:
		m.activePrompt = PromptThemeDepConfirm
		m.activeThemeDep = msg.DepName
		m.promptSel = 0 // default to Yes (0: Yes, 1: No)

		// Find catalog entry for this dep
		var entry *bridge.CatalogEntry
		if m.catalog != nil {
			for _, e := range m.catalog.Entries {
				if e.Name == msg.DepName {
					entry = &e
					break
				}
			}
		}
		if entry == nil {
			entry = &bridge.CatalogEntry{
				Name:      msg.DepName,
				ShortName: msg.ShortName,
				Kind:      "module",
			}
		}
		m.promptPkg = entry
		return m, nil

	case logLineMsg:
		line := string(msg)
		if strings.Contains(line, "█") || strings.Contains(line, "▄") || strings.Contains(line, "▀") {
			return m, m.listenToChannel()
		}
		if strings.Contains(line, "Local:") || strings.Contains(line, "Network:") || strings.Contains(line, "expose") || strings.Contains(line, "➜") {
			return m, m.listenToChannel()
		}
		m.addLog(line)
		return m, m.listenToChannel()

	case clearLogsMsg:
		m.logLines = []string{}
		return m, m.listenToChannel()

	case taskFinishedMsg:
		m.activeTask = TaskNone
		m.setupInProgress = false
		if msg.Success {
			m.statusMsg = "Task completed."
			m.addLog("✓ Wizard changes applied successfully")

			// Phase 2: check deps of newly installed packages
			if len(m.justInstalledAdds) > 0 {
				justInstalled := m.justInstalledAdds
				m.justInstalledAdds = make(map[string]string)
				if depCmd := m.checkPostInstallDeps(justInstalled); depCmd != nil {
					m.statusMsg = "Found additional dependencies. Choose install method…"
					return m, tea.Batch(depCmd, m.loadContextCmd(), m.listenToChannel())
				}
			}

			if m.startServerAfterSetup {
				m.startServerAfterSetup = false
				m.statusMsg = "Starting dev server…"
				m.activeTask = TaskServe
				m.RunServeTask()
			}
		} else {
			m.statusMsg = fmt.Sprintf("Task failed: %v", msg.Err)
			m.addLog(fmt.Sprintf("✗ Wizard changes FAILED to apply: %v", msg.Err))
			m.startServerAfterSetup = false
		}
		return m, tea.Batch(m.loadContextCmd(), m.loadCatalogCmd(false), m.updateWorkspaceGitStatusCmd(), m.listenToChannel())

	case serverStatusMsg:
		m.serverRunning = msg.Running
		if m.activeTask == TaskServe {
			m.activeTask = TaskNone
		}
		if !msg.Running {
			m.statusMsg = "Dev server stopped."
		} else {
			m.statusMsg = "Dev server running."
		}
		return m, m.listenToChannel()
	case workspaceGitStatusMsg:
		m.workspaceBranch = msg.Branch
		m.workspaceChanges = msg.Changes
		return m, nil
	}

	return m, nil
}

func (m *TuiModel) addLog(line string) {
	// Split on embedded newlines — Nuxt can emit multi-line chunks as a single string.
	// If we store them unsplit, the panel renders 1 counted line but N terminal lines → layout breaks.
	line = strings.ReplaceAll(line, "\r\n", "\n")
	sublines := strings.Split(line, "\n")
	for _, sl := range sublines {
		sl = strings.TrimRight(sl, "\r")
		if sl == "" {
			continue
		}
		m.logLines = append(m.logLines, sl)
	}
	if len(m.logLines) > 200 {
		m.logLines = m.logLines[len(m.logLines)-200:]
	}
}


// checkLocalChangesCmd runs a fast local-only git status check for every catalog entry.
// No network activity — only reads the local working tree.
// Runs every ~15s. Also detects which packages have their own .git folder.
func (m *TuiModel) checkLocalChangesCmd() tea.Cmd {
	return func() tea.Msg {
		gitChangesMap := make(map[string]GitChanges)
		localGitDirs := make(map[string]bool)

		if m.catalog == nil {
			return localChangesMsg{GitChanges: gitChangesMap, LocalGitDirs: localGitDirs}
		}

		for _, entry := range m.catalog.Entries {
			short := entry.ShortName
			var kindDir string
			switch entry.Kind {
			case "app":
				kindDir = "apps"
			case "module":
				kindDir = "packages"
			case "theme":
				kindDir = "themes"
			}
			if kindDir == "" {
				continue
			}

			pkgPath := filepath.Join(m.workspaceRoot, kindDir, short)
			gitDir := filepath.Join(pkgPath, ".git")

			if _, err := os.Stat(gitDir); err == nil {
				// Package has its own .git repo
				localGitDirs[short] = true
				added, modified, deleted, err := runGitStatus(pkgPath)
				if err == nil && (added > 0 || modified > 0 || deleted > 0) {
					gitChangesMap[short] = GitChanges{Added: added, Modified: modified, Deleted: deleted}
				}
			} else {
				// Fallback: check monorepo git for this subdirectory
				parentGitDir := filepath.Join(m.workspaceRoot, ".git")
				if _, err := os.Stat(parentGitDir); err == nil {
					relPath := filepath.Join(kindDir, short)
					added, modified, deleted, err := runGitStatusForSubdir(m.workspaceRoot, relPath)
					if err == nil && (added > 0 || modified > 0 || deleted > 0) {
						gitChangesMap[short] = GitChanges{Added: added, Modified: modified, Deleted: deleted}
					}
				}
			}
		}

		return localChangesMsg{GitChanges: gitChangesMap, LocalGitDirs: localGitDirs}
	}
}

// checkRemoteUpdatesCmd runs the slow network-dependent checks:
// - git behind count (uses cached remote refs, NO git fetch)
// - npm latest version check
// Runs every ~5 minutes.
func (m *TuiModel) checkRemoteUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		updatesMap := make(map[string]UpdateInfo)

		if m.catalog == nil {
			return remoteUpdatesMsg{Updates: updatesMap}
		}

		type result struct {
			shortName string
			update    *UpdateInfo
			err       error
		}

		resultsChan := make(chan result, len(m.catalog.Entries))
		sem := make(chan struct{}, 5) // modest concurrency — these are network ops

		for _, e := range m.catalog.Entries {
			sem <- struct{}{}
			go func(entry bridge.CatalogEntry) {
				defer func() { <-sem }()

				res := result{shortName: entry.ShortName}

				short := entry.ShortName
				var kindDir string
				switch entry.Kind {
				case "app":
					kindDir = "apps"
				case "module":
					kindDir = "packages"
				case "theme":
					kindDir = "themes"
				}

				// 1. Git behind check (NO fetch — uses cached remote refs)
				if kindDir != "" {
					pkgPath := filepath.Join(m.workspaceRoot, kindDir, short)
					gitDir := filepath.Join(pkgPath, ".git")
					if _, err := os.Stat(gitDir); err == nil {
						behind, err := runGitBehindCheckNoFetch(pkgPath)
						if err == nil && behind > 0 {
							res.update = &UpdateInfo{LocalGit: true, BehindCount: behind}
						}
					}
				}

				// 2. NPM latest version check
				if entry.Installed && !entry.LocalSource {
					localVer := getLocalVersion(m.workspaceRoot, entry)
					if localVer != "" {
						latestVer, err := fetchLatestNpmVersion(entry.Name)
						if err != nil {
							res.err = err
						} else if semverCompare(localVer, latestVer) < 0 {
							res.update = &UpdateInfo{Npm: true}
						}
					}
				}

				resultsChan <- res
			}(e)
		}

		var checkErr error
		for i := 0; i < len(m.catalog.Entries); i++ {
			res := <-resultsChan
			if res.update != nil {
				updatesMap[res.shortName] = *res.update
			}
			if res.err != nil && checkErr == nil {
				checkErr = res.err
			}
		}

		return remoteUpdatesMsg{Updates: updatesMap, Err: checkErr}
	}
}

func getLocalVersion(workspaceRoot string, entry bridge.CatalogEntry) string {
	short := entry.ShortName
	kindDir := ""
	switch entry.Kind {
	case "app":
		kindDir = "apps"
	case "module":
		kindDir = "packages"
	case "theme":
		kindDir = "themes"
	}
	var paths []string
	if kindDir != "" {
		paths = append(paths, filepath.Join(workspaceRoot, kindDir, short, "package.json"))
	}
	paths = append(paths, filepath.Join(workspaceRoot, "node_modules", entry.Name, "package.json"))

	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			var pkg struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal(data, &pkg); err == nil {
				return pkg.Version
			}
		}
	}
	return ""
}

func semverCompare(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		var n1, n2 int
		fmt.Sscanf(parts1[i], "%d", &n1)
		fmt.Sscanf(parts2[i], "%d", &n2)
		if n1 != n2 {
			return n1 - n2
		}
	}
	return len(parts1) - len(parts2)
}

func (m *TuiModel) loadPackageDetails(pkg *bridge.CatalogEntry) {
	m.promptPkgDeps = []string{}
	m.promptPkgHasPrimevue = false
	m.promptPkgHasTailwind = false

	if pkg == nil {
		return
	}

	short := pkg.ShortName
	var pathsToCheck []string

	kindDir := ""
	switch pkg.Kind {
	case "app":
		kindDir = "apps"
	case "module":
		kindDir = "packages"
	case "theme":
		kindDir = "themes"
	}

	if kindDir != "" {
		pathsToCheck = append(pathsToCheck, filepath.Join(m.workspaceRoot, kindDir, short, "package.json"))
	}
	pathsToCheck = append(pathsToCheck, filepath.Join(m.workspaceRoot, "node_modules", "@owdproject", short, "package.json"))

	var data []byte
	var err error
	for _, p := range pathsToCheck {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}

	if err != nil {
		return
	}

	var pkgJson struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}

	if err := json.Unmarshal(data, &pkgJson); err == nil {
		checkDep := func(name string) {
			if strings.HasPrefix(name, "@owdproject/") {
				for _, d := range m.promptPkgDeps {
					if d == name {
						return
					}
				}
				m.promptPkgDeps = append(m.promptPkgDeps, name)
			}
			if name == "@owdproject/kit-primevue" {
				m.promptPkgHasPrimevue = true
			}
			if name == "@owdproject/kit-tailwind" || name == "tailwindcss" {
				m.promptPkgHasTailwind = true
			}
		}
		for name := range pkgJson.Dependencies {
			checkDep(name)
		}
		for name := range pkgJson.DevDependencies {
			checkDep(name)
		}
	}
}

func (m *TuiModel) isClearLogsClick(msg tea.MouseMsg) bool {
	if msg.Y != 0 {
		return false
	}

	w := m.termWidth
	if w < 120 {
		return false
	}

	leftW := w * 40 / 100
	midW := w * 28 / 100
	catW := leftW + midW

	panelStartX := catW - 1

	rightPanelTitle := "Logs"
	if m.wizard != nil && !m.wizard.IsComplete() {
		rightPanelTitle = "Wizard Setup"
	} else if m.activeTask == TaskSetup || m.activeTask == TaskWipe {
		rightPanelTitle = "Setup"
	}

	prefixLen := len(rightPanelTitle) + 2
	clearStartX := panelStartX + 2 + prefixLen
	clearEndX := clearStartX + 7

	return msg.X >= clearStartX && msg.X < clearEndX
}
