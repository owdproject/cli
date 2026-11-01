# Desktop creation and control panel roadmap

## Product direction

- `nuxt-desktop`, `react-desktop` and any future framework desktops are private host projects. Publish reusable engines, bindings, extensions, the CLI and the `create-owd` initializer independently; do not publish a host merely to create a project. Vue, Svelte, Angular and Astro hosts remain planned until their own starters exist.
- The CLI should help a user choose a host, create a project, select compatible extensions and understand each operation. A terminal interface complements the package contracts; agent instructions alone cannot check or apply an installation.
- Each generated project owns its `.owd/project.json` profile. Separate project directories have separate profiles. A shared project index and project switching are not implemented.

## Current implementation

- `desktop init` offers Nuxt and React in the terminal; `--framework` supports scripts. It copies a starter from the selected host repository and records the framework in the project profile.
- Nuxt is the non-interactive default. The React repository and its generated project have not been verified, and the existing control panel deliberately refuses React profiles until a React adapter exists.
- The Nuxt control panel rejects explicitly framework-prefixed extensions for other frameworks. It cannot establish UI-kit compatibility when packages do not declare the information.
- Existing workspace packages, npm, Git HTTPS and Git SSH are supported in portions of the current control panel. Arbitrary external local directory installation is not yet a supported `desktop add` flow.
- The `create-owd` npm initializer is a separate distribution. Changes to this repository's `desktop init` do not automatically update an already published initializer.

## Remaining work

1. Provide and verify a real React host starter and adapter. Connect the initializer to the same starter selection, then test both from a clean install.
2. Define machine-readable compatibility metadata for each framework: required and provided integrations, versions and optional features. Use it to explain conflicts such as PrimeVue versus Nuxt UI before making changes; report missing metadata as unknown. Verify the result in standalone hosts.
3. Make package operations recoverable. The current wizard writes desktop configuration before installing packages; failed installs can leave partial changes. Plan, validate and apply changes with a recovery strategy for manifests, lockfiles and newly downloaded sources without overwriting existing work.
4. Exercise install, removal, update, local workspace linking and dev-server flows end to end in generated projects. Lock known compatible versions or Git revisions; Git's latest commit is not automatically compatible with published npm packages.
5. Design project discovery/switching and more playful progress around real task state. Keep the control panel usable without an account and preserve non-interactive CLI commands.
6. Use GitHub Sponsors for bundles. Define a catalog shared by the site and CLI with the repository and sponsor tier for each bundle. GitHub grants the qualifying sponsor access to the private repository; after the sponsor accepts the invitation, the CLI can download it using the user's existing GitHub HTTPS or SSH credentials. If access fails, explain that authentication, invitation acceptance, permission and connectivity are possible causes, and link to the relevant GitHub page. Do not embed paid bundle credentials in a starter. Bundle selection, checkout and download are not implemented yet.

GitHub Sponsors can grant tier sponsors access to an eligible organization-owned private repository, subject to its documented restrictions; only personal accounts can receive these invitations. [GitHub Sponsors documentation](https://docs.github.com/en/sponsors/receiving-sponsorships-through-github-sponsors/managing-your-sponsorship-tiers)
GitHub access is revoked when an applicable sponsorship ends, but copies already downloaded remain on the user's machine. Git authenticates using HTTPS or SSH. [GitHub authentication documentation](https://docs.github.com/en/get-started/git-basics/set-up-git)
