# Changelog

All notable changes to Pulse are documented in this file.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
D-numbers reference the decision log at `agents/handoffs/decisions.md`.

---

## [Unreleased]

## [0.4.3] - 2026-07-27

Honesty release. External review round 4 found the residual risk was concentrated in
*claims* rather than code: several documents asserted cluster capabilities that AMS 3.x
makes structurally impossible, and two release-note claims about 0.4.2 were falsifiable in
minutes. This release makes the documentation match the code, and fixes the supply-chain and
installer defects found alongside it. See `docs/assessment/marketplace-compliance-review-2026-07-27-round4.md`.

### Changed

- **Cluster capability claims corrected across the documentation set (D-176, review round 4
  F-03).** README, `overview.md`, `product.md`, `api-guide.md`, `ARCHITECTURE.md` and the
  demo voiceover script asserted "real origin/edge roles", a populated node version field,
  and active edge/origin viewer dedup. AMS 3.x exposes **no `role` and no `version`** on its
  cluster-nodes endpoint, so discovery defaults every node to `origin`, the version stays
  empty, and `IsEdgeStream()` — which requires `role == "edge"` — can never activate. These
  were statically provable facts, not pending validation. LIM-10 rewritten accordingly, and
  `/live/overview` now resolves node role through cluster discovery instead of hardcoding
  `"standalone"`, so it agrees with `/fleet/nodes`.
- **Marketplace release notes no longer overstate v0.4.2 (D-176, F-02).** Error webhooks are
  described as recorded and queryable (per LIM-27) rather than "captured for ingest health";
  the boot claim now names the documented evaluator command instead of the plain compose
  path, which publishes no ports and is not what CI exercises.

### Fixed

- **`CPUPctOK`/`MemPctOK` fabricated a measured 0% when the field was absent entirely
  (D-176, F-14).** With neither the real wire field nor the alias present, both returned
  `(0, true)` — a false "measured zero" reaching the Fleet card and the Welford anomaly
  baselines through a different input than the NaN case fixed in D-175. The test that pinned
  this behaviour as correct was rewritten and a regression test added.
- **The Helm chart's deprecated ClickHouse memory alias was silently ignored (D-176, F-09).**
  `maxMemoryUsageForAllQueries` was documented as honoured when `maxServerMemoryUsage` is
  unset, but shipping **both** as chart defaults made the `| default` fallback unreachable —
  an operator values file tuning only the old key was dropped and the server cap snapped back
  to 768 MB on upgrade. The default now lives in the template so precedence is genuinely
  new key → deprecated alias → 768 MB (verified across all four cases).
- **Release guard #16 never ran (D-176, F-09).** It resolves the previous tag with
  `git describe`, but the release checkout was shallow with no tags, so it silently no-opped
  on every release. Added `fetch-depth: 0` + `fetch-tags: true`; the check now fails loudly
  rather than skipping, and its diff is scoped to `templates/` + `values.yaml` so it no
  longer degenerates into "always bump the chart semver".
- **Quickstart installer: re-runs hard-failed and the port remedy was unreachable
  (D-176, F-08).** The new busy-port preflight matched *any* listener including Pulse's own
  published port, so re-running the installer on a healthy install exited 1 — breaking the
  idempotent re-run path. It now exempts a listener owned by the quickstart's own stack. A
  second preflight fails loudly when the pinned compose cannot honour a requested
  `PULSE_HOST_PORT` (releases up to v0.4.2 hardcode `8090:8090`) instead of health-polling a
  dead port until timeout. The header's `| bash` example corrected to `| bash -s --`.
- **mock-ams marked every cluster node permanently "down" (D-176, F-05).** Cluster fixtures
  hardcoded a **2024** `lastUpdateTime`, which is far beyond the staleness timeout, so every
  mock cluster run operated in the all-down state — and because first-sight transitions are
  not logged, nothing surfaced it. Timestamps are now `now`-relative.
- **The poller emitted cluster nodes with an empty identity (D-176, F-14).** `PrimaryID()`
  returns `""` when `id`, `nodeId` and `ip` are all absent; discovery guarded this, the
  poller did not, producing a blank phantom node and a `""` entry in the failure-streak
  fan-out set.
- **Documentation residue batch (D-176, F-10/F-11/F-13/F-14).** `install.md`'s tier table
  claimed Business = **5** nodes against the code's 50; schema counts were wrong (meta 14→16,
  ClickHouse 9 tables/5 MVs → 11/7); "all variables are listed below" covered 38 of 69;
  `make up` was recommended as the released-image path while it builds from source; the
  clean-install status contradicted the submission pack. Also: LIM-01's cluster field names
  corrected to the real `cpu`/`memory`, Kafka marked EXPERIMENTAL at the README and
  `install.md` surfaces, `alerting.md`'s two false `node_degraded` descriptions corrected,
  README's production-version claim corrected to the true v0.4.0-139, and three
  non-functional occurrences of the VPS public IP scrubbed.

### Security

- **Removed a production credential prefix from the repository (D-175).**
  `server/cmd/pulse/migrate_test.go` — the regression test that exists to prevent password
  leaks in logs — used the first 32 of the 48 hex characters of the live production
  `CLICKHOUSE_PASSWORD` as its test input, committed since `98b011c`. Replaced with a fixed
  dummy. Git history cannot be un-published, so **rotation of that credential is required**
  and is the top item in `docs/operator-expected.md`. ClickHouse is not internet-facing, so
  this was never remotely exploitable.
- **A failed release no longer leaves a vulnerable image publicly pullable (D-175, R7).**
  The Trivy quarantine flow pushed `candidate-<sha>` and never cleaned it up, so an image
  that FAILED its scan stayed public under the candidate tag forever. Cleanup now runs on
  every outcome — but only when the candidate tag is the digest's sole tag, because a GHCR
  package "version" is a digest and promotion re-tags that same digest; deleting it
  unconditionally would have deleted the published release, its SBOM and its signature.
  **Correction (external review round 4, F-07):** as first shipped this step could never
  execute — it authenticated with `GITHUB_TOKEN`, which has no authenticated-user context
  for `/user/packages/...` and lacks the `delete:packages` scope, so the list call came back
  empty and the step exited 0 reporting "nothing to clean up". It now uses the
  `/users/{owner}/...` endpoint with a dedicated `GHCR_CLEANUP_TOKEN` secret and asserts the
  HTTP status. **Until that secret is configured the step warns loudly and the quarantine
  tag must be deleted by hand** after a failed release.

### Fixed

- **Cluster degraded-node alerting was dead during AMS API outages (D-175, R1).** After the
  0.4.2 change that gave cluster events their real AMS node IDs, failure-streak events were
  still stamped with the single configured `PULSE_AMS_NODE_ID`. The aggregator drops
  `api_unreachable` events for unknown node keys, so `ConsecAPIErrors` never advanced for any
  real node and `node_degraded` could not fire for the entire outage — the exact condition
  the alert ladder exists to catch. Streak events now fan out over the nodes seen in the last
  successful cluster poll.
  **Scope (corrected after external review round 4, F-04):** this fixes the addressing, not
  the whole ladder. `node_degraded` needs 3 consecutive errors (15 s at the default cadence)
  while stale-node eviction also fires at 3×`PollInterval`, so on a cluster the degraded
  state can exist for only a short window before the node is evicted; a discovery poll
  succeeding mid-outage resets the streak; and an outage of `/rest/v2/applications`
  short-circuits the poll before the cluster branch runs at all. Cluster alerting during an
  AMS API outage remains **not fully reliable and not live-validated** — see LIM-10.
- **Cluster node metrics were being overwritten with fabricated zeros (D-175, R2).**
  `cluster.Discovery` emitted `disk_pct` / `net_in_mbps` / `net_out_mbps` / `jvm_heap_used_mb`
  / `version` unconditionally — fields real AMS 3.x never sends. Since both emitters now key
  on the same real node ID, those zeros overwrote the poller's clean event every 30 s: the
  Fleet card rendered "Disk 0%" as a measurement and the zeros fed the Welford anomaly
  baselines. Emission is now conditional and test-pinned to match the poller key-for-key.
- **A dead cluster node reported healthy forever (D-175, R6).** AMS keeps a dead member listed
  with a frozen `lastUpdateTime`, so the vanish-based staleness sweep never fired for it.
  AMS's own `status` and `lastUpdateTime` now mark a node down, which also releases the
  origin-viewer suppression that a dead edge used to hold open.
  **Scope (corrected after external review round 4, F-05):** the `down` state is currently
  internal — it is not emitted on `node_stats`, the Fleet API has no `down` value, and no
  alert keys on it, so its only user-visible effect today is a Warn log. Surfacing it end to
  end is tracked debt.
- **A single `NaN` CPU reading was recorded as a measured 0% (D-175, R5).** Non-finite values
  were rejected but fell through to an alias field that is 0 on real AMS. Readings with no
  usable value are now reported absent rather than fabricated.
- **The documented README evaluator command published no ports (D-175).** The base compose
  file is `expose:`-only and explicit `-f` flags suppress the auto-override, so
  `docker compose -f deploy/docker-compose.yml up -d` produced a healthy stack with an
  unreachable UI. Added `deploy/docker-compose.evaluator.yml` (loopback-only publish); CI now
  boots the documented command and asserts the UI over the published host port.
- **Quickstart installer (D-175, R11/R12).** A self-signed `PULSE_LICENSE_PUBKEY` was
  overwritten with the official key on every re-run; `export`-prefixed or indented keys in a
  hand-edited `.env` were missed by the value extractor, minting a new secret key and leaving
  the meta store undecryptable. Both fixed. The installer also now detects a busy host port
  with actionable guidance (`PULSE_HOST_PORT`), and keeps verifying the AMS connection past
  the collector's 30 s staleness floor instead of printing "healthy" ~20 s too early.
- **Helm ClickHouse tuning was partly inert (D-175, R4).** `max_threads` sat at server scope
  where ClickHouse silently ignores it, the deprecated `max_memory_usage_for_all_queries` was
  still rendered, and the server-wide memory cap was wired to the per-query value (512 MB
  instead of 768 MB), contradicting the compose XML's stated parity. Chart bumped to 0.3.1 —
  `helm push` overwrites an existing version, so a content change without a version bump
  silently replaces a published artifact. New release-guard check enforces that bump.
- **Marketplace assets (D-175).** The flagship screenshot showed all 8 streams as UNKNOWN
  state/health and 0 viewers/0 publishers per application, because the capture mocks used
  field names the API schema does not define; the alerting screenshot showed only the Rules
  tab while its caption promised incident history. Both regenerated and visually verified.
  The protocol-mix donut's slice labels, which overlapped its own legend and were clipped at
  the card edge, now fit.
- **Docs (D-175, R3/R9/R10/R15).** SDK install instructions pointed at the 0.4.1 tarball —
  the build whose `Pulse.init()` was a silent no-op — now 0.4.2 and guarded at release time.
  `licensing-public.md` understated the Pro tier (analytics CSV export is Pro+, not
  Business+). The submission-package index still announced v0.4.1. All 32 line-number source
  citations in `AMS-INTEGRATION.md` were de-numbered (one pointed at the wrong file entirely),
  removing the drift mechanism. New limitations disclosed: LIM-27 (ingest-error webhooks are
  recorded but not yet surfaced) and LIM-28 (cluster-only stream/node ID mismatch).

### Changed

- **CI boots the release commit instead of skipping it (D-175, R14).** `compose-boot` deferred
  whenever the image pin equalled `VERSION` and wasn't on GHCR yet — which is always true on
  the commit a release gates on, so the evaluator path was never booted against a
  to-be-released state. It now builds the pinned image locally and boots it.
- **Release version guard: 13 → 16 checks** — submission-package header stamp, SDK
  install-section tarball version, and the Helm chart-version bump.

---

## [0.4.2] - 2026-07-26

### Fixed

- **Cluster path hardened against real-cluster failure modes (D-174, REVIEW-MP3-2026-07-26
  N1/N2/N-cluster).** Adversarial re-review of the D-173 cluster rebuild found the fix itself
  had new defects; all closed before this tag:
  - A transient 500 from the nodes route on a REAL cluster (Mongo/Redis blip) was mapped to
    the standalone fallback, silently collapsing the fleet to a synthetic single node,
    resetting the failure-streak alerting, and poisoning the cluster-mode cache for ~60 calls
    (N1). The `cluster-mode-status` probe now owns the mode cache exclusively; first-page
    404/500 maps to standalone only while the probe has not said "cluster"; mid-pagination
    errors and cluster-mode errors surface as real failures feeding the streak ladder. The
    regression test that appeared to cover this passed for the wrong reason (its fixture
    lacked `system-status`) — fixture fixed, plus new transient-500/mid-pagination/cache-
    survival tests.
  - Every cluster node event was stamped with the single configured `PULSE_AMS_NODE_ID`
    (default `standalone`), collapsing an N-node fleet onto one flickering identity while
    cluster discovery emitted the same nodes under their real IDs (N2). Node events now keep
    the node's real cluster ID; the configured ID remains only the standalone-path identity.
  - Alias-only fields real AMS 3.x never sends (`disk_pct`, `net_*`, `jvm_heap_used_mb`,
    `version`) are no longer emitted as fabricated zeros; `"NaN"`/`"Inf"` CPU/mem strings
    (Java `Double.toString`) are rejected instead of reaching the WS broadcast where
    `json.Marshal` fails; numeric `cpu`/`memory` JSON variants decode instead of failing the
    page; pagination is capped at 40 pages (2 000 nodes) with a 10 s per-poll deadline; the
    mode probe backs off (~1 min) instead of firing every poll while the endpoint errors.
- **Hardened Compose overlay boots again (D-174, N3).** The Issue-C base-file fix
  (`CLICKHOUSE_SKIP_USER_SETUP: "1"`) merged per-key into the hardened overlay, which
  silently skipped named-user provisioning — the authenticated healthcheck then failed
  forever and `pulse`/`pulse-migrate` never started. The overlay now explicitly neutralizes
  the flag, and CI asserts the merged base+hardened config keeps auth enforced.
- **`stream_ingest_error` events are persisted, not write-only (D-174, N4).** The Issue-J fix
  mapped AMS error webhooks to a new event type but the ClickHouse sink discarded the
  payload, storing indistinguishable rows. Migration 0011 adds `action` +
  `stream_name` columns, the sink populates them, and the event schema gained a
  `stream_ingest_error` data clause with valid/invalid contract fixtures.
- **Quickstart re-runs are safe with special-character passwords (D-174, N5).** The Issue-G
  secret-reuse fix `source`d the `.env` it had written unquoted — an AMS password containing
  a space aborted every re-run, and `$(…)` was executed by the installer itself. Values are
  now grep-extracted (inert by construction), and an operator-added `PULSE_LICENSE_KEY` is
  preserved on re-run instead of being blanked.
- **Trivy gates the release tags again (D-174, N6).** The digest-scan rework had moved
  scanning after the push, so a HIGH/CRITICAL finding failed the release only after the
  vulnerable image was publicly pullable under `X.Y.Z`/`latest`. The image is now pushed
  under a `candidate-<sha>` quarantine tag, scanned by digest (both arches), and promoted to
  the real tags only after the scans pass (identical digest — SBOM/provenance/cosign
  unaffected).
- **Beacon-JS survives a lost version define (D-174, N7).** `declare const SDK_VERSION`
  emitted no JS, so a build without the tsup define turned every `Pulse.init()` into a
  silent no-op session (the ReferenceError is swallowed by the zero-throw guard). The define
  is now `__SDK_VERSION__` with a `0.0.0-dev` runtime fallback, and CI greps the built dist
  for the real version literal.
- **Mint-path end-to-end regression (D-174, Issue-F residue).** The D-173 "mint-path" test
  only decoded the claims it minted; a new server-side test runs the real `qa/licensegen`
  binary for every tier through real license verification and asserts the published ladder
  (Free 1/7 d · Pro 10/90 d · Business 50/396 d · Enterprise unlimited).
- **Release version guard extended to 13 checks (D-174).** Now also covers root `README.md`
  image pins (any stale semver pin fails), the `docs/marketplace/listing.md` version claim,
  and the four Helm golden-test values files.
- **Docs/CI accuracy batch (D-174, N8/N9).** Listing copy: Go 1.25, internal "A10" reference
  removed, dev command stripped, category aligned to "Analytics & Monitoring", title
  char-count corrected; submission-package gate list reconciled with the real one (v0.4.2
  cut, category confirmation, AV-15, npm publish); dangling section references fixed;
  `final-assessment.md` superseded rows annotated; AMS-INTEGRATION documents all three auth
  modes, the `/metrics` Business+ gate, the 500-when-standalone quirk, and drops stale line
  citations; OpenAPI/API-reference `PULSE_AMS_BASE_URL` typo → `PULSE_AMS_URL`;
  `.env.example` placeholder-guard claim scoped to what the code checks; webrtc-stats fetch
  failures now surface once per outage at Warn; `clickhouse-low-footprint.xml` settings moved
  to the scope ClickHouse honors (query-level settings into a profile; deprecated
  `max_memory_usage_for_all_queries` → `max_server_memory_usage`); webhook unknown-action log
  demoted to Debug; residual IP scrub in qa/realams README.
- **Cluster fleet discovery works against real AMS 3.x clusters (D-173, REVIEW-MP2-2026-07-25
  Issue A; settles G-21).** `amsclient.ClusterNodes` called a flat `GET /rest/v2/cluster/nodes`
  that AMS 3.x does not register (source-verified at tag `ams-v3.0.3`: `ClusterRestServiceV2`
  exposes only `nodes/{offset}/{size}`), so every real cluster silently degraded to
  "standalone". The client now probes `GET /rest/v2/cluster-mode-status` (cached, re-probed
  every ~60 calls) and pages through `nodes/{offset}/{size}` when in cluster mode; a live
  standalone AMS 3.0.3 probe confirmed the paginated path 500s (not 404s) outside cluster
  mode, so a first-page 404/500 maps to the standalone fallback (guarded — see the D-174
  entry above). `ClusterNodeDTO` now decodes the
  real AMS wire fields (`id`, `ip`, `lastUpdateTime`, `memory`, `cpu`, `dbQueryAveargeTimeMs`,
  `status` — previously only `ip` overlapped, so nodes decoded all-zero) with tolerant aliases
  for the old keys; the dead `NodeInfo` method is removed. `qa/mock-ams` now serves the real
  shapes (flat route 404s, paginated 500s when standalone) plus the previously missing
  `system-status`, `version`, `vods/list`, and `users/authenticate` endpoints. Live multi-node
  cluster validation remains pending (LIM-10).
- **Business-tier license keys mint with 50 nodes (Issue F).** `qa/licensegen` still encoded
  `max_nodes: 5` for Business — below Pro's 10 — so every real Business key carried the
  inversion D-166 fixed in the entitlement table (entitlements are claim-driven by design). Now
  50, with a mint-path regression test locking the whole ladder (Free 1 / Pro 10 / Business 50 /
  Enterprise unlimited).
- **Quickstart installer no longer destroys operator secrets on failure (Issue G).** The EXIT
  trap deleted `.env` — including the freshly generated AES-GCM secret key — on any non-zero
  exit even when containers and volumes kept running with data encrypted under that key, and a
  re-run minted a new key (permanently orphaning the meta store). Cleanup now only removes an
  `.env` this run created before the stack started; an existing `PULSE_SECRET_KEY` is reused;
  "Pulse up but AMS unreachable" (collector `degraded`) is diagnosed as a warning with the AMS
  URL named instead of failing the install; raw.githubusercontent downloads pin `PULSE_REF`.
- **README base-Compose path can authenticate to ClickHouse (Issue C).** The base
  `deploy/docker-compose.yml` clickhouse service set no auth env at all, so the documented
  `-f deploy/docker-compose.yml up -d` command (which skips the auto-loaded override) failed
  migration with `code 516: Authentication failed`. `CLICKHOUSE_SKIP_USER_SETUP: "1"` is now
  set in the base file (ports stay expose-only); a new CI job boots exactly that command
  against the pinned GHCR image and asserts component-scoped `/healthz`.
- **Helm chart: beacon ingest port is live, Postgres mode schedulable, S3 export functional
  (Issue D).** The chart declared/routed containerPort 8091 but never rendered
  `PULSE_INGEST_LISTEN_ADDR` (nothing listened); `postgres.enabled=true` suppressed the data
  PVC while the deployment still mounted it (pod stuck `Pending`); the S3 env block rendered
  variable names the binary does not read and omitted bucket/endpoint/region;
  `PULSE_REPORTS_DIR` was unset (reports written to the ephemeral container root); ClickHouse
  StatefulSet emitted a duplicate `volumes:` key with persistence off. All fixed; goldens
  regenerated plus two new locked variants (`ch-persistence-off`, `existing-secret`), now also
  diffed in CI.
- **Helm ClickHouse `existingSecret` mode actually locks down the default user (Issue H).**
  The `users.xml` emitted a passwordless `default` superuser (`::/0` networks,
  `access_management=1`) in every configuration — the existingSecret branch changed only
  comments. With a secret set, `default` is now localhost-only with `access_management=0`
  (probes run in-pod, unaffected); an opt-in NetworkPolicy template restricts ClickHouse
  ingress to the pulse/backup pods; chart README/NOTES claims are now truthful.
- **OpenAPI spec matches the deployed surface (Issue I).** The 8 root-mounted paths
  (`/healthz`, `/metrics`, `/ingest/beacon`, `/auth/me`, `/auth/oidc/*`) carried the global
  `/api/v1` server base, so generated clients called `/api/v1/healthz` (404). Each now has a
  path-level `servers: [{url: /}]`; the WS bearer-token-in-`Sec-WebSocket-Protocol` mechanism
  (`pulse.v1, <token>`) is documented; the `/metrics` description states the Business+ gate
  (it claimed "unauthenticated by default"); `POST /admin/sources` honestly documents that it
  stores config without starting extra pollers. Conformance tests updated (including a
  workaround for kin-openapi's sticky path-level-servers router bug); `schema.d.ts` and the
  rendered API reference regenerated — JSDoc-only diff, zero type churn.
- **Official AMS error webhooks are captured instead of silently dropped (Issue J).**
  `endpointFailed`, `publishTimeoutError`, and `encoderNotOpenedError` now map to a new
  `stream_ingest_error` server event (schema enum extended); remaining unknown actions are
  counted and logged. FAQ/AMS-INTEGRATION wording corrected from "GET-only" to "read-only —
  the only non-GET is the optional cookie-login POST".
- **Beacon SDKs report their real version (Issue K).** Both SDKs still embedded `0.1.0` as
  `player.sdk_version` (poisoning SDK-adoption telemetry). beacon-js now injects the version
  from `package.json` at build time via tsup `define` (drift impossible; unit test asserts
  it); beacon-swift bumped to the package version, both now covered by the release version
  guard.
- **Placeholder secrets rejected at boot (P3).** A `PULSE_SECRET_KEY` containing `changeme`
  (the `.env.example` placeholder, which passed the ≥16-byte check) now fails startup with a
  clear message, in both `serve` and `migrate`.
- **`PULSE_AMS_LOGIN_EMAIL` supports the `_FILE` secrets convention (P3)** like its password
  sibling.
- **WebRTC-stats poll errors are logged (P3)** instead of silently swallowed in the REST
  poller.

- **Kafka collector now consumes the real AMS Kafka feed (D-172, REVIEW-MP-2026-07-25
  Issue 1).** The consumer subscribed to `ams-server-events` — a topic AMS never publishes —
  and parsed flat `cpuUsage`/`memoryUsage`/`diskUsage` fields the official messages don't
  carry. Default topics are now `ams-instance-stats,ams-webrtc-stats` (verified against AMS
  `StatsCollector.java`), overridable via the new `PULSE_KAFKA_TOPICS` env var; the
  normalizer routes by topic and parses the official nested shapes (`instanceId`, nested
  `cpuUsage`/`systemMemoryInfo`/`fileSystemInfo`), pinned as source-derived testdata
  fixtures. `ams-webrtc-stats` messages are consumed and skipped (no clean domain mapping
  yet). Unknown/custom topics keep the legacy field-sniffing for bridge feeds. The feature
  stays EXPERIMENTAL/PREVIEW until live validation against a real AMS producer (AV-15).
- **Webhook listener accepts the official AMS payload (Issue 2).** The stream id now falls
  back to the official `id` field (`streamId` still honored first for existing proxies);
  `vodReady` events now capture `vod_id` and `duration_ms`; `application/x-www-form-urlencoded`
  bodies (an AMS-configurable content type) are parsed instead of 200-and-dropped, with
  numeric-string tolerance. HMAC-over-raw-body verification and the deliberate 200-on-parse-
  failure (AMS retry-storm avoidance) are unchanged.
- **Static-token auth works for management-scope AMS endpoints (Issue 3).** `amsclient` now
  sends `ProxyAuthorization: <jwt>` alongside `Authorization: Bearer <jwt>` on every GET, so
  `PULSE_AMS_AUTH_TOKEN` mode reaches `server.jwtServerControlEnabled`-protected endpoints
  (`/rest/v2/applications`, `cluster/nodes`, `system-status`, `version`) — previously fleet
  and cluster data silently failed in token-only mode. App-scope behavior unchanged.
- **Marketplace screenshot/demo captures render the brand-default dark theme (Issue 4 root
  cause).** Playwright's default emulated `prefers-color-scheme` is light, so with no stored
  choice the whole "dark" screenshot set and the D-170 demo rough-cut silently rendered
  light (also the real cause of the ss1-light byte-duplicate). Both capture scripts now pin
  the theme explicitly, assert it applied, resolve Playwright relative to the repo (no
  machine-specific paths), and the screenshot set is committed to the repo so
  `docs/user-guide.md` renders on GitHub.

### Changed

- **Marketplace listing copy externalized (D-173, REVIEW-MP2-2026-07-25 Issue E).** New
  `docs/marketplace/listing.md` is the clean submission copy: Free tier carries the
  PolyForm-Noncommercial disclosure, Prometheus `/metrics` and 396-day retention corrected
  Pro+→Business+, egress marked as an estimate (CDN logs for invoicing), the cluster bullet
  softened to "live cluster validation pending", air-gapped licensing corrected from
  "(roadmap)" to supported-today, lab numbers labeled as CI/lab-measured, a long-form
  description added, category proposed (Analytics & Monitoring, pending A10), and zero
  internal identifiers/paths/negotiating notes. `listing-draft.md` demoted to the internal
  working file; `release-notes.md` rewritten in plain prose; `submission-process.md` §4
  re-labels the nightly matrix honestly as mock wire-format profile tests.
- **CI/release fidelity (D-173, Issue L).** The "AMS version matrix" workflow is renamed to
  what it is (mock wire-format profile tests) and its smoke curl now uses the real client
  path shape; mock-ams learned the four missing endpoints (so the VoD poller and login path
  are CI-exercised); Trivy scans the pushed multi-arch digest (amd64+arm64) instead of a
  separate local build; the release now requires a green e2e run, attaches
  linux/amd64+arm64 `pulse` binaries with SHA256SUMS, and pushes the Helm chart to GHCR OCI;
  the release version guard extends from 4 to 10 checks (compose/quickstart/helm/README
  pins, `PULSE_REF`, both SDK version constants); new CI jobs shellcheck the deploy scripts
  and boot the README compose path; prod/hardened ClickHouse healthchecks no longer expose
  the password via `docker inspect`; the referenced-but-missing
  `clickhouse-low-footprint.xml` now exists.
- **Publication hygiene (D-173, Issue K).** The production VPS IP is redacted from the
  assessment pack, runbooks, and both review records; `final-assessment.md` restamped and
  marked superseded (rows 7/8/12 reconciled); customer-facing docs no longer link into
  internal drafts; `docs/licensing.md` reflects the official-pubkey default;
  install/productionize runbooks corrected (migrations are baked into the image,
  `pulse-migrate` is in the base compose, meta DB default `pulse_meta.db`, Go 1.25);
  `server/internal/config.Load` is explicitly marked as not wired
  (`pulse.yaml`/`PULSE_LICENSE_OFFLINE_FILE` inert).
- **Evaluator compose path defaults to the signed GHCR image (Issue 10).**
  `deploy/docker-compose.yml` now pulls `ghcr.io/aytekxr/ams-pulse:0.4.1` (cosign-signed,
  Trivy-gated, SBOM-attached) instead of building from source; source builds move behind the
  new `deploy/docker-compose.build.yml` overlay (tagged `ams-pulse:local-build` so a local
  build can never masquerade as the signed tag). CI e2e keeps building from source via the
  overlay. The VPS prod path (`docker-compose.prod.yml` + `deployment.sh` stamped builds) is
  deliberately unchanged. Adversarial review then caught that this documented path had never
  actually worked standalone: the `pulse-migrate` one-shot lived only in the auto-loaded
  override (so `-f deploy/docker-compose.yml` booted an unmigrated ClickHouse), and the base
  file never mapped `PULSE_SECRET_KEY`/AMS credentials from `deploy/.env` into the container
  — both fixed in the base file (migrate service + `depends_on: service_completed_successfully`
  + env substitution), and `make up` now builds BOTH services from source via the overlay
  (no GHCR-binary/source-schema version splits).
- **Go module paths renamed to the real repo (Issue 6).**
  `github.com/pulse-analytics/pulse/*` → `github.com/aytekXR/ams-pulse/*` across all three
  modules (server, qa/mock-ams, qa/licensegen); Helm chart home/sources/maintainer, OpenAPI
  license/contact, and event-schema `$id`s (`pulse.beyondkaira.com`) now carry the real
  vendor identity. PagerDuty alert events now report `source: ams-pulse`.
- **Beacon JS SDK renamed `@pulse/beacon` → `ams-pulse-beacon` (Issue 7).** The old name was
  never published (npm 404) and the scope isn't ownable. v0.4.1 tarball attached to the
  GitHub release for offline install; `release.yml` now creates the GitHub Release, attaches
  the SDK tarball, and publishes to npm automatically once an `NPM_TOKEN` secret exists.
  The Swift SDK README's unresolvable SPM-URL snippet is replaced with working local-path
  integration instructions.
- **`release.yml` gained a version-consistency guard (Issue 5).** A tag push now fails fast
  if the VERSION file, Helm `appVersion`, or the product/faq/known-limitations doc headers
  disagree with the tag — the drift class the marketplace review flagged cannot recur
  silently.

### Added

- **Collector-freshness scrape metrics (D-167, ROADMAP §2.45).** `GET /metrics` now exposes
  `pulse_collector_last_success_timestamp` (Unix time of Pulse's most recent successful AMS
  poll; `0` if none since boot) and `pulse_collector_up` (`1` when that poll is within the
  staleness window, mirroring the `/healthz` collector decision). This lets a Prometheus user
  alert on Pulse's *own* blindness — `pulse_collector_up == 0` — which its internal alert
  engine cannot, because that engine evaluates metrics derived from the collector and falls
  silent when the collector does (the cause of the D-164 outage going unpaged). Emitted only
  when a collector-health source is wired (absent on a pure-beacon deployment). The built-in
  self-alert rule remains a separate, decision-gated item. Docs: `docs/guides/prometheus.md`.

## [0.4.1] - 2026-07-24

### Security

- **Source-test SSRF guard (D-166, REVIEW-EXT-2026-07-24 item 4).** `POST
  /admin/sources/{id}/test` now installs `ssrfguard.DialControl` on its transport (plus an
  IP-literal boundary check), refusing link-local/IMDS/NAT64-embedded targets at dial time
  while keeping private/loopback AMS hosts allowed. Previously the endpoint would dial
  `169.254.169.254` — a blind SSRF timing/error oracle. Alert-channel senders (webhook,
  Slack) gained the same dial-time guard.
- **Scopeless API tokens no longer mint silent admins (item 6).** `POST /admin/tokens` with
  `kind:"api"` and no `scopes` now defaults the token to `["read"]`; admin requires an
  explicit `scopes:["admin"]`. Legacy scopeless tokens keep their historical semantics
  (documented at `canWrite`).
- **Main-port `/ingest/beacon` validates batches (LOWER item).** The main-port route now
  enforces the identical `beacon-event.schema.json` rules as the dedicated beacon port via a
  shared exported validator (422 `SCHEMA_ERROR`); it previously accepted any decodable JSON.
- **Alert-rule specs are validated at the API boundary (item 5).** Create/update now 422 on
  unknown metrics, unknown operators/severities, non-positive or >7-day windows — the exact
  hostile payloads the external review landed with HTTP 201 (`operator:"banana"`,
  `window_s:-3600`, `severity:"apocalypse"`) are all rejected, with a canonical
  `alert.ValidateRuleSpec` + exported known-name lists as the single source of truth.
- **Loud warnings for silent-weak configurations.** Boot now WARNs when `/metrics` is served
  without `PULSE_METRICS_TOKEN` and when the meta store falls back to unsalted SHA-256 token
  hashing because `PULSE_SECRET_KEY` is unset.

### Fixed

- **License-tier node ladder inversion (REVIEW-EXT blocker 2).** Business `MaxNodes` was 5 —
  below Pro's 10. Now 50 (persona-consistent ladder Free 1 / Pro 10 / Business 50 /
  Enterprise unlimited; PRD §7.11's old table superseded, pricing sign-off still an operator
  item), with a regression test pinning ladder monotonicity.
- **Silent Free-tier downgrade on the README compose path (blocker 3).** The embedded default
  license pubkey was the dev key, so vendor-signed licenses failed verification unless
  `PULSE_LICENSE_PUBKEY` was set explicitly. The official vendor key is now the embedded
  default and ships uncommented in `.env.example` and as the `real-ams.yml` default — all
  documented install paths verify vendor keys out of the box.
- **Wildcard `node_down` rules actually fire (item 5).** A rule targeting ANY node was
  permanently inert (BUG-011 residual); a cross-tick node-eviction tracker now detects
  disappearing nodes.
- **Alerts hold state on reader errors (item 5).** A ClickHouse/reader failure during
  evaluation no longer locks alerts open or flaps them; the tick holds state, WARNs
  (rate-limited), and recovers cleanly.
- **`viewer_drop_pct` renamed to `viewer_count_floor`.** The metric always compared an
  absolute viewer count; the honest name is now canonical (old name accepted as a deprecated
  alias, no stored-rule migration), and the seeded default rule uses `threshold:1` instead of
  firing at 0 viewers.
- **SDK: unload beacons no longer lose auth (D-165 follow-up).** `fetch(keepalive:true)` is
  now the primary flush path (carries `X-Pulse-Ingest-Token`); `navigator.sendBeacon` — which
  cannot set headers and thus silently 401'd `session_end` events — is a last-resort fallback
  only.
- **Onboarding wizard no longer duplicates the source on Back → re-submit (D-165 follow-up).**
  Re-submitting after Back updates the already-created source instead of creating a second.
- **Backup sidecar startup race (ROADMAP §2.46).** The daemon now waits (bounded, 120 s) for
  ClickHouse before its first cycle, retries network-class backup failures with backoff
  instead of sleeping 24 h, never prunes retention on a failed cycle, and reports a failed
  cycle as FAILED. A host reboot no longer costs a day of ClickHouse backups.
- **`PULSE_BASE_URL` is wired (item 7).** Alert deep-links honor the documented env var
  (absolute http/https, trailing slash stripped) instead of hardcoding the listen address.
- **`deployment.sh`: `PULSE_EXTRA_COMPOSE` seam (S101).** One optional overlay can be
  appended AFTER the canonical set (append-only — cannot omit an overlay); used to run the
  isolated D-164 verification stack that proved step 6 live.

### Documentation

- Alerting runbook: `ingest_bitrate_floor` described as raw kbps (was a wrong 0.5-ratio
  claim); the three inverted `stream_offline threshold:0` examples fixed (they fired while
  the stream was HEALTHY); `viewer_count_floor` documented with the deprecated alias.
  `licensing-public.md` Business row 50 nodes, inversion notes removed. `admin-guide.md`
  `PULSE_BASE_URL` and embedded-vendor-key rows now match code. `deploy/MIGRATION.md`
  scrubbed of the real VPS IP/SSH user. `docker-compose.override.yml` admin-on-:80 comment
  rewritten as an explicit security warning. `license-activation.md` no longer claims the
  default pubkey only accepts dev/CI keys.

### Changed

- **Helm chart default image tag `0.1.0` → `0.4.1`** (goldens regenerated byte-identical to
  real renders); quickstart/README pins move to `0.4.1`.
- **CI: `web-e2e` and `csp-e2e` are hard merge gates (D-162).** Both Playwright jobs ran as
  advisory (`continue-on-error: true`) through their bake period; they are now required, and
  `main`'s branch protection requires all 13 CI contexts. The one genuinely flaky CSP spec
  (dashboard test racing a `pulse:auth:401` bounce caused by an unmocked boot-time license
  fetch) was fixed at the spec with a catch-all API mock rather than promoted flaky.

### Added

- **Report-artifact retention pruning (D-143).** Scheduled report files (CSV/PDF) are now
  auto-pruned once they age past `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` (default 90; set 0 or
  negative to keep forever). Previously artifacts accumulated indefinitely on the pulse-data
  volume with no cleanup. The prune is strictly bounded — it removes only regular files
  matching the generated `pulse-usage-*.{csv,pdf}` pattern inside the reports directory (never
  the SQLite metastore or secret-key file that share the volume), runs on every scheduler tick
  independent of schedule-listing outcome, and skips symlinks. Report artifacts are now also
  persisted on the volume in the **base** compose (not just the hardened overlay), so
  non-hardened deployments retain artifacts across restarts. (Closes the one confirmed
  follow-up from the D-142 security-posture review.)

### Fixed

- **Live dashboard WebSocket now actually connects (D-165).** The web client dialed
  `/live/ws`, but the server registers the socket only at `/api/v1/live/ws`; the SPA
  fallback answered the bare path with HTTP 200, the upgrade never happened, and the
  dashboard silently ran on the 5 s polling fallback in every environment since the
  feature shipped. The client now dials the API-prefixed path (and the Vite dev proxy
  forwards WebSocket upgrades on `/api`). The "Live" indicator can now genuinely engage.
- **Settings → Ingest Tokens snippet is now correct (D-165).** The copy-paste SDK snippet
  shown after creating an ingest token used a wrong package name (`@pulse/beacon-js` — the
  package is `@pulse/beacon`, named export), a config key the SDK does not accept
  (`endpoint:` — the real key is `ingestUrl`, and the SDK appends `/ingest/beacon` itself,
  so the old value would also have doubled the path), omitted the required `streamId`, and
  embedded `window.location.origin` as code that would evaluate on the *player's* page.
  The snippet now bakes in the concrete Pulse origin and all required fields. The same
  wrong snippet in `docs/user-guide.md` is fixed to match.
- **Fresh `npm ci` works again on Node 20 LTS (D-165).** `web/package.json` pinned
  `@eslint/js@^10` against `eslint@^9` — a major-version peer mismatch that stock
  npm 10.8 rejects outright (CI's Node 22 npm tolerated it; the Docker build masked it
  with `--legacy-peer-deps`). `@eslint/js` now matches the eslint major (`^9.39.4`);
  a clean `npm ci` + lint + full test run passes with no override flags.
- **`make test` / `make lint` no longer mask web and SDK failures (D-165).** Skeleton-era
  `|| (echo "…not yet populated"; exit 0)` guards on `test-web`, `test-sdk`, `lint-web`
  and `lint-sdk` swallowed real failures (the web suite alone is 680 tests); the guards
  are removed so local `make test` fails when the suites fail, matching CI.
- **`branch-protection.sh` restores the real gate set (D-165).** The restore script still
  listed the 7 pre-D-162 contexts; re-running it would have silently dropped the
  `e2e`/`csp-e2e`/`web-e2e`/`sdk-swift` and CodeQL requirements. It now applies the full
  13-context list currently enforced on `main`.
- **Alert engine no longer leaks in-memory state (D-160).** The alert evaluator's per-rule,
  per-stream firing-state map was never pruned, so on a long-running server with high stream
  churn it grew without bound (one small entry per unique stream that ever matched a rule).
  Settled entries whose stream has gone and whose cooldown has lapsed are now evicted each tick;
  fire/resolve/cooldown behaviour is unchanged (an entry still accumulating toward a re-fire, or
  still firing, is never evicted). Internal robustness; no configuration or API change.
- **Wildcard "Stream offline" alert survives a brief disable / maintenance window (D-159).**
  A wildcard `stream_offline` critical alert whose watched stream goes offline and then has its
  rule briefly disabled (or enters a maintenance window) before the alert window elapses — then
  is re-enabled while the stream stays offline — now still pages, and an already-firing such
  alert now still auto-resolves at its hold. Previously the rule's edge-detection tracker was
  discarded on suspend, and the freshly-rebuilt tracker could not re-detect a stream that was
  already gone, so the offline event was silently dropped (or the alert stuck "firing" forever).
  The tracker now preserves in-flight offline state across a suspend while still preventing
  spurious edges on resume. Only affects wildcard offline rules under a disable/maintenance
  transition; the default rule ships muted.
- **Wildcard "Stream offline" hold is immune to a mid-event window change (D-159).** Shrinking a
  wildcard `stream_offline` rule's `window_s` while a stream is already offline no longer
  retroactively expires the in-flight offline event (which previously swallowed its page). The
  hold deadline is now frozen when the stream is first detected offline.
- **Load-testing lane isolation hardening (D-159).** The opt-in load lane's forbidden-host guard
  now also rejects the production VPS by raw IP (not just by hostname), and the
  `publisher.sh` / `viewer-sim.sh` / `failures.sh` harness bootstrap scripts now abort with a
  clear error when `AMS_URL` is unset instead of silently falling back to the shared/prod env —
  closing a footgun where a load test sourced without its dedicated env could target prod. QA
  tooling only; no runtime change.
- **Source connectivity-test error detail (D-151).** The Settings → "Add source" connectivity
  test (`POST /admin/sources/{id}/test`) now shows the real failure reason (no REST URL, an
  invalid URL scheme, or the network error) instead of a generic "Source unreachable". The
  server returned the detail under a `message` key, but the `AmsSourceStatus` API contract and
  the web UI both use `error`, so the detail was always discarded. On a successful test `error`
  is now correctly `null`, per the contract.
- **Analytics stream filter (D-151).** The audience / geo / devices analytics calls and the
  audience CSV export now send the per-stream filter under the `stream` query parameter the
  server and OpenAPI spec expect (they previously sent `stream_id`, which the server ignored —
  silently returning data for all streams). No UI currently passes a stream filter to these, so
  this is a latent-bug fix with no change to today's behaviour.
- **`make mock-ams` (D-151).** The developer target built the mock-AMS binary from the repo
  root, which has no `go.mod`, so it failed unconditionally; it now builds inside `qa/mock-ams`
  (its own module), matching the CI job.

### Documentation

- **Release-readiness accuracy sweep (D-165).** An independent full-repo review verified
  the documentation against code and fixed every confirmed drift: the alerting runbook's
  default-rule table was wrong on all four rows (docs said `stream_offline eq 0` — which
  matches the *online* state and would fire while a stream is healthy; actual seeded rule
  is `eq 1`; likewise `viewer_drop lt 0.5` not `lt 20`, `node_cpu gt 90`/120 s not
  `gt 80`/60 s, ingest floor absolute 500 kbps not "50 % of target") and its copy-paste
  example created the inverted rule — both corrected, a worked `POST /alerts/rules`
  example added, and `viewer_drop_pct`'s real semantics (absolute viewer count, not a
  drop percentage) documented honestly. Also fixed: the bootstrap-token log line shown in
  the user guide and API guide did not match what the server prints (users grepping the
  documented string would never find their only admin credential); four stale admin-guide
  notes claiming the corrected S3 variable names were wrong; three stale claims that the
  main-port beacon rate limit was an open backlog item (it shipped as A2); the user
  guide's CSV export URL (`/analytics/export` does not exist; real:
  `/analytics/audience?format=csv`); all 14 user-guide screenshot paths (`../marketplace/…`
  resolved outside the repo); `WRONG_TOKEN_KIND` documented as 401 (it is 403); a
  prominent notice that YAML config is not operative in v0.4.x; removal of a 213-line
  internal AI-session prompt from `docs/AMS-INTEGRATION.md`; post-nginx-cutover compose
  references in `docker-compose.real-ams.yml`, the monitoring/productionize/
  upgrade-rollback runbooks and compose-file comments; the quickstart token grep now
  anchors on the FIRST-RUN line; `pulse.Dockerfile` EXPOSEs the webhook port 8092; the
  listing draft's obsolete 20–30 % revenue-share figure and stale conformance claim; and
  `CLAUDE.md`'s "skeleton only" state description.
- **Logtail references removed (D-151).** The `logtail` collector was deleted in D-062, but a
  few docs still listed it as shipped/configurable: `docs/ARCHITECTURE.md` (component diagram +
  status table), `docs/AMS-INTEGRATION.md` (the `PULSE_LOG_TAIL_PATH` env-var row), and
  `README.md`'s architecture diagram are now corrected.

### Security

- **Production container hardening + supply-chain sweep (D-142).** A cross-cutting
  security-posture pass (the first non-subsystem audit). **(1) Container hardening** —
  the internet-facing `pulse` service (which parses untrusted beacon + webhook input)
  now runs with a **read-only root filesystem** (`read_only: true` + a `/tmp` tmpfs),
  **all Linux capabilities dropped** (`cap_drop: [ALL]` — the static `CGO_ENABLED=0`
  binary binds only high ports 8090-8092 and needs none), and **`no-new-privileges`**
  to block setuid escalation, layered on the already-non-root `USER pulse` image. A
  latent bug surfaced by the hardening is also fixed: report artifacts were written to
  the **ephemeral container root** (the relative `pulse-reports` default under WORKDIR
  `/`), so scheduled-report output was lost on every redeploy; `PULSE_REPORTS_DIR` now
  points at the persistent `/var/lib/pulse` volume. **(2) Dependency vulnerabilities** —
  `govulncheck` reports **0 reachable** Go vulnerabilities (one module-only
  `x/crypto/openpgp` advisory has no fix and is not imported). Three `npm audit` findings
  (a HIGH `undici`, two moderate `js-yaml`) were all **dev-toolchain-only** (test env /
  OpenAPI codegen — never in the shipped browser bundle) and are now pinned to patched
  in-major versions via `overrides` (`undici@7.28.0`, `js-yaml@^4.3.0`) → **`npm audit`
  clean**. Verified in production (read-only recreate, 0 EROFS/permission errors) and by
  an adversarial review (4 of 5 findings refuted; the 1 confirmed is a pre-existing,
  LOW report-artifact retention gap tracked as a follow-up).
- **Synthetic-probe URL SSRF guard (D-130).** Operator-stored probe URLs are fetched by the
  prober from inside the server's trust boundary. Previously a URL was accepted with no scheme
  or host validation, so an admin-scoped token could point a probe at the cloud instance-metadata
  endpoint (`http://169.254.169.254/…` → IAM-credential escalation), other link-local/unspecified
  addresses, or a non-HTTP scheme (`file://`, `gopher://`), and read reachability/TTFB back via
  probe results. A new `ssrfguard` policy now (a) rejects disallowed schemes at the API boundary
  (allowlist: http, https, ws, wss, rtmp, rtmps) → **422**, and (b) refuses, at *dial time* on the
  DNS-resolved IP, any connection to link-local (incl. IMDSv4 `169.254.169.254` and NAT64-embedded
  forms), IMDSv6 `fd00:ec2::254`, or the unspecified address — across every prober dial path (HLS/
  DASH/reachability HTTP client, RTMP, WebRTC signaling), DNS-rebinding-safe and re-checked per HTTP
  redirect hop, with `HTTP(S)_PROXY` disabled so a proxy cannot dial the destination behind the
  guard. **Loopback and private RFC-1918/ULA addresses remain allowed** — self-hosted AMS nodes are
  routinely on internal networks (consistent with the B4/A6 AMS-source-test ruling). (Found by the
  S62 subsystem audit, finding [21].)
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

- **Synthetic RTMP probe hardened against hostile servers (D-128).** The RTMP probe
  reassembles chunks from the monitored server, which may be untrusted. Two
  memory-exhaustion vectors are closed: the chunk demuxer now caps the number of
  distinct chunk streams it tracks (an attacker could otherwise open all 65,536 and
  buffer ~4 GB), and it no longer makes a throwaway per-message copy of
  silently-ignored control messages (which a hostile server could stream at 64 KiB
  each to drive sustained allocation). (Found by the S62 subsystem audit, finding
  [13]; the second vector was surfaced by an adversarial review of the fix.)
- **Synthetic DASH probe hardened against hostile manifests (D-127).** The DASH
  probe fetches and parses an MPD manifest from the monitored server, which may be
  untrusted. Three memory-exhaustion vectors are closed, any of which a single
  crafted manifest could use to OOM the prober: (1) the manifest body is now read
  through a 16 MiB limit before XML decode (the media-segment read was already
  capped); (2) the `$Number%…$` segment-template width is validated against a
  bounded `%0<width>d` allowlist, so a hostile `%999999999d` can no longer make the
  formatter allocate ~1 GB; (3) the `$RepresentationID$` template substitution is
  now size-bounded before expansion. (Found by the S62 subsystem audit, findings
  [3]/[4]; the third vector was surfaced by an adversarial review of the fix.)

### Fixed

- **Live dashboard no longer puts the API token in the WebSocket URL (D-140).** The Live view's WebSocket
  connection passed the bearer token as a `?token=` URL parameter, which reverse proxies record in their
  access logs — exposing a long-lived, replayable admin credential to anyone who can read the logs. The
  token now travels in the WebSocket handshake header (`Sec-WebSocket-Protocol`) instead, so it stays out
  of URL-based access logs. OIDC cookie sessions are unaffected. (Found by the S73 subsystem audit,
  finding [7]; if you have an operator token that was previously used with the Live dashboard, rotating it
  is a reasonable precaution.)
- **Settings page now reports failed actions (D-139).** Removing a source, revoking/creating an API or
  ingest token in the web Settings page previously showed no feedback at all when the request failed
  (e.g. server error) — the action silently did nothing and a user could unknowingly retry it. These
  handlers now surface an error toast on failure, matching the rest of the page. (Found by the S73
  subsystem audit, finding [8].)
- **Alert-history pruning race on Postgres (D-138).** The per-rule alert-history cap was enforced with a
  non-transactional count-then-delete, so under concurrent alert firing on a Postgres backend two prunes
  could together delete below the cap and permanently drop history rows. Pruning is now a single
  self-contained statement (keep the newest N per rule), eliminating the race. SQLite was unaffected.
  (Found by the S73 subsystem audit, finding [4].)
- **Multi-tenant isolation for ingest-health metrics (D-137).** The `GET /qoe/ingest` publisher
  ingest-health query was missing the tenant filter its sibling analytics queries all apply, so in a
  multi-tenant deployment where two tenants used the same app + stream name, the bitrate/fps/packet-loss
  figures were **blended across tenants**. The query is now tenant-scoped like its siblings (and the
  `tenant` query parameter is documented). Single-tenant deployments are unaffected. (Found by the S73
  subsystem audit, finding [1].)
- **Graceful shutdown, boolean env-vars, and diagnostic redaction (D-136).** Three fixes from the S73
  subsystem audit: (1) on `SIGTERM` the HTTP server is now **gracefully drained** (in-flight requests
  finish, WebSocket and rate-limiter background goroutines stop) instead of being killed abruptly —
  important for zero-downtime rolling deploys; (2) boolean environment toggles like `PULSE_ANONYMIZE_IP`
  and `PULSE_WEBHOOK_REQUIRE_TIMESTAMP` now accept the common `1` / `True` forms (and tolerate surrounding
  whitespace such as a Kubernetes-secret trailing newline), so an IP-anonymization/privacy control set via
  the Docker `1` idiom is no longer silently ignored; (3) `pulse diag` no longer prints AMS-URL credentials
  in the clear (the URL is credential-redacted, matching the running server's logs). (Found by the S73
  subsystem audit, findings [2], [3], and [6].)
- **TLS cert-expiry alerts + WebRTC probe timer (D-134).** Two fixes from the S62 subsystem audit
  (which this closes): (1) an alert rule watching for an **already-expired** TLS certificate
  (`cert_expiry lt 0`) never fired — an expired cert fails the TLS handshake, and the checker treated
  that as a generic error and skipped it; the checker now recognizes an expiry-specific verification
  failure and reports it so the rule fires (certificate verification remains enabled — a self-signed or
  internal-CA endpoint is a documented limitation, not silently trusted); (2) a WebRTC synthetic probe
  leaked a runtime timer for up to the stats-hold duration when its context was cancelled mid-hold — the
  timer is now stopped promptly. (Found by the S62 subsystem audit, findings [22] and [25].)
- **License manager — error visibility, tier validation, diagnostics (D-133).** Three fixes from
  the S62 subsystem audit: (1) when a configured license key is **rejected** (bad signature, malformed,
  unreadable offline file), the server now logs a warning and degrades to Free tier — previously the
  error was silently discarded, so an operator could not tell a rejected key from an unconfigured one;
  (2) an **unrecognized tier** in a (validly-signed) key is now rejected and degraded to Free instead of
  being trusted as a paid tier with unlimited capacity — the probe and beacon-ingest entitlement gates
  were tightened to match the other feature gates; (3) a misleading diagnostic when a malformed
  `PULSE_LICENSE_PUBKEY` triggered the dev-mode key fallback now reports the real underlying error. All
  internal robustness fixes — nothing to configure. (Found by the S62 subsystem audit, findings [12],
  [23], and [24].)
- **Anomaly flag detector — hysteresis + scope-key correctness (D-132).** Three fixes in the
  anomaly detector's flag path, from the S62 subsystem audit: (1) an `GET /anomalies` HTTP read
  (`ComputeFlags`) could arm the shared hysteresis cooldown and make the next detection tick skip
  writing the flag event, dropping the anomaly from the ClickHouse audit trail — the read path no
  longer arms the cooldown (it is now a true point-in-time snapshot that reports an active anomaly
  on every poll; the persist/tick path remains the sole writer, per ADR-0009 §4); (2) the cooldown
  suppressed one tick fewer than the documented `HysteresisTicks` (the decrement ran before
  detection) — a fired flag now suppresses exactly `HysteresisTicks` ticks, and the restart-dedup
  path (`WarmHysteresis`) was made consistent so a restart no longer re-fires early with a duplicate
  event; (3) the baseline scope key was built by unescaped string concatenation, so a stream/node ID
  containing a `"` corrupted the key and mis-attributed anomaly events to the wrong stream — IDs are
  now JSON-escaped (and parsed back with a real JSON decode), with normal IDs kept byte-identical so
  baselines are not reset on upgrade. The alert evaluator's scope-key builder now delegates to the
  same canonical function so its baseline lookups can't silently diverge. (Found by the S62 subsystem
  audit, findings [16], [17], and [18].)
- **HLS synthetic-probe manifest parsing (D-131).** Two correctness fixes in the HLS
  probe, which parses an untrusted manifest served by the monitored AMS/CDN: (1) a media
  segment preceded by a zero-duration or malformed `#EXTINF` was silently dropped and the
  playlist misreported as an empty master, so the probe returned "healthy" (`Success=true`,
  bitrate 0) **without ever fetching the segment** — it now captures and fetches the segment
  (bitrate is still only computed when the duration is > 0, so no divide-by-zero), turning a
  broken segment into an honest error; (2) segment/variant URIs are now resolved with RFC-3986
  reference resolution, so protocol-relative (`//cdn/seg.ts`) and absolute-path (`/seg.ts`)
  references resolve to the correct host instead of being concatenated onto the base path and
  misdirecting the fetch. A segment URI carrying a non-HTTP scheme in a hostile manifest is now
  classified as a `parse` error rather than `network`. (Found by the S62 subsystem audit,
  findings [14] and [15].)
- **Alert evaluator correctness — three metric-evaluator fixes (D-129).** From the
  S62 subsystem audit: (1) **node CPU/mem/disk threshold rules no longer false-fire
  on nodes that don't report that field** — a standalone AMS 3.x node omits cpu/mem/disk,
  which was read as a real `0` and tripped `lt`-style thresholds; the evaluator now skips
  the comparison for an unreported field (and still resolves a firing alert if a node
  stops reporting). (2) **`stream_offline` alerts now carry the correct value and honor
  the rule's operator/threshold** — the notification `value` was hardcoded to `0` even
  when firing (now `1.0` offline / `0.0` online) and the configured operator/threshold
  were ignored. **Behavior change:** a `stream_offline` rule now evaluates its operator
  like every other metric; the default/seeded `eq 1` rule is unaffected, but a hand-crafted
  non-canonical operator (e.g. `lt 1`) that previously fired-on-offline-regardless now
  follows its literal predicate — use `eq 1` (or `gt 0`) to fire when a stream goes offline.
  (3) **a `license_expiry` alert now resolves when the licence is renewed to perpetual** —
  it previously stayed stuck in `firing` forever. (Found by the S62 subsystem audit,
  findings [7]/[8]/[9]; an adversarial review of the fix caught and corrected a stuck-firing
  regression and a float32-range value overflow before merge.)
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

## [0.4.0] - 2026-07-13

Operator-approved release ("rollout quick → marketplace ASAP", D-089) carrying
the changes recorded as D-076 … D-089 in the decision log (including two
post-v0.3.0 hotfixes, D-076/D-076b). Delivers ten API-correctness fixes
surfaced by the Pulse × AMS real-validation program, a persistent anomaly
flag-event store, the AMS API early-warning ladder, VoD recording billing, and
marketplace-readiness infrastructure (one-command install, trial-license
lifecycle, compatibility and known-limitations docs). Also ships a light-theme,
density-mode, and reduced-motion web UI refresh.

### Added

- **AMS API early-warning ladder — `ams_api_latency_ms` anomaly metric (D-087).**
  The REST poller now measures its round-trip to each AMS node and surfaces
  `ams_api_latency_ms` as a Welford-baselined anomaly metric. The value is
  correctly absent (never zero) when a poll fails, so it does not false-arm on
  nodes that do not report it. Alert rules can reference it alongside
  `cpu_pct`/`mem_pct`/`disk_pct`/`viewer_count`/`ingest_bitrate_kbps`. Together
  with the API error-streak gate in `node_degraded`, this gives three rungs of
  early warning before a `node_down` event.
- **`node_down` alerts now fire — BUG-011 fixed (D-087).** The goroutine that
  evicts stale nodes and fires `node_down` rules was implemented but never
  started (`wireNodeEviction` was absent from `serve.go`). No deployment has
  ever fired a `node_down` alert. The goroutine is now wired with a 3 × PollInterval
  eviction threshold; a node that fails the poller for that window is evicted and
  triggers any matching `node_down` rule.
- **Persistent anomaly flag-event history (D-086).** `GET /anomalies` now accepts
  `from`/`to` and `cursor`/`limit` and returns from a ClickHouse-backed
  `anomaly_flag_events` store, so anomaly spikes are retained across restarts and
  queryable over any window. Previously the endpoint computed flags point-in-time
  and returned nothing meaningful for any time range. CH migration 0010 applies
  automatically on upgrade.
- **VoD recording billing — BUG-002 fixed (D-085).** `recording_gb` is now
  populated by polling the AMS VoD REST API on every 12th collector tick, with an
  immediate backfill on startup. Previously it was always 0: the only fill path was
  the `vodReady` webhook, which AMS 3.x sends unsigned and Pulse silently rejected.
  A `vod_poll_state` meta table (migration 0003) deduplicates on `(app, vod_id)`
  so a restart does not double-bill an existing VoD; CH migration 0009
  (`mv_recording_1d`) wires the events into `rollup_usage_1d`. Live-validated:
  `recording_gb = 0.003126` at 0.02% reconciliation vs AMS.
- **One-command quickstart install (D-089).** New `deploy/quickstart/` with a
  minimal compose file, six-variable `.env.example`, and `install.sh` (healthz-gated,
  no-TTY safe). Migrations are now baked into the Docker image at
  `/usr/share/pulse/migrations` (via `PULSE_MIGRATIONS_DIR`), so `pulse-migrate`
  finds them without a bind mount. `docs/runbooks/install.md` documents the Path A0
  install flow. Requires image ≥ 0.4.0 (prior images lack baked migrations).
- **Trial-license lifecycle + web banner (D-089).** The license manager now checks
  expiry on every reader call (Tier/Valid/all Check* methods): a trial that crosses
  `expires_at` mid-run degrades gracefully to free-tier entitlements and logs a
  single warning, with no restart required. Boot with an already-expired key
  produces the same honest `{tier:free, valid:false, expires_at retained}` state
  instead of silently discarding the error. A dismissable warning banner appears in
  the web UI when ≤14 days remain; a non-dismissable error banner appears when the
  trial has expired. The `LicenseInfo` API shape is unchanged.
- **ProbesPage — WebRTC metric columns (D-077).** The probe results table now shows
  `ice_state` (badge: connected/failed/timeout), `rtt_ms`, `jitter_ms`, and
  `loss_pct`. Absent values display as a dash, not zero; `loss_pct = 0.0%` is
  correctly distinguished from absent.
- **Light theme, density modes, and reduced-motion support (D-077).** The web UI
  applies the brandkit light theme when the OS or user selects light mode
  (`[data-theme=light]`, all 15 `tokens.json` color.light values). Two density
  modes — compact and wall — are persisted to localStorage and toggled via a
  sidebar control. `prefers-reduced-motion` collapses motion tokens throughout.
- **New documentation (D-081, D-089):** `docs/beacon-sdk.md` — complete SDK
  integration guide (12 sections, every API name cross-checked against source).
  `docs/compatibility.md` — AMS version and browser compatibility matrix.
  `docs/known-limitations.md` — 18 documented limitations.

### Fixed

- **`/qoe/ingest` ignores `from`/`to`/`app`/`stream`/`node` filter params —
  BUG-004 (D-082).** The ingest-health endpoint declared these parameters in the
  OpenAPI spec but discarded them server-side, returning the all-time, all-streams
  dataset on every call. The web client sends `from=now-15min&to=now` on every
  Ingest-page load, so every prod deployment was serving era-mixed all-time
  buckets. Parameters are now honored.
- **`/qoe/ingest` ignores `interval` bucket-size param — BUG-005 (D-083).** The
  bucket granularity was silently fixed at 60 s regardless of the declared
  `interval` parameter. The parameter now maps to the bucket query
  (hour → 3 600 s, day → 86 400 s, absent → 60 s kept as the fine-grain default
  for the F4 15 s visibility target).
- **Probe scheduler emits duplicate result rows every 60 s — BUG-003 (D-082).**
  The 60 s config-refresh loop cancelled and immediately respawned every probe on
  every tick even when the configuration was unchanged; the respawned goroutine
  fired at zero jitter, producing a second result row 0–1 ms after the first.
  The scheduler now tracks configuration per probe and only respawns when it
  changes. Phase was also being silently reset on every refresh, so probe timing
  was never truly periodic; this is fixed as a side-effect of the same mechanism
  change.
- **List endpoints ignore `limit` + `cursor` pagination — BUG-006/007/010
  (D-084).** Eight list endpoints declared `limit`/`cursor` in the OpenAPI spec
  but applied neither, always returning the full set. Keyset cursors and `limit`
  are now threaded through: `/alerts/rules`, `/alerts/history`, `/admin/users`,
  `/admin/tokens`, `/admin/sources`, `/probes`, `/probes/{id}/results`, and
  `/analytics/audience`. `GET /live/streams` now has a stability sort so cursor
  pages are deterministic (non-deterministic map iteration previously caused
  duplicate and dropped items across pages — BUG-009 partial, D-084).
  `GET /analytics/audience?format=csv` is now declared in the OpenAPI spec
  (BUG-010) to match the pre-existing implementation.
- **Two server panics under pagination (D-084).** A stale or malformed cursor on
  `GET /api/v1/live/streams` could produce a slice-OOB panic. `?limit=-1` on
  `GET /alerts/history` bypassed the zero guard and returned HTTP 500 via the
  chi Recoverer. Both are now clamped.
- **`node_degraded` status inconsistent across fleet page and live overview
  (D-088).** Three independent copies of the degraded predicate
  (`CPUPCT > 90 || MemPCT > 90 || ConsecAPIErrors ≥ 3`) had drifted: the fleet
  page omitted the `MemPCT` arm; the live overview omitted `ConsecAPIErrors`. All
  three now call a shared `LiveNodeStats.Degraded()` method.
- **Standalone nodes form poisoned zero-mean anomaly baselines (D-088).** AMS 3.x
  standalone nodes do not report cpu/mem/disk via REST; the absent field was
  recorded as `0`, building a `mean=0, stddev=0` baseline that guarantees an
  instant false alarm on the first real reading. Presence flags now gate baseline
  accumulation for these three metrics. A startup sweep purges any existing
  poisoned rows (logged as `anomaly: purged zero-mean baselines on startup
  count=N`); live-proved: prod purged 3 rows (cpu/mem n=8 813, disk n=3 578) on
  the D-089 rollout boot.
- **Web assets returned as `text/html` (D-076b).** `favicon.svg`, `icons/*`,
  `logo/*`, and `site.webmanifest` were caught by the SPA catch-all handler and
  served with `Content-Type: text/html`, breaking the browser tab icon and web
  manifest. The static-file handler now checks for these paths before falling
  through to the SPA; directory traversal is guarded.
- **AuthGate silently authenticates on a non-JSON `/auth/me` 200 (D-077).** A
  stale SPA fallback or misconfigured reverse proxy that returns `200 + index.html`
  for `/auth/me` was treated as a successful authentication (`.ok` was true),
  leaving the user on a broken dashboard with 401-ing API calls. The gate now
  requires `Content-Type: application/json` and validates the response shape before
  granting access.
- **Prod compose overlay never passed license env vars (D-076).** The `real-ams`
  compose overlay listed `PULSE_LICENSE_KEY` and `PULSE_LICENSE_PUBKEY` in a
  comment block only; `docker compose --env-file` interpolates but does not inject
  commented-out variables. Every prod deployment from that overlay ran in silent
  Free tier even with a key present in `deploy/.env`. The overlay now passes both
  variables explicitly.

### Documentation

- **AMS-INTEGRATION.md expanded (D-081).** Two sections added: (1) webhook
  downstream impact and the D-V2-1 unsigned-webhook workaround path; (2) implicit
  broadcast deletion — AMS 3.x removes RTMP broadcasts from the REST API on stop
  rather than marking them `finished`/`terminated_unexpectedly`, which affects
  stream-end webhook delivery and the `recording_gb` fill path.
- **Real-AMS validation harness (D-079/D-080).** `qa/realams/` adds reusable
  parity-helper scripts, 26 P0 and 24 P1 scenario scripts against a live AMS
  instance, and `make validate-realams-p0` / `make validate-realams-p1` targets.
  P0 final result vs production AMS 3.0.3 Enterprise: 24 PASS / 2 SKIP / 0 FAIL;
  stream publish→Pulse-visible in 4 s, stop→removed in 7 s (PRD ≤10 s budget).

### Database

- **Migration 0003** (`vod_poll_state` SQLite + Postgres meta table): tracks seen
  VoD IDs on `(app, vod_id)` for restart-safe recording deduplication. Applies
  automatically via `pulse-migrate` on upgrade; idempotent.
- **Migration 0009** (`mv_recording_1d` ClickHouse materialized view): wires
  recording-event bytes into `rollup_usage_1d` so `recording_gb` appears in usage
  reports and reconciliation. Applies automatically; idempotent.
- **Migration 0010** (`anomaly_flag_events` ClickHouse MergeTree table,
  `ORDER BY (detected_at, metric, scope)`, TTL = `{retention_days}` days): backs
  the persistent anomaly flag history and `GET /anomalies?from=…&to=…` queries.
  Applies automatically; idempotent.

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
  of hard-coded IP; compose default `${AMS_UPSTREAM:-<your-ams-host>:5080}` (D-062).

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
