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

**v0.5.0 is released and in production (S125, 2026-10-07 — D-193, D-194).** Pulse is free: every
feature on every install, no license key, commercial use included (PolyForm Shield 1.0.0; the
beacon SDKs are MIT); developer credit Aytekin Erdogan; no purchase link. Release: tag on
`8523b47`, `ghcr.io/aytekxr/ams-pulse:0.5.0` (multi-arch, cosign-verified, Trivy clean), chart
0.4.0. **Prod runs v0.5.0** (stamped `8523b47`), healthy and ingesting from **AMS 3.1.0
Enterprise** on this VPS (host networking; trial key valid to **2026-10-16**); loopback publishes
`127.0.0.1:8090-8092` restored; `PULSE_METRICS_TOKEN` set. Rollback image:
`pulse-prod-pulse:pre-d194` (= v0.4.5-9).

**⚠ Waiting on the operator** (the package's `operator-expected.md` §1, and `docs/operator-expected.md`):
1. **Enable the Pulse vhost** — `sudo ln -s /etc/nginx/sites-available/pulse.beyondkaira.com.conf
   /etc/nginx/sites-enabled/ && sudo nginx -t && sudo systemctl reload nginx`. Until then the domain
   serves the apex landing and the app is reachable on `127.0.0.1:8090` only. (`/metrics` needs the
   token, so exposing it is safe.)
2. **Renew the AMS license before 2026-10-16** (applied via the run command's `-l`, not the
   `LICENSE_KEY` env var). When it lapses, prod goes blind again.
3. Send the Ant Media ZIP (`bash qa/marketplace/build-submission-zip.sh ant-media`); confirm the
   name spelling (Aytekin Erdogan vs Erdoğan); delete GHCR `candidate-5c561bc4` (needs
   `delete:packages` or the web UI); read `/privacy/` and `/terms/`.

**★ Top engineering item: §2.49 analytics accuracy (D1–D4, LIM-30)** — operator ruling "list now,
fix next": audience analytics returns 0, the rollups count every heartbeat upsert, QoE ratios are
understated. One data-model change + backfill + producer-shaped fixtures. Do NOT fix D1 alone.

**The standing hazard, now dormant.** `pulse-migrate` bind-mounts `contracts/` from the working
tree, so a prod `up -d` applies the checkout's migrations to the deployed binary (2026-07-31:
five-minute ingest outage). Prod and `main` are both v0.5.0, so nothing is pending — the hazard
returns the moment `main` gains a migration prod lacks. Roll prod forward with it; pre-flight:
`deploy/runbooks/upgrade-rollback.md` §1.

**Tracks:** Marketplace — package delivered; waiting on Ant Media (requirements, review timeline,
load-test format, their terms for a free listing). iOS TestFlight — Apple Developer Program
enrolment only.

**Standing lessons that keep paying (condensed — session narration lives in `decisions.md`):**
- **Run the release gate before the release** (D-194): Trivy on the candidate image found fixable
  HIGH CVEs — the builder pin labelled 1.25 was go1.26.5. **Pin every tool CI fetches at run
  time**: an unpinned `npx @redocly/cli` turned `main` red on release day.
- **A "harness defect" on a scenario that used to pass is a claim about history** (D-194): the
  evidence dates turned TC-H-06 into a real UI defect (the alert-rule form).
- **A monitor that cannot fail is not a monitor:** this host's `gh` has no `pr checks --json`; run
  any poll command once by hand before arming a watch on it.
- **Run marketing captures through the real pipeline; read every panel** (D-193). Fixtures must use
  the producer's shape.
- **Run the guard the way CI runs it. Fix the class, not the instance. Test the artifact.**
- **A domain-level 200 is not service health**; **sample the moving number twice.**
- **⚠ Concurrent-session hazard is real.** If HEAD moves or the tree dirties with work you did
  not do, STOP and inspect. **`pkill -f <pattern>` matches its own shell.**

**Open engineering debt — beyond §2.49:**
- **§2.48 harness refresh for AMS 3.1.0** — six outdated scenario expectations, the `validate-all`
  evidence glob (reports TC-I-05-SRT as NOEVID), the `v3.1.0` mock profile. Autonomous while the
  AMS trial lasts.
- Alerting UX D6–D8; cosmetics D9/D10/D12; `AlertsPage` delete errors are still unhandled (the
  pattern F14 fixed for saves).
- **Dependabot backlog:** #276 (Go modules, incl. kin-openapi 0.149 — mind the sticky-servers
  trap), #278 (Go 1.27), #277, #280–#282, #275 — v0.5.0 took only the CVE-relevant subset.
- `pulse rekey` (CodeQL #6, ADR-0004). Web tests that flake under host load — check `uptime`.
- Waiting on external things: LIM-10 cluster rework · capacity number (needs a dedicated licensed
  AMS) · TestFlight · §2.45 self-alert (the operator's two semantics answers).

**Do first, every session:**
1. **Gate reads** — prod health from inside the container (component-scoped `/healthz`), a
   ClickHouse count sampled TWICE, git/PR drift, concurrent-session check.
2. **AMS license** (expires 2026-10-16) and prod's collector.
3. **Operator answers** — vhost enabled? the package's `operator-expected.md` §1? Ant Media's reply (§2)?
4. Then §2.49.

**Operator queue:** `docs/operator-expected.md`. **How we got here** (read only if you need it):
`decisions.md` (D-193, D-194) · `agents/handoffs/sessions/` · `docs/assessment/`.

---
## 1. CURRENT STATE (verified facts — refresh each session, never let this go stale)

- **Shipped product, pre-marketplace.** All 10 PRD features implemented. **Latest release:
  v0.5.0** (2026-10-07, tag on `8523b47`): every feature free (PolyForm Shield 1.0.0; the tier
  model is dormant code — `license.New` alone still enforces, `cmd/pulse` turns the
  all-features-free policy on). **Live-validated on AMS 3.1.0 Enterprise** (2026-10-07: 44/61
  scenario scripts pass, none of the failures an AMS regression; live alerting, player QoE, login
  backoff and the installer) and on AMS 3.0.3 Enterprise (46/50, July). **Known wrong:** F2's
  audience view returns 0 and F3/F6 aggregates are wrong for SDK traffic (LIM-30, §2.49).
- **Production** runs on this VPS on the stamped **v0.5.0** build (`8523b47`, rolled 2026-10-07,
  D-194; rollback image `pulse-prod-pulse:pre-d194` = v0.4.5-9). Health `ok` on every component,
  ingesting from the operator's `antmedia` container — **AMS 3.1.0 Enterprise, `--network host`,
  trial key to 2026-10-16**, app REST filter `127.0.0.1,172.16.0.0/12,<host public IP>/32`.
  Canonical three-file compose set; publishes `127.0.0.1:8090-8092` for host nginx;
  `PULSE_METRICS_TOKEN` set. **The `pulse.beyondkaira.com` vhost is NOT in `sites-enabled`**
  (removed ~2026-08-11; re-enabling needs the operator's `sudo`). Schema matches the tree
  (migrations 0001–0011 applied).
- **The iOS app exists and is CI-verified** (D-186). `ios/PulseKit` — Foundation-only, **296 tests
  green on Linux** (re-verified S122 in CI's `swift:6.1` container from a clean `git archive` copy), which is the point of the split: no Apple toolchain exists on this VPS, so
  anything living in a SwiftUI view is logic no gate here can check. `ios/PulseApp` — SwiftUI plus
  an AVPlayer view instrumented with our own `PulseBeacon` SDK, **50 tests green on a real iOS 26.2
  simulator** via `macos-15`. The archive/sign/upload job is written and has **never executed** —
  it cannot until an Apple Developer account exists. Measured runner facts (re-measure after an
  image bump): `docs/mobile/ci-runner-facts.md`.
- **The public website exists** (D-186) at `website/` — landing, `/beta/`, `/privacy/`, `/support/`,
  `/terms/` and the **`/get/` install page** (the Ant Media listing's main button points there),
  built from the brandkit against `tokens.json`, zero external requests (enforced by a check, not
  by intent). GitHub Pages is **enabled** (`build_type: workflow`) and publishes to
  `https://aytekxr.github.io/ams-pulse/` on merge to `main`.
- **`main` is protected** (strict, 1 review, `enforce_admins=false` so owner pushes work; **18**
  required contexts — D-185 added `shellcheck` and `doc-stamps`, D-186 added `ios-kit`; D-191 added `npm-audit` and `CodeQL`. A guard job
  that is not in that list cannot block a merge. `ios-app` is deliberately excluded: it depends on
  a third-party runner image and Homebrew, the same argument that excludes `compose-boot`. What
  that leaves uncovered is that a SwiftUI regression can merge on a red `ios-app` if ignored).
  The live setting and the script now MATCH — verified S122 via the API: 16 contexts, exactly the
  script's 14 plus the two CodeQL `Analyze (…)` jobs. The earlier warning that the script still
  needed re-running was stale. D-194 added a non-required `api-docs` job (it fetches a pinned
  ReDoc bundle from a CDN, so it must not block merges).
  Work on a branch → PR → merge on green.
- **Known limitations are disclosed, not hidden:** `docs/known-limitations.md` carries 30
  entries, 29 active (LIM-30, added S125: SDK-session analytics/usage/QoE aggregates are wrong —
  §2.49; LIM-29, the tier retention cap, retired in v0.5.0 and kept as a record). **LIM-01 was closed in D-179** (standalone CPU/mem/disk now work without Kafka) and
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
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014) —
  **dormant since v0.5.0** (all-features-free policy, D-194). Re-enabling any gate is an operator decision.
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
  migrations are in the current checkout — against whatever binary is deployed. Prod and `main`
  are both v0.5.0 (2026-10-07), so nothing is pending today — but it is a real landmine the moment
  `main` gains a migration prod lacks: on 2026-07-31 a password rotation recreated the stack, applied
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
