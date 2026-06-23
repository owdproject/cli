package tui

import (
	"fmt"
	"strings"

	"owd-cli/bridge"

	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// View structural layout
// ─────────────────────────────────────────────

func (m *TuiModel) View() string {
	w := m.termWidth
	h := m.termHeight
	if w < 40 {
		w = 40
	}
	if h < 10 {
		h = 10
	}

	showLogs := (m.serverRunning || m.activeTask != TaskNone) && w >= 120

	// Heights
	topH := 9    // top panels row (with borders = 9 rows)
	barH := 4    // status bar lines (including prepended/trailing newlines)
	// catalog gets the remaining height
	catalogH := h - topH - barH
	if catalogH < 4 {
		catalogH = 4
	}

	// Column widths (including borders each panel takes +2 cols)
	var leftW, midW, rightW int
	if showLogs {
		// 40% | 28% | 32%  — roughly matching original blessed layout
		rightW = w * 32 / 100
		leftW  = w * 40 / 100
		midW   = w - leftW - rightW
	} else {
		leftW = w * 58 / 100
		midW  = w - leftW
		rightW = 0
	}

	// ── Render panel contents ─────────────────────────────
	leftContent := m.renderClientPanel(leftW-4, topH-2)
	midContent  := m.renderMetricsPanel(midW-4, topH-2)

	desktopTitle := accentStyle.Render("Desktop") + " " +
		desktopSubTitleStyle.Render("control panel")

	var topRow, mainContent, catalogPanel string

	if showLogs {
		catW := leftW + midW
		logContent := m.renderLogsPanel(rightW-4, h-barH-2)

		// Top 2-panel row (left + mid)
		topTwoPanels := lipgloss.JoinHorizontal(lipgloss.Top,
			drawPanel(leftW-1, topH, desktopTitle, leftContent, false),
			drawPanel(midW, topH, "Metrics", midContent, false),
		)

		// Catalog panel — always rendered; modal overlaid on top if active
		catalogContent := m.renderCatalogPanel(catW-4, catalogH-2, showLogs)
		catalogPanel = drawPanel(catW-1, catalogH, "Catalog", catalogContent, true)
		promptToShow := m.activePrompt
		if (m.activeTask == TaskSetup || m.activeTask == TaskWipe) && (promptToShow == PromptNone || promptToShow == PromptUninstallConfirm || promptToShow == PromptInstallMethod || promptToShow == PromptWipeWorkspaceConfirm) {
			promptToShow = PromptSetupProgress
		}
		if promptToShow != PromptNone {
			modal := m.renderModal(promptToShow)
			styledModal := modalWrapperStyle.Render(modal)
			catalogPanel = overlayCenter(catalogPanel, styledModal)
		}

		// Left main block = top panels + catalog stacked
		leftMain := lipgloss.JoinVertical(lipgloss.Left, topTwoPanels, catalogPanel)

		// Right logs panel, spans full height
		logsPanel := drawPanel(rightW, h-barH, "Logs", logContent, false)

		topRow = lipgloss.JoinHorizontal(lipgloss.Top, leftMain, logsPanel)
		mainContent = topRow
	} else {
		// No logs: top row = left + right panels side by side
		topBlock := lipgloss.JoinHorizontal(lipgloss.Top,
			drawPanel(leftW-1, topH, desktopTitle, leftContent, false),
			drawPanel(midW, topH, "Metrics", midContent, false),
		)

		// Catalog full width — always rendered; modal overlaid on top if active
		catalogContent := m.renderCatalogPanel(w-4, catalogH-2, showLogs)
		catalogPanel = drawPanel(w, catalogH, "Catalog", catalogContent, true)
		promptToShow := m.activePrompt
		if (m.activeTask == TaskSetup || m.activeTask == TaskWipe) && (promptToShow == PromptNone || promptToShow == PromptUninstallConfirm || promptToShow == PromptInstallMethod || promptToShow == PromptWipeWorkspaceConfirm) {
			promptToShow = PromptSetupProgress
		}
		if promptToShow != PromptNone {
			modal := m.renderModal(promptToShow)
			styledModal := modalWrapperStyle.Render(modal)
			catalogPanel = overlayCenter(catalogPanel, styledModal)
		}

		mainContent = lipgloss.JoinVertical(lipgloss.Left, topBlock, catalogPanel)
	}

	// ── Status bar (borderless) ───────────────────────────
	statusBar := m.renderStatusBar(w)

	return mainContent + statusBar
}

// ─────────────────────────────────────────────
// Panel: Desktop Control Panel (left)
// ─────────────────────────────────────────────

func (m *TuiModel) renderClientPanel(w, h int) string {
	var lines []string

	// Server status line
	var statusLine string
	if m.activeTask == TaskServe && strings.Contains(strings.ToLower(m.statusMsg), "stopping") {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		dot := warnBoldStyle.Render(frame)
		statusLine = dot + " " + boldStyle.Render("STOPPING") + "  " + mutedStyle.Render("http://localhost:3000")
	} else if m.activeTask == TaskServe && strings.Contains(strings.ToLower(m.statusMsg), "starting") {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		dot := cyanBoldStyle.Render(frame)
		statusLine = dot + " " + boldStyle.Render("STARTING") + "  " + mutedStyle.Render("http://localhost:3000")
	} else if m.serverRunning {
		dot := accentStyle.Render("●")
		if m.blink {
			dot = blinkGreenStyle.Render("●")
		}
		url := cyanRegularStyle.Render("http://localhost:3000")
		statusLine = dot + " " + boldStyle.Render("RUNNING") + "   " + url
		if m.ctx != nil {
			_ = m.ctx
		}
	} else if m.activeTask != TaskNone {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		dot := warnBoldStyle.Render(frame)
		statusLine = dot + " " + boldStyle.Render("STARTING") + "  " + mutedStyle.Render("http://localhost:3000")
	} else {
		dot := errStyle.Render("○")
		statusLine = dot + " " + boldStyle.Render("STOPPED") + "   " + mutedStyle.Render("http://localhost:3000")
	}
	lines = append(lines, "  "+statusLine)
	lines = append(lines, "") // Space out server status line
	lines = append(lines, mutedStyle.Render("  System     ")+boldStyle.Render("ready"))

	// Theme
	theme := "—"
	if m.ctx != nil && m.ctx.Config.Theme != nil && *m.ctx.Config.Theme != "" {
		t := *m.ctx.Config.Theme
		if idx := strings.LastIndex(t, "/"); idx >= 0 {
			t = t[idx+1:]
		}
		theme = t
	}
	if m.pendingTheme != nil {
		t := *m.pendingTheme
		if idx := strings.LastIndex(t, "/"); idx >= 0 {
			t = t[idx+1:]
		}
		theme = t + warnStyle.Render(" (pending)")
	}
	lines = append(lines, mutedStyle.Render("  Theme      ")+boldStyle.Render(theme))

	// Source
	source := "—"
	if m.ctx != nil {
		if m.ctx.Paths.IsPlayground {
			source = "playground"
		} else {
			source = "monorepo"
		}
	}
	lines = append(lines, mutedStyle.Render("  Source     ")+boldStyle.Render(source))

	// Git Branch
	branch := gitBranch(m.workspaceRoot)
	lines = append(lines, mutedStyle.Render("  Git Branch ")+boldStyle.Render(branch))

	// Workspace Changes
	changes := gitChanges(m.workspaceRoot)
	lines = append(lines, mutedStyle.Render("  Changes    ")+changes)

	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────
// Panel: Metrics (right of control panel)
// ─────────────────────────────────────────────

func (m *TuiModel) renderMetricsPanel(w, h int) string {
	var lines []string

	// Memory sparkline
	spark := sparkline(m.memHistory)
	memVal := "—"
	if len(m.memHistory) > 0 && m.memHistory[len(m.memHistory)-1] > 0 {
		memVal = fmt.Sprintf("%d MiB", m.memHistory[len(m.memHistory)-1])
	}

	lines = append(lines, kv("Memory", memVal)+" "+spark)

	// Local counts
	appCount := countLocalDirs(m.workspaceRoot, "apps")
	pkgCount := countLocalDirs(m.workspaceRoot, "packages", "core", "cli", "nx")
	themeCount := countLocalDirs(m.workspaceRoot, "themes")

	lines = append(lines, kv("Local Apps", fmt.Sprintf("%d", appCount)))
	lines = append(lines, kv("Packages", fmt.Sprintf("%d", pkgCount)))
	lines = append(lines, kv("Themes", fmt.Sprintf("%d", themeCount)))

	// Nuxt & Core Versions
	nuxtRoot := m.workspaceRoot
	if m.ctx != nil && m.ctx.Paths.IsPlayground {
		nuxtRoot = m.ctx.Paths.PackageDir
	}
	vers := getVersions(nuxtRoot, m.workspaceRoot)
	lines = append(lines, kv("Nuxt", vers.Nuxt)+mutedStyle.Render(" · ")+kv("Core", vers.Owd))

	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────
// Panel: Catalog
// ─────────────────────────────────────────────

func (m *TuiModel) renderCatalogPanel(w, h int, _ bool) string {
	var lines []string

	// Tabs row
	var tabLine strings.Builder
	tabLine.WriteString(" ")
	if m.activeTab == 0 {
		tabLine.WriteString(tabActiveStyle.Render("Internal"))
	} else {
		tabLine.WriteString(tabInactiveStyle.Render("Internal"))
	}
	tabLine.WriteString(" ")
	if m.activeTab == 1 {
		tabLine.WriteString(tabActiveStyle.Render("Apps"))
	} else {
		tabLine.WriteString(tabInactiveStyle.Render("Apps"))
	}
	tabLine.WriteString(" ")
	if m.activeTab == 2 {
		tabLine.WriteString(tabActiveStyle.Render("Modules"))
	} else {
		tabLine.WriteString(tabInactiveStyle.Render("Modules"))
	}
	tabLine.WriteString(" ")
	if m.activeTab == 3 {
		tabLine.WriteString(tabActiveStyle.Render("Themes"))
	} else {
		tabLine.WriteString(tabInactiveStyle.Render("Themes"))
	}
	lines = append(lines, tabLine.String())
	lines = append(lines, "")

	items := m.getActiveItems()

	if len(items) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("No packages available in this tab"))
		return strings.Join(lines, "\n")
	}

	// Calculate pagination heights
	visibleRows := h - 5 // height minus tabs, spaces, detail pane
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Apply scrolling offsets
	if m.selectedIndex < m.scrollOffset {
		m.scrollOffset = m.selectedIndex
	} else if m.selectedIndex >= m.scrollOffset+visibleRows {
		m.scrollOffset = m.selectedIndex - visibleRows + 1
	}

	// Table header
	headerStyle := tableHeaderStyle
	colNameW := w * 32 / 100
	if colNameW < 12 {
		colNameW = 12
	}
	header := "   " +
		padRight("NAME", colNameW) + " " +
		padRight("VERSION", 11) + " " +
		padRight("SOURCE", 10) + " " +
		padRight("STATUS", 10) + " " +
		padRight("DESCRIPTION", w-colNameW-38)
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, headerStyle.Render("  "+strings.Repeat("─", w-4)))

	// Render page rows
	for i := 0; i < visibleRows; i++ {
		idx := m.scrollOffset + i
		if idx >= len(items) {
			break
		}
		item := items[idx]
		row := m.renderCatalogRow(item, idx == m.selectedIndex, w, colNameW)
		lines = append(lines, row)
	}

	// Pad empty space to keep layout stable
	for len(lines) < h-3 {
		lines = append(lines, "")
	}

	// Details panel
	lines = append(lines, headerStyle.Render("  "+strings.Repeat("─", w-4)))
	lines = append(lines, m.renderDetailRow(items))

	return strings.Join(lines, "\n")
}

func (m *TuiModel) renderCatalogRow(item bridge.CatalogEntry, selected bool, w, nameW int) string {
	var badge string
	if item.Kind == "theme" {
		// Radiobox
		active := false
		if m.ctx != nil && m.ctx.Config.Theme != nil && *m.ctx.Config.Theme == item.Name {
			active = true
		}
		if m.pendingTheme != nil && *m.pendingTheme == item.Name {
			active = true
		}

		if active {
			badge = accentStyle.Render("◉")
		} else {
			badge = subtleStyle.Render("○")
		}
	} else {
		// Checkbox
		active := item.Installed
		if val, exists := m.pendingPackages[item.Name]; exists {
			active = val
		}

		if active {
			badge = accentStyle.Render("☒")
		} else {
			badge = subtleStyle.Render("☐")
		}
	}

	shortName := item.ShortName
	name := badge + " " + shortName

	version := "—"
	if item.Version != nil {
		version = *item.Version
	}

	source := "npm"
	if item.LocalSource {
		if m.ctx != nil && m.ctx.Settings.LastInstallChoices != nil {
			if choice, exists := m.ctx.Settings.LastInstallChoices[item.Name]; exists {
				if choiceMap, ok := choice.(map[string]interface{}); ok {
					if _, hasGit := choiceMap["gitUrl"].(string); hasGit {
						source = "git"
					} else {
						source = "dev"
					}
				} else {
					source = "dev"
				}
			} else {
				source = "dev"
			}
		} else {
			source = "dev"
		}
	}

	var status string
	pendingVal, hasPending := m.pendingPackages[item.Name]
	pendingThemeVal := m.pendingTheme

	if item.Installed {
		if hasPending && !pendingVal {
			status = warnStyle.Render("remove*")
		} else {
			status = accentStyle.Render("installed")
		}
	} else {
		if hasPending && pendingVal {
			status = cyanStyle.Render("install*")
		} else if item.Kind == "theme" && pendingThemeVal != nil && *pendingThemeVal == item.Name {
			status = cyanStyle.Render("active*")
		} else {
			status = subtleStyle.Render("—")
		}
	}

	// Git behind indicator
	if upInfo, ok := m.updatesMap[item.Name]; ok {
		if upInfo.LocalGit && upInfo.BehindCount > 0 {
			status += warnStyle.Render(fmt.Sprintf(" (-%d)", upInfo.BehindCount))
		} else if upInfo.Npm {
			status += warnStyle.Render(" (upd)")
		}
	}

	// Local edits indicator (Git dirty status)
	if gitStat, ok := m.gitChangesMap[item.Name]; ok {
		total := gitStat.Added + gitStat.Modified + gitStat.Deleted
		if total > 0 {
			status += dirtyIndicatorStyle.Render("*")
		}
	}

	desc := item.Description

	var nameCol string
	if selected {
		nameCol = selectedRowStyle.Render(padRight(name, nameW))
	} else {
		nameCol = padRight(name, nameW)
	}

	row := " " + nameCol + " " +
		padRight(version, 11) + " " +
		padRight(source, 10) + " " +
		padRight(status, 18) + " " +
		padRight(desc, w-nameW-46)

	if selected {
		return selectedRowBgStyle.Render(row)
	}
	return row
}

func (m *TuiModel) renderDetailRow(items []bridge.CatalogEntry) string {
	if len(items) == 0 || m.selectedIndex >= len(items) {
		return "  " + mutedStyle.Render("Select a package for install preview")
	}

	item := items[m.selectedIndex]

	org := "—"
	if item.Org != "" {
		org = item.Org
	}
	stars := "—"
	if item.Stars > 0 {
		stars = fmt.Sprintf("★ %d", item.Stars)
	}
	desc := "No description provided."
	if item.Description != "" {
		desc = item.Description
	}

	var status string
	if item.Installed {
		status = accentStyle.Render("Installed")
		if item.LocalSource {
			status += " " + subtleStyle.Render("(Local symlink)")
		} else {
			status += " " + subtleStyle.Render("(NPM dependency)")
		}
	} else {
		status = subtleStyle.Render("Not installed")
	}

	kind := strings.Title(item.Kind)

	parts := []string{
		accentStyle.Render(item.ShortName),
		"  " + mutedStyle.Render("Publisher:") + " " + boldStyle.Render(org),
		"  " + mutedStyle.Render("Stars:") + " " + boldStyle.Render(stars),
		"  " + mutedStyle.Render("Status:") + " " + status,
		"  " + mutedStyle.Render("Type:") + " " + boldStyle.Render(kind),
		"  " + mutedStyle.Render("—") + " " + mutedStyle.Render(desc),
	}

	return "  " + strings.Join(parts, " ")
}

// ─────────────────────────────────────────────
// Panel: Logs (right column, when server running)
// ─────────────────────────────────────────────

func (m *TuiModel) isServerCmdNil() bool {
	m.runtime.serverMu.Lock()
	defer m.runtime.serverMu.Unlock()
	return m.runtime.serverCmd == nil
}

func (m *TuiModel) renderLogsPanel(w, h int) string {
	var lines []string

	lines = append(lines, "")

	// Server URLs
	if m.serverRunning {
		dot := accentStyle.Render("●")
		if !m.blink {
			dot = statusGreenStyle.Render("●")
		}

		lines = append(lines, "  "+dot+" "+accentStyle.Render("Dev server started"))
		lines = append(lines, "")
		lines = append(lines, "  "+accentStyle.Render("✓")+" "+mutedStyle.Render("Listening on http://localhost:3000"))
		lines = append(lines, "")
	}

	// Log lines — show last N that fit
	logH := h - len(lines) - 1
	if logH < 1 {
		logH = 1
	}
	logStart := len(m.logLines) - logH
	if logStart < 0 {
		logStart = 0
	}
	for i := logStart; i < len(m.logLines); i++ {
		line := m.logLines[i]
		line = truncate(line, w)
		// Colorize log levels
		switch {
		case strings.Contains(line, "WARN") || strings.Contains(line, "Warning"):
			lines = append(lines, warnStyle.Render("  "+line))
		case strings.Contains(line, "ERROR") || strings.Contains(line, "Error") || strings.Contains(line, "failed"):
			lines = append(lines, errStyle.Render("  "+line))
		case strings.HasPrefix(line, ">>>"):
			lines = append(lines, accentStyle.Render("  "+line))
		case strings.Contains(line, "✓") || strings.Contains(line, "ready") || strings.Contains(line, "started"):
			lines = append(lines, accentRegularStyle.Render("  "+line))
		default:
			lines = append(lines, mutedStyle.Render("  "+line))
		}
	}

	if len(m.logLines) == 0 {
		if m.serverRunning && m.isServerCmdNil() {
			lines = append(lines, mutedStyle.Render("  Logs unavailable (external server)"))
		} else {
			lines = append(lines, mutedStyle.Render("  Waiting for logs…"))
		}
	}

	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────
// Status Bar (borderless, Gemini-style)
// ─────────────────────────────────────────────

func (m *TuiModel) renderStatusBar(w int) string {
	mode := ""
	switch m.activeTab {
	case 0:
		mode = "Apps"
	case 1:
		mode = "Modules"
	case 2:
		mode = "Themes"
	}

	statusIcon := ""
	if m.serverRunning {
		if m.blink {
			statusIcon = accentRegularStyle.Render("● ")
		} else {
			statusIcon = statusGreenStyle.Render("● ")
		}
	}

	line1Parts := []string{
		statusIcon + barKeyStyle.Render("Select packages") + barStyle.Render(" · "),
	}
	if m.hasPendingChanges() {
		line1Parts = append(line1Parts, barKeyStyle.Render("s") + barStyle.Render(" save changes · "))
	}
	line1Parts = append(line1Parts,
		barKeyStyle.Render("g") + barStyle.Render(" settings"),
	)

	var serverShortcut string
	if m.serverRunning {
		serverShortcut = barKeyStyle.Render("x") + barStyle.Render(" stop server")
	} else {
		serverShortcut = barKeyStyle.Render("d") + barStyle.Render(" start server")
	}

	if m.activeTask != TaskNone {
		spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := cyanRegularStyle.Render(spinFrames[m.tickCount%len(spinFrames)])
		line1Parts = []string{spin + " " + barStyle.Render(m.statusMsg)}
	}

	_ = mode
	line1 := barStyle.Width(w).Render(strings.Join(line1Parts, ""))

	// Line 2: shortcuts
	sep := barSepStyle.Render(" │ ")
	var shortcutParts []string
	shortcutParts = append(shortcutParts,
		barKeyStyle.Render("↑↓") + barStyle.Render(" move"),
		barKeyStyle.Render("Space") + barStyle.Render(" toggle"),
		barKeyStyle.Render("c") + barStyle.Render(" manage"),
	)

	items := m.getActiveItems()
	hasHoveredInstalled := false
	if len(items) > 0 && m.selectedIndex < len(items) {
		hasHoveredInstalled = items[m.selectedIndex].Installed
	}
	if hasHoveredInstalled {
		shortcutParts = append(shortcutParts, barKeyStyle.Render("u") + barStyle.Render(" update"))
	}

	shortcutParts = append(shortcutParts,
		serverShortcut,
		barKeyStyle.Render("r") + barStyle.Render(" refresh"),
		barKeyStyle.Render("n") + barStyle.Render(" new"),
		barKeyStyle.Render("q") + barStyle.Render(" quit"),
	)
	line2 := barStyle.Width(w).Render(strings.Join(shortcutParts, sep))

	return "\n\n" + line1 + "\n" + line2 + "\n"
}

func (m *TuiModel) getActiveItems() []bridge.CatalogEntry {
	if m.catalog == nil {
		return nil
	}
	var res []bridge.CatalogEntry
	for _, e := range m.catalog.Entries {
		switch m.activeTab {
		case 0: // Internal
			if e.Kind == "module" && !strings.HasPrefix(e.ShortName, "module-") {
				res = append(res, e)
			}
		case 1: // Apps
			if e.Kind == "app" {
				res = append(res, e)
			}
		case 2: // Modules
			if e.Kind == "module" && strings.HasPrefix(e.ShortName, "module-") {
				res = append(res, e)
			}
		case 3: // Themes
			if e.Kind == "theme" {
				res = append(res, e)
			}
		}
	}
	return res
}
