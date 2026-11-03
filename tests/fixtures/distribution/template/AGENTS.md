# Nuxt desktop project

This project composes a Nuxt desktop through `desktop/desktop.config.ts`. Its default theme is Nuxt UI with the About app. It is a host project, not the framework-independent engine.

- Use `@owdproject/nuxt` for configuration, orchestration, app registration and shell APIs. Read the [Nuxt extension contract](https://github.com/owdproject/nuxt/blob/main/docs/extension-contract.md) at the revision matching your adapter.
- The theme owns visual design. App, theme, module and kit requirements must be explicit; do not assume every theme supplies the same UI kit or services. No universal tokens or familiar OS layout are required.
- Use `pnpm desktop create app <name>` or `pnpm desktop create module <name>` for Nuxt scaffolds. Their local AGENTS.md and role contracts apply to the generated extension. The control panel's new-package action invokes the same wizard.
- Keep application-specific code under its extension; register via public APIs instead of creating unmanaged windows. Keep playground demo lists separate from runtime dependencies.
- Persistence/authentication/filesystems are optional integrations. Verify provider-specific restoration and errors before claiming support.
- Run `pnpm dev` for development and `pnpm generate` for static output. Verify launch, close, reopening and required services in the actual desktop; a build alone does not establish those behaviors.
- Consult tooling for the current layer: Nuxt/Vue for this host, dependency documentation as needed, browser tools for interaction checks. Do not import another project's private stack or business rules.
- Preserve user changes. Never depend on deprecated unscoped `nuxt-desktop`; the scoped `@owdproject/nuxt-desktop` distribution is allowed. Local verification does not authorize public npm publication or deployment.
