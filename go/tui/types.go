package tui

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"owd-cli/bridge"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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

// localChangesMsg is the result of the fast local-only git status check (no network).
type localChangesMsg struct {
	GitChanges   map[string]GitChanges
	LocalGitDirs map[string]bool // shortName -> has .git folder
}

// remoteUpdatesMsg is the result of the slow network check (git behind + npm versions).
type remoteUpdatesMsg struct {
	Updates map[string]UpdateInfo
	Err     error
}

type setupProgressMsg struct {
	Step  int
	Total int
	Label string
}

type engineNeedsPromptMsg struct {
	Item WorkItem
}

type engineQueueUpdatedMsg struct {
	Queue WorkQueue
}

type enginePhaseMsg struct {
	Phase EnginePhase
}

type workspaceGitStatusMsg struct {
	Branch  string
	Changes string
}

type InstallMethod struct {
	Name  string
	Label string
	Desc  string
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
	PromptSetupProgress
	PromptResolveDependency
	PromptWipeWorkspaceConfirm
)

type ActiveTask int

const (
	TaskNone ActiveTask = iota
	TaskSetup
	TaskWipe
	TaskServe
)

// RuntimeState holds references to mutable OS/runtime resources.
type RuntimeState struct {
	serverCmd    *exec.Cmd
	serverMu     sync.Mutex
	logCancel    context.CancelFunc
	msgChan      chan tea.Msg
	engineResume chan string
}

// ─────────────────────────────────────────────
// Menu constants and actions
// ─────────────────────────────────────────────

var ManagePackageActions = []string{
	"Update package",
	"Switch to NPM registry",
	"Switch to Git (HTTPS)",
	"Switch to Git (SSH)",
	"Force Re-download (wipe changes)",
	"Back to catalog",
}

const (
	SettingsFieldCount   = 5 // Install Mode, Catalog Sort, Trusted Orgs, GitHub User, Reset Workspace
	SettingsActionCount  = 2 // Save, Cancel
	SettingsTotalOptions = SettingsFieldCount + SettingsActionCount
)

// ─────────────────────────────────────────────
// Model
// ─────────────────────────────────────────────



type TuiModel struct {
	workspaceRoot string
	ctx           *bridge.WorkspaceContext
	catalog       *bridge.CatalogResponse
	runtime       *RuntimeState

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
	localGitDirs  map[string]bool // shortName -> has own .git folder

	settingsSel         int
	settingsInstallMode string
	settingsCatalogSort string
	settingsOrgsInput   textinput.Model
	settingsUserInput   textinput.Model

	// Server status
	serverRunning   bool
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

	// Wizard engine
	workQueue   *WorkQueue
	enginePhase EnginePhase
	promptItem  *WorkItem

	startServerAfterSetup bool
	setupStep             int
	setupTotalSteps       int
	setupLabel            string
	workspaceBranch       string
	workspaceChanges      string
	clientStars           int
}
