import { execSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { desktopMetaDir } from './workspace.js'

const CACHE_TTL_MS = 60 * 60 * 1000
const SCOPE = '@owdproject/'

/**
 * dist-tag used to resolve `@owdproject/*` versions for the generated template.
 * During the v4 alpha cycle the packages are published under `alpha`, while
 * `latest` still points at the previous stable (e.g. core 3.x). Override with
 * `OWD_TEMPLATE_TAG` (e.g. `latest` after a stable release).
 */
export const OWD_TEMPLATE_TAG = process.env.OWD_TEMPLATE_TAG || 'alpha'

/**
 * @param {string} version
 */
export function formatCaretVersion(version) {
  const trimmed = String(version).trim()
  if (!trimmed) throw new Error('Invalid npm version (empty)')
  if (trimmed.startsWith('^') || trimmed.startsWith('~') || trimmed.startsWith('>=')) {
    return trimmed
  }
  return `^${trimmed}`
}

/**
 * @param {unknown} error
 */
function isNpmLookupSkippableError(error) {
  const msg = error?.stderr?.toString?.() || error?.message || String(error)
  return (
    /E404|404 Not Found/.test(msg) ||
    /EAI_AGAIN|ENOTFOUND|ETIMEDOUT|ECONNREFUSED|network/i.test(msg)
  )
}

/**
 * @param {string} pkgName
 * @param {{ optional?: boolean }} [options]
 * @returns {string | null}
 */
function npmViewVersion(spec, optional) {
  try {
    return execSync(`npm view ${JSON.stringify(spec)} version`, {
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe'],
    }).trim()
  } catch (error) {
    if (optional || isNpmLookupSkippableError(error)) return null
    const msg = error.stderr?.toString?.() || error.message || String(error)
    throw new Error(`npm view failed for ${spec}: ${msg}`)
  }
}

/**
 * Resolves the version to use for a package. When a `tag` is given (e.g. `alpha`
 * for `@owdproject/*`), it is tried first and falls back to `latest` when the tag
 * does not exist.
 *
 * @param {string} pkgName
 * @param {{ optional?: boolean, tag?: string }} [options]
 * @returns {string | null}
 */
export function fetchLatestVersion(pkgName, options = {}) {
  if (options.tag) {
    const tagged = npmViewVersion(`${pkgName}@${options.tag}`, true)
    if (tagged) return formatCaretVersion(tagged)
  }
  const out = npmViewVersion(pkgName, options.optional)
  return out ? formatCaretVersion(out) : null
}

/**
 * @param {string[]} pkgNames
 * @param {{ optional?: boolean, tag?: string }} [options]
 * @returns {Record<string, string>}
 */
export function fetchLatestVersions(pkgNames, options = {}) {
  const unique = [...new Set(pkgNames.filter(Boolean))]
  if (unique.length === 0) return {}

  /** @type {Record<string, string>} */
  const result = {}
  for (const name of unique) {
    const tag = options.tag ?? (isOwdScopedPackage(name) ? OWD_TEMPLATE_TAG : undefined)
    const version = fetchLatestVersion(name, { ...options, tag })
    if (version) result[name] = version
  }
  return result
}

function cachePath(workspaceRoot) {
  return join(desktopMetaDir(workspaceRoot), 'template-versions.json')
}

/**
 * @param {string} workspaceRoot
 * @param {string[]} pkgNames
 * @param {{ useCache?: boolean }} [options]
 */
export function resolveLatestVersions(workspaceRoot, pkgNames, options = {}) {
  const useCache = options.useCache !== false
  const unique = [...new Set(pkgNames.filter(Boolean))]
  const path = cachePath(workspaceRoot)

  if (useCache && existsSync(path)) {
    try {
      const data = JSON.parse(readFileSync(path, 'utf8'))
      if (
        Date.now() - data.fetchedAt < CACHE_TTL_MS &&
        data.versions &&
        data.tag === OWD_TEMPLATE_TAG
      ) {
        const cached = data.versions
        const missing = unique.filter((name) => !cached[name])
        if (missing.length === 0) {
          return Object.fromEntries(unique.map((name) => [name, cached[name]]))
        }
        const fresh = fetchLatestVersions(missing)
        const merged = { ...cached, ...fresh }
        writeVersionCache(workspaceRoot, merged)
        return Object.fromEntries(unique.map((name) => [name, merged[name]]))
      }
    } catch {
      /* ignore corrupt cache */
    }
  }

  const versions = fetchLatestVersions(unique)
  if (useCache) writeVersionCache(workspaceRoot, versions)
  return versions
}

/**
 * @param {string} workspaceRoot
 * @param {Record<string, string>} versions
 */
export function writeVersionCache(workspaceRoot, versions) {
  mkdirSync(desktopMetaDir(workspaceRoot), { recursive: true })
  writeFileSync(
    cachePath(workspaceRoot),
    JSON.stringify({ fetchedAt: Date.now(), tag: OWD_TEMPLATE_TAG, versions }, null, 2) + '\n',
  )
}

/**
 * @param {string} pkgName
 */
export function isOwdScopedPackage(pkgName) {
  return pkgName.startsWith(SCOPE)
}
