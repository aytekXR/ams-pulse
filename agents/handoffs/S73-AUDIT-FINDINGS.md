# S73 subsystem audit — confirmed findings ledger (D-135, 2026-07-17)

> Produced by the SESSION-73 adversarial audit workflow (5 finder lenses over the still-UN-swept
> subsystems + refute-by-default verifiers, 17 agents). **8 CONFIRMED (3 HIGH, 5 MEDIUM, 0 LOW), 4 refuted.**
> Scope (deduplicated against S48 §2.30 — collector/amsclient/reports/cluster/clickhouse — and S62 §2.31 —
> alert/license/prober/anomaly/api): **`store/meta`, `query`, `config`, `cmd/pulse`, and the never-audited
> `web/` frontend.** These are agent findings — **each MUST be re-verified against the code before building**
> (take the verified CORE, not the audit's literal scope). Work in coherent clusters, one scope per PR.
> Mark each ✅ DONE / ⏸️ DEFERRED (D-NNN) as it ships.

> ⚠ **[5] (QoEForStream tenant) — the finder's fix sketch is WRONG:** it claims the alert evaluator already
> has a per-stream tenant context, but `domain.AlertScope` / `meta.AlertRuleRow` / `domain.LiveStream` have
> NO Tenant field. The real fix threads Tenant through the aggregator→AlertScope/LiveStream pipeline first,
> then the QoEReader signature — a wider change. Re-scope at build. Both [1] and [5] only bite in
> multi-tenant deployments (Business+ tier); the primary self-hosted single-AMS model uses one tenant.

---

## [1] HIGH — IngestTimeseries has no tenant filter — cross-tenant ingest-metrics leak
- **loc:** `server/internal/query/query.go:935` (params) + `:975-996` (WHERE) + handler `server/internal/api/server.go:1378`  ·  **lens:** query-plane  ·  status: ⏳ TODO
- **mechanism:** `IngestTimeseriesParams` has no `Tenant` field and `IngestTimeseries()` never appends `AND tenant = ?`. Every sibling ClickHouse query in this file — `AudienceAnalytics` (:268), `GeoBreakdown` (:586), `DeviceBreakdown` (:676), `QoeSummary` (:794) — applies a tenant filter; this is the sole exception. The handler `handleIngestHealth` builds the params with only StreamID/App/NodeID/From/To/BucketSeconds — no tenant — so there is no call-site workaround. (Same class as the S48/D-110 `AudienceAnalytics` cross-tenant leak.)
- **scenario:** Two tenants both publish stream `broadcast` to app `live`. Tenant A calls `GET /qoe/ingest?app=live&stream=broadcast`; ClickHouse returns bitrate/fps/packet-loss averages blended across both tenants' `server_events` rows.
- **fix sketch:** Add `Tenant string` to `IngestTimeseriesParams`; inside `IngestTimeseries()` add `if p.Tenant != "" { where += " AND tenant = ?"; args = append(args, p.Tenant) }` (mirror :268); pass `q.Get("tenant")` in the handler at server.go:1378. Mutation-test: a 2-tenant fixture where tenant A's query must exclude tenant B's rows.
- **verifier:** CONFIRMED high. `APIToken` (meta.go:551) carries no tenant-scoping field, so auth middleware provides no implicit isolation; the leak is real for any deployment where distinct tenants share an (app, stream_id).

## [2] HIGH — server.Stop() never calls apiServer.Stop() — HTTP not drained on SIGTERM, goroutines leak
- **loc:** `server/cmd/pulse/serve.go:709` (server.Stop)  ·  **lens:** config-startup  ·  status: ⏳ TODO
- **mechanism:** `api.Server.Stop()` (api/server.go:390) calls `httpSrv.Shutdown` (10 s timeout) and stops the WS push-loop + two rate-limiter eviction goroutines started in `Start()`. `server.Stop()` calls reportScheduler/alertEval/beaconServer/meta/store Stop/Close — but **never `s.apiServer.Stop()`** (repo-wide grep: zero callers). The HTTP goroutine (api/server.go:379-383) is not context-aware; it only stops on Shutdown/Close.
- **scenario:** Pod gets SIGTERM during a k8s rolling update → `signal.NotifyContext` cancels ctx → `server.Stop()` runs → HTTP `Shutdown` never invoked → in-flight requests (write timeout 60 s) killed abruptly; WS push-loop + 2 eviction goroutines leak until process teardown.
- **fix sketch:** Add `s.apiServer.Stop()` inside `server.Stop()` (serve.go ~726, before `s.store.Close()`). `api.Server.Stop()` already has the correct 10 s timeout. Test: a Start→Stop lifecycle asserting Shutdown is called (or the goroutines exit).
- **verifier:** CONFIRMED high. Traced end-to-end (main.go:148 → serve.go:709 → api/server.go:390); concrete k8s rolling-update impact.

## [3] HIGH — PULSE_ANONYMIZE_IP=1 (Docker boolean idiom) silently leaves viewer IPs un-anonymized
- **loc:** `server/cmd/pulse/config.go:302` (AnonymizeIP) + `:248` (WebhookRequireTimestamp)  ·  **lens:** config-startup  ·  status: ⏳ TODO
- **mechanism:** Production `loadEnvConfig()` (main.go:110) does `cfg.AnonymizeIP = os.Getenv("PULSE_ANONYMIZE_IP") == "true"` — exact case-sensitive match. `PULSE_ANONYMIZE_IP=1` (or `True`/`TRUE`) leaves it false silently. The broader guard (`v=="1" || strings.EqualFold(v,"true")`) exists only in `internal/config/config.go:338`, which is **dead code** (config.Load never called in production — HOOK(BE-02), grep-confirmed). Same narrow compare at :248 for WebhookRequireTimestamp.
- **scenario:** Operator sets `PULSE_ANONYMIZE_IP=1` in a Docker `.env` for GDPR/KVKK compliance. `== "true"` is false → AnonymizeIP false → all beacon viewer IPs stored + geo-resolved in the clear. No error/warning; the privacy control is silently off.
- **fix sketch:** A shared `envBool(name) bool` helper (`v=="1" || strings.EqualFold(v,"true")`) applied at :302 and :248 (and any sibling boolean env). Mutation-test: `PULSE_ANONYMIZE_IP=1` → AnonymizeIP true. Consider deleting the dead `internal/config` path or reconciling it.
- **verifier:** CONFIRMED high. `PULSE_ANONYMIZE_IP=1` is the idiomatic Docker/.env boolean; a compliance control silently inactive is high-impact.

## [4] MEDIUM — PruneAlertHistory: non-transactional COUNT+DELETE race over-deletes on Postgres
- **loc:** `server/internal/store/meta/meta.go:1047` (COUNT) + `:1077` (DELETE)  ·  **lens:** meta-store-sql  ·  status: ⏳ TODO
- **mechanism:** `PruneAlertHistory` runs `SELECT COUNT(*)` then a separate `DELETE ... WHERE id IN (SELECT id ... LIMIT excess)` as two non-transactional statements, called unsynchronised after every `CreateAlertHistory` INSERT. On Postgres (MaxOpenConns=10) two concurrent workers each compute a stale `excess` in Go; under READ COMMITTED the DELETE subquery re-evaluates against a newer snapshot but keeps the stale `LIMIT excess`, so together they delete below the cap. SQLite (MaxOpenConns=1) is serialised and unaffected.
- **scenario:** 1001 rows, keep=1000. Worker A COUNT=1001 (excess 1); an INSERT lands (1002); Worker B COUNT=1002 (excess 2). A deletes 1 (→1001), B deletes 2 (→999). A kept row is permanently lost; deficit accumulates under sustained concurrent firing.
- **fix sketch:** Single self-contained statement: `DELETE FROM alert_history WHERE rule_id=? AND id NOT IN (SELECT id FROM alert_history WHERE rule_id=? ORDER BY ts DESC, id DESC LIMIT ?)` with `keep` as the bound. No intermediate snapshot gap. Test: concurrent inserts+prunes leave exactly `keep` rows (or a deterministic single-statement unit test).
- **verifier:** CONFIRMED medium. Permanent alert-history loss, Postgres-only, bounded per race but accumulates.

## [5] MEDIUM — QoEForStream omits tenant → alert evaluator reads cross-tenant QoE
- **loc:** `server/internal/query/query.go:898` (QoEForStream) → `QoeSummary` :794; caller `internal/alert/wave2.go:93`  ·  **lens:** query-plane  ·  status: ⏳ TODO
- **mechanism:** `QoEForStream(streamID, app)` builds `QoeParams` with `Tenant` empty; `QoeSummary` only adds `AND tenant = ?` when Tenant != "", so `rollup_qoe_1h` (multi-tenant, ORDER BY includes tenant) aggregates across every tenant sharing that (app, stream_id). The alert evaluator (wave2.go:93) uses the blended ratio to decide QoE alerts.
- **scenario:** Tenants A+B share app=`live`, stream=`broadcast`. B's stream rebuffers badly (0.8); A's rule threshold 0.05 fires a false alert on A from the blend — or symmetrically the blend suppresses a real alert.
- **fix sketch:** ⚠ WIDER than the finder claimed (see header warning): `AlertScope`/`AlertRuleRow`/`LiveStream` have no Tenant field. Thread Tenant from the aggregator into `AlertScope`/`LiveStream`, then add a `tenant` param to `QoEForStream` + `QoEReader` interface, then the `AND tenant=?`. Re-scope at build; may split into a "tenant-in-live-pipeline" prerequisite. Test: 2-tenant QoE fixture, evaluator only sees its own tenant.
- **verifier:** CONFIRMED, downgraded high→medium (only multi-tenant deployments with shared app/stream; primary model is single-tenant). Finder's fix sketch materially wrong.

## [6] MEDIUM — `pulse diag` / `checkAMS` print the raw AMS URL without credential redaction
- **loc:** `server/cmd/pulse/main.go:253` (runDiag) + `server/cmd/pulse/migrate.go:131` (checkAMS)  ·  **lens:** config-startup  ·  status: ⏳ TODO
- **mechanism:** `runServe()` (main.go:119-122) deliberately parses `cfg.AMSBaseURL` and calls `.Redacted()` before logging (B10 comment). `runDiag()` (:253) and `checkAMS()` (migrate.go:131) print `cfg.AMSBaseURL` / `baseURL` raw via `fmt.Printf`.
- **scenario:** Operator embeds creds in `PULSE_AMS_URL` (`http://admin:s3cr3t@ams.internal:5080`). `pulse diag` prints `AMS URL: http://admin:s3cr3t@ams.internal:5080` to stdout → captured by log aggregation → AMS password leaked.
- **fix sketch:** Apply the `url.Parse(...).Redacted()` pattern (already in runServe) at both sites. Test: a URL with userinfo → the printed string contains `xxxxx` / no password.
- **verifier:** CONFIRMED medium. Both sites verified raw; runServe's B10 redaction proves the risk is real, not hypothetical.

## [7] MEDIUM — Admin bearer token in the WebSocket upgrade URL query param → leaked to proxy access logs
- **loc:** `web/src/api/client.ts:570` (LiveSocket.connect `?token=`) + `deploy/config/Caddyfile.prod:82-86` (unfiltered `format json`)  ·  **lens:** web-frontend  ·  status: ⏳ TODO
- **mechanism:** `LiveSocket.connect()` builds `/live/ws?token=<bearer>` (browsers can't set an Authorization header on a WS handshake — deliberate). The Go `loggingMiddleware` logs only `r.URL.Path` (server.go:799) — but Caddy's default `format json` records the full request URI incl. query string, so every WS upgrade writes the raw token to Caddy's access log / docker logs / any SIEM.
- **scenario:** Operator opens the Live dashboard → WS upgrade to `/api/v1/live/ws?token=plt_ADMIN` → Caddy logs `{"request":{"uri":"...?token=plt_ADMIN"}}` → attacker with log-read replays the long-lived token against `/admin/*` and `/alerts/*`.
- **fix sketch:** Preferred (no Caddyfile change — Caddyfile.prod is do-not-commit, D-082/D-096): a short-lived single-use WS ticket — `POST /auth/ws-ticket` → `{ticket, expires_in:30}`, connect with `?ticket=`, server validates+discards on upgrade. (Alternative: authenticate via the first WS binary frame; or a Caddy `log_format` filter — but that touches the operator-managed Caddyfile.) Server + web change → web CI (schema.d.ts regen if OpenAPI changes). Test: ticket is single-use + expires; upgrade with a spent/expired ticket → 401.
- **verifier:** CONFIRMED medium (requires log-read access, but API tokens are long-lived + broad-scope). The `?token=` is intentional for WS; the leak is at the proxy layer the Go path can't mitigate.

## [8] MEDIUM — deleteSource / deleteToken (and createApiToken / createIngestToken) silently discard API errors
- **loc:** `web/src/features/settings/SettingsPage.tsx:132` (deleteSource) + `:169` (deleteToken); call sites `:303/:406/:502`; also `createApiToken` :139 / `createIngestToken` :160  ·  **lens:** web-frontend  ·  status: ⏳ TODO
- **mechanism:** These async handlers `await` the API call with no try/catch; the `toast()` + `loadAll()` sit after the await and are skipped on rejection. Call sites use `() => void handler(id)`, discarding the rejected Promise → no error UI at all (invisible unless DevTools open). The correct try/catch+toast pattern already exists in the same file (`saveLicense` :180) and in `ProbesPage.tsx`.
- **scenario:** User clicks Remove on a source → `deleteSource` → API 500 (ClickHouse down) → async throws → `void` swallows it → no toast, list neither refreshed nor updated → user re-clicks, issuing repeated DELETEs and masking the failure.
- **fix sketch:** Wrap each handler in `try/catch { toast(err instanceof ApiError ? err.message : 'Operation failed', 'error') }` (mirror `saveLicense`). Include the two create* handlers the verifier flagged. Web-only → web CI (vitest/component test asserting a toast on a mocked 500).
- **verifier:** CONFIRMED medium. Correctness/UX (silent failure, repeated requests); not a direct vuln. Two extra handlers share the gap.

---

## Refuted (do NOT build — recorded for provenance)
- ~~[meta-store-sql] applySchemaUpgrades rows.Err() unchecked~~ — REFUTED: real hygiene omission but unreachable (PRAGMA over ~6 in-memory rows can't be ctx-cancelled mid-scan; migrate uses 120 s timeout; table blocks are `CREATE TABLE IF NOT EXISTS` idempotent). Style, not a defect.
- ~~[meta-store-crypto] license.license_key stored plaintext~~ — REFUTED: `UpsertLicense` is dead production code (only caller is `meta_pg_integration_test.go`); the production license lifecycle is in-memory (`license.New` / `Refresh`); the column is always NULL in prod. No encryption-at-rest violation at runtime.
- ~~[meta-store-crypto] hasExplicitKey=false → HMAC token protection absent~~ — REFUTED: design inconsistency is real (plain SHA-256 when no PULSE_SECRET_KEY) but tokens are 192-bit crypto/rand; SHA-256 and HMAC-SHA256 of a 192-bit random are equally uncrackable — no practical exploit.
- ~~[web-frontend] bearer token in localStorage~~ — REFUTED: accurate description of localStorage use, but the attack needs a supply-chain/XSS prerequisite not present; no XSS sink found; React JSX auto-escapes; accepted trade-off for a self-hosted operator tool. (The concrete secondary issue — token in the WS URL — is filed separately as [7].)
