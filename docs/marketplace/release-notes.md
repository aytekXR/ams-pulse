<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

# What's New in Pulse 0.4

Pulse 0.4 is the first marketplace-ready release: it ships a one-command Docker Compose
install, fully wired `node_down` alerting, persistent anomaly history, and VoD recording
billing — all validated against a live AMS 3.0.3 Enterprise deployment. Ten API-correctness
bugs are fixed, a three-rung AMS node health-warning ladder is introduced, and the web UI
gains a light theme and density controls. Pulse 0.4 has been running in production on
beyondkaira.com since 2026-07-13.

## Highlights

- **One-command quickstart install** — `install.sh` (health-gated, no-TTY safe), a
  six-variable `.env.example`, and database migrations baked into the Docker image (no bind
  mount required). (`deploy/quickstart/`, D-089)
- **Three-rung AMS node health ladder** — `ams_api_latency_ms` is now a Welford-baselined
  anomaly metric (D-087); `node_degraded` gates on consecutive API errors (D-088); `node_down`
  alerts now fire — BUG-011: the eviction goroutine was implemented but never started (D-087).
- **Persistent anomaly flag history** — `GET /anomalies` now accepts `from`/`to` and returns
  from a ClickHouse-backed store so anomaly spikes survive restarts and are queryable over any
  window. (D-086)
- **VoD recording billing** — `recording_gb` is now populated by polling the AMS VoD REST API
  on every 12th collector tick; the prior webhook path was silently rejected on AMS 3.x.
  Live-validated: 0.02% reconciliation drift against AMS. (D-085, BUG-002 FIXED)
- **Trial-license lifecycle** — expiry is evaluated continuously; a trial that crosses its
  deadline degrades to Free tier with a non-dismissable dashboard banner and no restart required.
  (D-089)
- **WebRTC probe metric columns** — the probe results table shows `ice_state`, `rtt_ms`,
  `jitter_ms`, and `loss_pct`; absent values display as a dash, not zero. (D-077)
- **Light theme and density modes** — OS/user-selected light mode applies all 15 brandkit
  color tokens; compact and wall density modes persist to localStorage; `prefers-reduced-motion`
  collapses motion tokens throughout. (D-077)

## Reliability and security improvements

**API correctness:** ingest-health filter and bucket-size params were silently ignored
(BUG-004/005, D-082/083); the probe scheduler emitted a duplicate result row on every 60 s
config refresh (BUG-003, D-082); eight list endpoints ignored `limit`/`cursor` pagination
(BUG-006/007/010, D-084); two server panics under pagination are closed. Standalone AMS 3.x
nodes no longer build poisoned zero-mean anomaly baselines (D-088). The production compose
overlay now correctly passes license env vars (D-076).

## New in 0.4.1 (since 0.4.0)

- **Security hardening pass** — SSRF guard on synthetic-probe URLs closes cloud-metadata
  escalation (D-130); opt-in webhook replay protection via `PULSE_WEBHOOK_REQUIRE_TIMESTAMP`
  (D-123); STARTTLS fails closed and SMTP header injection is closed for alert email channels
  (D-125); RTMP/DASH prober memory-exhaustion vectors closed (D-127/D-128); read-only container
  root filesystem, all Linux capabilities dropped, `no-new-privileges`; `govulncheck` reports
  0 reachable Go vulnerabilities and `npm audit` is clean (D-142).

- **Alert engine memory bound** — the per-rule, per-stream firing-state map was never pruned;
  settled entries whose stream is gone and whose cooldown has lapsed are now evicted each tick,
  preventing unbounded growth on long-running servers with high stream churn. (D-160)
- **Stream-offline alert correctness** — a wildcard `stream_offline` rule correctly pages after
  a disable/maintenance window while the stream stays offline; the in-flight hold deadline is
  frozen at first offline detection and is no longer retroactively expired by a mid-event
  `window_s` change. (D-159)
- **Report-artifact retention pruning** — scheduled CSV/PDF artifacts are now auto-pruned after
  `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` days (default 90; 0 = keep forever); previously they
  accumulated indefinitely. Artifacts are also now persisted in the base compose, not only the
  hardened overlay. (D-143)

---

Full change history: [`CHANGELOG.md`](../../CHANGELOG.md)
