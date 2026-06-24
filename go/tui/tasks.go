package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"owd-cli/bridge"
)

// ─────────────────────────────────────────────
// Background tasks
// ─────────────────────────────────────────────

func (r *RuntimeState) runProcessAndStreamLogs(root, command string, args []string) {
	cmd := exec.Command(command, args...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf(">>> Failed to pipe stdout: %v", err))
		r.msgChan <- taskFinishedMsg{Success: false, Err: err}
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf(">>> Failed to start process: %v", err))
		r.msgChan <- taskFinishedMsg{Success: false, Err: err}
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
		r.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
	}

	err = cmd.Wait()
	if err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf(">>> Command failed: %v", err))
	}
	r.msgChan <- taskFinishedMsg{Success: err == nil, Err: err}
}

func (r *RuntimeState) runProcessAndStreamLogsSilent(root, command string, args []string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf(">>> Failed to pipe stdout: %v", err))
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf(">>> Failed to start process: %v", err))
		return err
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
	if err != nil {
		r.msgChan <- logLineMsg(fmt.Sprintf(">>> Command failed: %v", err))
	}
	return err
}

func (m *TuiModel) RunSetupTask(adds map[string]string) {
	workspaceRoot := m.workspaceRoot
	msgChan := m.runtime.msgChan
	runtime := m.runtime

	var githubUser string
	if m.ctx != nil && m.ctx.Settings.GithubUser != nil {
		githubUser = *m.ctx.Settings.GithubUser
	}

	var catalogEntries []bridge.CatalogEntry
	if m.catalog != nil {
		catalogEntries = make([]bridge.CatalogEntry, len(m.catalog.Entries))
		copy(catalogEntries, m.catalog.Entries)
	}

	go func() {
		desktopJs := filepath.Join(workspaceRoot, "packages", "cli", "bin", "desktop.js")

		totalSteps := len(adds) + 2
		step := 0
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Initializing setup task…"}

		// 1. Process all additions
		for pkgName, method := range adds {
			step++
			shortName := pkgName
			if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
				shortName = pkgName[idx+1:]
			}
			msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: fmt.Sprintf("Installing %s via %s…", shortName, method)}

			args := []string{desktopJs, "add", shortName}
			if method == "npm" {
				args = append(args, "--npm")
			} else if method == "local" {
				args = append(args, "--dev")
			} else {
				owner := "owdproject"
				if githubUser != "" {
					owner = githubUser
				} else {
					for _, e := range catalogEntries {
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

			msgChan <- logLineMsg(fmt.Sprintf(">>> Executing: node desktop.js add %s via %s", shortName, method))
			if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "node", args); err != nil {
				msgChan <- logLineMsg(fmt.Sprintf(">>> Add %s failed: %v", shortName, err))
				msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
		}

		// 2. Run pnpm install for cleanup/removal sync
		step++
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"}
		msgChan <- logLineMsg(">>> Running pnpm install (syncing workspace)…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install"}); err != nil {
			msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// 3. Rebuild stubs
		step++
		msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"}
		msgChan <- logLineMsg(">>> Rebuilding stubs…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Small pause so the user sees the progress bar reach 100% before it closes
		time.Sleep(500 * time.Millisecond)
		msgChan <- taskFinishedMsg{Success: true}
	}()
}

func (m *TuiModel) RunUpdatePackageTask(pkgName string, shortName string, kind string, isLocalSource bool) {
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime

	go func() {
		runtime.msgChan <- clearLogsMsg{}
		runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Starting update for %s…", shortName))
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
				runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Running git pull in %s…", pkgPath))
				if err := runtime.runProcessAndStreamLogsSilent(pkgPath, "git", []string{"pull"}); err != nil {
					runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Git pull failed: %v", err))
					runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
					return
				}
			}
		} else {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Re-installing %s from NPM to get latest version…", shortName))
			args := []string{desktopJs, "add", shortName, "--npm"}
			if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "node", args); err != nil {
				runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> NPM update failed: %v", err))
				runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
		}

		// Run pnpm install
		runtime.msgChan <- setupProgressMsg{Step: 2, Total: 3, Label: "Installing dependencies (pnpm install)…"}
		runtime.msgChan <- logLineMsg(">>> Running pnpm install (syncing workspace)…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install"}); err != nil {
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Rebuild stubs
		runtime.msgChan <- setupProgressMsg{Step: 3, Total: 3, Label: "Rebuilding stubs (prepare:modules)…"}
		runtime.msgChan <- logLineMsg(">>> Rebuilding stubs…")
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
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Failed to open log file: %v", err))
			runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}
		defer logFile.Close()

		logFile.WriteString(">>> Starting Nuxt dev server (pnpm run dev)…\n")

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
			logFile.WriteString(fmt.Sprintf(">>> Server failed to start: %v\n", err))
			runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}

		// Write PID file
		pidPath := filepath.Join(metaDir, "dev.pid")
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

		runtime.msgChan <- serverStatusMsg{Running: true}

		cmd.Wait()
		logFile.WriteString(">>> Dev server exited.\n")
		runtime.msgChan <- serverStatusMsg{Running: false}

		runtime.serverMu.Lock()
		runtime.serverCmd = nil
		runtime.serverMu.Unlock()
	}()
}

func (m *TuiModel) StopServeTask() {
	var metaDir string
	if m.ctx != nil {
		metaDir = m.ctx.Paths.MetaDir
	}
	workspaceRoot := m.workspaceRoot
	runtime := m.runtime

	go func() {
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
		runtime.msgChan <- logLineMsg(">>> Starting workspace reset task…")

		totalSteps := 7
		step := 0

		// Step 1: Clean apps/
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning apps directory…"}
		runtime.msgChan <- logLineMsg(">>> Cleaning apps/ directory…")
		if err := wipeDirectory(filepath.Join(workspaceRoot, "apps"), nil); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error cleaning apps: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 2: Clean themes/
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning themes directory…"}
		runtime.msgChan <- logLineMsg(">>> Cleaning themes/ directory…")
		if err := wipeDirectory(filepath.Join(workspaceRoot, "themes"), nil); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error cleaning themes: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 3: Clean packages/ (preserving core, cli, nx)
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning packages directory…"}
		runtime.msgChan <- logLineMsg(">>> Cleaning packages/ directory (preserving core, cli, nx)…")
		preservePkgs := map[string]bool{"core": true, "cli": true, "nx": true}
		if err := wipeDirectory(filepath.Join(workspaceRoot, "packages"), preservePkgs); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error cleaning packages: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 4: Reset desktop.config.ts
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting desktop config…"}
		runtime.msgChan <- logLineMsg(">>> Resetting desktop/desktop.config.ts to default…")
		defaultConfig := `import { defineDesktopConfig } from '@owdproject/core'

export default defineDesktopConfig({
  theme: '',
  apps: [],
  modules: [],
})
`
		configPath := filepath.Join(workspaceRoot, "desktop", "desktop.config.ts")
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error resetting desktop config: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 5: Reset package.json
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting package dependencies…"}
		runtime.msgChan <- logLineMsg(">>> Resetting desktop/package.json dependencies to core only…")
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
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error resetting package.json: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 6: Run pnpm install
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"}
		runtime.msgChan <- logLineMsg(">>> Running pnpm install (syncing workspace)…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"install"}); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error running pnpm install: %v", err))
			runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 7: Run prepare:modules
		step++
		runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"}
		runtime.msgChan <- logLineMsg(">>> Rebuilding stubs…")
		if err := runtime.runProcessAndStreamLogsSilent(workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error running prepare:modules: %v", err))
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
