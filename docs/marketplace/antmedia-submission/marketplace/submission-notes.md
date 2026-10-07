# Submission notes — verification record

How this package was produced, what was run, what changed, and what was found. **Internal to the
developer** — share selectively with Ant Media.

Two passes: **2026-10-07 (§0)** — the developer's decisions, v0.5.0 and a live run against a real,
licensed **AMS 3.1.0**; and **2026-10-01 (§1–§6)** — the original audit, kept as the record of what
was found and fixed then.

---

## 0. 2026-10-07 — v0.5.0: Pulse is free; tested live on AMS 3.1.0

### 0.1 Decisions applied

| Item | Decision |
|---|---|
| Developer credit | Aytekin Erdogan — individual developer (beyondkaira.com); no company |
| Pricing | Free: every feature, no license key, commercial use included; at least the first year |
| License | PolyForm Shield 1.0.0 (server, web UI, deploy tooling); MIT for the beacon SDKs |
| Purchase link | None. The page's main button points at the install page `…/ams-pulse/get/` |
| Page title | "Pulse for Ant Media Server" |
| Analytics accuracy defects D1–D4 | Not fixed for the listing; disclosed as LIM-30 and scheduled (ROADMAP-V2 §2.49) |
| Release | v0.5.0 (screenshots retaken from it) |

### 0.2 Changes

| # | Change | Verification |
|---|---|---|
| F7 | **All features free.** `license.Manager.SetAllFeaturesFree`, switched on in `cmd/pulse` (`newLicenseManager`); every `Check*` gate passes and entitlements are unlimited whatever key is loaded; `GET /admin/license` adds `all_features_free` (contract first); the web UI gates through `gatingTier()`; Settings → License says Pulse is free, hides the key form and shows "Unlimited" (fixes D11) | New Go tests: every gate open for keyless, paid, expired and refreshed managers, the enforcing default intact, `-race` toggle; API test proving 8 formerly gated endpoints (control: each was 403 under enforcement); `serve` wiring test; web unit tests per gated page; a Playwright spec |
| F8 | **License → PolyForm Shield 1.0.0** (verbatim official text); copyright names Aytekin Erdogan; docs, website and package rewritten for the free model | Text diffed byte-for-byte against the PolyForm 1.0.0 source |
| F9 | **AMS login backoff (found live on AMS 3.1.0).** A rejected login retried on every 5 s poll, and AMS locks an account for 300 s after two failures, so a wrong password (or Pulse starting before the AMS admin existed) kept the account locked indefinitely. Now 1, 2, 4, 8, 15 min, never sooner than the lock AMS reports; a 401/403 re-logins at most once a minute | 8 new tests; 5 fail against the old code (red proven in a scratch worktree) |
| F10 | **`/metrics` token.** Every install now serves `/metrics`; the quickstart installer generates `PULSE_METRICS_TOKEN` (kept on re-runs); the quickstart, base and production compose files pass it through | Test stack: 401 without/with a wrong token, 200 with it |
| F11 | **D14 fixed** — the license context fetches again after the first sign-in (`pulse:auth:token`) | Unit tests (provider, client) |
| F12 | Release prep: version pins → 0.5.0, Helm chart 0.4.0, goldens regenerated with Helm 3.17.0 | The release workflow's own version guard, run locally: PASS |
| F13 | **Alert-rule form offered metrics the server refuses (found via TC-H-06 on AMS 3.1.0).** For threshold rules it offered `cpu_pct`, `mem_pct`, `packet_loss_pct`, `jitter_ms`, `rtt_ms`, `health_score` — never evaluated, refused with 422 since v0.4.1 — and omitted `node_cpu`, `node_mem`, `node_disk`, `stream_offline` and others, so no CPU/memory threshold alert could be made in the UI. Now exactly the server's list (four API-only metrics, each with a stated reason); editing shows the stored metric; switching rule type maps `node_cpu` ↔ `cpu_pct`; contract and docs list the names | A guard test reads the server's Go lists; 5 new form tests fail against the old code |
| F14 | **Refused saves were invisible.** A rule or channel save the server rejected did nothing visible (unhandled rejection); now a toast gives the server's reason and the form stays open with the input | Page tests; the rule-save test fails against the old code |

### 0.3 Environment

- **AMS:** the old install (3.0.3, expired trial, unreachable since 2026-08-12) was removed with its
  data volume. `antmedia/enterprise:3.1.0` (build 20260831_1439) now runs with host networking and
  the operator's new trial license (valid until **2026-10-16**). Admin account recreated with the
  credentials Pulse uses; the `pulse-test` application recreated for the scenarios.
- **REST IP filter** on every application: `127.0.0.1,172.16.0.0/12,<host public IP>/32` (the
  host, its Docker networks, and its own public address, which the test harness uses). The old
  install allowed `0.0.0.0/0`, which exposed each application's REST API to the internet without
  authentication.
- **Prod Pulse is no longer blind:** with AMS reachable again it resumed ingest on 2026-10-07
  (99 server events in the first 10 minutes, verified in ClickHouse).

### 0.4 Test results

| Run | Result |
|---|---|
| Real-AMS sweep (`qa/realams`, 61 scenarios, AMS 3.1.0) | **44 pass, 10 skip, 7 fail.** The harness prints 43 pass + 1 `NOEVID`; that row is TC-I-05-SRT, which passed 2/2 — its evidence directory does not match the summary's glob (harness naming defect). Of the 7 failures, 5 are expectations written for AMS 3.0.3: FL-01/H-01 expect CPU/memory `null` on a standalone node, and Pulse now reports the real values (55 %, 81 %) from `/rest/v2/system-resources`; FL-02 expects version 3.0.3; P-03 expects the RTMP probe to stop at `handshake_complete`, and it now reaches `app_accepted`; WH-03 expects `recording_gb = 0` (vodReady not delivered), and the recording is now counted. L-01 is the AMS CPU guard refusing a new stream on the loaded test host (96 % > 75 %). **TC-H-06 exposed a real defect (F13):** it creates a threshold rule on `cpu_pct`, which passed in July and has been refused since v0.4.1 — the scenario is outdated, and so was the UI, which offered that name. Skips: preconditions (no VoD, no blocked app, no SRT port, VoD-poll flag unset, no webhook capture or signing proxy), the host's RTMP capacity (LIM-12) and HLS viewer-count inflation (LIM-02) |
| Every UI page in a real browser | 33/34 — the one "defect" (`/audit` 404) refuted: the route is `/audit-log` |
| Every API endpoint against the contract | 100/100, create/update/delete included |
| Alert delivery (real stream; e-mail and signed webhook) | 15/16, one documented skip (wildcard-rule wording, D6–D8, not exercised). The bitrate rule fired at 909.32 kbps and resolved at 2,640.6 kbps; stream-offline fired; every webhook's HMAC-SHA256 signature verified |
| Player QoE (real HLS playback with the beacon SDK) | 14/14 — startup p50 885.5 ms, p95 1,151.5 ms, rebuffer 0, errors 0; audience analytics shows 0, as LIM-30 documents |
| AMS login backoff (live, wrong password) | 9/9 — backoff 1 → 2 → 4 → 8 min observed; the 5-minute AMS lock respected; a person can sign in to AMS once the lock lapses; the right password logs in at once |
| Quickstart installer on AMS 3.1.0 | 19/19 — local-image fallback, `.env` mode 600, metrics token (401 without, 200 with), `all_features_free`, idempotent re-run |
| Go: `gofmt`, `vet`, build, `test -race` (golang:1.25) | Pass — 26/26 packages, 1,982 tests, 0 failed, 4 skipped (Kafka, `npx` and poppler are absent on this host) |
| Web: generated-types drift, build, lint, unit tests, Playwright | WEB_GATES_RESULT |
| Beacon SDK | 70/70 tests; 3.52 KB gzipped (budget 15 KB) |
| Release version guard (`release.yml`, run locally), doc stamps, website checks, Helm lint, ShellCheck 0.9.0 and 0.11.0 | Pass |

---

## 1. Build and test results

Tree: `main` @ `0cae261` (v0.4.5 + 10 commits) plus this session's changes (§3). Final run
recorded at the end of the session, after every change in §3 was in place:

| Gate | Result |
|---|---|
| Go `gofmt -l` (server) | Pass — no unformatted files |
| Go `go vet ./...` | Pass |
| Go `go build ./...` (`CGO_ENABLED=0`) | Pass |
| Go `go test -race -count=1 ./...` (repo-root mount, `golang:1.25`) | Pass — 26/26 packages, 1,952 tests passed, 0 failed, 0 data races. 4 skipped because they need something this host lacks (Kafka integration; two schema-fixture tests that need `npx`; a PDF check that needs poppler) |
| Web `lint` (ESLint) and `typecheck` (`tsc --noEmit`) | Pass — 0 errors |
| Web unit tests (vitest), 788 tests | **Not deterministic on this host** — 785/788 and 787/788 in two full runs; every failure is a timing failure in a test this session did not touch (note below) |
| Web production build (`tsc -b` + Vite) | Pass |
| Web end-to-end (Playwright, chromium, full suite) | Pass — 66/66 |
| Beacon SDK build / tests / size budget | Pass — 70/70 tests; 3.52 KB gzipped (budget 15 KB); lint 0 errors (1 warning) |
| AMS simulator tests (`qa/mock-ams`) | Pass — gofmt, vet and tests |
| Docker image build (`deploy/docker/pulse.Dockerfile`) | Pass — version `0.4.5-10-g0cae261-dirty`, 24.8 MB. The demo stack was re-run on this image: `/healthz` `ok` with the collector component `ok`; live API 1,312 viewers and 8 streams; beacon 202 with a valid token and 401 with a bad one; API 401 without a token; a fresh browser shows no "Session expired" (F1) |
| Helm `lint` + `template` (Helm 3.17.0, documented values) | Pass — 1 chart linted, 0 failed (info only: chart icon recommended); renders |
| ShellCheck 0.9.0 and 0.11.0 (installer + demo scripts) | Pass — no findings in either version |
| Website checks (`website/tests/run-checks.sh`, root and `/ams-pulse/`) | Pass — both hosting modes |
| Doc-stamps guard (`.github/check-doc-stamps.sh origin/main`, as CI runs it) | Pass — 15 stamped docs, including touch-the-stamp against `origin/main` |
| Released image signature — the page's `cosign verify` command, Cosign v3.1.3 | Pass — the released 0.4.5 image `ghcr.io/aytekxr/ams-pulse` (`internal/evidence/cosign-verify-2026-10-01.txt`) |

**About the vitest result.** The build host was shared with an unrelated job (load average 30–75 on 6
cores) throughout the final run. Default 5 s per-test timeout: **785/788** — three tests in
`OnboardingWizard.verify` and `TenantsTab` timed out at 5.5–7.1 s. With `--testTimeout=30000`:
**787/788** — those three passed, and a different test failed: `SettingsPage.interactions` › "Add source
button shows the onboarding hint toast" waits only until the API is *called*, then looks for the button
before it renders (a latent race in the test; `findByRole` would fix it). Each failing file passes
when re-run alone. None of these tests, or the code they cover, changed in this session. CI's `web`
job passed the same suite on the same base commit (run `36653444285`, parent `0cae261`). The
session's new AuthGate regression test passed in every run.

## 2. What was run

**Real application, simulated AMS.** All product screenshots come from the isolated demo stack in
`qa/marketplace/demo-stack/`. It runs the Pulse image built from this tree
(`0.4.5-10-g0cae261-dirty`, demo Business license minted with a throwaway key), a real
ClickHouse, Pulse's AMS simulator (`qa/mock-ams`, application `LiveApp`), and local
SMTP/webhook sinks. Viewer traffic was synthetic: 1,200 replayed sessions spread over 24 hours,
then live sessions, all POSTed to the public `/ingest/beacon` endpoint. Nothing was written to
the databases directly.

**Published install path.**

| Run | Result |
|---|---|
| Ant Media's draft command (no `--password`) | **Fails:** `Error: --password is required (no TTY available for interactive prompt)`, exit 1 |
| Published installer (`main`), v0.4.5 image from GHCR, fresh host | Exit **0** in **73 s**; collector verified; first sign-in lands on the live dashboard |
| Corrected installer (this session), same image, clean container | Exit **0** in 91 s (incl. container setup); new next-steps text |

**Alert delivery, end to end** (incident staged on the simulator, 2026-10-01 UTC):

| Time | Event |
|---|---|
| 20:03:18 | `studio-b` ingest drops to ~850 kbps; `keynote-hall` viewers start rebuffering |
| 20:03:44 | `backstage-cam` taken offline |
| 20:04:22.105 | *Stream went offline* fires (critical) → e-mail 20:04:22.845 |
| 20:04:32.121 | *Ingest bitrate below 1,500 kbps* fires at 842.976 kbps → e-mail 20:04:32.934, signed webhook 20:04:33 |
| 20:04:47 | *Stream went offline* resolves (stream still offline — see D8) → e-mail 20:04:48 |
| 20:10:24 | Incident cleared |
| 20:10:32 | Bitrate alert resolves → e-mail 20:10:33.248, signed webhook 20:10:33 |
| — | *Viewer QoE degraded (rebuffer > 5%)* **never fired**, although the synthetic rebuffering on that stream was ≈ 8% — see D4 |

## 3. Changes made in this session

| # | Change | Why | Verification |
|---|---|---|---|
| F1 | `web/src/components/AuthGate.tsx` — show "Session expired or token revoked" only if a token existed | A first-time visitor saw that error on the sign-in page after a fresh install (pre-login `/admin/license` → 401) | New regression test; AuthGate 15/15; fresh-browser check on the rebuilt image: message gone |
| F2 | `deploy/quickstart/install.sh` — (a) use an already-present local image when the registry pull fails; (b) correct the build-from-source hint (`docker build …`, not `make build`); (c) next steps no longer promise a wizard that does not appear | The documented offline/side-load and build-from-source paths could not work; the "4-step wizard" message did not match the product | ShellCheck 0.9.0 + 0.11.0 clean; real run exit 0 |
| F3 | `qa/mock-ams` — `/rest/v2/system-resources` in the real AMS 3.0.3 shape | Simulator showed 0 % CPU/RAM; Pulse prefers this route on real AMS | New test (red → green); dashboard shows CPU/RAM |
| F4 | Docs: `docs/troubleshooting.md`, `docs/faq.md`, `docs/runbooks/install.md`, `docs/runbooks/alerting.md` | Broken fallback advice; wizard claim; `/metrics` documented as 401 but served without auth; SMTP password documented as unencrypted (it is encrypted); UI-edit hazard (D6) | Each claim checked against code or the running stack |
| F5 | `website/` — FAQ claims about IP hashing and origin/edge dedup corrected; "Get Pulse" page added | Both FAQ claims were false; Ant Media needs a CTA target | `website/tests/run-checks.sh` (§1) |
| F6 | New tooling: `qa/marketplace/demo-stack/`, `qa/marketplace/capture-real-stack.mjs`, `qa/marketplace/tools/`, `qa/marketplace/build-submission-zip.sh` | Reproducible, non-mocked screenshots and the ZIP | Used to produce this package |

The served product (`server/`) is unchanged; the image differs from v0.4.5 by the UI polish
already on `main` (#252) and F1.

## 4. Defects found and **not** fixed (developer decision needed)

Severity is about marketplace impact. The first four share a cause: viewer sessions written
once per heartbeat, and heartbeat `watch_ms` sent as a running total, flow into rollups that
assume one row per session and per-interval values. Fixing them changes the data model
(materialized views and migrations) and needs a design decision, so they were documented, not
patched.

### Accuracy check used

One beacon session with a known truth on an isolated stream (`seed-demo.mjs truth`): **1 view,
600 s watched, one 6 s stall (rebuffer ratio 1.0 %)**, sent in the SDK's own format (20
heartbeats, cumulative `watch_ms`).

| Where | Reported | Truth |
|---|---|---|
| `GET /api/v1/qoe/summary` rebuffer ratio | **0.095 %** | 1.0 % |
| ClickHouse `rollup_audience_1h` views / watch time | **20 views / 6,300 s** | 1 / 600 s |
| ClickHouse `rollup_usage_1d` viewer-minutes (feeds usage reports) | **115** | 10 |
| ClickHouse `viewer_sessions FINAL` | 10 min | 10 min ✔ |
| `GET /api/v1/analytics/audience` | **0 views** (query error swallowed) | 1 |

Raw output: `internal/evidence/accuracy-check-2026-10-01.txt`.

| ID | Defect | Impact | Root cause | Suggested fix |
|---|---|---|---|---|
| **D1** — high | **Audience analytics always shows 0** (views, uniques, watch time, peak) whenever data exists | Pro+ "historical analytics" headline view is empty; CSV export empty | `query.go:330-331` returns `UInt64` from `countMerge`/`uniqMerge`; `AudienceBucket` fields are `int64` (`query.go:526-527`) → clickhouse-go `converting UInt64 to *int64 is unsupported`; `api/server.go:1324-1327` swallows the error and returns zeros without logging | Cast in SQL (`toInt64(countMerge(views))`, as the other two columns already do) **together with D2**; log query errors. Add a ClickHouse integration test that reads non-empty data |
| **D2** — high | Audience rollups count every session **upsert** as a view and sum running watch totals | Once D1 is fixed, views and watch time would be inflated ~(heartbeats + 1)/2 | `mv_audience_1h/1d` (`0001_init.sql:380-382, 399-401`) aggregate every inserted `viewer_sessions` row; the stitcher writes a row per heartbeat (`sessions/stitcher.go:254`) | Count `uniq(session_id)` for views; feed watch time as per-upsert deltas or from final rows only; re-backfill rollups |
| **D3** — high | **Usage-report viewer-minutes overstated** (×11.5 for a 10-minute view; egress estimate inherits it) | Business-tier billing reports; contradicts the "reconciled to ±1 %" claim in `listing.md` | `mv_usage_1d` (`0001_init.sql:460`) sums `watch_time_s/60` of every upsert; `reports/accounting.go:225` reads it | Same delta/final-row fix as D2; make `pulse diag reconcile` compare against `viewer_sessions FINAL` on real SDK-shaped data |
| **D4** — high | **QoE rebuffer ratio and error rate understated** — rebuffer by (n+1)/2 for n heartbeats (×10.5 for 10 minutes); error rate divides by QoE rows, not sessions | QoE page understates problems; `rebuffer_ratio` / `error_rate` alert rules under-fire (demo: ≈ 8 % actual vs 5 % rule — never fired) | `mv_qoe_1h/1d` (`0001_init.sql:419-421`) sum cumulative `watch_ms` (the SDK sends a running total — `sdk/beacon-js/src/hls.ts:158-165`) and use `countState()` as `session_count` | Store per-heartbeat deltas (server-side, per session) or aggregate max per session; count distinct sessions for error rate |
| D5 — low | Daily audience queries drop the first, partial day | "Last 24h" daily view shows only today | `buildTimeWhere` (`query.go:1402-1413`) compares a `Date` bucket with the exact `from` timestamp | Truncate `from` to the bucket granularity |
| D6 — medium | **Editing an e-mail channel in the UI drops its SMTP settings** | E-mail alerts silently stop after an innocent edit | UI form sends only `email_to` (`AlertChannelForm.tsx:55`); `PUT` replaces the config | Merge config on update, or add SMTP fields to the form (contract `AlertChannelConfig` lacks them too) |
| D7 — medium | Notifications for all-stream rules don't say **which stream**; e-mail has no link; History lists rule **IDs** | On-call cannot tell what broke from the e-mail; History is hard to read | E-mail body prints the rule's scope (`channels.go:231-233`), not `group_key`; History table renders `rule_id` (`AlertsPage.tsx:480-488`) | Include `group_key` and a `PULSE_BASE_URL` link in e-mail; show rule name + subject in History |
| D8 — medium | Wildcard *stream offline* sends **RESOLVED ~25 s after FIRING** while the stream stays down | False all-clear | Edge-triggered semantics for wildcard rules (documented in `docs/runbooks/alerting.md`) | Keep firing until the stream returns or a grace period expires; at minimum say "no longer tracked" instead of RESOLVED |
| D9 — low | Dashboard shows sub-1 Mbps bitrates unrounded ("904.002 Kbps") | Cosmetic | `StreamsTable.tsx:44` | Round kbps |
| D10 — low | Health score lenient: 81/100 "healthy" at 45 % of target bitrate | Score alone misses encoder drops | Score weights | Revisit weights or surface bitrate-vs-target |
| D11 — low | Settings → License shows raw `null` (unlimited streams); key placeholder `PULSE-XXXX-…` does not match the real format | Cosmetic / confusing | License tab rendering | Render "Unlimited"; realistic placeholder |
| D12 — low | Settings → Sources says "No AMS sources configured" on installs configured by environment | Confusing on the first visit to Settings | Sources tab lists DB sources only | Show the environment-configured source read-only |
| D13 — doc | Several docs say viewer IPs are "SHA-256 hashed before storage"; in fact **no viewer IP is stored** (`HashIP` is never called; `ip_hash` is never populated) | Inaccurate privacy claim (understates the real posture) | — | Website FAQ fixed (F5); also update `docs/marketplace/submission-process.md` §4 and any copy reused from it |
| D14 — low | After the **first** sign-in in a browser, the sidebar tier label and the trial-countdown banner stay hidden until the page is reloaded. Tier-gated pages are unaffected (each fetches the license itself) | Cosmetic; a trial user does not see the days-remaining banner on first sign-in | `LicenseProvider` (`web/src/lib/LicenseContext.tsx:64-80`) fetches `/admin/license` once, on mount. On a first visit that is before sign-in, so it gets 401, and nothing re-fetches once the token is set. Found while verifying F1 on the final image | Re-fetch the license when the auth token changes |

Evidence images: `internal/evidence/defect-*.png` and `internal/evidence/minor-*.png`.

**Marketing consequences until D1–D4 are fixed:** do not publish the analytics, QoE or usage-report
pages as screenshots (none are in `assets/`), and do not quote "viewer-minutes reconciled to
±1 %" or QoE ratios. `docs/marketplace/listing.md` still contains the ±1 % claim — it carries a
warning banner now.

## 5. Not verified in this session

- **A real Ant Media Server.** The operator's AMS container is unreachable from the host (bridge
  mode, no published ports since 2026-08-12) and its trial license expired on 2026-07-28. The
  46/50 live validation on AMS 3.0.3 is from July 2026.
- Slack, Telegram and PagerDuty delivery (no accounts used); e-mail and webhook were verified.
- Helm on a real cluster (rendered and linted only).
- Capacity / load (harness ready, needs a dedicated AMS).
- Light theme (all captures use the default dark theme).

## 6. Screenshot provenance

| Assets | Build | Data |
|---|---|---|
| `assets/screenshots/*`, `assets/hero/*`, `walkthrough/step-4a-sign-in.png` | **Retaken 2026-10-07** from the v0.5.0 release candidate built from this tree (`0.5.0-rc1`), keyless — every feature free, sidebar reads FREE | Demo stack: simulated AMS, synthetic viewers (1,192 replayed sessions + live traffic); incident staged 11:42–11:45 UTC |
| `walkthrough/step-1-install-output.png` | Corrected installer (F2) + released v0.4.5 image (2026-10-01) — to be retaken with the released v0.5.0 image | Real run; token and password masked; AMS URL is the simulator's bridge address |
| `walkthrough/step-4b-dashboard-first-run.png` | Released v0.4.5 image via the published installer (2026-10-01) — to be retaken with v0.5.0 | Real first sign-in, simulator data |

Hero facts, from this run's records: the bitrate rule fired on `studio-b` at 11:43:44.104 UTC at
894.46 kbps; the e-mail reached Mailpit at 11:43:44.711 (0.6 s) and the signed webhook arrived the
same second; it resolved at 11:45:04. Raw history: `internal/evidence/alert-history-incident.json`.
