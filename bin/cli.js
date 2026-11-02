import { spawn, spawnSync, execSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { runDevForeground } from './lib/status.js'
import { existsSync, readFileSync, writeSync } from 'node:fs'
import { join, resolve } from 'node:path'
import getopts from 'getopts'
import {
  resolveSource,
  printPlan,
  printNextSteps,
  runWorkspaceInstall,
  runNpmInstall,
} from './lib/install.js'
import { findWorkspaceRoot, inferKind, withoutFrameworkPrefix, FRAMEWORKS } from './lib/workspace.js'
import { resolveDevTarget } from './lib/playgroundContext.js'
import { scaffoldProject } from './lib/scaffold.js'

const SCOPE = '@owdproject/'

/** @type {Record<string, { configKey: string, workspaceDir: string, nx: string, label: string }>} */
const KINDS = {
  app: {
    label: 'app',
    configKey: 'apps',
    workspaceDir: 'apps',
    nx: 'desktop:install-app',
  },
  module: {
    label: 'module',
    configKey: 'modules',
    workspaceDir: 'packages',
    nx: 'desktop:install-module',
  },
  theme: {
    label: 'theme',
    configKey: 'theme',
    workspaceDir: 'themes',
    nx: 'desktop:install-theme',
  },
}

const LEGACY_COMMANDS = {
  'install-app': 'app',
  'install-module': 'module',
  'install-theme': 'theme',
}

/** @type {string} */
let commandName = 'desktop'

function buildHelp(name) {
  return `
${name} — add apps, modules, and themes to your Open Web Desktop

USAGE
  ${name}                 Open the control panel (interactive TUI)
  ${name} dev [--playground]  Start dev server (auto-detects module playground)

CONTROL PANEL (Bubble Tea)
  d / x                   Start / stop the dev server
  s                       Apply queued package and theme changes
  1 / 2 / 3               Apps / Modules / Themes catalog tabs
  ↑ / ↓ / Space           Navigate and select packages
  f                       Find the optional Nuxt file service
  c / u                   Manage / update the selected package
  g / r                   Settings / refresh catalog
  n                       Create a new app or module
  q                       Quit
  ${name} init [dir] [--framework nuxt|react]  Create a desktop project
  ${name} create                  Create a new app or module (interactive wizard)
  ${name} create <kind> <name>    Scaffold a new app or module (e.g. desktop create app todo → @owdproject/nuxt-app-todo)
  ${name} add <package> [options]
  ${name} add <kind> <name> [options]
  ${name} validate [path...]  Check Nuxt module + playground layout
  ${name} template [--dry-run] [--check]  Regenerate client/template/ (maintainers)

TEMPLATE (monorepo maintainers)
  ${name} template              Write template/ from blueprint + desktop/ + latest npm versions
  ${name} template --dry-run    Show planned @owdproject/* and starter versions
  ${name} template --check      Fail if committed template/ differs (CI)

VALIDATE
  ${name} validate            Validate cwd package, or all apps/themes/modules at repo root
  ${name} validate .          Validate explicit package directory
  ${name} validate apps       Validate every @owdproject/* module under apps/
  --json                      Machine-readable output
  --strict                    Treat warnings as failures
  --smoke                     Run dev:prepare + nuxt build playground (slow, CI)

INSTALL SOURCES (control panel or CLI)
  npm                   registry (default for desktop add)
  --dev                 clone github.com/owdproject/<package>
  --from <user>         clone from GitHub user or full git URL (SSH supported)

EXAMPLES
  pnpm ${name}            # control panel
  pnpm ${name} dev        # monorepo desktop (from repo root)
  cd apps/owd-nuxt-app-about && pnpm ${name} dev   # about app playground
  pnpm ${name} dev --playground           # force playground when cwd is in a module
  ${name} init my-desktop # scaffold + pnpm install + control panel
  ${name} add nuxt-app-todo --npm
  ${name} add nuxt-app-todo --dev
  ${name} add nuxt-module-pinia-persistence --from dxlliv

KINDS (optional — inferred from the package name)
  app       nuxt-app-*    → apps/     (react-app-* for React desktops)
  module    nuxt-module-* → packages/ (nuxt-kit-*, react-kit-* and *-template are not listed in the control panel)
  theme     nuxt-theme-*  → themes/   (react-theme-* for React desktops)

OPTIONS
  --playground      Start the module playground dev server (when cwd is inside an @owdproject/* package with playground/)
  --framework       Starter framework for init (nuxt or react; Nuxt is the non-interactive default)
  --starter-repo    Git repository containing the selected starter's template/ (init)
  --skip-install    Create the starter without installing dependencies (init)
  --skip-ui         Do not launch the control panel after setup (init)
  --from <source>   Git source (user, user/repo, or URL)
  --branch <name>   Git branch to clone
  --npm             Install from npm (default when --from is omitted)
  --dev, --workspace  Clone from github.com/owdproject/<package>
  --protocol https|ssh  With --from <user>, use HTTPS (default) or SSH clone URL
  --dry-run         Print the plan without changing anything
  -h, --help        Show this help

--from <source>
  (omit)          npm registry (default)
  npm             npm registry
  owdproject      Clone official repo (monorepo only)
  <github-user>   Clone github.com/<user>/<package> (your fork)
  <user>/<repo>   Explicit GitHub repo
  <git-url>       Full clone URL

LEGACY (still supported)
  ${name} install-app @owdproject/nuxt-app-todo
  → prefer: ${name} add app-todo
`
}

async function chooseStarter(target) {
  if (!process.stdin.isTTY || !process.stdout.isTTY || process.env.CI) return 'nuxt'

  const cliDir = join(fileURLToPath(import.meta.url), '..')
  const goDir = join(cliDir, '../go')
  const binaryPath = join(goDir, 'desktop-go')
  const build = spawnSync('go', ['build', '-buildvcs=false', '-o', binaryPath, '.'], { cwd: goDir, stdio: 'inherit' })
  if (build.status === 0) {
    const selected = spawnSync(binaryPath, ['--choose-starter', target], { stdio: 'inherit' })
    if (selected.status === 20) return 'nuxt'
    if (selected.status === 21) return 'react'
    if (selected.status === 22) return null
  }

  console.warn('The Bubble Tea starter picker is unavailable; using a text prompt.')
  console.log(`Workspace path: ${target}`)
  console.log('Vue, Svelte, Angular and Astro host starters are planned.')
  const readline = await import('node:readline/promises')
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout })
  try {
    for (;;) {
      const value = (await rl.question('Choose a starter (nuxt / react) [nuxt]: ')).trim().toLowerCase()
      if (!value || value === 'nuxt') return 'nuxt'
      if (value === 'react') return 'react'
      if (value === 'q' || value === 'cancel') return null
      console.log('Choose nuxt or react, or type q to cancel.')
    }
  } finally {
    rl.close()
  }
}

async function openControlPanel(name) {
  const cliDir = join(fileURLToPath(import.meta.url), '..')
  const goDir = join(cliDir, '../go')
  const binaryPath = join(goDir, 'desktop-go')
  const readline = await import('node:readline/promises')

  // Compile the Go Control Panel if binary doesn't exist
  if (!existsSync(binaryPath)) {
    console.log('Go Control Panel binary not found. Compiling...')
    try {
      execSync('go build -buildvcs=false -o desktop-go .', { cwd: goDir, stdio: 'inherit' })
      console.log('Go Control Panel compiled successfully!')
    } catch (err) {
      console.warn('Failed to compile Go Control Panel, falling back to legacy CP:', err.message)
      const child = spawn(process.execPath, [
        '-e',
        `import('./cp.js').then(m => m.runCp(${JSON.stringify(name)}))`
      ], {
        cwd: cliDir,
        stdio: 'inherit',
      })
      child.on('exit', async (code) => {
        if (code === 10) {
          const { runCreateCli } = await import('./lib/create.js')
          const workspaceRoot = findWorkspaceRoot()
          try {
            await runCreateCli({ workspaceRoot })
          } catch (err) {
            console.error('\n✗ Error creating package:', err.message)
            const rl = readline.createInterface({ input: process.stdin, output: process.stdout })
            await rl.question('\nPress Enter to return to the control panel...')
            rl.close()
          }
          await openControlPanel(name)
        } else {
          process.exit(code ?? 0)
        }
      })
      return
    }
  }

  // Execute Go TUI
  const child = spawn(binaryPath, [], { stdio: 'inherit' })
  child.on('exit', async (code) => {
    if (code === 10) {
      const { runCreateCli } = await import('./lib/create.js')
      const workspaceRoot = findWorkspaceRoot()
      try {
        await runCreateCli({ workspaceRoot })
      } catch (err) {
        console.error('\n✗ Error creating package:', err.message)
        const rl = readline.createInterface({ input: process.stdin, output: process.stdout })
        await rl.question('\nPress Enter to return to the control panel...')
        rl.close()
      }
      await openControlPanel(name)
    } else {
      process.exit(code ?? 0)
    }
  })
}

function fail(message, hint) {
  const hintLine = hint ? `  ${hint}\n` : ''
  // stderr may be a pipe; flush diagnostics synchronously before exiting.
  writeSync(process.stderr.fd, `\n✗ ${message}\n${hintLine}\nRun \`${commandName} --help\` for usage.\n\n`)
  process.exit(1)
}

function warn(message) {
  console.warn(`\n⚠ ${message}\n`)
}

/**
 * @param {string} input
 * @param {string | undefined} explicitKind
 */
function normalizePackage(input, explicitKind) {
  let raw = input.trim()
  const fullyScoped = raw.startsWith(SCOPE)
  if (fullyScoped) raw = raw.slice(SCOPE.length)

  if (!raw) {
    fail('Package name is required.', `Example: ${commandName} add app-todo`)
  }

  const kind = explicitKind ?? inferKind(raw)

  if (explicitKind) {
    const prefixes = {
      app: 'app-',
      theme: 'theme-',
      module: ['module-', 'kit-'],
    }
    const expected = prefixes[explicitKind]
    const list = Array.isArray(expected) ? expected : [expected]
    if (!list.some((p) => withoutFrameworkPrefix(raw).startsWith(p))) {
      raw = `${list[0]}${raw}`
    }
  }

  // Unscoped shorthand targets the current Nuxt host. A fully scoped name is
  // kept exact, including legacy package names explicitly requested by users.
  const firstSegment = raw.split('-')[0]
  if (!fullyScoped && !FRAMEWORKS.includes(firstSegment) && /^(app|module|theme|kit)-/.test(raw)) {
    raw = `nuxt-${raw}`
  }

  return { pkgName: `${SCOPE}${raw}`, shortName: raw, kind }
}

function mapLegacyFlags(parsed) {
  const { dev, workspace, fork, repo, branch, from, npm } = parsed
  let mappedFrom = from
  let mappedNpm = npm
  const forceDev = dev || workspace

  if (fork || repo) {
    warn('`--fork` and `--repo` are deprecated — use `--from` instead.')
    mappedFrom = repo || fork
  }
  if (forceDev && !mappedFrom && !mappedNpm) {
    mappedFrom = 'owdproject'
  }
  if (mappedNpm) mappedFrom = 'npm'

  return { from: mappedFrom, branch, npm: mappedNpm, dev: forceDev }
}

function parseAddArgs(positionals) {
  const kindNames = Object.keys(KINDS)
  if (positionals.length === 0) {
    fail('Missing package name.', `Example: ${commandName} add app-todo`)
  }

  if (kindNames.includes(positionals[0])) {
    const kind = positionals[0]
    const name = positionals[1]
    if (!name) {
      fail(`Missing name for kind "${kind}".`, `Example: ${commandName} add ${kind} todo`)
    }
    return normalizePackage(name, kind)
  }

  return normalizePackage(positionals[0])
}

function parseLegacyInstall(cmd, positionals) {
  const kind = LEGACY_COMMANDS[cmd]
  const raw = positionals[0]
  if (!raw) {
    fail('Missing package name.', `Example: ${commandName} ${cmd} ${SCOPE}app-todo`)
  }
  warn(`\`${cmd}\` is deprecated — use \`${commandName} add …\` instead.`)
  return normalizePackage(raw, kind)
}

/**
 * @param {string} name CLI binary name (`desktop` or deprecated `owd`)
 * @param {string[]} argv Arguments (without node and script path)
 * @param {{ deprecated?: boolean }} [options]
 */
export async function runCli(name, argv, options = {}) {
  commandName = name

  if (options.deprecated) {
    warn('The `owd` command is deprecated. Use `desktop` instead.')
  }

  const parsed = getopts(argv, {
    alias: { h: 'help', b: 'branch' },
    boolean: ['help', 'npm', 'dry-run', 'dev', 'workspace', 'playground', 'json', 'strict', 'smoke', 'check', 'skip-install', 'skip-ui'],
    string: ['from', 'branch', 'fork', 'repo', 'protocol', 'framework', 'starter-repo'],
  })

  const { _, help, npm: npmFlag } = parsed
  const dryRun = parsed['dry-run'] === true
  const { from, branch, npm: npmFromLegacy, dev: devFlag } = mapLegacyFlags(parsed)
  const protocol = parsed.protocol === 'ssh' ? 'ssh' : 'https'
  const useNpm = npmFlag || npmFromLegacy || (!from && !devFlag)

  if (help || _[0] === 'help') {
    console.log(buildHelp(name))
    return
  }

  const cmd = _[0]

  if (cmd === 'init') {
    const cwd = process.cwd()

    if (findWorkspaceRoot(cwd)) {
      fail(
        'Already inside an OWD workspace.',
        `Use \`${name}\` to manage this project, or run init from a parent directory.`,
      )
    }

    try {
      const framework = parsed.framework || await chooseStarter(resolve(cwd, _[1] ? String(_[1]).trim() : 'owd-nuxt-desktop'))
      if (!framework) {
        console.log('Starter creation cancelled.')
        return
      }
      await scaffoldProject({
        dir: _[1] ? String(_[1]).trim() : undefined,
        cwd,
        commandName: name,
        framework,
        repo: parsed['starter-repo'] || undefined,
        skipInstall: parsed['skip-install'],
        skipUi: parsed['skip-ui'],
      })
    } catch (error) {
      fail(error.message ?? String(error))
    }
    return
  }

  const workspaceRoot = findWorkspaceRoot()

  if (workspaceRoot) {
    const profilePath = join(workspaceRoot, '.owd', 'project.json')
    if (existsSync(profilePath)) {
      let profile
      try {
        profile = JSON.parse(readFileSync(profilePath, 'utf8'))
      } catch (error) {
        fail(`Cannot read project profile: ${error.message}`)
      }
      // Frameworks the control panel can drive. Planned hosts (vue/svelte/
      // angular/astro) still fail until their panel support lands.
      const panelFrameworks = ['nuxt', 'react']
      if (!panelFrameworks.includes(profile.framework ?? 'nuxt')) {
        fail(`The control panel does not yet support the ${profile.framework ?? 'unknown'} host.`,
          'Use this project’s own scripts until its adapter is supported.')
      }
    }
  }

  if (cmd === 'dev') {
    if (!workspaceRoot) {
      fail(
        'Not inside an OWD workspace.',
        'Run from the client repo, or try: desktop · desktop add <package>',
      )
    }
    const forcePlayground = parsed.playground === true
    const devTarget = resolveDevTarget(process.cwd(), workspaceRoot, { forcePlayground })
    if (forcePlayground && !devTarget) {
      fail(
        'No @owdproject module with playground/desktop.config.ts found from this directory.',
        'Run from an app/theme/module package (e.g. apps/owd-nuxt-app-about), or omit --playground for the monorepo desktop.',
      )
    }
    runDevForeground(devTarget)
    return
  }

  if (cmd === 'validate') {
    const { runValidateCli, formatValidationReport } = await import('./lib/validateModule.js')
    const strict = parsed.strict === true
    const json = parsed.json === true
    const smoke = parsed.smoke === true
    const paths = _.slice(1).map(String)
    const { exitCode, results } = runValidateCli(paths, {
      workspaceRoot,
      json,
      strict,
      smoke,
    })
    for (const result of results) {
      console.log(formatValidationReport(result, { json }))
      if (!json) console.log('')
    }
    process.exitCode = exitCode
    return
  }

  if (cmd === 'template') {
    if (!workspaceRoot) {
      fail(
        'Not inside an OWD workspace.',
        `Run from the client monorepo root to regenerate template/.`,
      )
    }
    const { runGenerateTemplateCli } = await import('./lib/generateTemplate.js')
    await runGenerateTemplateCli({
      workspaceRoot,
      dryRun: parsed['dry-run'] === true,
      check: parsed.check === true,
    })
    return
  }

  if (cmd === 'create' || cmd === 'new') {
    if (!workspaceRoot) {
      fail(
        'Not inside an OWD workspace.',
        `Run from inside an OWD workspace directory to scaffold a new package.`,
      )
    }
    const { runCreateCli } = await import('./lib/create.js')
    const kind = _[1]
    const name = _[2]
    await runCreateCli({ kind, name, workspaceRoot })
    return
  }

  if (!cmd) {
    if (!workspaceRoot) {
      if (!process.stdin.isTTY || !process.stdout.isTTY || process.env.CI) {
        fail('Not inside an OWD workspace.', `Run ${name} init . --framework nuxt to create one in an empty folder.`)
      }
      try {
        const framework = await chooseStarter(process.cwd())
        if (framework) await scaffoldProject({ dir: '.', cwd: process.cwd(), commandName: name, framework })
      } catch (error) {
        fail(error.message ?? String(error))
      }
      return
    }
    await openControlPanel(name)
    return
  }

  if (cmd === 'ui') {
    fail('The `ui` subcommand was removed.', `Use \`${name}\` to open the control panel.`)
  }

  if (!workspaceRoot) {
    fail(
      'Not inside an OWD workspace.',
      'Run from the client repo (needs pnpm-workspace.yaml + nx.json), or use a generated project with the same layout.',
    )
  }

  let pkgInfo

  if (cmd === 'add') {
    pkgInfo = parseAddArgs(_.slice(1))
  } else if (LEGACY_COMMANDS[cmd]) {
    pkgInfo = parseLegacyInstall(cmd, _.slice(1))
  } else {
    fail(`Unknown command: ${cmd}`, `Use \`${name}\`, \`${name} dev\`, or \`${name} add <package>\`.`)
  }

  const segments = pkgInfo.shortName.split('-')
  if (FRAMEWORKS.includes(segments[0]) && segments[0] !== 'nuxt' &&
      ['app', 'module', 'theme', 'kit'].includes(segments[1])) {
    fail(`${pkgInfo.pkgName} targets ${segments[0]}, but this control panel manages a Nuxt desktop.`,
      'Choose an extension built for the Nuxt host.')
  }

  const { pkgName, shortName, kind } = pkgInfo
  const inMonorepo = Boolean(workspaceRoot)
  let resolvedFrom = useNpm ? 'npm' : from
  if (
    !useNpm &&
    resolvedFrom &&
    resolvedFrom !== 'npm' &&
    resolvedFrom !== 'owdproject' &&
    resolvedFrom !== 'official' &&
    protocol === 'ssh' &&
    !/[@/]/.test(resolvedFrom)
  ) {
    resolvedFrom = `git@github.com:${resolvedFrom}/${shortName}.git`
  }
  const source = resolveSource(resolvedFrom, shortName, inMonorepo, {
    npm: useNpm,
    dev: devFlag,
  })

  if (source.mode === 'npm') {
    if (dryRun) {
      printPlan({
        pkgName,
        kind,
        targetDir: '(npm → node_modules)',
        source,
        branch: null,
        dryRun: true,
      })
      return
    }
    try {
      await runNpmInstall(kind, pkgName, workspaceRoot)
    } catch (error) {
      fail(error.message ?? String(error))
    }
  } else {
    try {
      await runWorkspaceInstall({
        pkgName,
        shortName,
        kind,
        workspaceRoot,
        source,
        branch,
        dryRun,
      })
    } catch (error) {
      fail(error.message ?? String(error))
    }
  }
}
