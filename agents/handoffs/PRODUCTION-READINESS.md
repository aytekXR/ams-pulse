# Pulse — Production-Readiness & Test-Coverage Brief (next-session prompt)

> ⚠️ **SUPERSEDED (2026-07-08, D-057) by `agents/handoffs/ROADMAP.md`** — the session-divided
> production-readiness plan of record. Keep this file for provenance; do NOT execute from it
> (its coverage table, phase list and immediate steps are stale — Phases 1 & 3 are done,
> the coverage numbers moved substantially).

> Produced by the `pulse-completeness-and-test-audit` workflow (D-032, 2026-06-22). This is the paste-ready
> next-session prompt: it answers "what is needed to complete the app fully", mandates testing at EVERY
> level (TDD + coverage gate), and orchestrates each phase with Workflows. Repo:
> `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Branch: `ams-integration` (ahead of `main`).
> Production: `https://beyondkaira.com` → real `test.antmedia.io` (AMS 3.0.3 Enterprise). Go is NOT on PATH
> — run Go only inside Docker (`golang:1.25`), **mount the REPO ROOT** for `go test` (D-028 lesson). Use
> `sg docker -c "..."`. Honor Verify + Commit + Handoff. **Orchestrate with Workflows for each phase.**

## Headline verdict

- **Functionally:** substantially complete MVP across F1–F10, but several features are **stub/partial** and
  silently no-op in production (alert channel test-fire, rebuffer/error-rate alerts, standalone node card,
  3 unenforced license gates, QoE/beacon needs a Pro+ license to flow).
- **Testing:** the unit tests that exist are genuine (321 Go test fns, non-tautological), **but it is NOT
  "tested at every level"** — 3 critical packages report **0.0%** in a normal `go test ./...` run
  (`internal/query`, `internal/store/clickhouse`, `internal/config`), there are **0 Playwright tests**, **no
  response-body contract validation**, **no enforced coverage gate** (Go or web), and the e2e is a 4-assertion
  smoke test. See coverage table in §"Phase 2".
- **Production-readiness:** live + hardened, but real customer-blocking gaps remain (B3 Docker secrets,
  webhook unreachable via Caddy, per-app IP allow-list, no alert-delivery retry, no backups, no resource
  limits, SHA-256 token hashing).

---

## Immediate Steps (do these first, in order)

### Step A — golang:1.26 → 1.25  ✅ DONE (this session, D-032)
`go1.26` is unreleased; `deploy/docker-compose.{hardened,ci,override}.yml` referenced `golang:1.26` (would
fail `docker pull`, breaking mock-ams + CI). Fixed to `golang:1.25`. `grep -rn golang:1.26 deploy/ .github/`
is now empty. (Committed with this brief.)

### Step B — Merge `ams-integration` → `main`
Production runs `ams-integration`; CI runs `main` (behind), so "main CI green" does NOT cover D-029/D-030/D-031.
1. Full suite on `ams-integration`, **repo-root mount**, confirm 0 FAIL / 0 SKIP (esp. api package):
   ```
   sg docker -c 'docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 golang:1.25 go test -race -count=1 ./... 2>&1 | grep -E "FAIL|ok|---"'
   ```
2. `git checkout main && git merge ams-integration --no-ff` (then push).
3. Drop vestigial `AMS_LOGIN_EMAIL`/`AMS_LOGIN_PASSWORD` from `deploy/.env.example`; add a commented
   `PULSE_AMS_APPLICATIONS=` + `PULSE_INGEST_TARGET_BITRATE_KBPS=` example.

### Step C — Wire the webhook route in Caddy
`Caddyfile.prod` has no `/webhook/*` route → AMS webhook POSTs 404. Add **before** the catch-all `handle`,
in BOTH `deploy/config/Caddyfile` and `Caddyfile.prod`:
```
handle /webhook/* { reverse_proxy pulse:8092 { header_up X-Forwarded-For {remote_host}; header_up X-Real-IP {remote_host} } }
```
Confirm pulse `expose:` includes the webhook port; `restart caddy`.

---

## Phase 1 Workflow — `pulse-p1-gaps` (close P0 functional gaps that silently fail in prod)

Fan out to disjoint-scope agents (author → unit test (TDD) → `go test -race` green → ORCH gates → ORCH commits):
1. **Alert channel test-fire stub** (`server.go:~1234`) — handler returns 202 but never calls `channel.Send()`.
   Resolve the channel and Send a synthetic payload. Test: `Send()` is invoked, returns 202.
2. **Standalone node card** — when `ClusterNodes` returns `(nil,nil)` (404 standalone), call
   `amsclient.SystemStats()` (`client.go:~532`, already implemented, currently never called) and emit
   `EventNodeStats` so Fleet/CPU·RAM aren't blank. Test: `TestRestPoller_StandaloneNode_EmitsNodeStats`.
3. **`EventWebRTCClientStats` aggregator case** — restpoller emits it; `aggregator.go` switch drops it →
   viewer RTT/jitter/loss never surface. Decide field ownership (viewer-side vs ingest-side), add the case.
4. **Wire `PULSE_ALLOWED_WS_ORIGINS`** through `config.go` → `serve.go` `apiCfg.AllowedWSOrigins` (currently
   Host-header fallback only). Test: parsing + applied.

---

## Phase 2 Workflow — `pulse-test-backfill` (TEST AT EVERY LEVEL — TDD + ENFORCED COVERAGE GATE)

**MANDATE (binding): TDD — write the test, watch it fail, implement, watch it pass. Add a clean-coverage gate
so CI cannot regress, and the three 0.0% packages reach ≥60%.** Current Go coverage (no `-tags integration`):

```
cmd/pulse 1.2% | alert 72.1 | alert/channels 56.8 | anomaly 76.1 | api 52.2 | cluster 89.0 |
collector 64.1 | aggregator 69.4 | beacon 68.8 | ingest 85.1 | kafka 72.7 | logtail 37.5 |
restpoller 72.4 | sessions 81.1 | webhook 58.1 | config 0.0 (NO TEST FILE) | license 36.9 |
prober 61.9 | query 0.0 (integration-only) | reports 58.8 | store/clickhouse 0.0 (integration-only) |
store/meta 29.7 | pkg/amsclient 75.9
```
Web: 12 vitest suites, **no --coverage / no threshold**. SDK beacon-js: best-covered (5 suites, 15KB gate).
E2E: 4 assertions. Playwright: 0. Response-body↔OpenAPI conformance: none.

### Sub-workflow A — Go unit coverage (the MISSING functional scenarios that MUST have tests)
- **`internal/config` (new `config_test.go`, 0% today):** valid env load; missing/short `PULSE_SECRET_KEY`
  → `validate()` error; DSN format; env-overrides-YAML.
- **`internal/query` (0% today — use a mock ClickHouse `Conn`):** `LiveOverview` (empty + with-streams),
  `LiveStreams` pagination, `AudienceAnalytics` retention-cap, `IngestTimeseries` empty→non-nil, `FleetNodes` mapping.
- **`internal/store/clickhouse` (0% today — extract batcher behind an interface):** drop-on-full counter,
  flush-on-tick, migrations `splitStatements`/`stripLeadingComments` (pure fns).
- **`internal/store/meta` (29.7%→70%):** token create/revoke, tenant CRUD, concurrent insert (in-memory sqlite).
- **`internal/license` (36.9%):** expiry enforced, grace period, `CheckNodeLimit`/`CheckDataAPI`/`CheckPrometheus`
  Free-blocked (these gates are ALSO unenforced in prod — see Phase 3).
- **`internal/alert/channels` (56.8%):** Telegram httptest (missing), webhook timeout, Slack 429-retry.
- **Multi-app restpoller (critical — D-029 fix only has unit-level aggregator coverage):**
  `TestRestPoller_MultiApp_NoFalseEnd` — two-app httptest, two poll cycles, assert no false `publish_end`.

### Sub-workflow B — Web coverage gate
- `vitest run --coverage`; thresholds (lines ≥60, branches ≥55) in `vite.config.ts`; **fail the CI web job**
  below threshold. Add `msw` for API interception; priority: LiveDashboard (WS→row), AlertRulesPage (CRUD),
  IngestPage (health/timeseries render).

### Sub-workflow C — E2E + contract
- **Response-body OpenAPI conformance** — `TestAPI_AllEndpoints_ResponseConformsToSpec`: httptest server,
  validate each route's response body against `pulse-api.yaml` (kin-openapi `ValidateResponse`).
- **Extend `e2e.yml`:** (e) alert rule fires → appears in `/alerts/history`; (f) beacon POST → `/qoe/summary`
  field present (mock Pro+ license in CI); (g) ingest-degrade → `health_score` drop sequence.
- **Playwright skeleton** (`web/e2e/`): unauth→/login redirect; CSP header present; 500-row table renders
  ≤25 DOM rows (VD-04). Non-required CI job to start.

**Gate command (Phase 2 done when only the justified integration-only packages show 0.0%):**
```
sg docker -c 'docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 golang:1.25 sh -c "go test -race -coverprofile=cover.out -covermode=atomic ./... && go tool cover -func=cover.out | grep -E \"^total|0.0%\""'
```

---

## Phase 3 Workflow — `pulse-prod-harden` (security/monetization/ops)
1. **Enforce the 3 missing license gates:** `CheckDataAPI` on `handleAudienceAnalytics/Geo/Device`;
   `CheckNodeLimit` on node registration; `CheckPrometheus` in `handleMetrics`. + tier-boundary tests.
2. **B3 Docker secrets** — compose `secrets:` + `_FILE` env reading in `config.go` for
   `PULSE_SECRET_KEY/CLICKHOUSE_PASSWORD/PULSE_AMS_LOGIN_PASSWORD/PULSE_WEBHOOK_SECRET`.
3. **Alert delivery retry** — 3× exp backoff in `evaluator.deliver()`; mark `delivery_failure` in history.
4. **`alert_history` pruning** — cap rows per `rule_id` (e.g. last 1000) / nightly vacuum.
5. **ClickHouse graceful drain** — replace `close(done)+100ms sleep` with a WaitGroup drain (no event loss).
6. **Container resource limits** in `hardened.yml` (pulse 512m/0.5cpu, CH 1g/1cpu) — avoid simultaneous OOM.
7. **Trivy scan + SBOM** in `release.yml`; **chi `middleware.RequestID`** + `slog` req-id correlation.

---

## Phase 4 Workflow — `pulse-feature-complete` (PRD gaps + scale)
QoE/beacon end-to-end (Pro+ license → SDK in a real player → real `rebuffer_ratio`/`error_rate` from
`rollup_qoe_1h`, replacing the `wave2.go` health-score proxy); AMS version surfacing; `speed_read_kbps`→
`speed_ratio` (contract CR); anomaly expansion (viewer-side metrics, rebuffer); native WebRTC/RTMP/DASH probes;
Enterprise white-label PDF logo; **B7** per-source webhook secret (contract CR); SSO/OIDC; mobile SDKs;
automated backup sidecar (ClickHouse `BACKUP` + sqlite `.backup` + S3 push); Postgres meta backend (HA).

---

## Real-AMS IP allow-list follow-up (operator action)
8/16 apps on `test.antmedia.io` 403 the VPS IP `161.97.172.146` ("Not allowed IP"); the user's 4 active
streams are in those apps. **Operator:** add `161.97.172.146` to each blocked app's REST CIDR allow-list in
the AMS console, then tell the agent which app(s) → agent adds them to `PULSE_AMS_APPLICATIONS` + redeploys.
**Agent (technical mitigation):** in `amsclient`, distinguish permanent-403 from transient-401/403, back off
a blocked app ~5 min, and surface blocked apps in `/healthz` (avoids the 403/re-login storm).

---

## Verify + Commit + Handoff (binding, every phase)
- **Verify:** independent adversarial re-run; default to "refuted" until reproduced; **repo-root mount** or the
  api suite silently skips (D-028). A verify harness that skips == no verify.
- **Commit:** explicit path only, never `git add -A`; parallel agents author-only, ORCH commits centrally.
  Message `<scope> D-0NN: <summary>`. Push when directed.
- **Handoff:** update `RESUME-PROMPT.md` + `decisions.md` (new D-0NN) every session.

## Hard rules (CLAUDE.md)
Go only in Docker `golang:1.25` (repo-root mount; `CGO_ENABLED=0` build, `=1` for `-race`). AMS wire formats
ONLY in `server/pkg/amsclient` + `server/internal/collector`. Contracts frozen (D-004) — changes via ORCH CR.
Never commit secrets / never `git add -A` / never run a server or ClickHouse in the foreground in an agent.
