package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"owd-cli/bridge"

	tea "github.com/charmbracelet/bubbletea"
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
)

// ─────────────────────────────────────────────
// Msg types
// ─────────────────────────────────────────────

type contextLoadedMsg struct {
	Ctx *bridge.WorkspaceContext
	Err error
}

type catalogLoadedMsg struct {
	Cat *bridge.CatalogResponse
	Err error
}

type logLineMsg string
type clearLogsMsg struct{}

type taskFinishedMsg struct {
	Success bool
	Err     error
}

type serverStatusMsg struct {
	Running bool
}

type promptThemeDepMsg struct {
	DepName   string
	ShortName string
}

type themeDepDecision struct {
	install bool
	method  string
}

type tickMsg time.Time

type memTickMsg struct {
	MemMB int
}

type GitChanges struct {
	Added    int
	Modified int
	Deleted  int
}

type UpdateInfo struct {
	LocalGit    bool
	BehindCount int
	Npm         bool
}

type updatesLoadedMsg struct {
	GitChanges map[string]GitChanges
	Updates    map[string]UpdateInfo
}

type setupProgressMsg struct {
	Step  int
	Total int
	Label string
}

// ─────────────────────────────────────────────
// Prompt states
// ─────────────────────────────────────────────

type PromptType int

const (
	PromptNone PromptType = iota
	PromptInstallMethod
	PromptUninstallConfirm
	PromptManagePackage
	PromptForceReinstallConfirm
	PromptSettings
	PromptThemeDepConfirm
	PromptThemeDepMethod
	PromptSetupProgress
	PromptWipeWorkspaceConfirm
)

type ActiveTask int

const (
	TaskNone ActiveTask = iota
	TaskSetup
	TaskWipe
	TaskServe
)

var Program *tea.Program
var ServerCmd *exec.Cmd

// ─────────────────────────────────────────────
// Model
// ─────────────────────────────────────────────

type pendingDecision struct {
	PkgName   string
	ShortName string
	Action    string // "install" or "uninstall"
	Kind      string // "app", "module", "theme"
	Entry     bridge.CatalogEntry
}

type TuiModel struct {
	workspaceRoot string
	ctx           *bridge.WorkspaceContext
	catalog       *bridge.CatalogResponse

	ExitCode     int
	activeTab    int // 0: Internal, 1: Apps, 2: Modules, 3: Themes
	err          error
	loading      bool
	statusMsg    string
	logLines     []string
	memHistory   []int

	// Selection
	selectedIndex int
	scrollOffset  int

	// Prompts/Modals
	activePrompt PromptType
	promptPkg    *bridge.CatalogEntry
	promptSel    int

	promptPkgDeps        []string
	promptPkgHasPrimevue bool
	promptPkgHasTailwind bool

	gitChangesMap map[string]GitChanges
	updatesMap    map[string]UpdateInfo

	settingsSel         int
	settingsInstallMode string
	settingsCatalogSort string

	// Server status
	serverRunning   bool
	taskActive      bool
	activeTask      ActiveTask
	checkingUpdates bool

	// Animation
	blink      bool
	tickCount  int

	// Layout
	termWidth  int
	termHeight int

	lastInstallMethod string
	logTailerStarted  bool

	// Queued changes
	pendingPackages map[string]bool // pkgName -> install(true)/uninstall(false)
	pendingTheme    *string         // pointer to theme name to activate

	// Wizard / Review Queue
	promptQueue      []pendingDecision
	promptQueueIndex int
	finalizedAdds    map[string]string // pkgName -> method
	finalizedRemoves []string          // pkgNames
	finalizedTheme   *string           // theme name to activate

	themeDepResolveChan chan themeDepDecision
	activeThemeDep      string
	startupCheckDone    bool
	setupStep           int
	setupTotalSteps     int
	setupLabel          string
}

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
		settingsSel:      0,
		termWidth:        160,
		termHeight:       40,
		lastInstallMethod: "npm",
		logTailerStarted:  false,
		pendingPackages:  make(map[string]bool),
		pendingTheme:     nil,
		finalizedAdds:    make(map[string]string),
		themeDepResolveChan: make(chan themeDepDecision, 1),
		startupCheckDone:    false,
		setupStep:           0,
		setupTotalSteps:     0,
		setupLabel:          "",
		activeTask:          TaskNone,
	}
}

func (m TuiModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadContextCmd(),
		m.loadCatalogCmd(false),
		m.checkServerStatusCmd(),
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

func (m TuiModel) loadContextCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, err := bridge.ReadWorkspaceContext(m.workspaceRoot)
		return contextLoadedMsg{Ctx: ctx, Err: err}
	}
}

func (m TuiModel) loadCatalogCmd(force bool) tea.Cmd {
	return func() tea.Msg {
		cat, err := bridge.ReadCatalog(m.workspaceRoot, force)
		return catalogLoadedMsg{Cat: cat, Err: err}
	}
}

func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func findProcessPID(port int) int {
	// Try lsof first with TCP:LISTEN filter to get only the listening process
	cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf(":%d", port), "-sTCP:LISTEN")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		var pid int
		if _, err := fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &pid); err == nil && pid > 0 {
			return pid
		}
	}

	// Try pgrep patterns
	patterns := []string{
		"nuxt dev",
		"nuxi dev",
		"nuxt.mjs dev",
		"nuxi.mjs dev",
		"nx run desktop:serve",
	}
	for _, pat := range patterns {
		cmd = exec.Command("pgrep", "-f", pat)
		stdout.Reset()
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			lines := strings.Split(stdout.String(), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var pid int
				if _, err := fmt.Sscanf(line, "%d", &pid); err == nil && pid > 0 {
					return pid
				}
			}
		}
	}
	return 0
}

func (m TuiModel) rebootServerCmd() tea.Cmd {
	return func() tea.Msg {
		if Program != nil {
			Program.Send(logLineMsg(">>> Theme change detected. Rebooting dev server…"))
			m.StopServeTask(Program)
			time.Sleep(1500 * time.Millisecond)
			m.RunServeTask(Program)
		}
		return nil
	}
}

func (m TuiModel) checkServerStatusCmd() tea.Cmd {
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

func (m TuiModel) hasPendingChanges() bool {
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

func (m *TuiModel) startStartupCheck() tea.Cmd {
	m.promptQueue = []pendingDecision{}
	m.promptQueueIndex = 0
	m.finalizedAdds = make(map[string]string)
	m.finalizedRemoves = []string{}
	m.finalizedTheme = nil

	if m.catalog == nil {
		return nil
	}

	// Non-installable core packages — never prompt for these
	nonInstallable := map[string]bool{
		"@owdproject/core": true,
		"@owdproject/cli":  true,
		"@owdproject/nx":   true,
	}

	// Track what is already queued to avoid duplicates
	queued := map[string]bool{}

	// Round 1: apps, modules, and theme from catalog
	for _, entry := range m.catalog.Entries {
		if entry.Installed && !entry.LocalSource {
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

	// Round 2: find the active/pending theme and read its kit/module deps
	var activeThemeShort string
	for _, dec := range m.promptQueue {
		if dec.Kind == "theme" {
			activeThemeShort = dec.ShortName
			break
		}
	}
	// Also check the current config theme if not in queue
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
				// Only kit-* and module-* prefixed @owdproject packages
				short := dep
				if idx := strings.LastIndex(dep, "/"); idx >= 0 {
					short = dep[idx+1:]
				}
				if !strings.HasPrefix(short, "kit-") && !strings.HasPrefix(short, "module-") {
					continue
				}
				// Skip if already installed locally
				if isLocallyAvailable(m.workspaceRoot, short) {
					continue
				}
				// Find in catalog (may not be listed)
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
					// Synthesize a minimal entry for the wizard
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

	// For themes already installed in the workspace, we don't need to ask for install method!
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
	m.taskActive = true
	m.activeTask = TaskSetup
	m.statusMsg = "Applying queued package changes…"
	m.addLog(">>> Applying queued package configuration changes…")

	payload := &bridge.WritePayload{
		Config:       &bridge.Config{Theme: m.ctx.Config.Theme, Apps: m.ctx.Config.Apps, Modules: m.ctx.Config.Modules},
		DepsToAdd:    make(map[string]string),
		DepsToRemove: []string{},
	}

	// 1. Process removals
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

	// 2. Process theme removal (if active theme was uninstalled):
	for _, name := range m.finalizedRemoves {
		if payload.Config.Theme != nil && *payload.Config.Theme == name {
			e := ""
			payload.Config.Theme = &e
		}
	}

	// 3. Process additions
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

	// 4. Process theme change
	if m.finalizedTheme != nil {
		payload.Config.Theme = m.finalizedTheme
	}

	// Capture variables for the closure
	finalAdds := m.finalizedAdds

	// Clear pending changes
	m.pendingPackages = make(map[string]bool)
	m.pendingTheme = nil

	return func() tea.Msg {
		// Write changes
		if err := bridge.WriteChanges(m.workspaceRoot, payload); err != nil {
			return taskFinishedMsg{Success: false, Err: err}
		}

		// Trigger setup task in background — return a log line so bubbletea
		// keeps taskActive=true until RunSetupTask sends taskFinishedMsg.
		if Program != nil {
			m.RunSetupTask(finalAdds, Program)
		}

		return logLineMsg(">>> Workspace changes written. Running setup task…")
	}
}

// ─────────────────────────────────────────────
// Update
// ─────────────────────────────────────────────

func (m TuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tickMsg:
		m.tickCount++
		if m.tickCount%4 == 0 {
			m.blink = !m.blink
		}
		
		var statusCmd tea.Cmd
		if m.tickCount%10 == 0 {
			// Sample Nuxt process memory stats
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
				statusCmd = m.checkServerStatusCmd()
			}
		}
		
		if m.tickCount%100 == 0 && !m.loading && !m.taskActive && !m.checkingUpdates {
			m.checkingUpdates = true
			if statusCmd != nil {
				return m, tea.Batch(tickCmd(), statusCmd, m.checkForUpdatesCmd())
			}
			return m, tea.Batch(tickCmd(), m.checkForUpdatesCmd())
		}
		
		if statusCmd != nil {
			return m, tea.Batch(tickCmd(), statusCmd)
		}
		return m, tickCmd()

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
			if !m.serverRunning && !m.taskActive {
				cmd := m.startStartupCheck()
				if cmd != nil {
					return m, cmd
				}

				m.statusMsg = "Starting dev server…"
				m.taskActive = true
				m.activeTask = TaskServe
				if Program != nil {
					m.RunServeTask(Program)
				}
			}
		case "x":
			if m.serverRunning {
				m.statusMsg = "Stopping dev server…"
				m.taskActive = true
				m.activeTask = TaskServe
				if Program != nil {
					m.StopServeTask(Program)
				}
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
				if pkg.Installed && !m.taskActive {
					m.taskActive = true
					m.activeTask = TaskSetup
					m.statusMsg = fmt.Sprintf("Updating %s…", pkg.ShortName)
					if Program != nil {
						m.RunUpdatePackageTask(pkg.Name, pkg.ShortName, pkg.Kind, pkg.LocalSource, Program)
					}
				}
			}
		case "g":
			if m.ctx != nil {
				m.settingsInstallMode = m.ctx.Settings.InstallMode
				m.settingsCatalogSort = m.ctx.Settings.CatalogSort
				m.settingsSel = 0
				m.activePrompt = PromptSettings
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
				StartLogTailer(logPath, Program)
			}

			if themeChanged && m.serverRunning {
				m.statusMsg = "Rebooting server for theme change…"
				m.taskActive = true
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

			m.checkingUpdates = true
			return m, m.checkForUpdatesCmd()
		}

	case updatesLoadedMsg:
		m.gitChangesMap = msg.GitChanges
		m.updatesMap = msg.Updates
		m.checkingUpdates = false
		m.loading = false
		count := 0
		for _, up := range msg.Updates {
			if up.LocalGit || up.Npm {
				count++
			}
		}
		if count > 0 {
			m.statusMsg = fmt.Sprintf("Updates check completed: %d update(s) available.", count)
		} else {
			m.statusMsg = "All packages are up to date."
		}

	case setupProgressMsg:
		m.setupStep = msg.Step
		m.setupTotalSteps = msg.Total
		m.setupLabel = msg.Label
		return m, nil
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
			break
		}
		if strings.Contains(line, "Local:") || strings.Contains(line, "Network:") || strings.Contains(line, "expose") || strings.Contains(line, "➜") {
			break
		}
		m.addLog(line)

	case clearLogsMsg:
		m.logLines = []string{}
		return m, nil

	case taskFinishedMsg:
		m.taskActive = false
		m.activeTask = TaskNone
		if msg.Success {
			m.statusMsg = "Task completed."
			m.addLog(">>> Task completed successfully.")
		} else {
			m.statusMsg = fmt.Sprintf("Task failed: %v", msg.Err)
			m.addLog(fmt.Sprintf(">>> Task failed: %v", msg.Err))
		}
		return m, tea.Batch(m.loadContextCmd(), m.loadCatalogCmd(false))

	case serverStatusMsg:
		m.serverRunning = msg.Running
		if m.activeTask == TaskServe {
			m.taskActive = false
			m.activeTask = TaskNone
		}
		if !msg.Running {
			m.statusMsg = "Dev server stopped."
		} else {
			m.statusMsg = "Dev server running."
		}
	}

	return m, nil
}

func (m *TuiModel) addLog(line string) {
	m.logLines = append(m.logLines, line)
	if len(m.logLines) > 200 {
		m.logLines = m.logLines[1:]
	}
}

// ─────────────────────────────────────────────
// Prompt keys
// ─────────────────────────────────────────────

func (m TuiModel) handlePromptKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	switch msg.String() {
	case "esc", "q":
		if m.activePrompt == PromptThemeDepConfirm || m.activePrompt == PromptThemeDepMethod {
			m.themeDepResolveChan <- themeDepDecision{install: false}
		}
		if m.activePrompt == PromptWipeWorkspaceConfirm {
			m.activePrompt = PromptSettings
			return m, nil
		}
		m.activePrompt = PromptNone
		m.promptPkg = nil
		m.promptQueue = nil
		m.promptQueueIndex = 0
		m.activeThemeDep = ""
		return m, nil
	case "up", "k":
		if m.activePrompt == PromptManagePackage {
			if m.promptSel > 0 {
				m.promptSel--
			}
		} else if m.activePrompt == PromptSettings {
			if m.settingsSel == 5 || m.settingsSel == 6 {
				m.settingsSel = 4
			} else if m.settingsSel > 0 {
				m.settingsSel--
			}
		} else if m.promptSel > 0 {
			m.promptSel--
		}
	case "down", "j":
		if m.activePrompt == PromptManagePackage {
			if m.promptSel < 5 {
				m.promptSel++
			}
		} else if m.activePrompt == PromptSettings {
			if m.settingsSel == 4 {
				m.settingsSel = 5
			} else if m.settingsSel < 4 {
				m.settingsSel++
			}
		} else {
			limit := 2
			if m.activePrompt == PromptUninstallConfirm || m.activePrompt == PromptForceReinstallConfirm || m.activePrompt == PromptThemeDepConfirm || m.activePrompt == PromptWipeWorkspaceConfirm {
				limit = 1
			} else if (m.activePrompt == PromptInstallMethod || m.activePrompt == PromptThemeDepMethod) && m.promptPkg != nil {
				limit = len(m.getInstallMethods(m.promptPkg)) - 1
			}
			if m.promptSel < limit {
				m.promptSel++
			}
		}
	case "left", "h":
		if m.activePrompt == PromptSettings {
			if m.settingsSel == 6 {
				m.settingsSel = 5
			}
		} else if m.activePrompt != PromptManagePackage && m.activePrompt != PromptInstallMethod && m.activePrompt != PromptThemeDepMethod {
			if m.promptSel > 0 {
				m.promptSel--
			}
		}
	case "right", "l":
		if m.activePrompt == PromptSettings {
			if m.settingsSel == 5 {
				m.settingsSel = 6
			}
		} else if m.activePrompt != PromptManagePackage && m.activePrompt != PromptInstallMethod && m.activePrompt != PromptThemeDepMethod {
			limit := 2
			if m.activePrompt == PromptUninstallConfirm || m.activePrompt == PromptForceReinstallConfirm || m.activePrompt == PromptThemeDepConfirm || m.activePrompt == PromptWipeWorkspaceConfirm {
				limit = 1
			}
			if m.promptSel < limit {
				m.promptSel++
			}
		}
	case "enter":
		pkg := m.promptPkg
		prompt := m.activePrompt

		if prompt == PromptWipeWorkspaceConfirm {
			if m.promptSel == 0 { // Yes
				m.activePrompt = PromptNone
				m.taskActive = true
				m.activeTask = TaskWipe
				m.statusMsg = "Resetting workspace…"
				m.addLog(">>> Initiating workspace reset task…")
				return m, m.runWipeWorkspaceCmd()
			} else { // No
				m.activePrompt = PromptSettings
				return m, nil
			}
		}

		if prompt == PromptThemeDepConfirm {
			if m.promptSel == 0 { // Yes
				m.activePrompt = PromptThemeDepMethod
				methods := m.getInstallMethods(pkg)
				selIdx := 0
				for idx, mth := range methods {
					if mth.Name == m.lastInstallMethod {
						selIdx = idx
						break
					}
				}
				m.promptSel = selIdx
				return m, nil
			} else { // No
				m.activePrompt = PromptNone
				m.promptPkg = nil
				m.activeThemeDep = ""
				m.themeDepResolveChan <- themeDepDecision{install: false}
				return m, nil
			}
		} else if prompt == PromptThemeDepMethod {
			m.activePrompt = PromptNone
			m.promptPkg = nil
			m.activeThemeDep = ""
			methods := m.getInstallMethods(pkg)
			if m.promptSel >= 0 && m.promptSel < len(methods) {
				selectedMethod := methods[m.promptSel].Name
				m.lastInstallMethod = selectedMethod
				m.themeDepResolveChan <- themeDepDecision{install: true, method: selectedMethod}
			} else {
				m.themeDepResolveChan <- themeDepDecision{install: false}
			}
			return m, nil
		}

		if len(m.promptQueue) > 0 {
			if prompt == PromptUninstallConfirm {
				if m.promptSel == 0 { // Yes
					m.finalizedRemoves = append(m.finalizedRemoves, pkg.Name)
				}
				m.promptQueueIndex++
				cmd := m.processNextQueueDecision()
				return m, cmd
			} else if prompt == PromptInstallMethod {
				methods := m.getInstallMethods(pkg)
				if m.promptSel >= 0 && m.promptSel < len(methods) {
					m.lastInstallMethod = methods[m.promptSel].Name
					m.finalizedAdds[pkg.Name] = methods[m.promptSel].Name
				}
				m.promptQueueIndex++
				cmd := m.processNextQueueDecision()
				return m, cmd
			}
			return m, nil
		}

		if prompt == PromptUninstallConfirm {
			m.activePrompt = PromptNone
			m.promptPkg = nil
			if m.promptSel == 0 {
				return m.triggerUninstall(pkg)
			}
		} else if prompt == PromptInstallMethod {
			m.activePrompt = PromptNone
			m.promptPkg = nil
			methods := m.getInstallMethods(pkg)
			if m.promptSel >= 0 && m.promptSel < len(methods) {
				m.lastInstallMethod = methods[m.promptSel].Name
				return m.triggerInstall(pkg, methods[m.promptSel].Name)
			}
			return m, nil
		} else if prompt == PromptForceReinstallConfirm {
			m.activePrompt = PromptNone
			m.promptPkg = nil
			if m.promptSel == 0 {
				return m.triggerForceReinstall(pkg)
			}
		} else if prompt == PromptManagePackage {
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
		} else if prompt == PromptSettings {
			switch m.settingsSel {
			case 0: // Install Mode
				if m.settingsInstallMode == "npm" {
					m.settingsInstallMode = "workspace"
				} else {
					m.settingsInstallMode = "npm"
				}
				m.activePrompt = PromptSettings
				return m, nil
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
				m.activePrompt = PromptSettings
				return m, nil
			case 2: // Trusted Orgs
				m.addLog(">>> To configure Trusted Orgs, modify .desktop/settings.json (githubOrgs field).")
				m.activePrompt = PromptSettings
				return m, nil
			case 3: // GitHub User
				m.addLog(">>> To configure GitHub User, modify .desktop/settings.json or use OWD_GITHUB_USER env var.")
				m.activePrompt = PromptSettings
				return m, nil
			case 4: // Reset Workspace
				m.activePrompt = PromptWipeWorkspaceConfirm
				m.promptSel = 1 // default to No
				return m, nil
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
				return m, nil
			}
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Actions
// ─────────────────────────────────────────────

func (m TuiModel) triggerUninstall(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.taskActive = true
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
		m.taskActive = false
		m.activeTask = TaskNone
		m.statusMsg = fmt.Sprintf("Uninstall failed: %v", err)
		return m, nil
	}
	if Program != nil {
		m.RunSetupTask(make(map[string]string), Program)
	}
	return m, nil
}

func (m TuiModel) triggerInstall(pkg *bridge.CatalogEntry, method string) (tea.Model, tea.Cmd) {
	m.taskActive = true
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
		m.taskActive = false
		m.activeTask = TaskNone
		m.statusMsg = fmt.Sprintf("Install failed: %v", err)
		return m, nil
	}
	if Program != nil {
		m.RunSetupTask(map[string]string{pkg.Name: method}, Program)
	}
	return m, nil
}

func (m TuiModel) triggerForceReinstall(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.taskActive = true
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

	if Program != nil {
		m.RunSetupTask(map[string]string{pkg.Name: method}, Program)
	}
	return m, nil
}

func (m TuiModel) triggerUpdate(pkg *bridge.CatalogEntry) (tea.Model, tea.Cmd) {
	m.taskActive = true
	m.activeTask = TaskSetup
	m.statusMsg = fmt.Sprintf("Updating %s…", pkg.ShortName)
	m.addLog(fmt.Sprintf(">>> Updating %s", pkg.Name))

	if Program != nil {
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
					Program.Send(logLineMsg(">>> Local Git repository detected. Running git pull…"))
					runProcessAndStreamLogs(pkgPath, "git", []string{"pull"}, Program)
					Program.Send(taskFinishedMsg{Success: true})
					return
				}
			}

			Program.Send(logLineMsg(fmt.Sprintf(">>> Running pnpm install %s@latest…", pkg.Name)))
			runProcessAndStreamLogs(m.workspaceRoot, "pnpm", []string{"install", pkg.Name + "@latest"}, Program)
			Program.Send(logLineMsg(">>> Preparing workspace modules…"))
			runProcessAndStreamLogs(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}, Program)
			Program.Send(taskFinishedMsg{Success: true})
		}()
	}
	return m, nil
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func (m TuiModel) getActiveItems() []bridge.CatalogEntry {
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

func sparkline(vals []int) string {
	blocks := []rune{'\u2581', '\u2582', '\u2583', '\u2584', '\u2585', '\u2586', '\u2587', '\u2588'}
	if len(vals) == 0 {
		mutedBar := lipgloss.NewStyle().Foreground(colorDim).Render(string('\u2581'))
		return strings.Repeat(mutedBar, 12)
	}

	min := vals[0]
	max := vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	var sb strings.Builder
	show := vals
	if len(show) > 12 {
		show = show[len(show)-12:]
	}

	for _, v := range show {
		var idx int
		if max == min {
			if v == 0 {
				idx = 0
			} else {
				idx = 2
			}
		} else {
			idx = int(float64(v-min) / float64(max-min) * float64(len(blocks)-1))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(blocks) {
				idx = len(blocks) - 1
			}
		}

		var color lipgloss.Color
		if v == 0 {
			color = colorDim
		} else {
			if v > 1024 {
				color = colorErr
			} else if v > 512 {
				color = colorWarn
			} else {
				color = colorCyan
			}
		}

		styledRune := lipgloss.NewStyle().Foreground(color).Render(string(blocks[idx]))
		sb.WriteString(styledRune)
	}
	return sb.String()
}

func gitBranch(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "—"
	}
	return strings.TrimSpace(string(out))
}

func gitChanges(root string) string {
	out, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return "clean"
	}
	mod, add, del := 0, 0, 0
	for _, l := range lines {
		if len(l) < 2 {
			continue
		}
		switch {
		case strings.HasPrefix(l, "??"):
			add++
		case strings.HasPrefix(l, " D") || strings.HasPrefix(l, "D "):
			del++
		default:
			mod++
		}
	}
	var parts []string
	if add > 0 {
		parts = append(parts, accentStyle.Render(fmt.Sprintf("+%d", add)))
	}
	if mod > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("~%d", mod)))
	}
	if del > 0 {
		parts = append(parts, errStyle.Render(fmt.Sprintf("-%d", del)))
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, " ")
}

func countLocalDirs(root, subdir string, excludePrefixes ...string) int {
	dir := filepath.Join(root, subdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		skip := false
		for _, pf := range excludePrefixes {
			if strings.HasPrefix(e.Name(), pf) {
				skip = true
				break
			}
		}
		if !skip {
			count++
		}
	}
	return count
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func formatCatalogAge(iso *string) string {
	if iso == nil || *iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, *iso)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", *iso)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05Z", *iso)
			if err != nil {
				return ""
			}
		}
	}
	duration := time.Since(t)
	days := int(duration.Hours() / 24)
	if days < 0 {
		return "<1d"
	}
	if days < 1 {
		return "<1d"
	}
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 30 {
		return fmt.Sprintf("%dw", days/7)
	}
	return fmt.Sprintf("%dmo", days/30)
}

// overlayCenter renders `overlay` centered on top of `bg` (both already rendered, ANSI-aware widths).
func overlayCenter(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	ovLines := strings.Split(overlay, "\n")
	bgH := len(bgLines)
	ovH := len(ovLines)
	bgW := 0
	for _, l := range bgLines {
		if w := lipgloss.Width(l); w > bgW {
			bgW = w
		}
	}
	ovW := 0
	for _, l := range ovLines {
		if w := lipgloss.Width(l); w > ovW {
			ovW = w
		}
	}
	topPad := (bgH - ovH) / 2
	leftPad := (bgW - ovW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	out := make([]string, bgH)
	copy(out, bgLines)
	for i, ovLine := range ovLines {
		row := topPad + i
		if row < 0 || row >= bgH {
			continue
		}
		bg := out[row]
		bgVisible := lipgloss.Width(bg)
		// pad bg line to bgW if shorter
		if bgVisible < bgW {
			bg = bg + strings.Repeat(" ", bgW-bgVisible)
		}
		// split bg into runes accounting for ANSI — use a simple approach:
		// write leftPad chars of bg, then overlay line, then rest of bg
		bgStripped := lipgloss.NewStyle().Render(bg) // no-op, keeps ansi
		// For simplicity: rebuild as left + overlay + right using byte slicing on stripped
		// We use lipgloss.Width-aware padding approach
		prefix := strings.Repeat(" ", leftPad)
		suffix := ""
		rightStart := leftPad + ovW
		if rightStart < bgW {
			suffix = strings.Repeat(" ", bgW-rightStart)
		}
		_ = bgStripped
		out[row] = prefix + ovLine + suffix
	}
	return strings.Join(out, "\n")
}

// ─────────────────────────────────────────────
// Rendering helpers
// ─────────────────────────────────────────────

// Panel drawing helper with border labels
func drawPanel(w, h int, title string, content string, active bool) string {
	style := panelBorderStyle
	if active {
		style = panelBorderActive
	}

	// Ensure the style has the correct dimensions
	style = style.Width(w - 2).Height(h - 2)

	rendered := style.Render(content)

	if title == "" {
		return rendered
	}

	// If the title contains escape sequences, use it as is; otherwise format as bold white
	var styledTitle string
	if strings.Contains(title, "\x1b") {
		styledTitle = title
	} else {
		styledTitle = lipgloss.NewStyle().Foreground(colorWhite).Bold(true).Render(title)
	}

	titleFmt := " " + styledTitle + " "
	titleWidth := lipgloss.Width(titleFmt)

	// Replace the first line's center with the title
	lines := strings.Split(rendered, "\n")
	if len(lines) > 0 {
		borderColor := colorBorder
		if active {
			borderColor = colorBorderAct
		}

		dashCount := w - 3 - titleWidth
		if dashCount < 2 {
			dashCount = 2
		}

		borderStyle := lipgloss.NewStyle().Foreground(borderColor)

		topBorder := borderStyle.Render("╭─") +
			titleFmt +
			borderStyle.Render(strings.Repeat("─", dashCount)+"╮")

		lines[0] = topBorder
		rendered = strings.Join(lines, "\n")
	}

	return rendered
}

type WorkspaceVersions struct {
	Nuxt string
	Pnpm string
	Owd  string
}

func getVersions(root string, workspaceRoot string) WorkspaceVersions {
	vers := WorkspaceVersions{Nuxt: "—", Pnpm: "—", Owd: "—"}
	
	// Read root package.json (monorepo or playground package)
	rootPkgPath := filepath.Join(root, "package.json")
	if data, err := os.ReadFile(rootPkgPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			if strings.Contains(l, `"packageManager"`) && strings.Contains(l, "pnpm@") {
				parts := strings.Split(l, "pnpm@")
				if len(parts) >= 2 {
					vers.Pnpm = strings.Trim(parts[1], " \t\r\n,\"")
					vers.Pnpm = strings.ReplaceAll(vers.Pnpm, `"`, "")
					vers.Pnpm = strings.TrimSpace(vers.Pnpm)
				}
			}
			if strings.Contains(l, `"nuxt"`) {
				parts := strings.Split(l, ":")
				if len(parts) >= 2 {
					v := strings.Trim(parts[1], " \t\r\n,\"^~")
					v = strings.ReplaceAll(v, `"`, "")
					vers.Nuxt = strings.TrimSpace(v)
				}
			}
		}
	}
	
	// Read OWD Core version from workspaceRoot/packages/core/package.json
	corePkgPath := filepath.Join(workspaceRoot, "packages", "core", "package.json")
	if data, err := os.ReadFile(corePkgPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			if strings.Contains(l, `"version"`) {
				parts := strings.Split(l, ":")
				if len(parts) >= 2 {
					v := strings.Trim(parts[1], " \t\r\n,\"")
					v = strings.ReplaceAll(v, `"`, "")
					vers.Owd = strings.TrimSpace(v)
					break
				}
			}
		}
	}
	
	return vers
}

func getProcessMemoryMB(pid int) int {
	if pid <= 0 {
		return 0
	}
	path := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "VmRSS:") {
			fields := strings.Fields(l)
			if len(fields) >= 2 {
				var kb int
				if _, err := fmt.Sscanf(fields[1], "%d", &kb); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

func renderPanelTitle(title string) string {
	return lipgloss.NewStyle().
		Foreground(colorWhite).
		Bold(true).
		Render(title)
}

func kv(label, value string) string {
	return mutedStyle.Render(padRight(label, 10)) + " " + boldStyle.Render(value)
}

func kvRaw(label, value string) string {
	return mutedStyle.Render(padRight(label, 10)) + " " + value
}

func pill(text string, color lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(text)
}

// ─────────────────────────────────────────────
// View
// ─────────────────────────────────────────────

func (m TuiModel) View() string {
	w := m.termWidth
	h := m.termHeight
	if w < 40 {
		w = 40
	}
	if h < 10 {
		h = 10
	}

	showLogs := (m.serverRunning || m.taskActive) && w >= 120

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

	desktopTitle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Desktop") + " " +
		lipgloss.NewStyle().Foreground(colorMuted).Render("control panel")

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
			styledModal := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorCyan).
				Padding(1, 3).
				Render(modal)
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
			styledModal := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorCyan).
				Padding(1, 3).
				Render(modal)
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

func (m TuiModel) renderClientPanel(w, h int) string {
	var lines []string

	// Server status line
	var statusLine string
	if m.taskActive && strings.Contains(strings.ToLower(m.statusMsg), "stopping") {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		dot := lipgloss.NewStyle().Foreground(colorWarn).Bold(true).Render(frame)
		statusLine = dot + " " + boldStyle.Render("STOPPING") + "  " + mutedStyle.Render("http://localhost:3000")
	} else if m.taskActive && strings.Contains(strings.ToLower(m.statusMsg), "starting") {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		dot := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(frame)
		statusLine = dot + " " + boldStyle.Render("STARTING") + "  " + mutedStyle.Render("http://localhost:3000")
	} else if m.serverRunning {
		dot := accentStyle.Render("●")
		if m.blink {
			dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Bold(true).Render("●")
		}
		url := lipgloss.NewStyle().Foreground(colorCyan).Render("http://localhost:3000")
		statusLine = dot + " " + boldStyle.Render("RUNNING") + "   " + url
		if m.ctx != nil {
			_ = m.ctx
		}
	} else if m.taskActive {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		dot := lipgloss.NewStyle().Foreground(colorWarn).Bold(true).Render(frame)
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
	lines = append(lines, mutedStyle.Render("  Theme      ")+boldStyle.Render(theme))

	// Workspace
	wsPath := "—"
	if m.ctx != nil {
		if m.ctx.Paths.IsPlayground {
			wsPath = strings.Replace(m.ctx.Paths.Desktop, os.Getenv("HOME"), "~", 1)
		} else {
			wsPath = strings.Replace(m.ctx.WorkspaceRoot, os.Getenv("HOME"), "~", 1)
		}
	} else if m.workspaceRoot != "" {
		wsPath = strings.Replace(m.workspaceRoot, os.Getenv("HOME"), "~", 1)
	}
	wsPath = truncate(wsPath, w-14)
	lines = append(lines, mutedStyle.Render("  Workspace  ")+boldStyle.Render(wsPath))

	// Git
	root := m.workspaceRoot
	workspaceRoot := m.workspaceRoot
	if m.ctx != nil {
		workspaceRoot = m.ctx.WorkspaceRoot
		if m.ctx.Paths.IsPlayground {
			root = m.ctx.Paths.PackageDir
		} else {
			root = m.ctx.WorkspaceRoot
		}
	}
	branch := gitBranch(root)
	changes := gitChanges(root)
	gitLine := "branch: " + accentStyle.Render(branch) + "  ·  changes: " + changes
	lines = append(lines, mutedStyle.Render("  Git        ")+gitLine)

	// Versions
	vers := getVersions(root, workspaceRoot)
	lines = append(lines, mutedStyle.Render("  Runtime    ")+boldStyle.Render(fmt.Sprintf("Nuxt v%s  ·  OWD v%s  ·  pnpm v%s", vers.Nuxt, vers.Owd, vers.Pnpm)))

	content := strings.Join(lines, "\n")
	return content
}

// ─────────────────────────────────────────────
// Panel: Metrics (right of control panel)
// ─────────────────────────────────────────────

func (m TuiModel) renderMetricsPanel(w, h int) string {
	var lines []string

	// Memory sparkline
	spark := sparkline(m.memHistory)
	memVal := "—"
	if len(m.memHistory) > 0 && m.memHistory[len(m.memHistory)-1] > 0 {
		memVal = fmt.Sprintf("%d MiB", m.memHistory[len(m.memHistory)-1])
	}
	memLine := mutedStyle.Render(padRight("Memory", 9)) + " " + boldStyle.Render(memVal) + "  " + spark
	lines = append(lines, "  "+memLine)

	// Local stats
	root := m.workspaceRoot
	if m.ctx != nil {
		root = m.ctx.WorkspaceRoot
	}
	localApps := countLocalDirs(root, "apps")
	localMods := countLocalDirs(root, "packages", "core", "nx", "kit-", "cli")
	localThemes := countLocalDirs(root, "themes")
	localLine := mutedStyle.Render(padRight("Local", 9)) + " " +
		boldStyle.Render(fmt.Sprintf("%d", localApps)) + mutedStyle.Render(" apps · ") +
		boldStyle.Render(fmt.Sprintf("%d", localMods)) + mutedStyle.Render(" modules · ") +
		boldStyle.Render(fmt.Sprintf("%d", localThemes)) + mutedStyle.Render(" themes")
	lines = append(lines, "  "+localLine)

	// Registry stats
	totalStars := 0
	if m.catalog != nil {
		for _, e := range m.catalog.Entries {
			totalStars += e.Stars
		}
	}
	catCount := "loading…"
	cacheAge := ""
	if m.catalog != nil {
		catCount = fmt.Sprintf("%d pkgs", len(m.catalog.Entries))
		cacheAge = m.catalog.CacheAge
	}
	regLine := mutedStyle.Render(padRight("Registry", 9)) + " " +
		boldStyle.Render(fmt.Sprintf("%d", totalStars)) + mutedStyle.Render(" stars")
	lines = append(lines, "  "+regLine)

	// GitHub user
	ghUser := "(not set)"
	sshText := warnStyle.Render("HTTPS")
	if m.ctx != nil && m.ctx.Settings.GithubUser != nil {
		ghUser = *m.ctx.Settings.GithubUser
		sshText = mutedStyle.Render("SSH ok")
	}
	ghLine := mutedStyle.Render(padRight("GitHub", 9)) + " " + boldStyle.Render(ghUser) + " · " + sshText
	lines = append(lines, "  "+ghLine)

	// Catalog
	catalogLine := mutedStyle.Render(padRight("Catalog", 9)) + " " + boldStyle.Render(catCount)
	if cacheAge != "" {
		catalogLine += " " + mutedStyle.Render("("+cacheAge+")")
	}
	lines = append(lines, "  "+catalogLine)

	// Divider + tip
	lines = append(lines, "  "+subtleStyle.Render(strings.Repeat("─", w-2)))

	tips := []string{
		"Press 1, 2, or 3 to switch catalog tabs",
		"Press [s] to start the dev server",
		"Press [Enter] to install / uninstall a package",
		"Press [c] to change the install source",
		"Press [r] to refresh the package catalog",
	}
	tipIdx := (m.tickCount / 6) % len(tips)
	tipText := "Tip: " + tips[tipIdx]
	tipLine := subtleStyle.Render(truncate(tipText, w-4))
	lines = append(lines, "  "+tipLine)

	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────
// Panel: Catalog
// ─────────────────────────────────────────────

func (m TuiModel) renderCatalogPanel(w, h int, _ bool) string {
	var lines []string

	// Tabs row
	var tabLine strings.Builder
	tabLine.WriteString(" ")
	if m.activeTab == 0 {
		tabLine.WriteString(tabActiveStyle.Render("0 Internal") + "  ")
	}
	tabs := []string{"Apps", "Modules", "Themes"}
	for i, t := range tabs {
		tabIdx := i + 1
		num := fmt.Sprintf("%d", tabIdx)
		if tabIdx == m.activeTab {
			tabLine.WriteString(tabActiveStyle.Render(num + " " + t))
		} else {
			tabLine.WriteString(tabInactiveStyle.Render(num + " " + t))
		}
		if i < len(tabs)-1 {
			tabLine.WriteString("  ")
		}
	}
	sortLabel := "  " + mutedStyle.Render("│") + " " + mutedStyle.Render("Sort: Updated")
	if m.loading {
		spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := lipgloss.NewStyle().Foreground(colorCyan).Render(spinFrames[m.tickCount%len(spinFrames)])
		sortLabel += "  " + spin
	}
	tabLine.WriteString(sortLabel)
	lines = append(lines, tabLine.String())

	// Column header (proportional to width)
	nameW := 26
	if w > 120 {
		nameW = 30
	}
	colHeader := "  " + lipgloss.NewStyle().Foreground(colorSubtle).Render(
		padRight("", 2) + "  " + // badge space
			padRight("NAME", nameW) + "  " +
			padRight("SRC", 5) + "  " +
			padRight("DIR", 5) + "  " +
			padRight("SYNC", 6) + "  " +
			padRight("PUBLISHER", 14) + "  " +
			padLeft("STARS/AGE", 14),
	)
	lines = append(lines, colHeader)
	lines = append(lines, "  "+subtleStyle.Render(strings.Repeat("─", max(w-2, 4))))

	// Item list — reserve 4 rows for: header, col header, divider above detail, detail itself
	detailLines := 2
	fixedRows := 4 // tabrow + colheader + topdivider + bottomdivider
	listH := h - fixedRows - detailLines
	if listH < 1 {
		listH = 1
	}

	items := m.getActiveItems()

	// Clamp scroll
	scrollOff := m.scrollOffset
	if m.selectedIndex >= scrollOff+listH {
		scrollOff = m.selectedIndex - listH + 1
	}
	if scrollOff < 0 {
		scrollOff = 0
	}

	if len(items) == 0 {
		if m.loading {
			lines = append(lines, "  "+mutedStyle.Render("Loading packages…"))
		} else {
			lines = append(lines, "  "+mutedStyle.Render("No packages found."))
		}
	} else {
		start := scrollOff
		end := start + listH
		if end > len(items) {
			end = len(items)
		}
		for i := start; i < end; i++ {
			item := items[i]
			line := m.renderCatalogRow(item, i == m.selectedIndex, w, nameW)
			lines = append(lines, line)
		}
		// Padding to fill listH
		for i := end - start; i < listH; i++ {
			lines = append(lines, "")
		}
	}

	// Detail panel at bottom
	lines = append(lines, "  "+subtleStyle.Render(strings.Repeat("─", max(w-2, 4))))
	detail := m.renderDetailRow(items)
	lines = append(lines, detail)

	return strings.Join(lines, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m TuiModel) renderCatalogRow(item bridge.CatalogEntry, selected bool, w, nameW int) string {
	var badge string
	if item.Kind == "theme" {
		// Radiobox
		active := false
		if m.ctx != nil && m.ctx.Config.Theme != nil && *m.ctx.Config.Theme == item.Name {
			active = true
		}

		pending := false
		if m.pendingTheme != nil && *m.pendingTheme == item.Name {
			pending = true
		}

		if pending {
			if active {
				badge = subtleStyle.Render("(") + accentStyle.Render("●") + subtleStyle.Render(")")
			} else {
				badge = subtleStyle.Render("(") + lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("+") + subtleStyle.Render(")")
			}
		} else {
			if active && m.pendingTheme == nil {
				badge = subtleStyle.Render("(") + accentStyle.Render("●") + subtleStyle.Render(")")
			} else {
				badge = subtleStyle.Render("( )")
			}
		}
	} else {
		// Checkbox
		installed := item.Installed
		pendingAdd := false
		pendingRemove := false

		if on, pending := m.pendingPackages[item.Name]; pending {
			if on {
				pendingAdd = true
			} else {
				pendingRemove = true
			}
		}

		if pendingAdd {
			badge = subtleStyle.Render("[") + lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("+") + subtleStyle.Render("]")
		} else if pendingRemove {
			badge = subtleStyle.Render("[") + lipgloss.NewStyle().Foreground(colorErr).Bold(true).Render("-") + subtleStyle.Render("]")
		} else if installed {
			badge = subtleStyle.Render("[") + accentStyle.Render("●") + subtleStyle.Render("]")
		} else {
			badge = subtleStyle.Render("[ ]")
		}
	}
	badge += " "

	name := truncate(item.ShortName, nameW)

	// Source badge
	src := "NPM"
	srcColor := colorNpm
	if item.LocalSource {
		src = "LOC"
		srcColor = colorLocal
	} else if item.Installed && !item.LocalSource {
		src = "GIT"
		srcColor = colorGit
	}
	srcStr := lipgloss.NewStyle().Foreground(srcColor).Bold(true).Render(src)
	srcPad := strings.Repeat(" ", max(0, 5-lipgloss.Width(srcStr)))

	dir := subtleStyle.Render("---")
	if item.Installed && !item.LocalSource {
		dir = errStyle.Bold(true).Render("MISS")
	} else if item.LocalSource {
		dir = accentStyle.Render("OK")
	}
	dirPad := strings.Repeat(" ", max(0, 5-lipgloss.Width(dir)))

	sync := subtleStyle.Render("---")
	var syncParts []string
	if item.LocalSource {
		if ch, hasChanges := m.gitChangesMap[item.ShortName]; hasChanges {
			if ch.Added > 0 {
				syncParts = append(syncParts, accentStyle.Render(fmt.Sprintf("+%d", ch.Added)))
			}
			if ch.Modified > 0 {
				syncParts = append(syncParts, warnStyle.Render(fmt.Sprintf("~%d", ch.Modified)))
			}
			if ch.Deleted > 0 {
				syncParts = append(syncParts, errStyle.Render(fmt.Sprintf("-%d", ch.Deleted)))
			}
		}
	}
	if up, hasUpdate := m.updatesMap[item.ShortName]; hasUpdate {
		if up.LocalGit {
			countStr := ""
			if up.BehindCount > 0 {
				countStr = fmt.Sprintf("%d", up.BehindCount)
			}
			syncParts = append(syncParts, cyanStyle.Render("↓"+countStr))
		} else if up.Npm {
			syncParts = append(syncParts, accentStyle.Render("↑"))
		}
	}
	if len(syncParts) > 0 {
		sync = strings.Join(syncParts, " ")
	}
	syncPad := strings.Repeat(" ", max(0, 6-lipgloss.Width(sync)))

	pub := item.Org
	if pub == "workspace" || pub == "" {
		pub = "owdproject"
	}
	pubFmt := padRight(truncate(pub, 14), 14)

	age := formatCatalogAge(item.UpdatedAt)
	starsStr := ""
	if item.Stars > 0 {
		starsStr = fmt.Sprintf("★%d", item.Stars)
	}

	metaStr := ""
	if starsStr != "" && age != "" {
		metaStr = warnStyle.Render(starsStr) + " " + subtleStyle.Render(age)
	} else if starsStr != "" {
		metaStr = warnStyle.Render(starsStr)
	} else if age != "" {
		metaStr = subtleStyle.Render(age)
	} else {
		metaStr = subtleStyle.Render("—")
	}

	metaW := lipgloss.Width(metaStr)
	metaPad := ""
	if metaW < 14 {
		metaPad = strings.Repeat(" ", 14-metaW)
	}
	metaFmt := metaPad + metaStr

	nameFmt := padRight(name, nameW)

	row := fmt.Sprintf("  %s  %s  %s%s  %s%s  %s%s  %s  %s",
		badge,
		nameFmt,
		srcStr,
		srcPad,
		dir,
		dirPad,
		sync,
		syncPad,
		pubFmt,
		metaFmt,
	)

	if selected {
		return selectedRowStyle.Render("▶ " + strings.TrimPrefix(row, "  "))
	}
	return row
}

func (m TuiModel) renderDetailRow(items []bridge.CatalogEntry) string {
	if len(items) == 0 || m.selectedIndex >= len(items) {
		return "  " + mutedStyle.Render("Select a package for install preview")
	}
	item := items[m.selectedIndex]

	status := mutedStyle.Render("Not Installed")
	if item.Installed {
		status = accentStyle.Render("Installed")
		if item.LocalSource {
			status += " " + lipgloss.NewStyle().Foreground(colorLocal).Render("(workspace)")
		}
	}

	desc := item.Description
	if desc == "" {
		desc = "—"
	}
	desc = truncate(desc, 60)

	org := item.Org
	if org == "workspace" || org == "" {
		org = "owdproject"
	}

	stars := fmt.Sprintf("%d", item.Stars)
	kind := strings.ToUpper(item.Kind)

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

func (m TuiModel) renderLogsPanel(w, h int) string {
	var lines []string

	lines = append(lines, "")

	// Server URLs
	if m.serverRunning {
		dot := accentStyle.Render("●")
		if !m.blink {
			dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Render("●")
		}

		lines = append(lines, "  "+dot+" "+lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Dev server started"))
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
			lines = append(lines, lipgloss.NewStyle().Foreground(colorAccent).Render("  "+line))
		default:
			lines = append(lines, mutedStyle.Render("  "+line))
		}
	}

	if len(m.logLines) == 0 {
		if m.serverRunning && ServerCmd == nil {
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

func (m TuiModel) renderStatusBar(w int) string {
	// Line 1: current mode + status
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
			statusIcon = lipgloss.NewStyle().Foreground(colorAccent).Render("● ")
		} else {
			statusIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ea043")).Render("● ")
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

	if m.taskActive {
		spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := lipgloss.NewStyle().Foreground(colorCyan).Render(spinFrames[m.tickCount%len(spinFrames)])
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

type InstallMethod struct {
	Name  string
	Label string
	Desc  string
}

func (m TuiModel) getInstallMethods(pkg *bridge.CatalogEntry) []InstallMethod {
	var methods []InstallMethod

	// 0. Local folder option if already present in workspace (pkg.LocalSource is true)
	if pkg.LocalSource {
		methods = append(methods, InstallMethod{
			Name:  "local",
			Label: "Use Existing Local Folder",
			Desc:  "Import/link the existing local folder in your workspace",
		})
	}

	// 1. NPM Registry
	if pkg.SourcesMeta != nil && pkg.SourcesMeta.Npm != nil {
		methods = append(methods, InstallMethod{
			Name:  "npm",
			Label: "NPM Registry",
			Desc:  "Install from npm (not recommended)",
		})
	}

	// 2. Git options
	hasFork := false
	forkOwner := ""
	officialOwner := pkg.Org
	if officialOwner == "" || officialOwner == "workspace" {
		officialOwner = "owdproject"
	}

	if pkg.SourcesMeta != nil && pkg.SourcesMeta.Github.Fork != nil && pkg.SourcesMeta.Github.Fork.Exists {
		// Only treat as "your fork" if GitHub confirms it's actually a fork (forked from another repo)
		if pkg.SourcesMeta.Github.Fork.IsFork {
			hasFork = true
			forkOwner = pkg.SourcesMeta.Github.Fork.Owner
		}
	}

	if hasFork {
		methods = append(methods, InstallMethod{
			Name:  "git-https",
			Label: "GIT HTTPS (your fork)",
			Desc:  fmt.Sprintf("Clone via HTTPS from github.com/%s/%s", forkOwner, pkg.ShortName),
		})
		methods = append(methods, InstallMethod{
			Name:  "git-ssh",
			Label: "GIT SSH (your fork)",
			Desc:  fmt.Sprintf("Clone via SSH from github.com/%s/%s", forkOwner, pkg.ShortName),
		})
	} else {
		methods = append(methods, InstallMethod{
			Name:  "git-https",
			Label: "GIT HTTPS",
			Desc:  fmt.Sprintf("Clone via HTTPS from github.com/%s/%s", officialOwner, pkg.ShortName),
		})
		methods = append(methods, InstallMethod{
			Name:  "git-ssh",
			Label: "GIT SSH",
			Desc:  fmt.Sprintf("Clone via SSH from github.com/%s/%s", officialOwner, pkg.ShortName),
		})
	}

	return methods
}

// ─────────────────────────────────────────────
// Modal
// ─────────────────────────────────────────────

func renderProgressBar(step, total, width int) string {
	if total <= 0 {
		return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("░", width))
	}
	filledWidth := (step * width) / total
	if filledWidth > width {
		filledWidth = width
	}
	if filledWidth < 0 {
		filledWidth = 0
	}
	emptyWidth := width - filledWidth
	filled := lipgloss.NewStyle().Foreground(colorCyan).Render(strings.Repeat("█", filledWidth))
	empty := lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("░", emptyWidth))
	return filled + empty
}

// ─────────────────────────────────────────────
// Modal
// ─────────────────────────────────────────────

func (m TuiModel) renderModal(prompt PromptType) string {
	pkg := m.promptPkg
	var content strings.Builder

	if prompt == PromptInstallMethod {
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
	} else if prompt == PromptThemeDepMethod {
		content.WriteString(boldStyle.Render("Install source for theme dependency: ") + accentStyle.Render(pkg.ShortName) + "\n\n")
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
	} else if prompt == PromptThemeDepConfirm {
		content.WriteString(boldStyle.Render("Theme dependency required: ") + accentStyle.Render(pkg.ShortName) + "\n")
		content.WriteString(subtleStyle.Render("The active or queued theme requires this package to function correctly.") + "\n\n")
		content.WriteString(boldStyle.Render("Install ") + accentStyle.Render(pkg.ShortName) + boldStyle.Render("?") + "\n\n")
		opts := []string{"Yes, install", "No, skip"}
		for i, opt := range opts {
			if i == m.promptSel {
				bg := colorAccent
				if i == 1 {
					bg = colorErr
				}
				content.WriteString(lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 2).Render(" "+opt+" "))
			} else {
				content.WriteString(modalOptionInactive.Render(" " + opt + " "))
			}
			content.WriteString("  ")
		}
		content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	} else if prompt == PromptUninstallConfirm {
		content.WriteString(boldStyle.Render("Uninstall ") + errStyle.Render(pkg.ShortName) + boldStyle.Render("?") + "\n\n")
		opts := []string{"Yes, uninstall", "No, keep it"}
		for i, opt := range opts {
			if i == m.promptSel {
				bg := colorErr
				if i == 1 {
					bg = colorAccent
				}
				content.WriteString(lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 2).Render(" "+opt+" "))
			} else {
				content.WriteString(modalOptionInactive.Render(" " + opt + " "))
			}
			content.WriteString("  ")
		}
		content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	} else if prompt == PromptForceReinstallConfirm {
		content.WriteString(boldStyle.Render("Force re-download ") + errStyle.Render(pkg.ShortName) + boldStyle.Render("?") + "\n")
		content.WriteString(warnStyle.Render("⚠ WARNING: This will delete the local directory and lose all modifications!") + "\n\n")
		opts := []string{"Yes, wipe & reinstall", "No, cancel"}
		for i, opt := range opts {
			if i == m.promptSel {
				bg := colorErr
				if i == 1 {
					bg = colorAccent
				}
				content.WriteString(lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 2).Render(" "+opt+" "))
			} else {
				content.WriteString(modalOptionInactive.Render(" " + opt + " "))
			}
			content.WriteString("  ")
		}
		content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	} else if prompt == PromptSetupProgress {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.tickCount%len(spinnerFrames)]
		spin := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(frame)

		title := "Installing packages…"
		if strings.Contains(strings.ToLower(m.setupLabel), "clean") || strings.Contains(strings.ToLower(m.setupLabel), "reset") || strings.Contains(strings.ToLower(m.setupLabel), "wipe") {
			title = "Resetting workspace…"
		}
		content.WriteString(boldStyle.Render(title) + "\n\n")
		content.WriteString("  " + spin + " " + boldStyle.Render(m.setupLabel) + "\n\n")

		bar := renderProgressBar(m.setupStep, m.setupTotalSteps, 40)
		content.WriteString("  " + bar + "\n")
		content.WriteString(fmt.Sprintf("  Step %d of %d\n", m.setupStep, m.setupTotalSteps))
		return modalStyle.Width(72).Render(content.String())
	} else if prompt == PromptManagePackage {
		var leftCols []string
		leftCols = append(leftCols, boldStyle.Render("Package:")+" "+accentStyle.Render(pkg.ShortName))
		leftCols = append(leftCols, mutedStyle.Render("Type:   ")+" "+boldStyle.Render(strings.ToUpper(pkg.Kind)))

		status := mutedStyle.Render("Not Installed")
		if pkg.Installed {
			status = accentStyle.Render("Installed")
			if pkg.LocalSource {
				status += " " + lipgloss.NewStyle().Foreground(colorLocal).Render("(workspace)")
			}
		}
		leftCols = append(leftCols, mutedStyle.Render("Status: ")+" "+status)

		src := "NPM"
		if pkg.LocalSource {
			src = "LOC (workspace)"
		} else if pkg.Installed && !pkg.LocalSource {
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

		actions := []string{
			"Update package",
			"Switch to NPM registry",
			"Switch to Git (HTTPS)",
			"Switch to Git (SSH)",
			"Force Re-download (wipe changes)",
			"Back to catalog",
		}

		for i, act := range actions {
			if i == m.promptSel {
				rightCols = append(rightCols, modalOptionActive.Render(fmt.Sprintf(" %s ", act)))
			} else {
				rightCols = append(rightCols, modalOptionInactive.Render(fmt.Sprintf(" %s ", act)))
			}
		}

		rightPanel := strings.Join(rightCols, "\n")

		splitLayout := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(65).Render(leftPanel),
			lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorDim).
				PaddingLeft(3).
				Render(rightPanel),
		)

		content.WriteString(splitLayout)
		content.WriteString("\n\n" + subtleStyle.Render("↑↓ select  Enter confirm  Esc close"))

		return modalStyle.Width(110).Render(content.String())
	} else if m.activePrompt == PromptSettings {
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
		var orgsVal string
		if m.ctx != nil && len(m.ctx.Settings.GithubOrgs) > 0 {
			orgsVal = strings.Join(m.ctx.Settings.GithubOrgs, ", ")
		} else {
			orgsVal = "owdproject, atproto-os" // default/fallback
		}
		orgsValText := boldStyle.Render(orgsVal) + "   " + subtleStyle.Render("(Read-only. Edit settings.json)")
		if m.settingsSel == 2 {
			content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(orgsLabel) + "\n   " + orgsValText + "\n")
		} else {
			content.WriteString("  " + boldStyle.Render(orgsLabel) + "\n   " + orgsValText + "\n")
		}
		content.WriteString("\n")

		// 4. GitHub User
		ghLabel := "4. GitHub User"
		ghUser := "(not set)"
		if m.ctx != nil && m.ctx.Settings.GithubUser != nil && *m.ctx.Settings.GithubUser != "" {
			ghUser = *m.ctx.Settings.GithubUser
		}
		ghVal := boldStyle.Render(ghUser) + "   " + subtleStyle.Render("(Read-only. Edit settings.json)")
		if m.settingsSel == 3 {
			content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(ghLabel) + "\n   " + ghVal + "\n")
		} else {
			content.WriteString("  " + boldStyle.Render(ghLabel) + "\n   " + ghVal + "\n")
		}
		content.WriteString("\n")

		// 5. Reset Workspace
		resetLabel := "5. Reset Workspace"
		resetVal := modalOptionInactive.Render(" [ WIPE EVERYTHING ] ")
		if m.settingsSel == 4 {
			resetVal = lipgloss.NewStyle().Background(colorErr).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 2).Render(" [ WIPE EVERYTHING ] ")
			content.WriteString(accentStyle.Render("▶ ") + boldStyle.Render(resetLabel) + "\n   " + resetVal + "\n")
		} else {
			content.WriteString("  " + boldStyle.Render(resetLabel) + "\n   " + resetVal + "\n")
		}
		content.WriteString("\n\n")

		// Buttons Row: Save and Cancel
		var saveBtn, cancelBtn string
		if m.settingsSel == 5 {
			saveBtn = lipgloss.NewStyle().Background(colorAccent).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 3).Render(" SAVE ")
		} else {
			saveBtn = lipgloss.NewStyle().Background(colorDim).Foreground(colorWhite).Padding(0, 3).Render(" SAVE ")
		}

		if m.settingsSel == 6 {
			cancelBtn = lipgloss.NewStyle().Background(colorErr).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 3).Render(" CANCEL ")
		} else {
			cancelBtn = lipgloss.NewStyle().Background(colorDim).Foreground(colorWhite).Padding(0, 3).Render(" CANCEL ")
		}

		content.WriteString("      " + saveBtn + "    " + cancelBtn + "\n\n")
		content.WriteString(subtleStyle.Render("↑↓ select item  ←→ toggle/buttons  Enter confirm  Esc exit"))
		return modalStyle.Width(72).Render(content.String())
	} else if prompt == PromptWipeWorkspaceConfirm {
		content.WriteString(boldStyle.Render("Wipe & Reset Workspace?") + "\n")
		content.WriteString(errStyle.Render("⚠ CAUTION: This will delete ALL non-core applications, modules, and themes!") + "\n")
		content.WriteString(subtleStyle.Render("Your configuration files (desktop.config.ts) and desktop package.json will be reset.") + "\n\n")
		opts := []string{"Yes, wipe everything", "No, cancel"}
		for i, opt := range opts {
			if i == m.promptSel {
				bg := colorErr
				if i == 1 {
					bg = colorAccent
				}
				content.WriteString(lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 2).Render(" " + opt + " "))
			} else {
				content.WriteString(modalOptionInactive.Render(" " + opt + " "))
			}
			content.WriteString("  ")
		}
		content.WriteString("\n\n" + subtleStyle.Render("← → select  Enter confirm  Esc cancel"))
	}

	return modalStyle.Render(content.String())
}

// ─────────────────────────────────────────────
// Background tasks
// ─────────────────────────────────────────────

func runProcessAndStreamLogs(root, command string, args []string, p *tea.Program) {
	cmd := exec.Command(command, args...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.Send(taskFinishedMsg{Success: false, Err: err})
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		p.Send(taskFinishedMsg{Success: false, Err: err})
		return
	}

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				// ignore partial read errors
			}
			break
		}
		p.Send(logLineMsg(strings.TrimRight(line, "\r\n")))
	}

	err = cmd.Wait()
	p.Send(taskFinishedMsg{Success: err == nil, Err: err})
}

func runProcessAndStreamLogsSilent(root, command string, args []string, p *tea.Program) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		p.Send(logLineMsg(strings.TrimRight(line, "\r\n")))
	}

	return cmd.Wait()
}

func (m *TuiModel) RunSetupTask(adds map[string]string, p *tea.Program) {
	go func() {
		desktopJs := filepath.Join(m.workspaceRoot, "packages", "cli", "bin", "desktop.js")

		totalSteps := len(adds) + 2
		step := 0
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Initializing setup task…"})

		// 1. Process all additions
		for pkgName, method := range adds {
			step++
			shortName := pkgName
			if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
				shortName = pkgName[idx+1:]
			}
			p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: fmt.Sprintf("Installing %s via %s…", shortName, method)})

			args := []string{desktopJs, "add", shortName}
			if method == "npm" {
				args = append(args, "--npm")
			} else if method == "local" {
				args = append(args, "--dev")
			} else {
				owner := "owdproject"
				if m.ctx != nil && m.ctx.Settings.GithubUser != nil && *m.ctx.Settings.GithubUser != "" {
					owner = *m.ctx.Settings.GithubUser
				} else if m.catalog != nil {
					for _, e := range m.catalog.Entries {
						if e.Name == pkgName && e.Org != "" {
							owner = e.Org
							break
						}
					}
				}

				var fromVal string
				if method == "git-ssh" {
					fromVal = fmt.Sprintf("git@github.com:%s/%s.git", owner, shortName)
				} else {
					fromVal = fmt.Sprintf("https://github.com/%s/%s.git", owner, shortName)
				}
				args = append(args, "--from", fromVal)
			}

			p.Send(logLineMsg(fmt.Sprintf(">>> Executing: node desktop.js add %s via %s", shortName, method)))
			if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "node", args, p); err != nil {
				p.Send(logLineMsg(fmt.Sprintf(">>> Add %s failed: %v", shortName, err)))
				p.Send(taskFinishedMsg{Success: false, Err: err})
				return
			}

			// Resolve theme dependencies if it's a theme
			isTheme := strings.HasPrefix(shortName, "theme-")
			if isTheme {
				if method == "npm" {
					p.Send(logLineMsg(">>> Running pnpm install to fetch theme package…"))
					if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}, p); err != nil {
						p.Send(logLineMsg(fmt.Sprintf(">>> Theme pnpm install failed: %v", err)))
					}
				}
				p.Send(logLineMsg(">>> Resolving theme dependencies…"))
				deps, err := getThemeDependencies(m.workspaceRoot, shortName)
				if err == nil {
					for _, dep := range deps {
						installed := false
						if m.ctx != nil {
							for _, d := range m.ctx.Deps {
								if d == dep {
									installed = true
									break
								}
							}
						}
						if !installed {
							if _, exists := adds[dep]; exists {
								installed = true
							}
						}

						if !installed {
							depShort := dep
							if idx := strings.LastIndex(dep, "/"); idx >= 0 {
								depShort = dep[idx+1:]
							}

							p.Send(logLineMsg(fmt.Sprintf(">>> Theme dependency detected: %s", depShort)))
							p.Send(promptThemeDepMsg{
								DepName:   dep,
								ShortName: depShort,
							})

							decision := <-m.themeDepResolveChan
							if decision.install {
								p.Send(logLineMsg(fmt.Sprintf(">>> Installing theme dependency: %s via %s…", depShort, decision.method)))
								depArgs := []string{desktopJs, "add", depShort}
								if decision.method == "npm" {
									depArgs = append(depArgs, "--npm")
								} else if decision.method == "local" {
									depArgs = append(depArgs, "--dev")
								} else {
									depOwner := "owdproject"
									if m.ctx != nil && m.ctx.Settings.GithubUser != nil && *m.ctx.Settings.GithubUser != "" {
										depOwner = *m.ctx.Settings.GithubUser
									}

									var fromVal string
									if decision.method == "git-ssh" {
										fromVal = fmt.Sprintf("git@github.com:%s/%s.git", depOwner, depShort)
									} else {
										fromVal = fmt.Sprintf("https://github.com/%s/%s.git", depOwner, depShort)
									}
									depArgs = append(depArgs, "--from", fromVal)
								}
								if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "node", depArgs, p); err != nil {
									p.Send(logLineMsg(fmt.Sprintf(">>> Dependency %s failed: %v", depShort, err)))
								}
							} else {
								p.Send(logLineMsg(fmt.Sprintf(">>> Skipped installing theme dependency: %s", depShort)))
							}
						}
					}
				}
			}
		}

		// 2. Run pnpm install for cleanup/removal sync
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"})
		p.Send(logLineMsg(">>> Running pnpm install (syncing workspace)…"))
		if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}, p); err != nil {
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// 3. Rebuild stubs
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"})
		p.Send(logLineMsg(">>> Rebuilding stubs…"))
		if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}, p); err != nil {
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		p.Send(taskFinishedMsg{Success: true})
	}()
}

func (m *TuiModel) RunUpdatePackageTask(pkgName string, shortName string, kind string, isLocalSource bool, p *tea.Program) {
	go func() {
		p.Send(clearLogsMsg{})
		p.Send(logLineMsg(fmt.Sprintf(">>> Starting update for %s…", shortName)))
		p.Send(setupProgressMsg{Step: 1, Total: 3, Label: fmt.Sprintf("Updating %s…", shortName)})

		desktopJs := filepath.Join(m.workspaceRoot, "packages", "cli", "bin", "desktop.js")

		if isLocalSource {
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
				pkgPath := filepath.Join(m.workspaceRoot, kindDir, shortName)
				p.Send(logLineMsg(fmt.Sprintf(">>> Running git pull in %s…", pkgPath)))
				if err := runProcessAndStreamLogsSilent(pkgPath, "git", []string{"pull"}, p); err != nil {
					p.Send(logLineMsg(fmt.Sprintf(">>> Git pull failed: %v", err)))
					p.Send(taskFinishedMsg{Success: false, Err: err})
					return
				}
			}
		} else {
			p.Send(logLineMsg(fmt.Sprintf(">>> Re-installing %s from NPM to get latest version…", shortName)))
			args := []string{desktopJs, "add", shortName, "--npm"}
			if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "node", args, p); err != nil {
				p.Send(logLineMsg(fmt.Sprintf(">>> NPM update failed: %v", err)))
				p.Send(taskFinishedMsg{Success: false, Err: err})
				return
			}
		}

		// Run pnpm install
		p.Send(setupProgressMsg{Step: 2, Total: 3, Label: "Installing dependencies (pnpm install)…"})
		p.Send(logLineMsg(">>> Running pnpm install (syncing workspace)…"))
		if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}, p); err != nil {
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Rebuild stubs
		p.Send(setupProgressMsg{Step: 3, Total: 3, Label: "Rebuilding stubs (prepare:modules)…"})
		p.Send(logLineMsg(">>> Rebuilding stubs…"))
		if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}, p); err != nil {
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		p.Send(taskFinishedMsg{Success: true})
	}()
}

func (m *TuiModel) RunServeTask(p *tea.Program) {
	go func() {
		if m.ctx == nil {
			p.Send(serverStatusMsg{Running: false})
			return
		}

		_ = os.MkdirAll(m.ctx.Paths.MetaDir, 0755)

		logPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Failed to open log file: %v", err)))
			p.Send(serverStatusMsg{Running: false})
			return
		}
		defer logFile.Close()

		logFile.WriteString(">>> Starting Nuxt dev server (pnpm run dev)…\n")

		cmd := exec.Command("pnpm", "run", "dev")
		if m.ctx.Paths.IsPlayground {
			cmd.Dir = m.ctx.Paths.PackageDir
		} else {
			cmd.Dir = m.workspaceRoot
		}

		cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", m.ctx.Settings.DevPort))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		ServerCmd = cmd

		if err := cmd.Start(); err != nil {
			logFile.WriteString(fmt.Sprintf(">>> Server failed to start: %v\n", err))
			p.Send(serverStatusMsg{Running: false})
			return
		}

		// Write PID file
		pidPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.pid")
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

		p.Send(serverStatusMsg{Running: true})

		cmd.Wait()
		logFile.WriteString(">>> Dev server exited.\n")
		p.Send(serverStatusMsg{Running: false})
		ServerCmd = nil
	}()
}

func (m *TuiModel) StopServeTask(p *tea.Program) {
	go func() {
		if m.ctx == nil {
			p.Send(serverStatusMsg{Running: false})
			return
		}

		if ServerCmd != nil && ServerCmd.Process != nil {
			_ = syscall.Kill(-ServerCmd.Process.Pid, syscall.SIGKILL)
			ServerCmd = nil
		}

		pidPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.pid")
		if data, err := os.ReadFile(pidPath); err == nil {
			var pid int
			if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err == nil {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			_ = os.Remove(pidPath)
		}

		_ = exec.Command("pkill", "-9", "-f", "nuxt.mjs").Run()
		_ = exec.Command("pkill", "-9", "-f", "nuxi.mjs").Run()
		_ = exec.Command("pkill", "-9", "-f", "nuxt dev").Run()
		_ = exec.Command("pkill", "-9", "-f", "nuxi dev").Run()
		_ = exec.Command("pkill", "-9", "-f", "nx run desktop:serve").Run()

		p.Send(serverStatusMsg{Running: false})
	}()
}

func StartLogTailer(logPath string, p *tea.Program) {
	go func() {
		var lastSize int64 = 0

		// Check if file exists and get its initial size
		if info, err := os.Stat(logPath); err == nil {
			lastSize = info.Size()
			// Seek back up to 10000 bytes (~100 lines) to show initial history
			offset := lastSize - 10000
			if offset < 0 {
				offset = 0
			}

			file, err := os.Open(logPath)
			if err == nil {
				_, _ = file.Seek(offset, 0)
				reader := bufio.NewReader(file)
				if offset > 0 {
					_, _ = reader.ReadString('\n') // discard first potentially cut line
				}
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					p.Send(logLineMsg(strings.TrimRight(line, "\r\n")))
				}
				file.Close()
			}
		}

		for {
			time.Sleep(250 * time.Millisecond)

			info, err := os.Stat(logPath)
			if err != nil {
				continue
			}

			if info.Size() < lastSize {
				// File was truncated/cleared
				lastSize = info.Size()
				p.Send(clearLogsMsg{})
				continue
			}

			if info.Size() > lastSize {
				file, err := os.Open(logPath)
				if err != nil {
					continue
				}
				_, err = file.Seek(lastSize, 0)
				if err != nil {
					file.Close()
					continue
				}

				reader := bufio.NewReader(file)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					p.Send(logLineMsg(strings.TrimRight(line, "\r\n")))
				}
				lastSize = info.Size()
				file.Close()
			}
		}
	}()
}

// ─────────────────────────────────────────────
// Git & Updates Helpers
// ─────────────────────────────────────────────

func runGitStatus(dir string) (added, modified, deleted int, err error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		if strings.HasPrefix(status, "??") {
			added++
		} else if strings.Contains(status, "D") {
			deleted++
		} else if strings.Contains(status, "M") || strings.Contains(status, "A") || strings.Contains(status, "R") || strings.Contains(status, "C") {
			modified++
		}
	}
	return added, modified, deleted, nil
}

func runGitStatusForSubdir(rootDir, subdir string) (added, modified, deleted int, err error) {
	cmd := exec.Command("git", "status", "--porcelain", subdir)
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		if strings.HasPrefix(status, "??") {
			added++
		} else if strings.Contains(status, "D") {
			deleted++
		} else if strings.Contains(status, "M") || strings.Contains(status, "A") || strings.Contains(status, "R") || strings.Contains(status, "C") {
			modified++
		}
	}
	return added, modified, deleted, nil
}

func runGitBehindCheck(dir string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmdFetch := exec.CommandContext(ctx, "git", "fetch")
	cmdFetch.Dir = dir
	_ = cmdFetch.Run() // ignore fetch errors

	cmdRev := exec.Command("git", "rev-list", "--count", "HEAD..@{u}")
	cmdRev.Dir = dir
	out, err := cmdRev.Output()
	if err != nil {
		// Try with origin/branch
		cmdBranch := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		cmdBranch.Dir = dir
		branchOut, errBranch := cmdBranch.Output()
		if errBranch != nil {
			return 0, errBranch
		}
		branch := strings.TrimSpace(string(branchOut))
		if branch == "" || branch == "HEAD" {
			return 0, err
		}
		cmdRev = exec.Command("git", "rev-list", "--count", fmt.Sprintf("HEAD..origin/%s", branch))
		cmdRev.Dir = dir
		out, err = cmdRev.Output()
		if err != nil {
			return 0, err
		}
	}

	var behind int
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &behind)
	return behind, err
}

func fetchLatestNpmVersion(pkgName string) (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://registry.npmjs.org/%s/latest", url.PathEscape(pkgName)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var data struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	return data.Version, nil
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

func (m TuiModel) checkForUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		gitChangesMap := make(map[string]GitChanges)
		updatesMap := make(map[string]UpdateInfo)

		if m.catalog == nil {
			return updatesLoadedMsg{GitChanges: gitChangesMap, Updates: updatesMap}
		}

		type result struct {
			shortName string
			changes   *GitChanges
			update    *UpdateInfo
		}

		resultsChan := make(chan result, len(m.catalog.Entries))
		sem := make(chan struct{}, 10) // Limit concurrency

		for _, e := range m.catalog.Entries {
			sem <- struct{}{}
			go func(entry bridge.CatalogEntry) {
				defer func() { <-sem }()

				res := result{shortName: entry.ShortName}

				// 1. Check local changes & upstream behind counts
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

				if kindDir != "" {
					pkgPath := filepath.Join(m.workspaceRoot, kindDir, short)
					gitDir := filepath.Join(pkgPath, ".git")
					if _, err := os.Stat(gitDir); err == nil {
						added, modified, deleted, err := runGitStatus(pkgPath)
						if err == nil && (added > 0 || modified > 0 || deleted > 0) {
							res.changes = &GitChanges{Added: added, Modified: modified, Deleted: deleted}
						}

						behind, err := runGitBehindCheck(pkgPath)
						if err == nil && behind > 0 {
							res.update = &UpdateInfo{LocalGit: true, BehindCount: behind}
						}
					} else {
						// Fallback: Check if the monorepo itself tracks changes for this directory
						parentGitDir := filepath.Join(m.workspaceRoot, ".git")
						if _, err := os.Stat(parentGitDir); err == nil {
							relPath := filepath.Join(kindDir, short)
							added, modified, deleted, err := runGitStatusForSubdir(m.workspaceRoot, relPath)
							if err == nil && (added > 0 || modified > 0 || deleted > 0) {
								res.changes = &GitChanges{Added: added, Modified: modified, Deleted: deleted}
							}
						}
					}
				}

				// 2. Check NPM updates
				if entry.Installed && !entry.LocalSource {
					localVer := getLocalVersion(m.workspaceRoot, entry)
					if localVer != "" {
						latestVer, err := fetchLatestNpmVersion(entry.Name)
						if err == nil && semverCompare(localVer, latestVer) < 0 {
							res.update = &UpdateInfo{Npm: true}
						}
					}
				}

				resultsChan <- res
			}(e)
		}

		for i := 0; i < len(m.catalog.Entries); i++ {
			res := <-resultsChan
			if res.changes != nil {
				gitChangesMap[res.shortName] = *res.changes
			}
			if res.update != nil {
				updatesMap[res.shortName] = *res.update
			}
		}

		return updatesLoadedMsg{GitChanges: gitChangesMap, Updates: updatesMap}
	}
}

// ─────────────────────────────────────────────
// Theme Dependencies & Package Metadata Helpers
// ─────────────────────────────────────────────

// isLocallyAvailable checks if a package (by short name) exists as a local
// workspace directory under apps/, themes/, or packages/.
func isLocallyAvailable(workspaceRoot, shortName string) bool {
	candidates := []string{
		filepath.Join(workspaceRoot, "apps", shortName),
		filepath.Join(workspaceRoot, "themes", shortName),
		filepath.Join(workspaceRoot, "packages", shortName),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func getThemeDependencies(workspaceRoot, themeShortName string) ([]string, error) {
	var pathsToCheck []string
	pathsToCheck = append(pathsToCheck, filepath.Join(workspaceRoot, "themes", themeShortName, "package.json"))
	pathsToCheck = append(pathsToCheck, filepath.Join(workspaceRoot, "node_modules", "@owdproject", themeShortName, "package.json"))
	pathsToCheck = append(pathsToCheck, filepath.Join(workspaceRoot, "desktop", "node_modules", "@owdproject", themeShortName, "package.json"))

	var data []byte
	var err error
	for _, p := range pathsToCheck {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	var deps []string
	seen := make(map[string]bool)
	for name := range pkg.Dependencies {
		if strings.HasPrefix(name, "@owdproject/") && !seen[name] {
			deps = append(deps, name)
			seen[name] = true
		}
	}
	for name := range pkg.DevDependencies {
		if strings.HasPrefix(name, "@owdproject/") && !seen[name] {
			deps = append(deps, name)
			seen[name] = true
		}
	}
	return deps, nil
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

// keep rand used (for future sparkline noise simulation)
var _ = rand.Float64

func (m *TuiModel) RunWipeWorkspaceTask(p *tea.Program) {
	go func() {
		p.Send(clearLogsMsg{})
		p.Send(logLineMsg(">>> Starting workspace reset task…"))

		totalSteps := 7
		step := 0

		// Step 1: Clean apps/
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning apps directory…"})
		p.Send(logLineMsg(">>> Cleaning apps/ directory…"))
		if err := wipeDirectory(filepath.Join(m.workspaceRoot, "apps"), nil); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error cleaning apps: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Step 2: Clean themes/
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning themes directory…"})
		p.Send(logLineMsg(">>> Cleaning themes/ directory…"))
		if err := wipeDirectory(filepath.Join(m.workspaceRoot, "themes"), nil); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error cleaning themes: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Step 3: Clean packages/ (preserving core, cli, nx)
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning packages directory…"})
		p.Send(logLineMsg(">>> Cleaning packages/ directory (preserving core, cli, nx)…"))
		preservePkgs := map[string]bool{"core": true, "cli": true, "nx": true}
		if err := wipeDirectory(filepath.Join(m.workspaceRoot, "packages"), preservePkgs); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error cleaning packages: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Step 4: Reset desktop.config.ts
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting desktop config…"})
		p.Send(logLineMsg(">>> Resetting desktop/desktop.config.ts to default…"))
		defaultConfig := `import { defineDesktopConfig } from '@owdproject/core'

export default defineDesktopConfig({
  theme: '',
  apps: [],
  modules: [],
})
`
		configPath := filepath.Join(m.workspaceRoot, "desktop", "desktop.config.ts")
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error resetting desktop config: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Step 5: Reset package.json
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting package dependencies…"})
		p.Send(logLineMsg(">>> Resetting desktop/package.json dependencies to core only…"))
		defaultPackageJson := `{
  "name": "@owdproject/client",
  "private": true,
  "nx": {
    "name": "desktop"
  },
  "scripts": {
    "build": "nuxt generate",
    "dev": "nuxt dev --host",
    "generate": "nuxt generate --dev",
    "postinstall": "nuxt prepare",
    "preview": "nuxt preview",
    "desktop": "desktop"
  },
  "dependencies": {
    "@owdproject/core": "workspace:*"
  }
}
`
		pkgJsonPath := filepath.Join(m.workspaceRoot, "desktop", "package.json")
		if err := os.WriteFile(pkgJsonPath, []byte(defaultPackageJson), 0644); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error resetting package.json: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Step 6: Run pnpm install
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"})
		p.Send(logLineMsg(">>> Running pnpm install (syncing workspace)…"))
		if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}, p); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error running pnpm install: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		// Step 7: Run prepare:modules
		step++
		p.Send(setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"})
		p.Send(logLineMsg(">>> Rebuilding stubs…"))
		if err := runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}, p); err != nil {
			p.Send(logLineMsg(fmt.Sprintf(">>> Error running prepare:modules: %v", err)))
			p.Send(taskFinishedMsg{Success: false, Err: err})
			return
		}

		p.Send(taskFinishedMsg{Success: true})
	}()
}

func (m *TuiModel) runWipeWorkspaceCmd() tea.Cmd {
	return func() tea.Msg {
		if Program != nil {
			m.RunWipeWorkspaceTask(Program)
		}
		return nil
	}
}

func wipeDirectory(dir string, preserve map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			name := entry.Name()
			if preserve != nil && preserve[name] {
				continue
			}
			err := os.RemoveAll(filepath.Join(dir, name))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
