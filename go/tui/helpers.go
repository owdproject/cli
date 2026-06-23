package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"owd-cli/bridge"
)

// ─────────────────────────────────────────────
// Helpers & Utilities
// ─────────────────────────────────────────────

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

func isLocallyAvailable(root, shortName string) bool {
	kindDirs := []string{"apps", "packages", "themes"}
	for _, kd := range kindDirs {
		if _, err := os.Stat(filepath.Join(root, kd, shortName)); err == nil {
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

// runGitBehindCheckNoFetch checks how many commits behind the remote the local repo is,
// WITHOUT running git fetch. Uses already-cached remote refs only — no network activity.
func runGitBehindCheckNoFetch(dir string) (int, error) {
	cmdRev := exec.Command("git", "rev-list", "--count", "HEAD..@{u}")
	cmdRev.Dir = dir
	out, err := cmdRev.Output()
	if err != nil {
		// Fallback: try origin/<branch>
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
	if max <= 0 {
		return ""
	}

	// If the printable width fits, return as-is
	if lipgloss.Width(s) <= max {
		return s
	}

	var result strings.Builder
	visibleLen := 0
	inEsc := false

	runes := []rune(s)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '\x1b' {
			inEsc = true
			result.WriteRune(r)
			continue
		}
		if inEsc {
			result.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}

		w := 1
		if r > 0x1100 && (r < 0x115f || r > 0x2329) {
			w = 2
		}

		if visibleLen+w > max-1 {
			result.WriteString("\x1b[0m…")
			return result.String()
		}

		result.WriteRune(r)
		visibleLen += w
	}

	return result.String()
}

func padRight(s string, width int) string {
	vis := lipgloss.Width(s)
	if vis >= width {
		return truncate(s, width)
	}
	return s + strings.Repeat(" ", width-vis)
}

func padLeft(s string, width int) string {
	vis := lipgloss.Width(s)
	if vis >= width {
		return truncate(s, width)
	}
	return strings.Repeat(" ", width-vis) + s
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
		if bgVisible < bgW {
			bg = bg + strings.Repeat(" ", bgW-bgVisible)
		}
		prefix := strings.Repeat(" ", leftPad)
		suffix := ""
		rightStart := leftPad + ovW
		if rightStart < bgW {
			suffix = strings.Repeat(" ", bgW-rightStart)
		}
		out[row] = prefix + ovLine + suffix
	}
	return strings.Join(out, "\n")
}

func drawPanel(w, h int, title string, content string, active bool) string {
	style := panelBorderStyle
	if active {
		style = panelBorderActive
	}

	style = style.Width(w - 2).Height(h - 2)
	rendered := style.Render(content)

	if title == "" {
		return rendered
	}

	var styledTitle string
	if strings.Contains(title, "\x1b") {
		styledTitle = title
	} else {
		styledTitle = lipgloss.NewStyle().Foreground(colorWhite).Bold(true).Render(title)
	}

	titleFmt := " " + styledTitle + " "
	titleWidth := lipgloss.Width(titleFmt)

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
	return lipgloss.NewStyle().Foreground(colorWhite).Bold(true).Render(title)
}

func kv(label, value string) string {
	return mutedStyle.Render(padRight(label, 10)) + " " + boldStyle.Render(value)
}

// keep rand used (for future sparkline noise simulation)
var _ = rand.Float64

func (m *TuiModel) getInstallMethods(pkg *bridge.CatalogEntry) []InstallMethod {
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

