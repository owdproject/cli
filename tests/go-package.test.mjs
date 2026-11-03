import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { delimiter, join } from 'node:path'
import { tmpdir } from 'node:os'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { ensureGoBinary } from '../bin/lib/goBinary.js'

test('package ships Go sources and no locally compiled executables', () => {
  const root = fileURLToPath(new URL('../', import.meta.url))
  const [pack] = JSON.parse(execFileSync(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['pack', '--dry-run', '--json', '--ignore-scripts'], { cwd: root, encoding: 'utf8', shell: process.platform === 'win32' }))
  assert(pack.files.some((entry) => entry.path === 'go/main.go'))
  assert(!pack.files.some((entry) => /go\/(desktop-go|owd-cli)(\.exe)?$/.test(entry.path)))
})

test('uses a source-specific cache instead of the binary left in the package directory', { skip: process.platform === 'win32' }, () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-go-cache-'))
  const previousPath = process.env.PATH
  const previousCache = process.env.XDG_CACHE_HOME
  try {
    const source = join(root, 'source'); const bin = join(root, 'bin')
    mkdirSync(source); mkdirSync(bin)
    writeFileSync(join(source, 'main.go'), 'first')
    writeFileSync(join(source, 'desktop-go'), 'obsolete packaged binary')
    const go = join(bin, 'go')
    writeFileSync(go, `#!${process.execPath}\nconst fs=require('node:fs');const output=process.argv[process.argv.indexOf('-o')+1];fs.writeFileSync(output,'fresh build')\n`)
    chmodSync(go, 0o755)
    process.env.PATH = `${bin}${delimiter}${previousPath}`
    process.env.XDG_CACHE_HOME = join(root, 'cache')
    const first = ensureGoBinary(source)
    assert.equal(readFileSync(first, 'utf8'), 'fresh build')
    assert.equal(ensureGoBinary(source), first)
    writeFileSync(join(source, 'main.go'), 'second')
    assert.notEqual(ensureGoBinary(source), first)
  } finally {
    process.env.PATH = previousPath
    if (previousCache === undefined) delete process.env.XDG_CACHE_HOME
    else process.env.XDG_CACHE_HOME = previousCache
    rmSync(root, { recursive: true, force: true })
  }
})
