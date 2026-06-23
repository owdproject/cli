package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────
// Colors & Styles
// ─────────────────────────────────────────────

var (
	colorAccent    = lipgloss.Color("#7ee787")
	colorCyan      = lipgloss.Color("#24ebff")
	colorDim       = lipgloss.Color("#3a3a4a")
	colorMuted     = lipgloss.Color("#6c7086")
	colorWarn      = lipgloss.Color("#f9e2af")
	colorErr       = lipgloss.Color("#f38ba8")
	colorNpm       = lipgloss.Color("#7ee787")
	colorGit       = lipgloss.Color("#89dceb")
	colorLocal     = lipgloss.Color("#ff79c6")
	colorBorder    = lipgloss.Color("#4a4a6a")
	colorBorderAct = lipgloss.Color("#7ee787")
	colorWhite     = lipgloss.Color("#cdd6f4")
	colorSubtle    = lipgloss.Color("#585b70")
	colorBarBg     = lipgloss.Color("#11111b")
	colorBarFg     = lipgloss.Color("#cdd6f4")
)

var (
	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	panelBorderActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderAct)

	accentStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	cyanStyle   = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	mutedStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	warnStyle   = lipgloss.NewStyle().Foreground(colorWarn)
	errStyle    = lipgloss.NewStyle().Foreground(colorErr)
	boldStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	dimStyle    = lipgloss.NewStyle().Foreground(colorDim)
	subtleStyle = lipgloss.NewStyle().Foreground(colorSubtle)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Background(colorAccent).
			Foreground(lipgloss.Color("#1e1e2e")).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Padding(0, 2)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	installedBadge   = lipgloss.NewStyle().Foreground(colorAccent).Render("●")
	uninstalledBadge = lipgloss.NewStyle().Foreground(colorSubtle).Render("○")

	// bottom bar — no border, gemini-style
	barStyle = lipgloss.NewStyle().
			Foreground(colorBarFg).
			PaddingLeft(2)

	barTextStyle = lipgloss.NewStyle().
			Foreground(colorBarFg)

	barKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	barSepStyle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	modalStyle = lipgloss.NewStyle().
			Padding(1, 3)

	modalOptionActive = lipgloss.NewStyle().
				Background(colorCyan).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 2)

	modalOptionInactive = lipgloss.NewStyle().
				Background(colorDim).
				Foreground(colorMuted).
				Padding(0, 2)

	modalConfirmActiveErr = lipgloss.NewStyle().
				Background(colorErr).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 2)

	modalConfirmActiveAccent = lipgloss.NewStyle().
				Background(colorAccent).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 2)

	settingsSaveActive = lipgloss.NewStyle().
				Background(colorAccent).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 3)

	settingsSaveInactive = lipgloss.NewStyle().
				Background(colorDim).
				Foreground(colorWhite).
				Padding(0, 3)

	settingsCancelActive = lipgloss.NewStyle().
				Background(colorErr).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Padding(0, 3)

	settingsCancelInactive = lipgloss.NewStyle().
				Background(colorDim).
				Foreground(colorWhite).
				Padding(0, 3)

	localSourceTag = lipgloss.NewStyle().
				Foreground(colorLocal)

	managePackageRightBorder = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorDim).
				PaddingLeft(3)

	progressBarEmptyStyle = lipgloss.NewStyle().Foreground(colorDim)
	progressBarFilledStyle = lipgloss.NewStyle().Foreground(colorCyan)
	spinnerStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	managePackageLeftPanelStyle = lipgloss.NewStyle().Width(65)
	desktopSubTitleStyle = lipgloss.NewStyle().Foreground(colorMuted)
	modalWrapperStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorCyan).
				Padding(1, 3)
	tableHeaderStyle = lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)
	dirtyIndicatorStyle = lipgloss.NewStyle().Foreground(colorNpm)
	selectedRowBgStyle = lipgloss.NewStyle().Background(colorDim)
	statusGreenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043"))
	accentRegularStyle = lipgloss.NewStyle().Foreground(colorAccent)
	cyanRegularStyle = lipgloss.NewStyle().Foreground(colorCyan)
	warnBoldStyle = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	cyanBoldStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	blinkGreenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Bold(true)
)
