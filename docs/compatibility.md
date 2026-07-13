# Pulse — AMS Version Compatibility Matrix

**Product:** Pulse: Self-Hosted Analytics, QoE Monitoring and Alerting for Ant Media Server  
**Last updated:** S27 / D-089 (2026-07-13)

---

## Quick reference

| AMS Version | Validation Status | Pulse Support Level | Source |
|-------------|------------------|---------------------|--------|
| 3.0.3 Enterprise (build 20260504\_1443) | **LIVE-VALIDATED** | **Supported — primary target** | 46/50 scenario scripts PASS, qa/realams S17–S18, D-079/D-080 |
| 3.0.2 | Mock-profile only | Mock-compatible | `.github/workflows/ams-version-matrix.yml`; `server/internal/collector/ams_version_matrix_test.go` lines 134–171 |
| 2.14.x | Mock-profile only | Mock-compatible | `ams_version_matrix_test.go` lines 99–133 |
| 2.10.0 | Mock-profile only | Mock-compatible | `ams_version_matrix_test.go` lines 63–97 |

**Recommendation: deploy AMS 3.x.** All versions earlier than 3.0.3 have mock-profile
coverage only. Real Docker images for those older versions are unavailable on Docker Hub
(all tags return 404 as of 2026-07-13; confirmed in the workflow header comment,
`.github/workflows/ams-version-matrix.yml` lines 5–8). The version-matrix CI workflow
runs Go in-process mock profiles — not live containers.

---

## Live-validated version detail

### AMS 3.0.3 Enterprise Edition (build 20260504\_1443)

**Validation program:** Sessions S17–S18, 2026-07-11
(`qa/realams/` harness against `161.97.172.146:5080`)

| Scenario class | Result |
|----------------|--------|
| P0 scripts (26) | **25 PASS / 1 SKIP / 0 FAIL** |
| P1 scripts (24) | **21 PASS / 3 SKIP / 0 FAIL** |
| **Combined (50 scripts)** | **46 PASS / 4 SKIP / 0 FAIL** |

All four SKIPs were premise-limited (no IP-blocked app available, AMS HLS semantics, VPS
RTMP capacity ~5–7 streams) and are not AMS version regressions. See
`docs/assessment/final-assessment.md` §1 for the full SKIP rationale.

**AMS 3.x-specific behaviors known to affect Pulse (all confirmed via live AV triage):**

| Behavior | Impact on Pulse | Source |
|----------|-----------------|--------|
| `currentFPS` absent from REST BroadcastDTO | `fps = 0` for all REST-polled streams; health score redistributes FPS weight | AV-04; `server/pkg/amsclient/client.go:97` comment; DG-03 |
| CPU / mem / disk absent from `GET /rest/v2/system-status` for standalone AMS | Fleet resource gauges empty; available only via Kafka (`ams-instance-stats`) or cluster mode | AV-06; DG-05 |
| AMS 3.0.3 cannot HMAC-sign lifecycle webhooks | Webhook listener returns 401 for all unsigned deliveries; REST polling covers stream lifecycle within ≤10 s budget | AV-08; decisions.md O3; DG-04 |
| `GET /rest/v2/applications/info` returns HTTP 405 | Not used by Pulse; VoD ground truth uses per-app `GET /{app}/rest/v2/vods/list` | S17 corrections; `docs/assessment/scenario-matrix.md` |
| Per-app RTMP broadcast deleted on stop (implicit broadcasts return 404, not `finished`) | Pulse treats 404-after-broadcasting as stream end (correct); operators polling for `finished` via AMS directly will miss the event | DG-11; TC-F-02 |
| `hlsViewerCount` is a sliding segment-request window, not a session count | Observed ~9× inflation (5 real viewers → AMS count 45); count persists >90 s after viewers stop | DG-01; TC-V-06 (S18) |
| `webRTCViewerCount` present and non-zero for WebRTC viewers | Pulse inline viewer count includes WebRTC viewers correctly | AV-02; TC-V-03 |
| Per-app `remoteAllowedCIDR` gates REST access | Apps with `remoteAllowedCIDR=127.0.0.1` return HTTP 403 from the Pulse container; no streams polled for those apps | DG-08; TC-APP-02 SKIP |

---

## Mock-profile-only versions

The following AMS versions are covered by mock profiles in
`server/internal/collector/ams_version_matrix_test.go`.
**No live containers were available for validation**; the mock profiles embody
design intent and in-code documentation only. Wire-format facts below are derived
from the Go test file header comments and CI mock fixtures (lines 41–160), not
from real AMS REST responses.

> **Honesty notice:** the `.github/workflows/ams-version-matrix.yml` workflow
> previously pulled `antmedia/ant-media-server-community:<ver>` images and stood
> up ClickHouse, but "those public image tags no longer exist (Docker Hub returns
> 404 for every tag AND the repo), so every leg silently fell back to mock-ams,
> making the 'version matrix' fictional" (workflow header comment, lines 5–8).
> The current workflow runs in-process mock tests only. If a pullable real-AMS
> image for these versions becomes available, add a container-backed job per the
> comment at `ams-version-matrix.yml` line 16.

### AMS 2.10.0

Mock profile source: `ams_version_matrix_test.go` lines 63–97.

| Field | Mock-profile behavior | Notes |
|-------|-----------------------|-------|
| `speed` | Primary bitrate field (~ratio, ~1.0 = real-time) | `ams_version_matrix_test.go:78`; labeled MISLEADING — stores AMS real-time ratio, not a kbps value |
| `bitrate` | Present in mock (set to 0); may be absent in real v2.10.x | `ams_version_matrix_test.go:79` comment: "CI-ONLY: real v2.10 may use 'speed' only" |
| `webRTCViewerCount` | Present in mock (50) | `ams_version_matrix_test.go:75` |
| `currentFPS` | Present in mock (30) | Mock-only; behavior on real v2.10.x unverified |

**Wire format note:** Pulse's normalizer reads **only** `BitRate` (from `bitrate`):
`bitrateKbps := b.BitRate / 1000.0`. A historical fallback to `Speed` when
`BitRate == 0` was deliberately **removed** — AMS `speed` is a real-time RATIO
(≈1.0), not a bitrate, and the fallback emitted ~1 "kbps" of garbage (see the
comment at `server/internal/collector/normalize.go:73-79`). Consequence for real
v2.10.x deployments whose DTOs carry only `speed`: Pulse reports bitrate 0
(honest absence) rather than a fabricated value.

### AMS 2.14.x

Mock profile source: `ams_version_matrix_test.go` lines 99–133.

| Field | Mock-profile behavior | Notes |
|-------|-----------------------|-------|
| `bitrate` | Primary bitrate field (set to 2500 bits/s in mock) | `ams_version_matrix_test.go:114` |
| `webRTCViewerCount` | Present and distinct from `hlsViewerCount` | `ams_version_matrix_test.go:111`; v2.14 comment: "adds webRTCViewerCount distinct from hlsViewerCount" |
| `currentFPS` | Present in mock (30) | Mock-only |
| `speed` | Absent from v2.14 mock | v2.14 switched primary bitrate to `bitrate` field |

**AMS analytics log added in v2.10:** the JSON analytics log
(`ant-media-server-analytics.log`) was introduced in AMS v2.10.0 per PRD §7.3
(`docs/prd-report.md` line 43). The Kafka producer was also introduced as an
optional feature in this era.

### AMS 3.0.2

Mock profile source: `ams_version_matrix_test.go` lines 134–171.

| Field | Mock-profile behavior | AMS 3.0.3 live reality |
|-------|----------------------|------------------------|
| `bitrate` | Primary bitrate field (3000 bits/s mock) | Confirmed live AV-04; normalize.go:79 |
| `speed` | May be absent (3.0.x switched to `bitrate`) | Live 3.0.3: `speed` ~1.0 still present in some captures |
| `currentFPS` | 60 in mock | **ABSENT in real AMS 3.0.3** REST BroadcastDTO (AV-04 CONFIRMED; `client.go:97` comment) |
| `webRTCViewerCount` | 80 in mock | Confirmed live (TC-V-03) |

The gap between mock (`currentFPS=60`) and live reality (`currentFPS` absent) is the
most important divergence in the version matrix. Operators on AMS 3.x should expect
`fps = 0` in Pulse until AMS restores the field or Pulse adds a Kafka FPS path (Q4 in
`docs/assessment/final-assessment.md` §6).

---

## Per-version feature support summary

| Capability | AMS 2.10.0 | AMS 2.14.x | AMS 3.0.2 | AMS 3.0.3 |
|-----------|-----------|-----------|-----------|-----------|
| Stream lifecycle (REST) | Mock-compatible | Mock-compatible | Mock-compatible | **LIVE-VALIDATED** |
| Viewer counts (inline BroadcastDTO) | Mock-compatible | Mock-compatible | Mock-compatible | **LIVE-VALIDATED** |
| Bitrate (kbps) | Via `speed` field | Via `bitrate` field | Via `bitrate` field | **LIVE-VALIDATED** |
| FPS | Via `currentFPS` (mock) | Via `currentFPS` (mock) | Via `currentFPS` (mock) | **0 (field absent in REST)** |
| WebRTC viewer count | Via `webRTCViewerCount` (mock) | Via `webRTCViewerCount` (mock) | Via `webRTCViewerCount` (mock) | **LIVE-VALIDATED** |
| Fleet resource metrics (CPU/mem) | Unknown | Unknown | Unknown | **Via Kafka only** (REST absent for standalone) |
| Webhook signing | Unknown | Unknown | Unknown | **UNSIGNED — webhook path disabled** |
| VoD recording (REST poll) | Unknown | Unknown | Unknown | **LIVE-VALIDATED** (BUG-002 FIXED S23/D-085) |
| Anomaly detection | Mock-compatible | Mock-compatible | Mock-compatible | **LIVE-VALIDATED** (0.259 false alarms/node-week) |

---

## Testing this matrix

Mock-profile tests run as part of the standard CI suite:

```sh
cd server
CGO_ENABLED=0 go test \
  -run 'TestAMSVersionMatrix|TestCollector' \
  ./internal/collector/... ./pkg/amsclient/... \
  -v -timeout 120s
```

The nightly workflow also runs a REST v2 contract smoke against the mock-ams binary
(`.github/workflows/ams-version-matrix.yml`).

To add a real-AMS container leg (when images become available), see the comment at
`.github/workflows/ams-version-matrix.yml` line 16.

---

*Produced at S27/D-089. Evidence sources: `docs/assessment/final-assessment.md`,
`docs/assessment/capability-map.md`, `server/internal/collector/ams_version_matrix_test.go`,
`.github/workflows/ams-version-matrix.yml`.*
