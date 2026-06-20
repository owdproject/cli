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
- **Scaffolding**: Instantly initialize new workspaces or scaffold new apps and themes.
- **Package Management**: Install and validate OWD modules from npm, local directories, or custom Git repositories.
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
Install apps, modules, or themes from npm, local directories, or custom repositories:
```bash
pnpm desktop add app-todo --npm
pnpm desktop add theme-nova --dev
pnpm desktop add module-fs --from <github-user>
```

#### Scaffold a Project
Initialize a fresh OWD workspace:
```bash
pnpm desktop init [project-name]
```

#### Validation
Check Nuxt module configuration and playground directory structures:
```bash
pnpm desktop validate
```

## License

This package is released under the [MIT License](LICENSE).
