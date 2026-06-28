package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"owd-cli/bridge"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// ProgressBar & Modal Renderers
// ─────────────────────────────────────────────

func renderProgressBar(step, total, width int) string {
	if total <= 0 {
		return progressBarEmptyStyle.Render(strings.Repeat("░", width))
	}
	filledWidth := (step * width) / total
	if filledWidth > width {
		filledWidth = width
	}
	if filledWidth < 0 {
		filledWidth = 0
	}
	emptyWidth := width - filledWidth
	filled := progressBarFilledStyle.Render(strings.Repeat("█", filledWidth))
	empty := progressBarEmptyStyle.Render(strings.Repeat("░", emptyWidth))
	return filled + empty
}

func (m *TuiModel) renderModal(prompt PromptType) string {
	pkg := m.promptPkg
	switch prompt {
	case PromptInstallMethod:
		return m.renderInstallMethodModal(pkg)
	case PromptUninstallConfirm:
		return m.renderUninstallConfirmModal(pkg)
	case PromptForceReinstallConfirm:
		return m.renderForceReinstallConfirmModal(pkg)
	case PromptResolveDependency:
		return m.renderResolveDependencyModal()
	case PromptSetupProgress:
		return m.renderSetupProgressModal()
	case PromptManagePackage:
		return m.renderManagePackageModal(pkg)
	case PromptSettings:
		return m.renderSettingsModal()
	case PromptWipeWorkspaceConfirm:
		return m.renderWipeWorkspaceConfirmModal()
	default:
		return ""
	}
}

func (m *TuiModel) renderInstallMethodModal(pkg *bridge.CatalogEntry) string {
	var content strings.Builder
	content.WriteString(boldStyle.Render("Install source for ") + accentStyle.Render(pkg.ShortName) + "\n\n")
	methods := m.getInstallMethods(pkg)
	for i, mth := range methods {
		if i > 0 {
			if mth.Name != "local" && methods[i-1].Name == "local" {
				content.WriteString("\n  " + subtleStyle.Render(strings.Repeat("─", 62)) + "\n\n")
			} else {
				content.WriteString("\n")
			}
		}
		var btn string
		if i == m.promptSel {
			btn = modalOptionActive.Render(mth.Label)
		} else {
			btn = modalOptionInactive.Render(mth.Label)
		}
		content.WriteString("  " + btn + "\n")
		content.WriteString("    " + mutedStyle.Render(mth.Desc) + "\n")
	}
	content.WriteString("\n" + subtleStyle.Render("↑↓ select  Enter confirm  Esc cancel"))
	return modalStyle.Width(72).Render(content.String())
}

func (m *TuiModel) renderUninstallConfirmModal(pkg *bridge.CatalogEntry) string {
	var content strings.Builder
	content.WriteString(boldStyle.Render("Uninstall ") + errStyle.Render(pkg.ShortName) + boldStyle.Render("?") + "\n\n")
	opts := []string{"Yes, uninstall", "No, keep it"}
	for i, opt := range opts {
		if i == m.promptSel {
			bgStyle := modalConfirmActiveErr
			if i == 1 {
				bgStyle = modalConfirmActiveAccent
			}
			content.WriteString(bgStyle.Render(" "+opt+" "))
		} else {
			content.WriteString(modalOptionInactive.Render(" " + opt + " "))
		}
		content.WriteString("  ")
	}
	content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	return modalStyle.Render(content.String())
}

func (m *TuiModel) renderForceReinstallConfirmModal(pkg *bridge.CatalogEntry) string {
	var content strings.Builder
	content.WriteString(boldStyle.Render("Force re-download ") + errStyle.Render(pkg.ShortName) + boldStyle.Render("?") + "\n")
	content.WriteString(warnStyle.Render("⚠ WARNING: This will delete the local directory and lose all modifications!") + "\n\n")
	opts := []string{"Yes, wipe & reinstall", "No, cancel"}
	for i, opt := range opts {
		if i == m.promptSel {
			bgStyle := modalConfirmActiveErr
			if i == 1 {
				bgStyle = modalConfirmActiveAccent
			}
			content.WriteString(bgStyle.Render(" "+opt+" "))
		} else {
			content.WriteString(modalOptionInactive.Render(" " + opt + " "))
		}
		content.WriteString("  ")
	}
	content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	return modalStyle.Render(content.String())
}

func (m *TuiModel) renderResolveDependencyModal() string {
	var content strings.Builder
	item := m.promptItem
	shortName := "package"
	if item != nil {
		shortName = item.ShortName
	}
	if item != nil && item.Discovered {
		content.WriteString(boldStyle.Render(shortName) + " was discovered as a required dependency\n\n")
	} else {
		content.WriteString(boldStyle.Render(shortName) + " is configured as workspace:*\n\n")
	}
	content.WriteString("How do you want to resolve it?\n\n")

	var options []struct {
		Name  string
		Label string
	}
	if isLocallyAvailable(m.workspaceRoot, shortName) {
		options = append(options, struct{ Name, Label string }{"local", "Use Existing Local Folder"})
	}
	options = append(options,
		struct{ Name, Label string }{"git-ssh", "Git SSH"},
		struct{ Name, Label string }{"git-https", "Git HTTPS"},
		struct{ Name, Label string }{"npm", "NPM Package"},
	)
	for i, opt := range options {
		if i == m.promptSel {
			content.WriteString("  " + modalOptionActive.Render(opt.Label) + "\n")
		} else {
			content.WriteString("  " + modalOptionInactive.Render(opt.Label) + "\n")
		}
	}
	content.WriteString("\n" + subtleStyle.Render("↑↓ select  Enter confirm  Esc cancel"))
	return modalStyle.Width(72).Render(content.String())
}

func (m *TuiModel) renderSetupProgressModal() string {
	var content strings.Builder
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
	spin := spinnerStyle.Render(frame)

	title := "Installing packages…"
	if strings.Contains(strings.ToLower(m.setupLabel), "clean") || strings.Contains(strings.ToLower(m.setupLabel), "reset") || strings.Contains(strings.ToLower(m.setupLabel), "wipe") {
		title = "Resetting workspace…"
	}
	content.WriteString(boldStyle.Render(title) + "\n\n")
	content.WriteString("  " + spin + " " + boldStyle.Render(m.setupLabel) + "\n\n")

	if m.setupTotalSteps > 0 {
		bar := renderProgressBar(m.setupStep, m.setupTotalSteps, 40)
		content.WriteString("  " + bar + "\n")
		content.WriteString(fmt.Sprintf("  Progress %d/%d\n", m.setupStep, m.setupTotalSteps))
		if m.enginePhase != PhaseIdle && m.enginePhase != "" {
			content.WriteString("  " + mutedStyle.Render("Phase: "+string(m.enginePhase)) + "\n")
		}
	} else {
		content.WriteString("  " + renderProgressBar(0, 1, 40) + "\n")
		content.WriteString("  Preparing...\n")
	}
	return modalStyle.Width(72).Render(content.String())
}

func (m *TuiModel) renderManagePackageModal(pkg *bridge.CatalogEntry) string {
	var content strings.Builder
	var leftCols []string
	leftCols = append(leftCols, boldStyle.Render("Package:")+" "+accentStyle.Render(pkg.ShortName))
	leftCols = append(leftCols, mutedStyle.Render("Type:   ")+" "+boldStyle.Render(strings.ToUpper(pkg.Kind)))

	status := mutedStyle.Render("Not Installed")
	if pkg.Installed {
		status = accentStyle.Render("Installed")
		if pkg.LocalSource {
			status += " " + localSourceTag.Render("(workspace)")
		}
	}
	leftCols = append(leftCols, mutedStyle.Render("Status: ")+" "+status)

	src := "NPM"
	if pkg.LocalSource {
		src = "LOC (workspace)"
	} else if pkg.Installed && !pkg.LocalSource && !pkg.InPackageJson {
		src = "GIT"
	}
	leftCols = append(leftCols, mutedStyle.Render("Source: ")+" "+boldStyle.Render(src))

	// Kits
	var kits []string
	for _, dep := range m.promptPkgDeps {
		if strings.HasPrefix(dep, "@owdproject/kit-") {
			shortKit := dep[strings.LastIndex(dep, "/")+1:]
			kits = append(kits, shortKit)
		}
	}
	kitsStr := "none"
	if len(kits) > 0 {
		var styledKits []string
		for _, k := range kits {
			styledKits = append(styledKits, accentStyle.Render(k))
		}
		kitsStr = strings.Join(styledKits, ", ")
	}
	leftCols = append(leftCols, mutedStyle.Render("Kits:   ")+" "+kitsStr)

	gitStatus := "—"
	if pkg.LocalSource {
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
			pkgPath := filepath.Join(m.workspaceRoot, kindDir, pkg.ShortName)
			gitDir := filepath.Join(pkgPath, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				branch := gitBranch(pkgPath)
				changes := gitChanges(pkgPath)
				gitStatus = fmt.Sprintf("branch: %s  (%s)", branch, changes)
			}
		}
	}
	leftCols = append(leftCols, mutedStyle.Render("Git:    ")+" "+gitStatus)

	// NPM & GitHub Links
	npmLink := fmt.Sprintf("https://www.npmjs.com/package/%s", pkg.Name)
	leftCols = append(leftCols, mutedStyle.Render("NPM:    ")+" "+cyanStyle.Render(npmLink))

	ghLink := fmt.Sprintf("https://github.com/owdproject/%s", pkg.ShortName)
	if pkg.HtmlUrl != nil && *pkg.HtmlUrl != "" {
		ghLink = *pkg.HtmlUrl
	}
	leftCols = append(leftCols, mutedStyle.Render("GitHub: ")+" "+cyanStyle.Render(ghLink))

	// Deps (excluding kits)
	var otherDeps []string
	for _, d := range m.promptPkgDeps {
		if !strings.HasPrefix(d, "@owdproject/kit-") {
			s := d
			if idx := strings.LastIndex(d, "/"); idx >= 0 {
				s = d[idx+1:]
			}
			otherDeps = append(otherDeps, s)
		}
	}
	depsStr := "none"
	if len(otherDeps) > 0 {
		depsStr = strings.Join(otherDeps, ", ")
	}
	leftCols = append(leftCols, mutedStyle.Render("Deps:   ")+" "+boldStyle.Render(depsStr))

	leftPanel := strings.Join(leftCols, "\n")

	var rightCols []string
	rightCols = append(rightCols, boldStyle.Render("Management Actions:")+"\n")

	actions := ManagePackageActions

	for i, act := range actions {
		if i == m.promptSel {
			rightCols = append(rightCols, modalOptionActive.Render(fmt.Sprintf(" %s ", act)))
		} else {
			rightCols = append(rightCols, modalOptionInactive.Render(fmt.Sprintf(" %s ", act)))
		}
	}

	rightPanel := strings.Join(rightCols, "\n")

	splitLayout := lipgloss.JoinHorizontal(lipgloss.Top,
		managePackageLeftPanelStyle.Render(leftPanel),
		managePackageRightBorder.Render(rightPanel),
	)

	content.WriteString(splitLayout)
	content.WriteString("\n\n" + subtleStyle.Render("↑↓ select  Enter confirm  Esc close"))

	return modalStyle.Width(110).Render(content.String())
}

func (m *TuiModel) renderSettingsModal() string {
	var content strings.Builder
	content.WriteString(boldStyle.Render("Control Panel Settings") + "\n\n")

	// 1. Install Mode
	modeLabel := "1. Install Mode"
	var modeVal string
	if m.settingsInstallMode == "npm" {
		modeVal = modalOptionActive.Render(" NPM registry ") + "  " + modalOptionInactive.Render(" workspace ")
	} else {
		modeVal = modalOptionInactive.Render(" NPM registry ") + "  " + modalOptionActive.Render(" workspace ")
	}
	if m.settingsSel == 0 {
		content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(modeLabel) + "\n   " + modeVal + "\n")
	} else {
		content.WriteString("  " + boldStyle.Render(modeLabel) + "\n   " + modeVal + "\n")
	}
	content.WriteString("\n")

	// 2. Catalog Sort
	sortLabel := "2. Catalog Sort"
	var sortVal strings.Builder
	sortModes := []string{"updated", "name", "stars", "installed"}
	for _, sm := range sortModes {
		if sm == m.settingsCatalogSort {
			sortVal.WriteString(modalOptionActive.Render(" "+sm+" ") + " ")
		} else {
			sortVal.WriteString(modalOptionInactive.Render(" "+sm+" ") + " ")
		}
	}
	if m.settingsSel == 1 {
		content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(sortLabel) + "\n   " + sortVal.String() + "\n")
	} else {
		content.WriteString("  " + boldStyle.Render(sortLabel) + "\n   " + sortVal.String() + "\n")
	}
	content.WriteString("\n")

	// 3. Trusted Orgs (Authors)
	orgsLabel := "3. Trusted Orgs"
	var orgsValText string
	if m.settingsSel == 2 {
		orgsValText = m.settingsOrgsInput.View()
		content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(orgsLabel) + "\n   " + orgsValText + "\n")
	} else {
		orgsValText = subtleStyle.Render(m.settingsOrgsInput.Value())
		content.WriteString("  " + boldStyle.Render(orgsLabel) + "\n   " + orgsValText + "\n")
	}
	content.WriteString("\n")

	// 4. GitHub User
	ghLabel := "4. GitHub User"
	var ghValText string
	if m.settingsSel == 3 {
		ghValText = m.settingsUserInput.View()
		content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(ghLabel) + "\n   " + ghValText + "\n")
	} else {
		val := m.settingsUserInput.Value()
		if val == "" {
			val = "(not set)"
		}
		ghValText = subtleStyle.Render(val)
		content.WriteString("  " + boldStyle.Render(ghLabel) + "\n   " + ghValText + "\n")
	}
	content.WriteString("\n")

	// 5. Reset Workspace
	resetLabel := "5. Reset Workspace"
	resetVal := modalOptionInactive.Render(" [ WIPE EVERYTHING ] ")
	if m.settingsSel == 4 {
		resetVal = modalConfirmActiveErr.Render(" [ WIPE EVERYTHING ] ")
		content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(resetLabel) + "\n   " + resetVal + "\n")
	} else {
		content.WriteString("  " + boldStyle.Render(resetLabel) + "\n   " + resetVal + "\n")
	}
	content.WriteString("\n\n")

	// Buttons Row: Save and Cancel
	var saveBtn, cancelBtn string
	if m.settingsSel == 5 {
		saveBtn = settingsSaveActive.Render(" SAVE ")
	} else {
		saveBtn = settingsSaveInactive.Render(" SAVE ")
	}

	if m.settingsSel == 6 {
		cancelBtn = settingsCancelActive.Render(" CANCEL ")
	} else {
		cancelBtn = settingsCancelInactive.Render(" CANCEL ")
	}

	content.WriteString("      " + saveBtn + "    " + cancelBtn + "\n\n")
	content.WriteString(subtleStyle.Render("↑↓ select item  ←→ toggle/buttons  Enter confirm  Esc exit"))
	return modalStyle.Width(72).Render(content.String())
}

func (m *TuiModel) renderWipeWorkspaceConfirmModal() string {
	var content strings.Builder
	content.WriteString(boldStyle.Render("Wipe & Reset Workspace?") + "\n")
	content.WriteString(errStyle.Render("⚠ CAUTION: This will delete ALL non-core applications, modules, and themes!") + "\n")
	content.WriteString(subtleStyle.Render("Your configuration files (desktop.config.ts) and desktop package.json will be reset.") + "\n\n")
	opts := []string{"Yes, wipe everything", "No, cancel"}
	for i, opt := range opts {
		if i == m.promptSel {
			bgStyle := modalConfirmActiveErr
			if i == 1 {
				bgStyle = modalConfirmActiveAccent
			}
			content.WriteString(bgStyle.Render(" " + opt + " "))
		} else {
			content.WriteString(modalOptionInactive.Render(" " + opt + " "))
		}
		content.WriteString("  ")
	}
	content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	return modalStyle.Render(content.String())
}
