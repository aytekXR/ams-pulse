# Pulse — End-to-End & Ant Media Integration Test Design

> **Scope:** the full test surface of Pulse (self-hosted analytics / QoE / alerting for Ant Media Server) —
> from the AMS wire protocols through the collector, stores, API, and web UI. Written to be **extended**: every
> file path, endpoint, command, and fixture below is real and runnable.
> **Authored:** 2026-07-17 · **Prod baseline:** `v0.4.0-98-g641b4e2` · **AMS under test:** 3.0.3 Enterprise Edition.
> Companion docs: `docs/ARCHITECTURE.md` (layering rules), `docs/AMS-INTEGRATION.md` (operator wiring),
> `docs/assessment/prd-validation-matrix.md` (F1–F10 verdicts), and the older `agents/handoffs/E2E-TEST-PLAN.md`
> (2026-07-07, partially superseded by this document).

---

## 1. Purpose and scope

This document is the single map of **how Pulse is tested end-to-end and how its Ant Media Server integration is
verified**, and the reference for adding new tests. It covers six test layers (Go unit, Go integration/conformance,
web unit, web e2e, the CI e2e stack, and the real-AMS harness) and the AMS surface each one exercises.

**The golden rule for every test you add here: it must be impossible to false-green.** A test may pass **only** if
the real path under test actually works. In practice that means:

- Drive behavior through the **real seam**, not a re-implementation of it. Assert an observable *output* of the full
  chain (an API response body, a stored row, a fired alert), not an intermediate you also computed in the test.
- Choose thresholds a broken implementation **cannot** accidentally satisfy (e.g. the CI alert rule uses
  `threshold=99999` against a fixed `2000` kbps publish — the only way it fires is the full
  poller → snapshot → evaluator → meta-store → API path working).
- When you cannot observe the real output (no ClickHouse fixture, no multi-node cluster), **say so** — mark the param
  `exempt` with a reason (see §6.1) or mark the scenario `SKIP` (exit 77), never a silent pass.

Use §5 (scenario catalog) to find the journey you want to cover, §4 for the AMS-specific mechanics, and §6 for the
copy-paste recipes to add each kind of test.

---

## 2. System under test

Pulse is a **single Go binary** whose five layers are wired together at startup in `server/cmd/pulse/serve.go`.
Data flows one way — AMS/players in, dashboards/alerts out:

```
                 ┌──────────────────────── INGEST (collector.Source) ────────────────────────┐
  AMS REST v2 ──▶│ restpoller      webhook (/webhook/ams)   beacon (/ingest/beacon)   kafka   │
  players    ──▶ │  (5s poll)       HMAC, fail-closed         player QoE SDK          (opt)   │
                 └───────────────┬───────────────────────────────────────────────────────────┘
                                 │ normalize → domain.ServerEvent / domain.BeaconEvent
                                 ▼
                          Fanout (domain.EventSink) ──── synchronous delivery to all Consumers
                                 │
        ┌────────────────────────┼───────────────────────── PROCESSING ─────────────────────┐
        ▼                        ▼                           ▼                                │
   Aggregator              Session Stitcher            Ingest Health          + tick loops:   │
   (O(1) LiveSnapshot)     (join/hb/leave → session)   (ComputeHealthScore)   Alert Evaluator │
        │                        │                           │                (5s), Anomaly    │
        │                        │                           │                Detector, Prober │
        ▼                        ▼                           ▼                Runner, Cluster   │
        └───────────────┬────────┴───────────────────────────┴──── EventNodeStats ◀── Discovery┘
                        │
      ┌─────────────────┴──────── STORAGE (never crossed — ARCHITECTURE §3) ────────────────┐
      ▼                                                        ▼                             │
  ClickHouse (high-volume append + rollup MVs):          Meta store (SQLite / Postgres):     │
  server_events, beacon_events, viewer_sessions,         alert rules, probe configs, anomaly │
  probe_results, rollup_qoe_1h / usage_1d / concurrency  baselines, tokens, users, tenants   │
      └─────────────────┬───────────────────────────────────────┬─────────────────────────┘
                        ▼                                        ▼
                   query.Service (analytics/QoE/probes)     in-memory snapshot
                        └───────────────┬───────────────────────┘
                                        ▼
                          chi REST API (server/internal/api) + /live/ws  ──▶  React SPA (web/)
```

| Layer | Where | Role |
|---|---|---|
| **Ingest** | `server/internal/collector/{restpoller,webhook,beacon,kafka}`, `server/pkg/amsclient` | Four `collector.Source`s normalize AMS wire data / player beacons into `domain.ServerEvent` / `domain.BeaconEvent` and push to the Fanout. |
| **Processing** | `collector/aggregator`, `collector/sessions`, `collector/ingest`, `internal/alert`, `internal/anomaly`, `internal/prober`, `internal/cluster` | Consumers act on every event (live snapshot, session stitching, health score); tick loops evaluate alerts and anomalies; the prober pool and cluster discovery run independently. |
| **Storage** | `server/internal/store/clickhouse`, `server/internal/store/meta` | **Strict split, never crossed:** metrics/time-series in ClickHouse; config/low-cardinality relational state in the meta store. |
| **Serving** | `server/internal/api` | chi REST API reads in-memory (live) or via `query.Service` (analytics/QoE/probes); `/live/ws` pushes `LiveOverview` snapshot+delta envelopes. |
| **Web UI** | `web/src` | React SPA, WS-first / REST-fallback, TanStack-virtualized stream tables, all API types generated from the OpenAPI schema. |

---

## 3. Test layers

Pick the **highest-fidelity layer that can give real confidence** for what you are testing. Prefer a Go unit/integration
test for backend logic, a mock-AMS e2e assertion for cross-cutting behavior, and the real-AMS harness for AMS-wire
fidelity.

| Layer | Proves | Where | Run |
|---|---|---|---|
| **Go unit** (race) | Handler logic, normalization, evaluators, probes — pure + `httptest`. Coverage floor **70.2%**. | `server/**/*_test.go` (171 files, 25 pkgs) | `cd server && CGO_ENABLED=0 go test ./... -race` |
| **Go integration + contract** | Real ClickHouse v26.6.1 + Postgres 16 queries; OpenAPI response/param conformance. | `-tags integration`; `openapi_conformance_test.go`, `param_conformance_test.go`, `conformance_s3_test.go` | `cd server && CGO_ENABLED=0 go test -tags integration ./...` |
| **Web unit** (Vitest) | Component/store logic. Gate: lines ≥59%, branches ≥54%, functions ≥45% (`coverage-gate.test.ts`). | `web/src/**/*.test.ts` (43 files) | `cd web && npm test` |
| **Web e2e** (Playwright) | User flows in a real browser. 16 of 17 specs are route-mocked via `stubApp()`; 1 needs the compose stack. | `web/e2e/*.spec.ts` (17 specs) | `cd web && npx playwright test` |
| **CI e2e stack** (mock-AMS) | The full binary against a deterministic AMS mock + real ClickHouse; 13 named assertions. | `.github/workflows/e2e.yml`, `deploy/docker-compose.ci.yml`, `qa/mock-ams` | `gh workflow run e2e.yml` (or local compose, §7.1) |
| **Real-AMS harness** | AMS-wire fidelity against live AMS 3.0.3 EE; 50 scenarios (46 PASS / 4 SKIP). | `qa/realams/scenarios/TC-*.sh` | `make -C qa/realams validate-all` |

The Docker-based Go run used in this repo (host has no Go/CH on PATH):

```bash
docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
  -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build \
  -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...
```

---

## 4. Ant Media Server integration testing

This is the core of the suite: everything that touches the AMS wire. Pulse consumes AMS through **four** paths — REST
polling (the supported real-time path), the webhook listener (ready but AMS 3.0.3 cannot sign), synthetic probes, and
(optionally) a Kafka stats topic. Each is exercised by unit fixtures, a deterministic **mock-AMS** in CI, and the
**real-AMS harness** for wire fidelity.

### 4.1 AMS REST v2 surface

All REST calls live in `server/pkg/amsclient/client.go`; the polling loop that drives them is
`server/internal/collector/restpoller/restpoller.go` (5 s default interval). Auth is **cookie-session first** for
AMS 3.x (`POST /rest/v2/users/authenticate` → `JSESSIONID`; JWT is disabled by default), falling back to a Bearer token
for older builds. A custom `simpleCookieJar` is used because Go's `net/http/cookiejar` rejects bare-IP hosts. The eight
consumed endpoints and their traps:

| Endpoint | Method | Purpose | Notes / quirks |
|---|---|---|---|
| `/rest/v2/applications` | GET | Discover app names | Response envelope is **polymorphic**: AMS 3.x returns `{"applications":["live",...]}` (string array); older AMS returns `[{"name":"live"},...]` (object array). `Client.ListApplications` handles both forms. |
| `/{app}/rest/v2/broadcasts/list/{offset}/{size}` | GET | Broadcast list, paginated, 200/page | `bitrate` is **bits/sec** (÷1000 = kbps). `speed` is a dimensionless realtime ratio, **not** a bitrate. `currentFPS` is **absent** from AMS 3.0.3 REST BroadcastDTO (decodes to 0). SRT ingest is misreported as `publishType=RTMP`. `ListBroadcastsPaged` paginates until a short page. |
| `/{app}/rest/v2/broadcasts/{streamId}/webrtc-client-stats/0/100` | GET | Per-peer WebRTC QoE | `streamId` **must be `url.PathEscape`d** — publisher-chosen IDs can contain `#`, `?`, `/`. Missing escape caused a silent stream miss (fix at `client.go:479-489`). |
| `/rest/v2/cluster/nodes` | GET | Cluster node list (`[]ClusterNodeDTO`) | Returns HTTP **404 on standalone AMS**, mapped to `(nil, nil)` — no error raised. |
| `/rest/v2/cluster/nodes/{nodeId}` | GET | Single-node detail | Also 404-tolerant. |
| `/rest/v2/system-status` | GET | OS / JVM info | On AMS 3.x standalone: returns `osName`, `osArch`, `javaVersion`, `processorCount` **only** — no CPU/mem/disk. Those require the Kafka `ams-instance-stats` topic. |
| `/rest/v2/version` | GET | AMS version | `versionType` is the two-word string `"Enterprise Edition"`, **not** `"Enterprise"`. Returns 404/405 on older AMS — client maps to `(nil, nil)`. |
| `/{app}/rest/v2/vods/list/{offset}/200` | GET | VoD list for recording billing | `duration` field is **milliseconds**, not seconds. `creationDate` is Unix epoch **milliseconds**. Polled every 12 broadcast ticks (60 s at 5 s default). Deduped via `vod_poll_state` meta table. |

**AMS version matrix** (mock-profile-only in CI; real Docker images not available on Docker Hub):

| AMS mock profile | Key delta vs current | Test file |
|---|---|---|
| v2.10 | `speed`-only DTO — no `bitrate` field | `server/internal/collector/ams_version_matrix_test.go` |
| v2.14 | `bitrate` field present; `currentFPS` absent | `server/internal/collector/ams_version_matrix_test.go` |
| v3.0.2 | `currentFPS=0` always; full cluster nodes endpoint; polymorphic applications envelope | `server/internal/collector/ams_version_matrix_test.go` |

Test fixtures live in `server/pkg/amsclient/testdata/`: `applications.json`, `broadcasts_*.json`, `cluster_nodes.json`, `system_status.json`, `version.json`, `webrtc_client_stats.json`, `vods_list.json`.

```bash
# AMS client unit tests
cd server && CGO_ENABLED=0 go test ./pkg/amsclient/... -v -timeout 60s

# AMS version matrix
cd server && CGO_ENABLED=0 go test -run TestAMSVersionMatrix ./internal/collector/... -v -timeout 60s

# Normalize field-mapping tests
cd server && CGO_ENABLED=0 go test -run TestNormalize ./internal/collector/... -v -timeout 60s
```

### 4.2 Webhook path

Implementation: `server/internal/collector/webhook/webhook.go`

**Routes:**
- `POST /webhook/ams` — legacy; validates against global `SharedSecret` only.
- `POST /webhook/ams/{sourceName}` — per-source; uses `SourceSecrets[name]` with `SharedSecret` fallback for unknown source names. Both absent → 401 (no 404, to avoid leaking source names).

**HMAC validation:**
- Header: `X-Ams-Signature: sha256=<hex(HMAC-SHA256(payload, secret))>`
- **Fail-closed**: `validateHMAC` returns `false` when `secret == ""` — a misconfigured instance cannot accept unsigned requests.
- Empty `SharedSecret` at startup logs an error-level message and rejects all requests.

**Replay protection** (`PULSE_WEBHOOK_REQUIRE_TIMESTAMP=true`, default `false`):
- Requires `X-Ams-Timestamp` (Unix seconds) within ±`TimestampSkew` (default 5 min) of host clock.
- HMAC is computed over `"<decimal-unix-seconds>.<raw-body>"` — the timestamp is bound into the signature so captured requests cannot be replayed.
- The flag is global across both routes simultaneously.
- Clock is injectable via `now func()` for test determinism.

**Parsed action strings:** `liveStreamStarted`, `startBroadcast`, `publish_started`, `liveStreamEnded`, `stopBroadcast`, `publish_ended`, `vodReady`, `recording_ready`. Both JSON-object and JSON-array payload forms are handled.

**Production gap:** AMS 3.0.3 Management Console does **not** send `X-Ams-Signature`. Pointing AMS directly at Pulse yields 401s on every delivery. REST polling (5 s default) is the supported real-time path. Webhook is ready for a signing proxy or a future AMS version that adds HMAC signing.

**Existing test files:**
- `server/internal/collector/webhook/webhook_test.go` — HMAC validation, 200/401 paths.
- `server/internal/collector/webhook/webhook_more_test.go` — `parseWebhook`, `translateWebhook`.
- `server/internal/collector/webhook/webhook_persource_test.go` — cross-source secret isolation.

**Uncovered:** `handleWebhook` and `handleWebhookWithSecret` HTTP handler layer — no test POSTs a signed request to the live `httptest.Server`. See gap G-06 in [Section 8](#8-coverage-gaps-and-priorities).

### 4.3 Synthetic probe protocols (F10)

All probes live in `server/internal/prober/`. All HTTP/WS/TCP probes are routed through `ssrfguard.DialControl` (link-local and private ranges blocked; `transport.Proxy=nil` to prevent proxy bypass). Pro+ entitlement gate checked before every execution. Runner holds a 4-worker goroutine pool; per-probe context timeout defaults to 10 s (`ProbeConfig.TimeoutS`). Probe config is refreshed every 60 s; unchanged configs are not cancelled and re-spawned (BUG-003 fix — prevents duplicate result rows in ClickHouse).

#### HLS probe

File: `server/internal/prober/prober.go` (function `probeHLS`, private).

1. GET manifest → parse `#EXTM3U`. If master playlist: follow one level to a variant playlist.
2. Fetch first segment; compute `bitrate_kbps = bytes×8 / segDuration_s / 1000`.
3. Measures `TTFBMs` (manifest) and `SegmentTTFBMs` (segment, stored separately as `segment_ttfb_ms`).
4. Relative URIs resolved via `net/url.ResolveReference` (RFC 3986) — D-131 fix for the string-truncation bug.
5. Segment body cap: 32 MiB; over-cap → `error_code="segment_too_large"`, `Success=true` (bonus-measurement rule).

Key test assertions from `prober_test.go`: `success=true`, `ttfb_ms>0`, `bitrate_kbps>0`, `segment_ttfb_ms>0`.

#### WebRTC probe

File: `server/internal/prober/probe_webrtc_ice.go`.

URL convention: `ws(s)://host/{app}/websocket?streamId=<id>`.

Steps:
1. WS dial via `nhooyr.io/websocket` (SSRF-safe guarded client).
2. Send `{command:"play",streamId:...}`.
3. **Skip notification messages** (`subtrackAdded`, etc.) — real AMS 3.0.3 sends these **before** the offer.
4. Wait for `{command:"takeConfiguration",type:"offer"}` → record `ConnectTimeMs` (dial→offer).
5. pion ICE: `SetRemoteDescription` → `CreateAnswer` → `SetLocalDescription` → send answer → trickle candidates → wait for ICE connected / failed / timeout.
6. 2 s RTP stats hold for `jitter_ms`, `rtt_ms`, `loss_pct` via `pc.GetStats()`.

ICE outcome **never flips `Success`** (bonus-measurement rule). `error_codes`: `ws_refused`, `ws_timeout`, `ws_error`, `ice_failed`, `ice_timeout`.

#### RTMP probe

File: `server/internal/prober/probe_rtmp.go`.

URL: `rtmp://host[:port][/app]`.

1. TCP dial via `ssrfguard.DialControl`.
2. Send `C0(0x03) + C1(1536 B)` → receive `S0+S1+S2` → strict S2 echo check → send `C2` best-effort.
3. No app segment in URL → `success=true + handshake_complete`.
4. With app → send AMF0 `connect` chunk (fmt=0, csid=3, payload split at 128-byte RTMP boundaries) → read response via a minimal chunk demuxer (handles `SetChunkSize` 0x01, extended timestamps, reassembles fragmented messages across fmt=0/1/2/3).
5. `_result` → `signaling_state=app_accepted`; `_error` → `signaling_state=app_rejected`.
6. Defenses: CSID state cap (`maxCSIDStates=256`), message size cap (`rtmpMaxMsgSize=64 KB`).

Fixture binary: `server/internal/prober/testdata/ams-connect-response.bin`.

#### DASH probe

File: `server/internal/prober/probe_dash.go`.

- GET MPD (`encoding/xml`; body cap 16 MiB).
- Parse `SegmentTemplate` or `SegmentList`; compute timescale-adjusted `bitrate_kbps`.
- `$Number$` template expansion with safe printf-width guard (`reSafeNumberSpec`, bound to ≤999 width digits). A hostile manifest using `%1000d` or wider is a parse failure.
- Relative URIs resolved via `net/url.ResolveReference`.
- DASH muxing is **disabled** on live AMS 3.0.3 (returns 404); fixture-based tests only.

#### Unknown protocol (`probeReachability`)

Any `Protocol` value not in {`hls`, `webrtc`, `rtmp`, `dash`}: HTTP GET for reachability only. Always returns `error_code="not_probed"`, `Success=false`. No faked success.

### 4.4 Mock-AMS control API

The mock-AMS binary at `qa/mock-ams/mock-ams` (Go, separate `go.mod`) is the **primary deterministic lever** for CI and local e2e tests. It emulates the AMS REST v2 surface exactly as consumed by `server/pkg/amsclient/client.go`.

**Startup flags:**

```
-addr         HTTP listen address (default :9090)
-rtmp-addr    TCP RTMP listen address (disabled by default)
-webrtc-ice   Enable pion ICE phase-2a (default false = static offer + close)
-log-dir      Emit JSON events to ant-media-server-analytics.log in this directory
-scenario N   Auto-run a predefined scenario (0 = manual control)
-app          App name (default "live")
```

**Control endpoints (test levers):**

| Method + Path | Body (JSON) | Effect |
|---|---|---|
| `POST /control/publish` | `{"stream_id":"x","viewers":N[,"bitrate":N_bits_per_sec]}` | Publishes or updates a stream; viewers split WebRTC:HLS at N:N/3; default `BitRate`=2000 bits/s |
| `POST /control/unpublish` | `{"stream_id":"x"}` | Sets `status=finished`, sets `EndTime` |
| `POST /control/set_viewers` | `{"stream_id":"x","viewers":N}` | Updates WebRTC+HLS viewer counts in-place |
| `POST /control/set_bitrate` | `{"stream_id":"x","bitrate":N}` | Updates `BitRate` (bits/sec); 400 if missing `stream_id`; 404 if stream not found |
| `POST /control/bulk_publish` | `{"count":N,"prefix":"str-","viewers_each":0}` | Publishes N streams with IDs `<prefix>0001`…`<prefix>000N`; returns `{"status":"ok","count":N}` |
| `GET /truth/viewers/{id}` | — | Ground-truth oracle: returns `{"stream_id":"x","viewers":total}` |
| `GET /healthz` | — | Returns `{"status":"ok"}` |

**AMS REST v2 routes served by mock-ams** (matching `amsclient` paths exactly):

- `GET /rest/v2/applications`
- `GET /{app}/rest/v2/broadcasts/list/{offset}/{size}` (paginated, sorted by StreamID for determinism)
- `GET /{app}/rest/v2/broadcasts/{streamId}/webrtc-client-stats/0/100`
- `GET /{app}/rest/v2/broadcasts/{streamId}/statistics`
- `GET /rest/v2/cluster/nodes`

**Media / protocol routes:**
- `WS /{app}/websocket` — WebRTC signaling (static offer + close by default; full pion ICE with `-webrtc-ice=true`)
- `GET /{app}/streams/{streamId}.mpd` — static DASH MPD (timescale=90000, 2 s segments)
- `GET /{app}/streams/{streamId}-seg-N.m4s` — 50000-byte static payload (`byte[i]=i%256`, yields 200 kbps at 2 s segments)
- TCP RTMP listener on `-rtmp-addr`; `app="rejected"` → `_error` response (deterministic rejection hook)

**Recommended deterministic e2e pattern:**

```bash
MOCK="http://localhost:9090"
PULSE="http://localhost:8090"
TOKEN="$PULSE_ADMIN_TOKEN"

# 1. Publish a stream with a known bitrate (2 Mbps in bits/sec)
curl -s -X POST "$MOCK/control/publish" \
  -H "Content-Type: application/json" \
  -d '{"stream_id":"e2e-test-1","viewers":10,"bitrate":2000000}'

# 2. Poll Pulse until stream appears (budget 15 s)
DEADLINE=$((SECONDS + 15))
while [ "$SECONDS" -lt "$DEADLINE" ]; do
  PUB=$(curl -s -H "Authorization: Bearer $TOKEN" \
    "$PULSE/api/v1/live/overview" | jq '.total_publishers')
  [ "$PUB" -ge 1 ] && break
  sleep 1
done
[ "$PUB" -ge 1 ] || { echo "FAIL: stream not visible in 15 s"; exit 1; }

# 3. Verify bitrate conversion: AMS wire 2000000 bits/s → Pulse 2000 kbps
KBPS=$(curl -s -H "Authorization: Bearer $TOKEN" \
  "$PULSE/api/v1/live/streams" | \
  jq '.streams[] | select(.stream_id=="e2e-test-1") | .bitrate_kbps')
[ "$KBPS" -eq 2000 ] || { echo "FAIL: bitrate_kbps=$KBPS want 2000"; exit 1; }

# 4. Clean up
curl -s -X POST "$MOCK/control/unpublish" \
  -d '{"stream_id":"e2e-test-1"}'
```

When asserting total publisher counts after `bulk_publish`, always assert `total_publishers >= expected_floor` — never an exact global count, because other streams may already exist in the mock.

```bash
cd qa/mock-ams && go test -race -count=1 -timeout 300s ./...
```

### 4.5 Real-AMS harness (`qa/realams/`)

The real-AMS harness validates behavior against AMS 3.0.3 Enterprise Edition.

**Harness utilities (`qa/realams/harness/`):**

| File | Purpose |
|---|---|
| `env.sh` | Exports `PULSE_URL`, `AMS_URL`, `PULSE_TOKEN`, `AMS_COOKIE_FILE`, `EVIDENCE_ROOT` |
| `auth.sh` | **One attempt only** per invocation; reuses cookie if valid; **never loops** (AMS lockout: 2 bad logins = 5-min account lock; `admin@` is shared with the prod poller) |
| `assert.sh` | `assert_eq`, `assert_approx`, `assert_gte`, `assert_lte`, `assert_within`, `scenario_verdict` |
| `capture.sh` | `capture_ams`, `capture_pulse`, `compare_viewer_count` |
| `publisher.sh` | `ffmpeg` RTMP publishers; unique `val-` ID prefix |
| `viewer-sim.sh` | HLS and WebRTC viewer simulation |

**Scenario naming convention:** `TC-{category}-{NN}-{slug}.sh`

Categories: `H` (health), `FL` (fleet), `APP`, `WH` (webhook), `P` (probe), `L` (lifecycle), `F` (failure), `V` (viewer), `I` (ingest), `AN` (anomaly), `A` (analytics), `S` (stress), `REC` (recording).

**Make targets:**

```bash
make -C qa/realams auth                      # Authenticate once (idempotent)
make -C qa/realams validate-p0               # P0: 26 scenarios
make -C qa/realams validate-p1               # P1: 24 scenarios
make -C qa/realams validate-all              # All TC-*.sh sorted; SKIP(77) OK; FAIL→exit 1
make -C qa/realams validate-TC-L-01          # Single scenario by TC-ID
```

**Evidence package** written by every scenario to `${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)/`:

| File | Contents |
|---|---|
| `ams-pre-baseline-*.json` | AMS REST state before publisher started |
| `ams-broadcasting-*.json` | AMS REST state during broadcast |
| `pulse-pre-baseline-*.json` | Pulse API state before publisher started |
| `pulse-broadcasting-*.json` | Pulse API state during broadcast |
| `compare-vc-*.json` | Viewer count parity between AMS and Pulse |
| `checks.txt` | Per-assertion PASS/FAIL log |
| `timeline.txt` | Timestamped event log |
| `verdict.txt` | `PASS` or `FAIL` with list of failed checks |

`evidence/` is gitignored. Small reference fixtures go to `agents/handoffs/real-ams-captures/`.

**Current status** (`docs/compatibility.md`): 46/50 scripts PASS (25P/1S P0; 21P/3S P1). The 4 SKIPs are premise-limited: IP-blocked app unavailable, HLS viewer-count semantics differ from test assumptions (known 9× inflation window), VPS RTMP capacity ~5–7 concurrent streams.

**`hlsViewerCount` inflation warning:** AMS `hlsViewerCount` is a sliding segment-request window that inflates approximately 9× vs real viewer count and persists >90 s after viewers stop. TC-V-06 documents this as a known limitation. Do **not** write assertions that compare `hlsViewerCount` directly to `webRTCViewerCount` or to Pulse-reported total viewer count.

### 4.6 What cannot be validated without a real multi-node AMS cluster

The following require at minimum two AMS nodes (one origin, one edge). No such environment exists in CI or the `qa/realams` VPS.

**Edge-origin viewer dedup** (`server/internal/cluster/discovery.go:271-284`, `IsEdgeStream`): When an edge node serves a stream, the origin's `viewer_count` is suppressed in `TotalViewers` to avoid double-counting. The `Status!="down"` guard is critical — without it, a crashed edge permanently suppresses the origin's viewer count. This guard is unit-tested in `server/internal/cluster/discovery_edge_status_test.go` with `d.nodes` seeded directly (5 table cases), but no live HTTP-level multi-node evidence exists.

**Aggregator wiring in cluster mode** (`aggregator.go:344`): `edgeChecker.IsEdgeStream` is called per event; `edgeChecker` is `nil` in standalone deployments. The nil path is exercised in all CI and real-AMS tests. The non-nil cluster path is exercised only via `mockEdgeChecker` in `aggregator_test.go`.

**N3 PRD requirement** (viewer count accuracy in cluster mode): Currently marked **PARTIALLY** — standalone confirmed, cluster ENV-LIMIT.

Any new test claiming to cover edge-origin dedup **must** seed `Discovery`'s `d.nodes` map directly and **must not** claim live cluster validation.

### 4.7 AMS integration test matrix

| Area | Test file(s) | Covered | Gap |
|---|---|---|---|
| REST client: all 8 endpoints | `server/pkg/amsclient/client_test.go`, `server/pkg/amsclient/vods_test.go` | All endpoints via httptest fixtures | Applications string-array vs object-array polymorphism; empty `streamId` guard; `url.PathEscape` on `#`/`?`; VodDTO `duration`-in-ms trap; `ListApplications` HTTP 401 → re-login |
| Normalize BroadcastDTO → domain | `server/internal/collector/normalize_test.go`, `normalize_realcapture_test.go` | Field mapping, unit conversions | `normalizePublishType` at line 291 has no covering tests; SRT→RTMP mismatch path untested |
| AMS version matrix | `server/internal/collector/ams_version_matrix_test.go` | v2.10 / v2.14 / v3.0.2 mock profiles | All mock-only; v2.10 speed-only DTO not live-validated |
| Poll latency | `server/internal/collector/restpoller/latency_test.go` | `TestLatency_StreamVisibleWithin10s` with mock AMS | `poll()` and `pollApp()` have no direct unit tests |
| VoD cadence | `server/internal/collector/restpoller/restpoller_vods_test.go` | 12-tick cadence; at-most-once `MarkVodSeen` | `MarkVodSeen` success + full channel scenario (blocked emit) not covered |
| API failure streak | `server/internal/collector/restpoller/api_streak_test.go` | Consecutive errors emit FAILURE-STREAK | — |
| Webhook HMAC | `webhook_test.go`, `webhook_more_test.go`, `webhook_persource_test.go` | HMAC validation; per-source routing; cross-source isolation | HTTP handler layer (`handleWebhook`, `handleWebhookWithSecret`) entirely untested; replay protection stale-timestamp path not tested |
| HLS probe | `server/internal/prober/prober_test.go` | Manifest+segment, master→variant, 404, timeout, body cap | `probeHLS` private function has no direct unit test; RFC 3986 absolute-path URI resolution regression test absent |
| RTMP probe | `server/internal/prober/probe_rtmp_test.go`, `probe_rtmp_s66_test.go` | C0/C1/S0/S1/S2/C2 handshake; AMF0 connect; mock binary fixture | `SetChunkSize` (0x01 type) sent before AMF0 `_result` not covered in a dedicated test |
| DASH probe | `server/internal/prober/probe_dash_test.go` | MPD parsing with DASH-IF spec fixtures | No live AMS DASH (returns 404); hostile `$Number%1000d$` template width not verified |
| WebRTC ICE probe | `server/internal/prober/probe_webrtc_ice_test.go` | ICE phases; in-process pion loopback | RTP stats hold context-expiry-before-stats path; notification-skip loop with in-process server |
| Cluster edge dedup | `server/internal/cluster/discovery_edge_status_test.go`, `discovery_test.go` | `Status!="down"` guard; 5 table cases | No live multi-node coverage; ok→down→ok state transition across polls |
| Cross-app dedup | `server/internal/collector/dedup_test.go` | Same streamId, different App → not a duplicate | Aggregator-level end-to-end cross-app collision not tested |

---

## 5. End-to-End Scenario Catalog

The table maps PRD features F1–F10 and cross-cutting concerns to testable journeys. **Target layer** is the highest-fidelity tier that provides real confidence. **Current coverage** lists what exists today; **Gap** is what is missing.

| ID | Journey (PRD feature) | Entry action | Determinism mechanism | Observable exit / assertion | Target layer | Current coverage | Gap |
|---|---|---|---|---|---|---|---|
| J-01 | **F1** — Publish stream → live dashboard ≤10 s | AMS publisher starts stream | REST poller tick injectable; mock-AMS `/control/publish` | `GET /api/v1/live/overview` returns stream; `total_publishers` updated within budget | CI e2e + unit | `TestLatency_StreamVisibleWithin10s`; TC-WH-02 (4 s confirmed on real AMS) | WS delta message sequence not asserted end-to-end; `?tenant` filter silently dropped (BUG-009) |
| J-02 | **F1** — WebSocket snapshot → delta message flow | Aggregator push after stream state change | Mock aggregator in `httptest`; leading-edge 1 s rate limiter | WS client receives `{type:"snapshot"}` then `{type:"delta"}` with updated `total_publishers` | Go unit | `s46_live_ws_auth_test.go` (auth only) | Delta message sequence with real aggregator push not tested; 30 s heartbeat not tested |
| J-03 | **F3** — Beacon POST → `qoe/summary` reflects startup p50 | Player SDK POSTs batch with `startup_time_ms` | Deterministic `session_id`; `TestQuery_QoeSummary_RealStartupP50` | `GET /api/v1/qoe/summary` returns `startup_p50_ms > 0`; CI assertion A2 | Go integration + CI e2e | `vd10_beacon_test.go`; `vd24_ingest_seeded_test.go`; CI A2 | 413 body oversize rejection (64 KB); partial-accept 202 with schema failures; 429 rate-limit response |
| J-04 | **F4** — Ingest degradation → health_score drop ≤15 s | Publisher drops bitrate or increases packet loss | `ComputeHealthScore()` pure function; REST poll ≤5 s; 250 µs in-process | `GET /api/v1/qoe/ingest` returns `health_score < 80` within 15 s | Go unit + real AMS | `TestVD24_IngestQoE_TimeseriesNonEmpty`; TC-I-02, TC-I-06 | WebRTC `packetLostRatio` non-zero (requires remote peer); fps=-1 sentinel weight redistribution |
| J-05 | **F5** — Alert rule fires → history entry + channel delivery ≤30 s | POST alert rule; ingest condition breached | FakeClock in evaluator; mock channel adapter | `GET /api/v1/alerts/history` returns `state=firing`; 201 ms CI wall-clock | Go unit + CI e2e | `evaluator_test.go`; `TestEvaluator_DetectAndNotify_WallClockBudget`; CI A1, A3 | `rebuffer_ratio`/`error_rate` rules need seeded CH rollup data; `license_expiry` rule untested |
| J-06 | **F5** — Webhook delivery failure recorded | POST alert channel with dead URL; alert fires | Mock webhook URL returns 500 | Alert history entry with `delivery_failure` state | CI e2e | CI assertion A4 | Success delivery to a real receiver has no e2e guard |
| J-07 | **F6** — Report export → CSV download (Business+ gate) | `GET /api/v1/reports/export?format=csv` with Business+ token | `qa/licensegen -tier business`; `?token=` query-param auth | HTTP 200 + `text/csv` + correct `Content-Disposition`; 403 for Pro/Free; 501 for `format=pdf` | Go unit | **Well covered:** `export_test.go` (6 tests — CSV 200, Free-tier 403, `?token=` 200, missing-token 401, pdf→501, default-format CSV); `?format` is a param-conformance probe (D-147); documented in OpenAPI (D-147) | CSV *content* (column set / row values) is not schema-validated (CSV is not JSON) — a golden-file assertion would close it; no browser-download e2e |
| J-08 | **F7** — Fleet nodes visible with live stats | `GET /api/v1/fleet/nodes` | `FleetNodes()` merges `clusterDiscovery.Snapshot()` with live node stats | Node cards show `cpuUsage`, `memoryUsage`, `activeStreamCount`; stale nodes handled | Go unit + Playwright e2e | `query_pure_test.go` (mock provider); `web/e2e/fleet.spec.ts` | Merge when some nodes are absent from live snapshot not tested |
| J-09 | **F9** — Anomaly flag visible after σ deviation (Enterprise gate) | Metric deviates > `min_sigma` from Welford baseline | 10,000 synthetic Gaussian samples in `TestAnomaly_FalseAlarmRate_ModeledTarget` | `GET /api/v1/anomalies?min_sigma=2.0` returns flag with `metric`, `sigma`, `ts`; 403 for non-Enterprise | Go unit + CI e2e | `TestAnomaly_FalseAlarmRate_ModeledTarget`; CI A5, A5b | `WarmHysteresis()` startup warmup not isolated; `error_rate`/`rebuffer_ratio` signals confirmed MISSING (TC-AN-05) |
| J-10 | **F10** — Synthetic probe result stored and readable | `POST /api/v1/probes`; runner executes probe | Mock HTTP / WS / TCP servers in probe test files; BUG-003 fix prevents duplicates | `GET /api/v1/probes/{id}/results` returns entry with `ttfb_ms > 0`, `success=true` | Go unit + CI e2e | `TestHLSProbe_Success`; `TestHLSProbe_MasterFollowsVariant`; CI WebRTC/RTMP/DASH assertions | `probeHLS` private function has no direct unit test; RTMP `SetChunkSize`-before-AMF0 path; DASH hostile template |
| J-11 | **F2** — Analytics audience/geo/device breakdown (Pro+ gate) | `GET /api/v1/analytics/audience` with seeded `viewer_sessions` | `query_integration_test.go` with real ClickHouse | Bucketed rows with `viewers`, `watch_time_s`; 403 for Free tier | Go integration | `query_integration_test.go` (geo/device VD-06) | `geo_country` always empty unless `PULSE_GEO_MMDB_PATH` is operator-mounted; no CI test exercises a non-empty geo response |
| J-12 | **Token kind isolation** | Bearer request to `/api/v1/live/overview` with a kind=`ingest` token | Meta store `kind` field; `authz_test.go` | 403 `WRONG_TOKEN_KIND` on `/api/v1/*` with ingest token; 403 on `/ingest/beacon` with api token | Go unit | `authz_test.go` (generic) | No test verifies the specific kind=`ingest` → 403 on bearer routes and vice versa |
| J-13 | **OIDC SSO** — Login → session cookie → `/auth/me` (Enterprise gate) | Browser navigates to `GET /auth/oidc/login` with Enterprise license | Mock JWKS server; injectable nonce; HMAC-signed state | `pulse_session` cookie set; `GET /auth/me` returns `auth_method=cookie`; bad state → 400; unconfigured → 501 | Go unit | `oidc_test.go` (callback); `s46_live_ws_auth_test.go` (cookie→WS) | PKCE verifier not actually validated by mock; mismatched state → 400 not tested; `pulse_session` on `/reports/export` not tested |
| J-14 | **Admin audit log** — mutations captured and readable | `POST /admin/tokens`, `DELETE /admin/users/{id}`, etc. | `audit_s40_test.go` in-process; SQLite append-only | `GET /admin/audit-log` returns entry for every mutating call; DELETE on already-deleted resource → 204 | Go unit | `audit_s40_test.go`; `s47_password_internal_test.go` | Audit log persistence across server restarts not tested; TOCTOU node-limit race at `POST /admin/sources` |

---

## 6. How to Extend the Tests

### 6.1 Add a Go unit test

Place the test in the same package as the code being tested, in a `_test.go` file with no build tag.

**Pattern for a new handler test using the shared `httptest` server:**

```go
// server/internal/api/my_feature_test.go
package api

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestMyFeature_HappyPath(t *testing.T) {
    s := newTestServer(t) // shared helper in api_test.go
    req := httptest.NewRequest(http.MethodGet, "/api/v1/my/endpoint", nil)
    req.Header.Set("Authorization", "Bearer "+s.apiToken)
    w := httptest.NewRecorder()
    s.router.ServeHTTP(w, req)
    if w.Code != http.StatusOK {
        t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
    }
    // Validate response body against OpenAPI schema using the
    // kin-openapi pattern from openapi_conformance_test.go
}
```

**Adding a query param to the param-conformance audit** (`server/internal/api/param_conformance_test.go`) — REQUIRED whenever you add a query parameter to `contracts/openapi/pulse-api.yaml`, or the gate fails:

1. Open `param_conformance_test.go`. The registry is keyed `"METHOD /openapi-path ?paramName"` (no `/api/v1` prefix — spec paths omit it).
2. Add one registry entry **per new query param**, dispositioned as `paramProbe` (a real differential probe fn), `paramExempt` (with an `exemptReason` — e.g. no ClickHouse fixture), or `paramKnownViolation` (with a `bugRef`). See the `GET /reports/export` block (added in D-147) for the exact pattern: `from/to/app/stream/tenant` are `exempt` (shared nil-CH `ComputeUsage` backing), `format` is a real `csv→200 / pdf→501` probe.
3. Bump the two non-vacuity floors to match the new totals: `minSpecParams` (currently **94** — every declared query param must be enumerated) and `minProbes` (currently **38**). Never *lower* either floor; raise them by exactly the number you add.

**Run before opening a PR:**

```bash
cd server && CGO_ENABLED=0 go test ./... -race \
  -coverprofile=/tmp/cover.out -covermode=atomic
go tool cover -func=/tmp/cover.out | awk '/^total:/'
```

Ensure the total stays at or above 70.2%.

### 6.2 Add a Go integration test

Integration tests require a real ClickHouse v26.6.1 binary at `/tmp/clickhouse` and optionally a Postgres DSN. See [Section 7.4](#74-clickhouse-binary-for-integration-tests) for the binary download command.

**Required build constraint:**

```go
//go:build integration

package clickhouse_test
```

**Pattern using the existing helper:**

```go
func TestMyIntegration(t *testing.T) {
    store := testutil.RequireClickHouseBin(t) // t.Fatal if binary absent
    ctx := context.Background()

    // Insert test data via the store's public methods
    err := store.WriteServerEvent(ctx, myEvent)
    if err != nil {
        t.Fatal(err)
    }

    // Query and assert
    rows, err := store.QueryMyMetric(ctx, params)
    if err != nil {
        t.Fatal(err)
    }
    if len(rows) == 0 {
        t.Fatal("expected rows, got none")
    }
}
```

**ClickHouse rollup latency gotcha:** Materialized views (`rollup_qoe_1h`, `rollup_usage_1d`, `rollup_concurrency_1d`) populate asynchronously. A test that inserts into `beacon_events` and immediately queries `rollup_qoe_1h` will see an empty result. Use a polling loop:

```go
deadline := time.Now().Add(15 * time.Second)
for time.Now().Before(deadline) {
    rows, err := store.QueryQoeSummary(ctx, params)
    if err == nil && len(rows) > 0 && rows[0].StartupP50Ms > 0 {
        break
    }
    time.Sleep(500 * time.Millisecond)
}
if len(rows) == 0 || rows[0].StartupP50Ms == 0 {
    t.Fatal("rollup did not populate within 15 s")
}
```

**Run locally:**

```bash
cd server && CGO_ENABLED=0 go test -tags integration \
  ./internal/store/clickhouse/... -v -timeout 120s
```

### 6.3 Add a Playwright e2e spec (route-mocked)

All 16 route-mocked specs share the `stubApp` helper. Do not access a real backend from a route-mocked spec.

```typescript
// web/e2e/my-feature.spec.ts
import { test, expect } from '@playwright/test';
import { stubApp } from './support/stubs';

test.describe('MyFeature', () => {
  test.beforeEach(async ({ page }) => {
    // stubApp installs 4 boot-time mocks (license, auth/me, oidc/status, healthz)
    // and seeds localStorage['pulse_token']
    await stubApp(page, { tier: 'pro' });

    // Add feature-specific route mocks
    await page.route('**/api/v1/my/endpoint', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ my_field: 'value' }),
      });
    });

    await page.goto('/');
  });

  test('renders heading', async ({ page }) => {
    await page.getByRole('link', { name: 'My Feature' }).click();
    await expect(
      page.getByRole('heading', { name: 'My Feature' })
    ).toBeVisible();
  });

  test('enterprise gate blocks non-enterprise tier', async ({ page }) => {
    await stubApp(page, { tier: 'pro' });
    await page.goto('/anomalies');
    await expect(page.getByText(/upgrade/i)).toBeVisible();
  });
});
```

**Run:**

```bash
cd web && npx playwright test e2e/my-feature.spec.ts --headed
```

### 6.4 Add a mock-AMS control + deterministic e2e assertion

This is the preferred approach for CI-gated behavioral assertions that need a controllable AMS state.

```bash
#!/usr/bin/env bash
# Template for a mock-ams-backed e2e assertion

MOCK="http://localhost:9090"
PULSE="http://localhost:8090"
TOKEN="$PULSE_ADMIN_TOKEN"

# 1. Publish a stream with known bitrate (bits/sec on the wire)
curl -sf -X POST "$MOCK/control/publish" \
  -H "Content-Type: application/json" \
  -d '{"stream_id":"ci-stream-1","viewers":10,"bitrate":2000000}'

# 2. Poll Pulse until stream appears (budget 15 s)
DEADLINE=$((SECONDS + 15))
while [ "$SECONDS" -lt "$DEADLINE" ]; do
  PUB=$(curl -sf -H "Authorization: Bearer $TOKEN" \
    "$PULSE/api/v1/live/overview" | jq '.total_publishers // 0')
  [ "$PUB" -ge 1 ] && break
  sleep 1
done
[ "$PUB" -ge 1 ] || { echo "FAIL: stream not visible after 15 s"; exit 1; }

# 3. Assert bitrate unit conversion: 2000000 bits/s → 2000 kbps
KBPS=$(curl -sf -H "Authorization: Bearer $TOKEN" \
  "$PULSE/api/v1/live/streams" | \
  jq '.streams[] | select(.stream_id=="ci-stream-1") | .bitrate_kbps')
[ "$KBPS" -eq 2000 ] || { echo "FAIL: bitrate_kbps=$KBPS want 2000"; exit 1; }

# 4. Bump viewer count and verify via ground-truth oracle
curl -sf -X POST "$MOCK/control/set_viewers" \
  -d '{"stream_id":"ci-stream-1","viewers":50}'
TRUTH=$(curl -sf "$MOCK/truth/viewers/ci-stream-1" | jq '.viewers')
[ "$TRUTH" -eq 50 ] || { echo "FAIL: truth viewers=$TRUTH want 50"; exit 1; }

# 5. Unpublish and clean up
curl -sf -X POST "$MOCK/control/unpublish" \
  -d '{"stream_id":"ci-stream-1"}'
```

**When using `bulk_publish` for large-stream-count tests:** assert `total_publishers >= expected_floor`, never an exact global count.

### 6.5 Add a real-AMS TC-*.sh scenario

Follow this template exactly. Exit-code discipline (`0` / `1` / `77`) is required for `make validate-p0/p1/all` to process results correctly.

```bash
#!/usr/bin/env bash
# TC-{CATEGORY}-{NN}-{slug}.sh — one-line description
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../harness/env.sh"
source "$SCRIPT_DIR/../harness/assert.sh"
source "$SCRIPT_DIR/../harness/capture.sh"
source "$SCRIPT_DIR/../harness/publisher.sh"

SCENARIO="TC-{CATEGORY}-{NN}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$EVIDENCE_DIR"

# Use val-/pulse-pub-val- prefix to avoid collisions with other scenarios
STREAM_ID="val-$(openssl rand -hex 4)"

# Always set the EXIT trap before starting a publisher
trap 'stop_publisher "$STREAM_ID" 2>/dev/null || true' EXIT

# Check preconditions — exit 77 if premise cannot be met
AMS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  "$AMS_URL/rest/v2/version" 2>/dev/null || echo 000)
[ "$AMS_STATUS" = "200" ] || { echo "AMS unreachable, skipping"; exit 77; }

# Capture pre-baseline before starting the publisher
capture_pulse "$EVIDENCE_DIR/pulse-pre.json" /api/v1/live/overview

# Start publisher and wait for at least one poll cycle
start_publisher "$STREAM_ID"
sleep 6

# Capture during broadcast
capture_ams  "$EVIDENCE_DIR/ams-live.json"   "/live/rest/v2/broadcasts/list/0/50"
capture_pulse "$EVIDENCE_DIR/pulse-live.json" /api/v1/live/overview

# Assert deltas, never absolute global counts
PRE=$(jq '.total_publishers' "$EVIDENCE_DIR/pulse-pre.json")
LIVE=$(jq '.total_publishers' "$EVIDENCE_DIR/pulse-live.json")
assert_gte "$LIVE" "$((PRE + 1))" \
  "total_publishers increased by ≥1" "$EVIDENCE_DIR/checks.txt"

scenario_verdict "$SCENARIO" \
  "$EVIDENCE_DIR/checks.txt" "$EVIDENCE_DIR/verdict.txt"
```

**Rules:**
- Always use `val-` or `pulse-pub-val-` prefix for stream IDs.
- Always capture the pre-baseline **before** starting the publisher.
- Assert per-stream values or before/after deltas — never global absolute counts.
- Set the `EXIT` trap before starting any publisher so cleanup runs even on assertion failure.
- Call `scenario_verdict` at the end; it writes `verdict.txt` and sets the exit code.

### 6.6 Shared setup: license generation and token minting

**Ephemeral Business license** (webhook channels require Business+; used in CI e2e):

```bash
# Outputs two lines to stdout: PULSE_LICENSE_KEY=... and PULSE_LICENSE_PUBKEY=...
eval $(cd qa/licensegen && go run . -tier business)

# Other tiers: free | pro | business | enterprise
# Time-limited license for expiry tests (2-minute window):
eval $(cd qa/licensegen && go run . -tier pro -expires-minutes 2)
```

**Bootstrap admin token** (extracted from pulse container logs):

```bash
PULSE_ADMIN_TOKEN=$(docker logs pulse 2>&1 | \
  grep -oP 'plt_[A-Za-z0-9_-]*' | head -1)
```

**Ingest token creation** (required for beacon tests):

```bash
INGEST_TOKEN=$(curl -s -X POST \
  -H "Authorization: Bearer $PULSE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:8090/api/v1/admin/tokens \
  -d '{"kind":"ingest","label":"test-ingest"}' | jq -r '.raw_token')
```

The raw token value is returned **once only** in the 201 response. Subsequent `GET /admin/tokens` responses show only `id`, `kind`, and `created_at`.

---

## 7. Environments and Fixtures

### 7.1 CI e2e stack (`e2e.yml`)

The `e2e.yml` pipeline brings up a full Docker Compose stack and asserts 13 named scenarios.

**Services started** via `deploy/docker-compose.yml` + `deploy/docker-compose.ci.yml`:

| Service | Image / source |
|---|---|
| `clickhouse` | ClickHouse 24.8 (pinned in CI overlay) |
| `mock-ams` | Compiled from `qa/mock-ams/` in the CI image |
| `pulse-migrate` | Runs DB migration; must exit 0 before pulse starts |
| `pulse` | Main binary built from `server/` |

**License minting in CI:**

```bash
cd qa/licensegen && go run . -tier business >> $GITHUB_ENV
# Sets PULSE_LICENSE_KEY and PULSE_LICENSE_PUBKEY in the job environment
```

**The 13 CI e2e assertion groups (in execution order):**

| Label | Assertion |
|---|---|
| Infra | `/healthz` returns HTTP 200 |
| Infra | `pulse-migrate` exited 0 |
| Infra | ClickHouse schema tables exist |
| Infra | `mock-ams /healthz` returns HTTP 200 |
| A-05 | WebRTC probe: `connect_time_ms>0`, `signaling_state=offer_received`, `ice_state=connected` |
| A-06 | RTMP probe: `signaling_state=app_accepted` |
| A-07 | DASH probe: `ttfb_ms>0`, `bitrate_kbps>0` |
| A-08 | Live overview: `total_publishers>0`, `total_viewers>0` (after `bulk_publish`) |
| A1 | Alert `ingest_bitrate_floor` rule fires; entry appears in alert history |
| A3 | Alert: `health_score` 100→50 transition on a dedicated stream |
| A2 | Beacon→rollup→QoE: `startup_p50_ms>0`; rebuffer ratio alert fires |
| A4 | Webhook `delivery_failure` state for a dead webhook URL |
| A5/A5b | Anomaly: `viewer_count` and `ingest_bitrate_kbps` anomaly flags fire |
| WO-4 | 500-stream: `bulk_publish {"count":500}` → Pulse reports `total_publishers ≥ 502` |

**Local reproduction of the e2e stack:**

```bash
PULSE_SECRET_KEY=$(openssl rand -hex 32) \
  docker compose -p pulse-e2e \
    -f deploy/docker-compose.yml \
    -f deploy/docker-compose.ci.yml \
  up -d --build
```

**Trigger CI jobs manually:**

```bash
gh workflow run e2e.yml
gh workflow run ci.yml
```

### 7.2 Local development environment

```bash
# Server unit tests
cd server && CGO_ENABLED=0 go test ./... -race -timeout 300s

# Server integration tests (requires /tmp/clickhouse)
cd server && CGO_ENABLED=0 go test -tags integration ./... -timeout 300s

# Web unit tests (coverage always collected)
cd web && npm test

# Web e2e — route-mocked (16 specs, no backend)
cd web && npx playwright test

# SDK tests + size gate
cd sdk/beacon-js && npm test && npm run size

# Mock-AMS unit tests
cd qa/mock-ams && go test -race -count=1 -timeout 300s ./...

# Budget regressions (not CI-gated; run manually before releasing)
bash qa/budgets/run-budget-tests.sh
```

### 7.3 Key environment variables

| Variable | Used for | How to obtain |
|---|---|---|
| `PULSE_LICENSE_KEY` | License claims + ed25519 signature | `qa/licensegen` stdout |
| `PULSE_LICENSE_PUBKEY` | ed25519 public key for license verification | `qa/licensegen` stdout |
| `PULSE_CLICKHOUSE_DSN` | ClickHouse DSN for integration tests | `clickhouse://localhost:9000/pulse` |
| `PULSE_META_TEST_PG_DSN` | Postgres DSN for meta store parity tests | `postgres://pulse:pulse@localhost:5432/pulse_meta_test?sslmode=disable` |
| `PULSE_ADMIN_TOKEN` | Bootstrap admin bearer token | `docker logs pulse` grep `plt_` |
| `PULSE_WEBHOOK_SECRET` | HMAC secret for webhook tests | Any non-empty string |
| `PULSE_WEBHOOK_REQUIRE_TIMESTAMP` | Enable replay protection | `true` / `false` (default `false`) |
| `PULSE_GEO_MMDB_PATH` | MaxMind GeoLite2-City.mmdb path | **Operator-mounted; not bundled** (license/distribution constraint) |
| `AMS_COOKIE_FILE` | AMS session cookie file | Written by `qa/realams/harness/auth.sh` |
| `EVIDENCE_ROOT` | Root dir for real-AMS evidence packages | Set in `qa/realams/harness/env.sh` |

### 7.4 ClickHouse binary for integration tests

`testutil.RequireClickHouseBin` expects ClickHouse v26.6.1-stable amd64 at `/tmp/clickhouse`. In CI, this is downloaded by the `server` job in `ci.yml`. When `CI=true` and the binary is absent, the helper calls `t.Fatal` — not `t.Skip`.

```bash
curl -L -o /tmp/clickhouse \
  "https://github.com/ClickHouse/ClickHouse/releases/download/v26.6.1-stable/clickhouse-linux-amd64"
chmod +x /tmp/clickhouse
```

### 7.5 Rollup latency in integration and e2e tests

ClickHouse materialized views (`rollup_qoe_1h`, `rollup_usage_1d`, `rollup_concurrency_1d`) populate asynchronously. Inserting into `beacon_events` or `server_events` and immediately querying a rollup MV will return zero rows.

- In Go integration tests: poll with a 15 s deadline (see [Section 6.2](#62-add-a-go-integration-test)).
- In `e2e.yml`: the CI stack uses a flush trigger so rollups populate within the assertion window; the assertion for A2 waits up to 30 s.
- `TestQuery_QoeSummary_RealStartupP50` confirms the rollup query is real, not stubbed — use it as a reference for the polling pattern.

---

## 8. Coverage Gaps and Priorities

Gaps are ordered by risk: data-correctness bugs (silent wrong results) first, then unguarded failure modes, then missing scenario coverage.

### P0 — Data-correctness risk

| Gap ID | Description | File | Recommended test approach |
|---|---|---|---|
| G-01 | `normalizePublishType` at line 291 of `normalize.go` has no covering tests. SRT streams silently appear as RTMP in the protocol breakdown. | `server/internal/collector/normalize.go:291` | Unit test: call `NormalizeBroadcast` with `publishType="SRT"`; assert the resulting `PublishType` in the domain event. |
| G-02 | Kafka `dashViewerCount` sum (FIX 4 comment) has no separate assertion — could silently regress to excluding it. | `server/internal/collector/kafka/kafka.go` | Unit test in `kafka_test.go`: pass a message with all four viewer fields set; assert sum = `hlsViewerCount + webRTCViewerCount + rtmpViewerCount + dashViewerCount`. |
| G-03 | WebRTC `packetLostRatio` unit conversion (×1000 at `normalize.go:185`) is never exercised at a non-zero value in CI — all loopback test peers give 0 loss. | `server/internal/collector/normalize.go:185` | Add a `normalize_test.go` case with `packetLostRatio=0.015`; assert `PacketLostRatio=15` in the output domain event. |
| G-04 | AMS `applications` envelope polymorphism: only one form is confirmed tested. A future AMS version returning the object-array form would silently break app discovery. | `server/pkg/amsclient/client.go` | Add `testdata/applications_object_form.json` fixture; assert `ListApplications` returns the same app names for both the string-array and object-array envelope forms. |
| G-05 | VoD `duration` field is milliseconds. No test guards against a refactor that reinterprets it as seconds, which would cause billing errors. | `server/pkg/amsclient/vods_test.go` | Add a fixture-based test that asserts a `VodDTO` with `duration=60000` decodes as 60000 ms (not 60000 s). |

### P1 — Unguarded failure modes

| Gap ID | Description | File | Recommended test approach |
|---|---|---|---|
| G-06 | `handleWebhook` and `handleWebhookWithSecret` HTTP handler layer is completely untested. HMAC rejection on bad signatures, correct event type emitted, and per-source secret lookup are all unguarded at the HTTP level. | `server/internal/collector/webhook/webhook.go` | HTTP handler test: start an `httptest.Server` with the handler; POST a signed payload; assert 200 on valid sig and correct event type emitted to sink; POST with bad sig; assert 401. |
| G-07 | Beacon handler `MaxBytesError`-vs-generic-read-error distinction in the 64 KB body cap is untested. A request at 65537 bytes must return 413; a reader returning `io.ErrUnexpectedEOF` must return a different error code. | `server/internal/collector/beacon/beacon.go` | Unit test: POST 65537-byte body; assert 413. POST where reader returns `io.ErrUnexpectedEOF`; assert response code is not 413. |
| G-08 | `collector.Collector` supervisor exponential backoff (100 ms → 60 s cap; clean-exit reset) has no unit test. A bug in the cap or reset logic is silent. | `server/internal/collector/collector.go` | Test: create a `fakeSource` that returns errors N times then exits cleanly; run `supervise()`; assert each restart delay matches the expected backoff sequence and the next-clean-exit resets to the base delay. |
| G-09 | Session stitcher: beacon heartbeat creating a new session when no prior join was received; leave-without-join early return; `SweepStale()` idle timeout eviction. None of these three branches is tested. | `server/internal/collector/sessions/stitcher.go` | Three separate unit tests, one per branch. |
| G-10 | ClickHouse `beacon_events` batch atomicity (D-118 fix): single `PrepareBatch`+`Send` per flush. A mid-batch `Send` failure must not partial-commit. | `server/internal/store/clickhouse/clickhouse.go` | Integration test: inject a `Send` error at batch boundary; assert zero rows committed in ClickHouse and the drop counter is incremented. |
| G-11 | Ingest HealthTracker: fps=-1 sentinel weight redistribution (AMS 3.x omits fps); `ev.TS<=0` timestamp fallback guard (D-029v). Neither path is explicitly tested. | `server/internal/collector/ingest/health.go` | Unit tests: call `ComputeHealthScore` with `fps=-1`; assert weights still sum to 1.0. Call `OnServerEvent` with `ev.TS=0`; assert no panic or wrong state. |
| G-12 | `probeReachability` always returns `error_code="not_probed"`, `Success=false` for unknown protocols (e.g., `"srt"`). No unit test pins this behavior. | `server/internal/prober/prober.go` | Unit test: create a probe with `Protocol="srt"`; call `executeProbe`; assert `Success=false` and `ErrorCode="not_probed"`. |

### P2 — Missing scenario coverage

| Gap ID | Description | File | Recommended test approach |
|---|---|---|---|
| G-13 | `GET /reports/export` handler paths are **already well covered** by `export_test.go` (200 CSV, 403 Pro/Free, 401 missing-token, 501 pdf, `?token=`, default-format) and the `?format` param-conformance probe + OpenAPI doc (all D-147). The only residual: the **CSV body content** (column set + row values) is not asserted. | `server/internal/api/export.go`, `server/internal/reports/` | Golden-file test: seed a known `UsageReport`, call the handler, byte-compare the emitted CSV (header + rows) against a committed fixture. Low priority. |
| G-14 | Anomaly `WarmHysteresis()` startup warmup from `RecentFlagKeys` is not directly tested. Pre-existing flag keys could be lost silently across restarts. | `server/internal/anomaly/anomaly.go` | Unit test: pre-seed `RecentFlagKeys` in the meta store; call `WarmHysteresis()`; assert hysteresis state is pre-populated and the first tick does not re-fire all stale flags as new. |
| G-15 | `useLiveDashboard` hook (web) has no tests. WS-connected→REST-fallback transition, polling interval setup/teardown, and delta-merge logic are untested. | `web/src/features/live/useLiveDashboard.ts` | Add `useLiveDashboard.test.ts` using `renderHook`; mock `LiveSocket` to simulate a close event; assert REST polling begins after WS disconnects. |
| G-16 | WebSocket `/live/ws` snapshot→delta message sequence not tested end-to-end with a real aggregator push. | `server/internal/api/server.go` | Integration test using an `nhooyr.io/websocket` client: connect, receive initial snapshot, trigger aggregator update, assert delta message received with updated values. |
| G-17 | Meta store `ProbeRow` CRUD (`CreateProbe`, `ListProbes`, `UpdateProbe`, `DeleteProbe`, `RecordResult`) has no covering tests per codegraph. | `server/internal/store/meta/probe.go` | Table-driven unit test covering all five operations including not-found error paths. |
| G-18 | `qa/budgets/run-budget-tests.sh` is not wired into any CI job. B-01 (stream latency ≤10 s) and B-03 (alert latency ≤30 s) can regress silently. | `qa/budgets/run-budget-tests.sh` | Wire as a `budgets` job in `ci.yml` that runs after the `server` job succeeds. |
| G-19 | Token kind isolation: no test verifies a kind=`ingest` token returns 403 on bearer-auth-only `/api/v1/*` routes, and a kind=`api` token returns 403 on `/ingest/beacon`. | `server/internal/api/authz_test.go` | Add two cases to `authz_test.go`: ingest token on `GET /api/v1/live/overview` → 403 `WRONG_TOKEN_KIND`; api token on `POST /ingest/beacon` (with `X-Pulse-Ingest-Token` header) → 403. |
| G-20 | `poll()` and `pollApp()` in `restpoller.go` have no direct unit tests; coverage is only indirect through latency and vod tests. A refactor of the poll logic may not be caught. | `server/internal/collector/restpoller/restpoller.go` | Wire a mock AMS `httptest.Server`; call the poll entry point directly via a package-internal test function or an exported `PollOnce()` method; assert the expected domain events are emitted to the sink. |

---

## 9. Appendix

### 9.1 Endpoint inventory summary

59 endpoints registered in `server/internal/api/server.go` (chi v5). Full specification: `contracts/openapi/pulse-api.yaml`. OpenAPI conformance gate: 51/52 response-body conformant (`GET /live/ws` WebSocket 101 waived).

| Feature area | Count | Auth scheme | License gate |
|---|---|---|---|
| live F1 | 3 | bearerAuth (overview, streams); wsTokenQuery + cookieAuth (ws) | None — all tiers |
| analytics F2 | 3 | bearerAuth | Pro+ (`CheckDataAPI`) |
| qoe F3/F4 | 2 | bearerAuth | Pro+ (`CheckDataAPI`) |
| alerts F5 | 10 | bearerAuth | Channel-type specific (`CheckChannelAllowed`) |
| reports F6 | 6 | bearerAuth; wsTokenQuery + cookieAuth (`/export`) | Business+ (`CheckReports`) |
| fleet F7 | 1 | bearerAuth | None |
| anomalies F9 | 1 | bearerAuth | Enterprise only (`CheckAnomalies`) |
| probes F10 | 5 | bearerAuth | Pro+ (`CheckProbes`) |
| ingest/beacon | 1 | ingestTokenHeader (`X-Pulse-Ingest-Token`) | Pro+ (`CheckBeaconIngest`) |
| operational | 2 | None (`/healthz`); optional scrape token (`/metrics`) | Business+ for `/metrics` (`CheckPrometheus`) |
| auth / OIDC | 5 | None (login, callback, status); bearerAuth + cookieAuth (me, logout) | Enterprise (`CheckSSO`) |
| admin | 20 | bearerAuth | Multi-tenant: Business+ (`CheckMultiTenant`); others: any authed |

**Known violations pinned in conformance registry:**
- BUG-009: `?tenant` filter on `GET /live/overview` and `GET /live/streams` is silently dropped at the query layer (in-memory `LiveSnapshot` has no tenant dimension).
- `GET /live/ws` 101 Switching Protocols is the one waived operation in `openapi_conformance_test.go`.

### 9.2 File map

```
server/
  cmd/pulse/                     Entry point; serve.go wires all layers (42.3% coverage — waived)
  pkg/amsclient/
    client.go                    ALL raw AMS HTTP; custom simpleCookieJar; url.PathEscape for streamId
    testdata/                    JSON fixtures for each of the 8 AMS endpoints
  internal/collector/
    restpoller/                  Poller.Run(); poll(); pollApp(); pollVods()
    webhook/                     HMAC-SHA256; per-source secrets; replay protection; injectable clock
    beacon/                      7-step pipeline; 64 KB cap; MaxBytesError path; geo/UA enrichment
    kafka/                       processMessage(); normalizeKafkaMessage(); Lag/ParseErrors counters
    collector.go                 Supervisor; exponential backoff 100 ms → 60 s cap
    fanout.go                    Synchronous delivery to all Consumers; panic recovery; drop counter
    dedup.go                     Cross-source dedup; dedup key includes App field (D-111)
    normalize.go                 NormalizeBroadcast; NormalizeClusterNode; NormalizeSystemStats
    aggregator/                  O(1) LiveSnapshot; incremental delta helpers; Subscribe()
    sessions/                    Stitcher; ViewerSession; SweepStale()
    ingest/                      ComputeHealthScore; fps=-1 sentinel; SweepStale; Snapshot()
  internal/alert/
    evaluator.go                 5 s tick; 7 eval functions; state machine; FakeClock; channel delivery
    channels/                    email, slack, telegram, pagerduty, webhook channel adapters
  internal/anomaly/
    anomaly.go                   Welford; z-score; effStddev = max(stddev, relEps, absEps); hysteresis;
                                   BaselineSweeper; per-metric presence flags (honest-absent)
  internal/prober/
    prober.go                    Runner; 4-worker pool; executeProbe() dispatch; BUG-003 fix
    probe_rtmp.go                C0/C1/S0/S1/S2/C2; AMF0 connect; minimal chunk demuxer
    probe_dash.go                MPD; SegmentTemplate; reSafeNumberSpec guard
    probe_webrtc_ice.go          WS signaling; notification-skip loop; pion ICE; RTP stats
  internal/store/
    clickhouse/
      clickhouse.go              3 async flush channels; BatchSize=1000; FlushInterval=2 s; drain
    meta/
      meta.go                    SQLite/Postgres; AES-256-GCM; HMAC-SHA256 tokens; alert_history cap
      probe.go                   ProbeRow CRUD (CreateProbe, ListProbes, UpdateProbe, DeleteProbe,
                                   RecordResult) — no covering tests (gap G-17)
  internal/api/
    server.go                    chi router; 59 endpoints; 4 auth schemes; 11 license gate calls
    export.go                    GET /reports/export; CheckReports; CSV / PDF 501 (gap G-13)
    oidc.go                      PKCE S256; HMAC-signed state; id_token validation
    audit.go                     Append-only audit_log writes; audit() helper
  internal/query/
    query.go                     LiveOverview/Streams (in-memory); analytics/QoE/probes (CH);
                                   applyRetention() per tier
  internal/cluster/
    discovery.go                 ClusterNodes poller; IsEdgeStream; Status!="down" guard (line 271-284)
  internal/license/
    license.go                   ed25519 verify; tier entitlements; maybeExpireLocked()

sdk/beacon-js/src/
  index.ts                       Pulse.init(); LiveSession; NoOpSession (sampled-out / bad config)
  transport.ts                   Batching; POST with X-Pulse-Ingest-Token header
  hls.ts                         HlsAdapter; MANIFEST_LOADED; LEVEL_SWITCHED; BUFFER_STALLED_ERROR
  types.ts                       BeaconEventItem; BeaconBatch type definitions

web/src/
  api/client.ts                  LiveSocket class; auto-reconnect; REST fallback trigger
  features/live/
    useLiveDashboard.ts          WS-first / REST-fallback hook (no tests — gap G-15)
    StreamsTable.tsx             TanStack Virtual; density-aware rowHeight (default=40 compact=32 wall=48)
  test/coverage-gate.test.ts    Lines≥59%, branches≥54%, functions≥45%; pinned exclude array

contracts/
  openapi/pulse-api.yaml         59 operations; source of truth for all conformance tests
  events/beacon-event.schema.json  9 event types; server-side validateBeaconBatch uses this

qa/
  mock-ams/
    main.go                      Fake AMS HTTP + TCP; all control endpoints; DASH/RTMP/WebRTC media
    webrtc_ice.go                pion ICE phase-2a offerer (activated with -webrtc-ice=true)
  licensegen/
    main.go                      ed25519 key gen per run; all four tiers; -expires / -expires-minutes
  realams/
    harness/                     env.sh; auth.sh (ONE attempt); assert.sh; capture.sh; publisher.sh
    scenarios/                   50 × TC-*.sh (46 PASS / 4 SKIP vs AMS 3.0.3 EE)
    Makefile                     validate-p0; validate-p1; validate-all; validate-TC-{ID}
  budgets/
    run-budget-tests.sh          8 assertions vs ARCHITECTURE.md §4; NOT wired into CI (gap G-18)

.github/workflows/
  ci.yml                         7 jobs: contracts, server, web, sdk, docker-build, helm, web-e2e
  e2e.yml                        2 jobs: e2e (mock-ams stack + 13 assertions); csp-e2e (Caddy-fronted)
deploy/
  docker-compose.ci.yml          CI overlay: ClickHouse 24.8, mock-ams, pulse-migrate, pulse
```

### 9.3 Commands cheat-sheet

```bash
# ── Go server ─────────────────────────────────────────────────────────────────

# Full unit suite with coverage
cd server && CGO_ENABLED=0 go test ./... -race -timeout 300s \
  -coverprofile=/tmp/cover.out -covermode=atomic
go tool cover -func=/tmp/cover.out | awk '/^total:/'

# Integration suite (requires /tmp/clickhouse v26.6.1)
cd server && CGO_ENABLED=0 go test -tags integration ./... -timeout 300s

# Named tests (examples)
cd server && CGO_ENABLED=0 go test ./internal/collector/restpoller/... \
  -v -run TestLatency_StreamVisibleWithin10s -timeout 30s
cd server && CGO_ENABLED=0 go test -run TestNormalize \
  ./internal/collector/... -v
cd server && CGO_ENABLED=0 go test -run TestAMSVersionMatrix \
  ./internal/collector/... -v
cd server && CGO_ENABLED=0 go test ./internal/api/... \
  -v -run TestOpenAPIConformance
cd server && CGO_ENABLED=0 go test ./internal/alert/... \
  -v -run TestEvaluator_DetectAndNotify_WallClockBudget
cd server && CGO_ENABLED=0 go test ./internal/anomaly/... \
  -v -run TestAnomaly_FalseAlarmRate_ModeledTarget
cd server && CGO_ENABLED=0 go test ./internal/prober/... \
  -v -run 'TestHLSProbe_Success|TestHLSProbe_MasterFollowsVariant'
cd server && CGO_ENABLED=0 go test ./internal/api/... \
  -v -run TestAPI_Healthz_KafkaStats

# Aggregator benchmark (O(1) incremental path)
cd server && CGO_ENABLED=0 go test ./internal/collector/aggregator/... \
  -bench=BenchmarkPollCycle -benchmem -count=3 -benchtime=2s

# Alloc bound assertion (≤1 alloc/event at N=1000)
cd server && CGO_ENABLED=0 go test ./internal/collector/aggregator/... \
  -v -run TestPollCycle_AllocsPerEvent_Bounded

# AMS client
cd server && CGO_ENABLED=0 go test ./pkg/amsclient/... -v -timeout 60s

# ClickHouse integration only
cd server && CGO_ENABLED=0 go test -tags integration \
  ./internal/store/clickhouse/... -v -timeout 120s

# QoE integration (real CH rollup)
cd server && CGO_ENABLED=0 go test -tags integration \
  ./internal/query/... -v -run TestQuery_QoeSummary_RealStartupP50

# ── Web unit (Vitest) ─────────────────────────────────────────────────────────
cd web && npm test
cd web && npx vitest run src/test/coverage-gate.test.ts

# ── Web e2e (Playwright) ──────────────────────────────────────────────────────
cd web && npx playwright test                          # 16 route-mocked specs
cd web && npx playwright test e2e/alerts.spec.ts --headed
cd web && npx playwright test --config playwright.csp.config.ts
cd web && npx playwright test --config playwright.realstack.config.ts \
  e2e/streams-render-500.spec.ts --reporter=list

# ── SDK ───────────────────────────────────────────────────────────────────────
cd sdk/beacon-js && npm test
cd sdk/beacon-js && npm run build && npm run size       # 15 KB gzip gate

# ── QA modules ───────────────────────────────────────────────────────────────
cd qa/mock-ams   && go test -race -count=1 -timeout 300s ./...
cd qa/licensegen && go test -race -count=1 -timeout 300s ./...
cd qa/licensegen && go run . -tier business            # emit PULSE_LICENSE_KEY + PUBKEY

# ── Real-AMS harness ──────────────────────────────────────────────────────────
make -C qa/realams auth
make -C qa/realams validate-p0
make -C qa/realams validate-p1
make -C qa/realams validate-all
make -C qa/realams validate-TC-L-01

# ── Budget regressions ────────────────────────────────────────────────────────
bash qa/budgets/run-budget-tests.sh

# ── ClickHouse binary (local integration) ─────────────────────────────────────
curl -L -o /tmp/clickhouse \
  "https://github.com/ClickHouse/ClickHouse/releases/download/v26.6.1-stable/clickhouse-linux-amd64"
chmod +x /tmp/clickhouse

# ── Local e2e compose stack ───────────────────────────────────────────────────
PULSE_SECRET_KEY=$(openssl rand -hex 32) \
  docker compose -p pulse-e2e \
    -f deploy/docker-compose.yml \
    -f deploy/docker-compose.ci.yml \
  up -d --build

# ── CI triggers ───────────────────────────────────────────────────────────────
gh workflow run ci.yml
gh workflow run e2e.yml
```