import assert from 'node:assert/strict'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { loadCatalog } from '../bin/lib/catalog.js'

test('an unreachable catalog reports offline instead of claiming live results', async () => {
  const root = mkdtempSync(join(tmpdir(), 'owd-offline-'))
  const fetchBefore = globalThis.fetch
  globalThis.fetch = async () => { throw new Error('offline') }
  try {
    const catalog = await loadCatalog(root, { githubOrgs: ['owdproject'] })
    assert.equal(catalog.cacheAge, 'offline')
  } finally {
    globalThis.fetch = fetchBefore
    rmSync(root, { recursive: true, force: true })
  }
})
