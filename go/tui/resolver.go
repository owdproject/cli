package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"owd-cli/bridge"
)

// readDesktopPackageDeps reads @owdproject/* deps with versions from desktop/package.json.
func readDesktopPackageDeps(packageJsonPath string) (map[string]string, error) {
	data, err := os.ReadFile(packageJsonPath)
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
	deps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		if strings.HasPrefix(k, "@owdproject/") {
			deps[k] = v
		}
	}
	for k, v := range pkg.DevDependencies {
		if strings.HasPrefix(k, "@owdproject/") {
			deps[k] = v
		}
	}
	return deps, nil
}

func isNpmVersion(version string) bool {
	if version == "" {
		return false
	}
	if version == "workspace:*" || version == "workspace:^" {
		return false
	}
	return true
}

func shortNameFromPkg(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func kindDirForKind(kind string) string {
	switch kind {
	case "app":
		return "apps"
	case "module":
		return "packages"
	case "theme":
		return "themes"
	default:
		return "packages"
	}
}

func catalogEntryForName(catalog *bridge.CatalogResponse, name string) bridge.CatalogEntry {
	if catalog != nil {
		for _, e := range catalog.Entries {
			if e.Name == name {
				return e
			}
		}
	}
	short := shortNameFromPkg(name)
	kind := "module"
	if strings.HasPrefix(short, "app-") {
		kind = "app"
	} else if strings.HasPrefix(short, "theme-") {
		kind = "theme"
	}
	return bridge.CatalogEntry{
		Name:      name,
		ShortName: short,
		Kind:      kind,
	}
}

// packagesFromConfig returns all configured package names from desktop.config.ts.
func packagesFromConfig(config *bridge.Config) []struct {
	Name string
	Kind string
} {
	var out []struct {
		Name string
		Kind string
	}
	if config == nil {
		return out
	}
	for _, name := range config.Apps {
		out = append(out, struct {
			Name string
			Kind string
		}{Name: name, Kind: "app"})
	}
	for _, name := range config.Modules {
		out = append(out, struct {
			Name string
			Kind string
		}{Name: name, Kind: "module"})
	}
	if config.Theme != nil && *config.Theme != "" {
		out = append(out, struct {
			Name string
			Kind string
		}{Name: *config.Theme, Kind: "theme"})
	}
	return out
}

// BuildQueueFromConfig resolves packages in config against desktop/package.json.
func BuildQueueFromConfig(
	workspaceRoot string,
	ctx *bridge.WorkspaceContext,
	catalog *bridge.CatalogResponse,
	logFn func(string),
) *WorkQueue {
	q := NewWorkQueue()
	if ctx == nil {
		return q
	}

	deps, err := readDesktopPackageDeps(ctx.Paths.PackageJson)
	if err != nil {
		logFn("✖ Failed to read desktop/package.json: " + err.Error())
		return q
	}

	seen := map[string]bool{}
	for _, pkg := range packagesFromConfig(&ctx.Config) {
		if seen[pkg.Name] {
			continue
		}
		seen[pkg.Name] = true
		addPackageToQueue(q, workspaceRoot, pkg.Name, pkg.Kind, deps, catalog, false, logFn)
	}
	return q
}

// BuildStartupCheckQueue builds queue for installed-but-not-local packages.
func BuildStartupCheckQueue(
	workspaceRoot string,
	ctx *bridge.WorkspaceContext,
	catalog *bridge.CatalogResponse,
	logFn func(string),
) *WorkQueue {
	q := NewWorkQueue()
	if catalog == nil || ctx == nil {
		return q
	}

	deps, _ := readDesktopPackageDeps(ctx.Paths.PackageJson)
	seen := map[string]bool{}

	for _, entry := range catalog.Entries {
		if entry.Installed && !entry.LocalSource {
			if !seen[entry.Name] {
				seen[entry.Name] = true
				addPackageToQueue(q, workspaceRoot, entry.Name, entry.Kind, deps, catalog, false, logFn)
			}
		}
	}

	// Theme kit dependencies
	var themeShort string
	if ctx.Config.Theme != nil && *ctx.Config.Theme != "" {
		themeShort = shortNameFromPkg(*ctx.Config.Theme)
	}
	if themeShort != "" {
		themeDeps, err := getThemeDependencies(workspaceRoot, themeShort)
		if err == nil {
			for _, dep := range themeDeps {
				if seen[dep] {
					continue
				}
				short := shortNameFromPkg(dep)
				if !strings.HasPrefix(short, "kit-") && !strings.HasPrefix(short, "module-") {
					continue
				}
				if isLocallyAvailable(workspaceRoot, short) {
					logFn("ℹ " + short + " already available")
					continue
				}
				seen[dep] = true
				addPackageToQueue(q, workspaceRoot, dep, "module", deps, catalog, true, logFn)
			}
		}
	}

	return q
}

func addPackageToQueue(
	q *WorkQueue,
	workspaceRoot, name, kind string,
	deps map[string]string,
	catalog *bridge.CatalogResponse,
	discovered bool,
	logFn func(string),
) {
	entry := catalogEntryForName(catalog, name)
	if kind != "" {
		entry.Kind = kind
	}
	short := entry.ShortName
	if short == "" {
		short = shortNameFromPkg(name)
	}

	if entry.LocalSource || isLocallyAvailable(workspaceRoot, short) {
		logFn("ℹ " + short + " already available")
		q.AppendUnique(WorkItem{
			Name:       name,
			ShortName:  short,
			Type:       entry.Kind,
			Source:     "local",
			Status:     StatusSkipped,
			Kind:       KindLocal,
			TargetDir:  filepath.Join(workspaceRoot, kindDirForKind(entry.Kind), short),
			Discovered: discovered,
			Entry:      entry,
		})
		return
	}

	version, inPkg := deps[name]
	if !inPkg {
		logFn("⚠ " + short + " missing from package.json")
		q.AppendUnique(WorkItem{
			Name:       name,
			ShortName:  short,
			Type:       entry.Kind,
			Source:     SourcePending,
			Status:     StatusPending,
			Kind:       KindPrompt,
			Discovered: discovered,
			Entry:      entry,
		})
		return
	}

	if version == "workspace:*" || version == "workspace:^" {
		logFn("⚠ " + short + " configured as workspace dependency")
		q.AppendUnique(WorkItem{
			Name:       name,
			ShortName:  short,
			Type:       entry.Kind,
			Source:     SourcePending,
			Status:     StatusPending,
			Kind:       KindClone,
			TargetDir:  filepath.Join(workspaceRoot, kindDirForKind(entry.Kind), short),
			Discovered: discovered,
			Entry:      entry,
		})
		return
	}

	if isNpmVersion(version) {
		logFn("ℹ " + short + " resolved as npm package (" + version + ")")
		q.AppendUnique(WorkItem{
			Name:       name,
			ShortName:  short,
			Type:       entry.Kind,
			Source:     "npm:" + version,
			Status:     StatusSkipped,
			Kind:       KindNpm,
			Discovered: discovered,
			Entry:      entry,
		})
		return
	}

	logFn("⚠ " + short + " has unknown source (" + version + ") — needs resolution")
	q.AppendUnique(WorkItem{
		Name:       name,
		ShortName:  short,
		Type:       entry.Kind,
		Source:     SourcePending,
		Status:     StatusPending,
		Kind:       KindPrompt,
		Discovered: discovered,
		Entry:      entry,
	})
}

// DiscoverNestedDeps scans a cloned package for kit-*/module-* deps.
func DiscoverNestedDeps(
	workspaceRoot string,
	item WorkItem,
	catalog *bridge.CatalogResponse,
	existing *WorkQueue,
	logFn func(string),
) {
	deps, found := getOwdDepsLocal(workspaceRoot, item.ShortName, item.Type)
	if !found {
		return
	}

	for _, dep := range deps {
		if existing.FindByName(dep) != nil {
			continue
		}
		short := shortNameFromPkg(dep)
		if isLocallyAvailable(workspaceRoot, short) {
			logFn("ℹ " + short + " already available")
			continue
		}
		logFn("⚠ Missing dependency discovered: " + short)
		pkgDeps, _ := readDesktopPackageDeps(filepath.Join(workspaceRoot, "desktop", "package.json"))
		addPackageToQueue(existing, workspaceRoot, dep, "module", pkgDeps, catalog, true, logFn)
	}
}
