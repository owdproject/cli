# Nuxt application authoring

This repository is a Nuxt application extension, not a theme or an independent engine. Read [the local app contract](docs/app-contract.md) before changing it. This guidance travels with projects created from this scaffold.

- Keep application identity, entries, commands and window models in `src/runtime/app.config.ts`; register through the public Nuxt app API in `src/runtime/plugin.ts`.
- Declare actual framework, adapter, kit and service requirements in the module metadata and package dependencies. Shared headless logic may be portable; Vue components require their own host integration.
- Use application/window lifecycle APIs and clean up resources. Do not bypass the application manager to create unmanaged windows.
- Keep demo packages in the private playground, outside published dependencies and artifacts. Verify against a compatible theme; its UI kit is not universal.
- Consult Nuxt documentation tools for Nuxt work and the relevant library tools for dependencies actually used here. There is no mandatory visual token system or OS layout.
- Preserve user changes. Never depend on deprecated unscoped `nuxt-desktop`; scoped `@owdproject/nuxt-desktop` is allowed. Use local packs/Verdaccio for standalone checks; no public publication during verification.
