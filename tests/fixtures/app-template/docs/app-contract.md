# Application contract

This scaffold targets `@owdproject/nuxt`. The adapter's [extension contract](https://github.com/owdproject/nuxt/blob/main/docs/extension-contract.md) describes the integration; use the revision matching the installed adapter. Importing core does not automatically load its agent instructions.

The module's metadata describes its kind, framework and adapter; `packageName` must match the generated package. Declare required host kits/modules through `requires` when used. Dependencies describe code imports; host integration requirements describe services that must actually be active. Document supported versions in the manifest and supported playground combinations. Do not invent a generic version-negotiation API.

Register the app through `src/runtime/plugin.ts`. Describe windows, entries and commands in `src/runtime/app.config.ts`. Use the registered application's public actions for opening/closing windows. A window component owns cleanup on unmount; closing a window must not be assumed to terminate the process or invoke every application callback.

The application owns its content and logic. The theme owns shell chrome and presentation; requiring a particular UI kit is permitted and must be explicit. Persistence, filesystems and authentication are optional services, not implicit features of every desktop. If used, specify what state is serializable and how missing service/readiness/errors are handled.

Before handing off: build the extension, run its standalone private playground with locally packed dependencies, and verify launch, close and reopening through the real host. Test any declared required service. Keep playground-only apps out of the published package. Do not treat a successful build as proof of persistence or cross-framework compatibility.
