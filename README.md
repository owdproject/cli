<p align="center">
  <img width="160" height="160" src="https://avatars.githubusercontent.com/u/65117737?s=160&v=4" />
</p>
<h1 align="center">CLI</h1>
<h3 align="center">
  Control Panel and Command Line Interface for Open Web Desktop.
</h3>

<br />

## Overview

This package provides the official `desktop` (and legacy `owd`) CLI tool for Open Web Desktop. It features a rich Terminal User Interface (TUI) Control Panel to manage your running local OWD dev server, install/remove apps, modules, and themes, configure workspace settings, and scaffold new OWD components.

## Installation

```bash
pnpm add -g @owdproject/cli
```
*Note: You can also install it as a devDependency in your OWD workspace project and run it via `pnpm desktop`.*

## Features

- **Control Panel (TUI)**: An interactive terminal interface for managing OWD projects.
- **Scaffolding**: Initialize new workspaces or scaffold new Nuxt apps and modules.
- **Package Management**: Install from npm or Git, or use packages already present in the workspace.
- **Graceful Fallback**: The interactive Control Panel compiles a fast Go-based TUI locally, falling back seamlessly to a JavaScript runner if Go is not available.

## Usage

Once installed, run the `desktop` command to open the control panel:

```bash
pnpm desktop
```

### CLI Reference

#### Dev Server
Start the development server for the monorepo or automatically detect a module's playground:
```bash
pnpm desktop dev [--playground]
```

#### Add Packages
Install apps, modules, or themes from npm or custom Git repositories:
```bash
pnpm desktop add app-todo --npm
pnpm desktop add theme-paper --dev
pnpm desktop add module-fs --from <github-user>
```

#### Scaffold a Project
Initialize a fresh OWD workspace:
```bash
pnpm desktop init [project-name]
```

In an interactive terminal the Bubble Tea picker offers Nuxt and React and shows
the planned Vue, Svelte, Angular and Astro hosts. For scripts,
choose explicitly with `desktop init my-desktop --framework nuxt` (the default in
non-interactive sessions is Nuxt). `--skip-install` leaves dependency installation
for later; `--skip-ui` does not open the control panel after setup.

Each generated project keeps its own `.owd/project.json` framework profile. The
current control panel supports the Nuxt host; it refuses to operate on a React
profile until a React host adapter is implemented. The React choice expects a
working `owdproject/react-desktop` starter repository, whose availability and
end-to-end behavior have not yet been verified. `--starter-repo <git-url>` can
point to an alternative starter for development. No profile index shared between
projects is currently provided.

For a Nuxt project, the control panel and `desktop add` reject extensions whose
names explicitly identify another framework. UI-kit and module compatibility
requires metadata supplied by those packages: a missing declaration is not proof
of compatibility. Choose npm or Git as appropriate for the version you need;
neither the latest npm version nor the latest Git commit guarantees compatibility.
Press `f` in the control panel to find the optional Nuxt file service. Its
"configured" status reports the module's presence in config; it does not assert
that a file manager is installed or that storage is ready.
The remaining host, compatibility and bundle work is described in
[the desktop roadmap](docs/desktop-roadmap.md).

#### Validation
Check Nuxt module configuration and playground directory structures:
```bash
pnpm desktop validate
```

## License

This package is released under the [MIT License](LICENSE).
