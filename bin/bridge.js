#!/usr/bin/env node

import { writeFileSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  findWorkspaceRoot,
  loadSettings,
  desktopPaths,
  saveSettings
} from './lib/workspace.js'
import {
  readDesktopConfig,
  writeDesktopConfig,
  readDesktopDependencies,
  writeDesktopDependencies
} from './lib/config.js'
import { loadCatalog, mergeInstalled } from './lib/catalog.js'
import { resolveDevTarget } from './lib/playgroundContext.js'
import {
  resolveDesktopConfigPath,
  desktopConfigWritePath
} from './lib/desktopConfig.js'

function getWorkspaceContext() {
  const workspaceRoot = findWorkspaceRoot()
  if (!workspaceRoot) {
    throw new Error('Not inside an OWD workspace')
  }

  const devTarget = resolveDevTarget(process.cwd(), workspaceRoot)
  let paths
  
  if (devTarget && devTarget.mode === 'playground') {
    const playgroundDir = devTarget.playgroundDir
    const resolved = resolveDesktopConfigPath(playgroundDir)
    const configWrite = desktopConfigWritePath(playgroundDir)
    paths = {
      desktop: playgroundDir,
      config: resolved?.path ?? configWrite,
      configWrite,
      configLegacy: resolved?.legacy ?? false,
      packageJson: join(playgroundDir, 'package.json'),
      isPlayground: true,
      packageName: devTarget.packageName,
      packageDir: devTarget.packageDir,
      metaDir: join(playgroundDir, '.desktop'),
    }
  } else {
    paths = {
      ...desktopPaths(workspaceRoot),
      isPlayground: false,
      metaDir: join(workspaceRoot, '.desktop'),
    }
  }

  const settings = loadSettings(workspaceRoot, paths.metaDir)
  
  let config = { theme: null, apps: [], modules: [] }
  try {
    config = readDesktopConfig(paths.config, workspaceRoot)
  } catch (err) {
    console.error('Config load error:', err)
  }
  const deps = readDesktopDependencies(paths.packageJson)
  return { workspaceRoot, settings, paths, config, deps }
}

async function main() {
  const args = process.argv.slice(2)
  const cmd = args[0]

  try {
    if (cmd === '--read') {
      const ctx = getWorkspaceContext()
      console.log(JSON.stringify({
        workspaceRoot: ctx.workspaceRoot,
        settings: ctx.settings,
        config: ctx.config,
        deps: ctx.deps,
        paths: ctx.paths,
      }, null, 2))
    } else if (cmd === '--catalog') {
      const ctx = getWorkspaceContext()
      const force = args.includes('--force')
      const catalog = await loadCatalog(ctx.workspaceRoot, ctx.settings, { force })
      catalog.entries = mergeInstalled(catalog.entries, ctx.config, ctx.deps, ctx.workspaceRoot)
      console.log(JSON.stringify(catalog, null, 2))
    } else if (cmd === '--write') {
      const ctx = getWorkspaceContext()
      
      // Read JSON payload from stdin
      let input = ''
      for await (const chunk of process.stdin) {
        input += chunk
      }
      
      const payload = JSON.parse(input)
      // payload = { config: { theme, apps, modules }, depsToAdd: { name: version }, depsToRemove: [name], settings?: { ... } }
      
      if (payload.config) {
        writeDesktopConfig(ctx.paths.config, ctx.workspaceRoot, payload.config)
      }
      
      if (payload.depsToAdd || payload.depsToRemove) {
        writeDesktopDependencies(
          ctx.paths.packageJson,
          payload.depsToAdd || {},
          payload.depsToRemove || []
        )
      }

      if (payload.settings) {
        saveSettings(ctx.workspaceRoot, payload.settings, ctx.paths.metaDir)
      }
      
      console.log(JSON.stringify({ success: true }))
    } else {
      console.error('Usage: node bridge.js [--read|--catalog [--force]|--write]')
      process.exit(1)
    }
  } catch (err) {
    console.error(JSON.stringify({ error: err.message }))
    process.exit(1)
  }
}

main()
