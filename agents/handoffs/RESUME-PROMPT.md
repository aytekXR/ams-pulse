# Pulse — Resume / handoff prompt (SINGLE source of truth)

> **This is the one handoff doc.** It supersedes the previous separate "next-session" prompt (merged 2026-06-29,
> D-037); don't recreate a second handoff file — update THIS one + `decisions.md` each session.
> Pulse = self-hosted analytics/QoE/alerting for Ant Media Server. Repo: `/home/aytek/repo/ams-pulse`
> on VPS `161.97.172.146`. Full decision log: `agents/handoffs/decisions.md` (D-001…D-037, binding).
> Detailed phase plan: `agents/handoffs/PRODUCTION-READINESS.md`. AMS operator guide:
> `agents/handoffs/AMS-INTEGRATION.md`. Go-live runbook + rollback: `deploy/runbooks/real-ams-go-live.md`.
> Operator creds/keys (gitignored, never commit): `oguz-testing.md`.

---

## ▶ START HERE (next session — operator-directed 2026-06-29/30)

**FIRST — get CI GREEN (it is currently RED; keep working on `main`).** Root cause (verified 2026-06-30, D-038):
`.github/workflows/ci.yml` → `server` job → **"Integration tests"** step downloads ClickHouse from an **unpinned,
rolling `master` URL** (`https://builds.clickhouse.com/master/amd64/clickhouse`; the comment claims "v26.6.1" but the
URL is `master`). It's environmental — `git diff 1d7a26f(last-green D-034)..HEAD -- server/ contracts/ .github/` is
EMPTY (every commit since is docs/deploy); the master binary rolled (26.6.1 → 26.7.1.281) and the 06-29 snapshot was
broken. Reproduced faithfully (golang:1.25, repo-root mount, exact CI cmd): the CURRENT master PASSES all integration
tests locally — i.e. it self-healed, but `master` is non-deterministic and will break again. **FIX — pin the binary**
(replace the download step's URL; tarball verified reachable 200):
```
CH_VER=26.6.1.1193   # = the "v26.6.1" the comment intended; same 26.x line the harness is validated against
curl -fsSL -o /tmp/ch.tgz \
  https://github.com/ClickHouse/ClickHouse/releases/download/v${CH_VER}-stable/clickhouse-common-static-${CH_VER}-amd64.tgz
tar -xf /tmp/ch.tgz -C /tmp
cp "$(find /tmp -type f -name clickhouse -path '*/bin/*' | head -1)" /tmp/clickhouse
chmod +x /tmp/clickhouse && /tmp/clickhouse --version
```
Also fix the misleading `v26.6.1` comment. **Verify loop:** (1) reproduce locally — golang:1.25 + repo-root mount,
download the pinned binary, then `cd server && go test -tags integration ./... -timeout 300s` → all `ok`; (2) commit
`ci.yml` by explicit path, push to `main`; (3) `gh run watch $(gh run list --branch main --workflow=ci.yml -L1 --json databaseId -q '.[0].databaseId')`
→ confirm the whole `ci` workflow is green. `gh` is installed+authed now (U6 ✅), so read Actions directly. *(Separately,
the scheduled `ams-version-matrix.yml` workflow is also red — a different, pre-existing issue; not part of this fix.)*

**THEN — run the `pulse-p1-gaps` workflow** — close the P0 silently-stubbed features, TDD red→green, one disjoint-scope
author per item, then ORCH gate + commit-by-explicit-path:
1. Alert **test-fire** must really call `Send()` (today returns 202, never sends).
2. **Enforce** the 3 license gates `CheckDataAPI` (analytics), `CheckNodeLimit`, `CheckPrometheus` (monetization leak).
3. **Standalone node card** via `SystemStats()` (implemented but never called → Fleet CPU/RAM blank).
4. **WebRTC** `EventWebRTCClientStats` aggregator case (viewer QoE dropped) + `PULSE_ALLOWED_WS_ORIGINS`.

End with the binding **Verify → Commit (explicit path) → Handoff** flow (§11). **Then:** retire the stale
`ams-integration` pointer + wire the Caddy `/webhook/*` route (§3), then run `pulse-test-backfill` (§6).
AMS web login is RESOLVED (D-036) — not a blocker.

---

## 0. VERIFIED CURRENT STATE (facts, not assumptions)

- **Production is LIVE on a SELF-HOSTED AMS (D-034).** `https://beyondkaira.com` (apex) + subdomains
  `https://pulse.beyondkaira.com` (app) and `https://ams.beyondkaira.com` (AMS panel) — all real Let's Encrypt
  TLS via Caddy. Backend = operator-owned `antmedia` container (AMS Enterprise 3.0.3, `--network host`,
  `http://161.97.172.146:5080`), **NOT** test.antmedia.io. `/healthz` = ok (clickhouse/collector/meta_store);
  `/api/v1/live/overview` → `total_publishers:1` (LiveApp `teststream` = a synthetic 2 Mbps publisher in container
  `ams-teststream` — `docker rm -f ams-teststream` once real streams flow). The mock-ams seeded demo is **retired**.
  [re-verified by curl 2026-06-29].
- **AMS web-console login RESOLVED (D-036, 2026-06-29).** The AMS console MD5-hashes the password client-side, but
  both admin accounts were REST-provisioned (D-034) with the plaintext password, so the browser's hashed submission
  never matched. Fixed by re-provisioning `aytek@` + `admin@` with `MD5(realpassword)`; both now web-login, Pulse
  (plaintext) unaffected. Brute-force lockout = **2 tries → 5-min block, per-EMAIL not IP**. AMS is the **latest
  stable** (3.0.3 == Docker Hub `latest`); trial license valid to 2026-07-12. Opened the newly-created `pulse-test`
  app's `remoteAllowedCIDR` 127.0.0.1→0.0.0.0/0 (logs clean — every new AMS app defaults to 127.0.0.1). Values in
  `oguz-testing.md`.
- **Branch state (CORRECTED 2026-06-29) — the old "main is 7 behind / prod runs ams-integration" note is OBSOLETE.**
  `main` @ `33efe35` is the working branch and is **ahead of / fully contains** `ams-integration`
  (`git rev-list --count main..ams-integration` = **0**; `ams-integration..main` = **5**). `ams-integration`
  (@ `4dd448a`) is now a **stale pointer to retire**. `main` is ahead of `origin/main` (the handoff commits D-036–D-037,
  push pending). Remaining branch work: delete the stale `ams-integration` ref + apply branch protection + a `v*` tag (U4).
- **Go suite green / coverage 47.5%** as of the last full run (`go test ./... -race -cover`, **repo-root mount**,
  golang:1.25, 2026-06-28). Re-run at the start of `pulse-p1-gaps` to confirm before editing (§6/§7).
- **The prod image embeds the web UI** (multi-stage `deploy/docker/pulse.Dockerfile`: `npm ci && npm run build` →
  embedded in the Go binary), so a passing go-live build implies the web build passed.

---

## 1. PENDING USER ACTIONS (only the operator can do these — persist every session)

| # | Action | Why it's blocked / needed |
|---|---|---|
| U1 | ✅ **RESOLVED (D-034).** Self-hosted AMS on this VPS; per-app `remoteAllowedCIDR=0.0.0.0/0` so Pulse polls cleanly (200). No external allow-list dependency. | (was: 8/16 apps 403'd the VPS on test.antmedia.io). |
| U2 | **Agent task now (no longer operator-blocked):** get the `ci` workflow GREEN — it's RED at the integration step (verified root cause + pinned-binary fix in ▶ START HERE). | `gh` is installed (U6 ✅) → the agent reads, fixes, and verifies CI itself. |
| U3 | **Activate a Pro+ Pulse license** on `beyondkaira.com` (`PULSE_LICENSE_KEY`, see §5). | QoE/beacon ingest (F3) is gated to Pro+ (`CheckBeaconIngest` 403 on Free). Without it `beacon_events` stays empty; QoE features/alerts can't be exercised in prod. *(This is a Pulse license — separate from the AMS license.)* |
| U4 | **GitHub admin: run `.github/branch-protection.sh` + push a `v*` tag.** | Needs `gh` + repo-admin; gates `main` and exercises the GHCR release. Can't be done from the VPS. |
| U5 | **Open `https://beyondkaira.com` AND `https://pulse.beyondkaira.com` in a browser; confirm the SPA renders with no CSP console errors on each** (Caddy serves both — apex via the catch-all, subdomain via its own block, so they can fail independently). | The agent can't run a real browser; CSP is browser-enforced. Report any violation → instant fix. |
| U6 | ✅ **DONE (2026-06-30).** `gh` is installed + authed (account `aytekXR`, ssh). The CI blind spot is gone — the agent now reads Actions directly (so it can also do U4). | — |

---

## 2. DONE (verified) vs MISSING (backlog) — no "done" without verification

**DONE — verified live or by green test:** real-AMS go-live (D-031); real-AMS wire correctness — bitrate
bps→kbps, FPS-redistribution, QoE fields, `terminated_unexpectedly`, WebRTC single-track (D-029v/D-030);
`maskDSN` password-leak fix (D-031); aggregator honors configured bitrate target (D-031); cookie-session auth +
per-app paths + multi-app keying (D-029); `golang:1.26`→`1.25` (D-032); subdomains + Caddy TLS (D-034/D-035);
AMS web-console login (D-036); `ams-integration` is now contained in `main` (branch divergence resolved).

**MISSING / NOT DONE (actionable backlog — detail in `PRODUCTION-READINESS.md`):**
- **Silently-stubbed features (look done, fail in prod) [P0]:** alert-channel **test-fire is a no-op** (returns 202,
  never calls `Send()`); **3 license gates unenforced** (`CheckDataAPI`, `CheckNodeLimit`, `CheckPrometheus`) =
  monetization leak; **standalone node card blank** (`SystemStats()` never called → Fleet/CPU/RAM empty);
  **`EventWebRTCClientStats` dropped** by the aggregator (viewer QoE never surfaces). *(The `rebuffer_ratio`/`error_rate`
  alerts proxy from HealthScore, not real beacon data — fixing that needs actual beacon data → blocked on U3; tracked
  under QoE/beacon e2e in phase 4 (§4), NOT a `pulse-p1-gaps` target.)*
- **Webhook unreachable [P1]:** `Caddyfile.prod` has no `/webhook/*` route → AMS lifecycle webhooks 404.
- **Branch cleanup [P2]:** retire the stale `ams-integration` pointer; branch protection + `v*` tag (U4).
- **Reliability gaps [P1–P2]:** alert delivery has **no retry** (transient SMTP/Slack failure = silent miss); no
  backups; no container resource limits; ClickHouse shutdown drops ~2s of events (sleep, not drain); `alert_history`
  table never pruned.
- **Security [P2–P3]:** secrets are plaintext env vars (B3 Docker secrets); API tokens SHA-256 (not bcrypt);
  B7 per-source webhook secret (contract CR).
- **Feature completion (PRD) [P3]:** QoE/beacon e2e (needs U3); Postgres meta backend (HA); SSO/OIDC; mobile SDKs;
  native WebRTC/RTMP/DASH probes; white-label PDF logo.
- **Testing [P0 for prod-readiness]:** 3 packages at 0.0%, 0 Playwright, no coverage gate, no response-body contract
  tests, shallow e2e — full breakdown in §6.

---

## 3. IMMEDIATE NEXT STEPS (do in order — each with verification)

- **Step A — `golang:1.26`→`1.25`** ✅ DONE (D-032). Verify: `grep -rn golang:1.26 deploy/ .github/` → empty.
- **Step B — Merge `ams-integration` → `main`** ✅ EFFECTIVELY DONE (2026-06-29): `main` now contains `ams-integration`
  (`git log main..ams-integration` empty). Remaining: **delete the stale `ams-integration` branch** (local + origin
  after a final diff confirms 0 unique commits), drop vestigial `AMS_LOGIN_*` from `deploy/.env.example`, add commented
  `PULSE_AMS_APPLICATIONS=` + `PULSE_INGEST_TARGET_BITRATE_KBPS=`.
- **Step C — Wire the Caddy `/webhook/*` route** in `deploy/config/Caddyfile` + `Caddyfile.prod` (before the catch-all):
  `handle /webhook/* { reverse_proxy pulse:8092 { header_up X-Forwarded-For {remote_host} } }`; confirm pulse `expose:`
  includes the webhook port; restart caddy. Verify: POST a signed test event → 200.

---

## 4. BACKLOG = WORKFLOW-DRIVEN PHASES (orchestrate EACH phase as a Workflow)

Full detail + exact scopes/commands in **`agents/handoffs/PRODUCTION-READINESS.md`**. Sequence:
1. **`pulse-p1-gaps`** — close the P0 silently-stubbed features (alert test-fire real `Send()`, **enforce the 3 license
   gates** `CheckDataAPI`/`CheckNodeLimit`/`CheckPrometheus`, standalone node card via `SystemStats()`, WebRTC
   `EventWebRTCClientStats` aggregator case, `PULSE_ALLOWED_WS_ORIGINS`). TDD each. *(Next session — see ▶ START HERE.)*
2. **`pulse-test-backfill`** — TDD coverage to every level + enforced gate (3 sub-workflows: Go unit, web coverage
   gate, e2e+contract). See §6/§7.
3. **`pulse-prod-harden`** — B3 Docker secrets, alert retry, `alert_history` pruning, CH drain, resource limits,
   Trivy/SBOM, request-ID middleware. (License-gate **enforcement** moves up to `pulse-p1-gaps`/phase 1; this phase
   only deepens coverage of the gates per §6.)
4. **`pulse-feature-complete`** — QoE/beacon e2e (after U3), AMS version surfacing, anomaly expansion, native probes,
   white-label PDF, B7 (contract CR), SSO/OIDC, mobile SDKs, backup sidecar, Postgres backend.

---

## 5. INTEGRATION KEYS (operator provides any subset; agent wires + verifies each on staging first, then prod)

Agent stores in `deploy/.env` (gitignored), wires, and verifies **real** behavior end-to-end. **Never commit keys.**
⚠️ Wire each alongside fixing the **stub the key would otherwise hide** (alert test-fire no-op; the 3 unenforced
license gates) — TDD each.

| Capability | Provide | Unlocks |
|---|---|---|
| **Pulse license** (Pro+/Business/Ent) | `PULSE_LICENSE_KEY` (or signed file + `PULSE_LICENSE_PUBKEY`) | QoE/beacon ingest (U3), anomalies, data API, probes, reports, Prometheus, multi-tenant — today gated to Free |
| **Email alerts** | SMTP host/port/user/pass (or SES/SendGrid key) | email alert delivery |
| **Slack alerts** | Slack incoming-webhook URL | Slack alert delivery |
| **PagerDuty** | routing/integration key | PagerDuty alert delivery |
| **Telegram** | bot token + chat id | Telegram alert delivery |
| **Generic webhook** | target URL + shared secret | webhook alert delivery |
| **S3 report export** | `PULSE_S3_ACCESS_KEY_ID`/`_SECRET_ACCESS_KEY`/`_BUCKET`/`_REGION`(/`_ENDPOINT`) | CSV/PDF report storage |
| **Geo enrichment** | MaxMind license key → GeoLite2-City.mmdb (`PULSE_GEO_MMDB_PATH`) | viewer country/region |
| **Prometheus** | `PULSE_METRICS_TOKEN` (self-generate) | authed `/metrics` |

Implemented alert channels: **email, slack, pagerduty, telegram, webhook**.

---

## 6. TEST & CI HARDENING (so breakage is caught in CI) — orchestrate as workflows, TDD red→green

Baseline coverage (2026-06-28): total **47.5%**, all pass, no races (repo-root mount, golang:1.25).

**ZERO unit coverage (write tests FIRST):**
- `internal/query` **0%** — powers every dashboard chart + API read (highest blast radius). Unit-test with a mock Conn.
- `internal/config` **0%** — env parsing / startup correctness. Every var + bad-input failure paths.
- `internal/store/clickhouse` **0% unit** (integration covers only ~3/12 query methods) + `.../migrations` **0%**.
- `cmd/pulse` **1.2%** — serve/migrate/diag wiring.

**LOW + critical:** `internal/license` **36.9%** (billing/tier gates = revenue), `store/meta` **29.7%**,
`collector/logtail` **37.5%**, `internal/api` **52.2%**, `alert/channels` **56.8%**.
**STRONG (keep ratcheting):** collector/ingest 85, cluster 89, sessions 81, anomaly 76, amsclient 76, restpoller 72,
alert 72.

**Priority (critical-business-logic-first):**
1. `license` 37→≥85 **and ENFORCE** the 3 gates + alert test-fire real `Send()`.
2. `query` 0→≥70 (mock-Conn unit) — analytics behind every chart.
3. alert firing→delivery (`channels` 57→≥80) + **retry** + alert→history e2e. **[VERIFIED 2026-06-29 — real gap]**
   Unmuting the `Stream offline` default rule + stopping the zombie RTMP test stream produced **NO** history entry in
   130 s: `evalStreamOffline` reads the live snapshot and a vanished stream isn't in it. To *demonstrate* a visible
   alert use a snapshot-present metric (e.g. `ingest_bitrate_floor` with a threshold above the live bitrate) or a
   tracked/registered stream — and the firing→history path itself has no e2e. Fix + test this FIRST.
4. `config` 0→≥80 — all env vars + failure paths.
5. `store/clickhouse` + `meta` — unit + expand integration to all query methods.
6. AMS wire **fixture-replay regression** pinning D-029/D-031 (bps→kbps, FPS-redistribution, `terminated_unexpectedly`,
   WebRTC single-track).

**CI gaps to close (`.github/workflows`) — the "see breakage in CI" asks:**
- **ADD a coverage gate** — fail the build if total < floor OR any package regresses (ratchet). *(the #1 request)*
- **ADD Playwright browser e2e** (`web/e2e/`, NEW — none today): SPA renders, auth redirect, CSP enforced, large-table
  virtualization, zero console errors.
- **ADD response-body contract tests** (kin-openapi) in `internal/api`: assert real responses conform to
  `contracts/openapi/pulse-api.yaml` (CI only lints the spec today, never the responses).
- **ADD web coverage threshold** (`vitest --coverage` gate).
- **DEEPEN `e2e.yml`**: assert alert fires→delivered, beacon→QoE (after license), real-AMS fixture replay (today only
  checks overview activity>0 vs mock-ams).

---

## 7. TDD ENFORCEMENT (BINDING — bias toward test coverage over implementation speed)

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

## 8. VERIFICATION WORKFLOW (BINDING — every implementation runs ALL of these before "done")

1. **Build:** `go build ./...` (CGO_ENABLED=0) + `cd web && npm run build`.
2. **Lint:** `cd web && npm run lint`; Go `gofmt -l` (must be empty) + `go vet ./...`.
3. **Type-check:** `cd web && npm run typecheck` (or `tsc --noEmit`).
4. **Test (race):** `go test ./... -race -count=1` **repo-root mount** (D-028: server-only mount silently skips ~90 api
   tests → false green). Confirm **0 FAIL, 0 unexpected SKIP**.
5. **Coverage:** the gate command in §7; attach numbers to the handoff.
6. **Contract drift:** `cd web && npm run gen:api` then `git diff --exit-code` (generated types match spec);
   `redocly lint` + `ajv` on event schemas.
7. **Staging verify:** bring the change up on an **isolated compose project** (NOT pulse-prod) and curl the affected
   endpoints. Never verify on prod first.
8. **Deploy smoke (after a prod change):** `/healthz` ok via `--resolve`; affected endpoint returns expected real
   data; `pulse logs` shows no 401/403/decode/login errors; for migrate, DSN masked (`:xxxxx@`).
9. **Independent/adversarial re-check:** default to "refuted" until reproduced on a fresh build (D-013/017/019). A
   verify harness that silently skips == no verify (D-028).

---

## 9. WORKFLOW SUGGESTIONS (prefer workflows; break large tasks into small verifiable ones)

- **Feature:** `pulse-feature-<name>` — fan out disjoint-scope authors → TDD tests → adversarial verify → ORCH gate →
  ORCH commit by explicit path.
- **Testing:** `pulse-test-backfill` — per-package finder measures coverage, authors the missing unit/edge/failure
  tests TDD-style, re-measures; a completeness critic asks "which exported fn has no test?".
- **Deployment:** `pulse-deploy-<target>` — pre-flight (config -q + login) → isolated staging verify → prod swap →
  post-swap smoke → handoff. (Pattern: `deploy/runbooks/real-ams-go-live.md`.)
- **Monitoring:** `pulse-monitor` — periodic poll of `/healthz` + `/live/overview` + `pulse logs` for AMS wire drift /
  403 storms / decode errors; surface regressions.
- **Rollback:** `pulse-rollback` — re-point pulse to the prior image/overlay (no `-v`), restore the prior state,
  smoke-verify. (Real-AMS rollback steps: runbook §5.)
- **Verification/audit:** `pulse-<x>-audit` — adversarial finders + refute pass (pattern proven in D-029v/D-031/D-032).

---

## 10. ASSUMPTIONS TO ELIMINATE (replace each with a verified fact; bias toward verification)

| # | Assumption (currently unverified or known-false) | How to eliminate |
|---|---|---|
| A1 | ✅ Resolved (2026-06-29): `main` now **contains** `ams-integration` (`main..ams-integration` empty). | Retire the stale `ams-integration` ref + branch protection (U4). |
| A2 | "CI is green." **KNOWN-FALSE (2026-06-30, D-038)** — `ci` is RED at the integration step (unpinned ClickHouse `master`); now readable via `gh` (U6 ✅, no longer an assumption). | Apply the pinned-binary fix (▶ START HERE) → `gh run watch` green. |
| A3 | "Alerts fire and deliver." **UNVERIFIED** — test-fire is a stub, no retry, no e2e (verified gap §6.3). | Implement `Send()` + retry; add alert-fires→history e2e + a delivery test. |
| A4 | "Coverage is adequate." **FALSE** — 3 pkgs 0%, no gate. | `pulse-test-backfill` + coverage gate (§7). |
| A5 | "The 0.0% pkgs are covered by integration tests." Partially — only ~3 of ~12 query methods. | Add unit tests with a mock Conn (§6). |
| A6 | "QoE/beacon works in prod." **UNVERIFIED** — needs Pro+ license. | U3 + beacon→qoe/summary e2e. |
| A7 | "The SPA renders / CSP is correct." **UNVERIFIED** — no Playwright, no browser run. | U5 + Playwright CSP/render test. |
| A8 | "Response bodies match the OpenAPI contract." **UNVERIFIED** — only spec-linting. | Response-body contract tests (kin-openapi). |
| A9 | "The real-AMS wire format is fully characterized." Partial — fixtures from one capture. | Watch pulse logs for decode errors; add a fixture-replay contract test; re-capture periodically. |
| A10 | "The teststream represents production load." **FALSE** — 1 low-bitrate publisher, 0 viewers. | Load/perf test (many streams/apps/viewers); VD-04 render-time at scale. |
| A11 | "Migrations are idempotent & safe." Assumed (`IF NOT EXISTS`). | Explicit migrate round-trip + re-run test. |
| A12 | "ClickHouse shutdown loses no events." **FALSE** — 100ms sleep, not drain. | Drain-on-close + a no-loss test. |
| A13 | ✅ Moot (D-034): self-hosted AMS; `remoteAllowedCIDR=0.0.0.0/0` lets Pulse poll all apps (200). New apps default to 127.0.0.1 — open them. | — |

---

## 11. BINDING FLOWS — every workflow MUST end with these (user directive)

- **Verify** — independent/adversarial re-check of *every* claim against a running stack or fresh build; default to
  "refuted" until reproduced; **repo-root mount** or api tests silently skip (D-028). QA alone is not authoritative
  (D-013/017/019).
- **Commit** — by **EXPLICIT path**, per scope; never `git add -A/-u/.` (parallel agents share the tree — D-008/D-011).
  In a workflow, agents AUTHOR only; ORCH commits centrally (avoids `.git/index.lock` races). Message
  `<scope> D-0NN: <summary>` + evidence. Push when the user directs.
- **Handoff** — update **THIS `RESUME-PROMPT.md`** + `decisions.md` (new D-0NN) every session, then commit + push.

## 12. OPERATING PROTOCOL (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase = one Workflow: ORCH writes the plan + pre-approved CRs to
  `decisions.md`, fans out to disjoint-scope agents, then **independently gates**. Background work is harness-tracked —
  you're re-invoked on completion; don't poll-spin.
- **Anti-stall (D-016):** NEVER run `pulse serve`/`clickhouse server` in the foreground inside an agent. Use
  `docker compose up -d` (detached) + health polling; CH unit work via the integration harness. `timeout` on builds,
  `-timeout` on `go test`, vitest `run` not watch, `curl -m`. Long local repros: Bash `run_in_background: true`.
- **Single-writer scope map** in `agents/manifest.yaml`. **Contracts frozen (D-004)** — changes only via an
  ORCH-approved CR applied by INT-01 (OpenAPI + event schemas + migrations).
- **⚠️ Workflow/fork agents have Write+commit access** — a reviewer fork once auto-committed during a concurrent ORCH
  edit (D-030 process note). Scope reviewer agents read-only when ORCH is editing the same files.

## 13. HARD RULES (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in ClickHouse, config in the
  meta store, never crossed; web UI consumes ONLY generated public-API types; beacon ingest is hostile input.
- `CGO_ENABLED=0` for the shipping build (pure-Go sqlite); single binary `pulse serve|migrate|diag`; React 19 + RR7 +
  Vite + TS strict; recharts; no external fonts/CDNs. `go test -race` needs `CGO_ENABLED=1` + gcc.
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/`. Base `docker-compose.yml` stays clean (`expose:`, no host ports); exposure in
  overrides. Prod stack = `base + hardened + prod-tls + real-ams`.

## 14. ENVIRONMENT (VPS)

- **Ubuntu 24.04 VPS `161.97.172.146`**, Docker 29 + Compose v5. **`go` is NOT on PATH** — run Go only in Docker
  (`golang:1.25`). node 20 + npm 10 on PATH. **`gh` NOT installed.**
- **⚠️ For `go test` mount the REPO ROOT** (`-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e
  GOFLAGS=-buildvcs=false`): a `server/`-only mount makes `metaDDLPath` escape the mount → `t.Skip` →
  skip-counts-as-pass false green (~90 api tests). Confirm **0 SKIP** for api.
- **Docker:** user `aytek` is in `docker` group but stale in non-login shells → prefix `sg docker -c "…"`. `sudo` needs
  a password → ask the user via the `! <cmd>` prompt for privileged ops. For host-root debugging without sudo, run a
  privileged container in the host netns (e.g. `docker run --rm --net=host --cap-add=NET_RAW corfr/tcpdump …`, D-036).
- **Real-AMS prod ops** (run from repo root): `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml
  --env-file deploy/.env"`. Status: `sg docker -c "docker compose $DC ps"`. Admin token: in `oguz-testing.md`
  (gitignored) — persisted in the `pulse-prod_pulse-data` volume; **never `down -v` that volume.** TLS check: always
  `--resolve beyondkaira.com:443:161.97.172.146` (VPS DNS is stale). Rollback: runbook §5.
- `deploy/.env`, `*.db*`, `oguz-testing.md`, `web/pulse_secret.key` are gitignored — never commit.
- ⚠️ The working tree may carry an **uncommitted** `deploy/config/Caddyfile.prod` change + an untracked
  `Caddyfile.prod.bak-brier` — that's the operator's **separate `brier.<domain>` project** (a Next.js app on host:3000),
  NOT Pulse (noted in D-035). Leave it uncommitted; never fold it into a Pulse commit (commit by explicit path).
