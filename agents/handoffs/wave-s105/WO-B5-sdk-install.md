# WO-B5 (S105) — beacon SDK install paths + release workflow (npm name, tarball, guards)

**Review finding (Issue 7, P0, verified):** `npm install @pulse/beacon` 404s (never published,
no publish workflow); the Swift README's SPM URL snippet is doubly unresolvable (Package.swift
not at repo root; no 0.1.0 tag). Also (Issue 5) release.yml has no version-consistency guard,
and no GitHub Release assets are attached today.

**Decisions already made for you:**
- npm package name → **`ams-pulse-beacon`** (unscoped; verified unclaimed on registry.npmjs.org
  today). Version → align to **0.4.1**.
- npm credentials do NOT exist on this host or in repo secrets — the docs must lead with the
  install path that works TODAY (tarball from the GitHub release; the orchestrator attaches
  `ams-pulse-beacon-0.4.1.tgz` to release v0.4.1 after this wave merges), with the npm-registry
  path documented as "available once published" + automated in the workflow.
- Swift: document local-path SPM integration (clone + `.package(path:)`) and source-drop; remove
  the URL+`from: "0.1.0"` snippet; note a dedicated tagged SPM repo is planned.

## Scope — you may ONLY edit these files
- `sdk/beacon-js/package.json` (and `package-lock.json` via `npm install --package-lock-only`
  or by running npm install in that dir to refresh the lock's name/version fields)
- `sdk/beacon-js/README.md`
- `docs/beacon-sdk.md`
- `docs/user-guide.md` — ONLY the beacon install snippet(s) if they reference @pulse/beacon
- `web/src/features/settings/SettingsPage.tsx` — ONLY the displayed install snippet referencing
  @pulse/beacon (grep confirmed it appears there), plus its test file if the string is asserted
- `sdk/beacon-swift/README.md`
- `.github/workflows/release.yml`

Do NOT touch sdk/beacon-js/src/* internals except where the package name appears in
comments/headers; do NOT touch LICENSE files (another agent owns them). No git commands.

## Work items

1. **`sdk/beacon-js/package.json`:** `name` → `ams-pulse-beacon`, `version` → `0.4.1`; add
   `repository` (git+https://github.com/aytekXR/ams-pulse.git, directory sdk/beacon-js),
   `homepage`, `bugs` (support@beyondkaira.com or repo issues); ensure `files` includes dist +
   README + LICENSE so `npm pack` yields a complete tarball. Refresh the lockfile name/version.
   Run `npm run build && npm run size` in sdk/beacon-js — the 15 KB gate must stay green — and
   `npm pack --dry-run` to verify tarball contents.

2. **Install docs (`sdk/beacon-js/README.md`, `docs/beacon-sdk.md`, user-guide snippet,
   SettingsPage snippet):** replace every `@pulse/beacon` reference. Install section pattern:
   - Option A (works today): download `ams-pulse-beacon-0.4.1.tgz` from the GitHub release
     (https://github.com/aytekXR/ams-pulse/releases) → `npm install ./ams-pulse-beacon-0.4.1.tgz`
   - Option B: `npm install ams-pulse-beacon` — note: publishing to the npm registry is automated
     in the release workflow and pending credentials; until the package page exists this option
     is listed as "coming".
   - Import statements in examples change from `@pulse/beacon` to `ams-pulse-beacon`.
   - CHANGELOG.md historical mentions: leave untouched.
   - For SettingsPage.tsx: keep the change minimal (string/snippet only); run the related web
     test if one asserts the string (`cd web && npx vitest run src/features/settings` or the
     project's equivalent) and fix the assertion.

3. **`sdk/beacon-swift/README.md`:** replace the SPM URL snippet with honest working options:
   (a) local path — clone the repo and `.package(path: "../ams-pulse/sdk/beacon-swift")`;
   (b) copy the sources (list the files). Add a line that a dedicated tagged SPM repository is
   planned; remove `from: "0.1.0"` entirely.

4. **`.github/workflows/release.yml`** (repo gate: workflows must stay actionlint-clean; run
   actionlint if available on PATH, else be conservative):
   - **Version-consistency guard** (new early step in the tag-push path): assert VERSION file ==
     tag (strip v), `deploy/helm/pulse/Chart.yaml` appVersion == tag, and fail if
     `docs/product.md`, `docs/faq.md`, or `docs/known-limitations.md` still carry a DIFFERENT
     `v0.x.y` product stamp than the tag (a targeted grep of those three headers, not a blind
     repo-wide grep).
   - **GitHub Release + SDK tarball:** on tag push, after the image push: build sdk/beacon-js
     (`npm ci && npm run build && npm pack`), create the GitHub Release for the tag if it doesn't
     exist (`gh release create --verify-tag`) and upload the tarball
     (`gh release upload --clobber`). Needs `permissions: contents: write` on that job.
   - **npm publish (gated):** a step/job that runs ONLY when `secrets.NPM_TOKEN` is non-empty
     (use the env-indirection pattern: put the secret in an env var, `if: env.NPM_TOKEN != ''`
     at step level via a preceding check step — actionlint-clean), doing
     `npm publish --provenance --access public` in sdk/beacon-js. Also allow manual
     `workflow_dispatch` publishing for an already-tagged version (checkout the tag ref input).
     Missing token must NOT fail the release.

## Definition of done
No `@pulse/beacon` reference remains outside CHANGELOG/history/agents archives; sdk build+size
gates green; release.yml passes actionlint (or careful manual review if actionlint unavailable);
lockfile consistent. Return: files changed, gate results, tarball content listing, any web test
you ran.
