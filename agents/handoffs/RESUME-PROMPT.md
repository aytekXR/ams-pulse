# Pulse — Resume / handoff prompt (SINGLE source of truth)

> **This is the one handoff doc.** Update THIS file + `decisions.md` every session; never
> create a second handoff file, and never stack past sessions here (operator directive).
>
> Pulse = self-hosted analytics/QoE/alerting for Ant Media Server. Repo:
> `/home/aytek/repo/ams-pulse` on the operator's VPS.
>
> - Decision log (binding): `agents/handoffs/decisions.md`
> - Per-session detail: `agents/handoffs/sessions/SESSION-NNN.md`
> - Plan of record: `agents/handoffs/ROADMAP-V2.md`
> - Operator queue: `docs/operator-expected.md`
> - AMS integration facts: `docs/AMS-INTEGRATION.md`
> - Go-live + rollback: `deploy/runbooks/real-ams-go-live.md`
> - Operator creds/keys (gitignored, NEVER commit): `oguz-testing.md`

---

## ▶ START HERE — current state

> **This file carries only current, forward-looking state.** No session history, no superseded
> blocks, nothing struck through. How we got here lives in
> `agents/handoffs/sessions/SESSION-NNN.md` and `decisions.md` (operator directive).
> **Replace this block each session — never append to it.**

**Where the product is:** **v0.4.5 is the marketplace submission target, and the last technical
blocker is gone.** `CLICKHOUSE_PASSWORD` was rotated on 2026-07-31 (S123) and verified the only
way that counts: `git log -S` on the new value's 32-char prefix returns **0 commits across all
refs**. The two commits carrying the old prefix now hold a dead password. **Submission is now an
operator action, not an engineering one.**

**The rotation's real yield was the landmine it tripped, not the rotation.** Recreating the prod
stack applied `0011_server_events_ingest_error.sql` *from the working tree* to the pinned
v0.4.0-139 binary, took `server_events` 40 → 42 columns, and dropped ingest for five minutes
(`expected 42 arguments, got 40`). Cause: **`pulse-migrate` bind-mounts `contracts/` from the host
repo, so prod's schema follows the git checkout rather than the deployed image.** Any `up -d` on
prod would have done this. Recovered to the *pre-rotation* state deliberately (dropped the two
columns, cleared the ledger row) rather than rolling forward, because a version roll was not
authorised. `rotate-clickhouse-password.sh` now refuses on a schema/binary mismatch, and the
pre-flight check is in `deploy/runbooks/upgrade-rollback.md` §1. **Before ANY prod `up -d`, check
pending migrations against the deployed commit.**

**The documented prod command was stale and would have failed.** It named
`deploy/docker-compose.prod-tls.yml`, deleted by PR #199 in the Caddy→nginx cutover. The live
stack runs **three** files (`prod + real-ams + backup`). Fixed in §6/§7. The lesson generalises:
**read the running stack's own `com.docker.compose.project.config_files` label rather than trusting
a doc.**

**A nine-lens hostile marketplace audit (32 agents) plus my own sweep found 17 verified defects.**
Two were customer-facing tier lies: the website sold **historical analytics (F2)** and **ingest
health (F4)** inside the Free plan while both are `CheckDataAPI`-gated (Pro+) — a Free user
following our own pricing page got `403 LICENSE_REQUIRED` on both. The operator ruled **code is
right, F4 is Pro+**; all ten features now agree across code, `product.md`, `overview.md`,
`licensing-public.md` and the site. **The audit's own all-clears were then attacked, and its
loudest "BLOCKER — no cosign signatures exist" was REFUTED by running the documented command with
a v3 client (digest `542fead1…`, exit 0). The attacker had fallen into the exact trap our docs warn
about.** Audit the audit.

**Three gate gaps were closing over real defects:**
- **npm advisories were ungated entirely.** Trivy scans the image (OS packages + Go binary), so the
  web UI's bundled npm deps were analysed by nothing. `react-router` 7.18.1 carried a HIGH
  (GHSA-qwww-vcr4-c8h2) for weeks with every gate green. Migrated to 8.3.0 (the only patched
  version) — `react-router-dom` is folded into `react-router` in v8, so it was a package swap plus
  four imports. New `npm-audit` job, now a **required context (17)**.
- **`THIRD-PARTY-LICENSES.md` had a `--check` mode nothing ever ran.** It was silently stale. Now a
  hard gate.
- **The shellcheck guard was a hand-list of 6 paths** that had already drifted once. Three more were
  uncovered, one of which **ships to customers inside the Helm chart**. Now discovers every tracked
  `*.sh` under `deploy/ scripts/ .github/ website/` (9 → 10 files) with exclusions written inside
  the guard. ⚠ My first attempt used `scripts/**/*.sh`, which git pathspec requires an intervening
  directory for — it silently covered LESS than the list it replaced. Caught only by testing the
  guard; it now asserts a discovery floor and runs a negative test.

**A test that mirrors production logic instead of importing it is not a test.**
`TierGate.test.ts` defined its own copies of the gate predicates, so when anomalies moved to
Business+ it kept passing while asserting `isAnomaliesGated("business") === true`. Rules now live
in `web/src/lib/entitlements.ts` (each mapped to its server `Check*`), the test imports them, and
planting the historical drift now fails. `ProbesPage` also gated on `tier !== "free"` — negative
membership, so an unknown tier was *granted* access; the server abandoned that shape in D-133.

**The contract lied to integrators.** `pulse-api.yaml` still described the pre-D-166/pre-S122
matrix: Pro 1–2 nodes (really 10), Business ≤5 (really 50), Business anomalies "no" (really yes),
`/anomalies` 403 as "Enterprise required". It generates `docs/api/index.html`, the reference a
reviewer opens — which also loaded ReDoc from a CDN **and Google Fonts**, despite being advertised
as self-contained and despite the repo's no-CDN rule. Now built by `scripts/build-api-docs.sh`,
inlined against redocly's own SRI hash. ⚠ A static tag-grep passed while the browser still fetched
`cdn.redoc.ly` — the bundle injects its logo at runtime from a string in minified JS. **Only
loading the page with `--network none` found it.**

**Undisclosed behaviour is now LIM-29:** historical queries are **silently** capped to the tier's
retention window — a Free request for 30 days returns 7, HTTP 200, no warning field, indistinguishable
from a complete answer.

**Two independent tracks remain:**
- **Marketplace** — **unblocked.** Submit `v0.4.5`. Queue: `docs/operator-expected.md` §B.
- **iOS TestFlight** — blocked by **Apple Developer Program enrolment** only. §A is the eight-step path.

**Standing lessons that keep paying (condensed — session narration lives in `sessions/`):**

- **Test the artifact, not the documentation, and run it the way CI will.** Re-earned twice this
  session: the API reference passed a static CDN grep and still hit the network; and extracting a CI
  guard with `set -euo pipefail` made it fail where CI's `bash -e` passes. **Run the guard the way
  CI runs it.**
- **Audit the exculpations, not just the findings.** Five rounds running. This time the audit's own
  BLOCKER was the false one.
- **A fix that does not change the artifact has not been verified.** Prove a guard in BOTH
  directions — plant the defect, watch it fail.
- **Before filing a defect against code, confirm the artifact you tested was built from it.**
  `pulse version` + `git merge-base --is-ancestor` is the whole procedure.
- **⚠ Concurrent-session hazard is real.** If HEAD moves or the tree dirties with work you did not
  do, STOP and inspect. Quarantine, never delete.
- **A subagent lane's self-report is not a gate.** Read every diff yourself; ORCH commits centrally.

**Open engineering debt (loop-owned, non-blocking):** roll prod forward (would permanently close
the migration/binary gap) · the app's `KeychainService` duplicates PulseKit's `TokenStore` ·
cluster node-alerting rework (LIM-10, waits on a real cluster) · surface `stream_ingest_error`
(LIM-27) · thread the owning node through per-app polling (LIM-28) · the ClickHouse integration
harness's 45 s startup budget is too tight for a slow runner (flaky in a required context) ·
release: switch the candidate push to buildx `push-by-digest=true` (round 6 H-09) · probe whether
`ams-webrtc-stats` can restore per-stream FPS (LIM-04 rests on a LIM-01-style inference) ·
self-host the IBM Plex OFL woff2 files for `website/` and the iOS app (the **web UI already
self-hosts them**).

**Do first, every session:**
1. **Gate reads** — prod health (component-scoped `/healthz`, plus a ClickHouse count AND a second
   sample to prove it is *moving*), git/PR drift, concurrent-session check.
2. **Check whether the Apple account exists yet** (`gh secret list` for the three
   `APP_STORE_CONNECT_*` secrets).
3. **If a TestFlight public link arrives**, do the `/beta/` swap and redeploy.

**Operator queue:** `docs/operator-expected.md`. **How we got here** (read only if you need it):
`decisions.md` · `agents/handoffs/sessions/` · `docs/assessment/`.

---
## 1. CURRENT STATE (verified facts — refresh each session, never let this go stale)

- **Shipped product, pre-marketplace.** All 10 PRD features implemented and live-validated
  against a real AMS 3.0.3 Enterprise (46/50 scenarios). **Latest release: v0.4.5** (2026-07-30,
  the marketplace submission target; it carries review rounds 7–11 plus the S122 corrections,
  which v0.4.4 did not). AMS 3.0.3 is still the latest AMS release.
- **Production** runs behind host nginx on this VPS at `https://pulse.beyondkaira.com`, against
  the operator's own `antmedia` container (AMS Enterprise 3.0.3, `--network host`). It is on the
  stamped **v0.4.0-139** build — rolling prod forward is deliberate and operator-gated, never
  automatic. Health at last check (S123, 2026-07-31, AFTER the rotation and its 5-minute ingest
  outage): all three `/healthz` components `ok`, **1,400,267** server events, newest 5 s old,
  0 ERROR lines in the preceding 90 s, row count verified *increasing* across two samples.
  ⚠ Prod's schema is now one migration BEHIND the tree on purpose (0011 reverted) so the pinned
  binary keeps working — see §7.
- **The iOS app exists and is CI-verified** (D-186). `ios/PulseKit` — Foundation-only, **296 tests
  green on Linux** (re-verified S122 in CI's `swift:6.1` container from a clean `git archive` copy), which is the point of the split: no Apple toolchain exists on this VPS, so
  anything living in a SwiftUI view is logic no gate here can check. `ios/PulseApp` — SwiftUI plus
  an AVPlayer view instrumented with our own `PulseBeacon` SDK, **50 tests green on a real iOS 26.2
  simulator** via `macos-15`. The archive/sign/upload job is written and has **never executed** —
  it cannot until an Apple Developer account exists. Measured runner facts (re-measure after an
  image bump): `docs/mobile/ci-runner-facts.md`.
- **The public website exists** (D-186) at `website/` — landing, `/beta/`, `/privacy/`, `/support/`,
  `/terms/`, built from the brandkit against `tokens.json`, zero external requests (enforced by a
  check, not by intent). GitHub Pages is **enabled** (`build_type: workflow`) and publishes to
  `https://aytekxr.github.io/ams-pulse/` on merge to `main`.
- **`main` is protected** (strict, 1 review, `enforce_admins=false` so owner pushes work; **16**
  required contexts — D-185 added `shellcheck` and `doc-stamps`, D-186 added `ios-kit`; D-191 added `npm-audit`. A guard job
  that is not in that list cannot block a merge. `ios-app` is deliberately excluded: it depends on
  a third-party runner image and Homebrew, the same argument that excludes `compose-boot`. What
  that leaves uncovered is that a SwiftUI regression can merge on a red `ios-app` if ignored).
  The live setting and the script now MATCH — verified S122 via the API: 16 contexts, exactly the
  script's 14 plus the two CodeQL `Analyze (…)` jobs. The earlier warning that the script still
  needed re-running was stale.
  Work on a branch → PR → merge on green.
- **Known limitations are disclosed, not hidden:** `docs/known-limitations.md` carries 28
  entries. **LIM-01 was closed in D-179** (standalone CPU/mem/disk now work without Kafka) and
  rewritten down to a memory-threshold calibration note rather than deleted. LIM-10 (cluster) is
  the significant remaining one — AMS 3.x exposes no node role or version, so
  all nodes display as `origin`, edge/origin viewer dedup is inert, and node alerting during an
  AMS API outage is not fully reliable. All deliberate and written down.
- **Operator-gated items** (never do these autonomously): secret rotation · marketplace listing
  submission · billing · the Ankush reply · the PAYG load lane · prod rolls. Queue:
  `docs/operator-expected.md`.

---

## 2. TDD ENFORCEMENT (BINDING — bias toward test coverage over implementation speed)

**Every change follows red→green→refactor: write the failing test FIRST, watch it fail, implement, watch it pass.**
For each unit of work produce tests at ALL applicable levels (do not stop at "unit"):

| Level | What it asserts | Where |
|---|---|---|
| **Unit** | pure logic, table-driven, both branches | `*_test.go`, `*.test.ts(x)` |
| **Integration** | real ClickHouse/sqlite via the Go harness (`-tags integration`, `/tmp/clickhouse`) | `*_integration_test.go` |
| **Contract** | HTTP response bodies validated against `contracts/openapi/pulse-api.yaml` (kin-openapi) | `internal/api/*_contract_test.go` |
| **Functional** | a feature's user-visible behavior end-to-end through the API (publish→visible, alert→history) | `e2e.yml` steps + api tests |
| **E2E (browser)** | dashboard render, auth redirect, CSP header, large-table virtualization | `web/e2e/*.spec.ts` (Playwright — NEW) |
| **Regression** | a fixed bug stays fixed (every D-0NN fix gets a pinning test) | co-located with the fix |
| **Edge-case** | empty/zero/max/null/unicode/pagination boundaries | per package |
| **Failure-path** | timeouts, 4xx/5xx, drop-on-full, retry exhaustion, decode errors | per package |

**Coverage gate (must not regress; the three 0.0% packages must reach ≥60%):**
```
sg docker -c 'docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 golang:1.25 sh -c "go test -race -coverprofile=cover.out -covermode=atomic ./... && go tool cover -func=cover.out | grep -E \"^total|0.0%\""'
```
**Prioritize critical business logic first:** (1) license/tier enforcement, (2) alert firing + delivery, (3) ingest
health scoring, (4) AMS wire decode/normalize, (5) the query layer. Report coverage in every handoff.

---

## 3. VERIFICATION WORKFLOW (BINDING — every implementation runs ALL of these before "done")

1. **Build:** `go build ./...` (CGO_ENABLED=0) + `cd web && npm run build`.
2. **Lint:** `cd web && npm run lint`; Go `gofmt -l` (must be empty) + `go vet ./...`.
3. **Type-check:** `cd web && npm run typecheck` (or `tsc --noEmit`).
4. **Test (race):** `go test ./... -race -count=1` **repo-root mount** (D-028: server-only mount silently skips ~90 api
   tests → false green). Confirm **0 FAIL, 0 unexpected SKIP**.
5. **Coverage:** the gate command in §2; attach numbers to the handoff.
6. **Contract drift:** `cd web && npm run gen:api` then `git diff --exit-code` (generated types match spec);
   `redocly lint` + `ajv` on event schemas.
7. **Staging verify:** bring the change up on an **isolated compose project** (NOT pulse-prod) and curl the affected
   endpoints. Never verify on prod first.
8. **Deploy smoke (after a prod change):** `/healthz` ok via `--resolve`; affected endpoint returns expected real
   data; `pulse logs` shows no 401/403/decode/login errors; for migrate, DSN masked (`:xxxxx@`).
9. **Independent/adversarial re-check:** default to "refuted" until reproduced on a fresh build (D-013/017/019). A
   verify harness that silently skips == no verify (D-028).

---

## 4. BINDING FLOWS — every workflow MUST end with these (user directive)

- **Verify** — independent/adversarial re-check of *every* claim against a running stack or fresh build; default to
  "refuted" until reproduced; **repo-root mount** or api tests silently skip (D-028). QA alone is not authoritative
  (D-013/017/019).
- **Commit** — by **EXPLICIT path**, per scope; never `git add -A/-u/.` (parallel agents share the tree — D-008/D-011).
  In a workflow, agents AUTHOR only; ORCH commits centrally (avoids `.git/index.lock` races). Message
  `<scope> D-0NN: <summary>` + evidence. Push when the user directs.
- **Handoff** — update **THIS `RESUME-PROMPT.md`** + `decisions.md` (new D-0NN) every session, then commit + push.

---

## 5. OPERATING PROTOCOL (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase = one Workflow: ORCH writes the plan + pre-approved CRs to
  `decisions.md`, fans out to disjoint-scope agents, then **independently gates**. Background work is harness-tracked —
  you're re-invoked on completion; don't poll-spin.
- **CodeGraph (operator-installed 2026-07-09, D-061).** Local index `.codegraph/` + CLI `~/.local/bin/codegraph`.
  Scouts/authors query the graph BEFORE grep/file sweeps: `codegraph explore "<question>"`,
  `codegraph node <sym>`, `codegraph callers <sym>` (blast radius). Put this in every agent work order
  (subagents use the CLI via Bash). **Closing protocol: `codegraph sync` after the last commit** (+
  `codegraph status` to confirm; stale lock → `codegraph unlock`).
- **Local compose stacks NEVER run from the real repo** — compose auto-loads `deploy/.env` (prod secrets) from
  the `-f` dir. Use a pristine working-tree copy:
  `git ls-files -co --exclude-standard -z | tar --null -T - -cf - | tar -C <scratch> -xf -` + unique `-p` name (D-061).
- **Anti-stall (D-016):** NEVER run `pulse serve`/`clickhouse server` in the foreground inside an agent. Use
  `docker compose up -d` (detached) + health polling; CH unit work via the integration harness. `timeout` on builds,
  `-timeout` on `go test`, vitest `run` not watch, `curl -m`. Long local repros: Bash `run_in_background: true`.
- **Single-writer scope map** in `agents/manifest.yaml`. **Contracts frozen (D-004)** — changes only via an
  ORCH-approved CR applied by INT-01 (OpenAPI + event schemas + migrations).
- **⚠️ Workflow/fork agents have Write+commit access** — a reviewer fork once auto-committed during a concurrent ORCH
  edit (D-030 process note). Scope reviewer agents read-only when ORCH is editing the same files.
- **⚠️ Subagents NEVER revert shared-tree files (D-063):** no `git restore` / `git checkout --` /
  `git stash` inside workflow agents — concurrent agents' UNCOMMITTED work shares the tree, and a
  verifier reading `git status` cannot tell foreign work from scope violations. Violations are
  REPORTED; ORCH decides and reverts. ORCH also commits early per scope to shrink the window.
  (A wo6 fixer once destroyed two files of verified work; recovered only via transcript-replay.)

---

## 6. HARD RULES (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in ClickHouse, config in the
  meta store, never crossed; web UI consumes ONLY generated public-API types; beacon ingest is hostile input.
- `CGO_ENABLED=0` for the shipping build (pure-Go sqlite); single binary `pulse serve|migrate|diag`; React 19 + RR7 +
  Vite + TS strict; recharts; no external fonts/CDNs. `go test -race` needs `CGO_ENABLED=1` + gcc.
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/`. Base `docker-compose.yml` stays clean (`expose:`, no host ports); exposure in
  overrides. **Prod stack = `prod + real-ams + backup` (THREE files).** The old "5 overlays
  (base + hardened + prod-tls + real-ams + backup)" wording was stale: PR #199 (the Caddy →
  host-nginx cutover, 2026-07-23) **deleted `docker-compose.prod-tls.yml`** and consolidated the
  stack into `docker-compose.prod.yml`. The documented command named a file that no longer exists.
  Canonical command: `deploy/runbooks/upgrade-rollback.md` §1. Verified against the live stack's
  own `com.docker.compose.project.config_files` label on 2026-07-31.

---

## 7. ENVIRONMENT (VPS)

- **Ubuntu 24.04 VPS `161.97.172.146`**, Docker 29 + Compose v5. **`go` is NOT on PATH** — run Go only in Docker
  (`golang:1.25`). node 20 + npm 10 on PATH. **`gh` IS installed + authed as owner `aytekXR`**.
- **⚠️ For `go test` mount the REPO ROOT** (`-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e
  GOFLAGS=-buildvcs=false`): a `server/`-only mount makes `metaDDLPath` escape the mount → `t.Skip` →
  skip-counts-as-pass false green (~90 api tests). Confirm **0 SKIP** for api.
- **Docker:** user `aytek` is in `docker` group but stale in non-login shells → prefix `sg docker -c "…"`. `sudo` needs
  a password → ask the user via the `! <cmd>` prompt for privileged ops. For host-root debugging without sudo, run a
  privileged container in the host netns (e.g. `docker run --rm --net=host --cap-add=NET_RAW corfr/tcpdump …`, D-036).
- **Real-AMS prod ops** (run from repo root): `DC="-p pulse-prod -f deploy/docker-compose.prod.yml
  -f deploy/docker-compose.real-ams.yml -f deploy/docker-compose.backup.yml --env-file deploy/.env"`
  — **three files, not five.** The backup overlay is part of the standing combo; omitting it on
  `up -d` would REMOVE the backup sidecar. Status: `sg docker -c "docker compose $DC ps"`. Admin
  token: in `oguz-testing.md` (gitignored) — persisted in the `pulse-prod_pulse-data` volume;
  **never `down -v` that volume.** TLS check: always
  `--resolve beyondkaira.com:443:161.97.172.146` (VPS DNS is stale). Rollback: runbook §5.
  *(The previous five-file command here named `docker-compose.prod-tls.yml`, deleted by PR #199
  on 2026-07-23. Never trust this block over the running stack's own
  `com.docker.compose.project.config_files` label.)*
- **⚠⚠ `docker compose up -d` ON PROD APPLIES WORKING-TREE MIGRATIONS.** `pulse-migrate`
  **bind-mounts `contracts/` from the host repo**, so recreating the stack runs whatever
  migrations are in the current checkout — against whatever binary is deployed. Prod is pinned to
  an old build (v0.4.0-139, 2026-07-23) while `main` has moved on, so this is a live landmine, not
  a theoretical one: on 2026-07-31 a password rotation recreated the stack, applied
  `0011_server_events_ingest_error.sql` (v0.4.2), took `server_events` from 40 to 42 columns, and
  every insert failed for five minutes with *"expected 42 arguments, got 40"* until the two
  columns were dropped and the ledger row cleared. **Before any prod `up -d`: check whether
  `contracts/db/clickhouse/*.sql` has files not in `pulse.schema_migrations`, and whether the
  deployed commit contains them.** `deploy/scripts/rotate-clickhouse-password.sh` now does this
  automatically and refuses; borrow its `check_pending_migrations` for any other prod recreate.
- `deploy/.env`, `deploy/.env.*`, `*.db*`, `oguz-testing.md`, `web/pulse_secret.key` are gitignored — never commit.
- ⚠️ **Concurrent-session hazard (learned D-062):** the operator may run a second Claude session in
  this repo. If HEAD moves or the tree dirties mid-session with work you didn't do, STOP and inspect
  before committing/pushing — a foreign unpushed commit once carried a hardcoded live secret (O11).
