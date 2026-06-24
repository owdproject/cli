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
	if h < 11 {
		h = 11
	}
	h = h - 1 // Safety margin of 1 line at the bottom to prevent scrolling

	showRightPanel := w >= 120

	// Heights
	topH := 9    // top panels row (with borders = 9 rows)
	barH := 5    // status bar: 3 blank lines + line1 + line2 = 5 rows
	// catalog gets the remaining height
	catalogH := h - topH - barH
	if catalogH < 4 {
		catalogH = 4
	}

	// Column widths (including borders each panel takes +2 cols)
	var leftW, midW, rightW int
	if showRightPanel {
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

	if showRightPanel {
		catW := leftW + midW
		var rightPanel string
		logContent := m.renderLogsPanel(rightW-4, h-barH-2)
		baseTitle := "Logs"
		if m.wizard != nil && !m.wizard.IsComplete() {
			baseTitle = "Wizard Setup"
		} else if m.activeTask == TaskSetup || m.activeTask == TaskWipe {
			baseTitle = "Setup"
		}
		baseStyle := lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
		styledBase := baseStyle.Render(baseTitle)
		clearBtn := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("[Clear]")
		rightPanelTitle := styledBase + "  " + clearBtn

		rightPanel = drawPanel(rightW, h-barH, rightPanelTitle, logContent, false)

		// Top 2-panel row (left + mid)
		topTwoPanels := lipgloss.JoinHorizontal(lipgloss.Top,
			drawPanel(leftW-1, topH, desktopTitle, leftContent, false),
			drawPanel(midW, topH, "Metrics", midContent, false),
		)

		// Catalog panel — always rendered; modal overlaid on top if active
		catalogContent := m.renderCatalogPanel(catW-4, catalogH-2, showRightPanel)
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

		topRow = lipgloss.JoinHorizontal(lipgloss.Top, leftMain, rightPanel)
		mainContent = topRow
	} else {
		// No logs: top row = left + right panels side by side
		topBlock := lipgloss.JoinHorizontal(lipgloss.Top,
			drawPanel(leftW-1, topH, desktopTitle, leftContent, false),
			drawPanel(midW, topH, "Metrics", midContent, false),
		)

		// Catalog full width — always rendered; modal overlaid on top if active
		catalogContent := m.renderCatalogPanel(w-4, catalogH-2, showRightPanel)
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

	out := mainContent + statusBar

	// Hard clamp: ensure the final output never exceeds termHeight lines.
	// This is the last line of defence against terminal scrolling.
	outLines := strings.Split(out, "\n")
	if len(outLines) > m.termHeight {
		out = strings.Join(outLines[:m.termHeight], "\n")
	}

	return out
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
	lines = append(lines, mutedStyle.Render("  Git Branch ")+boldStyle.Render(m.workspaceBranch))

	// Workspace Changes
	lines = append(lines, mutedStyle.Render("  Changes    ")+m.workspaceChanges)

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
	visibleRows := h - 6 // height minus tabs, spaces, detail pane separator and details row
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
	colNameW := w * 28 / 100
	if colNameW < 12 {
		colNameW = 12
	}
	// Row prefix: "  "(2) + badge(1) + "   "(3) = 6 chars before the name text.
	// Header must use the same 6-char prefix, NAME label shrunk by 4 to compensate.
	header := "      " +
		padRight("NAME", colNameW-4) + " " +
		padRight("VERSION", 9) + " " +
		padRight("SRC", 5) + " " +
		padRight("SYNC", 14) + " " +
		padRight("PUBLISHER", 12) + " " +
		padRight("★", 6) + " " +
		"AGE"
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
	for len(lines) < h-2 {
		lines = append(lines, "")
	}

	// Details panel
	lines = append(lines, headerStyle.Render("  "+strings.Repeat("─", w-4)))
	lines = append(lines, m.renderDetailRow(items))

	return strings.Join(lines, "\n")
}

func (m *TuiModel) renderCatalogRow(item bridge.CatalogEntry, selected bool, w, nameW int) string {
	// ── Badge (checkbox / radiobox) ─────────────────────────────
	var badge string
	if item.Kind == "theme" {
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
	name := badge + "   " + shortName

	// ── VERSION ─────────────────────────────────────────────────
	version := "—"
	if item.Version != nil {
		version = *item.Version
	}

	// ── SRC (npm / git / dev) ────────────────────────────────────
	source := "npm"
	if item.LocalSource {
		if m.localGitDirs[item.ShortName] {
			source = "git"
		} else {
			source = "dev"
		}
	}

	// ── SYNC column: git changes + update indicators ─────────────
	var syncParts []string
	if gitStat, ok := m.gitChangesMap[item.ShortName]; ok {
		if gitStat.Added > 0 {
			syncParts = append(syncParts, accentStyle.Render(fmt.Sprintf("+%d", gitStat.Added)))
		}
		if gitStat.Modified > 0 {
			syncParts = append(syncParts, warnStyle.Render(fmt.Sprintf("~%d", gitStat.Modified)))
		}
		if gitStat.Deleted > 0 {
			syncParts = append(syncParts, errStyle.Render(fmt.Sprintf("-%d", gitStat.Deleted)))
		}
	}
	if upInfo, ok := m.updatesMap[item.ShortName]; ok {
		if upInfo.LocalGit && upInfo.BehindCount > 0 {
			syncParts = append(syncParts, cyanStyle.Render(fmt.Sprintf("↓%d", upInfo.BehindCount)))
		} else if upInfo.Npm {
			syncParts = append(syncParts, accentStyle.Render("↑"))
		}
	}
	sync := subtleStyle.Render("—")
	if len(syncParts) > 0 {
		sync = strings.Join(syncParts, " ")
	}

	// ── PUBLISHER ────────────────────────────────────────────────
	pub := item.Org
	if pub == "" || pub == "workspace" {
		pub = "owdproject"
	}
	publisher := truncate(pub, 12)

	// ── STARS ────────────────────────────────────────────────────
	stars := subtleStyle.Render("—")
	if item.Stars > 0 {
		stars = warnStyle.Render(fmt.Sprintf("★ %d", item.Stars))
	}

	// ── AGE (last push) ──────────────────────────────────────────
	age := subtleStyle.Render("—")
	if item.PushedAt != nil {
		age = mutedStyle.Render(formatCatalogAge(item.PushedAt))
	} else if item.UpdatedAt != nil {
		age = mutedStyle.Render(formatCatalogAge(item.UpdatedAt))
	}

	// ── Assemble row ─────────────────────────────────────────────
	var nameCol string
	if selected {
		nameCol = selectedRowStyle.Render(padRight(name, nameW))
	} else {
		nameCol = padRight(name, nameW)
	}

	row := "  " + nameCol + " " +
		padRight(version, 9) + " " +
		padRight(source, 5) + " " +
		padRight(sync, 14) + " " +
		padRight(publisher, 12) + " " +
		padRight(stars, 6) + " " +
		age

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

	// Wizard progress info — shown during active wizard
	if m.wizard != nil && !m.wizard.IsComplete() {
		curr := m.wizard.Current()
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		spin := cyanBoldStyle.Render(frame)

		var stepMsg string
		if curr.Action == "uninstall" {
			stepMsg = fmt.Sprintf("Awaiting decision: Uninstall %s", curr.ShortName)
		} else if curr.Action == "install" {
			stepMsg = fmt.Sprintf("Awaiting decision: Install %s", curr.ShortName)
		} else {
			stepMsg = fmt.Sprintf("Awaiting decision: %s %s", curr.Action, curr.ShortName)
		}

		lines = append(lines, "  "+spin+" "+boldStyle.Render(stepMsg))
		lines = append(lines, "  "+mutedStyle.Render(fmt.Sprintf("Step %d of %d in wizard queue", m.wizard.Index+1, len(m.wizard.Queue))))
		lines = append(lines, "")
	}

	// Setup progress header — shown during active setup/wipe tasks
	if (m.activeTask == TaskSetup || m.activeTask == TaskWipe) && m.setupTotalSteps > 0 {
		// Progress bar
		barWidth := w - 6
		if barWidth < 4 {
			barWidth = 4
		}
		filled := 0
		if m.setupTotalSteps > 0 {
			filled = (m.setupStep * barWidth) / m.setupTotalSteps
		}
		if filled > barWidth {
			filled = barWidth
		}
		progBar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
		progPct := 0
		if m.setupTotalSteps > 0 {
			progPct = (m.setupStep * 100) / m.setupTotalSteps
		}
		lines = append(lines, "  "+accentStyle.Render(fmt.Sprintf("%s %d%%", progBar, progPct)))
		lines = append(lines, "  "+mutedStyle.Render(m.setupLabel))
		lines = append(lines, "")
	}

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
		case strings.Contains(line, "ERROR") || strings.Contains(line, "Error") || strings.Contains(line, "failed") || strings.HasPrefix(line, "✗"):
			lines = append(lines, errStyle.Render("  "+line))
		case strings.HasPrefix(line, "⚙"):
			lines = append(lines, cyanRegularStyle.Render("  "+line))
		case strings.HasPrefix(line, "ℹ"):
			lines = append(lines, accentRegularStyle.Render("  "+line))
		case strings.HasPrefix(line, "✓") || strings.Contains(line, "ready") || strings.Contains(line, "started"):
			lines = append(lines, accentRegularStyle.Render("  "+line))
		default:
			lines = append(lines, mutedStyle.Render("  "+line))
		}
	}

	if len(m.logLines) == 0 {
		if m.serverRunning && m.isServerCmdNil() {
			lines = append(lines, mutedStyle.Render("  Logs unavailable (external server)"))
		} else if m.activeTask != TaskNone {
			// Spinner while waiting for first log line during an active task
			spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spin := spinners[m.tickCount%len(spinners)]
			lines = append(lines, mutedStyle.Render("  "+spin+" Running…"))
		} else if m.serverRunning {
			lines = append(lines, mutedStyle.Render("  Waiting for logs…"))
		}
	}

	return strings.Join(lines, "\n")
}

func (m *TuiModel) renderInitializingPanel(w, h int) string {
	var lines []string

	lines = append(lines, "")

	// Vertical spacing to center content roughly
	paddingTop := (h - 6) / 2
	if paddingTop < 1 {
		paddingTop = 1
	}
	for i := 0; i < paddingTop; i++ {
		lines = append(lines, "")
	}

	// Spinner frames (braille pattern makes a beautiful spinner)
	spinnerFrames := []string{
		"⢹", "⢺", "⢽", "⢾", "⡿", "⢿", "⣻", "⣽", "⣾", "⣿",
		"⣇", "⣆", "⣃", "⣅", "⣄", "⣠", "⣡", "⣢", "⣣", "⣤",
	}
	frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
	spin := spinnerStyle.Render(frame)

	lines = append(lines, "  "+spin+"  "+accentStyle.Render("Initializing OWD…"))
	lines = append(lines, "")

	taskLabel := m.statusMsg
	if m.setupLabel != "" {
		taskLabel = m.setupLabel
	}
	// Trim/truncate label to fit
	taskLabel = truncate(taskLabel, w-6)
	lines = append(lines, "     "+boldStyle.Render(taskLabel))

	if m.setupTotalSteps > 0 {
		lines = append(lines, "")
		bar := renderProgressBar(m.setupStep, m.setupTotalSteps, w-10)
		lines = append(lines, "     "+bar)
		lines = append(lines, fmt.Sprintf("     Step %d of %d", m.setupStep, m.setupTotalSteps))
	}

	// Pad the rest of the lines
	for len(lines) < h {
		lines = append(lines, "")
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

	var serverShortcut string
	if m.serverRunning {
		serverShortcut = barKeyStyle.Render("x") + barTextStyle.Render(" stop server")
	} else {
		serverShortcut = barKeyStyle.Render("d") + barTextStyle.Render(" start server")
	}

	line1Parts := []string{
		statusIcon + barKeyStyle.Render("Select packages") + barTextStyle.Render(" · "),
	}
	if m.hasPendingChanges() {
		line1Parts = append(line1Parts, barKeyStyle.Render("s")+barTextStyle.Render(" save changes · "))
	}
	line1Parts = append(line1Parts,
		serverShortcut+barTextStyle.Render(" · "),
		barKeyStyle.Render("g")+barTextStyle.Render(" settings"),
	)

	if m.activeTask != TaskNone {
		spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := cyanRegularStyle.Render(spinFrames[m.tickCount%len(spinFrames)])
		line1Parts = []string{spin + " " + barTextStyle.Render(m.statusMsg)}
	}

	_ = mode

	// Render line1 and line2 WITHOUT Width() to prevent wrapping.
	// Then truncate to exactly w columns using lipgloss-aware truncation.
	line1Raw := barStyle.PaddingLeft(2).Render(strings.Join(line1Parts, ""))
	if lipgloss.Width(line1Raw) > w {
		line1Raw = truncate(line1Raw, w)
	}

	// Line 2: shortcuts
	sep := barSepStyle.Render(" │ ")
	var shortcutParts []string
	shortcutParts = append(shortcutParts,
		barKeyStyle.Render("↑↓")+barTextStyle.Render(" move"),
		barKeyStyle.Render("Space")+barTextStyle.Render(" toggle"),
		barKeyStyle.Render("c")+barTextStyle.Render(" manage"),
	)

	items := m.getActiveItems()
	hasHoveredInstalled := false
	if len(items) > 0 && m.selectedIndex < len(items) {
		hasHoveredInstalled = items[m.selectedIndex].Installed
	}
	if hasHoveredInstalled {
		shortcutParts = append(shortcutParts, barKeyStyle.Render("u")+barTextStyle.Render(" update"))
	}

	shortcutParts = append(shortcutParts,
		barKeyStyle.Render("r")+barTextStyle.Render(" refresh"),
		barKeyStyle.Render("n")+barTextStyle.Render(" new"),
		barKeyStyle.Render("q")+barTextStyle.Render(" quit"),
	)
	line2Raw := barStyle.PaddingLeft(2).Render(strings.Join(shortcutParts, sep))
	if lipgloss.Width(line2Raw) > w {
		line2Raw = truncate(line2Raw, w)
	}

	// Always return exactly 5 lines: 3 blank + line1 + line2
	return "\n\n\n" + line1Raw + "\n" + line2Raw
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
