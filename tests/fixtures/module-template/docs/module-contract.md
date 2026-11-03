# Module contract

This scaffold targets `@owdproject/nuxt`; consult its [extension contract](https://github.com/owdproject/nuxt/blob/main/docs/extension-contract.md) at a revision matching the installed adapter. Package imports do not load another repository's agent instructions.

The module metadata identifies kind, framework, package, adapter and configuration namespace. Update `configKey` to a namespace specific to the new integration before adding configuration. Declare `requires` for integrations the host must have active, and declare imports in dependencies/peers. Do not infer service availability from a package merely existing on disk.

Expose and document the service API actually implemented: inputs/outputs, registration timing, readiness, errors, subscriptions and teardown. Keep secrets out of public runtime config. Consumers must be able to distinguish a missing optional service from one that is not ready or has failed.

The desktop chooses modules and providers. A service module does not supply a universal permissions model or persistence policy. If it persists state, define ownership, serializable state and restoration order; do not register competing providers without a supported policy. App/window lifecycle semantics remain owned by the host integration.

Before handoff: build, run a standalone private playground with local packs, demonstrate a consumer using the service, and verify failure/cleanup as applicable. Keep demo-only dependencies outside the published runtime. No public npm publication is part of local verification.
