package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"owd-cli/bridge"

	tea "github.com/charmbracelet/bubbletea"
)

// ─────────────────────────────────────────────
// Background tasks
// ─────────────────────────────────────────────

type ProcessResult struct {
	ExitCode int
	Err      error
}

func (r *RuntimeState) runProcessStream(cwd, command string, args []string) ProcessResult {
	cmdLine := command + " " + strings.Join(args, " ")
	r.msgChan <- logLineMsg("ℹ " + cmdLine)

	cmd := exec.Command(command, args...)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf("✖ Failed to pipe stdout: %v", err))
		return ProcessResult{ExitCode: 1, Err: err}
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf("✖ Failed to start process: %v", err))
		return ProcessResult{ExitCode: 1, Err: err}
	}

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		r.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
	}

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		r.msgChan <- logLineMsg(fmt.Sprintf("✖ Command failed (exit %d): %v", exitCode, err))
	} else {
		r.msgChan <- logLineMsg(fmt.Sprintf("✔ Command completed (exit 0)"))
	}
	return ProcessResult{ExitCode: exitCode, Err: err}
}

func (r *RuntimeState) runProcessAndStreamLogs(root, command string, args []string) {
	result := r.runProcessStream(root, command, args)
	r.msgChan <- taskFinishedMsg{Success: result.Err == nil, Err: result.Err}
}

func (r *RuntimeState) runProcessAndStreamLogsSilent(root, command string, args []string) error {
	result := r.runProcessStream(root, command, args)
	return result.Err
}

func (m *TuiModel) RunSetupTask(adds map[string]string) {
	workspaceRoot := m.workspaceRoot
	msgChan := m.runtime.msgChan
	runtime := m.runtime

	var catalogEntries []bridge.CatalogEntry
	if m.catalog != nil {
		catalogEntries = make([]bridge.CatalogEntry, len(m.catalog.Entries))
		copy(catalogEntries, m.catalog.Entries)
	}

	currentBatch := make(map[string]string)
	for k, v := range adds {
		currentBatch[k] = v
	}

	cloneCount := 0
	for _, method := range adds {
		if method == "git-https" || method == "git-ssh" {
			cloneCount++
		}
	}
	totalSteps := cloneCount + 2
	step := 0

	go func() {
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Initializing…"}

		// clonedDirs tracks successfully cloned target directories for dep scanning in this batch
		type clonedInfo struct {
			shortName string
			targetDir string
			pkgName   string
		}
		var clonedDirs []clonedInfo

		// ── Phase 1: Clone each package in the current batch ──────────────────
		for pkgName, method := range currentBatch {
			shortName := pkgName
			if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
				shortName = pkgName[idx+1:]
			}

			if method == "npm" {
				msgChan <- logLineMsg(fmt.Sprintf("ℹ %s will be installed via npm (pnpm install)", shortName))
				continue
			}
			if method == "local" {
				msgChan <- logLineMsg(fmt.Sprintf("ℹ %s already available as local workspace package", shortName))
				continue
			}

			// Determine target directory
			kindDir := kindDirForEntry(pkgName, shortName, catalogEntries)
			targetDir := filepath.Join(workspaceRoot, kindDir, shortName)

			// Skip if already cloned
			if _, err := os.Stat(filepath.Join(targetDir, "package.json")); err == nil {
				msgChan <- logLineMsg(fmt.Sprintf("ℹ %s already cloned — skipping", shortName))
				clonedDirs = append(clonedDirs, clonedInfo{shortName, targetDir, pkgName})
				continue
			}

			step++
			msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: fmt.Sprintf("Cloning %s…", shortName)}

			// Resolve git URL
			owner := resolveOwner(pkgName, catalogEntries)
			var gitURL string
			if method == "git-ssh" {
				gitURL = fmt.Sprintf("git@github.com:%s/%s.git", owner, shortName)
			} else {
				gitURL = fmt.Sprintf("https://github.com/%s/%s.git", owner, shortName)
			}

			msgChan <- logLineMsg(fmt.Sprintf("ℹ Cloning %s from %s", shortName, gitURL))
			if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "git", []string{"clone", gitURL, targetDir}); err != nil {
				msgChan <- logLineMsg(fmt.Sprintf("✗ Clone failed for %s: %v", shortName, err))
				msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
			msgChan <- logLineMsg(fmt.Sprintf("✓ %s cloned", shortName))
			clonedDirs = append(clonedDirs, clonedInfo{shortName, targetDir, pkgName})
		}

		// ── Phase 1.5: Recursively scan and clone missing workspace dependencies ──
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Scanning package dependencies…"}

		logFunc := func(line string) { msgChan <- logLineMsg(line) }
		if err := ResolveAndCloneMissingDependencies(workspaceRoot, catalogEntries, runtime, logFunc, msgChan, &step, &totalSteps); err != nil {
			msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// ── Phase 2: pnpm install ─────────────────────────────────────────────
		step++
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"}
		msgChan <- logLineMsg("ℹ Running pnpm install (syncing workspace)…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install"}); err != nil {
			msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// ── Phase 3: Rebuild stubs ────────────────────────────────────────────
		step++
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"}
		msgChan <- logLineMsg("ℹ Rebuilding stubs…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		time.Sleep(500 * time.Millisecond)
		msgChan <- taskFinishedMsg{Success: true}
	}()
}

// kindDirForEntry resolves the workspace subdirectory for a package.
// Uses catalog entries first, falls back to shortname prefix.
func kindDirForEntry(pkgName, shortName string, entries []bridge.CatalogEntry) string {
	for _, e := range entries {
		if e.Name == pkgName {
			switch e.Kind {
			case "app":
				return "apps"
			case "module":
				return "packages"
			case "theme":
				return "themes"
			}
		}
	}
	return kindDirForShortName(shortName)
}

// kindDirForShortName infers the workspace subdirectory from the package short name.
func kindDirForShortName(shortName string) string {
	switch {
	case strings.HasPrefix(shortName, "app-"):
		return "apps"
	case strings.HasPrefix(shortName, "theme-"):
		return "themes"
	default:
		return "packages" // module-*, kit-*, and anything else
	}
}

// resolveOwner returns the GitHub owner to use for cloning.
// Priority: fork owner (if fork is confirmed to exist) > catalog org > "owdproject".
func resolveOwner(pkgName string, entries []bridge.CatalogEntry) string {
	var entry *bridge.CatalogEntry
	for i := range entries {
		if entries[i].Name == pkgName {
			entry = &entries[i]
			break
		}
	}
	if entry != nil && entry.SourcesMeta != nil && entry.SourcesMeta.Github.Fork != nil && entry.SourcesMeta.Github.Fork.Exists && entry.SourcesMeta.Github.Fork.IsFork {
		return entry.SourcesMeta.Github.Fork.Owner
	}
	if entry != nil && entry.Org != "" && entry.Org != "workspace" {
		return entry.Org
	}
	return "owdproject"
}


func (m *TuiModel) RunUpdatePackageTask(pkgName string, shortName string, kind string, isLocalSource bool) {
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime

	go func() {
		runtime.msgChan <- clearLogsMsg{}
		runtime.msgChan <- logLineMsg(fmt.Sprintf("ℹ Starting update for %s…", shortName))
		runtime.msgChan <- setupProgressMsg{Step: 1, Total: 3, Label: fmt.Sprintf("Updating %s…", shortName)}

		desktopJs := filepath.Join(workspaceRoot, "packages", "cli", "bin", "desktop.js")

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
				pkgPath := filepath.Join(workspaceRoot, kindDir, shortName)
				runtime.msgChan <- logLineMsg(fmt.Sprintf("ℹ Running git pull in %s…", pkgPath))
				if err := runtime.runProcessAndStreamLogsSilent(pkgPath, "git", []string{"pull"}); err != nil {
					runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Git pull failed: %v", err))
					runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
					return
				}
			}
		} else {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("ℹ Re-installing %s from NPM to get latest version…", shortName))
			args := []string{desktopJs, "add", shortName, "--npm"}
			if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "node", args); err != nil {
				runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ NPM update failed: %v", err))
				runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
		}

		// Run pnpm install
		runtime.msgChan <- setupProgressMsg{Step: 2, Total: 3, Label: "Installing dependencies (pnpm install)…"}
		runtime.msgChan <- logLineMsg("ℹ Running pnpm install (syncing workspace)…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install"}); err != nil {
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Rebuild stubs
		runtime.msgChan <- setupProgressMsg{Step: 3, Total: 3, Label: "Rebuilding stubs (prepare:modules)…"}
		runtime.msgChan <- logLineMsg("ℹ Rebuilding stubs…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		runtime.msgChan <- taskFinishedMsg{Success: true}
	}()
}

func (m *TuiModel) RunServeTask() {
	var metaDir string
	var isPlayground bool
	var packageDir string
	var devPort int

	if m.ctx != nil {
		metaDir = m.ctx.Paths.MetaDir
		isPlayground = m.ctx.Paths.IsPlayground
		packageDir = m.ctx.Paths.PackageDir
		devPort = m.ctx.Settings.DevPort
	}
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime

	go func() {
		if metaDir == "" {
			runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}

		_ = os.MkdirAll(metaDir, 0755)

		logPath := filepath.Join(metaDir, "dev.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Failed to open log file: %v", err))
			runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}
		defer logFile.Close()

		logFile.WriteString("ℹ Starting Nuxt dev server (pnpm run dev)…\n")

		cmd := exec.Command("pnpm", "run", "dev")
		if isPlayground {
			cmd.Dir = packageDir
		} else {
			cmd.Dir = workspaceRoot
		}

		if devPort == 0 {
			devPort = 3000
		}
		cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", devPort))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		runtime.serverMu.Lock()
		runtime.serverCmd = cmd
		runtime.serverMu.Unlock()

		if err := cmd.Start(); err != nil {
			logFile.WriteString(fmt.Sprintf("✗ Server failed to start: %v\n", err))
			runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}

		// Write PID file
		pidPath := filepath.Join(metaDir, "dev.pid")
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

		runtime.msgChan <- serverStatusMsg{Running: true}

		cmd.Wait()
		logFile.WriteString("ℹ Dev server exited.\n")
		runtime.msgChan <- serverStatusMsg{Running: false}

		runtime.serverMu.Lock()
		runtime.serverCmd = nil
		runtime.serverMu.Unlock()
	}()
}

func stopServeTaskSync(workspaceRoot, metaDir string, runtime *RuntimeState) {
	if metaDir == "" {
		runtime.msgChan <- serverStatusMsg{Running: false}
		return
	}

	runtime.serverMu.Lock()
	cmd := runtime.serverCmd
	runtime.serverCmd = nil
	runtime.serverMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	pidPath := filepath.Join(metaDir, "dev.pid")
	if data, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err == nil && pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		_ = os.Remove(pidPath)
	}

	// Safe, scoped process cleanup targeting processes for this workspace
	killWorkspaceProcesses(workspaceRoot)

	runtime.msgChan <- serverStatusMsg{Running: false}
}

func (m *TuiModel) StopServeTask() {
	var metaDir string
	if m.ctx != nil {
		metaDir = m.ctx.Paths.MetaDir
	}
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime

	go func() {
		stopServeTaskSync(workspaceRoot, metaDir, runtime)
	}()
}

func (m *TuiModel) StopLogTailer() {
	m.runtime.serverMu.Lock()
	defer m.runtime.serverMu.Unlock()
	if m.runtime.logCancel != nil {
		m.runtime.logCancel()
		m.runtime.logCancel = nil
	}
}

func (m *TuiModel) StartLogTailer(logPath string) {
	m.StopLogTailer()

	m.runtime.serverMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	m.runtime.logCancel = cancel
	m.runtime.serverMu.Unlock()

	runtime := m.runtime

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
					select {
					case <-ctx.Done():
						file.Close()
						return
					default:
					}
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					runtime.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
				}
				file.Close()
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			time.Sleep(250 * time.Millisecond)

			info, err := os.Stat(logPath)
			if err != nil {
				continue
			}

			if info.Size() < lastSize {
				// File was truncated/cleared
				lastSize = info.Size()
				runtime.msgChan <- clearLogsMsg{}
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
					select {
					case <-ctx.Done():
						file.Close()
						return
					default:
					}
					line, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					runtime.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
				}
				lastSize = info.Size()
				file.Close()
			}
		}
	}()
}

func (m *TuiModel) RunWipeWorkspaceTask() {
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime

	go func() {
		runtime.msgChan <- clearLogsMsg{}
		runtime.msgChan <- logLineMsg("ℹ Starting workspace reset task…")

		totalSteps := 7
		step := 0

		// Step 1: Clean apps/
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning apps directory…"}
		runtime.msgChan <- logLineMsg("ℹ Cleaning apps/ directory…")
		if err := wipeDirectory(filepath.Join(workspaceRoot, "apps"), nil); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error cleaning apps: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 2: Clean themes/
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning themes directory…"}
		runtime.msgChan <- logLineMsg("ℹ Cleaning themes/ directory…")
		if err := wipeDirectory(filepath.Join(workspaceRoot, "themes"), nil); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error cleaning themes: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 3: Clean packages/ (preserving core, cli, nx)
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning packages directory…"}
		runtime.msgChan <- logLineMsg("ℹ Cleaning packages/ directory (preserving core, cli, nx)…")
		preservePkgs := map[string]bool{"core": true, "cli": true, "nx": true}
		if err := wipeDirectory(filepath.Join(workspaceRoot, "packages"), preservePkgs); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error cleaning packages: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 4: Reset desktop.config.ts
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting desktop config…"}
		runtime.msgChan <- logLineMsg("ℹ Resetting desktop/desktop.config.ts to default…")
		defaultConfig := `import { defineDesktopConfig } from '@owdproject/core'

export default defineDesktopConfig({
  theme: '',
  apps: [],
  modules: [],
})
`
		configPath := filepath.Join(workspaceRoot, "desktop", "desktop.config.ts")
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error resetting desktop config: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 5: Reset package.json
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting package dependencies…"}
		runtime.msgChan <- logLineMsg("ℹ Resetting desktop/package.json dependencies to core only…")
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
		pkgJsonPath := filepath.Join(workspaceRoot, "desktop", "package.json")
		if err := os.WriteFile(pkgJsonPath, []byte(defaultPackageJson), 0644); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error resetting package.json: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 6: Run pnpm install
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"}
		runtime.msgChan <- logLineMsg("ℹ Running pnpm install (syncing workspace)…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install"}); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error running pnpm install: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 7: Run prepare:modules
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"}
		runtime.msgChan <- logLineMsg("ℹ Rebuilding stubs…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf("✗ Error running prepare:modules: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		runtime.msgChan <- taskFinishedMsg{Success: true}
	}()
}

// killWorkspaceProcesses kills any target processes (node, nuxt, etc.) that are associated with the workspaceRoot
func killWorkspaceProcesses(workspaceRoot string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	myPid := os.Getpid()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		var pid int
		if _, err := fmt.Sscanf(name, "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		if pid == myPid {
			continue
		}

		cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
		data, err := os.ReadFile(cmdlinePath)
		if err != nil {
			continue
		}

		args := strings.Split(string(data), "\x00")
		if len(args) == 0 || args[0] == "" {
			continue
		}

		hasWorkspace := false
		for _, arg := range args {
			if strings.Contains(arg, workspaceRoot) {
				hasWorkspace = true
				break
			}
		}

		if !hasWorkspace {
			continue
		}

		exe := args[0]
		isTarget := false
		targets := []string{"node", "nuxt", "nuxi", "pnpm", "vite", "nitro", "next"}
		for _, t := range targets {
			if strings.Contains(strings.ToLower(exe), t) {
				isTarget = true
				break
			}
		}

		if !isTarget {
			for _, arg := range args {
				for _, t := range targets {
					if strings.Contains(strings.ToLower(arg), t) {
						isTarget = true
						break
					}
				}
				if isTarget {
					break
				}
			}
		}

		if isTarget {
			// Kill process group, then the process itself
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// ResolveAndCloneMissingDependencies recursively scans the workspace for any missing workspace:* dependencies and clones them.
func ResolveAndCloneMissingDependencies(
	workspaceRoot string,
	catalogEntries []bridge.CatalogEntry,
	runtime *RuntimeState,
	log func(string),
	msgChan chan<- tea.Msg,
	step *int,
	totalSteps *int,
) error {
	inWorkspace := map[string]bool{}

	scanWorkspace := func() ([]string, error) {
		localFolders := []string{filepath.Join(workspaceRoot, "desktop")}
		dirs := []string{"apps", "packages", "themes"}
		for _, d := range dirs {
			p := filepath.Join(workspaceRoot, d)
			entries, err := os.ReadDir(p)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					localFolders = append(localFolders, filepath.Join(p, entry.Name()))
				}
			}
		}

		// Clean/reset inWorkspace map
		for k := range inWorkspace {
			delete(inWorkspace, k)
		}

		for _, folder := range localFolders {
			data, err := os.ReadFile(filepath.Join(folder, "package.json"))
			if err != nil {
				continue
			}
			var pkg struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(data, &pkg) == nil && pkg.Name != "" {
				inWorkspace[pkg.Name] = true
			}
		}

		var missing []string
		for _, folder := range localFolders {
			data, err := os.ReadFile(filepath.Join(folder, "package.json"))
			if err != nil {
				continue
			}
			var pkg struct {
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
			}
			if json.Unmarshal(data, &pkg) != nil {
				continue
			}

			allDeps := make(map[string]string)
			for k, v := range pkg.Dependencies {
				allDeps[k] = v
			}
			for k, v := range pkg.DevDependencies {
				allDeps[k] = v
			}

			for depName, depVer := range allDeps {
				if !strings.HasPrefix(depName, "@owdproject/") {
					continue
				}
				if !strings.HasPrefix(depVer, "workspace:") {
					continue
				}
				if !inWorkspace[depName] {
					alreadyAdded := false
					for _, m := range missing {
						if m == depName {
							alreadyAdded = true
							break
						}
					}
					if !alreadyAdded {
						missing = append(missing, depName)
					}
				}
			}
		}
		return missing, nil
	}

	for {
		missing, err := scanWorkspace()
		if err != nil || len(missing) == 0 {
			break
		}

		depName := missing[0]
		depShort := depName[strings.LastIndex(depName, "/")+1:]
		depKindDir := kindDirForShortName(depShort)
		depTarget := filepath.Join(workspaceRoot, depKindDir, depShort)

		owner := resolveOwner(depName, catalogEntries)
		gitURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, depShort)

		if step != nil && totalSteps != nil && msgChan != nil {
			*totalSteps++
			*step++
			msgChan <- setupProgressMsg{Step: *step, Total: *totalSteps, Label: fmt.Sprintf("Cloning dep %s…", depShort)}
		}
		log(fmt.Sprintf("ℹ Auto-cloning missing workspace dependency %s from %s", depShort, gitURL))

		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "git", []string{"clone", gitURL, depTarget}); err != nil {
			log(fmt.Sprintf("✗ Auto-clone failed for %s: %v", depShort, err))
			return err
		}
		log(fmt.Sprintf("✓ %s auto-cloned", depShort))
		inWorkspace[depName] = true
	}

	return nil
}

