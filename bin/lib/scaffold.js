import { execSync, spawn, spawnSync } from 'node:child_process'
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const NUXT_TEMPLATE = resolve(__dirname, '../../../../template')
export const STARTER_REPOS = {
  nuxt: 'https://github.com/owdproject/nuxt-desktop.git',
  react: 'https://github.com/owdproject/react-desktop.git',
}
export const PLANNED_STARTERS = ['vue', 'svelte', 'angular', 'astro']

export function validateFramework(framework) {
  if (PLANNED_STARTERS.includes(framework)) {
    throw new Error(`The ${framework} desktop starter is planned but is not available yet.`)
  }
  if (!Object.hasOwn(STARTER_REPOS, framework)) {
    throw new Error(`Unknown starter "${framework}". Choose nuxt or react.`)
  }
  return framework
}

/**
 * @param {string} [startDir]
 */
export function resolveLocalTemplatePath(startDir = process.cwd(), framework = 'nuxt') {
  validateFramework(framework)
  const fromEnv = (framework === 'react' ? process.env.OWD_REACT_TEMPLATE_PATH : process.env.OWD_TEMPLATE_PATH)?.trim()
  if (fromEnv && existsSync(join(fromEnv, 'package.json'))) {
    return resolve(fromEnv)
  }

  // The template beside this CLI belongs to the Nuxt host only.
  if (framework === 'react') return null
  if (existsSync(join(NUXT_TEMPLATE, 'package.json'))) {
    return NUXT_TEMPLATE
  }

  let dir = startDir
  for (;;) {
    const candidate = join(dir, 'template')
    if (
      existsSync(join(candidate, 'package.json')) &&
      existsSync(join(dir, 'pnpm-workspace.yaml'))
    ) {
      return candidate
    }
    const parent = dirname(dir)
    if (parent === dir) break
    dir = parent
  }

  return null
}

/**
 * @param {string} templatePath
 * @param {string} targetDir
 */
export function copyTemplate(templatePath, targetDir) {
  const excluded = new Set(['.git', 'node_modules', '.nuxt', '.output'])
  cpSync(templatePath, targetDir, {
    recursive: true,
    filter: (src) => !relative(templatePath, src).split(sep).some((part) => excluded.has(part)),
  })
}

function cloneGit(repo, targetDir) {
  return new Promise((resolve, reject) => {
    const child = spawn('git', ['clone', '--depth', '1', repo, targetDir], { stdio: 'inherit' })
    child.once('error', reject)
    child.once('close', (code) => {
      if (code === 0) resolve()
      else reject(new Error(`Could not clone starter repository ${repo} (git exit ${code}).`))
    })
  })
}

/**
 * @param {string} targetDir
 * @param {{ repo?: string, templatePath?: string | null, cwd?: string, framework?: 'nuxt' | 'react', setupType?: 'template' | 'clone' }} [options]
 */
export async function cloneTemplate(targetDir, options = {}) {
  const setupType = options.setupType ?? 'template'
  const framework = validateFramework(options.framework ?? 'nuxt')
  const repo = options.repo || STARTER_REPOS[framework]

  if (setupType === 'clone') {
    console.log()
    await cloneGit(repo, targetDir)
    return { source: 'git', path: repo }
  }

  const localTemplate = options.templatePath ?? (options.repo ? null : resolveLocalTemplatePath(options.cwd, framework))

  if (localTemplate) {
    copyTemplate(localTemplate, targetDir)
    return { source: 'local', path: localTemplate }
  }

  const tempParent = mkdtempSync(join(tmpdir(), 'owd-scaffold-'))
  const tempClone = join(tempParent, `${framework}-desktop`)

  try {
    console.log()
    await cloneGit(repo, tempClone)

    const candidate = join(tempClone, 'template')
    const templatePath = existsSync(join(candidate, 'package.json'))
      ? candidate
      : framework === 'react' && existsSync(join(tempClone, 'package.json')) ? tempClone : null
    if (!templatePath) {
      throw new Error(`The ${framework} desktop repository does not contain a template/ starter.`)
    }

    copyTemplate(templatePath, targetDir)
    return { source: 'git', path: repo }
  } finally {
    try {
      rmSync(tempParent, { recursive: true, force: true })
    } catch {
      /* ignore */
    }
  }
}

/**
 * @param {string} targetDir
 */
export function runPostScaffold(targetDir) {
  execSync('pnpm install', { cwd: targetDir, stdio: 'inherit' })
}

export function canLaunchUi() {
  return Boolean(process.stdout.isTTY) && !process.env.CI
}

/**
 * @param {string} targetDir
 * @param {string} [commandName]
 */
export function launchControlPanel(targetDir, commandName = 'desktop') {
  const relDir = basename(targetDir)

  if (!canLaunchUi()) {
    console.log(`\nNext: cd ${relDir} && pnpm desktop\n`)
    return false
  }

  console.log('\nOpening control panel…\n')
  const result = spawnSync('pnpm', ['exec', commandName], {
    cwd: targetDir,
    stdio: 'inherit',
    shell: false,
  })

  if (result.status !== 0 && result.status !== null) {
    console.log(`\nControl panel exited. Run again with: cd ${relDir} && pnpm desktop\n`)
  }

  return true
}

/**
 * @param {{ dir?: string, cwd?: string, commandName?: string, skipInstall?: boolean, skipUi?: boolean, repo?: string, templatePath?: string | null, framework?: 'nuxt' | 'react', setupType?: 'template' | 'clone' }} [options]
 */
export async function scaffoldProject(options = {}) {
  const cwd = options.cwd ?? process.cwd()
  const framework = validateFramework(options.framework ?? 'nuxt')
  const dir = (options.dir ?? `owd-${framework}-desktop`).trim()
  const commandName = options.commandName ?? 'desktop'
  const setupType = options.setupType ?? 'template'

  if (!dir) {
    throw new Error('Project directory name is required.')
  }

  const targetDir = resolve(cwd, dir)

  if (existsSync(targetDir)) {
    const isEmptyCurrentDir = targetDir === resolve(cwd) && statSync(targetDir).isDirectory() && readdirSync(targetDir).length === 0
    if (!isEmptyCurrentDir && existsSync(join(targetDir, 'package.json'))) {
      throw new Error(`Directory "${dir}" already exists and contains a package.json.`)
    }
    if (!isEmptyCurrentDir) throw new Error(`Directory "${dir}" already exists and is not empty.`)
  }

  const cloneResult = await cloneTemplate(targetDir, {
    repo: options.repo,
    templatePath: options.templatePath,
    cwd,
    setupType,
    framework,
  })

  const profileDir = join(targetDir, '.owd')
  const profilePath = join(profileDir, 'project.json')
  if (existsSync(profilePath)) {
    const profile = JSON.parse(readFileSync(profilePath, 'utf8'))
    if (profile.framework !== framework) throw new Error(`Starter profile in ${profilePath} does not match ${framework}.`)
  } else {
    mkdirSync(profileDir, { recursive: true })
    writeFileSync(profilePath, JSON.stringify({ schemaVersion: 1, framework }, null, 2) + '\n')
  }

  if (cloneResult.source === 'local') {
    console.log(`Using local template: ${cloneResult.path}\n`)
  }

  if (!options.skipInstall) {
    console.log('Installing dependencies (pnpm install)…\n')
    try {
      runPostScaffold(targetDir)
    } catch (error) {
      throw new Error(`Dependency installation failed in ${targetDir}. The project files were kept. Check the error above; if pnpm requests build approval, run \`pnpm approve-builds\`, then \`pnpm install\` again from that directory.`, { cause: error })
    }
  }

  console.log(options.skipInstall
    ? '\n✓ Starter created; dependencies have not been installed\n'
    : '\n✓ Project ready\n')

  if (!options.skipUi && options.skipInstall) {
    console.log(framework === 'react'
      ? 'Install dependencies before starting the dev server.'
      : 'Install dependencies before opening the control panel.')
  } else if (!options.skipUi && framework === 'nuxt') {
    launchControlPanel(targetDir, commandName)
  } else if (!options.skipUi && framework === 'react') {
    console.log('The React control panel is not implemented yet; use the project scripts.')
  }

  // Framework-aware next steps: Nuxt hosts run the control panel, React hosts
  // run their own Vite scripts. Skip `cd` when the starter is created in place.
  const runCommand = framework === 'react' ? 'pnpm dev' : 'pnpm desktop'
  const inPlace = resolve(cwd) === targetDir
  if (!inPlace) console.log(`  cd ${dir}`)
  if (options.skipInstall) console.log('  pnpm install')
  console.log(`  ${runCommand}\n`)

  return { targetDir, dir, framework }
}

/** Copy scaffold.js into create-owd for npm publish (run from repo root). */
export function vendorScaffoldForCreateOwd(createOwdRoot) {
  const src = join(__dirname, 'scaffold.js')
  const destDir = join(createOwdRoot, 'src/lib')
  const dest = join(destDir, 'scaffold.js')
  mkdirSync(destDir, { recursive: true })
  writeFileSync(dest, readFileSync(src, 'utf8'))
  return dest
}
