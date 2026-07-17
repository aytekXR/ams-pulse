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

*Implements the operator's "Full End-to-End Validation Run" plan, verified against the code. Companion:
`docs/testing/e2e-ams-test-design.md`.*
