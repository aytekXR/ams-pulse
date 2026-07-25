# SESSION-105 — 2026-07-25 (late evening) — D-172: act on the third-party marketplace-readiness review (REVIEW-MP-2026-07-25); all 12 issues closed or honestly de-scoped

**Operator input (directive):** a third-party marketplace-readiness review (verdict "NOT YET
READY — but close", 12 issues, P0–P2 checklist) + `/goal make the app ready for the
marketplace. Follow the below review.` This is arrived operator input — the wait-loop is
superseded for this session.

**Protocol reads at gate (green):** prod collector component `ok`; binary v0.4.0-139-gf9e9c69
unchanged; ClickHouse `server_events` 1,302,987 rows, last event seconds-old; backups fresh
(16:13 UTC). NO prod roll this session. Working tree clean at start; single writer.

## Method

Verification-first (the review audited a DIFFERENT clone at `/Users/ae/...`): a 12-agent
read-only wave verified every claim against this tree AND against primary sources (AMS
`StatsCollector.java`, `AntMediaApplicationAdapter.java`, `JWTFilter` docs, registry checks).
Result: all 12 issues held at least partially; several were stale (VERSION/CHANGELOG/README
already 0.4.1; local media existed — the reviewer's clone lacked gitignored dirs; **the v0.4.1
GitHub RELEASE did not exist** — only the tag+image, a defect the review missed). Then an
11-agent implementation wave (work orders in `agents/handoffs/wave-s105/`), orchestrator-owned
follow-ups, and a 5-agent adversarial review before commit.

## Shipped (code)

- **Kafka collector consumes the real AMS feed (Issue 1, P0).** Default topics
  `ams-instance-stats,ams-webrtc-stats` (from AMS source; the old `ams-server-events` is a
  topic AMS never publishes); new `PULSE_KAFKA_TOPICS` env; normalizer routes by topic and
  parses official nested shapes (`instanceId`, `cpuUsage.systemCpuLoad`, `systemMemoryInfo`,
  `fileSystemInfo`) with source-derived testdata fixtures; legacy flat-field parsing kept for
  unknown/custom bridge topics; `ams-webrtc-stats` consumed-and-skipped (no clean domain
  mapping). **Validated against a REAL Kafka broker** (disposable Redpanda; env-gated
  `TestKafka_Integration` produced the fixture and asserted end-to-end normalize: cpu 42.00,
  mem 87.50, disk 50.00). Remains EXPERIMENTAL/PREVIEW in all docs until AV-15 (live AMS
  producer) runs.
- **Webhook accepts the official AMS payload (Issue 2, P1).** `id` fallback for stream id;
  vodReady emits `vod_id` + `duration_ms`; `application/x-www-form-urlencoded` parsed (HMAC
  still verified over raw body first; 200-on-parse-failure retry-storm behavior kept);
  numeric-string-tolerant helpers.
- **Token mode reaches management-scope endpoints (Issue 3, P1).** `amsclient` sends
  `ProxyAuthorization` alongside `Authorization: Bearer` on every GET.
- **Module identity (Issue 6).** All three Go modules renamed
  `github.com/pulse-analytics/pulse/*` → `github.com/aytekXR/ams-pulse/*` (181+ files);
  PagerDuty event `source` → `ams-pulse`; test cert CN de-branded.

## Shipped (packaging / media / docs)

- **v0.4.1 GitHub Release created** (didn't exist; releases page was stuck at v0.4.0 while
  README claimed v0.4.1) — notes + install/cosign instructions; assets:
  `pulse-demo-roughcut.webm` (re-rendered DARK) and `ams-pulse-beacon-0.4.1.tgz`.
- **Media root cause found & fixed (Issue 4, P0 — deeper than the review knew):** Playwright's
  default emulated `prefers-color-scheme` is LIGHT, so the ENTIRE screenshot set and the D-170
  demo rough-cut silently rendered light (that's also the real cause of the historical
  ss1-light byte-dup — both shots were light, not both dark). Both capture scripts now pin the
  theme explicitly (localStorage + colorScheme), assert the light shot applied, resolve
  Playwright relative to the repo (no `/home/aytek` paths), and exit cleanly in containers.
  Fresh dark set + genuine light variant captured and **committed to the repo**
  (`docs/marketplace/screenshots/` un-gitignored — user-guide's 12 images now render on
  GitHub; demo webm stays out of git, lives on the release).
- **Versions (Issue 5, P0):** product.md/faq.md/known-limitations.md → v0.4.1; Chart.yaml
  appVersion 0.4.1 + chart 0.2.0; quickstart `.env.example` pin; OpenAPI declared
  independently-versioned in-file; **release.yml gained a version-consistency guard**
  (VERSION/Chart/docs-headers vs tag, fail-fast).
- **SDK install paths (Issue 7, P0):** npm package renamed `@pulse/beacon` (never published,
  unownable) → **`ams-pulse-beacon`** @ 0.4.1; docs lead with the tarball install that works
  TODAY (release asset), npm-registry path marked pending; release.yml builds+attaches the
  tarball, creates the GitHub Release, and auto-publishes to npm **iff** `NPM_TOKEN` secret
  exists (missing token ≠ failure); Swift README's unresolvable SPM-URL snippet → local-path
  integration; SettingsPage snippet + user-guide + beacon-sdk.md updated; 3.52 KB gate green.
- **Identities (Issue 6):** Chart.yaml home/sources/maintainer → aytekXR/ams-pulse +
  support@beyondkaira.com; OpenAPI license PolyForm NC 1.0.0 + contact; schema `$id`s →
  `pulse.beyondkaira.com`; docs/api/index.html strings aligned.
- **LICENSE (Issue 8):** licensor notice on root LICENSE (PolyForm NC), beacon-js LICENSE
  holder fixed, beacon-swift LICENSE (MIT) added, README license section names the holder.
  Holder text per the review's prescription: "Copyright (c) 2026 Aytek Erdoğan
  (beyondkaira.com)" — **operator: confirm the legal-name form**.
- **Trial story unified (Issue 9, P0):** canonical wording everywhere (14-day Pro, no card,
  request via marketplace listing or support@beyondkaira.com, key by email; NO "self-serve"
  claim until marketplace billing exists); faq Q21 broken cross-ref fixed.
- **Compose default = signed GHCR image (Issue 10):** `deploy/docker-compose.yml` pulls
  `ghcr.io/aytekxr/ams-pulse:0.4.1`; source builds via new `docker-compose.build.yml` overlay
  (tagged `ams-pulse:local-build`, can't masquerade as the signed tag); e2e CI updated to keep
  building from source; **prod path (`docker-compose.prod.yml` + `deployment.sh`) deliberately
  untouched** (S100 lesson).
- **Hygiene (Issue 11):** DRAFT/D-081 banners removed (7 files + developer-meeting-brief);
  NEEDS-OPERATOR rows resolved to the decided state; internal competitive HTML comment
  stripped from listing-draft; VPS IP scrubbed from upgrade-rollback.md (`<VPS_IP>`);
  `.github/ISSUE_TEMPLATE/bug_report.yml` + `config.yml` added; `poem.md` → `agents/poem.md`.
- **AMS-version claim unified (Issue 12):** "Validated live on AMS 3.0.3 Enterprise;
  best-effort 2.10+ via version-tolerance tests (mock profiles)" — the unsupported "v2.8+"
  claim removed. B7 §7 rewritten to the signing-proxy reality (the old text instructed AMS
  console fields that don't exist).

## Gates (all verified by the orchestrator, not trusted from agents)

Server: gofmt clean · `go build` + `go vet` + FULL suite green (26 pkgs) — run in the pinned
`golang:1.25` container because **this host has NO native Go toolchain** (see gotchas). Web:
full vitest suite green. SDK: tests green, size 3.52 KB < 15 KB. qa/mock-ams + qa/licensegen
build. actionlint clean on both changed workflows. Compose base/base+build/base+build+ci all
`config -q` clean. Kafka real-broker integration test PASS. Root-owned ClickHouse artifact
dirs (`preprocessed_configs/`, `access/` under server/internal/*) removed via container.

## Environment gotchas (for the next session)

- **No native Go on this host** — `go`/`gofmt` are NOT installed; use
  `docker run … -v ~/go/pkg:/go/pkg golang:1.25`. (Wave agents that reported running go tests
  natively could not have; the orchestrator re-ran every gate in the container.)
- **Playwright captures MUST pin the theme** — default emulated color-scheme is light.
- The capture container path (`mcr.microsoft.com/playwright:v1.61.1-jammy`, `--network host`,
  repo mounted at `/repo`) is the only local render path (Chromium OS libs still absent).

## New / changed operator items

1. **NPM_TOKEN secret** (their npm account) → next tag push (or `workflow_dispatch` with
   `publish_tag`) auto-publishes `ams-pulse-beacon`. Optional but closes the "npm install"
   listing path.
2. **AV-15 live Kafka validation** — pairs naturally with the load-lane PAYG AMS (set
   `server.kafka_brokers` in red5.properties on that instance; consumer side is now aligned +
   broker-validated).
3. **Demo final** — the rough cut on the release is now DARK (brand-correct); re-record
   voiceover over it or regenerate.
4. **Confirm the licensor legal-name line** in LICENSE/licensing docs.
5. Carried: submit listing · billing setup · load lane · send Ankush reply · optional prod
   roll to 0.4.1 · rotate exposed secrets · optional VPS Chromium deps.

Decision record: `decisions.md` D-172. Work orders: `agents/handoffs/wave-s105/`. PR: see
D-172 addendum for number + CI state.

## Addendum — adversarial-review findings (5-agent refute wave) and resolutions

The pre-commit adversarial wave found **1 code BLOCKER, 2 doc BLOCKERs, and a set of majors** —
all fixed before commit:

1. **[BLOCKER, code] CPU double-scaling.** The new normalizer multiplied
   `cpuUsage.systemCPULoad` by 100 assuming AMS publishes [0,1] — but AMS's `SystemUtils`
   already converts to an INTEGER percent before serializing (`(int)(getSystemCpuLoad()*100)`),
   so real messages would have reported 4200% CPU. Worse, the fixture encoded the same wrong
   assumption (0.42), so unit tests were green while production would be broken — the exact
   plausible-but-wrong failure adversarial review exists for. FIXED: multiplication removed,
   fixture/tests use integer percents, re-validated against a real broker (cpu 42.0).
2. **[BLOCKER, docs] kafka-integration §4.3–4.6 described `ams-webrtc-stats` parsing that the
   code deliberately does NOT do** (it skips those messages). Rewritten to the truth (skip +
   rationale; legacy flat-field parsing documented as the custom-bridge-topic path); same
   overclaim scrubbed from AMS-INTEGRATION §3.7, compatibility (AV-06 + matrix row), LIM-04.
3. **[BLOCKER, deploy] The README evaluator compose path booted an UNMIGRATED ClickHouse and
   never delivered PULSE_SECRET_KEY** — `pulse-migrate` lived only in the auto-loaded override
   (skipped by `-f deploy/docker-compose.yml`), and the base file's secret env vars were
   commented out, so deploy/.env values never reached the container. FIXED: image-based
   `pulse-migrate` one-shot added to the base file (+ `depends_on: service_completed_successfully`),
   secrets/AMS vars now substituted from deploy/.env; build/ci/override overlays all pin
   `image: ams-pulse:local-build` on BOTH services (tag-masquerade guard) and `make up` uses
   the build overlay explicitly (no more GHCR-pulse + source-migrate version split).
4. **Majors/minors fixed:** release.yml CI gate now certifies `git rev-parse HEAD` (the
   publish_tag checkout), not the dispatch-branch `GITHUB_SHA`; version guard fails on a
   MISSING doc stamp and now also checks sdk/beacon-js/package.json; cosign example tag
   de-v-prefixed; ProxyAuthorization stripped on cross-host redirects via CheckRedirect
   (Go's sensitive-header list doesn't cover the non-standard header) + test; webrtc-skip
   debug log re-arms per broker session; beacon install links pinned to the v0.4.1 release
   tag URL; operator-expected item-4 Kafka row de-staled; stale `ss2-stream-detail.png`
   deleted and screenshot-list/list-draft counts corrected; Helm ClickHouse image
   digest-pinned (same digest as compose) and render-verified.
5. **Declined (deliberate):** rewriting historical `ams-server-events` mentions inside
   decisions.md / superseded RESUME-PROMPT blocks — those are append-only session records;
   D-172 and the new START HERE record the rename.
