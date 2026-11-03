# Nuxt service module authoring

This repository is a Nuxt module extension. Read [the local module contract](docs/module-contract.md) before changing it. This guidance travels with projects created from this scaffold.

- Own one explicit service/integration and its configuration namespace. Keep reusable framework-independent implementation outside Nuxt presentation/orchestration code.
- Declare actual adapter and host integration requirements, supported versions, readiness, failures and teardown. Use public `@owdproject/nuxt` authoring APIs.
- A module does not own the desktop's app list or theme design. Do not install demo apps into consumer dependencies or impose global visual tokens.
- Keep credentials out of public runtime config. Optional capabilities need explicit checks and documented fallback or errors.
- Consult documentation/MCP tools for the layer and dependencies actually used here. Preserve user changes and verify in the private standalone playground with local packs/Verdaccio.
- Never depend on deprecated unscoped `nuxt-desktop`. The scoped distribution is allowed. Do not publish to public npm during verification.
