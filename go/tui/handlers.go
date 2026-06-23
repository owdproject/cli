package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"owd-cli/bridge"

	tea "github.com/charmbracelet/bubbletea"
)

// ─────────────────────────────────────────────
// Prompt keys & Navigation
// ─────────────────────────────────────────────

func (m *TuiModel) handlePromptKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	promptToShow := m.activePrompt
	if (m.activeTask == TaskSetup || m.activeTask == TaskWipe) && (promptToShow == PromptNone || promptToShow == PromptUninstallConfirm || promptToShow == PromptInstallMethod || promptToShow == PromptWipeWorkspaceConfirm) {
		promptToShow = PromptSetupProgress
	}
	if promptToShow == PromptSetupProgress {
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}

	switch promptToShow {
	case PromptSettings:
		return m.handleSettingsKeys(msg)
	case PromptManagePackage:
		return m.handleManagePackageKeys(msg)
	case PromptUninstallConfirm:
		return m.handleUninstallConfirmKeys(msg)
	case PromptInstallMethod:
		return m.handleInstallMethodKeys(msg)
	case PromptForceReinstallConfirm:
		return m.handleForceReinstallConfirmKeys(msg)
	case PromptWipeWorkspaceConfirm:
		return m.handleWipeWorkspaceConfirmKeys(msg)
	default:
		if msg.String() == "esc" || msg.String() == "q" {
			m.activePrompt = PromptNone
			m.promptPkg = nil
			m.promptQueue = nil
			m.promptQueueIndex = 0
			m.activeThemeDep = ""
		}
		return m, nil
	}
}

func (m *TuiModel) handleSettingsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptNone
		return m, nil
	case "up", "k":
		if m.settingsSel >= SettingsFieldCount {
			m.settingsSel = SettingsFieldCount - 1
		} else if m.settingsSel > 0 {
			m.settingsSel--
		}
	case "down", "j":
		if m.settingsSel == SettingsFieldCount-1 {
			m.settingsSel = SettingsFieldCount
		} else if m.settingsSel < SettingsFieldCount-1 {
			m.settingsSel++
		}
	case "left", "h":
		if m.settingsSel == SettingsFieldCount+1 {
			m.settingsSel = SettingsFieldCount
		}
	case "right", "l":
		if m.settingsSel == SettingsFieldCount {
			m.settingsSel = SettingsFieldCount + 1
		}
	case "enter":
		switch m.settingsSel {
		case 0: // Install Mode
			if m.settingsInstallMode == "npm" {
				m.settingsInstallMode = "workspace"
			} else {
				m.settingsInstallMode = "npm"
			}
		case 1: // Catalog Sort
			modes := []string{"updated", "name", "stars", "installed"}
			idx := 0
			for i, mth := range modes {
				if mth == m.settingsCatalogSort {
					idx = (i + 1) % len(modes)
					break
				}
			}
			m.settingsCatalogSort = modes[idx]
		case 2: // Trusted Orgs
			m.addLog(">>> To configure Trusted Orgs, modify .desktop/settings.json (githubOrgs field).")
		case 3: // GitHub User
			m.addLog(">>> To configure GitHub User, modify .desktop/settings.json or use OWD_GITHUB_USER env var.")
		case 4: // Reset Workspace
			m.activePrompt = PromptWipeWorkspaceConfirm
			m.promptSel = 1 // default to No
		case 5: // Save
			m.activePrompt = PromptNone
			if m.ctx != nil {
				settings := m.ctx.Settings
				settings.InstallMode = m.settingsInstallMode
				settings.CatalogSort = m.settingsCatalogSort
				payload := &bridge.WritePayload{
					Settings: &settings,
				}
				m.statusMsg = "Saving settings…"
				m.addLog(">>> Saving settings configuration…")
				if err := bridge.WriteChanges(m.workspaceRoot, payload); err != nil {
					m.statusMsg = fmt.Sprintf("Save failed: %v", err)
					m.addLog(fmt.Sprintf(">>> Save settings failed: %v", err))
				} else {
					m.statusMsg = "Settings saved successfully."
					m.addLog(">>> Settings saved successfully.")
				}
			}
			m.loading = true
			return m, tea.Batch(m.loadContextCmd(), m.loadCatalogCmd(false))
		case 6: // Cancel
			m.activePrompt = PromptNone
		}
	}
	return m, nil
}

func (m *TuiModel) handleManagePackageKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	actionsCount := len(ManagePackageActions)

	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptNone
		m.promptPkg = nil
		return m, nil
	case "up", "k":
		if m.promptSel > 0 {
			m.promptSel--
		}
	case "down", "j":
		if m.promptSel < actionsCount-1 {
			m.promptSel++
		}
	case "enter":
		pkg := m.promptPkg
		m.activePrompt = PromptNone
		m.promptPkg = nil
		switch m.promptSel {
		case 0: // Update Package
			return m.triggerUpdate(pkg)
		case 1: // Switch to NPM
			m.lastInstallMethod = "npm"
			return m.triggerInstall(pkg, "npm")
		case 2: // Switch to Git HTTPS
			m.lastInstallMethod = "git-https"
			return m.triggerInstall(pkg, "git-https")
		case 3: // Switch to Git SSH
			m.lastInstallMethod = "git-ssh"
			return m.triggerInstall(pkg, "git-ssh")
		case 4: // Force Re-download
			m.promptPkg = pkg
			m.activePrompt = PromptForceReinstallConfirm
			m.promptSel = 1 // default to No
			return m, nil
		case 5: // Back
			return m, nil
		}
	}
	return m, nil
}

func (m *TuiModel) handleUninstallConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptNone
		m.promptPkg = nil
		m.promptQueue = nil
		m.promptQueueIndex = 0
		return m, nil
	case "left", "h", "up", "k":
		m.promptSel = 0
	case "right", "l", "down", "j":
		m.promptSel = 1
	case "enter":
		pkg := m.promptPkg
		if len(m.promptQueue) > 0 {
			if m.promptSel == 0 { // Yes
				m.finalizedRemoves = append(m.finalizedRemoves, pkg.Name)
			}
			m.promptQueueIndex++
			return m, m.processNextQueueDecision()
		}

		m.activePrompt = PromptNone
		m.promptPkg = nil
		if m.promptSel == 0 {
			return m.triggerUninstall(pkg)
		}
	}
	return m, nil
}

func (m *TuiModel) handleInstallMethodKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pkg := m.promptPkg
	if pkg == nil {
		m.activePrompt = PromptNone
		return m, nil
	}
	methods := m.getInstallMethods(pkg)
	methodsCount := len(methods)

	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptNone
		m.promptPkg = nil
		m.promptQueue = nil
		m.promptQueueIndex = 0
		return m, nil
	case "up", "k":
		if m.promptSel > 0 {
			m.promptSel--
		}
	case "down", "j":
		if m.promptSel < methodsCount-1 {
			m.promptSel++
		}
	case "enter":
		if m.promptSel >= 0 && m.promptSel < methodsCount {
			selectedMethod := methods[m.promptSel].Name
			if len(m.promptQueue) > 0 {
				m.lastInstallMethod = selectedMethod
				m.finalizedAdds[pkg.Name] = selectedMethod
				m.promptQueueIndex++
				return m, m.processNextQueueDecision()
			}

			m.activePrompt = PromptNone
			m.promptPkg = nil
			m.lastInstallMethod = selectedMethod
			return m.triggerInstall(pkg, selectedMethod)
		}
	}
	return m, nil
}

func (m *TuiModel) handleForceReinstallConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptNone
		m.promptPkg = nil
		return m, nil
	case "left", "h", "up", "k":
		m.promptSel = 0
	case "right", "l", "down", "j":
		m.promptSel = 1
	case "enter":
		pkg := m.promptPkg
		m.activePrompt = PromptNone
		m.promptPkg = nil
		if m.promptSel == 0 {
			return m.triggerForceReinstall(pkg)
		}
	}
	return m, nil
}

func (m *TuiModel) handleWipeWorkspaceConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptSettings
		return m, nil
	case "left", "h", "up", "k":
		m.promptSel = 0
	case "right", "l", "down", "j":
		m.promptSel = 1
	case "enter":
		if m.promptSel == 0 { // Yes
			m.activePrompt = PromptNone
			m.activeTask = TaskWipe
			m.statusMsg = "Resetting workspace…"
			m.addLog(">>> Initiating workspace reset task…")
			return m, m.runWipeWorkspaceCmd()
		} else { // No
			m.activePrompt = PromptSettings
			return m, nil
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Installer Actions
// ─────────────────────────────────────────────

func (m *TuiModel) triggerUninstall(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Uninstalling %s…", pkg.ShortName)
	m.addLog(fmt.Sprintf(">>> Uninstalling %s", pkg.Name))

	payload := &bridge.WritePayload{
		Config:       &bridge.Config{Theme: m.ctx.Config.Theme, Apps: m.ctx.Config.Apps, Modules: m.ctx.Config.Modules},
		DepsToRemove: []string{pkg.Name},
	}
	if pkg.Kind == "app" {
		var next []string
		for _, a := range m.ctx.Config.Apps {
			if a != pkg.Name {
				next = append(next, a)
			}
		}
		payload.Config.Apps = next
	} else if pkg.Kind == "module" {
		var next []string
		for _, mod := range m.ctx.Config.Modules {
			if mod != pkg.Name {
				next = append(next, mod)
			}
		}
		payload.Config.Modules = next
	} else if pkg.Kind == "theme" {
		e := ""
		payload.Config.Theme = &e
	}

	if err := bridge.WriteChanges(m.workspaceRoot, payload); err != nil {
		m.activeTask = TaskNone
		m.statusMsg = fmt.Sprintf("Uninstall failed: %v", err)
		return m, nil
	}
	m.RunSetupTask(make(map[string]string))
	return m, nil
}

func (m *TuiModel) triggerInstall(pkg *bridge.CatalogEntry, method string) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Installing %s via %s…", pkg.ShortName, method)
	m.addLog(fmt.Sprintf(">>> Installing %s via %s", pkg.Name, method))

	payload := &bridge.WritePayload{
		Config:    &bridge.Config{Theme: m.ctx.Config.Theme, Apps: m.ctx.Config.Apps, Modules: m.ctx.Config.Modules},
		DepsToAdd: make(map[string]string),
	}
	version := "latest"
	if pkg.Version != nil {
		version = *pkg.Version
	}

	if method == "npm" {
		payload.DepsToAdd[pkg.Name] = version
	} else if method == "local" {
		payload.DepsToAdd[pkg.Name] = "workspace:*"
	} else {
		user := "owdproject"
		if m.ctx.Settings.GithubUser != nil && *m.ctx.Settings.GithubUser != "" {
			user = *m.ctx.Settings.GithubUser
		}
		var gitUrl string
		if method == "git-ssh" {
			gitUrl = fmt.Sprintf("git@github.com:%s/%s.git", user, pkg.ShortName)
		} else {
			gitUrl = fmt.Sprintf("https://github.com/%s/%s.git", user, pkg.ShortName)
		}
		payload.DepsToAdd[pkg.Name] = "workspace:*"
		settings := m.ctx.Settings
		if settings.LastInstallChoices == nil {
			settings.LastInstallChoices = make(map[string]interface{})
		}
		settings.LastInstallChoices[pkg.Name] = map[string]string{"type": "git", "gitUrl": gitUrl}
		payload.Settings = &settings
	}

	if pkg.Kind == "app" {
		payload.Config.Apps = append(payload.Config.Apps, pkg.Name)
	} else if pkg.Kind == "module" {
		payload.Config.Modules = append(payload.Config.Modules, pkg.Name)
	} else if pkg.Kind == "theme" {
		payload.Config.Theme = &pkg.Name
	}

	if err := bridge.WriteChanges(m.workspaceRoot, payload); err != nil {
		m.activeTask = TaskNone
		m.statusMsg = fmt.Sprintf("Install failed: %v", err)
		return m, nil
	}
	m.RunSetupTask(map[string]string{pkg.Name: method})
	return m, nil
}

func (m *TuiModel) triggerForceReinstall(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Force reinstalling %s…", pkg.ShortName)
	m.addLog(fmt.Sprintf(">>> Deleting local folder and reinstalling %s", pkg.Name))

	// Delete local folder
	short := pkg.ShortName
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
		pkgPath := filepath.Join(m.workspaceRoot, kindDir, short)
		m.addLog(fmt.Sprintf(">>> Removing directory: %s", pkgPath))
		_ = os.RemoveAll(pkgPath)
	}

	// Reinstall using the original source method
	method := "npm"
	if pkg.LocalSource {
		if m.ctx != nil && m.ctx.Settings.LastInstallChoices != nil {
			if choice, exists := m.ctx.Settings.LastInstallChoices[pkg.Name]; exists {
				if choiceMap, ok := choice.(map[string]interface{}); ok {
					if gitUrl, hasGit := choiceMap["gitUrl"].(string); hasGit {
						if strings.HasPrefix(gitUrl, "git@") {
							method = "git-ssh"
						} else {
							method = "git-https"
						}
					}
				}
			}
		}
	}

	m.RunSetupTask(map[string]string{pkg.Name: method})
	return m, nil
}

func (m *TuiModel) triggerUpdate(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Updating %s…", pkg.ShortName)
	m.addLog(fmt.Sprintf(">>> Updating %s", pkg.Name))

	go func() {
		short := pkg.ShortName
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
			pkgPath := filepath.Join(m.workspaceRoot, kindDir, short)
			gitDir := filepath.Join(pkgPath, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				m.runtime.msgChan <- logLineMsg(">>> Local Git repository detected. Running git pull…")
				m.runProcessAndStreamLogs(pkgPath, "git", []string{"pull"})
				return
			}
		}

		m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Running pnpm install %s@latest…", pkg.Name))
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install", pkg.Name + "@latest"}); err != nil {
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}
		m.runtime.msgChan <- logLineMsg(">>> Preparing workspace modules…")
		err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"})
		m.runtime.msgChan <- taskFinishedMsg{Success: err == nil, Err: err}
	}()
	return m, nil
}

// ─────────────────────────────────────────────
// Queue reviews
// ─────────────────────────────────────────────

func (m *TuiModel) startStartupCheck() tea.Cmd {
	m.promptQueue = []pendingDecision{}
	m.promptQueueIndex = 0
	m.finalizedAdds = make(map[string]string)
	m.finalizedRemoves = []string{}
	m.finalizedTheme = nil

	if m.catalog == nil {
		return nil
	}

	nonInstallable := map[string]bool{
		"@owdproject/core": true,
		"@owdproject/cli":  true,
		"@owdproject/nx":   true,
	}

	queued := map[string]bool{}

	for _, entry := range m.catalog.Entries {
		if entry.Installed && !entry.LocalSource && !entry.InPackageJson {
			m.promptQueue = append(m.promptQueue, pendingDecision{
				PkgName:   entry.Name,
				ShortName: entry.ShortName,
				Action:    "install",
				Kind:      entry.Kind,
				Entry:     entry,
			})
			queued[entry.Name] = true
		}
	}

	var activeThemeShort string
	for _, dec := range m.promptQueue {
		if dec.Kind == "theme" {
			activeThemeShort = dec.ShortName
			break
		}
	}
	if activeThemeShort == "" && m.ctx != nil && m.ctx.Config.Theme != nil && *m.ctx.Config.Theme != "" {
		themeName := *m.ctx.Config.Theme
		if idx := strings.LastIndex(themeName, "/"); idx >= 0 {
			activeThemeShort = themeName[idx+1:]
		} else {
			activeThemeShort = themeName
		}
	}

	if activeThemeShort != "" {
		themeDeps, err := getThemeDependencies(m.workspaceRoot, activeThemeShort)
		if err == nil {
			for _, dep := range themeDeps {
				if nonInstallable[dep] || queued[dep] {
					continue
				}
				short := dep
				if idx := strings.LastIndex(dep, "/"); idx >= 0 {
					short = dep[idx+1:]
				}
				if !strings.HasPrefix(short, "kit-") && !strings.HasPrefix(short, "module-") {
					continue
				}
				if isLocallyAvailable(m.workspaceRoot, short) {
					continue
				}
				var entry *bridge.CatalogEntry
				if m.catalog != nil {
					for _, e := range m.catalog.Entries {
						if e.Name == dep {
							entry = &e
							break
						}
					}
				}
				if entry == nil {
					synth := bridge.CatalogEntry{
						Name:      dep,
						ShortName: short,
						Kind:      "module",
					}
					entry = &synth
				}
				m.promptQueue = append(m.promptQueue, pendingDecision{
					PkgName:   dep,
					ShortName: short,
					Action:    "install",
					Kind:      entry.Kind,
					Entry:     *entry,
				})
				queued[dep] = true
			}
		}
	}

	if len(m.promptQueue) == 0 {
		m.activePrompt = PromptNone
		return nil
	}

	return m.processNextQueueDecision()
}

func (m *TuiModel) processNextQueueDecision() tea.Cmd {
	if m.promptQueueIndex >= len(m.promptQueue) {
		m.activePrompt = PromptNone
		return m.applyQueueChangesCmd()
	}

	dec := m.promptQueue[m.promptQueueIndex]
	m.promptPkg = &dec.Entry

	if dec.Action == "uninstall" {
		m.activePrompt = PromptUninstallConfirm
		m.promptSel = 1 // default to No
		return nil
	}

	if dec.Kind == "theme" && (dec.Entry.LocalSource || dec.Entry.InPackageJson) {
		themeName := dec.PkgName
		m.finalizedTheme = &themeName
		m.promptQueueIndex++
		return m.processNextQueueDecision()
	}

	m.activePrompt = PromptInstallMethod
	methods := m.getInstallMethods(&dec.Entry)
	selIdx := 0
	isLocal := false
	if dec.Entry.LocalSource {
		for idx, mth := range methods {
			if mth.Name == "local" {
				selIdx = idx
				isLocal = true
				break
			}
		}
	}
	if !isLocal {
		for idx, mth := range methods {
			if mth.Name == m.lastInstallMethod {
				selIdx = idx
				break
			}
		}
	}
	m.promptSel = selIdx
	return nil
}

func (m *TuiModel) applyQueueChangesCmd() tea.Cmd {
	m.activeTask = TaskSetup
	m.statusMsg = "Applying queued package changes…"
	m.addLog(">>> Applying queued package configuration changes…")

	payload := &bridge.WritePayload{
		Config:       &bridge.Config{Theme: m.ctx.Config.Theme, Apps: m.ctx.Config.Apps, Modules: m.ctx.Config.Modules},
		DepsToAdd:    make(map[string]string),
		DepsToRemove: []string{},
	}

	for _, name := range m.finalizedRemoves {
		payload.DepsToRemove = append(payload.DepsToRemove, name)

		var nextApps []string
		for _, a := range payload.Config.Apps {
			if a != name {
				nextApps = append(nextApps, a)
			}
		}
		payload.Config.Apps = nextApps

		var nextModules []string
		for _, mod := range payload.Config.Modules {
			if mod != name {
				nextModules = append(nextModules, mod)
			}
		}
		payload.Config.Modules = nextModules
	}

	for _, name := range m.finalizedRemoves {
		if payload.Config.Theme != nil && *payload.Config.Theme == name {
			e := ""
			payload.Config.Theme = &e
		}
	}

	for name, method := range m.finalizedAdds {
		var entry *bridge.CatalogEntry
		if m.catalog != nil {
			for _, e := range m.catalog.Entries {
				if e.Name == name {
					entry = &e
					break
				}
			}
		}
		version := "latest"
		if entry != nil && entry.Version != nil {
			version = *entry.Version
		}

		if method == "npm" {
			payload.DepsToAdd[name] = version
		} else if method == "local" {
			payload.DepsToAdd[name] = "workspace:*"
		} else {
			user := "owdproject"
			if m.ctx != nil && m.ctx.Settings.GithubUser != nil && *m.ctx.Settings.GithubUser != "" {
				user = *m.ctx.Settings.GithubUser
			}
			var shortName string
			if entry != nil {
				shortName = entry.ShortName
			} else {
				shortName = name[strings.LastIndex(name, "/")+1:]
			}
			var gitUrl string
			if method == "git-ssh" {
				gitUrl = fmt.Sprintf("git@github.com:%s/%s.git", user, shortName)
			} else {
				gitUrl = fmt.Sprintf("https://github.com/%s/%s.git", user, shortName)
			}
			payload.DepsToAdd[name] = "workspace:*"

			if m.ctx != nil {
				settings := m.ctx.Settings
				if settings.LastInstallChoices == nil {
					settings.LastInstallChoices = make(map[string]interface{})
				}
				settings.LastInstallChoices[name] = map[string]string{"type": "git", "gitUrl": gitUrl}
				payload.Settings = &settings
			}
		}

		if entry != nil {
			if entry.Kind == "app" {
				found := false
				for _, a := range payload.Config.Apps {
					if a == name {
						found = true
						break
					}
				}
				if !found {
					payload.Config.Apps = append(payload.Config.Apps, name)
				}
			} else if entry.Kind == "module" {
				found := false
				for _, mod := range payload.Config.Modules {
					if mod == name {
						found = true
						break
					}
				}
				if !found {
					payload.Config.Modules = append(payload.Config.Modules, name)
				}
			} else if entry.Kind == "theme" {
				themeName := name
				payload.Config.Theme = &themeName
			}
		}
	}

	if m.finalizedTheme != nil {
		payload.Config.Theme = m.finalizedTheme
	}

	finalAdds := m.finalizedAdds
	m.pendingPackages = make(map[string]bool)
	m.pendingTheme = nil

	return func() tea.Msg {
		if err := bridge.WriteChanges(m.workspaceRoot, payload); err != nil {
			return taskFinishedMsg{Success: false, Err: err}
		}

		m.RunSetupTask(finalAdds)
		return logLineMsg(">>> Workspace changes written. Running setup task…")
	}
}

func (m *TuiModel) runWipeWorkspaceCmd() tea.Cmd {
	return func() tea.Msg {
		m.RunWipeWorkspaceTask()
		return nil
	}
}

func (m *TuiModel) startQueueReview() tea.Cmd {
	m.promptQueue = []pendingDecision{}
	m.promptQueueIndex = 0
	m.finalizedAdds = make(map[string]string)
	m.finalizedRemoves = []string{}
	m.finalizedTheme = nil

	for name, on := range m.pendingPackages {
		var entry *bridge.CatalogEntry
		if m.catalog != nil {
			for _, e := range m.catalog.Entries {
				if e.Name == name {
					entry = &e
					break
				}
			}
		}
		if entry == nil {
			continue
		}

		if on {
			m.promptQueue = append(m.promptQueue, pendingDecision{
				PkgName:   entry.Name,
				ShortName: entry.ShortName,
				Action:    "install",
				Kind:      entry.Kind,
				Entry:     *entry,
			})
		} else {
			m.promptQueue = append(m.promptQueue, pendingDecision{
				PkgName:   entry.Name,
				ShortName: entry.ShortName,
				Action:    "uninstall",
				Kind:      entry.Kind,
				Entry:     *entry,
			})
		}
	}

	if m.pendingTheme != nil {
		var entry *bridge.CatalogEntry
		if m.catalog != nil {
			for _, e := range m.catalog.Entries {
				if e.Name == *m.pendingTheme {
					entry = &e
					break
				}
			}
		}
		if entry != nil {
			m.promptQueue = append(m.promptQueue, pendingDecision{
				PkgName:   entry.Name,
				ShortName: entry.ShortName,
				Action:    "install",
				Kind:      "theme",
				Entry:     *entry,
			})
		}
	}

	// Queue theme dependencies upfront
	var themeShort string
	for _, dec := range m.promptQueue {
		if dec.Kind == "theme" && dec.Action == "install" {
			themeShort = dec.ShortName
			break
		}
	}
	if themeShort != "" {
		themeDeps, err := getThemeDependencies(m.workspaceRoot, themeShort)
		if err == nil {
			nonInstallable := map[string]bool{
				"@owdproject/core": true,
				"@owdproject/cli":  true,
				"@owdproject/nx":   true,
			}
			for _, dep := range themeDeps {
				if nonInstallable[dep] {
					continue
				}
				// Skip if already in finalized Adds/Removes or pending packages
				alreadyQueued := false
				for _, dec := range m.promptQueue {
					if dec.PkgName == dep {
						alreadyQueued = true
						break
					}
				}
				if alreadyQueued {
					continue
				}

				short := dep
				if idx := strings.LastIndex(dep, "/"); idx >= 0 {
					short = dep[idx+1:]
				}
				if !strings.HasPrefix(short, "kit-") && !strings.HasPrefix(short, "module-") {
					continue
				}
				if isLocallyAvailable(m.workspaceRoot, short) {
					continue
				}

				var depEntry *bridge.CatalogEntry
				if m.catalog != nil {
					for _, e := range m.catalog.Entries {
						if e.Name == dep {
							depEntry = &e
							break
						}
					}
				}
				if depEntry == nil {
					synth := bridge.CatalogEntry{
						Name:      dep,
						ShortName: short,
						Kind:      "module",
					}
					depEntry = &synth
				}

				m.promptQueue = append(m.promptQueue, pendingDecision{
					PkgName:   dep,
					ShortName: short,
					Action:    "install",
					Kind:      depEntry.Kind,
					Entry:     *depEntry,
				})
			}
		}
	}

	if len(m.promptQueue) == 0 {
		m.activePrompt = PromptNone
		return nil
	}

	return m.processNextQueueDecision()
}
