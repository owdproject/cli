<p align="center">
  <img width="160" height="160" src="https://avatars.githubusercontent.com/u/65117737?s=160&v=4" />
</p>
<h1 align="center">Template Module</h1>
<h3 align="center">
  Quick start a new Open Web Desktop module.
</h3>

<br />

## Overview

A template for your new Open Web Desktop module. This template is designed to help you quickly bootstrap and build custom Nuxt modules that extend the core desktop functionality.

[Demo](https://owdproject.github.io/) · [Documentation](https://owdproject.github.io/docs/) · [Support](https://github.com/sponsors/owdproject)

## Getting Started

### Method 1: Using the OWD CLI (Recommended)

Run the following command in your OWD workspace root to scaffold a new module automatically:

```bash
pnpm desktop create module <name>
```

This command will:
- Copy the template files into `packages/nuxt-module-<name>`
- Perform package name and namespace replacements
- Run `pnpm install` in the workspace root
- Link the new module and register it under the `modules` key in your `desktop/desktop.config.ts`

### Method 2: Manual Installation

If you prefer to manually clone and set up the module template:

1. Download or clone this repository into your `/packages` directory:
   ```bash
   cd <your-owd-client-path>/packages
   wget -O - https://github.com/owdproject/module-template/archive/refs/heads/main.zip | unzip -d nuxt-module-<name> -
   ```

2. Open the newly created `packages/nuxt-module-<name>/package.json` and change the `"name"` field to `@owdproject/nuxt-module-<name>`.

3. Register the module under the `modules` key in your `desktop/desktop.config.ts`:
   ```typescript
   export default defineDesktopConfig({
     modules: [
       '@owdproject/nuxt-module-<name>'
     ]
   })
   ```

4. Install dependencies in your workspace to enable internal linking:
   ```bash
   pnpm install
   ```

## Agent guidance and integration contract

Read [AGENTS.md](AGENTS.md) and the [local module contract](docs/module-contract.md). They are copied into generated projects. The module metadata declares its framework and adapter; declare any required host integrations explicitly.

## Development

Run the module playground to test your implementation:

```bash
cd packages/nuxt-module-<name>
pnpm run dev
```

## License

This application is released under the [MIT License](LICENSE).

