import { existsSync, mkdirSync, readFileSync, writeFileSync, readdirSync, statSync, mkdtempSync, rmSync } from 'node:fs'
import { join, dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import readline from 'node:readline/promises'
import { stdin as input, stdout as output } from 'node:process'
import { KINDS } from './workspace.js'
import {
  resolveDesktopConfigPath,
  desktopConfigWritePath,
} from './desktopConfig.js'
import {
  linkWorkspacePackage,
  spawnAsync,
} from './install.js'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const __dirname = dirname(fileURLToPath(import.meta.url))
const CLI_ROOT = resolve(__dirname, '../..')

function pascalCase(str) {
  return str
    .replace(/(?:^|-|_)(\w)/g, (_, c) => c.toUpperCase())
    .replace(/[-_]/g, '')
}

function camelCase(str) {
  const p = pascalCase(str)
  return p.charAt(0).toLowerCase() + p.slice(1)
}

async function cloneTemplate(gitUrl, destDir) {
  try {
    await spawnAsync('git', ['clone', '--depth', '1', gitUrl, destDir])
  } catch (err) {
    if (gitUrl.startsWith('git@')) {
      const httpsUrl = gitUrl
        .replace('git@github.com:', 'https://github.com/')
        .replace(/\.git$/, '') + '.git'
      console.log(`SSH clone failed, trying HTTPS fallback: ${httpsUrl}`)
      await spawnAsync('git', ['clone', '--depth', '1', httpsUrl, destDir])
    } else {
      throw err
    }
  }
}

function copyAndScaffold(srcDir, destDir, replacements) {
  if (!existsSync(destDir)) {
    mkdirSync(destDir, { recursive: true })
  }

  const entries = readdirSync(srcDir)
  for (const entry of entries) {
    if (
      entry === 'node_modules' ||
      entry === '.nuxt' ||
      entry === 'dist' ||
      entry === '.output' ||
      entry === '.git'
    ) {
      continue
    }

    const srcPath = join(srcDir, entry)
    let destEntry = entry
    for (const [placeholder, val] of Object.entries(replacements)) {
      destEntry = destEntry.replaceAll(placeholder, val)
    }
    const destPath = join(destDir, destEntry)

    const stat = statSync(srcPath)
    if (stat.isDirectory()) {
      copyAndScaffold(srcPath, destPath, replacements)
    } else {
      const fileContent = readFileSync(srcPath)
      const isText = !fileContent.slice(0, 512).includes(0)
      if (isText) {
        let textContent = fileContent.toString('utf8')
        for (const [placeholder, val] of Object.entries(replacements)) {
          textContent = textContent.replaceAll(placeholder, val)
        }
        writeFileSync(destPath, textContent, 'utf8')
      } else {
        writeFileSync(destPath, fileContent)
      }
    }
  }
}

export async function runCreateCli(options = {}) {
  const workspaceRoot = options.workspaceRoot
  if (!workspaceRoot) {
    throw new Error('Not inside an OWD workspace.')
  }

  let { kind, name } = options

  const rl = readline.createInterface({ input, output })

  try {
    // 1. Prompt for Kind if missing or invalid
    while (!kind || (kind !== 'app' && kind !== 'module')) {
      const answer = await rl.question('Select package kind (app / module) [app]: ')
      const cleaned = answer.trim().toLowerCase()
      if (cleaned === '') {
        kind = 'app'
      } else if (cleaned === 'app' || cleaned === 'module') {
        kind = cleaned
      } else {
        console.log('Invalid kind. Please select "app" or "module".')
      }
    }

    // 2. Prompt for Name if missing
    while (!name || name.trim() === '') {
      const answer = await rl.question(`Enter ${kind} name: `)
      name = answer.trim().toLowerCase().replace(/[^a-z0-9-_]/g, '')
      if (name === '') {
        console.log('Name is required and should contain only alphanumeric characters, dashes, or underscores.')
      }
    }
  } finally {
    rl.close()
  }

  // Normalize prefix from name (e.g. if user entered "app-todo" instead of "todo")
  if (kind === 'app' && name.startsWith('app-')) {
    name = name.slice(4)
  } else if (kind === 'module' && name.startsWith('module-')) {
    name = name.slice(7)
  }

  const pkgShortName = `${kind}-${name}`
  const pkgFullName = `@owdproject/${pkgShortName}`
  const workspaceDir = KINDS[kind].workspaceDir
  const targetDir = join(workspaceRoot, workspaceDir, pkgShortName)

  if (existsSync(targetDir)) {
    throw new Error(`Directory already exists: ${targetDir}`)
  }

  console.log(`\nCreating new ${kind} in ${targetDir}...\n`)

  let templateSrc = null
  let tempCloneDir = null

  if (kind === 'app') {
    const monorepoPath = join(workspaceRoot, 'apps/app-template')
    if (existsSync(join(monorepoPath, 'package.json'))) {
      templateSrc = monorepoPath
    }
  } else if (kind === 'module') {
    const monorepoPath = join(workspaceRoot, 'packages/module-template')
    if (existsSync(join(monorepoPath, 'package.json'))) {
      templateSrc = monorepoPath
    }
  }

  if (!templateSrc) {
    const gitUrl = kind === 'app'
      ? 'git@github.com:owdproject/app-template.git'
      : 'git@github.com:owdproject/module-template.git'

    console.log(`Cloning template from ${gitUrl}...`)
    try {
      const tmpPrefix = join(workspaceRoot, '.owd-template-')
      tempCloneDir = mkdtempSync(tmpPrefix)
      await cloneTemplate(gitUrl, tempCloneDir)
      templateSrc = tempCloneDir
    } catch (err) {
      if (tempCloneDir && existsSync(tempCloneDir)) {
        try {
          rmSync(tempCloneDir, { recursive: true, force: true })
        } catch {}
      }
      throw new Error(`Failed to clone template from GitHub: ${err.message}`)
    }
  }

  // Prepare replacements
  const replacements = {}
  if (kind === 'app') {
    replacements['@owdproject/app-template'] = pkgFullName
    replacements['app-template'] = pkgShortName
    replacements['desktop-app-template'] = `desktop-app-${name}`
    replacements['desktop-app-template-register'] = `desktop-app-${name}-register`
    replacements['appTemplate'] = `app${pascalCase(name)}`
    replacements['AppTemplate'] = `App${pascalCase(name)}`
    replacements['WindowTemplate'] = `Window${pascalCase(name)}`
    replacements['myApp'] = `app${pascalCase(name)}`
    replacements['my-app'] = `app-${name}`
    replacements['ext.domain.app'] = `ext.domain.${name}`
    replacements['Template'] = pascalCase(name)
  } else {
    replacements['@owdproject/module-template'] = pkgFullName
    replacements['module-template'] = pkgShortName
    replacements['desktop-module-template'] = `desktop-module-${name}`
    replacements['desktop-module-template-register'] = `desktop-module-${name}-register`
    replacements['moduleTemplate'] = `module${pascalCase(name)}`
    replacements['ModuleTemplate'] = `Module${pascalCase(name)}`
    replacements['ext.domain.module'] = `ext.domain.${name}`
    replacements['owd-module-template'] = `owd-module-${name}`
    replacements['owd-plugin-template'] = `owd-plugin-${name}`
    replacements['Template'] = pascalCase(name)
  }

  // Copy and replace placeholders
  try {
    copyAndScaffold(templateSrc, targetDir, replacements)
    console.log(`✓ Files scaffolded from template.`)
  } finally {
    if (tempCloneDir && existsSync(tempCloneDir)) {
      try {
        rmSync(tempCloneDir, { recursive: true, force: true })
      } catch {}
    }
  }

  // Run pnpm install in root
  console.log('Running pnpm install in workspace root...')
  await spawnAsync('pnpm', ['install'], { cwd: workspaceRoot })

  // Link package in desktop
  console.log(`Linking ${pkgFullName} in desktop project...`)
  const desktopPath = join(workspaceRoot, 'desktop')
  await linkWorkspacePackage(desktopPath, pkgFullName)

  // Update desktop/desktop.config.ts
  console.log('Registering package in desktop.config.ts...')
  const resolved = resolveDesktopConfigPath(desktopPath)
  const configPath = resolved?.path ?? desktopConfigWritePath(desktopPath)

  const utilPath = join(
    workspaceRoot,
    'node_modules/@owdproject/nx/dist/utils/utilConfig.js',
  )
  if (!existsSync(utilPath)) {
    throw new Error('Workspace install needs @owdproject/nx. Run `pnpm install` at the repo root.')
  }
  const { addToDesktopConfig } = require(utilPath)
  addToDesktopConfig(configPath, KINDS[kind].configKey, pkgFullName)

  // Run dev:prepare
  console.log(`Running dev:prepare on ${pkgFullName}...`)
  try {
    await spawnAsync('pnpm', ['--filter', pkgFullName, 'run', 'dev:prepare'], { cwd: workspaceRoot })
  } catch (err) {
    console.warn(`(warning: dev:prepare failed — it's normal if package does not have the script)`)
  }

  console.log(`\n✓ Scaffolded and registered new ${kind}: ${pkgFullName}\n`)
}
