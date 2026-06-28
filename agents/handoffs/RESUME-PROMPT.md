# Resume prompt — Pulse (next session)

> **Rewritten 2026-06-24 (session 9 audit) — stale/contradictory history pruned; every claim below was
> re-verified this session, not assumed.** Pulse = self-hosted analytics/QoE/alerting for Ant Media Server.
> Repo: `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Full decision log: `agents/handoffs/decisions.md`
> (D-001…D-034, binding). Detailed phase plan: `agents/handoffs/PRODUCTION-READINESS.md`. AMS operator guide:
> `agents/handoffs/AMS-INTEGRATION.md`. Go-live runbook + rollback: `deploy/runbooks/real-ams-go-live.md`.

---

## 0. VERIFIED CURRENT STATE (re-verified 2026-06-28 — facts, not assumptions; prod now on self-hosted AMS, D-034)

- **Production is LIVE on a SELF-HOSTED AMS (D-034, 2026-06-28).** `https://beyondkaira.com` → operator-owned
  `antmedia` container (AMS Enterprise 3.0.3, `--network host`, `http://161.97.172.146:5080`), **NOT**
  test.antmedia.io. `/healthz` = ok (clickhouse/collector/meta_store all ok); `/api/v1/live/overview` →
  `total_publishers:1` (LiveApp `teststream` = a synthetic 2 Mbps publisher in container `ams-teststream` —
  remove via `docker rm -f ams-teststream` once real streams flow). AMS admin + license in `oguz-testing.md`
  (gitignored). [verified by curl + `docker ps` 2026-06-28]. The mock-ams seeded demo is **retired**.
- **Branch `ams-integration` @ `ea30367`, in sync with origin, clean tree.** ⚠️ **It is 7 commits AHEAD of
  `main` — `main` is STALE** and does NOT contain D-029/D-030/D-031/D-032 (the real-AMS wire fixes, maskDSN,
  aggregator target fix, golang:1.25 fix). Production runs `ams-integration`, NOT `main`. [verified by
  `git log main..ams-integration`].
- **Go suite green at `ea30367`** (`go test ./... -race`, repo-root mount) — re-run this session, see §B.
- **The prod image embeds the web UI** (multi-stage `deploy/docker/pulse.Dockerfile`: `npm ci && npm run
  build` → embedded in the Go binary), so the go-live build passing implies the web build passed. [verified].

---

## 1. PENDING USER ACTIONS (only the operator can do these — persist these every session)

| # | Action | Why it's blocked / needed |
|---|---|---|
| U1 | ✅ **RESOLVED (D-034).** Replaced the shared test.antmedia.io with a self-hosted AMS on this VPS and set each app's `remoteAllowedCIDR=0.0.0.0/0`, so the Pulse container polls cleanly (200). No external allow-list dependency remains. | (was: 8/16 apps 403'd the VPS "Not allowed IP" on test.antmedia.io). |
| U2 | **Confirm GitHub Actions CI is green** (or paste red job logs). | Repo is private and `gh` is NOT installed on the VPS → the agent **cannot see Actions**. CI-green-on-GitHub has NEVER been confirmed — it is an unverified assumption (see §Assumptions A2). |
| U3 | **Activate a Pro+ license** on `beyondkaira.com`. | QoE/beacon ingest (F3) is gated to Pro+ (`CheckBeaconIngest` returns 403 on Free). Without it, `beacon_events` stays empty and QoE features/alerts can't be exercised in prod. |
| U4 | **GitHub admin: run `.github/branch-protection.sh` + push a `v*` tag.** | Needs `gh` + repo-admin; gates `main` and exercises the GHCR release. Can't be done from the VPS. |
| U5 | **Open `https://beyondkaira.com` in a browser, confirm the SPA renders with no CSP console errors.** | The agent can't run a real browser; CSP is browser-enforced. Report any violation → instant fix. |
| U6 | **(Optional) install `gh` on the VPS** (`! sudo apt-get install -y gh && gh auth login`, needs sudo pw). | Removes the CI blind spot (U2) and lets the agent run U4 itself. |

---

## 2. DONE (verified) vs MISSING (backlog) — no "done" without verification

**DONE — verified live or by green test:** real-AMS go-live (D-031); real-AMS wire correctness — bitrate
bps→kbps, FPS-redistribution, QoE fields, `terminated_unexpectedly`, WebRTC single-track (D-029v/D-030);
`maskDSN` password-leak fix (D-031); aggregator honors configured bitrate target (D-031); cookie-session
auth + per-app paths + multi-app keying (D-029); `golang:1.26`→`1.25` (D-032).

**MISSING / NOT DONE (actionable backlog — detail in `PRODUCTION-READINESS.md`):**
- **Branch hygiene:** `ams-integration` not merged to `main` (prod & main diverged — deploying from main ships
  OLD buggy code). [P0]
- **Webhook unreachable:** `Caddyfile.prod` has no `/webhook/*` route → AMS lifecycle webhooks 404. [P1]
- **Silently-stubbed features (look done, fail in prod):** alert-channel **test-fire is a no-op** (returns 202,
  never calls `Send()`); **3 license gates unenforced** (`CheckDataAPI` on analytics, `CheckNodeLimit`,
  `CheckPrometheus`) = monetization leak; **standalone node card blank** (`SystemStats()` implemented but
  never called → Fleet/CPU/RAM empty); **`EventWebRTCClientStats` dropped** by the aggregator (viewer QoE
  never surfaces); `rebuffer_ratio`/`error_rate` alerts proxy from HealthScore, not real beacon data. [P0–P1]
- **Reliability gaps:** alert delivery has **no retry** (transient SMTP/Slack failure = silent miss); no
  backups; no container resource limits; ClickHouse shutdown drops ~2s of events (sleep, not drain);
  `alert_history` table never pruned. [P1–P2]
- **Security:** secrets are plaintext env vars (B3 Docker secrets); API tokens SHA-256 (not bcrypt); B7
  per-source webhook secret (contract CR). [P2–P3]
- **Feature completion (PRD):** QoE/beacon e2e (needs U3); Postgres meta backend (HA); SSO/OIDC; mobile SDKs;
  native WebRTC/RTMP/DASH probes; white-label PDF logo. [P3]
- **Testing:** see §B — 3 packages at 0.0%, 0 Playwright, no coverage gate, no response-body contract tests,
  shallow e2e. [P0 for prod-readiness]

---

## 3. IMMEDIATE NEXT STEPS (do first, in order — each with verification)

- **Step A — `golang:1.26`→`1.25`** ✅ DONE (D-032, committed `ea30367`). Verify: `grep -rn golang:1.26 deploy/ .github/` → empty.
- **Step B — Merge `ams-integration` → `main`.** Gate: full `-race` (repo-root mount) GREEN + **0 SKIP** in api
  pkg. Then `git checkout main && git merge ams-integration --no-ff && git push`. Drop vestigial
  `AMS_LOGIN_*` from `deploy/.env.example`; add commented `PULSE_AMS_APPLICATIONS=` + `PULSE_INGEST_TARGET_BITRATE_KBPS=`.
  Verify: `git log main..ams-integration` → empty; re-run CI.
- **Step C — Wire the Caddy `/webhook/*` route** in `deploy/config/Caddyfile` + `Caddyfile.prod` (before the
  catch-all): `handle /webhook/* { reverse_proxy pulse:8092 { header_up X-Forwarded-For {remote_host} } }`;
  confirm pulse `expose:` includes the webhook port; `restart caddy`. Verify: POST a signed test event → 200.

---

## 4. BACKLOG = WORKFLOW-DRIVEN PHASES (orchestrate EACH phase as a Workflow)

Full detail + exact scopes/commands in **`agents/handoffs/PRODUCTION-READINESS.md`**. Sequence:
1. **`pulse-p1-gaps`** — close the P0 silently-stubbed features (alert test-fire, standalone node card,
   WebRTC aggregator case, `PULSE_ALLOWED_WS_ORIGINS`). TDD each.
2. **`pulse-test-backfill`** — TDD coverage to every level + enforced gate (3 sub-workflows: Go unit, web
   coverage gate, e2e+contract). See §5/§6.
3. **`pulse-prod-harden`** — license-gate enforcement, B3 Docker secrets, alert retry, `alert_history`
   pruning, CH drain, resource limits, Trivy/SBOM, request-ID middleware.
4. **`pulse-feature-complete`** — QoE/beacon e2e (after U3), AMS version surfacing, anomaly expansion, native
   probes, white-label PDF, B7 (contract CR), SSO/OIDC, mobile SDKs, backup sidecar, Postgres backend.

---

## 5. TDD ENFORCEMENT (BINDING from now on — bias toward test coverage over implementation speed)

**Every change follows red→green→refactor: write the failing test FIRST, watch it fail, implement, watch it
pass.** For each unit of work produce tests at ALL applicable levels (do not stop at "unit"):

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
**Prioritize critical business logic first:** (1) license/tier enforcement, (2) alert firing + delivery,
(3) ingest health scoring, (4) AMS wire decode/normalize, (5) the query layer. Report coverage in every handoff.

---

## 6. VERIFICATION WORKFLOW (BINDING — every implementation runs ALL of these before "done")

1. **Build:** `go build ./...` (CGO_ENABLED=0) + `cd web && npm run build`.
2. **Lint:** `cd web && npm run lint`; Go `gofmt -l` (must be empty) + `go vet ./...`.
3. **Type-check:** `cd web && npm run typecheck` (or `tsc --noEmit`).
4. **Test (race):** `go test ./... -race -count=1` **repo-root mount** (D-028: server-only mount silently
   skips ~90 api tests → false green). Confirm **0 FAIL, 0 unexpected SKIP**.
5. **Coverage:** the gate command in §5; attach numbers to the handoff.
6. **Contract drift:** `cd web && npm run gen:api` then `git diff --exit-code` (generated types match spec);
   `redocly lint` + `ajv` on event schemas.
7. **Staging verify:** bring the change up on an **isolated compose project** (NOT pulse-prod) and curl the
   affected endpoints. Never verify on prod first.
8. **Deploy smoke (after a prod change):** `/healthz` ok via `--resolve`; affected endpoint returns expected
   real data; `pulse logs` shows no 401/403/decode/login errors; for migrate, DSN masked (`:xxxxx@`).
9. **Independent/adversarial re-check:** default to "refuted" until reproduced on a fresh build (D-013/017/019).
   A verify harness that silently skips == no verify (D-028).

---

## 7. WORKFLOW SUGGESTIONS (prefer workflows; break large tasks into small verifiable ones)

- **Feature:** `pulse-feature-<name>` — fan out disjoint-scope authors → TDD tests → adversarial verify →
  ORCH gate → ORCH commit by explicit path.
- **Testing:** `pulse-test-backfill` — per-package finder measures coverage, authors the missing
  unit/edge/failure tests TDD-style, re-measures; a completeness critic asks "which exported fn has no test?".
- **Deployment:** `pulse-deploy-<target>` — pre-flight (config -q + login) → isolated staging verify →
  prod swap → post-swap smoke → handoff. (Pattern: `deploy/runbooks/real-ams-go-live.md`.)
- **Monitoring:** `pulse-monitor` — periodic poll of `/healthz` + `/live/overview` + `pulse logs` for AMS
  wire drift / 403 storms / decode errors; surface regressions.
- **Rollback:** `pulse-rollback` — re-point pulse to the prior image/overlay (no `-v`), restore the prior
  state, smoke-verify. (Real-AMS rollback steps: runbook §5.)
- **Verification/audit:** `pulse-<x>-audit` — adversarial finders + refute pass (pattern proven in
  D-029v/D-031/D-032).

---

## 8. ASSUMPTIONS TO ELIMINATE (replace each with a verified fact; bias toward verification)

| # | Assumption (currently unverified or known-false) | How to eliminate |
|---|---|---|
| A1 | "`main` reflects production." **FALSE** — 7 commits behind. | Step B merge → `git log main..ams-integration` empty. |
| A2 | "CI is green on GitHub." **UNVERIFIED** (private repo, no gh). | U2 (user confirms) or U6 (install gh) → read the run. |
| A3 | "Alerts fire and deliver." **UNVERIFIED** — test-fire is a stub, no retry, no e2e. | Implement Send() + retry; add alert-fires→history e2e + a delivery test. |
| A4 | "Coverage is adequate." **FALSE** — 3 pkgs 0%, no gate. | `pulse-test-backfill` + coverage gate (§5). |
| A5 | "The 0.0% pkgs are covered by integration tests." Partially — only 3 of ~12 query methods. | Add unit tests with a mock Conn (§B). |
| A6 | "QoE/beacon works in prod." **UNVERIFIED** — needs Pro+ license. | U3 + beacon→qoe/summary e2e. |
| A7 | "The SPA renders / CSP is correct." **UNVERIFIED** — no Playwright, no browser run. | U5 + Playwright CSP/render test. |
| A8 | "Response bodies match the OpenAPI contract." **UNVERIFIED** — only spec-linting. | Response-body contract tests (kin-openapi). |
| A9 | "The real-AMS wire format is fully characterized." Partial — fixtures from one capture. | Watch pulse logs for decode errors; add a fixture-replay contract test; re-capture periodically. |
| A10 | "test123 represents production load." **FALSE** — 1 low-bitrate camera, 0 viewers. | Load/perf test (many streams/apps/viewers); VD-04 render-time at scale. |
| A11 | "Migrations are idempotent & safe." Assumed (`IF NOT EXISTS`). | Explicit migrate round-trip + re-run test. |
| A12 | "ClickHouse shutdown loses no events." **FALSE** — 100ms sleep, not drain. | Drain-on-close + a no-loss test. |
| A13 | ✅ Moot (D-034): switched to a self-hosted AMS; `remoteAllowedCIDR=0.0.0.0/0` lets Pulse poll all apps (200). | — |

---

## 9. BINDING FLOWS — every workflow MUST end with these (user directive)

- **Verify** — independent/adversarial re-check of *every* claim against a running stack or fresh build;
  default to "refuted" until reproduced; **repo-root mount** or api tests silently skip (D-028). QA alone is
  not authoritative (D-013/017/019).
- **Commit** — by **EXPLICIT path**, per scope; never `git add -A/-u/.` (parallel agents share the tree —
  D-008/D-011). In a workflow, agents AUTHOR only; ORCH commits centrally (avoids `.git/index.lock` races).
  Message `<scope> D-0NN: <summary>` + evidence. Push when the user directs.
- **Handoff** — update THIS `RESUME-PROMPT.md` + `decisions.md` (new D-0NN) every session, then commit + push.

## 10. OPERATING PROTOCOL (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase = one Workflow: ORCH writes the plan + pre-approved CRs to
  `decisions.md`, fans out to disjoint-scope agents, then **independently gates**. Background work is
  harness-tracked — you're re-invoked on completion; don't poll-spin.
- **Anti-stall (D-016):** NEVER run `pulse serve`/`clickhouse server` in the foreground inside an agent. Use
  `docker compose up -d` (detached) + health polling; CH unit work via the integration harness. `timeout` on
  builds, `-timeout` on `go test`, vitest `run` not watch, `curl -m`. Long local repros: Bash
  `run_in_background: true`.
- **Single-writer scope map** in `agents/manifest.yaml`. **Contracts frozen (D-004)** — changes only via an
  ORCH-approved CR applied by INT-01 (OpenAPI + event schemas + migrations).
- **⚠️ Workflow/fork agents have Write+commit access** — a reviewer fork once auto-committed during a
  concurrent ORCH edit (D-030 process note). Scope reviewer agents read-only when ORCH is editing the same files.

## 11. HARD RULES (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in ClickHouse, config
  in the meta store, never crossed; web UI consumes ONLY generated public-API types; beacon ingest is hostile input.
- `CGO_ENABLED=0` for the shipping build (pure-Go sqlite); single binary `pulse serve|migrate|diag`; React 19 +
  RR7 + Vite + TS strict; recharts; no external fonts/CDNs. `go test -race` needs `CGO_ENABLED=1` + gcc.
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/`. Base `docker-compose.yml` stays clean (`expose:`, no host ports); exposure in
  overrides. Prod stack = `base + hardened + prod-tls + real-ams`.

## 12. ENVIRONMENT (VPS)

- **Ubuntu 24.04 VPS `161.97.172.146`**, Docker 29 + Compose v5. **`go` is NOT on PATH** — run Go only in
  Docker (`golang:1.25`). node 20 + npm 10 on PATH. **`gh` NOT installed.**
- **⚠️ For `go test` mount the REPO ROOT** (`-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e
  GOFLAGS=-buildvcs=false`): a `server/`-only mount makes `metaDDLPath` escape the mount → `t.Skip` →
  skip-counts-as-pass false green (~90 api tests). Confirm **0 SKIP** for api.
- **Docker:** user `aytek` is in `docker` group but stale in non-login shells → prefix `sg docker -c "…"`.
  `sudo` needs a password → ask the user via the `! <cmd>` prompt for privileged ops.
- **Real-AMS prod ops** (run from repo root): `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml
  --env-file deploy/.env"`. Status: `sg docker -c "docker compose $DC ps"`. Admin token: in `oguz-testing.md`
  (gitignored) — persisted in the `pulse-prod_pulse-data` volume; **never `down -v` that volume.** TLS check:
  always `--resolve beyondkaira.com:443:161.97.172.146` (VPS DNS is stale). Rollback: runbook §5.
- `deploy/.env`, `*.db*`, `oguz-testing.md`, `web/pulse_secret.key` are gitignored — never commit.
