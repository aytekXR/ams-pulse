# Pulse (ams-pulse) — Ant Media Marketplace Compliance & Readiness Review

> Provenance: third-party external review supplied by the operator 2026-07-26 (review dated
> 2026-07-25, snapshot: working tree at v0.4.1 + unreleased S105/D-172 changes). Recorded
> verbatim below; acted on in SESSION-106. Independent of, and building on,
> `marketplace-readiness-review-2026-07-25.md` (REVIEW-MP-2026-07-25, 12 issues → D-172).

**Date:** 2026-07-25 · **Repo state reviewed:** working tree at v0.4.1 + unreleased S105/D-172 changes · **Reviewer scope:** full codebase (server, web, SDKs, contracts, deploy, qa, docs) cross-checked against official Ant Media Server sources (docs.antmedia.io, antmedia.io/marketplace, ant-media/Ant-Media-Server source at tag `ams-v3.0.3` and `master`).

This review is independent of, and builds on, the repo's own `docs/assessment/marketplace-readiness-review-2026-07-25.md` (12 issues, which drove the S105 fixes now sitting in `[Unreleased]`). Findings below are either **new**, or **confirmations of previously open questions**, or **residual gaps** that survived that review. Every claim was verified directly against code or an official source; file:line references are to the current working tree.

---

## 1. Verdict up front

**NOT READY to submit today — but close.** The application itself is well-engineered, correctly integrated with AMS for its primary (standalone, REST-polling) path, and the documentation pack is unusually complete. Three things gate submission:

1. **The released artifact doesn't match the code or the docs.** The S105 integration fixes (Kafka topics, webhook payload, ProxyAuthorization header) landed *after* the `v0.4.1` tag. The image evaluators are told to pull (`ghcr.io/aytekxr/ams-pulse:0.4.1`) still contains the broken behavior your own docs now describe as fixed. **v0.4.2 must be cut first; everything else follows.**
2. **Two of the three documented install paths are defective** (base Compose path cannot authenticate to ClickHouse; the Helm chart has a dead beacon port and an unschedulable Postgres configuration). Only the quickstart path is verified end-to-end.
3. **One integration bug is now confirmed, not just suspected:** AMS 3.0.3 has no flat `/rest/v2/cluster/nodes` endpoint — it is paginated — so cluster fleet discovery (F7, claimed in the listing) silently degrades to "standalone" against a real cluster.

None of these are architectural. The architecture (external read-only sidecar, own storage, zero AMS modification) is sound and has a listable precedent on the marketplace.

---

## 2. What the application is (derived from the implementation)

**Pulse** is a self-hosted observability suite for Ant Media Server: real-time ops dashboard, historical audience analytics, player QoE telemetry, ingest-health monitoring, alerting (email/Slack/Telegram/PagerDuty/webhook), usage/billing reports (CSV/PDF/S3), cluster fleet view, Prometheus metrics, anomaly detection, and synthetic probes (HLS/DASH/WebRTC/RTMP).

**Architecture (from code, not docs):**

- **One Go binary** (`server/cmd/pulse`, subcommands `serve`/`migrate`/`diag`) containing: AMS REST poller, optional Kafka consumer, optional webhook listener, beacon ingest, session stitcher, cluster discovery, alert evaluator, report scheduler, probe runner, anomaly detector, query API, and the embedded React UI. Ports: **:8090** (UI + API + `/healthz` + `/metrics` + `/ingest/beacon` + WS), **:8091** (dedicated beacon ingest, only when `PULSE_INGEST_LISTEN_ADDR` set), **:8092** (webhook listener, only when secret set).
- **Two stores:** ClickHouse (events + rollups, 90-day raw / 13-month rollups, 10 migrations) and SQLite-or-Postgres meta store (rules, tokens, users, schedules, 4 mirrored migrations). Contracts-first: OpenAPI 3.1 (42 paths/59 ops), 3 JSON-Schema event contracts.
- **Two MIT beacon SDKs** (`ams-pulse-beacon` TypeScript, 3.52 KB gz, with a real Ant Media `WebRTCAdaptor` adapter polling `getStats()`; plus a real Swift SDK). Both POST batches to `/ingest/beacon` with `X-Pulse-Ingest-Token`.
- **Licensing:** offline ed25519-signed keys, 4 tiers (Free 1 node / Pro 10 / Business 50 / Enterprise ∞), 403 `LICENSE_REQUIRED` gates, fail-open to Free, no phone-home.
- **Distribution:** public GHCR image (cosign-signed, SBOM + provenance, Trivy-gated, multi-arch), Docker Compose bundle, Helm chart, local binary.

**AMS integration is confined to two packages by architecture rule** (`server/pkg/amsclient` + `server/internal/collector`), is read-only in substance, and never modifies AMS state.

### AMS touchpoints (complete inventory, verified against official sources)

| Pulse call | Official AMS surface | Verified against | Status |
|---|---|---|---|
| `POST /rest/v2/users/authenticate` (plaintext password, cookie jar) | Web-panel REST; server MD5-hashes submitted password (`doesUserExist(email, md5(pass)) \|\| md5(md5(pass))`) | `CommonRestService.java` (master) + Web Panel REST docs | ✅ correct (plaintext works) |
| `GET /rest/v2/applications` | `RestServiceV2.getApplications` | source (master) | ✅ |
| `GET /rest/v2/system-status` | `RestServiceV2.getSystemInfo` `@Path("/v2/system-status")` | source (master) | ✅ path exists |
| `GET /rest/v2/version` | `RestServiceV2.getVersion` | source (master) | ✅ |
| `GET /rest/v2/cluster/nodes` (flat) | **`ClusterRestServiceV2` exposes only `/v2/cluster/nodes/{offset}/{size}`** | source at tag `ams-v3.0.3` **and** master | ❌ **Issue A** |
| `GET /{app}/rest/v2/broadcasts/list/{offset}/{size}` | `BroadcastRestService @Path("/list/{offset}/{size}")` | source (master) + REST quickstart | ✅ |
| `GET /{app}/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100` | `@Path("/{stream_id}/webrtc-client-stats/{offset}/{size}")` | source (master) | ✅ |
| `GET /{app}/rest/v2/vods/list/{offset}/{size}` | `VoDRestService @Path("/list/{offset}/{size}")` | source (master) | ✅ |
| Auth headers: `Authorization: Bearer <jwt>` + `ProxyAuthorization: <jwt>` on every GET | App scope: `Authorization: Bearer {JWT}` (JWT REST filter guide, v3.0); management scope: `ProxyAuthorization` (Web Panel REST docs) | official docs | ✅ in main / ❌ **not in the 0.4.1 image** (Issue B) |
| Kafka consumer: topics `ams-instance-stats`, `ams-webrtc-stats`; AMS-side `server.kafka_brokers` in `red5.properties` | Identical topic + property names | Monitoring AMS with Grafana docs | ✅ in main / ❌ 0.4.1 image consumes `ams-server-events`, a topic AMS never publishes (Issue B) |
| Webhook listener: actions `liveStreamStarted`/`liveStreamEnded`/`vodReady`, `application/x-www-form-urlencoded` + JSON, keys `id`/`streamId` | Same action names; official payload is form-urlencoded with `id`, `action`, `streamName`, `category`, `vodName`, `vodId`; **no HMAC/signing exists in AMS** | Webhooks docs | ✅ in main / ❌ 0.4.1 image drops form-urlencoded (Issue B); error-event actions unmapped (Issue J) |
| Probes: `{AMS}/{app}/streams/{id}.m3u8`, `ws(s)://host/{app}/websocket?streamId=`, `rtmp://host:1935/{app}`, `.mpd` | Standard AMS play paths | qa/realams scenarios + live validation record | ✅ (DASH honestly marked unavailable on AMS 3.0.3) |

**Version currency:** the latest AMS release is **Community 3.0.3, May 5 2026** — still current as of this review (GitHub releases). Pulse's live-validated target (AMS 3.0.3 Enterprise, 46/50 scenarios) **is the latest supported version**, and the poller has no deprecated-endpoint usage. The "AMS 2.10/2.14/3.0.2 mock-profile only" caveat is honestly disclosed in `docs/compatibility.md`.

---

## 3. Marketplace context (what the official requirements actually are)

Verified 2026-07-25:

- The Ant Media Marketplace lists plugins (JAR), WAR applications installed via the AMS panel (e.g. TunaDesk), **and externally hosted integrations that are not installed into AMS at all** (e.g. Mobiotics OTT — "Contact us to enable integration"). **Pulse's external-sidecar model has a live listing precedent**, which de-risks assumption A1 in `docs/marketplace/submission-process.md`.
- **There is still no published submission checklist, artifact spec, review SLA, or qualification threshold** anywhere on antmedia.io or docs.antmedia.io — consistent with the repo's own research finding. The only public entry point is the "become a marketplace vendor" application on the marketplace page; your process runs through the direct Ant Media contact (per the recorded email thread), with qualification steps to be handed over at the developer meeting.
- Existing listings show **no consistent metadata schema** (Scribe: hero tagline, feature grid, 8-step install, 4 screenshots, Buy + Docs CTAs; TunaDesk: prerequisites + panel-install steps + support section; Mobiotics: description + features + contact-sales). Your prepared pack (title 43 chars, 240-char description, 6× 1920×1080 screenshots, logo kit, docs set) meets or exceeds anything currently visible on the marketplace.
- Consequence for this review: for integration correctness I cite AMS source/docs; for packaging/deployment I cite Docker/Compose/Helm semantics and your own published claims; for listing content, absent a published Ant Media spec, the standard applied is "claims must be true and externally verifiable" — the bar any reviewer applies. Items that depend on Ant Media's unpublished process are flagged as meeting questions, matching your A1–A10 ledger.
- One caveat: I could **not** independently locate a public statement of the "first-year 100% revenue / no commission" vendor terms; treat that as thread-sourced (A4) and get post-year-1 terms in writing, as `submission-process.md` already plans.

---

## 4. Issues

Ordered by severity. Format per issue: **why it's a problem → what it violates → exact fix**.

### ISSUE A (P0) — Cluster fleet discovery calls an endpoint that does not exist on AMS 3.x: `/rest/v2/cluster/nodes` is paginated, not flat

- **Where:** `server/pkg/amsclient/client.go:516` (`getJSON(ctx, "/rest/v2/cluster/nodes", …)`), called from both `server/internal/cluster/discovery.go:125` (30 s cadence) and `server/internal/collector/restpoller.go:241` (5 s cadence). The 404 is deliberately swallowed: `client.go:517-521` maps 404 → `(nil, nil)` = "standalone AMS".
- **Why it's a problem:** In AMS 3.0.3 — your primary, live-validated target — the console REST service registers **only** `@Path("/nodes/{offset}/{size}")` (plus `/node-count` and `DELETE /node/{id}`). I verified this in `ClusterRestServiceV2.java` at tag `ams-v3.0.3` **and** at `master`; there is no flat `/nodes` route. JAX-RS returns 404 for the flat path, which Pulse interprets as "standalone" — so against a real AMS cluster, F7 fleet view, origin/edge role mapping, and edge-viewer dedup silently collapse to a single synthetic node. No error is ever surfaced. This falsifies the listing claim "auto-discovers all AMS apps and cluster nodes" (`docs/marketplace/listing-draft.md:42-43`) and turns the known-open question G-21 (`docs/compatibility.md` §Panel-revamp, "flat vs paginated — still not settled") into a **confirmed defect**. Your own mock (`qa/mock-ams/main.go`, flat `/rest/v2/cluster/nodes` route) encodes the wrong shape, which is why CI never caught it — and cluster mode was never live-validated (LIM-10, `docs/known-limitations.md`).
- **What it violates:** the actual AMS REST surface (source of truth: `io/antmedia/console/rest/ClusterRestServiceV2.java`, tag `ams-v3.0.3`); your own architecture rule that `amsclient` mirrors real wire formats (`docs/AMS-INTEGRATION.md` §1); truth-in-listing for the F7 bullet.
- **Fix (exact):**
  1. In `client.go`, page the call the same way `ListBroadcastsPaged` does: `GET /rest/v2/cluster/nodes/{offset}/{size}` with pageSize 50–200 until a short page; keep the 404→standalone mapping for genuinely standalone nodes (404 will still occur there because the paginated route also 404s outside cluster mode — verify once against a non-clustered 3.0.3, since an empty list vs 404 distinction matters).
  2. Optionally probe `GET /rest/v2/cluster-mode-status` (`RestServiceV2.isInClusterMode`, verified present) first, which cleanly separates "not a cluster" from "cluster but call failed".
  3. Update `qa/mock-ams` to serve the paginated route (and keep the flat one returning 404 so the regression is representative), plus a fixture with >1 page to lock pagination.
  4. Until this ships and is validated against a real cluster, soften the listing bullet to "auto-discovers AMS applications; cluster fleet view validated on standalone, cluster validation pending" — or run the cluster validation before submitting. Delete the never-called `NodeInfo` (`client.go:528`, no production caller) or fix its path the same way.

### ISSUE B (P0) — The published 0.4.1 image does not contain the integration fixes your docs describe; v0.4.2 is uncut

- **Where:** `CHANGELOG.md:11-43` (`[Unreleased]`): Kafka topics corrected `ams-server-events` → `ams-instance-stats`/`ams-webrtc-stats` with official nested shapes; webhook accepts the official form-urlencoded payload and `id` key; `ProxyAuthorization` added so static-token auth reaches management-scope endpoints. `docs/operator-expected.md:5` states it plainly: "the published 0.4.1 image does not contain them, while the docs on main now describe the fixed behavior."
- **Why it's a problem:** every evaluator instruction (README:34, quickstart `install.sh:28`, compose defaults, Helm `values.yaml`) points at `ghcr.io/aytekxr/ams-pulse:0.4.1`. An Ant Media reviewer following your own docs gets: (a) Kafka source consuming a topic AMS never publishes — feature dead on arrival; (b) AMS webhooks (form-urlencoded per the official webhook docs) parsed as JSON-only and dropped with HTTP 200; (c) in `PULSE_AMS_AUTH_TOKEN`-only mode, `/rest/v2/applications`, `cluster/nodes`, `system-status`, `version` all failing because only `Authorization: Bearer` was sent while management scope reads `ProxyAuthorization` — i.e. app auto-discovery and fleet data silently empty. Docs/product mismatch is precisely the kind of thing a functional install review catches.
- **What it violates:** consistency between the submitted artifact and its documentation (universal review requirement; also your own release discipline — the new version-consistency guard in `release.yml` exists for exactly this class).
- **Fix (exact):** bump `VERSION`, `Chart.yaml appVersion`, sdk `package.json`, doc stamps → tag `v0.4.2` → let `release.yml` produce the signed image → update the pinned tag in `deploy/docker-compose.yml:29,48`, `deploy/quickstart/docker-compose.quickstart.yml:20,38`, `deploy/quickstart/install.sh:28`, `deploy/helm/pulse/values.yaml:14`, helm golden test values, README. Extend the release version-guard (`release.yml:83-124`) to cover those deploy-surface files — today it checks only VERSION/Chart/doc-stamps/sdk, so the eight hardcoded `0.4.1` strings can drift silently again.

### ISSUE C (P0) — The README's primary evaluator install path cannot connect to ClickHouse (code 516)

- **Where:** `deploy/docker-compose.yml:134-157` — the `clickhouse` service sets **no** `environment:` at all. Your own override documents the failure mode: "ClickHouse 24.8 image disables network access for the `default` user unless CLICKHOUSE_USER/PASSWORD are set — which made the remote clickhouse-go client fail with `code 516: Authentication failed` while the local healthcheck passed. SKIP_USER_SETUP=1 keeps the built-in permissive default" (`deploy/docker-compose.override.yml:2-9`).
- **Why it's a problem:** README's "recommended for evaluators" command is `docker compose -f deploy/docker-compose.yml up -d` (README:23-32). With an explicit `-f`, Compose does **not** auto-load `docker-compose.override.yml` — so the base file runs alone, with a pinned `clickhouse/clickhouse-server@sha256:1d1f65…` (24.8) and no `CLICKHOUSE_SKIP_USER_SETUP`/`CLICKHOUSE_USER`. Result: `pulse-migrate` fails auth → `pulse` never starts (`depends_on: service_completed_successfully`). Every other lane sets one of the two (override, prod, hardened, quickstart, ci) — only the base file, the exact path in the README, doesn't. CI never boots this combination (`ci.yml` runs `config --quiet` only), and the D-168 clean-room verification used the **quickstart** path, not this one.
- **What it violates:** "installation process complete and free of missing or incorrect steps" (your submission criterion); the documented behavior of the pinned ClickHouse image.
- **Fix (exact):** add to the base `clickhouse` service: `environment: { CLICKHOUSE_SKIP_USER_SETUP: "1" }` with the same safety comment as the override (ports are `expose:`-only, never published), **or** wire `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` from `.env` and build the DSN like `prod.yml:94` does. Then add a CI job that actually boots `-f deploy/docker-compose.yml` alone against the released image and asserts `/healthz` — the gap that let both this and the pre-S105 migrate/env bugs live in the "recommended" path.

### ISSUE D (P0) — Helm chart: beacon ingest port is dead, and `postgres.enabled=true` produces an unschedulable pod

- **Where:**
  - `deploy/helm/pulse/templates/deployment.yaml:63-69` declares `containerPort: 8091 (ingest)` and `service.yaml`/`ingress.yaml` route to it, but the env block (`deployment.yaml:78-194`) **never renders `PULSE_INGEST_LISTEN_ADDR`** — and `serve.go` only binds :8091 when that var is set (the compose file even documents this exact trap: `docker-compose.yml:56-59`). `values.yaml:47` (`ingestListenAddr`) is read by nothing. Confirmed absent in the committed golden `deploy/helm/tests/golden-default.yaml`.
  - `deployment.yaml:225-232` mounts PVC `<fullname>-data` whenever `pulse.persistence.enabled`, but `pvc.yaml:1` skips creating it when `postgres.enabled` — dangling claim, pod stuck `Pending`. The committed golden `golden-postgres-s3.yaml` proves it (references `pulse-data`, contains no PVC).
  - Also: `s3Export.enabled` renders only `PULSE_S3_EXPORT_KEY_ID/SECRET_KEY` (`deployment.yaml:176-189`) — env names the binary does not read (code reads `PULSE_S3_ENDPOINT/BUCKET/REGION` + `PULSE_S3_ACCESS_KEY_ENV`/`PULSE_S3_SECRET_KEY_ENV`, `server/cmd/pulse/config.go:381-386`), and never sets bucket/endpoint/region — S3 export cannot work from the chart. `.env.example:100-109` documents the corrected names; the chart never got the fix.
- **Why it's a problem:** the Helm path is advertised in the listing/docs set ("Install: Compose, Helm, binary"). A reviewer or customer enabling QoE beacons (a headline feature) on Kubernetes gets a Service and Ingress pointing at a port nothing listens on; enabling the documented HA meta store makes the deployment unschedulable; enabling S3 export silently does nothing. Golden tests lock in the broken output rather than catching it.
- **What it violates:** correctness of the documented deployment process; the chart's own README claims (values table, "production" posture).
- **Fix (exact):**
  1. In `deployment.yaml` env block add PULSE_INGEST_LISTEN_ADDR from `.Values.pulse.ingestListenAddr` (defaulting `:8091` in `values.yaml`), or gate port/Service/Ingress on the value being set.
  2. Change `pvc.yaml:1` to create the PVC whenever `pulse.persistence.enabled` (the volume still stores the license cache and reports even with Postgres), or in `deployment.yaml` fall back to `emptyDir` when `postgres.enabled`.
  3. Replace the S3 env block with the real names and add `PULSE_S3_ENDPOINT/BUCKET/REGION/PREFIX` values; while there, render `PULSE_REPORTS_DIR=/var/lib/pulse/reports` (the chart currently leaves reports in the ephemeral container root — the exact bug `docker-compose.yml:96-101` fixes for Compose).
  4. Regenerate all three golden files; add a golden variant with `clickhouse.persistence.enabled=false` (currently that path emits a duplicate `volumes:` key — `statefulset-clickhouse.yaml:90` vs `:111-116`).
  5. Fix `deploy/helm/pulse/README.md:73` (`tag: 0.1.0` → current) and bump `Chart.yaml version`.

### ISSUE E (P0) — Listing copy: the Free tier is advertised with no noncommercial disclosure, plus claims that won't survive review

- **Where/why:**
  - `docs/marketplace/listing-draft.md` contains **zero** mention of PolyForm/noncommercial (verified by search), and presents "Free $0" as a normal tier — while `docs/licensing-public.md:199-205` states commercial deployments may **not** use the Free tier. The marketplace audience is almost entirely commercial AMS operators; omitting the restriction from the listing is the single likeliest reviewer bounce, and arguably misleading advertising.
  - Tier-mixing: the 396-day retention window and Prometheus `/metrics` appear in a bullet marked "(Pro+)" though both are Business+ (`licensing-public.md:82,104`).
  - "±1% reconciliation" is sold for usage/billing while `docs/known-limitations.md` LIM-07 says `egress_gb` is a bitrate×watch-time heuristic "not for invoicing" — the demo script even narrates egress reconciliation.
  - "Auto-discovers … cluster nodes" (see Issue A); "Air-gapped licensing (roadmap)" contradicts `licensing-public.md:219-225` ("Yes, entirely offline").
  - **37 internal identifiers** (D-xxx, S-xx, BUG-xxx, TC-xx) plus source paths and repo-regeneration instructions sit inside the copy that would be pasted into the marketplace form; `release-notes.md` (customer-facing) carries 29 more; internal negotiating notes ("do not use [the 20-30%] figure in discussions") are embedded in the listing body.
  - Lab numbers presented as product specs ("201 ms detect→notify", "0.259 false alarms/node-week", "≤4 s visibility" from a single live test against a 10 s budget) invite challenge; keep them, but labeled as measured-in-CI/lab.
- **What it violates:** truth-in-listing/consumer-clarity norms every marketplace applies; your own separation of internal vs external docs (`listing-draft.md:1` says "Internal working copy" — but `licensing-public.md:225`, a customer-facing doc, links into it).
- **Fix (exact):** produce a clean external `listing.md`: add one line under the Free tier — "Free tier is for noncommercial use (PolyForm Noncommercial 1.0.0); commercial deployments start at Pro" — matching `licensing-public.md` §5; move `/metrics` + 396-day retention under Business+; change the reports bullet to "viewer-minutes reconciled to ±1% (CI-verified); egress is an estimate — use CDN logs for invoicing"; fix the cluster and air-gap bullets; strip every D-/S-/BUG-/TC- identifier, source path, and internal note; caveat lab numbers. Same pass over `release-notes.md`. Pick a category (analytics/monitoring — A10) and write the long-form description (only the 250-char short one exists today).

### ISSUE F (P1) — Business-tier license keys are still minted with `max_nodes: 5` (the inversion v0.4.1 claims to have fixed)

- **Where:** `qa/licensegen/main.go:69` (`maxNodes := 5` for `business`) vs `server/internal/license/license.go:135` (`MaxNodes: 50`) and `licensing-public.md:78` (50). `buildEntitlements` takes `MaxNodes` **from the key's claims**, not the tier table (`license.go:530-537`), so any Business key minted with this tool grants 5 nodes — *below Pro's 10*. `license_coverage_test.go:136` still asserts 5, masking the drift. CHANGELOG 0.4.1 explicitly claims this inversion was fixed (D-166) — it was fixed in the entitlement table, not the minting tool that produces real customer keys.
- **Why it's a problem:** the first paying Business customer ($299/mo) gets fewer nodes than a Pro customer; the listing's tier table becomes false at the moment of first sale; a marketplace trial reviewer minting a demo key hits it too.
- **What it violates:** your published tier ladder (listing + `licensing-public.md`), and the D-166 release claim.
- **Fix (exact):** `qa/licensegen/main.go:69` → `maxNodes := 50`; update `license_coverage_test.go:136` to assert 50; add a regression test that mints via `licensegen` and asserts server-side entitlements match the published ladder for every tier (the existing D-166 regression covers the table, not the mint path).

### ISSUE G (P1) — Quickstart wizard can destroy the operator's secrets on failure, and mis-detects "unhealthy" for wrong AMS credentials

- **Where:** `deploy/quickstart/install.sh:53-60` (`trap cleanup EXIT` deletes `$ENV_FILE` on any non-zero exit), `:196-198` (regenerates `PULSE_SECRET_KEY` unless present *in the environment* — never reads the existing `.env`), `:229` (health gate requires `"status":"ok"`), vs `server/internal/api/server.go:983-985` (`/healthz` reports `"degraded"` with HTTP 200 while the collector has no snapshot — i.e. whenever the AMS URL/credentials are wrong).
- **Why it's a problem:** three interacting defects on the marketplace's showcase path. (1) Wrong AMS URL → stack actually starts, but health never reaches "ok" → script exits 1 → **trap deletes the `.env` containing the freshly generated AES-GCM secret key while the containers and volumes keep running with data encrypted under it.** (2) A re-run generates a *new* key, so the persisted meta store's encrypted credentials become permanently undecryptable. (3) The failure is reported as a hard install failure with no AMS-specific diagnosis, though the actual condition is "Pulse up, AMS unreachable". Also: when fetched via `curl | bash`, the script downloads `docker-compose.quickstart.yml` from `main` while pinning image `0.4.1` — a skew that will bite at the first breaking compose change; and no CI job shellchecks or boots the quickstart.
- **What it violates:** install-path robustness (your own criterion "NEVER claim success without positive body evidence" is honored, but the failure handling destroys state); basic idempotency expectations for an installer.
- **Fix (exact):** (1) in `cleanup()`, delete the `.env` only if this run *created* it (`CREATED_ENV=1` flag) and the stack was never started, else print "kept `.env` (contains your secret key)". (2) Before generating a key, source an existing `$ENV_FILE` and reuse its `PULSE_SECRET_KEY`. (3) Accept `"status":"ok"` **or** `"degraded"` as "Pulse is up", then separately probe collector health (`/healthz` body `components.collector`) and print "Pulse running; AMS unreachable at $AMS_URL — check URL/credentials" instead of failing. (4) Download the compose file from the release tag (`raw.githubusercontent.com/…/v0.4.2/…`), not `main`. (5) Add a CI job: shellcheck + boot the quickstart against mock-ams.

### ISSUE H (P1) — Helm ClickHouse ships a passwordless `default` superuser in all configurations, contradicting the chart's stated production posture

- **Where:** `deploy/helm/pulse/templates/configmap-clickhouse.yaml:61-77` — the `{{ if .Values.clickhouse.auth.existingSecret }}` branch changes only XML *comments*; `<password></password>`, `<networks><ip>::/0</ip></networks>`, `<access_management>1</access_management>` are emitted unconditionally. `README.md:22-25` and NOTES.txt claim the secret-based setup secures the database.
- **Why it's a problem:** any pod in the cluster that can reach the ClickHouse Service connects as a passwordless admin (`access_management=1` = can create users/grants), even in the "production" configuration. This lands squarely in a marketplace security review, and it contradicts your own hardened-compose posture (prod compose refuses `CLICKHOUSE_SKIP_USER_SETUP`).
- **What it violates:** the chart's own security claims; parity with `docker-compose.prod.yml:148`; least-privilege defaults expected by any security reviewer.
- **Fix (exact):** when `existingSecret` is set, render the `default` user with localhost-only networks and `access_management` 0 (readiness probes run in-pod via localhost, so probes keep working), or drop the inline `users.xml` override entirely and let the image's env-based setup govern. Add a NetworkPolicy template restricting CH ingress to the pulse/backup pods as defense-in-depth. Update the golden files.

### ISSUE I (P1) — OpenAPI spec: `servers: /api/v1` breaks generated clients for the 8 root-mounted paths; two advertised auth mechanisms don't match the implementation

- **Where:** `contracts/openapi/pulse-api.yaml:38-40` (single `servers: url: /api/v1`) vs routes registered at server root (`server/internal/api/server.go:447-556`): `/healthz`, `/metrics`, `/ingest/beacon`, `/auth/me`, `/auth/oidc/{login,callback,logout,status}`. The YAML marks them `# non-versioned` in comments but never overrides `servers` per-path. Also: `wsTokenQuery` (token-in-query) is listed as an accepted scheme for `/live/ws` while the middleware deliberately rejects `?token=` and the real mechanism (bearer token as second `Sec-WebSocket-Protocol` value, `pulse.v1`) is undocumented in the spec; `/metrics` prose says "unauthenticated by default" while the handler 403s below Business regardless of token (`server.go:1056-1060`).
- **Why it's a problem:** the listing sells "OpenAPI-conformant Data API". The first thing an integrator does is generate a client — which will call `/api/v1/ingest/beacon` and `/api/v1/healthz` (404s), fail WS auth by putting the token in the query string, and misread `/metrics` availability. That's a doc-accuracy defect in the exact artifact you present as the contract source of truth (it's also linked in the rendered `docs/api/index.html`).
- **What it violates:** OpenAPI 3.1 semantics (path-level `servers` exist precisely for this); your contracts-first rule (`CLAUDE.md` §2: contract is source of truth).
- **Fix (exact):** add per-path `servers: [{url: /}]` at each of the 8 root-mounted path items (OpenAPI 3.1 supports per-path `servers`); document the `Sec-WebSocket-Protocol: pulse.v1, <token>` subprotocol on `/live/ws` (and either implement `?token=` for non-browser WS clients or remove `wsTokenQuery` from the spec); rewrite the `/metrics` description to "requires Business+ license; token optional via `PULSE_METRICS_TOKEN`". Re-run `redocly lint` + regenerate `web/src/lib/api/schema.d.ts` (types are generated from this file, so verify no drift).

### ISSUE J (P2) — Official AMS error webhooks are silently dropped; login POST contradicts the "GET-only" claim

- **Where:** `server/internal/collector/webhook/webhook.go:297-357` maps `liveStreamStarted`/`liveStreamEnded`/`vodReady` (+ aliases) and drops unknown actions — including the official `endpointFailed`, `publishTimeoutError`, `encoderNotOpenedError` events documented in the webhooks guide. These are ingest-failure signals squarely inside Pulse's F4 mission. Separately, `docs/faq.md:44` claims integration is "read-only REST **GET** requests" while `client.go:264` POSTs `/rest/v2/users/authenticate` (creates a session, mutates nothing) — the strongest form of an otherwise-true claim.
- **What it violates:** completeness against the official webhook contract; claim accuracy (a reviewer grepping for POST will find one).
- **Fix (exact):** map the three error actions to a `stream_error`-style ServerEvent (fields: `id`, `action`, `streamName`, `metadata`) feeding ingest-health/alerting, or at minimum count-and-log them instead of dropping; reword FAQ/README/AMS-INTEGRATION to "read-only: no state-changing calls; the only non-GET is the optional login POST when cookie auth is used". Note the webhook path remains unusable against stock AMS (no HMAC exists AMS-side — verified in the official docs — and your listener is fail-closed by design); keep that honestly documented as proxy-only, as `AMS-INTEGRATION.md:519-527` already does.

### ISSUE K (P2) — Residual publication hygiene: production IP, internal-draft cross-links, stale assessment, SDK version telemetry

- **Where/why (each verified):**
  - The production VPS IP `<VPS_IP>` remains in `docs/assessment/README.md`, `scenario-matrix.md`, `validation-environment.md`, `bugs/BUG-002-…md`, and `deploy/runbooks/monitoring.md:3` — the D-172 cleanup covered only `upgrade-rollback.md`, while `operator-expected.md:9` claims the leak is cleaned. The repo is public; this is a live, unauthenticated-scan target tied to your marketplace demo.
  - Customer-facing docs link into internal drafts: `licensing-public.md:225` → `listing-draft.md` ("Internal working copy"); `known-limitations.md`/`compatibility.md` → `final-assessment.md`, whose header says "DRAFT — OPERATOR REVIEW REQUIRED BEFORE SHARING".
  - `final-assessment.md` is titled **v0.3.0** and rows 7/8/12 contradict the now-closed reality (support/licensing/GHCR) — three different answers across the pack for the same checklist.
  - `sdk/beacon-js/src/index.ts:21` and `sdk/beacon-swift/…/Types.swift:4` still embed `SDK_VERSION = '0.1.0'` while the packages are 0.4.1 — every beacon batch reports the wrong `player.sdk_version`, poisoning your own SDK-adoption telemetry from day one. `web/package.json` is a third version string (0.1.0, private, harmless but noisy).
  - `docs/licensing.md` (internal) still describes the dev-pubkey-default behavior that 0.4.1 replaced with `officialPublicKeyHex` — an operator following it would mis-mint.
- **Fix (exact):** replace the IP with `<VPS_IP>` in the five files; stamp `final-assessment.md` to v0.4.x and reconcile rows 7/8/12 (or mark the whole file superseded and stop linking it from customer-facing docs); route the `licensing-public.md` §6 pointer to the public listing once it exists; bump both `SDK_VERSION` constants (ideally inject from package.json at build via tsup `define` so it can't drift again); refresh `docs/licensing.md` to the official-key default; rotate the exposed VPS/`.env` secrets (already on your operator list — do it before the repo gets marketplace traffic).

### ISSUE L (P2) — Test/CI fidelity gaps that will surface in Ant Media's qualification review

- **Where/why:** the nightly "AMS version matrix" runs mock profiles only — the workflow header itself admits every real-container leg silently fell back to mock ("making the 'version matrix' fictional"), and its REST smoke curls a path shape the client never uses (`/rest/v2/broadcasts/live/list?offset=0&size=10`), passing only because the mock matches on `strings.Contains(path, "/list")`. `qa/mock-ams` doesn't implement `vods`, `system-status`, `version`, or `users/authenticate` at all, so the VoD poller and login path are never exercised by CI (the mock's header also still lists a stale un-prefixed route). Trivy scans a separate amd64-only build with no digest-equality assertion against the pushed multi-arch image; `e2e.yml` is not a release gate; no `pulse` binary or checksums are attached to releases despite `install.md:302` offering one; the Helm chart is never packaged/published; `.github` never validates the quickstart.
- **What it violates:** your own evidence claims in `submission-process.md` §4 ("nightly AMS version-matrix" is listed as validation evidence — as stated, it's over-claimed) and standard supply-chain review expectations for a signed image.
- **Fix (exact):** rename the workflow/claim to "wire-format profile tests" or wire it to real AMS containers via the PAYG instance when available; teach mock-ams the four missing endpoints and fix its smoke-test path to the real client shape; scan the pushed digest for both architectures; add `e2e.yml` to the release gate; either attach a `pulse` binary + SHA256SUMS to releases or drop that sentence from `install.md`; `helm package` + push to GHCR OCI (or drop "Helm install" from the listing to "chart in repo").

### Smaller items (P3, batched)

Doc/code drift, each independently verified: `docs/runbooks/install.md` says migrations are *not* baked into the image (they are — `pulse.Dockerfile:41-45`), attributes `pulse-migrate` to the override (it's in the base since S105), names the meta DB `pulse.db` (code default `pulse_meta.db`, `serve.go:53`), and requires Go 1.22 (toolchain is 1.25); `deploy/config/clickhouse-low-footprint.xml` is referenced (compose + Helm values) but does not exist; `.env.example:122-126` claims `PULSE_BASE_URL` is inert (it is read — `config.go:455-460`); `AMS-INTEGRATION.md` omits the HTTP-Basic source-test mode, misstates the `PULSE_SECRET_KEY` default ("empty = no encryption" — boot actually hard-fails), has ~a dozen stale line citations, and lists `NodeInfo` as part of the poll cycle (dead code); `PULSE_AMS_LOGIN_EMAIL` lacks `_FILE` support unlike its password sibling; `server/internal/config` (YAML loader, `PULSE_LICENSE_OFFLINE_FILE`, `pulse.yaml`) is unreachable from `pulse serve` — delete or wire it, it actively confuses (two license-file env names in docs); prod/hardened CH healthchecks embed `${CLICKHOUSE_PASSWORD}` in `docker inspect` output (use `--password-file` or env indirection); `docker-compose.backup.yml:39` uses `${CLICKHOUSE_USER}` with no default; `productionize.md` prints a fictional `.env.example` and tells operators to set a DSN that `prod.yml:94` overrides; quickstart compose publishes `8090:8090` on 0.0.0.0 plain-HTTP with no warning comment (add the override's warning block); `.env.example` placeholder `changeme-…` values pass the ≥16-byte boot check — add a known-placeholder rejection at boot; registering additional AMS sources via `POST /api/v1/admin/sources` looks like it activates polling but only loads webhook secrets (document or implement); `restpoller.go:476-483` swallows WebRTC-stats errors with no log.

---

## 5. Where Pulse already complies (verified, not assumed)

- **Integration approach is valid and current.** Every REST path the poller uses (broadcasts/vods paginated lists, webrtc-client-stats, applications, system-status, version, authenticate) exists verbatim in AMS 3.0.3/master sources; nothing deprecated; tolerant decoding handles v2/v3 shape differences (e.g. dual-shape `/rest/v2/applications`). The single exception is the cluster path (Issue A).
- **Authentication matches the official docs** — post-S105: `Authorization: Bearer` for app-scope JWT, hyphen-less `ProxyAuthorization` for management scope, and cookie login whose plaintext-password behavior I verified against `CommonRestService.authenticateUser` (server MD5-hashes submitted values — your D-036 note is exactly right). The redirect-stripping of `ProxyAuthorization` shows real care.
- **Kafka and webhook contracts match the official docs** (post-S105): topics `ams-instance-stats`/`ams-webrtc-stats` and `server.kafka_brokers` are exactly the documented names; webhook action names and the form-urlencoded content type match the webhooks guide; the honest EXPERIMENTAL label on Kafka (never validated against a live producer) is the right call.
- **AMS version currency:** 3.0.3 (May 2026) is still the latest release; your live validation targets it; the panel-revamp risk assessment (G-27) is evidence-based and reasonable.
- **Read-only, upgrade-tolerant posture is real:** one grep-verified non-GET (the login POST); no writes to AMS state; SSRF-guarded probe client; no AMS files touched. This is the strongest possible answer to a marketplace security review, matched by: cosign-signed multi-arch images with SBOM/provenance, Trivy gate, fail-closed webhook HMAC, `_FILE` secrets convention, encrypted-at-rest credentials, constant-time scrape-token compare, WS origin enforcement, IP hashing + optional anonymization, no phone-home licensing, audit log. SECURITY.md is complete.
- **Marketplace pack quality:** all 15 screenshots exist at 1920×1080 (byte-distinct light variant — the old duplicate bug is genuinely fixed), full logo/OG asset kit, 43-char title and 240-char description within your researched limits, pricing consistent across listing/licensing/support docs, support mailbox live with SLA, 14-day trial mechanics defined, external-integration precedent on the marketplace (Mobiotics) supports your listing model, and `submission-process.md`'s facts-vs-assumptions ledger (A1–A10) is exactly the right way to handle Ant Media's unpublished process.
- **Engineering hygiene that will impress a reviewer:** contracts-first with CI validation, generated API types, 70.2% coverage floor with `-race`, golden-tested Helm, size-gated SDK, Keep-a-Changelog discipline, and 26 honestly disclosed limitations.

---

## 6. Readiness assessment & prioritized checklist

**Verdict: NOT READY today.** No architectural blockers; the gate is one release cut plus a bounded list of fixes. With P0 items done (about a week of focused work plus the operator-only steps), Pulse would be in genuinely strong shape for the developer meeting — better documented and better evidenced than most current marketplace listings.

**P0 — must complete before submitting/evaluator contact**
1. Fix cluster-nodes pagination (Issue A) — or de-scope cluster claims everywhere until validated.
2. Cut and publish **v0.4.2**; repoint every pinned tag; extend the release version guard to the deploy surface (Issue B).
3. Make the README base-Compose path actually boot: ClickHouse auth env + a CI boot test of that exact command (Issue C).
4. Helm: render `PULSE_INGEST_LISTEN_ADDR`; fix the Postgres PVC condition; fix S3 env names; regenerate goldens (Issue D).
5. Fix `licensegen` Business `max_nodes` 5→50 + mint-path regression test (Issue F).
6. Externalize the listing copy: noncommercial disclosure on Free, tier corrections, egress caveat, strip internal IDs; choose category; write the long description (Issue E).
7. Operator: submit via the agreed Ankush path (reply draft is ready; fill the brackets), set up billing, record the demo voiceover.

**P1 — before or immediately after the developer meeting**
8. Quickstart hardening: preserve `.env`, reuse existing secret key, degraded-vs-ok health logic, tag-pinned compose download, CI coverage (Issue G).
9. Helm ClickHouse `default`-user lockdown + NetworkPolicy (Issue H).
10. OpenAPI: per-path `servers` for root-mounted routes, WS subprotocol documentation, `/metrics` gating prose (Issue I).
11. Hygiene sweep: production IP, internal-draft links, `final-assessment.md` refresh, SDK_VERSION constants, `docs/licensing.md` pubkey framing; rotate exposed secrets (Issue K).
12. Publish the beacon SDK (add `NPM_TOKEN`) or remove "npm (coming)" from customer docs; decide the Swift SPM story.
13. Capacity number: run the load lane on the PAYG AMS (also unlocks AV-15 live Kafka validation) and replace the PROVISIONAL claim in `compatibility.md`.

**P2 — quality debt to clear before the listing goes live**
14. Webhook error-action mapping + read-only wording (Issue J).
15. CI fidelity: honest version-matrix labeling, mock-ams missing endpoints + smoke path, Trivy digest/arm64, e2e release gate, binary artifacts or doc removal, chart publishing (Issue L).
16. The P3 doc/code drift batch (§4, Smaller items).

**Standing assumptions to close at the meeting (from your own A-ledger, still correct):** listing format/category (A2/A10), asset specs (A3), post-year-1 revenue terms in writing (A4 — I could not find the first-year terms published publicly either; get both in writing), review flow/SLA (A5), trial expectations (A6), AMS version-support expectations (A7), docs-linking policy (A8), load-evidence format (A9).

---

## Appendix — Official sources used

- Ant Media Marketplace: https://antmedia.io/marketplace/ (+ listings: Scribe, TunaDesk, Mobiotics OTT)
- AMS releases (3.0.3 latest, 2026-05-05): https://github.com/ant-media/Ant-Media-Server/releases
- Web Panel REST API (auth, ProxyAuthorization, applications): https://docs.antmedia.io/guides/developer-sdk-and-api/rest-api-guide/management-rest-apis/
- JWT REST API Filter (app-scope Authorization Bearer): https://docs.antmedia.io/guides/developer-sdk-and-api/rest-api-guide/jwt-rest-api-filter/
- Webhooks (events, form-urlencoded payload, retries, no signing): https://docs.antmedia.io/guides/advanced-usage/webhooks/
- Kafka monitoring (server.kafka_brokers; ams-instance-stats / ams-webrtc-stats): https://docs.antmedia.io/guides/monitoring/monitoring-ams-with-grafana
- AMS source verified at tag `ams-v3.0.3` and `master`: `io/antmedia/console/rest/ClusterRestServiceV2.java` (paginated `/v2/cluster/nodes/{offset}/{size}`), `RestServiceV2.java` (`/v2/system-status`, `/v2/version`, `/v2/applications`, `/v2/users/authenticate`, `/v2/cluster-mode-status`), `CommonRestService.java` (server-side MD5 of submitted passwords), `io/antmedia/rest/BroadcastRestService.java` (`/list/{offset}/{size}`, `/{stream_id}/webrtc-client-stats/{offset}/{size}`), `io/antmedia/rest/VoDRestService.java` (`/list/{offset}/{size}`)
