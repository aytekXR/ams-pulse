# What's New in Pulse 0.4

Pulse 0.4 is the first marketplace-ready release: it ships a one-command Docker Compose install,
fully wired node-down alerting, persistent anomaly history, and VoD recording billing — all
validated against a live AMS 3.0.3 Enterprise deployment. Ten API-correctness bugs are fixed, a
three-rung AMS node health-warning ladder is introduced, and the web UI gains a light theme and
density controls. Pulse 0.4 has been running in production on beyondkaira.com since 2026-07-13.

## Highlights

- **One-command quickstart install** — `install.sh` (health-gated, no-TTY safe), a
  six-variable `.env.example`, and database migrations baked into the Docker image (no bind
  mount required).
- **Three-rung AMS node health ladder** — `ams_api_latency_ms` is now a Welford-baselined
  anomaly metric; `node_degraded` gates on consecutive API errors; `node_down` alerts now fire
  correctly — the eviction goroutine was implemented but never started in prior releases.
- **Persistent anomaly flag history** — `GET /anomalies` now accepts `from`/`to` and returns
  from a ClickHouse-backed store so anomaly spikes survive restarts and are queryable over any
  window.
- **VoD recording billing** — `recording_gb` is now populated by polling the AMS VoD REST API
  on every 12th collector tick; the prior webhook path was silently rejected on AMS 3.x.
  Live-validated: 0.02% reconciliation drift against AMS.
- **Trial-license lifecycle** — expiry is evaluated continuously; a trial that crosses its
  deadline degrades to Free tier with a non-dismissable dashboard banner and no restart required.
- **WebRTC probe metric columns** — the probe results table shows `ice_state`, `rtt_ms`,
  `jitter_ms`, and `loss_pct`; absent values display as a dash, not zero.
- **Light theme and density modes** — OS/user-selected light mode applies all 15 brandkit color
  tokens; compact and wall density modes persist to localStorage; `prefers-reduced-motion`
  collapses motion tokens throughout.

## Reliability and security improvements

**API correctness:** ingest-health filter and bucket-size params were silently ignored; the
probe scheduler emitted a duplicate result row on every 60-second config refresh; eight list
endpoints ignored `limit`/`cursor` pagination; two server panics under pagination are closed.
Standalone AMS 3.x nodes no longer build poisoned zero-mean anomaly baselines. The production
compose overlay now correctly passes license env vars.

## New in 0.4.1 (since 0.4.0)

- **Security hardening pass** — SSRF guard on synthetic-probe URLs closes cloud-metadata
  escalation; opt-in webhook replay protection via `PULSE_WEBHOOK_REQUIRE_TIMESTAMP`; STARTTLS
  fails closed and SMTP header injection is closed for alert email channels; RTMP/DASH prober
  memory-exhaustion vectors closed; read-only container root filesystem, all Linux capabilities
  dropped, `no-new-privileges`; `govulncheck` reports 0 reachable Go vulnerabilities and
  `npm audit` is clean.

- **Alert engine memory bound** — the per-rule, per-stream firing-state map was never pruned;
  settled entries whose stream is gone and whose cooldown has lapsed are now evicted each tick,
  preventing unbounded growth on long-running servers with high stream churn.

- **Stream-offline alert correctness** — a wildcard `stream_offline` rule correctly pages after
  a disable/maintenance window while the stream stays offline; the in-flight hold deadline is
  frozen at first offline detection and is no longer retroactively expired by a mid-event
  `window_s` change.

- **Report-artifact retention pruning** — scheduled CSV/PDF artifacts are now auto-pruned after
  `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` days (default 90; 0 = keep forever); previously they
  accumulated indefinitely. Artifacts are also now persisted in the base compose, not only the
  hardened overlay.

---

The [GitHub release page for v0.4.1](https://github.com/aytekXR/ams-pulse/releases/tag/v0.4.1)
is live; the demo rough-cut video (`pulse-demo-roughcut.webm`) is attached to that release.

Full change history: [`CHANGELOG.md`](../../CHANGELOG.md)
