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
	if (m.activeTask == TaskSetup || m.activeTask == TaskWipe) &&
		promptToShow != PromptResolveDependency &&
		(promptToShow == PromptNone || promptToShow == PromptUninstallConfirm || promptToShow == PromptInstallMethod || promptToShow == PromptWipeWorkspaceConfirm) {
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
	case PromptResolveDependency:
		return m.handleResolveDependencyKeys(msg)
	case PromptForceReinstallConfirm:
		return m.handleForceReinstallConfirmKeys(msg)
	case PromptWipeWorkspaceConfirm:
		return m.handleWipeWorkspaceConfirmKeys(msg)
	default:
		if msg.String() == "esc" || msg.String() == "q" {
			m.activePrompt = PromptNone
			m.promptPkg = nil
			m.promptItem = nil
		}
		return m, nil
	}
}

func (m *TuiModel) handleSettingsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()
	isInputFocused := m.settingsSel == 2 || m.settingsSel == 3

	if isInputFocused {
		switch keyStr {
		case "esc":
			m.activePrompt = PromptNone
			m.settingsOrgsInput.Blur()
			m.settingsUserInput.Blur()
			return m, nil
		case "up", "down", "enter":
			// Intercept and fall through to navigation/actions below
		default:
			var cmd tea.Cmd
			if m.settingsSel == 2 {
				m.settingsOrgsInput, cmd = m.settingsOrgsInput.Update(msg)
			} else {
				m.settingsUserInput, cmd = m.settingsUserInput.Update(msg)
			}
			return m, cmd
		}
	}

	switch keyStr {
	case "esc", "q":
		m.activePrompt = PromptNone
		m.settingsOrgsInput.Blur()
		m.settingsUserInput.Blur()
		return m, nil
	case "up", "k":
		if m.settingsSel >= SettingsFieldCount {
			m.settingsSel = SettingsFieldCount - 1
		} else if m.settingsSel > 0 {
			m.settingsSel--
		}
		m.updateSettingsFocus()
	case "down", "j", "enter":
		if keyStr == "enter" && isInputFocused {
			m.settingsSel = 5 // Jump directly to SAVE button
			m.updateSettingsFocus()
			return m, nil
		}

		if keyStr == "down" || keyStr == "j" {
			if m.settingsSel == SettingsFieldCount-1 {
				m.settingsSel = SettingsFieldCount
			} else if m.settingsSel < SettingsFieldCount-1 {
				m.settingsSel++
			}
			m.updateSettingsFocus()
		} else if keyStr == "enter" {
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
				// Handled by textinput
			case 3: // GitHub User
				// Handled by textinput
			case 4: // Reset Workspace
				m.activePrompt = PromptWipeWorkspaceConfirm
				m.promptSel = 1 // default to No
			case 5: // Save
				m.activePrompt = PromptNone
				m.settingsOrgsInput.Blur()
				m.settingsUserInput.Blur()
				if m.ctx != nil {
					settings := m.ctx.Settings
					settings.InstallMode = m.settingsInstallMode
					settings.CatalogSort = m.settingsCatalogSort

					// Parse Trusted Orgs (GithubOrgs) from comma-separated list
					rawOrgs := m.settingsOrgsInput.Value()
					var orgs []string
					for _, part := range strings.Split(rawOrgs, ",") {
						trimmed := strings.TrimSpace(part)
						if trimmed != "" {
							orgs = append(orgs, trimmed)
						}
					}
					settings.GithubOrgs = orgs

					// Parse GitHub User (GithubUser)
					ghUserVal := m.settingsUserInput.Value()
					if ghUserVal == "" {
						settings.GithubUser = nil
					} else {
						settings.GithubUser = &ghUserVal
					}

					payload := &bridge.WritePayload{
						Settings: &settings,
					}
					m.statusMsg = "Saving settings…"
					m.addLog("ℹ Saving settings configuration…")
					if err := bridge.WriteChanges(m.workspaceRoot, payload); err != nil {
						m.statusMsg = fmt.Sprintf("Save failed: %v", err)
						m.addLog(fmt.Sprintf("✗ Save settings failed: %v", err))
					} else {
						m.statusMsg = "Settings saved successfully."
						m.addLog("✓ Settings saved successfully.")
					}
				}
				m.loading = true
				return m, tea.Batch(m.loadContextCmd(), m.loadCatalogCmd(true))
			case 6: // Cancel
				m.activePrompt = PromptNone
				m.settingsOrgsInput.Blur()
				m.settingsUserInput.Blur()
			}
		}
	case "left", "h":
		if m.settingsSel == SettingsFieldCount+1 {
			m.settingsSel = SettingsFieldCount
		}
	case "right", "l":
		if m.settingsSel == SettingsFieldCount {
			m.settingsSel = SettingsFieldCount + 1
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
		m.addLog("✗ Uninstall cancelled")
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
			return m.triggerUninstall(pkg)
		}
	}
	return m, nil
}

func (m *TuiModel) handleResolveDependencyKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	shortName := "package"
	if m.promptItem != nil {
		shortName = m.promptItem.ShortName
	}
	if shortName == "" && m.promptPkg != nil {
		shortName = m.promptPkg.ShortName
	}

	var methods []struct {
		Name  string
		Label string
	}
	if isLocallyAvailable(m.workspaceRoot, shortName) {
		methods = append(methods, struct{ Name, Label string }{"local", "Use Existing Local Folder"})
	}
	methods = append(methods,
		struct{ Name, Label string }{"git-ssh", "Git SSH"},
		struct{ Name, Label string }{"git-https", "Git HTTPS"},
		struct{ Name, Label string }{"npm", "NPM Package"},
	)

	switch msg.String() {
	case "esc", "q":
		m.activePrompt = PromptNone
		m.promptItem = nil
		m.promptPkg = nil
		m.startServerAfterSetup = false
		m.addLog("✖ Wizard aborted by user")
		m.resolveEnginePrompt("abort")
		return m, nil
	case "up", "k":
		if m.promptSel > 0 {
			m.promptSel--
		}
	case "down", "j":
		if m.promptSel < len(methods)-1 {
			m.promptSel++
		}
	case "enter":
		if m.promptSel >= 0 && m.promptSel < len(methods) {
			selected := methods[m.promptSel].Name
			m.lastInstallMethod = selected
			m.activePrompt = PromptNone
			item := m.promptItem
			m.promptItem = nil
			m.promptPkg = nil
			if item != nil {
				m.addLog(fmt.Sprintf("ℹ Download method selected: %s", resolveMethodLabel(selected)))
			}
			m.resolveEnginePrompt(selected)
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
		m.addLog("✗ Install cancelled")
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
			m.addLog("ℹ Initiating workspace reset task…")
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
	m.addLog(fmt.Sprintf("⚙ Selected Option: Uninstall package %s", pkg.Name))

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
	return m, m.listenToChannel()
}

func (m *TuiModel) triggerInstall(pkg *bridge.CatalogEntry, method string) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Installing %s via %s…", pkg.ShortName, method)
	m.addLog(fmt.Sprintf("⚙ Selected Option: Install package %s via %s", pkg.Name, method))

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
		owner := m.resolveOwner(pkg.Name)
		var gitUrl string
		if method == "git-ssh" {
			gitUrl = fmt.Sprintf("git@github.com:%s/%s.git", owner, pkg.ShortName)
		} else {
			gitUrl = fmt.Sprintf("https://github.com/%s/%s.git", owner, pkg.ShortName)
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
	return m, m.listenToChannel()
}

func (m *TuiModel) triggerForceReinstall(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Force reinstalling %s…", pkg.ShortName)
	m.addLog(fmt.Sprintf("ℹ Deleting local folder and reinstalling %s", pkg.Name))

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
		m.addLog(fmt.Sprintf("ℹ Removing directory: %s", pkgPath))
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
	return m, m.listenToChannel()
}

func (m *TuiModel) triggerUpdate(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Updating %s…", pkg.ShortName)
	m.addLog(fmt.Sprintf("ℹ Updating %s", pkg.Name))

	workspaceRoot := m.workspaceRoot
	runtime := m.runtime
	pkgName := pkg.Name
	short := pkg.ShortName
	kind := pkg.Kind

	go func() {
		kindDir := ""
		switch kind {
		case "app":
			kindDir = "apps"
		case "module":
			kindDir = "packages"
		case "theme":
			kindDir = "themes"
		}
		if kindDir != "" {
			pkgPath := filepath.Join(workspaceRoot, kindDir, short)
			gitDir := filepath.Join(pkgPath, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				runtime.msgChan <- logLineMsg("ℹ Local Git repository detected. Running git pull…")
				runtime.runProcessAndStreamLogs(pkgPath, "git", []string{"pull"})
				return
			}
		}

		runtime.msgChan <- logLineMsg(fmt.Sprintf("ℹ Running pnpm install %s@latest…", pkgName))
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install", pkgName + "@latest"}); err != nil {
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}
		runtime.msgChan <- logLineMsg("ℹ Preparing workspace modules…")
		err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"run", "prepare:modules"})
		runtime.msgChan <- taskFinishedMsg{Success: err == nil, Err: err}
	}()
	return m, m.listenToChannel()
}

// ─────────────────────────────────────────────
// Wipe workspace
// ─────────────────────────────────────────────

func (m *TuiModel) runWipeWorkspaceCmd() tea.Cmd {
	return func() tea.Msg {
		m.RunWipeWorkspaceTask()
		return logLineMsg("ℹ Wipe workspace task started…")
	}
}

func (m *TuiModel) updateSettingsFocus() {
	if m.activePrompt == PromptSettings {
		if m.settingsSel == 2 {
			m.settingsOrgsInput.Focus()
			m.settingsUserInput.Blur()
		} else if m.settingsSel == 3 {
			m.settingsUserInput.Focus()
			m.settingsOrgsInput.Blur()
		} else {
			m.settingsOrgsInput.Blur()
			m.settingsUserInput.Blur()
		}
	} else {
		m.settingsOrgsInput.Blur()
		m.settingsUserInput.Blur()
	}
}
