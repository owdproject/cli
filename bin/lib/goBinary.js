import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { homedir } from 'node:os'
import { execFileSync } from 'node:child_process'

/** Cache only binaries compiled for this platform from these exact Go sources. */
export function ensureGoBinary(goDir) {
  const hash = createHash('sha256').update(`${process.platform}/${process.arch}`)
  function scan(dir) {
    for (const entry of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) scan(path)
      else if (/\.go$|^go\.(mod|sum)$/.test(entry.name)) hash.update(path.slice(goDir.length)).update(readFileSync(path))
    }
  }
  scan(goDir)
  const cache = join(process.env.XDG_CACHE_HOME || join(homedir(), '.cache'), 'owd-cli', hash.digest('hex'))
  const binary = join(cache, process.platform === 'win32' ? 'desktop-go.exe' : 'desktop-go')
  if (!existsSync(binary)) {
    mkdirSync(cache, { recursive: true })
    // go build writes the result atomically; concurrent launches can share this cache.
    execFileSync('go', ['build', '-buildvcs=false', '-o', binary, '.'], { cwd: goDir, stdio: 'inherit' })
  }
  return binary
}
