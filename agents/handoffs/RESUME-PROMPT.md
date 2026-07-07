# Pulse — Resume / handoff prompt (SINGLE source of truth)

> **This is the one handoff doc.** It supersedes the previous separate "next-session" prompt (merged 2026-06-29,
> D-037); don't recreate a second handoff file — update THIS one + `decisions.md` each session.
> Pulse = self-hosted analytics/QoE/alerting for Ant Media Server. Repo: `/home/aytek/repo/ams-pulse`
> on VPS `161.97.172.146`. Full decision log: `agents/handoffs/decisions.md` (D-001…D-051 + session notes, binding).
> Detailed phase plan: `agents/handoffs/PRODUCTION-READINESS.md`. AMS operator guide:
> `agents/handoffs/AMS-INTEGRATION.md`. Go-live runbook + rollback: `deploy/runbooks/real-ams-go-live.md`.
> Operator creds/keys (gitignored, never commit): `oguz-testing.md`.

---

## ▶ START HERE (next session — resume `pulse-prod-harden` MID-P2)

**Session 2026-07-07 result: P1 is DONE (all 4 orders implemented, adversarially verified, committed, CI-GREEN);
P2 was cut mid-flight by a usage limit.** Verify with `git log --oneline -5`:
- **D-048 `54aac48`** webhook path complete — expose 8092 + fail-closed `${PULSE_WEBHOOK_SECRET:?}` env +
  `TestEndToEndWebhookTCPListener` (real TCP, mutation-proven). Real secret IS set in gitignored `deploy/.env`.
- **D-049 `ff6510f`** alert-delivery retry (async per-channel, 1+3 attempts, 500ms·2^n cap 5s ±20% jitter,
  ctx-abortable, Stop() bounded) + `delivery_failure` alert_history rows. **Contract CR applied:** state enum +=
  `delivery_failure` (both places) + regenerated `web/src/lib/api/schema.d.ts`.
- **D-050 `cf053c8`** backups — `docker-compose.backup.yml` sidecar + CH backups disk + `pulse-backup.sh` (keep-7)
  + `deploy/runbooks/backup-restore.md`; restore PROVEN for both stores (incl. the stale-WAL doc bug verifiers caught).
- **D-051 `0400373`** ClickHouse graceful drain (WaitGroup drain → conn.Close last) **+ serve.go one-liner**
  `store.Start(context.Background())` — without it SIGTERM fast-exited flushers before Close and the drain was dead code.
Coverage 47.5% → **57.8%** total. Detail: `decisions.md` D-048…D-051. Do NOT re-do any of this.

**▶ FIRST ACTION — finish P2 (items 5–9 below) as D-052.** The `pulse-harden-p2-batch` workflow was STOPPED after
Wave 1. Its output sits **UNCOMMITTED + UNVERIFIED in the working tree** (`git status` — it COMPILES, tests NOT run):
- Item 5 secrets `_FILE` — **landed:** `server/internal/config/secrets.go` (GetSecret) + tests + `cmd/pulse/config.go`
  wiring + `deploy/docker-compose.secrets.yml` (opt-in overlay) + `deploy/secrets/` (gitignored) + `.env.example` docs.
- Item 6 token HMAC — **landed but VERIFY DEEPLY:** `hash_alg` present in `meta.go` + BOTH `0001_init.sql` copies +
  token_hmac tests (api+meta); `server/internal/api/server.go` was modified yet greps 0 for `hash_alg` — confirm where
  the api-side hashing actually happens. ⚠️ **The live prod admin token is a legacy SHA-256 row — back-compat is
  non-negotiable** (verify against a REAL old-schema DB: `git show 0400373:contracts/db/meta/0001_init.sql`).
- Item 7 `alert_history` pruning — **NOT STARTED** (no `PruneAlertHistory` anywhere; Wave 2 never ran). Work order:
  `PruneAlertHistory(ctx, ruleID, keep)` per-rule cap ~1000, called after insert on the evaluator recording path
  (incl. the D-049 delivery_failure recorder). Test: N>keep → COUNT==keep, newest-by-ts kept, other rules untouched.
- Items 8+9 — **landed:** resource limits (3 `resources:` blocks in hardened.yml + backup overlay); `validate()`
  rejects empty/<16-byte SecretKey (+ updated `TestValidate_AcceptsValidConfig`).
**Resume plan (new workflow — same-session resume is impossible):** (1) one author finishes item 7 on the existing
tree; (2) run the FIVE per-item adversarial verifiers — their full prompts are in the stopped script, READ IT:
`/home/aytek/.claude/projects/-home-aytek-repo-ams-pulse/4e5bc3e6-4ea6-4b48-8cd3-13c342dd454a/workflows/scripts/pulse-harden-p2-batch-wf_c0a5740c-995.js`
(tell agents the tree already contains Wave-1 edits); (3) fix loop; (4) ORCH gate = full `-race` repo-root mount +
coverage + web lint/tsc/vitest/build; (5) commit per scope by explicit path as D-052, push,
`gh run watch $(gh run list --branch main --workflow=ci.yml -L1 --json databaseId -q '.[0].databaseId') --exit-status`.

**Then, in order:**
1. **Tree-wide gofmt + CI gofmt gate** — 33 pre-existing files fail `gofmt -l` (go1.25.11 doc-comment reflow) and CI
   has NO gofmt step. One mechanical `style:` commit (in-container `gofmt -w` over server/) + a `gofmt -l` gate in the
   ci.yml server job. AFTER D-052 lands (avoid conflicting with in-flight edits). Until then, gate gofmt only on
   session-changed files.
2. **Prod rollout** of D-048…D-052 (image rebuild needed for D-049/D-051 Go code; new env/overlays for the rest):
   `sg docker -c "docker compose -p pulse-prod -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f deploy/docker-compose.backup.yml --env-file deploy/.env up -d --build"`
   then §8 smoke: `/healthz` ok (`--resolve beyondkaira.com:443:161.97.172.146`), signed POST `/webhook/ams` → 200,
   backup sidecar first run produces dated artifacts, `docker inspect` shows memory/cpu limits, admin token still works
   (legacy sha256 row!), logs clean. Staging-verify first (§8.7) if anything is in doubt.
3. **e2e backfill** — the full grounded plan (requested by the operator 2026-07-07) is in
   **`agents/handoffs/E2E-TEST-PLAN.md`**: A1 alert→history (ingest_bitrate_floor vs mock-ams fixed 2000 kbps),
   A3 health-score 100→50 transition (add mock-ams `set_bitrate`), A2 `qa/licensegen` + beacon → `rollup_qoe_1h` →
   `/qoe/summary` (120s bounded poll), B Playwright skeleton (route-mocked, `vite preview`, non-required CI job).
   Key corrections baked in: there is NO `/login` route (AuthGate renders in-place) and CSP comes from Caddy, not Go.
   Run as a `pulse-e2e-backfill` workflow.

### Operator-only actions (surface every session)
- **U3 — activate a Pro+ Pulse license.** Until then QoE/beacon data does NOT flow in prod; rebuffer/error-rate alerts
  run off the HealthScore proxy. (The e2e plan's mock license only covers CI.)
- **U4 — branch protection + a `v*` tag** (repo-admin; also retire the stale `ams-integration` ref).
- **U5 — open `beyondkaira.com` + `pulse.beyondkaira.com`**, confirm no CSP console errors.
- **NEW: point AMS at the webhook** — configure the AMS app(s) to POST lifecycle webhooks to
  `https://beyondkaira.com/webhook/ams` with the HMAC secret from `deploy/.env` (Pulse side is done; live after the
  prod rollout). ~~Set PULSE_WEBHOOK_SECRET~~ — DONE this session.

**Binding (unchanged, hard-won):** Go ONLY in Docker `golang:1.25`, **mount the repo ROOT** (`-v <repo>:/repo -w /repo/server
-e GOFLAGS=-buildvcs=false`) or ~90 api tests silently `t.Skip` → false green (D-028). Api integration tests need
`-tags integration` + `/tmp/clickhouse` (the unit `-race` gate skips them). **No false-green:** a "flake" that never resolves
with more waiting is a deterministic bug — read the code, don't bump the timeout (D-042); verify adversarially; reproduce CI
faithfully via `gh`. Commit by **explicit path** only, never `git add -A`. `Verify → Commit → Handoff` (§11); update THIS
file + `decisions.md` (new D-0NN) each session. AMS web login is RESOLVED (D-036). The `brier` project is DROPPED (D-046) —
`Caddyfile.prod` is now plain committable Pulse config.

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
- **Go suite green / coverage 57.8%** as of 2026-07-07 (full `-race` + coverage, **repo-root mount**, golang:1.25,
  after D-048…D-051; was 47.5% on 2026-06-28). NOTE: the working tree additionally carries the UNCOMMITTED P2
  Wave-1 changes (see ▶ START HERE) — it compiles, but re-run the suite before editing.
- **The prod image embeds the web UI** (multi-stage `deploy/docker/pulse.Dockerfile`: `npm ci && npm run build` →
  embedded in the Go binary), so a passing go-live build implies the web build passed.

---

## 1. PENDING USER ACTIONS (only the operator can do these — persist every session)

| # | Action | Why it's blocked / needed |
|---|---|---|
| U1 | ✅ **RESOLVED (D-034).** Self-hosted AMS on this VPS; per-app `remoteAllowedCIDR=0.0.0.0/0` so Pulse polls cleanly (200). No external allow-list dependency. | (was: 8/16 apps 403'd the VPS on test.antmedia.io). |
| U2 | ✅ **RESOLVED (D-039, 2026-06-30).** `ci` workflow is GREEN (de-flaked `TestQuery_QoeSummary_RealStartupP50`, 15s→90s poll); verified via `gh` (run 28429722100, 7/7 jobs). | — |
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
- ✅ **Silently-stubbed features — DONE (D-041):** alert test-fire now delivers (real `Send` via `buildChannelFromRow`,
  contract keys, `200 {accepted,message}`, sanitized error body); 3 license gates enforced (+`/qoe/ingest`, +TOCTOU
  mutex); standalone node card shows real identity (os/cores/java/version — AMS 3.x exposes **no** standalone cpu/mem via
  REST, a documented AMS limit, A9); WebRTC viewer QoE captured **and** surfaced as `viewer_*` on `/live/streams`.
  *(Still open: the `rebuffer_ratio`/`error_rate` alerts proxy from HealthScore, not real beacon data — needs actual
  beacon data → blocked on U3; tracked under QoE/beacon e2e in phase 4 (§4).)*
- ✅ **Webhook path — DONE (D-046 route + D-048 config/test).** Prod rollout + AMS-side webhook URL config pending.
- **Branch cleanup [P2]:** retire the stale `ams-integration` pointer; branch protection + `v*` tag (U4).
- **Reliability gaps — MOSTLY DONE 2026-07-07:** ✅ alert retry + delivery_failure (D-049); ✅ backups w/ verified
  restore (D-050); ✅ CH graceful drain (D-051); resource limits + `alert_history` pruning are in the in-flight P2
  batch (limits landed uncommitted; pruning NOT started — see ▶ START HERE).
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
1. ✅ **`pulse-p1-gaps`** — DONE (D-041): alert test-fire real delivery, 3 license gates enforced (+`/qoe/ingest`, +TOCTOU
   mutex), standalone node honest identity (AMS 3.x has no standalone cpu/mem via REST), WebRTC viewer QoE surfaced as
   `viewer_*`, `PULSE_ALLOWED_WS_ORIGINS` wired. Two adversarial-verify rounds.
2. **`pulse-test-backfill`** — TDD coverage to every level + enforced gate (3 sub-workflows: Go unit, web coverage
   gate, e2e+contract). See §6/§7.
3. **`pulse-prod-harden`** — B3 Docker secrets, alert retry, `alert_history` pruning, CH drain, resource limits,
   Trivy/SBOM, request-ID middleware. (License-gate **enforcement** moves up to `pulse-p1-gaps`/phase 1; this phase
   only deepens coverage of the gates per §6.)
4. **`pulse-feature-complete`** — QoE/beacon e2e (after U3), AMS version surfacing, anomaly expansion, native probes,
   white-label PDF, B7 (contract CR), SSO/OIDC, mobile SDKs, backup sidecar, Postgres backend.

---

## 4a. `pulse-p1-gaps` — ✅ EXECUTED & VERIFIED (D-041, 2026-06-30)

> **DONE.** All 4 items below were implemented TDD + closed through **two adversarial-verify rounds**. The verify rounds
> overturned several of the round-1 "green" results (false-positive tests): item 1 read internal keys not contract keys
> (`webhook_url`/`email_to`/`telegram_chat_id`) and leaked secrets in the 502 body; item 3's premise was wrong — real AMS
> 3.x `/rest/v2/system-status` has **no cpu/mem**, so it now reports honest node identity (os/cores/java/`GetVersion`)
> instead; item 2 missed the `/qoe/ingest` gate + had a TOCTOU race (now mutex-guarded); item 4 was dead data (now exposed
> as `viewer_*` on `/live/streams`). The original scouted plan is kept below for provenance. **Do not re-run this workflow.**


Scouted by a read-only fan-out (4 agents); file:line below were read, not guessed. **Treat the approach as the plan,
not verified code — each item is TDD red→green (write the failing test FIRST, watch it fail, implement, watch it pass)
and re-confirmed against the live tree during implementation.** Launch as the `pulse-p1-gaps` workflow: one
disjoint-scope author per item (scopes are non-overlapping → safe to run in parallel), then ORCH gates (full `-race`
repo-root mount, §8) + commits by explicit path, then re-confirm CI green via `gh run watch`.

1. **Alert test-fire actually delivers** · scope `server/internal/api`
   - Now: `handleTestAlertChannel` (`server.go:1234-1243`) returns 202 and **never calls `Send()`**; the ready helper
     `alert.TestFireChannel` (`alert/evaluator.go:652-680`) is unused; no `buildChannelFromRow` exists.
   - Fix: add `buildChannelFromRow(store,row)` (decrypt `ConfigEnc`, switch `row.Type` → `channels.New{Slack,Webhook,
     Telegram,PagerDuty,Email}Channel`) + call `alert.TestFireChannel` in the handler; 200 on delivery, 5xx on failure.
     Channel impls + `Send` signatures in `alert/channels/*.go`.
   - Red test (`api/wave2_test.go`): POST `/alerts/channels/{id}/test` at an `httptest` webhook sink → assert the sink
     RECEIVED a body (fails today). Verify: `go test ./internal/api/... -run TestHandleTestAlertChannel`.

2. **Enforce the 3 license gates** · scope `server/internal/api/server.go` + new `license_gates_test.go`
   - Now: `CheckDataAPI`/`CheckNodeLimit`/`CheckPrometheus` (`license.go:288/250/347`) are **defined but never called** →
     Free tier 200s on `/analytics/{audience,geo,devices}`+`/qoe/summary`, registers unlimited sources, scrapes `/metrics`.
   - Fix: `if err := s.lic.CheckX(); err != nil { writeError(403,"LICENSE_REQUIRED",…); return }` at the top of
     `handleAudienceAnalytics(908)/handleGeoAnalytics(941)/handleDeviceAnalytics(961)/handleQoeSummary(982)` [DataAPI];
     `handleCreateSource(1316)` count `ListAMSSources+1` vs `CheckNodeLimit`; `handleMetrics(672)` `CheckPrometheus`.
     Pattern: `handleReportUsage` (`reports_wave2.go:26-29`).
   - Red test (`api/license_gates_test.go`, pattern `v3b_guard_test.go`): Free-tier request that should 403 (200s today).

3. **Standalone node card (`SystemStats`)** · scope `server/internal/collector` (BE-01)
   - Now: `SystemStats()` (`amsclient/client.go:532-541`, GET `/rest/v2/system-status`) has **0 callers**; for a
     standalone AMS, `ClusterNodes()` 404→nil → 0 `node_stats` → `snap.Nodes` empty → `FleetNodes()`=`[]` → blank card.
   - Fix: in `restpoller.poll()` (`restpoller.go:123-153`), when `ClusterNodes` returns nil, call `SystemStats()` + a new
     `NormalizeSystemStats` (`normalize.go`) → emit a `node_stats` event. `aggregator.onNodeStats` + `query.FleetNodes`
     already consume it (CPU/Mem wired).
   - Red test (`restpoller/standalone_node_stats_test.go`): mock AMS 404 on `/cluster/nodes` + `{cpuUsage,…}` on
     `/system-status` → assert an `EventNodeStats` with `cpu_pct` is emitted.

4. **WebRTC viewer QoE (`EventWebRTCClientStats`)** · scope `collector/aggregator` + `domain/types.go` + `cmd/pulse`
   - Now: aggregator `OnServerEvent` switch (`aggregator.go:115-134`) has **no case** for `EventWebRTCClientStats` → every
     `webrtc_client_stats` event (`restpoller.go:185-195`, `NormalizeWebRTCStats` `normalize.go:163-190`) is dropped;
     `domain.LiveStream` (`types.go:279-299`) has no viewer-QoE fields.
   - Fix: add `ViewerRTTMS/ViewerJitterMS/ViewerLossPct` to `LiveStream` + a `case domain.EventWebRTCClientStats:
     a.onWebRTCClientStats(ev)` handler that writes rtt/jitter/loss into the stream snapshot. **`PULSE_ALLOWED_WS_ORIGINS`:**
     `api Config.AllowedWSOrigins` (`server.go:70`) is consumed but never set — add the field to `EnvConfig` (`config.go`)
     + wire in `serve.go` `apiCfg` (~295-300).
   - Red test (`aggregator/aggregator_test.go`): feed publish-start + `webrtc_client_stats` → assert snapshot has `ViewerRTTMS` etc.

Full per-item detail (current behavior, fix, red test, verify cmd) was captured by the scout — re-scout cheaply with the
same fan-out if stale. Cross-check scopes against `agents/manifest.yaml` single-writer map before launching.

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
7. **De-flake `TestDiscovery_NewNodeVisible`** (`internal/cluster/discovery_test.go:116`, observed D-041): 60ms (3×20ms)
   latency budget is too tight on a CPU-contended/2-vCPU runner (measured 68.8ms once under whole-suite `-race`; 3/3 pass
   unloaded). Loosen the budget like D-039 did — a real future CI-red risk.

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
| A2 | ✅ **VERIFIED GREEN (2026-06-30, D-039)** — `ci` all-green (run 28429722100) after de-flaking the QoE rollup test (15s→90s); readable via `gh` (U6 ✅), no longer an assumption. | Keep green: `gh run watch` after pushes. |
| A3 | **test-fire DELIVERS (D-041)** — real `Send()` via `buildChannelFromRow`, verified to a httptest sink + adversarially. Still open: no **retry** (transient SMTP/Slack fail = silent miss); no alert-fires→history **e2e**. | Add delivery retry (`pulse-prod-harden`) + alert-fires→history e2e (`pulse-test-backfill`). |
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
