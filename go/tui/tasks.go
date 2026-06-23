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
)

// ─────────────────────────────────────────────
// Background tasks
// ─────────────────────────────────────────────

func (m *TuiModel) runProcessAndStreamLogs(root, command string, args []string) {
	cmd := exec.Command(command, args...)
	cmd.Dir = root

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
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
		m.runtime.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
	}

	err = cmd.Wait()
	m.runtime.msgChan <- taskFinishedMsg{Success: err == nil, Err: err}
}

func (m *TuiModel) runProcessAndStreamLogsSilent(root, command string, args []string) error {
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
		m.runtime.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
	}

	return cmd.Wait()
}

func (m *TuiModel) RunSetupTask(adds map[string]string) {
	go func() {
		desktopJs := filepath.Join(m.workspaceRoot, "packages", "cli", "bin", "desktop.js")

		totalSteps := len(adds) + 2
		step := 0
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Initializing setup task…"}

		// 1. Process all additions
		for pkgName, method := range adds {
			step++
			shortName := pkgName
			if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
				shortName = pkgName[idx+1:]
			}
			m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: fmt.Sprintf("Installing %s via %s…", shortName, method)}

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

			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Executing: node desktop.js add %s via %s", shortName, method))
			if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "node", args); err != nil {
				m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Add %s failed: %v", shortName, err))
				m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
		}

		// 2. Run pnpm install for cleanup/removal sync
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"}
		m.runtime.msgChan <- logLineMsg(">>> Running pnpm install (syncing workspace)…")
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}); err != nil {
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// 3. Rebuild stubs
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"}
		m.runtime.msgChan <- logLineMsg(">>> Rebuilding stubs…")
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		m.runtime.msgChan <- taskFinishedMsg{Success: true}
	}()
}

func (m *TuiModel) RunUpdatePackageTask(pkgName string, shortName string, kind string, isLocalSource bool) {
	go func() {
		m.runtime.msgChan <- clearLogsMsg{}
		m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Starting update for %s…", shortName))
		m.runtime.msgChan <- setupProgressMsg{Step: 1, Total: 3, Label: fmt.Sprintf("Updating %s…", shortName)}

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
				m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Running git pull in %s…", pkgPath))
				if err := m.runProcessAndStreamLogsSilent(pkgPath, "git", []string{"pull"}); err != nil {
					m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Git pull failed: %v", err))
					m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
					return
				}
			}
		} else {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Re-installing %s from NPM to get latest version…", shortName))
			args := []string{desktopJs, "add", shortName, "--npm"}
			if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "node", args); err != nil {
				m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> NPM update failed: %v", err))
				m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
				return
			}
		}

		// Run pnpm install
		m.runtime.msgChan <- setupProgressMsg{Step: 2, Total: 3, Label: "Installing dependencies (pnpm install)…"}
		m.runtime.msgChan <- logLineMsg(">>> Running pnpm install (syncing workspace)…")
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}); err != nil {
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Rebuild stubs
		m.runtime.msgChan <- setupProgressMsg{Step: 3, Total: 3, Label: "Rebuilding stubs (prepare:modules)…"}
		m.runtime.msgChan <- logLineMsg(">>> Rebuilding stubs…")
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		m.runtime.msgChan <- taskFinishedMsg{Success: true}
	}()
}

func (m *TuiModel) RunServeTask() {
	go func() {
		if m.ctx == nil {
			m.runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}

		_ = os.MkdirAll(m.ctx.Paths.MetaDir, 0755)

		logPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Failed to open log file: %v", err))
			m.runtime.msgChan <- serverStatusMsg{Running: false}
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

		m.runtime.serverMu.Lock()
		m.runtime.serverCmd = cmd
		m.runtime.serverMu.Unlock()

		if err := cmd.Start(); err != nil {
			logFile.WriteString(fmt.Sprintf(">>> Server failed to start: %v\n", err))
			m.runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}

		// Write PID file
		pidPath := filepath.Join(m.ctx.Paths.MetaDir, "dev.pid")
		_ = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

		m.runtime.msgChan <- serverStatusMsg{Running: true}

		cmd.Wait()
		logFile.WriteString(">>> Dev server exited.\n")
		m.runtime.msgChan <- serverStatusMsg{Running: false}

		m.runtime.serverMu.Lock()
		m.runtime.serverCmd = nil
		m.runtime.serverMu.Unlock()
	}()
}

func (m *TuiModel) StopServeTask() {
	go func() {
		if m.ctx == nil {
			m.runtime.msgChan <- serverStatusMsg{Running: false}
			return
		}

		m.runtime.serverMu.Lock()
		cmd := m.runtime.serverCmd
		m.runtime.serverCmd = nil
		m.runtime.serverMu.Unlock()

		if cmd != nil && cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
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

		m.runtime.msgChan <- serverStatusMsg{Running: false}
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
					m.runtime.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
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
				m.runtime.msgChan <- clearLogsMsg{}
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
					m.runtime.msgChan <- logLineMsg(strings.TrimRight(line, "\r\n"))
				}
				lastSize = info.Size()
				file.Close()
			}
		}
	}()
}

func (m *TuiModel) RunWipeWorkspaceTask() {
	go func() {
		m.runtime.msgChan <- clearLogsMsg{}
		m.runtime.msgChan <- logLineMsg(">>> Starting workspace reset task…")

		totalSteps := 7
		step := 0

		// Step 1: Clean apps/
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning apps directory…"}
		m.runtime.msgChan <- logLineMsg(">>> Cleaning apps/ directory…")
		if err := wipeDirectory(filepath.Join(m.workspaceRoot, "apps"), nil); err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error cleaning apps: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 2: Clean themes/
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning themes directory…"}
		m.runtime.msgChan <- logLineMsg(">>> Cleaning themes/ directory…")
		if err := wipeDirectory(filepath.Join(m.workspaceRoot, "themes"), nil); err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error cleaning themes: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 3: Clean packages/ (preserving core, cli, nx)
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Cleaning packages directory…"}
		m.runtime.msgChan <- logLineMsg(">>> Cleaning packages/ directory (preserving core, cli, nx)…")
		preservePkgs := map[string]bool{"core": true, "cli": true, "nx": true}
		if err := wipeDirectory(filepath.Join(m.workspaceRoot, "packages"), preservePkgs); err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error cleaning packages: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 4: Reset desktop.config.ts
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting desktop config…"}
		m.runtime.msgChan <- logLineMsg(">>> Resetting desktop/desktop.config.ts to default…")
		defaultConfig := `import { defineDesktopConfig } from '@owdproject/core'

export default defineDesktopConfig({
  theme: '',
  apps: [],
  modules: [],
})
`
		configPath := filepath.Join(m.workspaceRoot, "desktop", "desktop.config.ts")
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error resetting desktop config: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 5: Reset package.json
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Resetting package dependencies…"}
		m.runtime.msgChan <- logLineMsg(">>> Resetting desktop/package.json dependencies to core only…")
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
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error resetting package.json: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 6: Run pnpm install
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Installing dependencies (pnpm install)…"}
		m.runtime.msgChan <- logLineMsg(">>> Running pnpm install (syncing workspace)…")
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"install"}); err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error running pnpm install: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		// Step 7: Run prepare:modules
		step++
		m.runtime.msgChan <- setupProgressMsg{Step: step, Total: totalSteps, Label: "Rebuilding stubs (prepare:modules)…"}
		m.runtime.msgChan <- logLineMsg(">>> Rebuilding stubs…")
		if err := m.runProcessAndStreamLogsSilent(m.workspaceRoot, "pnpm", []string{"run", "prepare:modules"}); err != nil {
			m.runtime.msgChan <- logLineMsg(fmt.Sprintf(">>> Error running prepare:modules: %v", err))
			m.runtime.msgChan <- taskFinishedMsg{Success: false, Err: err}
			return
		}

		m.runtime.msgChan <- taskFinishedMsg{Success: true}
	}()
}
