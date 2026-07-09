# Changelog

All notable changes to Pulse are documented in this file.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
D-numbers reference the decision log at `agents/handoffs/decisions.md`.

---

## [Unreleased]

Nothing yet — next changes land here.

---

## [0.2.0] - 2026-07-09

**GA release** (declared D-065; tag chosen by the operator, D-066). Post-v0.1.0
changes from SESSION-02 through SESSION-08. Coverage ratchets and test-quality
improvements are noted as operator-visible because they gate the release of the
next versioned package.

### Licensing

- Repository licensed under **PolyForm Noncommercial 1.0.0** (root `LICENSE`,
  operator decision D-066): noncommercial use, modification, and sharing are
  free; commercial use requires a vendor license. The beacon SDK remains MIT
  (`sdk/beacon-js/LICENSE`). Product license-key mechanics documented in
  `docs/licensing.md`.

### Added

- Go server test coverage 59.4% → 73.2%; coverage floor ratcheted to 70; OpenAPI
  conformance harness made honest (`t.Fatalf` not `t.Skipf`) — 51/52 operations
  validated (D-059, D-060).
- e2e CI gate: A1 rule-firing, A2 beacon→QoE, A3 health-score transition,
  A4 `delivery_failure` via dead-URL channel (D-059, D-061).
- VD-04 closed: 500-stream Playwright render benchmark 668/459 ms on VPS vs
  2000 ms budget; 4 confirmed runs; CI result 426/196 ms (D-061).
- `csp-e2e` CI job: Playwright CSP byte-exact assertion against a real Caddy
  stack; bake clock started 2026-07-09 (D-061).
- CodeQL workflow: Go + JavaScript/TypeScript matrix; runs on push to main, pull
  requests, and weekly cron (D-062).
- `alert.QoEReader` seam: `rebuffer_ratio` and `error_rate` alert rules now query
  `rollup_qoe_1h` via ClickHouse, replacing the ingest-health heuristic proxy.
  Nil-reader / reader error safe: at most one WARN per tick, stream skipped (D-062).
- B7 per-source webhook secrets: `/webhook/ams/{name}` route with cross-source
  isolation — per-source secret used exclusively when configured (no SharedSecret
  fallback); `ams_sources.webhook_secret_enc` column + `applySchemaUpgrades`
  migration; webhook package coverage 94.7% (D-062).
- Slack notifications CI step via `${{ secrets.SLACK_WEBHOOK_URL }}` (D-062; the
  literal URL was intercepted before public push and rewritten to the secrets ref).
- Docs GA batch (D-063): `SECURITY.md`, upgrade/rollback + monitoring runbooks,
  docs truth pass (productionize, alerting, install, ARCHITECTURE §6); Helm
  parity batch (canonical image ref, ClickHouse auth Secret, backup CronJob,
  `optional: false` secret refs, NOTES.txt) — chart remains explicitly
  experimental.
- A10 load smoke recorded (D-064): 500 streams + 3,000 viewers, 15-minute soak —
  pulse 18.6 MiB peak, ClickHouse 610 MiB, API 9 ms avg, 0 errors; numbers in
  `docs/ARCHITECTURE.md` §4.
- CI-loud integration harness (D-065): `testutil.RequireClickHouseBin` — a
  missing ClickHouse test binary now fails CI loudly instead of silently
  skipping (kept as skip for local dev).

### Changed (GA punch list, D-064/D-065)

- pulse container CPU limit 0.5 → 1.0 vCPU (compose hardened overlay + Helm
  values): A10 measured 147%-of-a-core poll-boundary bursts CFS-throttled at
  0.5 (D-065).
- Health-degraded logging aggregated: one INFO line per sweep with count and up
  to 3 example stream IDs (was one line per degraded stream per tick — ~100
  lines/s at 500 degraded streams); per-stream detail moved to DEBUG (D-065).
- Go coverage floor ratcheted 66 → 70 (D-061) → 70.2 (GA achieved−3, D-065).
- Remaining floating base images digest-pinned: hardened-overlay mock-ams
  builder (`golang:1.25`), Helm busybox initContainer via `clickhouse.waitImage`
  (D-065).

### Fixed

- P0: rule→channel alert delivery never worked in prod since D-041 — the evaluator
  created an empty channel registry at startup and nothing populated it from the
  meta store. `syncRegistryFromStore()` now runs every tick (D-061).
- Mock-AMS pagination: off-by-one at ≥200 streams; non-deterministic Go map
  iteration causing 30–60 stream gaps in the union of pages across requests (D-061).
- Six D-028-class `t.Skipf("meta DDL not found")` hatches in the API conformance
  suite converted to `t.Fatalf` — a broken test mount now fails loudly instead
  of silently voiding ~90 tests (D-064).
- Upgrade runbook truth (first real exercise, D-065): resource-limit inspect
  targeted the image instead of the container; stale rollback-tag table;
  SQLite-WAL schema-verification gotcha documented.

### Removed

- logtail collector (`server/pkg/logtail`, `SourceLogTail`): AMS analytics log
  lines carry a log4j prefix causing `json.Unmarshal` to fail on every line (100%).
  The REST poller and webhook cover the same event data. The collector is removed
  entirely; compose stubs, Helm values, and serve wiring are all deleted (D-062).

---

## [0.1.0] - 2026-07-08

Tag `v0.1.0` at commit `1a701d6`.  
First production release. Rolled to `pulse-prod` (beyondkaira.com) 2026-07-08.

### Added

**Core features (Wave 1, 2026-06-11–15):**
- Live ops dashboard (F1): real-time viewers, streams, nodes; WebSocket push
  broadcasts `LiveOverview`; ≤10 s stream visibility; edge/origin viewer dedup.
- Historical analytics (F2): geo + device breakdown; 13-month rollups at 150 ms
  measured (budget 3 s); MaxMind GeoLite2-City.mmdb reader (no DB bundled).
- Core alerting (F5): Email (Free+), Slack/Telegram (Pro+), PagerDuty/Webhook
  (Business+); maintenance windows with range cron; `muted` suppression;
  `group_by` storm collapse; `node_down` fires on node absence.
- Docker Compose base stack: pulse (all-in-one binary) + ClickHouse; `expose:`
  ports (cluster-internal); SQLite meta store on `pulse-data` volume.

**Wave 2 features (2026-06-15–17, D-006..D-028):**
- QoE beacon SDK (F3): TypeScript, 3.52 KB gzip (budget 15 KB), 65 tests, MIT;
  `rebuffer_end` from `HlsAdapter`; `X-Pulse-Ingest-Token` round-trip to
  `/ingest/beacon`; Pro+ tier required; events geo/UA enriched (D-007, D-041).
- Ingest health monitoring (F4): health score 0–100 scale; 250 µs detection
  (budget 15 s); timeseries + `drop_events` in API (D-041).
- Usage/billing reports (F6): Business+; CSV + PDF; S3 export; ±1%
  reconciliation; 5-field cron; `peak_concurrency` from true windowed max
  (`rollup_concurrency_1d` `maxState`/`maxMerge`).
- Cluster fleet view (F7): auto-discovery ≤30 s (budget 2 min); real
  origin/edge roles; node version field populated.
- Prometheus `/metrics` (F8): 7 gauges (`pulse_live_viewers`,
  `pulse_live_streams`, `pulse_live_publishers`, `pulse_ingest_bitrate_kbps`,
  `pulse_node_cpu_pct{node}`, `pulse_node_mem_pct{node}`, `pulse_alerts_firing`);
  scrape token constant-time compare; Business+ gate (403 for Free/Pro);
  rate-limited 10 rps / burst 20 (D-028).
- Helm chart: `ghcr.io/aytekxr/ams-pulse`; lint and template verified (Wave 2).
- Onboarding wizard: 4-step first-run flow.

**Wave 3 features (2026-06-14–15):**
- Anomaly detection (F9): Welford baselines; σ=4.0; 0.259 false alarms/node-week
  (target < 1); `minSamples=30` warmup; hysteresis cooldown; epsilon floor;
  Enterprise tier.
- Synthetic probes (F10): HLS full — master + media playlists; `ttfb_ms` +
  `segment_ttfb_ms` stored separately; 4-worker pool; 60 s config refresh;
  90-day result TTL; Pro+ tier.

**Production hardening (2026-07-06–08, D-046..D-058):**
- Backup sidecar (`deploy/docker-compose.backup.yml`): 24 h cycles, first cycle
  immediately on start; 7-artifact retention per type; ClickHouse `BACKUP SQL`
  zip + SQLite file copy with magic-byte integrity verify; `deploy/runbooks/backup-restore.md`
  (D-050).
- Alert delivery retry: ≤3 retries with 500 ms × 2^n ±20% jitter backoff capped
  at 5 s; `delivery_failure` state recorded in `alert_history` on exhaustion
  with sanitised `{channel_id, error}` JSON (D-049).
- Secrets `_FILE` convention: `GetSecret` resolves `<VAR>_FILE` for
  `PULSE_SECRET_KEY`, `PULSE_WEBHOOK_SECRET`, `PULSE_AMS_LOGIN_PASSWORD`,
  `PULSE_METRICS_TOKEN`, `PULSE_AMS_AUTH_TOKEN`, and `PULSE_AMS_<NAME>_TOKEN`;
  missing file is a hard startup error (D-052).
- `alert_history` auto-prune: capped at 1000 rows per `rule_id` (`AlertHistoryDefaultKeep`)
  after every insert; O(excess) single DELETE (D-052).
- Resource limits in hardened overlay: pulse 512m/0.5 cpu, ClickHouse 2g/1.0,
  Caddy 256m/0.5, backup 256m/0.25 (D-052).
- `PULSE_SECRET_KEY` startup guard: server refuses to start with an actionable
  error if key is absent or < 16 bytes for non-`:memory:` DSNs (D-052).
- API token storage: HMAC-SHA256(hmacKey, rawToken) with `hash_alg='hmac-sha256'`
  when `PULSE_SECRET_KEY` is set; legacy `sha256` rows still authenticate
  (transparent upgrade) (D-052).
- Version stamping: `VERSION`/`COMMIT`/`BUILD_DATE` via Dockerfile `ARG` +
  `-ldflags`; `pulse version` output must not show `dev/unknown` in prod (D-058).
- Multi-arch release pipeline: amd64 + arm64; Trivy HIGH/CRITICAL scan;
  SBOM + provenance attached; cosign keyless signed (Rekor tlog index 2110636506)
  (D-058).
- Dependabot: gomod, npm (web + sdk), docker, docker-compose, actions; weekly
  grouped minor+patch (D-058).
- Branch protection on `main`: required CI contexts + 1 review; `enforce_admins=false`
  so owner direct-pushes (session workflow) still work (D-058).
- Webhook HMAC listener: `PULSE_WEBHOOK_ADDR=:8092` in hardened overlay;
  `PULSE_WEBHOOK_SECRET` required (fail-closed at startup if absent) (D-048).
- ClickHouse graceful drain on `Close()`: flushers drain their channels fully and
  flush the final partial batch before `conn.Close()`; `WaitGroup`-tracked;
  SIGTERM no longer drops queued events (D-051).

### Changed

- Production compose stack: 5-overlay (base + hardened + prod-tls + real-ams +
  backup); `PULSE_DOMAIN` required; public TLS via Let's Encrypt; Caddy is the
  sole TLS terminator; pulse has zero host port bindings (D-022, D-023, D-024,
  D-050).
- AMS REST paths corrected to real AMS v3 Enterprise wire format: proper endpoint
  paths, bps→kbps normalisation, `terminatedUnexpectedly` field, WebRTC
  single-track handling (D-025, D-030).
- QoE startup-time median: `quantilesStateIf` excludes heartbeat events (which
  carry `startup_ms=0`), correcting the diluted-toward-0 prod metric;
  migration `0004_qoe_startup_quantile_fix.sql` (D-042).
- AMS upstream in `Caddyfile.prod` now read from `{$AMS_UPSTREAM}` env var instead
  of hard-coded IP; compose default `${AMS_UPSTREAM:-161.97.172.146:5080}` (D-062).

### Fixed

- Live dashboard deadlock (AB→BA lock-order): `Discovery.poll` and
  `aggregator.EvictStale` held a state lock while calling into the event sink.
  Fix: collect events under the lock, emit after release (D-021).
- AMS web console login: provisioned accounts now MD5-hash the password
  client-side before submit, matching AMS's authentication model (D-036).
- QoE startup-quantile dilution: `quantilesStateIf` migration corrects the
  historical 0-dilution bug; prior values in `mv_qoe_1h` are immutable (D-042).
- Beacon ingest always returned 401 after D-052: ingest token lookup now uses
  `LookupToken` (HMAC-aware with legacy SHA-256 fallback) instead of the
  raw-hash path (D-056).
- `/beacon` Caddy route: `handle_path` strips the `/beacon` prefix before
  forwarding to the dedicated listener on `:8091`; without it the listener
  received `/beacon/ingest/beacon` and returned 404 (D-058).
- Beacon dedicated listener license gate was fail-open (`Config.License` was nil);
  Free tier now correctly returns 403 LICENSE_REQUIRED (D-058).

### Security

- HMAC-SHA256 webhook signature validation; empty secret always fails
  (fail-closed 401, not 404 to avoid name-existence leaks); constant-time
  `hmac.Equal` comparison (D-027, D-048).
- CORS allowlist: `PULSE_CORS_ALLOWED_ORIGINS`; beacon endpoint stays permissive
  (D-027).
- Rate limiting: `/metrics` 10 rps/burst 20 per IP; `/ingest/beacon` 100 rps/burst
  200 per token (D-027, D-028).
- CSP + Permissions-Policy headers via Caddy; `frame-ancestors 'none'`;
  `script-src 'self'` (no inline scripts) (D-027).
- AMS bearer-token cleartext WARN logged when `PULSE_AMS_URL` is `http://` and
  points to a remote host (D-027).
- 4-tier license enforcement (Free/Pro/Business/Enterprise); `/metrics` returns
  403 LICENSE_REQUIRED for non-Business tier (D-014 ruling + Wave 2).
- ClickHouse + meta store use `expose:` (cluster-internal only) in base compose;
  no external network binding without explicit host-port override (D-022).
- `PULSE_SECRET_KEY` fail-closed: server refuses start if key absent or < 16 bytes
  for non-`:memory:` DSNs (D-052).
- API tokens stored HMAC-SHA256 at rest; legacy SHA-256 rows authenticated via
  `LookupToken` fallback (D-052).
- Caution: rotating `PULSE_SECRET_KEY` invalidates `hmac-sha256` tokens (D-052).
