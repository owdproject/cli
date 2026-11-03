import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { runCli } from '../bin/cli.js'
import { scaffoldProject } from '../bin/lib/scaffold.js'
import { runNpmInstall } from '../bin/lib/install.js'

// Default init tests use a complete local starter fixture, without network access.
process.env.OWD_TEMPLATE_PATH = fileURLToPath(new URL('./fixtures/distribution/template', import.meta.url))

const cli = resolve(dirname(fileURLToPath(import.meta.url)), '../bin/desktop.js')

function run(command, args, cwd) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd, stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = ''
    let stderr = ''
    child.stdout.on('data', (chunk) => { stdout += chunk })
    child.stderr.on('data', (chunk) => { stderr += chunk })
    child.once('error', reject)
    child.once('close', (status) => resolve({ status, stdout, stderr }))
  })
}

async function gitRepo(root, framework, withTemplate) {
  const repo = join(root, `${framework}-source`)
  mkdirSync(repo)
  const starter = withTemplate ? join(repo, 'template') : repo
  mkdirSync(starter, { recursive: true })
  writeFileSync(join(starter, 'package.json'), JSON.stringify({ name: `${framework}-fixture`, private: true }))
  writeFileSync(join(starter, 'pnpm-workspace.yaml'), "packages:\n  - 'desktop'\n")
  mkdirSync(join(starter, 'desktop'))
  writeFileSync(join(starter, 'desktop/package.json'), '{"name":"fixture-host","private":true}')
  for (const args of [
    ['init', '-q', repo],
    ['-C', repo, 'add', '.'],
    ['-C', repo, '-c', 'user.name=OWD test', '-c', 'user.email=owd-test@example.invalid', 'commit', '-qm', 'fixture'],
  ]) {
    const result = await run('git', args, root)
    assert.equal(result.status, 0, result.stderr)
  }
  return repo
}

test('init chooses each starter and records an isolated project profile', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-starters-'))
  try {
    const nuxt = await gitRepo(root, 'nuxt', true)
    const react = await gitRepo(root, 'react', false)
    for (const [framework, repo] of [['nuxt', nuxt], ['react', react]]) {
      const result = await run(process.execPath, [cli, 'init', `my-${framework}`, '--framework', framework,
        '--starter-repo', repo, '--skip-install', '--skip-ui'], root)
      assert.equal(result.status, 0, result.stderr + result.stdout)
      const project = join(root, `my-${framework}`)
      assert.equal(JSON.parse(readFileSync(join(project, 'package.json'), 'utf8')).name, `${framework}-fixture`)
      assert.deepEqual(JSON.parse(readFileSync(join(project, '.owd/project.json'), 'utf8')), { schemaVersion: 1, framework })
      assert.equal(existsSync(join(project, '.git')), false)
      assert.match(result.stderr, new RegExp(`owd-scaffold-[^/]+/${framework}-desktop`))
      assert.doesNotMatch(result.stderr, /owd-scaffold-[^/]+\/client/)
    }
    // Exercise framework validation through a noninteractive command. Opening a
    // TUI without a terminal tests the terminal environment, not React support.
    const reactHelp = await run(process.execPath, [cli, '--help'], join(root, 'my-react'))
    assert.equal(reactHelp.status, 0, reactHelp.stderr)
    assert.match(reactHelp.stdout, /react/)
    const previousCwd = process.cwd()
    const previousLog = console.log
    let output = ''
    try {
      process.chdir(join(root, 'my-nuxt'))
      console.log = (...args) => { output += args.join(' ') }
      await runCli('desktop', ['add', 'app', 'nuxt-app-sample', '--dry-run'])
    } finally {
      console.log = previousLog
      process.chdir(previousCwd)
    }
    assert.match(output, /@owdproject\/nuxt-app-sample/)
    assert.doesNotMatch(output, /app-nuxt-app/)
    const incompatible = await run(process.execPath, [cli, 'add', 'react-app-sample', '--dry-run'], join(root, 'my-nuxt'))
    assert.notEqual(incompatible.status, 0)
    assert.match(incompatible.stderr, /targets react/)
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test('an unknown framework fails without creating a project', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-invalid-starter-'))
  try {
    const result = await run(process.execPath, [cli, 'init', 'bad', '--framework', 'unknown', '--skip-install'], root)
    assert.notEqual(result.status, 0)
    assert.match(result.stderr, /Unknown starter/)
    assert.equal(existsSync(join(root, 'bad')), false)
    for (const framework of ['vue', 'svelte', 'angular', 'astro']) {
      const dir = `${framework}-project`
      const planned = await run(process.execPath, [cli, 'init', dir, '--framework', framework, '--skip-install'], root)
      assert.notEqual(planned.status, 0)
      assert.match(planned.stderr, /planned but is not available yet/)
      assert.equal(existsSync(join(root, dir)), false)
    }
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test('initializing the current folder requires it to be empty', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-current-'))
  const empty = join(root, 'empty')
  const occupied = join(root, 'occupied')
  mkdirSync(empty)
  mkdirSync(occupied)
  writeFileSync(join(occupied, 'keep.txt'), 'original')
  try {
    await scaffoldProject({ cwd: empty, dir: '.', skipInstall: true, skipUi: true })
    assert.equal(JSON.parse(readFileSync(join(empty, '.owd/project.json'), 'utf8')).framework, 'nuxt')
    await assert.rejects(scaffoldProject({ cwd: occupied, dir: '.', skipInstall: true, skipUi: true }), /not empty/)
    assert.equal(readFileSync(join(occupied, 'keep.txt'), 'utf8'), 'original')
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test('help describes the Bubble Tea shortcuts used by the panel', async () => {
  const previousLog = console.log
  let output = ''
  try {
    console.log = (...args) => { output += args.join(' ') }
    await runCli('desktop', ['--help'])
  } finally {
    console.log = previousLog
  }
  assert.match(output, /d \/ x\s+Start \/ stop/)
  assert.match(output, /s\s+Apply queued/)
  assert.match(output, /f\s+Find the optional Nuxt file service/)
  assert.doesNotMatch(output, /s\s+Start the Nuxt dev server/)
})

test('npm install updates a fresh starter without repository-only scripts', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-npm-starter-'))
  const oldPath = process.env.PATH
  const oldLog = process.env.OWD_PNPM_TEST_LOG
  try {
    const { targetDir } = await scaffoldProject({ cwd: root, dir: 'desktop', skipInstall: true, skipUi: true })
    assert.equal(existsSync(join(targetDir, 'scripts/install-app.mjs')), false)
    const fakeBin = join(root, 'bin')
    mkdirSync(fakeBin)
    const pnpm = join(fakeBin, 'pnpm')
    writeFileSync(pnpm, `#!${process.execPath}\nimport('node:fs').then(({ writeFileSync }) => writeFileSync(process.env.OWD_PNPM_TEST_LOG, JSON.stringify({ cwd: process.cwd(), args: process.argv.slice(2) })))\n`)
    chmodSync(pnpm, 0o755)
    process.env.PATH = `${fakeBin}${delimiter}${oldPath ?? ''}`
    process.env.OWD_PNPM_TEST_LOG = join(root, 'pnpm.json')
    await runNpmInstall('app', '@owdproject/nuxt-app-sample', targetDir, 'pipe')
    const call = JSON.parse(readFileSync(process.env.OWD_PNPM_TEST_LOG, 'utf8'))
    assert.equal(call.cwd, join(targetDir, 'desktop'))
    assert.deepEqual(call.args, ['add', '@owdproject/nuxt-app-sample'])
    const configPath = join(targetDir, 'desktop/desktop.config.ts')
    const afterInstall = readFileSync(configPath, 'utf8')
    assert.match(afterInstall, /@owdproject\/nuxt-app-sample/)
    writeFileSync(pnpm, `#!${process.execPath}\nprocess.exit(1)\n`)
    await assert.rejects(runNpmInstall('app', '@owdproject/nuxt-app-failing', targetDir, 'pipe'))
    assert.equal(readFileSync(configPath, 'utf8'), afterInstall)
  } finally {
    if (oldPath === undefined) delete process.env.PATH
    else process.env.PATH = oldPath
    if (oldLog === undefined) delete process.env.OWD_PNPM_TEST_LOG
    else process.env.OWD_PNPM_TEST_LOG = oldLog
    rmSync(root, { recursive: true, force: true })
  }
})
