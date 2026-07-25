# Pulse (ams-pulse) — Ant Media Marketplace Readiness Review

**Reviewed:** 2026-07-25 · **Version audited:** v0.4.1 (working tree at `/Users/ae/repo/ams-pulse`)
**Method:** Full codebase analysis (server, web, SDKs, contracts, deploy, docs, QA) cross-checked against the official Ant Media Server documentation (docs.antmedia.io), the Ant Media Marketplace site, the Ant-Media-Server GitHub source/releases, and live registry checks (GHCR, npmjs). Every claim below is derived from the implementation or a cited source — no assumptions.

---

## 1. Executive summary and verdict

**Verdict: NOT YET READY — but close.** Pulse is architecturally sound, uses the current AMS REST v2 API correctly, is live-validated against AMS 3.0.3 (the latest stable release, published 2026-05-05), and its packaging/supply-chain quality (digest-pinned images, cosign-signed multi-arch GHCR releases, SBOM, Trivy gate) exceeds what any existing marketplace listing demonstrates. The submission groundwork (listing copy, pricing, support SLA, compatibility docs) is unusually thorough.

However, there are **2 genuine integration defects against the official AMS documentation** (Kafka collector, webhook payload mapping), **1 partial auth-mode defect** (management-scope JWT header), and a set of **marketplace-facing packaging gaps** (listing media not actually present or reproducible, SDK install instructions that fail, version drift, placeholder identities in shipped metadata). None are architectural; all are fixable in days, not weeks.

Important context on "requirements": **Ant Media publishes no formal marketplace submission checklist** — confirmed both by your own research (`docs/marketplace/submission-process.md`) and by my review of antmedia.io/marketplace (the only public CTA is "apply today to become a marketplace vendor"; process runs through direct contact, which you already have via Ankush Banyal). Compliance therefore means: (a) correctness against official AMS integration docs, (b) matching the de-facto structure of existing listings, and (c) internal consistency of your own submission package. Findings below are graded against those three.

---

## 2. What the application is (derived from the implementation)

Pulse is a **self-hosted, read-only analytics, QoE-monitoring and alerting sidecar for Ant Media Server**. It never modifies AMS and holds no AMS-side component (zero `.java`/`.jar` files in the repo; nothing is installed into AMS).

- **Single Go binary** (`server/cmd/pulse`, subcommands `serve` / `migrate` / `version` / `diag`) serving the embedded React 19 dashboard (`web/`, built into the Docker image at `/usr/share/pulse/web`).
- **Stores:** ClickHouse (events, sessions, rollups; migrations baked into the image) + SQLite (default) or Postgres meta store.
- **Four ingest paths** (supervised, independently restartable — `server/internal/collector/collector.go`):
  1. **REST poller** (primary, always on, 5 s default) → AMS REST v2, read-only GETs.
  2. **Webhook listener** (optional, HMAC-SHA256 fail-closed, `:8092`).
  3. **Kafka consumer** (optional) for the AMS native Kafka producer feed.
  4. **Beacon ingest** (`POST /ingest/beacon`) for the MIT-licensed player QoE SDK (`sdk/beacon-js`, 3.52 KB gzip; Swift counterpart in `sdk/beacon-swift`).
- **Features:** live overview + WebSocket push, viewer session stitching, ingest health scoring, geo/UA enrichment, alerting (email/Slack/Telegram/PagerDuty/webhook), anomaly detection (Welford baselines), usage/billing reports (CSV/PDF, S3, ±1% reconciliation), synthetic probes (HLS/DASH/WebRTC/RTMP), multi-tenancy, cluster fleet view, Prometheus `/metrics`, OIDC SSO, audit log.
- **Licensing:** offline ed25519-signed keys, four tiers (Free/Pro/Business/Enterprise), fail-open-to-Free for reads; production vendor pubkey embedded since v0.4.1.
- **Deployment:** quickstart installer (`deploy/quickstart/install.sh` → GHCR image `ghcr.io/aytekxr/ams-pulse:0.4.1`), Docker Compose prod stack, experimental Helm chart, host-nginx TLS edge.
- **Marketplace positioning** (your own docs): a standalone **Application/Integration** listing — which the marketplace demonstrably supports (existing categories include Applications like Scribe/Scotty/Stamp, Platforms like Mobiotics OTT/CamOS, Tools like TunaDesk/Circle, alongside JAR Plugins). Your open question A1 (standalone service vs. AMS-side artifact) remains the right thing to confirm at the developer meeting, but the marketplace's own catalog supports your positioning.

---

## 3. Issues found

Severity scale: **P0** = fix before submitting; **P1** = fix before the listing goes live / evaluators touch it; **P2** = quality/consistency improvements.

### ISSUE 1 (P0/P1*) — Kafka collector cannot consume the real AMS Kafka feed

**Why it's a problem.** The official AMS Kafka producer (enabled via `server.kafka_brokers` in `conf/red5.properties`) publishes to **three topics: `ams-instance-stats`, `ams-webrtc-stats`, `kafka-webrtc-tester-stats`**, and instance-stats messages carry fields `instanceId`, `cpuUsage`, `jvmMemoryUsage`, `systemMemoryInfo`, `fileSystemInfo`, … (nested JSON objects). Pulse's collector:

- subscribes to a single hardcoded default topic **`ams-server-events`** — a topic AMS never publishes to (`server/internal/collector/kafka/kafka.go:35,58,100`), and there is **no env var to change it** (`server/cmd/pulse/config.go:304–312` only reads `PULSE_KAFKA_BROKERS` and `PULSE_KAFKA_GROUP_ID`);
- parses **flat numeric fields** `cpuUsage`, `memoryUsage`, `diskUsage` (`normalizeKafkaMessage`, `kafka.go:218–296`) — but official `cpuUsage` is a nested object, and `memoryUsage`/`diskUsage` don't exist (the real fields are `jvmMemoryUsage`, `systemMemoryInfo`, `fileSystemInfo`);
- attributes node identity from `nodeId` — the official field is `instanceId`.

Net effect: even with correct brokers configured, Pulse receives nothing (wrong topic), and would parse zeros if it did (wrong shape). This matters doubly because your own `docs/known-limitations.md` LIM-01 states standalone CPU/mem/disk gauges **depend entirely on the Kafka feed** (AMS 3.x REST `system-status` has no load data), and `docs/kafka-integration.md` is honest that this path is AV-15 BLOCKED / never live-validated (LIM-19).

**What it violates.** Official integration doc: *Monitoring AMS with Grafana* (docs.antmedia.io/guides/monitoring/monitoring-ams-with-grafana) — exact property `server.kafka_brokers`, exact topics and instance-stat field list. Also violates your listing draft's implied claim that Kafka ingest is a working source.

**Exact fix.**
1. `server/internal/collector/kafka/kafka.go:58` — change default to the real topics:
   ```go
   Topics: []string{"ams-instance-stats", "ams-webrtc-stats"},
   ```
2. `server/cmd/pulse/config.go` (~line 312) — add an override:
   ```go
   if v := os.Getenv("PULSE_KAFKA_TOPICS"); v != "" {
       cfg.KafkaTopics = strings.Split(v, ",")
   }
   ```
   and thread it through `serve.go` into `kafka.Config.Topics`. Document it in `docs/AMS-INTEGRATION.md` §3.7 and `docs/admin-guide.md`.
3. Rewrite `normalizeKafkaMessage` to parse the official shapes: route `ams-instance-stats` messages by topic (not field-sniffing), take node identity from `instanceId`, and extract CPU/mem/disk from the nested `cpuUsage` / `systemMemoryInfo` / `fileSystemInfo` objects (capture one real message during validation and pin it as a testdata fixture, as you did for REST in `pkg/amsclient/testdata`).
4. Run the AV-15 live validation you already scoped (real AMS + real broker) before claiming the feature.

\* **Alternative (fully P0-compliant too):** if you don't want to gate submission on this, **de-scope Kafka from the listing** — remove it from marketplace copy and mark it "experimental/preview" in `docs/AMS-INTEGRATION.md`, `docs/kafka-integration.md` and the FAQ. Your honest-limitations culture supports this; what's not acceptable for a marketplace evaluator is shipping it as a documented source in its current state, because the failure is silent (consumer connects, lag stays 0, dashboards stay blank).

### ISSUE 2 (P1) — Webhook parser doesn't match the official AMS webhook payload

**Why it's a problem.** Per the official webhook documentation, AMS lifecycle webhook payloads carry the stream id in **`id`** (fields: `id`, `action`, `streamName`, `category`, `metadata`, `timestamp`; `vodReady` adds `vodName`, `vodId`, `app`, `duration`), default `Content-Type: application/json` but **configurable to `application/x-www-form-urlencoded`**. Pulse's `translateWebhook` (`server/internal/collector/webhook/webhook.go:259–314`):

- reads only `streamId` — never `id` → events from a verbatim AMS payload would have an **empty stream id**;
- reads `vodSize` — a field the official payload doesn't include (it includes `vodId`/`duration`);
- parses JSON only — a form-urlencoded configuration would 200-and-drop every event.

Today this is latent (you correctly document in `docs/AMS-INTEGRATION.md` §4.5/D-066 that AMS 3.0.3 webhooks are unsigned and must NOT be pointed at Pulse's HMAC-fail-closed listener — my check of the official webhook doc confirms there is no signature/HMAC/custom-header option). But your documented supported use case is an **HMAC signing proxy forwarding AMS payloads** — and a verbatim-forwarding proxy hits exactly these field mismatches.

**What it violates.** Official *Webhooks* doc (docs.antmedia.io/guides/advanced-usage/webhooks): payload field names and content types.

**Exact fix.** In `translateWebhook`:
```go
streamID := jsonString(raw["streamId"])
if streamID == "" {
    streamID = jsonString(raw["id"]) // official AMS webhook field
}
```
For `vodReady`, also read `vodId` and `duration`; keep `vodSize` as a fallback. Optionally accept `application/x-www-form-urlencoded` bodies (parse `r.PostForm` into the same map) or explicitly document JSON-only and instruct proxy authors accordingly.

**Related doc contradiction (fix together):** `docs/AMS-INTEGRATION.md` §7 B7 walks the operator through "AMS Management Console > Settings > Webhooks … Header name: X-Ams-Signature" — a capability §4.5 of the same document (and the official docs) says AMS doesn't have. Rewrite B7 to describe the signing-proxy pattern instead. Also note for your D-V2-1 decision (unsigned mode + IP allowlist): AMS **retries on any non-200** (`webhookRetryCount`/`webhookRetryDelay`, ≥2.8.3), so Pulse's 401 responses would trigger retry storms from a misconfigured AMS — an extra argument for implementing the allowlist mode before advertising webhook ingest at all.

### ISSUE 3 (P1) — Static-token auth sends the wrong header for management-scope endpoints

**Why it's a problem.** Pulse sends `Authorization: Bearer <token>` on **every** request (`server/pkg/amsclient/client.go:213,339`). Per official docs this is correct for **application-scope** REST (`settings.jwtControlEnabled` → `Authorization: Bearer {JWTToken}` — matches your `/{app}/rest/v2/broadcasts/...`, `/vods/...` calls), but the **web-panel/management** REST API (`server.jwtServerControlEnabled=true` + `server.jwtServerSecretKey`) expects the token in a **`ProxyAuthorization: <jwt>`** header. Pulse calls five management-scope endpoints on every poll (`/rest/v2/applications`, `/rest/v2/cluster/nodes`, `/rest/v2/cluster/nodes/{id}`, `/rest/v2/system-status`, `/rest/v2/version`) — so `PULSE_AMS_AUTH_TOKEN`-only mode fails exactly where cluster/fleet data comes from. (Your primary, live-validated path — cookie-session login — is unaffected; this bug only bites token-mode users, and your docs currently call the token "AMS JWT/bearer token" without this caveat.)

**What it violates.** Official *Web Panel REST API* doc (docs.antmedia.io/guides/developer-sdk-and-api/rest-api-guide/management-rest-apis): `ProxyAuthorization` header with example curl.

**Exact fix.** In `doGet` (`client.go:331–340`), send both headers — harmless where unused:
```go
if c.authHeader != "" {
    req.Header.Set("Authorization", c.authHeader)                        // app-scope JWT (Bearer)
    req.Header.Set("ProxyAuthorization", strings.TrimPrefix(c.authHeader, "Bearer ")) // management-scope JWT
}
```
And in `docs/AMS-INTEGRATION.md` §2, document which AMS setting each mode maps to (`server.jwtServerControlEnabled` vs app-level `jwtControlEnabled` vs cookie-session).

### ISSUE 4 (P0) — Listing media doesn't exist and isn't reproducible on another machine

**Why it's a problem.** `docs/marketplace/submission-package.md` marks screenshots "READY (regenerable)" and states a demo rough-cut "exists (`docs/marketplace/demo/pulse-demo-roughcut.webm`, D-170)". Verified on your machine: **neither `docs/marketplace/screenshots/` nor `docs/marketplace/demo/` exists** (both gitignored, `.gitignore:74,86`, and never rendered in this working tree). Worse, the regeneration commands the package cites can't run anywhere but the original dev box: `qa/marketplace/capture-live-screenshots.mjs:16` and `qa/marketplace/capture-demo-video.mjs:28,35` hardcode `/home/aytek/repo/ams-pulse/...`. Screenshots and a demo video are standard sections of existing marketplace listings (verified against the live Ant Assist listing: screenshots, embedded YouTube video, install steps, support contact) — you cannot submit without them, and `docs/user-guide.md` embeds 14 images from the missing directory (broken wherever the doc renders, e.g. GitHub).

**What it violates.** De-facto marketplace listing structure (antmedia.io/marketplace listings); internal consistency of `submission-package.md`; your own `screenshot-list.md` plan.

**Exact fix.**
1. Make the scripts portable — replace the hardcoded imports with a relative resolution, e.g.:
   ```js
   import { fileURLToPath } from "node:url";
   import path from "node:path";
   const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
   const pkg = await import(path.join(REPO_ROOT, "web/node_modules/@playwright/test/index.js"));
   ```
2. Run `node qa/marketplace/capture-live-screenshots.mjs` and the demo capture; store outputs somewhere durable (commit them, or attach to the GitHub release — pick one and update `submission-package.md` to match reality).
3. Either commit the user-guide screenshot set or strip/replace the 14 image references in `docs/user-guide.md` before the doc is linked from a listing.

### ISSUE 5 (P0) — Version drift across submission-facing files

**Why it's a problem.** An evaluator cross-reading your docs sees three different versions for one product: `VERSION`/CHANGELOG/quickstart/Helm image tag say **0.4.1**; `docs/product.md:33` says "current release v0.4.0"; `docs/faq.md:3` header "Pulse v0.4.0"; `docs/known-limitations.md:3,8` "v0.4.0"; `deploy/helm/pulse/Chart.yaml:9-11` `version: 0.1.0` / `appVersion: "0.1.0"` (its own comment says "App version matches the pulse binary release tag" — it doesn't); `contracts/openapi/pulse-api.yaml` `info.version: 1.0.0`. All of product.md/faq/known-limitations are listed as linkable docs in the submission package.

**What it violates.** Internal consistency of the submission package; Helm chart's own stated convention.

**Exact fix.** Bump `docs/product.md:33`, `docs/faq.md:3`, `docs/known-limitations.md:3,8` to v0.4.1; set `Chart.yaml` `appVersion: "0.4.1"` (and bump chart `version`); either version the OpenAPI doc independently on purpose (state so in the file) or set `info.version` to the product version. Add a release-checklist grep so this can't recur (e.g. `grep -rn "v0\.4\.0" docs/ && exit 1` in `release.yml`).

### ISSUE 6 (P1) — Placeholder identities in shipped metadata (org, domain, license name)

**Why it's a problem.** Several published artifacts identify a vendor that doesn't exist: `server/go.mod:1` module `github.com/pulse-analytics/pulse/server`; `deploy/helm/pulse/Chart.yaml:20-26` `home`/`sources` → `https://github.com/pulse-analytics/pulse`, maintainer `infra@pulse-analytics.io`; `contracts/openapi/pulse-api.yaml:5-7` license `"Proprietary"` at `https://pulse.dev/license`; all `contracts/events/*.schema.json` `$id`s under `https://pulse.dev/schemas/...`. The real project is `github.com/aytekXR/ams-pulse` under PolyForm Noncommercial 1.0.0 (+ MIT SDK). For a marketplace vendor-vetting process this reads as either abandoned scaffolding or a different owner; the OpenAPI license name also **contradicts** your published licensing docs.

**What it violates.** Marketplace vendor due-diligence expectations (vendor identity is a listing section on every existing listing); consistency with `docs/licensing-public.md`.

**Exact fix.** `Chart.yaml`: point `home`/`sources` at `https://github.com/aytekXR/ams-pulse`, maintainer to your real contact (`support@beyondkaira.com`). OpenAPI: `license: {name: "PolyForm Noncommercial 1.0.0 (server) / commercial keys", url: "https://github.com/aytekXR/ams-pulse/blob/main/docs/licensing-public.md"}`. Schema `$id`s: move to a domain you own (e.g. `https://pulse.beyondkaira.com/schemas/...`) — they're opaque URIs, so this is a safe string change. The `go.mod` module path is internal-only (no importers); renaming to `github.com/aytekXR/ams-pulse/server` is a mechanical find-replace across imports — do it now or explicitly accept it as internal naming (document the exception in CLAUDE.md/contributing docs so it stops looking accidental).

### ISSUE 7 (P0) — Beacon SDK install instructions fail (npm 404 / unresolvable SPM URL)

**Why it's a problem.** The QoE beacon is a headline listing bullet, and the first command in its docs fails: `npm install @pulse/beacon` (`sdk/beacon-js/README.md:24,26`, `docs/beacon-sdk.md:93,95`) — **verified 404 on registry.npmjs.org (2026-07-25)**; the package has never been published and no publish workflow exists in `.github/workflows/`. The Swift README (`sdk/beacon-swift/README.md:17`) instructs `.package(url: "https://github.com/aytekXR/ams-pulse.git", from: "0.1.0")` — unresolvable twice over: SPM URL dependencies require `Package.swift` at the repo **root** (yours is in `sdk/beacon-swift/`), and no `0.1.0` tag exists (tags are `v0.4.x`).

**What it violates.** Install-flow completeness (the marketplace de-facto standard is step-by-step install instructions that work — every existing listing has them); your own `docs/beacon-sdk.md` integration guide.

**Exact fix (choose per SDK).** JS: publish to npm under a scope you can own (the `pulse` org is almost certainly taken — e.g. `@beyondkaira/pulse-beacon` or unscoped `ams-pulse-beacon`), add a `publish-npm` job to `release.yml`, and update the two READMEs + `docs/beacon-sdk.md`; **or** drop npm from the docs and document the vendored path (`npm install <tarball from GitHub release>` — attach the tsup build output to releases). Swift: either split `beacon-swift` into its own tagged repo (SPM-standard), or document local-path/vendored integration only and delete the misleading URL snippet.

### ISSUE 8 (P1) — LICENSE file doesn't identify the licensor

**Why it's a problem.** Root `LICENSE` is the verbatim PolyForm Noncommercial 1.0.0 text with **no copyright/licensor line**. PolyForm licenses are written to be applied with a notice identifying who grants the license (the text's "licensor" is otherwise undefined for your copy), and your whole commercial model (selling license keys for noncommercially-licensed code) depends on unambiguous ownership. A marketplace vendor review will look at exactly this file.

**What it violates.** PolyForm usage convention (licensor identification); commercial-listing due diligence.

**Exact fix.** Prepend to `LICENSE` (and mirror in README's license section):
```
Copyright (c) 2026 Aytek Erdoğan (beyondkaira.com)

Licensed under the PolyForm Noncommercial License 1.0.0 (below).
Commercial licenses are available — see docs/licensing-public.md.
```

### ISSUE 9 (P0) — Trial-key story is told three different ways

**Why it's a problem.** `deploy/quickstart/.env.example:23` — "A trial key is included with the marketplace listing"; `docs/licensing-public.md` — trial via "contact support@beyondkaira.com or the marketplace support channel"; `docs/marketplace/listing-draft.md` — "14-day self-serve Pro trial, no card". An evaluator hits this file set in their first 15 minutes; pick one mechanism (the listing's self-serve promise is the strongest — but only if you can actually mint/deliver keys that way) and align all three files, plus FAQ Q21.

**What it violates.** Internal consistency of submission-facing docs.

**Exact fix.** Decide the delivery mechanism; then edit `deploy/quickstart/.env.example:23-24`, `docs/licensing-public.md` §3, `docs/faq.md` Q21 to describe the same flow as `listing-draft.md`.

### ISSUE 10 (P1) — Production compose path ignores your own published, signed image

**Why it's a problem.** `deploy/docker-compose.yml:22` still carries `TODO(INFRA-01 wave-1): switch to published image once first release ships` — four releases have shipped. The README's "supported production path" (`docker compose ... --build`) makes evaluators compile from source, bypassing the cosign-signed, Trivy-gated, SBOM-attached GHCR image that is your best trust signal (and that the quickstart correctly uses). Slow first-run + supply-chain story undercut in the same move.

**What it violates.** Your own release/verification narrative (`docs/marketplace/release-notes.md`, `README.md:18-21`); deployment best practice.

**Exact fix.** In `deploy/docker-compose.yml` (and the prod overlay): `image: ghcr.io/aytekxr/ams-pulse:0.4.1` with the `build:` block moved behind a `--profile build-from-source` (or a separate `docker-compose.dev.yml`); delete the TODO; update `docs/runbooks/install.md` Path A accordingly.

### ISSUE 11 (P1) — Submission-doc staleness, leaks, and hygiene

Each small, all evaluator-visible; batch-fix in one pass:

1. **Stale DRAFT banners:** `listing-draft.md`, `screenshot-list.md`, `release-notes.md`, `demo-video-script.md`, `submission-package.md`, `submission-process.md`, `docs/licensing-public.md` all still say "DRAFT — INTERNAL … gated on D-081" although `submission-package.md` records D-081 as CLEARED (D-169). Remove the banners (licensing-public especially — it's a listed public doc).
2. **Contradictory NEEDS-OPERATOR rows:** `listing-draft.md:21,246,250+` still say support channel undecided; `docs/support.md`/`licensing-public.md` name `support@beyondkaira.com`. Same for `submission-process.md` §3 prerequisites (GHCR "401", pricing "unresolved" — both closed). Update to the decided state.
3. **Internal-only HTML comment** (competitive positioning vs `Management-panel-reborn`) inside `listing-draft.md` — strip before any copy-paste into a listing form.
4. **Infrastructure leak in a linkable doc:** `deploy/runbooks/upgrade-rollback.md:3` contains your VPS IP `161.97.172.146`; nginx confs hardcode `beyondkaira.com` internals. Fine in-repo as reference, but the runbook is marked "READY / externally linkable" in the submission package — scrub the IP or de-list the doc.
5. **Missing referenced file:** `docs/support.md` §4 proposes `.github/ISSUE_TEMPLATE/bug_report.yml` — directory doesn't exist. Add the template (it's also just good triage hygiene once marketplace users arrive).
6. **Secret rotation:** `docs/operator-expected.md` item 8 records chat-exposed/VPS-group-readable secrets in `deploy/.env` pending rotation. Rotate before inviting external evaluators to the production instance.
7. Cosmetic: `poem.md` at repo root in a vendor-reviewed snapshot.

### ISSUE 12 (P2) — Single inconsistent AMS-version-support claim

`docs/AMS-INTEGRATION.md` §2.4: "v2.8 and above are supported" vs `docs/compatibility.md`: earliest coverage 2.10.0 (mock-only), only 3.0.3 live-validated, "deploy AMS 3.x" vs meeting brief: "matrix back to 2.10". Pick one public claim — recommended: **"Validated on AMS 3.0.3 Enterprise (current release); best-effort compatibility 2.10+ via version-tolerance tests"** — and align all three files. (Your tolerance engineering — dual applications envelope, 404/405-tolerant version/cluster probes, disappearance-based end detection — genuinely supports the 2.10+ claim at the code level: `server/pkg/amsclient/client.go:396-427,494-504,543-553`.)

---

## 4. Where Pulse already complies (verified, not assumed)

1. **Uses the current AMS API generation, correctly.** All nine REST calls are `/rest/v2/*` paths that match the current official REST guide; AMS 3.0.3 (2026-05-05) is the latest stable and is exactly what Pulse was live-validated against (46/50 scenarios). Cookie-session auth matches the official flow (`POST /rest/v2/users/authenticate` → JSESSIONID) — and I verified against the AMS source (`CommonRestService.authenticateUser`) that sending the **raw** password is valid (the server MD5-hashes incoming passwords; the docs' "must be MD5" describes the panel client, not a hard requirement), so your implementation is correct despite the ambiguous official wording. The IP-safe cookie jar and throttled re-login (≤2 per 3 s window) even handle AMS's 2-attempt/5-minute lockout sensibly.
2. **App-scope JWT format is right.** `Authorization: Bearer {JWT}` matches the official JWT REST filter doc for `/{app}/rest/v2/*` (the gap is only management scope — Issue 3).
3. **Webhook event vocabulary matches the official action names** (`liveStreamStarted`, `liveStreamEnded`, `vodReady` + sensible aliases), the HMAC listener is fail-closed with constant-time compare, replay protection is optional, and — critically — your docs correctly tell operators **not** to point AMS's unsigned `listenerHookURL` at Pulse. My doc check confirms AMS has no outgoing-webhook signing; your REST-first decision is the correct integration for current AMS.
4. **Read-only, zero-footprint integration.** No AMS-side artifact, no writes, version-tolerant parsing, honest handling of real-wire quirks (bitrate in bps, `speed` ≠ bitrate, `hlsViewerCount` inflation, implicit-RTMP deletion, SRT-as-RTMP) — this is the "upgrade-tolerant sidecar" posture marketplace evaluators like, and it's implemented, not just claimed.
5. **Standalone-application listing type is real.** The marketplace's own catalog contains external applications/platforms/tools (Scribe, Scotty, Mobiotics OTT, CamOS, TunaDesk, Circle) — your positioning (A1) has precedent; no evidence anywhere that a JAR/WAR artifact is mandatory.
6. **Listing content maps to the de-facto listing structure.** Comparing `listing-draft.md` against a live listing (Ant Assist): title/tagline/description/highlights ✓, prerequisites + step-by-step install ✓ (quickstart), support contact + hours/SLA ✓ (`docs/support.md`), vendor branding ✓ (brandkit logos + OG banner present on disk), screenshots/video — planned but not rendered (Issue 4).
7. **Packaging and supply chain exceed the bar.** Digest-pinned multi-stage Dockerfile, non-root runtime, healthcheck, one-shot migrate service, loopback-only prod ports, multi-arch GHCR image with cosign keyless signature + SBOM + provenance, Trivy release gate, CodeQL, Dependabot. GitHub repo and v0.4.1 release verified public.
8. **Install/config/deploy flow is complete and tested.** `install.sh` is defensive (compose-v2 check, pull preflight with honest 401 messaging and build-from-source fallback, secret generation, healthz gate, bootstrap-token extraction, cleanup trap); quickstart was anonymously clean-room verified (D-168); admin-guide documents all 69 env vars; migrations are baked and ordered; upgrade/rollback runbook exists.
9. **Security architecture is marketplace-grade.** Token kinds/scopes, HMAC-hashed tokens at rest, AES-GCM secrets, fail-closed boot without `PULSE_SECRET_KEY`, SSRF guard with dial-time IP checks on every outbound leaf, OIDC with fail-closed group mapping, rate limits, no phone-home/telemetry (a real differentiator for a self-hosted marketplace listing).
10. **Honesty artifacts.** 26-item known-limitations doc, capacity marked "provisional" until the load lane runs, mock-vs-live validation clearly separated. Keep these linked in the listing — they're credibility, not liability.

---

## 5. Readiness assessment

**Current state: ~85% ready. Submission-blocking work is concentrated in media assets, doc consistency, and two integration fixes (or one fix + one honest de-scope).** The architecture needs no changes; nothing here threatens acceptance structurally, since the marketplace demonstrably lists standalone applications and your contact-driven submission process is already underway.

### Prioritized pre-submission checklist

**P0 — do before sending the Ankush reply / listing content:**
1. Render the six listing screenshots + demo video for real: fix the hardcoded `/home/aytek` paths in `qa/marketplace/capture-live-screenshots.mjs:16` and `capture-demo-video.mjs:28,35`, run both, store the artifacts, and correct the "exists/READY" claims in `submission-package.md` (Issue 4).
2. Decide Kafka: fix topics/schema/node-id (+ add `PULSE_KAFKA_TOPICS`) **or** de-scope Kafka from listing copy and mark experimental in docs (Issue 1).
3. Fix beacon install paths: publish the npm package under an ownable name (+ release-workflow job) or rewrite the install docs to the vendored/tarball path; fix or remove the Swift SPM snippet (Issue 7).
4. Version sweep to 0.4.1: `docs/product.md`, `docs/faq.md`, `docs/known-limitations.md`, `Chart.yaml appVersion` (Issue 5).
5. Unify the trial-key story across `.env.example` / `licensing-public.md` / `listing-draft.md` / FAQ Q21 (Issue 9).
6. Update stale marketplace-doc rows and banners (D-081 cleared; support channel decided; strip the internal HTML comment) (Issue 11.1–11.3).
7. Rotate the exposed `deploy/.env` secrets on the production VPS (Issue 11.6).

**P1 — do before evaluators get hands-on (can overlap the meeting):**
8. Webhook parser: accept `id` as stream id, read `vodId`/`duration`, decide on form-urlencoded; rewrite `AMS-INTEGRATION.md` §7 B7 to the signing-proxy reality (Issue 2).
9. Add `ProxyAuthorization` alongside `Authorization: Bearer` in `amsclient.doGet`; document token-mode scope (Issue 3).
10. Default prod compose to the signed GHCR image; remove the stale TODO; update install runbook Path A (Issue 10).
11. Replace placeholder identities: `Chart.yaml` home/sources/maintainer, OpenAPI license name+URL, schema `$id` domains; decide on the go.mod module path (Issue 6).
12. Add the licensor notice to `LICENSE` (Issue 8).
13. Fix or remove the 14 broken screenshot references in `docs/user-guide.md`; scrub the VPS IP from `upgrade-rollback.md`; add `.github/ISSUE_TEMPLATE/bug_report.yml` (Issues 4/11.4/11.5).
14. Run the capacity load lane on a PAYG AMS instance and fill the compatibility-matrix capacity row — Ant Media signaled load validation is part of qualification (your own submission-process notes).

**P2 — quality follow-ups:**
15. Unify the AMS-version-support claim (2.8 vs 2.10 vs 3.x) across the three docs (Issue 12).
16. Implement D-V2-1 (unsigned-webhook mode behind an IP allowlist) if you want native AMS webhook ingest to be a real feature rather than proxy-only.
17. Helm: bump chart version/appVersion, digest-pin the ClickHouse image, and schedule the real-cluster `helm install` validation (D-002) before ever removing the EXPERIMENTAL label.
18. Consider splitting `beacon-swift` into its own repo for standard SPM consumption.
19. Confirm at the developer meeting (your A1–A10 list is good): listing artifact type, media specs, post-year-1 revenue terms, trial expectations, and whether they want a panel-embedded view (your nginx currently sends `X-Frame-Options: DENY` / `frame-ancestors 'none'` — if panel embedding is ever requested, that's a deliberate config change, not a bug).

### Bottom line

Ship the P0 list (roughly 2–4 focused days), and Pulse is in genuinely strong shape for submission — the integration layer is validated against the newest AMS release, the deployment story is one command, and the documentation depth is beyond anything currently listed on the marketplace. The Kafka and webhook findings are the only places where the implementation diverges from Ant Media's official documentation; everything else is consistency and presentation.

---

## Appendix: Official sources used

- Ant Media Marketplace catalog and listing structure: antmedia.io/marketplace/ and antmedia.io/marketplace/ant-assist-wordpress-plugin/
- Webhooks (events, payload fields, no signing, retry): docs.antmedia.io/guides/advanced-usage/webhooks/
- Kafka producer (property `server.kafka_brokers`, topics `ams-instance-stats` / `ams-webrtc-stats` / `kafka-webrtc-tester-stats`, instance-stat fields): docs.antmedia.io/guides/monitoring/monitoring-ams-with-grafana
- Management REST auth (`ProxyAuthorization`, `POST /rest/v2/users/authenticate`): docs.antmedia.io/guides/developer-sdk-and-api/rest-api-guide/management-rest-apis/
- App-scope JWT (`Authorization: Bearer`): docs.antmedia.io/guides/developer-sdk-and-api/rest-api-guide/jwt-rest-api-filter/
- Plugin development guide (for the plugin-vs-application distinction): antmedia.io/plugin-development-guide/
- AMS releases (3.0.3 latest, 2026-05-05): github.com/ant-media/Ant-Media-Server/releases
- AMS source, password handling in `authenticateUser`: github.com/ant-media/Ant-Media-Server → `io/antmedia/console/rest/CommonRestService.java`
- Registry checks (2026-07-25): registry.npmjs.org (`@pulse/beacon` → 404), github.com/aytekXR/ams-pulse (public, v0.4.1, GHCR package present)
