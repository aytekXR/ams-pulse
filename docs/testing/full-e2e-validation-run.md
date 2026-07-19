# Pulse ↔ Ant Media Server — Full E2E Validation Run (implementation record)

> **What this is:** the *verified implementation* of the operator-supplied "Full End-to-End Validation Run"
> plan. Every proposed test in that plan was checked against the **actual Pulse code** before anything was
> written — the plan was authored against AMS *source*, so a large fraction of its Go snippets targeted
> gaps that are already covered or used wrong names/behavior. Following the plan's own #1 rule (**no
> false-greens**), only genuine, non-redundant gaps were implemented, and every new Go test was **run green**.
> Live-AMS scenarios are egress-blocked in this environment, so they are packaged for the VPS (corrected
> against the real harness API) and `bash -n`-checked, not run here.
> **Authored:** 2026-07-17 · **Prod:** `v0.4.0-98-g641b4e2` · **AMS:** 3.0.3 EE.
> **Companion:** `docs/testing/e2e-ams-test-design.md` (the base test-design doc).
> **Rev 2 (2026-07-19):** added the opt-in **load lane** (§7) per Ant Media's "verify how your stats hold
> up under load" pointer, and the **panel-revamp / marketplace** assessment (§8). Same #1 rule applies:
> the operator plan's load scripts were **verified and rewritten against the real harness API** before
> anything was committed — see §7.3 for the corrections.

---

## 1. Executive summary

- **Verification first.** A 3-scout workflow classified every gap in the plan against the real code. Of the
  ~20 proposed Go tests, **only 6 were genuine gaps**; the rest were already covered, or the plan's claim
  was wrong.
- **4 new Go tests shipped and run green** (G-01, G-09, G-13, G-20). 2 more (G-08, G-16) are left as
  document-only recipes because a correct test needs a code change (injectable clock / real-WS + controllable
  provider harness) — a flaky test would violate no-false-green.
- **9 live scenarios + an orchestrator + a drift-watch** packaged for the VPS, rewritten against the real
  `qa/realams` harness API (the plan's versions had systematic harness-API errors — see §5).
- **Nothing prod-facing changed** (tests + qa scripts + docs only) → no prod deploy.

---

## 2. Verification results — every proposed gap vs the real code

| Gap | Plan's claim | Reality (evidence) | Disposition |
|---|---|---|---|
| **G-01** | `normalizePublishType` SRT untested | **REAL GAP** — the two-param `normalizePublishType` (normalize.go:291) had no collector-package test; the webhook pkg has a *different* single-param version | ✅ **Implemented** — `normalize_publishtype_test.go` |
| **G-03** | packet-loss `PacketLostRatio` ×1000 → per-mille | **CLAIM WRONG** — field is `Video/AudioPacketLostRatio` (client.go:118); conversion is ×100 → **percent** (normalize.go:187); already tested (normalize_test.go:376) | Document-only |
| **G-04** | apps envelope polymorphism untested | **ALREADY COVERED** — `TestListApplications_DecodesEnvelope` + `_ObjectFormStillDecodes` (client_test.go:369/399) | Skip (redundant) |
| **G-05** | VoD duration ms untested | **ALREADY COVERED** — `vods_test.go:62` asserts ms; field comment authoritative | Skip (redundant) |
| **G-06** | webhook HTTP handler untested | **ALREADY COVERED** — `TestHMACAccepted/RejectedBadSignature/RejectedMissingSignature` (webhook_test.go:76/93/110). NB real accessor is `HTTPHandler()` not `Routes()`; `New` takes 3 args | Skip (redundant) |
| **G-07** | beacon 64 KB cap 413-vs-read-error untested | **ALREADY COVERED** — beacon_test.go:258 (413) + :289 (400 read-error) | Skip (redundant) |
| **G-08** | supervisor backoff cap/reset untested | **REAL GAP but not cleanly testable** — backoff uses real `time.After(100ms…60s)` with no injectable clock; a cap test needs minutes of sleeps, a window-count test is flaky | 📝 **Recipe** (§4) |
| **G-09** | stitcher leave-without-join untested | **REAL GAP (partial)** — heartbeat-creates-session + `EvictIdle` are covered; the leave-without-join early return (stitcher.go:163) was not. (Plan misnamed `EvictIdle` as `SweepStale`.) | ✅ **Implemented** — `stitcher_leave_test.go` |
| **G-10** | CH batch atomicity untested | **ALREADY COVERED** — drain_test.go:586/606/630 (single-batch, never-partial, atomic-failure) | Skip (redundant) |
| **G-11** | ingest fps=-1 / TS≤0 untested | **ALREADY COVERED** — health_test.go:308 (fps=-1) + :435 (TS≤0) | Skip (redundant) |
| **G-12** | unknown-protocol probe honesty untested | **ALREADY COVERED** — `TestProbe_NotProbed` (prober_test.go:470) | Skip (redundant) |
| **G-13** | report-export CSV body content untested | **REAL GAP** — export_test.go checks status + header only; csv_safety checks neutralization only; no golden data-row assertion | ✅ **Implemented** — `export_csv_golden_test.go` |
| **G-14** | anomaly `WarmHysteresis` untested | **ALREADY COVERED** — anomaly_flagstore_test.go:441/505 + s70_d132:187 | Skip (redundant) |
| **G-16** | WS snapshot→delta sequence untested | **REAL GAP** — existing WS tests assert auth only (no real dial / message read). A correct test needs a real `websocket.Dial` + a controllable `LiveProvider` for a deterministic delta | 📝 **Recipe** (§4) |
| **G-17** | ProbeRow CRUD untested | **ALREADY COVERED** — `TestMetaStore_Probes_RoundTrip` (meta_coverage_test.go:30) all 5 ops | Skip (redundant) |
| **G-19** | token-kind isolation untested | **CLAIM WRONG (part 2)** — part 1 covered (v3b_guard_test.go:503, 403 WRONG_TOKEN_KIND). Part 2: api-token on `/ingest/beacon` returns **401 UNAUTHORIZED**, not 403 WRONG_TOKEN_KIND (server.go:2271 conflates wrong-kind with not-found to avoid leaking token metadata) | Document-only |
| **G-02** | kafka 4-way viewer sum untested | **ALREADY COVERED** — kafka_test.go:205/245 (dash included; matches REST) | Skip (redundant) |
| **G-20** | restpoller `poll()`/`pollApp()` direct untested | **REAL GAP** — existing direct-`poll()` test uses an empty broadcast list (pollApp is a no-op); Run()-based tests use goroutines | ✅ **Implemented** — `restpoller_poll_direct_test.go` |
| **G-21** | AMS cluster paths are a wire bug | **UNVERIFIABLE HERE** — Pulse calls the unpaginated `/rest/v2/cluster/nodes` (client.go:496), 404-tolerant, already tested. Whether AMS 3.0.3 *requires* `/nodes/{offset}/{size}` is an **AMS-source claim** that needs a live cluster to confirm | ⚑ **Flag operator** (§6) |

**Bottom line:** committing the plan's Go snippets verbatim would have added ~8 redundant tests, ~2 tests asserting
*wrong* behavior (false-red or misleading), and 1 pin-test for a code change that was never made.

---

## 3. New Go tests (implemented, run green)

All four compile and pass in the repo's Docker Go toolchain; the affected packages
(`collector`, `collector/sessions`, `collector/restpoller`, `reports`) stay fully green with `go build ./...` OK.

| Test file | Gap | What it pins |
|---|---|---|
| `server/internal/collector/normalize_publishtype_test.go` | G-01 | The full `normalizePublishType` switch via `NormalizeBroadcast`: RTMP/webrtc/hls/mp4 mapping, the SRT-string→`other` and `liveStream`→`rtmp` fallbacks, and the AMS SRT-as-RTMP-mislabel (RTMP→`rtmp`). |
| `server/internal/collector/sessions/stitcher_leave_test.go` | G-09 | A `viewer_leave` for a never-joined (or empty) viewer_id is a no-op: `ActiveCount()==0`, zero fabricated sessions. |
| `server/internal/reports/export_csv_golden_test.go` | G-13 | Golden data-row values of `WriteUsageCSV` (the `/reports/export` writer): column order, number formatting, and nil `StreamID`/`Tenant` → empty cells. |
| `server/internal/collector/restpoller/restpoller_poll_direct_test.go` | G-20 | A synchronous `poll()` over a non-empty broadcasts/list emits `publish_start` (+ `publish_type`) and `stream_stats` with the bits/sec→kbps conversion (624000→624). |

Run them:

```bash
docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod \
  -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 \
  go test ./internal/collector/... ./internal/reports/... -count=1
```

---

## 4. Recipes (not committed as tests — would need a code change)

**G-08 — supervisor backoff.** `collector.go` `supervise()` sleeps via `time.After(100ms…60s)` with no
injectable clock, so asserting the 60 s cap needs minutes of real sleeps and a restart-count-in-a-window test
is timing-flaky (and is already weakly covered by `restpoller/latency_test.go`). *To make it testable:* extract a
pure `computeBackoff(attempt int) time.Duration` and unit-test the doubling + 60 s clamp; or inject a
`sleep func(time.Duration)` into `Collector` and assert the sequence + the clean-exit reset to 100 ms.

**G-16 — WS snapshot→delta.** The existing `/live/ws` tests assert auth status only (they never open a real
socket). *To make it testable:* use `nhooyr.io/websocket` (already a dep) to `Dial` the httptest server with a
valid token, read frame 1 and assert `type=="snapshot"`; then push through a controllable `LiveProvider` (or
call the aggregator's subscribe/broadcast path) and assert the next frame is `type=="delta"`. Keep it off the
5 s snapshot ticker to stay deterministic.

---

## 5. Packaged VPS artifacts

Live-AMS work is egress-blocked here, so these are **packaged for the VPS** and `bash -n`-checked, not run.
They were **rewritten against the real `qa/realams` harness API** — the plan's originals had systematic errors
that would crash or false-green every scenario:

| Plan's version | Real harness API |
|---|---|
| `scenario_verdict "$S" "$checks" "$verdict"` (3 args) | `scenario_verdict` (**0 args**; reads `$EVIDENCE_DIR/checks.txt`) + `exit $?` |
| `start_publisher "$ID"` (1 arg) | `start_publisher ID APP KBPS` (**3 args**) |
| `assert_gte A B LABEL "$checks"` (4 args) | `assert_gte A B LABEL` (**3 args**) + trailing `|| true` (under `set -e`) |
| manual `echo "FAIL …" >> checks.txt` | only `assert_*`-written lines count toward the verdict |
| `${PULSE_URL}/api/v1/live/…` | `${PULSE_URL}/live/…` (PULSE_URL **already includes** `/api/v1`) |
| `.streams[]` in the live list | `(.items // [])[]` |

Files:
- `qa/realams/run-full-e2e.sh` — one-command phased runbook (preflight → auth → Go unit+floor → integration →
  web → sdk → qa units → CI e2e → playwright → real-AMS validate-all → **TC-*-1x pack** → budgets → `REPORT.md`).
- `qa/realams/scenarios/TC-*-1x-*.sh` — 9 new scenarios: A-10 count-parity, P-10 live-HLS-probe, I-10
  SRT-publishType, L-10 hostile-streamId, REC-10 VoD-duration-ms, V-10 HLS-inflation, H-10 WS-snapshot-delta,
  WH-10 hook-wire-capture (settings-mutating, guarded), WH-11 signing-proxy-e2e (settings-mutating, guarded).
- `qa/tools/ams-drift-watch.sh` — nightly AMS release-drift watch (G-26).

Run on the VPS:

```bash
cd /home/aytek/repo/ams-pulse
bash qa/realams/run-full-e2e.sh            # full phased run → REPORT.md under $EVIDENCE_ROOT
# settings-mutating webhook scenarios are opt-in and self-restoring:
ALLOW_SETTINGS_MUTATION=1 bash qa/realams/scenarios/TC-WH-10-hook-wire-capture.sh
```

Standing safety rules (unchanged from the base doc): one console-auth attempt per run; never negative-test
console login (5-min lockout on the shared account); settings mutations are snapshotted and restored in the
EXIT trap.

---

## 6. Flagged for the operator (only you can resolve)

1. **G-21 — cluster REST pagination (AMS-source claim, unverifiable in this sandbox).** The plan claims AMS
   3.0.3 requires `GET /rest/v2/cluster/nodes/{offset}/{size}` and that Pulse's unpaginated
   `/rest/v2/cluster/nodes` therefore 404s on a real cluster (silently degrading fleet discovery to
   standalone). Pulse's current call and its 404-tolerance **are tested and correct for what they do**; the
   AMS-side requirement was **not** independently confirmed here (no live multi-node cluster; the AMS source
   was not fetched). **Do not change `amsclient` on this unverified claim.** To resolve: confirm the real
   endpoint against a live 2-node AMS 3.0.3 cluster (or the `ams-v3.0.3` source). If pagination is required,
   it is a genuine P0 `amsclient` fix + mock-ams update — a scoped code change, not a test.
2. **Webhook mapping (G-22, from the plan's AMS-source review).** The plan reports AMS 3.0.3 emits 14 hook
   actions and Pulse maps 3; error actions (`publishTimeoutError`, `encoderNotOpenedError`, `endpointFailed`,
   `idleTimeIsExpired`) and the periodic `liveStreamStatus` are candidate ingest signals. This is a product
   decision (build event-driven ingest vs keep REST polling) — not a test. Recorded for your call.
3. **Credentials hygiene.** The plan notes an AMS console password and a Pulse admin token passed through a
   chat channel to author it — rotate both after the validation cycle.

---

## 7. Rev 2 — the load lane (`qa/realams/load/`)

**Goal (Ant Media's framing):** prove not just that a real AMS carries the media load, but that **Pulse's
numbers stay correct and its budgets hold while it does** — convergence, oracle parity, API latency, alert
latency, and ghost-free teardown. Every assertion is a **delta on `val-load-` streams we own**; none is an
absolute global count, so none can false-green. The mock tier already proves cheap Pulse-side scale (CI
WO-4: `bulk_publish 500` → `total_publishers ≥ 502`); this lane is the next fidelity tier — real wire, real
media — against a **dedicated instance**.

### 7.1 What shipped

| Artifact | Purpose |
|---|---|
| `qa/realams/harness/load-env.sh.example` | committed template; operator copies to `load-env.sh` (gitignored) |
| `qa/realams/harness/load-gen.sh` | generator abstraction: **native** (default) vs **official** Ant Media Scripts |
| `qa/realams/load/scenarios/TC-S-10..13` | publisher ramp · viewer scale · soak · churn |
| `qa/realams/run-load-suite.sh` | lane runner → `LOAD-REPORT.md` |
| `run-full-e2e.sh` **phase 45** | opt-in; SKIPs 77 when `load-env.sh` absent |
| `docs/compatibility.md` "Capacity and load validation" | where the measured capacity number lands |

### 7.2 Two design decisions that differ from the operator plan

1. **Native generators are the default; the official Ant Media tools are an opt-in.** The plan wrapped
   `ant-media/Scripts` (`rtmp_publisher.sh`, `hls_players.sh`) + the Java WebRTC Load Test Tool as the
   *primary* path. Those tools are real (verified: `rtmp_publisher.sh` takes `file server N` → `server_1..N`;
   the WebRTC tool is `run.sh -m publisher|player -f -s -n -i -u`), but the repo already has
   dependency-free equivalents — `start_bulk_publishers`, `ramp_hls_viewers`, `start_webrtc_viewer` — that
   need no downloads, no `sample.mp4`, and whose HLS sim uses **real-player segment-once** semantics (no
   refetch inflation). So `LOAD_GENERATOR=native` is the default; `=official` wraps the Ant Media tools when
   the marketplace submission should speak their toolkit's vocabulary or when WebRTC viewer scale exceeds the
   native Playwright cap (8 browsers).
2. **The load scenarios live in `load/scenarios/`, NOT `scenarios/` — a safety fix.** The plan put
   `TC-S-1x` in `qa/realams/scenarios/`, where `make validate-all` (`scenarios/TC-*.sh`) **and** phase 41
   (`scenarios/TC-*-1[0-9]-*.sh`) would both sweep them up and fire load generators **at the shared VPS** —
   exactly what the safety rail forbids. Isolating them one directory deeper keeps every shared-VPS path
   blind to them. Belt-and-suspenders: `load-env.sh` sets the harness vars itself and is **never** sourced
   alongside `env.sh` (which is the only file that knows the shared VPS), and it hard-aborts (exit 1) if a
   forbidden host (`beyondkaira.com` / `antmedia.io` / the staging host) is detected. All three guards were
   exercised: placeholder → 77, forbidden host → 1, valid dedicated host → 0.

### 7.3 Corrections vs the operator plan's scripts (same class as §5)

The plan's §9 scripts had the §5 harness-API errors **plus** load-specific ones. Rewritten against reality:

| Plan's version | Real API / fix |
|---|---|
| `${LOAD_PULSE_URL}/api/v1/live/overview` | `${PULSE_URL}/live/overview` (URL already includes `/api/v1`) |
| `.streams[]` in the live list | `(.items // [])[]` |
| alert rule body `{"op":"lt","stream_id":…}` | `AlertRuleWrite` requires `operator` (not `op`), plus `window_s` + `severity`; scoping is `scope.{app,stream_id}` — the plan's body would 400 |
| alert history match on `.rule_name` | history has **`rule_id`** (there is no `rule_name`); match `rule_id == <created id>` |
| load scenarios in `scenarios/` | `load/scenarios/` (see §7.2 #2) |
| download official tools as the only generator | native default; official behind `LOAD_GENERATOR=official` |

### 7.4 Budgets L-1…L-9 (asserted unless noted)

L-1 convergence (`total_publishers` +N ≤120 s) · L-2 oracle parity (AMS `active-live-stream-count` **and**
Pulse `total_publishers` both reach base+N — asserted as independent lower bounds per side, not a strict
AMS-delta == Pulse-delta equality) · L-3 `/live/overview` p95 <300 ms (assert N≤50, record above) · L-4 poll
stability (count == base+N in ≥95% of soak samples) · L-5 alert-under-load (guaranteed-fire rule reaches
`firing` ≤30 s; asserted only when the rule is accepted) · L-6 WebRTC viewer parity ±10% (HLS **recorded**,
not asserted — the ~9× inflation is a known AMS semantic, see TC-V-10) · L-7 pipeline drop counters = 0
(**reserved — NOT collected by TC-S-10..13**; needs a `/metrics` scrape a future scenario adds) · L-8
scratch-Pulse RSS slope (record; flag >10%/h) · L-9 ghost-free teardown (counts back to baseline ≤60 s).

### 7.5 Run it

```bash
cp qa/realams/harness/load-env.sh.example qa/realams/harness/load-env.sh   # then fill in the dedicated hosts
bash qa/realams/run-load-suite.sh                                          # → LOAD-REPORT.md under $EVIDENCE_ROOT
```

Scenarios are `bash -n`-clean; like the rest of the live lane they are **packaged for a dedicated
instance**, not run in this egress-blocked sandbox.

---

## 8. Rev 2 — panel revamp & marketplace readiness

Ant Media (Ankush) gave us confidential read-only access to a **web-panel revamp** and framed the
marketplace listing as: ship a fully-ready, documented plugin, then meet their developer. The full
**business + development assessment** — does the revamp threaten our opportunity, and what it means on the
dev end — is written up for the operator in **`docs/operator-expected.md`** (top banner), with the technical
integration-surface detail pinned in **`docs/compatibility.md` → "Panel-revamp (G-27) compatibility"**.

**One-line verdict (medium confidence):** the revamp is a real but **non-existential** concern — the
endpoints that carry Pulse's core value (`/{app}/rest/v2/broadcasts/list`, `webrtc-client-stats`,
`vods/list`) are in a data-plane namespace a UI overhaul does not touch, and the two console dependencies
most at risk (auth, app discovery) already have deployed bypasses (`PULSE_AMS_AUTH_TOKEN`,
`PULSE_AMS_APPLICATIONS`). The right response is targeted due diligence at the developer meeting + a pinned
G-27 note, **not** an architecture change.

> **Honesty note (do not repeat the plan's overstatements):** an earlier draft of the operator plan called
> the G-21 cluster-pagination issue a "confirmed P0 wire bug", cited 7 "M-1..M-7" marketplace gates, and
> called the compatibility matrix "standalone-only". Verified against the repo: G-21 is **UNVERIFIED** (repo
> says do **not** change `amsclient` until a live 2-node cluster confirms it); the real readiness structure
> is the **17-row** checklist in `docs/assessment/final-assessment.md` §3; the matrix covers four AMS
> versions (3.0.3 live, three mock). The two load-relevant claims that **are** confirmed: ~9× HLS inflation
> (TC-V-06) and the ~5–7 concurrent-RTMP ceiling on the shared VPS (LIM-12).

---

*Implements the operator's "Full End-to-End Validation Run" plan (Rev 2), verified against the code.
Companion: `docs/testing/e2e-ams-test-design.md`. Operator-facing summary: `docs/operator-expected.md`.*
