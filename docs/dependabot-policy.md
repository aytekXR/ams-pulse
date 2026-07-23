# Dependabot steady-state policy

## 1. Overview

Dependabot opens PRs weekly across six ecosystems (see `.github/dependabot.yml`).
This document governs review cadence, verification gates, and merge mechanics on top
of the live config. It does not replace the YAML; it operationalises it.

Config facts (`.github/dependabot.yml`):
- **Ecosystems:** gomod (`/server`), npm (`/web`), npm (`/sdk/beacon-js`), docker
  (`/deploy/docker`), docker-compose (`/deploy`), github-actions (`/`)
- **Cadence:** weekly for all six
- **Concurrent PR cap:** `open-pull-requests-limit: 5` per ecosystem
- **Grouping:** minor and patch updates are grouped per ecosystem (`minor-and-patch`);
  majors surface as individual PRs and cannot be silently bundled
- **golang semver ignore:** both docker and docker-compose ecosystems ignore
  `version-update:semver-major` and `version-update:semver-minor` for `golang`
  (D-032 pin — see §2d below and decisions.md D-066)

---

## 2. Bump classes and cadence

### 2a. Digest and patch bumps

**Target:** approve and squash-merge within **1 week** of green CI.

Docker and docker-compose digest bumps require an additional staging smoke before any
prod container refresh:

1. **Staging smoke (pristine-copy stack):** spin up a full compose stack from a
   clean copy of `deploy/` with a unique `-p <name>` project (never the prod `.env`).
   The stack has no TLS layer of its own (the edge is host nginx, out of scope for a
   container-digest smoke). Verify:
   - `healthz` responds OK on the pulse container's HTTP port
   - The webhook listener returns fail-closed (403/no-HMAC) on an unsigned probe
2. **Prod refresh:** pull the new digest and recreate **only the updated services by
   naming them explicitly** (`docker compose … up -d --no-deps <svc> …`; `--no-deps`
   keeps compose from also restarting their dependencies) — never the `pulse` app
   container unless `pulse.Dockerfile` itself changed (D-067 batch-2 pattern:
   clickhouse/backup recreated — plus the caddy service that existed at v0.2.0,
   since removed; pulse untouched).
3. **Teardown:** `docker compose -p <name> down -v` to destroy the staging stack.

### 2b. Minor bumps (dependabot-grouped)

**Target:** review within **2 weeks** of PR open.

Verification requirements by ecosystem:

| Ecosystem | Gate |
|-----------|------|
| gomod | Full docker `-race` run with REPO-ROOT mount (golang:1.25); floor 70.2%; 0 FAIL / 0 unexpected SKIP (D-067 batch-3: #14 go minors verified this way) |
| npm/web | Full vitest coverage at current gates: **59/54/45** (lines/branches/functions; vitest-4 rolldown baselines — do NOT compare to pre-vitest-4 numbers; D-067) |
| npm/sdk | Full vitest coverage at current gates: **63/43/67**; beacon size ≤15 KB |
| docker / docker-compose | Staging smoke (§2a); prod refresh if smoke passes |
| github-actions | Check runner/toolchain-pin interactions (e.g. `setup-go` version changes may affect toolchain resolution if `go-version` is not pinned explicitly) |

### 2c. Major bumps

**Target:** explicit session work order. Never merge a major ad-hoc.

Protocol:

1. **Pre-verify all open majors together before merging any.** Running them
   concurrently in scratch checkouts exposes co-upgrade peer conflicts that sequential
   single-PR merges miss (D-067: this assumption was wrong and cost a batch-3 rework).
2. **Cluster co-dependent majors into ONE carrier PR.** Land the entire co-upgrade
   cluster as a single squash — never split peer-pinned packages across separate PRs
   (see §6 for known clusters).
3. **TDD:** write failing tests before the upgrade commit; confirm red; implement;
   confirm green.
4. **Coverage re-baseline:** re-baseline thresholds ONLY when the instrumentation
   engine changes (e.g. upgrading vitest also upgrades rolldown/v8). When re-baselining:
   - Run the full coverage suite; record the new readings
   - Update enforcement thresholds in the config
   - Prove enforcement with a 99%-threshold hard-fail check (intentionally exceed
     thresholds; confirm CI fails)
   - Never compare numbers across instrumentation engines (D-067: web 76/72/45 →
     59/54/45 and sdk 62/73/70 → 63/43/67 are not regressions; they are a measurement
     change)
5. **Actions majors additionally require a release-pipeline dry-run proof before
   merge** (D-067 batch-1: `gh workflow run release.yml -f version=0.0.0-dry` → run
   29028802644 GREEN — no push/sign, CI-gate + build + Trivy only).

### 2d. Golang version bumps — BLOCKED

golang semver bumps (minor and major) are **blocked by the D-032 pin** (golang:1.25).
Dependabot already ignores them in both docker and docker-compose ecosystems
(`.github/dependabot.yml` lines 57-61 and 76-79). The ignore rules are D-032-gated
(decisions.md D-066); do not remove or "clean up" those ignore entries — they exist
by deliberate decision, not oversight.

golang DIGEST refreshes are welcome and follow path §2a.

Lifting the golang version pin is its own dedicated session work order; it is never a
side effect of a dependabot merge.

---

## 3. Merge mechanics

Two PR types require different update commands; using the wrong one destroys work.

### Pristine dependabot PRs (no session commits on the branch)

Use `@dependabot rebase`. This regenerates lockfiles from scratch rather than doing a
textual merge.

- **Never use the API `update-branch` endpoint** on pristine lockfile PRs: it textually
  merges `package-lock.json`, producing EUSAGE desyncs ("Missing: esbuild@0.28.1 from
  lock file") — proven on PR #15 (D-067).

### PRs touching `.github/workflows/*`

Always use `@dependabot rebase`. The gh CLI token lacks the `workflow` scope, so
`update-branch` returns 403 on workflow-file PRs (D-067 process lesson 1). Optional
operator fix: `gh auth refresh -s workflow`.

### Carrier PRs (session commits pushed on top of the dependabot branch)

Use the **API `update-branch` only** — never `@dependabot rebase`. Dependabot rebase
force-pushes the branch, destroying any commits the session added (D-067 process
lesson 2). The API update-branch is safe because it only merges base into the existing
branch tip.

### Merging (no auto-merge configured)

The repo has NO auto-merge. Merge loop:

```
gh pr checks <PR> --required   # poll until all required contexts pass
gh pr merge <PR> --squash       # squash-merge
```

Merge PRs serially — one at a time. Parallel merges race on the branch-up-to-date
requirement and produce out-of-order squash commits that are hard to bisect.

---

## 4. Verification gates summary

| Class | Time limit | CI green | Extra gate | Merge method |
|-------|-----------|----------|------------|--------------|
| Digest / patch | 1 week | Required | Docker/compose: staging smoke + selective prod refresh | squash |
| Minor (grouped) | 2 weeks | Required | See §2b per-ecosystem table | squash |
| Major | Session WO | Required | Pre-verify all together; cluster carrier PR; TDD; coverage re-baseline if engine changes; actions: release dry-run | squash on carrier |
| golang semver | BLOCKED | N/A | D-032 pin; lift in dedicated WO | N/A |

---

## 5. Batch absorption (queue > 5 PRs)

When the queue accumulates (e.g. after a release sprint), absorb in this order
(D-066/D-067 precedent):

1. **Actions first** — minimal blast radius; validates the release pipeline early.
   Run a release dry-run (`gh workflow run release.yml -f version=0.0.0-dry`) after
   merging all actions PRs; do not proceed if it fails.
2. **Digests next** — run the staging smoke (§2a) before merging. Execute the prod
   refresh immediately after merge while the digest is still the current tested image.
3. **Majors last** — pre-verify all open majors together before merging any; identify
   co-upgrade clusters (§6); build carrier PRs; TDD; land cluster by cluster.

Serial merge within each batch. Update §6 after every major absorption.

---

## 6. Known co-upgrade clusters

Sessions must update this list after every major absorption cycle.

### web cluster (proven D-067, session 9)

The following packages **must move together** in one carrier PR:

- `vite` (→ 8.1.3)
- `vitest` (→ 4.1.10)
- `@vitest/coverage-v8` (→ 4.1.10)
- `@vitejs/plugin-react` (→ 6.0.3)

Peer constraints: `plugin-react` ≥5 requires `vite ^8`; `coverage-v8` 4 requires
`vitest` 4 (no `BaseCoverageProvider` export in vitest 3); vitest 4 crashes with
`coverage-v8` 3 (`fetchCache` TypeError); `vite` 8 drops `test` from `UserConfig`
types (import must move to `vitest/config`). Splitting this cluster across PRs will
produce CI failures.

Additional `vite.config.ts` changes required with this cluster:
- Import from `vitest/config` (not `vite`)
- `resolve.dedupe: ['react', 'react-dom']` (plugin-react ≥5 dropped auto-dedupe)
- `coverage.exclude` must include `**/*.md` (rolldown parses all files matched by
  `include`)
- Any test with tight timing should use `userEvent.setup({delay: null})` (rolldown is
  slower than esbuild; 5 s timeout boundaries become flaky)

**Coverage re-baseline:** web gates moved 76/72/45 → 59/54/45 (achieved 62.13/57.6/51
at D-067 merge time). This is an instrumentation delta, not a test regression.

### sdk cluster (proven D-067, session 9)

- `vitest` (→ 4.x)
- `@vitest/coverage-v8` (→ 4.1.10)

Exact peer pin → ERESOLVE if bumped separately. Also align `size-limit` CLI to the
preset version used (`^12.1.0`) to avoid dual-version install conflicts.

**Coverage re-baseline:** sdk gates moved 62/73/70 → 63/43/67 (achieved
66.06/45.79/70.42; lines ratcheted UP 62→63; branch drop is a v8 branch-granularity
change, not a regression). Go floor 70.2 is unchanged and does not require re-baseline
on a vitest upgrade.
