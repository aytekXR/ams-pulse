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

**Where the product is:** **Our side of the marketplace submission is DONE.** `v0.4.5` is the
submission target, `CLICKHOUSE_PASSWORD` is rotated (`git log -S` on the live value returns **0
commits across all refs**), and the outreach email to Ankush at Ant Media is **sent** (S123,
2026-08-01). Per the agreed process they now arrange a developer meeting and hand over the
qualification steps their dev team defined. **The next move is theirs, not ours.**

**Nothing on the engineering side blocks the marketplace.** Everything left is waiting on
something external or deliberately deferred — the list is at the bottom of this block. Do not
invent work to fill the wait; re-verify the delta instead (see the standing lessons).

**⚠⚠ THE LIVE HAZARD, and it is the single most important thing on this page.** `pulse-migrate`
**bind-mounts `contracts/` from the host working tree**, so prod's schema follows the git
checkout rather than the deployed image. Prod is pinned to v0.4.0-139 (2026-07-23) while `main`
has moved well past it, so **any `docker compose up -d` on prod applies migrations the deployed
binary does not understand.** On 2026-07-31 a password rotation did exactly that: it applied
`0011_server_events_ingest_error.sql`, took `server_events` 40 → 42 columns, and every insert
failed (`expected 42 arguments, got 40`) for five minutes. Recovered by dropping the two columns
and clearing the ledger row, deliberately back to the pre-rotation state rather than forward,
because a version roll was not authorised. `deploy/scripts/rotate-clickhouse-password.sh` now
refuses on this mismatch (`check_pending_migrations`); the manual pre-flight is in
`deploy/runbooks/upgrade-rollback.md` §1. **Rolling prod forward closes it permanently and is the
highest-value item on the debt list.**

**The prod compose command in this file was stale and would have failed** — it named
`deploy/docker-compose.prod-tls.yml`, deleted by PR #199 in the Caddy→nginx cutover. The live
stack runs **three** files (`prod + real-ams + backup`). Fixed in §6/§7. Generalisation worth
keeping: **ask the running stack (`com.docker.compose.project.config_files`), do not trust a doc
about it.**

**All CodeQL alerts are triaged and zero are open** (`docs/security/codeql-triage.md`). Three
dismissed false-positive, one dismissed won't-fix **with a mitigation shipped**, two excluded as
vendored ReDoc bundle code. The judge in that triage **overruled both analysts** on the one that
mattered: they argued "high-entropy machine secret, the rule does not apply", which holds only
for the hex path — `deriveKey` SHA-256s anything that is not exactly 64 hex chars, and
`PULSE_SECRET_KEY=mysecretpassword1` clears the 16-byte floor. **A floor on length is not a floor
on entropy.** The derivation was NOT changed: a KDF swap orphans every existing encrypted
credential and no `pulse rekey` exists. A startup warning ships instead.

**Four gate gaps were closed this session, and all four were the same shape: a check that could
not fail.**
- **npm advisories were ungated entirely** — Trivy scans the image (OS packages + Go binary) and
  never sees bundled JS, so a HIGH in `react-router` sat green for weeks. New `npm-audit` job.
- **`THIRD-PARTY-LICENSES.md --check` was never run by anything** — silently stale.
- **The shellcheck guard was a 6-path hand-list** that had already drifted; three more scripts
  were uncovered, one shipping to customers inside the Helm chart. Now discovery-based.
- **`CodeQL` was not a required context** — only `Analyze (…)`, which reports that the scan RAN,
  not what it FOUND. That is why four HIGH alerts sat open for three weeks behind green CI.
Required contexts are now **18** and the live setting matches the script (verified via the API).

**Two defects were "wrong in two places at once", which is why nothing caught them.** The website
sold historical analytics (F2) and ingest health (F4) inside the **Free** plan while both are
`CheckDataAPI`-gated — a Free user following our own pricing page got `403 LICENSE_REQUIRED`.
F2 was website-vs-docs; **F4 was wrong in the docs AND the website simultaneously**, so the
cross-check compared two copies of the same error. Operator ruled the code is right. All ten
features now agree across code, `product.md`, `overview.md`, `licensing-public.md` and the site.

**A test that mirrors production logic cannot test production.** `TierGate.test.ts` kept its own
copies of the gate predicates and passed while asserting the opposite of shipped behaviour. Rules
now live in `web/src/lib/entitlements.ts`, each mapped to its server `Check*`; planting the
historical drift now fails.

**Two independent tracks:**
- **Marketplace** — our side complete; awaiting Ant Media's reply. Queue:
  `docs/operator-expected.md` §B (item 5 lists what to capture).
- **iOS TestFlight** — blocked by **Apple Developer Program enrolment** only. §A is the path.

**Standing lessons that keep paying (condensed — session narration lives in `sessions/`):**

- **Run the guard the way CI runs it — same shell flags AND same argument passing.** Re-earned
  twice: extracting a guard under `set -euo pipefail` failed where CI's `bash -e` passes, and
  passing `check-doc-stamps.sh` its base ref as an ENV var silently skipped check B and reported
  PASS, because the script reads it positionally.
- **Fix the class, not the instance — then check you actually did.** The shellcheck guard, the
  startup-flake budget (6 sites across 4 files, and my first pass fixed ONE), and the four
  copy-pasted secret-key validators were all the same failure. Grep for siblings before claiming
  a fix.
- **Test the artifact, not the source.** The API reference passed a static CDN grep while the
  browser still fetched `cdn.redoc.ly` — the bundle injects its logo at runtime from a string in
  minified JS. Only loading it with `--network none` found it.
- **Audit the exculpations.** Five rounds running. This session the hostile audit's loudest
  finding — "BLOCKER: no cosign signatures exist" — was itself **refuted** by running the
  documented command with a v3 client. Its author fell into the trap our docs warn about.
- **A fix that does not change the artifact has not been verified.** Prove a guard BOTH ways —
  plant the defect, watch it fail.
- **Before filing a defect against code, confirm the artifact you tested was built from it.**
- **⚠ Concurrent-session hazard is real.** If HEAD moves or the tree dirties with work you did
  not do, STOP and inspect. Quarantine, never delete.

**Open engineering debt — NOTHING here blocks the marketplace:**
- **Roll prod forward** — closes the migrate/binary hazard above permanently. Highest value.
- **`pulse rekey`** — prerequisite for properly closing CodeQL #6 (Argon2id behind a versioned
  blob format, with migration). ADR-0004 defers it.
- Waiting on external things: cluster node-alerting rework (LIM-10, needs a real 2-node cluster) ·
  capacity number + AV-15 live Kafka validation (needs a PAYG AMS) · TestFlight (needs Apple).
- Deliberately deferred: release candidate push via buildx `push-by-digest=true` (round 6 H-09) —
  changes the publish mechanism and cannot be exercised by the dry-run path, only a real tag.
- Smaller: the iOS app's `KeychainService` duplicates PulseKit's `TokenStore` · surface
  `stream_ingest_error` (LIM-27) · thread the owning node through per-app polling (LIM-28) ·
  probe whether `ams-webrtc-stats` can restore per-stream FPS (LIM-04 rests on a LIM-01-style
  inference) · self-host the IBM Plex OFL woff2 files for `website/` and the iOS app (the **web
  UI already self-hosts them**) · optional: a `crypto.getRandomValues` middle tier in the beacon
  SDK (better than the `Math.random` fallback in HTTP contexts; would not close any alert).

**Do first, every session:**
1. **Gate reads** — prod health (component-scoped `/healthz`, plus a ClickHouse count sampled
   TWICE to prove it is *moving*, not merely non-zero), git/PR drift, concurrent-session check.
2. **Has Ant Media replied?** If so, capture the answers per `docs/operator-expected.md` item 5.
3. **Check whether the Apple account exists yet** (`gh secret list` for `APP_STORE_CONNECT_*`).
4. **If a TestFlight public link arrives**, do the `/beta/` swap and redeploy.

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
  0 ERROR lines in the preceding 30 min, row count verified *increasing* across two samples
  (re-read S123 close, 2026-08-01: **1,417,734** events, newest 4 s old).
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
- **`main` is protected** (strict, 1 review, `enforce_admins=false` so owner pushes work; **18**
  required contexts — D-185 added `shellcheck` and `doc-stamps`, D-186 added `ios-kit`; D-191 added `npm-audit` and `CodeQL`. A guard job
  that is not in that list cannot block a merge. `ios-app` is deliberately excluded: it depends on
  a third-party runner image and Homebrew, the same argument that excludes `compose-boot`. What
  that leaves uncovered is that a SwiftUI regression can merge on a red `ios-app` if ignored).
  The live setting and the script now MATCH — verified S122 via the API: 16 contexts, exactly the
  script's 14 plus the two CodeQL `Analyze (…)` jobs. The earlier warning that the script still
  needed re-running was stale.
  Work on a branch → PR → merge on green.
- **Known limitations are disclosed, not hidden:** `docs/known-limitations.md` carries 29
  entries (LIM-29, added S123: historical queries are silently capped to the tier's retention
  window — a Free request for 30 days returns 7, HTTP 200, no warning field). **LIM-01 was closed in D-179** (standalone CPU/mem/disk now work without Kafka) and
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
