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

## New in 0.4.3 (since 0.4.2)

An honesty-and-hardening release. An external technical review checked the 0.4.2 artifact
against its own documentation and found the remaining risk was mostly in *claims*, not code.
Everything below is a correction we made rather than a feature we added — we would rather you
find our limits written down than discover them yourself.

- **Cluster capabilities are now described accurately.** Earlier docs said Pulse showed "real
  origin/edge roles" with node versions, and that edge/origin viewer deduplication was active.
  Ant Media Server 3.x does not expose a node role or version on its cluster-nodes endpoint,
  so **every node displays as `origin` with no version, and viewer dedup does not activate**.
  The code is in place and would work unchanged if a future AMS exposed roles. This is
  documented in Known Limitations LIM-10, along with the situations in which cluster node
  alerting can miss during an AMS API outage.
- **Two 0.4.2 release-note claims corrected.** AMS error webhooks are recorded to ClickHouse
  and queryable, but have no dashboard or alert surface yet (LIM-27) — they were previously
  described as feeding ingest health. And the compose path that "boots out of the box" is the
  documented evaluator command (base file + `docker-compose.evaluator.yml`); the base file
  alone deliberately publishes no ports.
- **Quickstart installer fixes.** Re-running the installer against a healthy install no longer
  fails on Pulse's own port, and `PULSE_HOST_PORT` is honoured on the piped-install path (0.4.2
  hardcoded 8090). The installer now fails immediately with an explanation instead of polling a
  port nothing is listening on.
- **Helm: the deprecated ClickHouse memory key works again.** If your values file tuned
  `maxMemoryUsageForAllQueries` and not `maxServerMemoryUsage`, your setting was silently
  dropped and the server cap fell back to 768 MB. It is now honoured.
- **Supply chain.** The release pipeline's quarantine cleanup — which removes an image that
  failed its vulnerability scan — could never execute due to a token/endpoint mismatch, and
  reported success while doing nothing. Fixed, with the failure now surfaced loudly. A release
  guard that verifies the Helm chart version was bumped had also been silently skipping.
- **Documentation accuracy pass.** The install guide's tier table showed Business at 5 nodes
  instead of 50; schema table counts, the environment-variable reference, and several
  install-path instructions were corrected. AMS Kafka ingest is now labelled EXPERIMENTAL
  everywhere it appears, matching what LIM-19 already said.

## New in 0.4.2 (since 0.4.1)

- **Cluster fleet discovery works against real AMS 3.x clusters.** Node discovery now uses the
  cluster-mode probe plus the paginated cluster-nodes endpoint that Ant Media Server actually
  serves (verified against the AMS 3.0.3 source), and decodes the real node fields — per-node
  IDs, CPU and memory. Previously a clustered AMS silently appeared as a single standalone
  node. Two limits are disclosed in Known Limitations (LIM-10): AMS 3.x sends no node **role**
  or **version** on that endpoint, so all nodes display as `origin` with no version and
  edge/origin viewer dedup stays inactive; and live multi-node validation is still pending.
- **Kafka ingest consumes the official AMS topics** (`ams-instance-stats`,
  `ams-webrtc-stats`) with the official message shapes; the previous topic name was never
  published by AMS. The feature remains marked experimental until validated against a live
  AMS Kafka producer.
- **Webhook ingest accepts the official AMS payload** — form-urlencoded bodies and the
  official `id` field are parsed; stream-error events (`endpointFailed`,
  `publishTimeoutError`, `encoderNotOpenedError`) are now recorded to ClickHouse instead of
  being dropped. They are queryable today; a dashboard panel and alert condition for them
  have not shipped yet (see Known Limitations LIM-27).
- **Token-mode authentication reaches all management endpoints** — the `ProxyAuthorization`
  header AMS expects for management-scope calls is now sent, so app auto-discovery and fleet
  data work with a static bearer token (no console login needed).
- **Business licenses grant 50 nodes end-to-end** — a remaining key-generation path still
  issued 5-node Business keys; the full tier ladder is now regression-locked.
- **Installer hardening** — the quickstart preserves your `.env` (and its encryption key) on
  failed installs, reuses an existing key on re-runs, and distinguishes "Pulse up, AMS
  unreachable" from a failed install; the documented evaluator command (the base compose file
  plus `deploy/docker-compose.evaluator.yml`, which publishes the UI on localhost) now boots
  out of the box, and CI boots that exact command and asserts the UI over the published host
  port. The base file alone deliberately publishes no ports.
- **Helm chart fixes** — the beacon-ingest port is actually served, Postgres-backed installs
  schedule correctly, S3 report export works from chart values, and the bundled ClickHouse
  locks down its default account when a password secret is configured (plus an opt-in
  NetworkPolicy).
- **Supply-chain additions** — releases now attach standalone `pulse` binaries
  (linux/amd64 + arm64) with SHA256SUMS, publish the Helm chart as an OCI artifact, and the
  published multi-arch image is vulnerability-scanned by digest for both architectures.

The [GitHub release page for v0.4.2](https://github.com/aytekXR/ams-pulse/releases/tag/v0.4.2)
carries the signed-image digest, binaries, checksums, chart, and SDK tarball.

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
