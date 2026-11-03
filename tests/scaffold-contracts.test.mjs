import assert from 'node:assert/strict'
import { chmodSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve, sep } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { runCreateCli } from '../bin/lib/create.js'
import { writeGeneratedFiles } from '../bin/lib/generateTemplate.js'
import { copyTemplate, scaffoldProject } from '../bin/lib/scaffold.js'

// Default init tests use a complete local starter fixture, without network access.
process.env.OWD_TEMPLATE_PATH = fileURLToPath(new URL('./fixtures/distribution/template', import.meta.url))

const fixtures = resolve(dirname(fileURLToPath(import.meta.url)), 'fixtures')
const ignored = new Set(['node_modules', '.git', '.nuxt', '.output', 'dist', '.data'])

function copySource(from, to) {
  cpSync(from, to, { recursive: true, filter: (path) => !path.split(sep).some((part) => ignored.has(part)) })
}

test('create propagates app/module contracts, metadata and registration without touching the real workspace', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-create-contracts-'))
  const previousPath = process.env.PATH
  const previousLog = process.env.OWD_SCAFFOLD_TEST_LOG
  try {
    mkdirSync(join(root, 'desktop'), { recursive: true })
    mkdirSync(join(root, 'apps'), { recursive: true })
    mkdirSync(join(root, 'packages'), { recursive: true })
    mkdirSync(join(root, 'test-bin'))
    writeFileSync(join(root, 'pnpm-workspace.yaml'), "packages:\n  - 'apps/*'\n  - 'packages/*'\n  - 'desktop'\n")
    writeFileSync(join(root, 'desktop/desktop.config.ts'), "export default defineDesktopConfig({ apps: [], modules: [] })\n")
    copySource(join(fixtures, 'app-template'), join(root, 'apps/app-template'))
    copySource(join(fixtures, 'module-template'), join(root, 'packages/module-template'))

    // Only the external package-manager process is replaced. File generation,
    // substitutions and desktop.config registration run through the real CLI.
    const pnpm = join(root, 'test-bin/pnpm')
    writeFileSync(pnpm, `#!${process.execPath}\nimport('node:fs').then(({ appendFileSync }) => appendFileSync(process.env.OWD_SCAFFOLD_TEST_LOG, JSON.stringify({ cwd: process.cwd(), args: process.argv.slice(2) }) + '\\n'))\n`)
    chmodSync(pnpm, 0o755)
    process.env.PATH = `${join(root, 'test-bin')}${sep === '\\' ? ';' : ':'}${previousPath ?? ''}`
    process.env.OWD_SCAFFOLD_TEST_LOG = join(root, 'commands.jsonl')

    for (const kind of ['app', 'module']) {
      const name = `contract-${kind}`
      await runCreateCli({ workspaceRoot: root, kind, name })
      const shortName = `nuxt-${kind}-${name}`
      const directory = join(root, kind === 'app' ? 'apps' : 'packages', shortName)
      assert.equal(JSON.parse(readFileSync(join(directory, 'package.json'), 'utf8')).name, `@owdproject/${shortName}`)
      const guidance = readFileSync(join(directory, 'AGENTS.md'), 'utf8')
      assert.match(guidance, new RegExp(`docs/${kind}-contract\\.md`))
      assert.ok(existsSync(join(directory, `docs/${kind}-contract.md`)))
      assert.doesNotMatch(guidance, /MineHub|GameAssist|\/home\//)
      const moduleSource = readFileSync(join(directory, 'src/module.ts'), 'utf8')
      assert.ok(moduleSource.includes(`kind: '${kind}'`))
      assert.ok(moduleSource.includes("framework: 'nuxt'"))
      assert.ok(moduleSource.includes(`packageName: '@owdproject/${shortName}'`))
      assert.ok(moduleSource.includes("adapter: '@owdproject/nuxt'"))
      assert.ok(moduleSource.includes(`configKey: 'contract${kind === 'app' ? 'App' : 'Module'}'`))
      assert.doesNotMatch(moduleSource, /@owdproject\/(?:app|module)-template/)
      assert.ok(readFileSync(join(root, 'desktop/desktop.config.ts'), 'utf8').includes(`@owdproject/${shortName}`))
    }

    const commands = readFileSync(process.env.OWD_SCAFFOLD_TEST_LOG, 'utf8').trim().split('\n').map(JSON.parse)
    assert.equal(commands.length, 6)
    assert.equal(commands.filter(({ args }) => args[0] === 'install').length, 2)
    assert.equal(commands.filter(({ args }) => args[0] === 'add').length, 2)
  } finally {
    if (previousPath === undefined) delete process.env.PATH
    else process.env.PATH = previousPath
    if (previousLog === undefined) delete process.env.OWD_SCAFFOLD_TEST_LOG
    else process.env.OWD_SCAFFOLD_TEST_LOG = previousLog
    rmSync(root, { recursive: true, force: true })
  }
})

test('starter generation and init copying retain blueprint guidance and the configured theme without private paths', () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-starter-contracts-'))
  try {
    const clientRoot = join(fixtures, 'distribution')
    const desktop = JSON.parse(readFileSync(join(clientRoot, 'template/desktop/package.json'), 'utf8'))
    const starter = JSON.parse(readFileSync(join(clientRoot, 'template/package.json'), 'utf8'))
    const workspacePackage = JSON.parse(readFileSync(join(clientRoot, 'package.json'), 'utf8'))
    assert.equal(starter.name, 'owd-nuxt-desktop')
    assert.equal(workspacePackage.name, starter.name)
    const versions = { ...starter.devDependencies, ...desktop.dependencies }
    const generated = join(root, 'generated')
    const consumer = join(root, 'consumer')
    writeGeneratedFiles(clientRoot, generated, { versions })
    assert.equal(JSON.parse(readFileSync(join(generated, 'package.json'), 'utf8')).name, starter.name)
    copyTemplate(generated, consumer)
    const blueprintGuidance = readFileSync(join(clientRoot, 'template-blueprint/AGENTS.md'), 'utf8')
    assert.equal(readFileSync(join(consumer, 'AGENTS.md'), 'utf8'), blueprintGuidance)
    assert.match(blueprintGuidance, /desktop create app/)
    assert.match(blueprintGuidance, /desktop create module/)
    assert.doesNotMatch(blueprintGuidance, /MineHub|GameAssist|\/home\//)
    assert.match(readFileSync(join(consumer, 'desktop/desktop.config.ts'), 'utf8'), /@owdproject\/nuxt-theme-ui/)
    assert.equal(JSON.parse(readFileSync(join(consumer, 'desktop/package.json'), 'utf8')).dependencies['@owdproject/nuxt'], versions['@owdproject/nuxt'])
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test('init without a directory uses the Nuxt desktop workspace name', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-init-default-'))
  try {
    const result = await scaffoldProject({ cwd: root, skipInstall: true, skipUi: true })
    assert.equal(result.dir, 'owd-nuxt-desktop')
    assert.ok(existsSync(join(result.targetDir, 'desktop/desktop.config.ts')))
    assert.equal(JSON.parse(readFileSync(join(result.targetDir, 'package.json'), 'utf8')).name, 'owd-nuxt-desktop')
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})
