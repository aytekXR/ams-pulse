# Changelog

All notable changes to Pulse are documented in this file.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
D-numbers reference the decision log at `agents/handoffs/decisions.md`.

---

## [Unreleased]

### Security

- **Opt-in webhook replay protection (D-123).** The AMS webhook endpoint authenticated
  each request's HMAC signature but had no freshness check, so a captured, validly-signed
  webhook could be replayed indefinitely (duplicate stream-start/stop/recording events).
  Setting `PULSE_WEBHOOK_REQUIRE_TIMESTAMP=true` now requires each request to carry a fresh
  `X-Ams-Timestamp` header (within ±`PULSE_WEBHOOK_TIMESTAMP_SKEW`, default 5 min) and to
  sign the timestamp-bound payload, so a captured request can no longer be replayed once it
  ages past the window. **Default off** — the signing contract is unchanged until you enable
  it and update your signing proxy to send the timestamp (see `docs/AMS-INTEGRATION.md` §4.7).
  (Found by the S48 subsystem audit, finding [8].)
- **Alert notification channels hardened (D-125).** Four fixes from the S62 subsystem audit:
  (1) **Email STARTTLS now fails closed** — if `STARTTLS=true` and the TLS upgrade fails, the
  send aborts instead of silently continuing on a plaintext socket (which had sent the message
  and any SMTP AUTH credentials in cleartext). **Behavior change:** `STARTTLS=true` is now
  *mandatory* TLS, not opportunistic — set `STARTTLS=false` if you intend a plaintext relay.
  (2) **Telegram bot token no longer leaks into logs** — transport errors from the Bot API call
  embedded the token-bearing URL; the token is now redacted from returned/logged errors.
  (3) **SMTP Subject header injection closed** — a publisher-controlled stream name in the alert
  title can no longer inject email headers via CR/LF. (4) The Telegram dashboard link is now
  attribute-escaped (defense-in-depth).

### Fixed

- **Report-schedule and tenant update/read endpoints no longer misreport a
  transient database error, or crash on a concurrent delete (D-126).** Three
  robustness fixes from the S62 subsystem audit, all in the admin/reports handlers:
  (1) after a successful schedule/tenant update the handler re-read the row and
  dereferenced the result without checking it — a concurrent delete (or a transient
  store error) between the write and the re-read could nil-dereference and return a
  bare 500 for an operation that actually succeeded; the schedule path now renders
  the row already in hand (no re-read) and the tenant path guards the re-read.
  (2) A store error while loading a schedule or tenant was reported to clients as a
  definitive `404 Not Found` instead of `500`, so an SDK or UI cache could
  permanently mark an existing resource as deleted; genuine errors now return 500 and
  only a truly missing row returns 404. (Found by the S62 subsystem audit, findings
  [5]/[6]/[19].)
- **The beacon ingest endpoint returns the right error when a client upload is cut
  off (D-120).** A dropped connection partway through a large-but-in-limit upload
  was misreported as `413 Request Entity Too Large` instead of `400` (read error),
  because the code guessed "too large" from the byte count rather than the actual
  error. It now distinguishes a genuine size-limit breach from a broken connection
  by error type, so clients get an accurate status. (Found by the S48 subsystem
  audit, finding [14].)
- **Cluster fleet metrics no longer double-count a node when the AMS cluster API
  returns a duplicate entry (D-119).** If two node records resolved to the same
  identity (for example both missing their node ID and IP), each poll emitted two
  `node_stats` events for that one node — doubling its CPU/memory/network figures in
  ClickHouse and showing a phantom extra node on the fleet page. Each node is now
  counted once per poll. (Found by the S48 subsystem audit, finding [16].)
- **Player-beacon (QoE) events now save atomically, with accurate ingest metrics
  (D-118).** The ClickHouse writer opened a separate insert for every beacon item
  in a flush, so a transient failure partway through committed the earlier items
  while the writer reported the whole flush as failed — the "inserted" count
  under-reported reality and the rest of the batch was dropped without a retry.
  Each flush is now a single atomic insert (matching the server-event and
  viewer-session writers): on failure nothing is written, so the metrics always
  match what was stored. (Found by the S48 subsystem audit, finding [13].)
- **Usage reports now disclose the egress method they actually used (D-117).**
  The report-level "Egress method" line on billing statements (CSV/PDF) and the
  `egress_method` API field were hardcoded to `bitrate_x_watch_time` even when the
  figures were driven by AMS REST byte counters — a false methodology disclosure
  (PRD F6). The report now reflects the method actually used: `bitrate_x_watch_time`,
  `ams_rest_stats_byte_counter`, or `mixed` when a single report blends both across
  its streams. Per-row disclosure was already correct and is unchanged. (Found by the
  S48 subsystem audit, finding [10].)
- **The REST poller no longer leaks memory for idle streams that come and go
  (D-116).** The poller tracked the last-seen status of every stream but only
  cleaned up entries for streams that had been actively broadcasting; an
  idle/created stream that appeared and later disappeared from Ant Media left a
  permanent entry, so the tracking map grew unbounded over long uptimes. All
  disappeared streams are now cleaned up (a "stream ended" event is still emitted
  only for ones that were broadcasting). (Found by the S48 subsystem audit,
  finding [9].)
- **A publisher whose ingest stats arrive without a timestamp is no longer
  falsely dropped (D-115).** An ingest health event with a zero timestamp was
  recorded as last-seen in 1970, so the next staleness sweep immediately evicted
  the publisher with a spurious "source gone" warning and hid its real health. The
  guard now checks the timestamp field directly. (Found by the S48 subsystem
  audit, finding [7].)
- **Origin viewer counts recover after an edge node goes down (D-114).** In an
  origin+edge cluster, Pulse skips the origin's viewer count for a stream while an
  edge is serving it (the origin's number already includes edge viewers). But a
  crashed edge was marked "down" without clearing its last-known active-stream
  count, so it was treated as still-serving forever — permanently suppressing
  origin viewer totals to 0 even though the origin was the only node left serving.
  Downed edges are now excluded from that check. (Found by the S48 subsystem audit,
  finding [5] — the last of the six high-severity findings.)
- **Scheduled monthly reports cover the correct calendar month (D-113).** The
  previous-month statement used an inclusive end bound set to the first day of the
  *current* month, so that day's usage rolled into the prior month's report
  (over-counting viewer-minutes, egress and peak concurrency, and mislabelling the
  period end). The range is now the first-to-last day of the previous month.
- **Report schedule cron times are interpreted in UTC (D-113).** The next-run
  calculation read the cron hour/day in the server's local timezone while the rest
  of the reporting pipeline is UTC, so on a non-UTC-configured host a schedule like
  "0 6 1 * *" fired at 06:00 local instead of 06:00 UTC. The cron seed is now
  normalized to UTC. (No effect on UTC-configured servers.) (Both found by the S48
  subsystem audit, findings [4] and [15].)
- **WebRTC QoE stats are collected for streams whose id contains a URL-special
  character (D-112).** AMS stream ids are chosen by the publisher; one containing
  `#`, `?`, a space or `/` (e.g. `test#peer`) broke the `webrtc-client-stats`
  request URL — the poller silently hit the wrong AMS endpoint and dropped that
  stream's viewer-side quality metrics with no error. The stream id is now
  percent-escaped before it goes into the path. Ordinary ids are unaffected
  (byte-identical). (Found by the S48 subsystem audit, finding [3].)
- **Two AMS apps can host the same stream id without colliding (D-111).** AMS
  stream identity is `(app, streamId)`, but two collector paths keyed only on the
  bare `streamId`. (1) The REST-poll deduplicator dropped the second app's
  `publish_start`/`end` when both apps had a stream with the same id in one dedup
  window, so that app never appeared in ClickHouse; its key now includes `app`.
  (2) The live-snapshot aggregator, whose per-stream map is keyed by bare
  `streamId` (last-write-wins), evicted the *other* app's still-active stream when
  one ended; the delete is now guarded by pointer equality so only the owning
  stream removes its entry. (Found by the S48 subsystem audit, findings [1]+[2].)

### Security

- **Audience analytics is scoped to the requested tenant (D-110).** `GET
  /api/v1/analytics/audience?tenant=X` returned every tenant's audience rollups
  because the query omitted the `tenant` filter that the geo, device and QoE
  analytics queries already applied — a cross-tenant data-isolation leak. The
  audience query now filters by tenant like its siblings. (Found by a fresh
  adversarial audit of previously un-audited subsystems.)
- **Passwords are never hashed with a fast digest (D-109, CWE-916).** The password
  hasher used bcrypt but fell back to a single SHA-256 (a crackable, GPU-friendly
  digest) if bcrypt errored — which happens for passwords longer than 72 bytes.
  The fallback is removed (hashing fails closed instead), and creating a user with
  an over-long password now returns 422. Existing users with legacy `sha256:`
  password rows continue to authenticate (backward compatible).
- **API token `kind` is validated against an allowlist (D-109).** `POST
  /admin/tokens` accepted any `kind`, storing e.g. a `kind:"superadmin"` token that
  authenticates nowhere (a dead but valid-looking credential). It now accepts only
  `api` and `ingest` (422 otherwise) — the two kinds the auth layer honors.
- **Synthetic probes now stop at runtime when a tenant downgrades below the probe
  tier (D-108).** The HTTP probe-CRUD handlers gate `CheckProbes()` (403 on Free),
  but the background probe scheduler executed every enabled probe regardless — a
  tenant that downgraded Pro→Free kept probing indefinitely. The runner now checks
  a per-probe entitlement gate (wired to the license manager's `CheckProbes`) before
  each execution and skips the probe when the tier no longer permits it.
- **CSV export/statements are now formula-injection-safe (D-106).** The usage
  export (`GET /api/v1/reports/export`) and white-label statement generator wrote
  publisher-controlled columns (`app`, `stream_id`, `tenant` — an AMS
  application/stream name is chosen by whoever publishes) into CSV without
  neutralizing leading formula triggers (`= + - @`, tab, CR). A stream named
  `=cmd|'/c calc'!A0` (or `=HYPERLINK(...)`) became a live formula when the
  operator opened the file in Excel/Sheets/LibreOffice — which
  `docs/known-limitations.md` explicitly directs them to do. Both writers now go
  through a shared `reports.CSVSafeCell`/`UsageCSVRecord` that prefixes such cells
  with a single quote (OWASP CSV Injection mitigation); numeric columns are
  unchanged. Output is byte-identical for benign data.
- **Email/SMTP alert-channel credentials are encrypted at rest (D-106).** The
  `password`/`username` of an email channel were serialized into `config_public`
  in plaintext (the `secretFields` allowlist omitted them); they are now encrypted
  into `config_enc` like every other channel secret. Existing channels keep working
  (the factory merges public + decrypted config on read).
- **OIDC login state cookie is `Secure` on HTTPS (D-106).** The `pulse_oidc_state`
  cookie (which carries the PKCE `code_verifier`) lacked the `Secure` attribute, so
  a browser could transmit it over plaintext HTTP on an HTTPS deployment. It now
  mirrors the `pulse_session` policy (`Secure` when the redirect URL is https).

### Fixed

- **Deleting or revoking a non-existent user/token no longer writes a phantom
  audit entry (D-109).** `DELETE /admin/users/{id}` and `DELETE /admin/tokens/{id}`
  are idempotent (204 even for a missing id, by design), but they recorded a
  fabricated `user.delete` / `token.revoke` in the audit log for ids that never
  existed. The audit entry is now written only when a row was actually removed; the
  idempotent 204 is unchanged.
- **The default-preset and boundary anomaly alerts fire consistently (D-109).** An
  observed value whose z-score landed exactly on the configured sigma threshold was
  flagged by the detection pass but silently suppressed by the alert-evaluation pass
  (`>` vs `>=`). Both paths now use the same inclusive boundary.
- **A committed user/token create is always audited (D-109).** The create handlers
  recorded the audit entry after a response re-fetch that could return nil (a
  concurrent-delete race), leaving the committed create unrecorded — the same class
  fixed for updates in the S40 audit work. The create is now audited before the
  re-fetch.
- **The live dashboard WebSocket now accepts browser (cookie / `?token=`) auth
  (D-108).** `GET /api/v1/live/ws` sat behind the header/cookie-only bearer
  middleware while its handler re-extracted the token from the header/`?token=`
  only — so an OIDC `pulse_session` cookie session (no header) was rejected, and a
  browser connecting via `?token=` (the only method a browser can use for a
  WebSocket) was blocked by the middleware before the handler ran. The route now
  uses the same auth path as file downloads (header / `pulse_session` cookie /
  `?token=`) and reads the validated token from request context. This path also
  enforces `kind=api` + expiry, which the previous inline lookup did not.
- **Editing a report schedule no longer silences it (D-107).** `PUT
  /api/v1/reports/schedules/{id}` rebuilt the row from the request body, which
  NULLed `next_run_at`; the scheduler selects due schedules with `next_run_at IS
  NOT NULL`, so any edited schedule stopped firing permanently. The update handler
  now recomputes `next_run_at` from the (possibly changed) cron and preserves
  `last_run_at`, matching the create handler.
- **The "Monthly" report-schedule preset now fires monthly, not daily (D-107).**
  The 5-field cron parser dropped the day-of-month field, so the UI's default
  preset `0 6 1 * *` ("Monthly, 1st of month, 6 AM UTC") matched the next 06:00 on
  *any* day. `nextCronTime` now honors day-of-month (standard Vixie cron
  dom/weekday semantics); weekly/daily presets are unaffected.

---

## [0.3.0] - 2026-07-11

Operator-approved release ("ship v0.3.0", D-076) carrying SESSION-10 through
SESSION-15 (D-068 … D-075). First release rendering the brandkit UI in production.

### Added

- **Synthetic probes — all four protocols are now real probes** (was: HLS only):
  - **WebRTC**: full chain — WS signaling (`signaling_state`, `connect_time_ms`,
    D-072), pion ICE media-path check (`ice_state`, D-074), and per-run network
    stats `rtt_ms` / `jitter_ms` / `loss_pct` measured from ~2 s of inbound RTP
    (D-075). Metrics not measured are *absent*, never zero. Live-verified against
    a production AMS 3.0.3.
  - **RTMP**: real TCP handshake probe (C0/C1→S0/S1/S2→C2 with strict S2-echo
    validation; `connect_time_ms`; D-073).
  - **DASH**: full MPD parse + segment fetch with timescale-adjusted bitrate
    (D-073).
- **SSO / OIDC** end-to-end: server-side OIDC (D-070) and SPA login — "Sign in
  with SSO" button, cookie-session browser auth, `/auth/oidc/status` +
  `/auth/me`, OIDC-aware sign-out (D-074).
- **Postgres meta-store backend** (`PULSE_META_BACKEND=postgres`) for HA
  deployments; SQLite remains the zero-config default (D-072).
- **Anomaly detection**: two new metrics — `ingest_bitrate_kbps` (per-stream) and
  `disk_pct` (per-node) — alongside viewers/CPU/memory (D-074); anomaly rule
  editor UI (D-070).
- **White-label PDF reports**: operator logo in report headers (D-070).
- **`qa/licensegen`**: `-privkey` / `-expires` flags — self-serve production
  license minting (D-068, documented in `docs/licensing.md` §3).
- **Probe results retention**: `{retention_days}`-configurable ClickHouse TTL
  (default 90 days, D-073).

### Changed

- **Brandkit UI re-theme** (D-071/D-072): the web UI now uses the operator
  brandkit design system (`brandkit/design-system/tokens.json`) — IBM Plex
  (self-hosted), new palette, dark theme. Light theme/density/motion follow in a
  later release.
- **Live snapshot rebuild is O(1) incremental** (was O(N²) per event at high
  stream counts): ~688× faster at 1k streams, allocations per event 1021→1
  (D-068).

### Fixed

- **WebRTC probes against real AMS**: real AMS 3.0.3 sends a `notification`
  (e.g. `subtrackAdded`) *before* the SDP offer — the probe's signaling parse
  failed against every live stream while CI's mock passed (mock-only ordering).
  Fixed with a notification-skip read loop; the AMS error `definition` is now
  surfaced in `error_msg`; CI mock now mirrors the real ordering (D-074).
- **Probe segment downloads capped at 32 MB** (`LimitReader`): a huge or
  misbehaving segment can no longer produce a silently wrong bitrate or unbounded
  memory use; over-cap runs report `segment_too_large` (D-074).

### Security

- **go-jose/v4 bumped 4.0.5 → 4.1.4** (CVE-2026-34986, HIGH: DoS via crafted JSON
  Web Encryption; go-jose is part of the OIDC token-verification stack). Caught by
  the release pipeline's Trivy gate during this release (D-076).

### Database

- ClickHouse migrations **0006** (probe-results TTL), **0007** (`ice_state`),
  **0008** (`rtt_ms`/`jitter_ms`/`loss_pct`, `Nullable(Float32)`) apply
  automatically via the `pulse-migrate` one-shot on upgrade; all are idempotent
  (`IF NOT EXISTS`).

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
