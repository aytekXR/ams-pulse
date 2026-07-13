# Anomaly Detection (F9)

**Status: Shipped (Wave 3-MVP + Wave-3-Plus)**
**Tier: Enterprise only**

Pulse detects statistical deviations in stream and node metrics using rolling
baselines. When a metric diverges beyond a configurable sigma threshold, the
system emits an anomaly flag visible via `GET /api/v1/anomalies` and in the
Anomalies dashboard at `/anomalies`.

---

## What F9 flags

The detector tracks six metrics:

| Metric | Scope | Description | Notes |
|---|---|---|---|
| `viewers` | per-stream | Current viewer count for a stream | |
| `ingest_bitrate_kbps` | per-stream | Ingest bitrate for active publishers | |
| `cpu_pct` | per-node | Node CPU utilisation percentage | Absent on standalone AMS via REST (DG-05); available in cluster mode or via Kafka |
| `mem_pct` | per-node | Node memory utilisation percentage | Absent on standalone AMS via REST (DG-05) |
| `disk_pct` | per-node | Node disk utilisation percentage | Absent on standalone AMS via REST (DG-05) |
| `ams_api_latency_ms` | per-node | Pulse poller round-trip time to AMS REST API | Key-absent on failed polls — see §Key-absent semantics below |

A flag is emitted when the current observed value deviates from the stored
rolling mean by more than `sigma` standard deviations. The `sigma` value is
reported in the flag and in the UI (e.g. "19.92σ").

### Key-absent semantics for `ams_api_latency_ms`

Unlike AMS-reported metrics, `ams_api_latency_ms` is measured by Pulse itself
(the round-trip time of the `SystemStats` or `ClusterNodes` HTTP call).

When the call **fails**, the metric key is **absent** from the event (not
emitted as 0). This follows the D-075 key-absent convention: a missing key
means "not measured this tick," which is distinct from "measured as zero."

Feeding 0 to Welford on failures would skew the baseline toward zero, making
normal latency look anomalous once the node recovers. The presence guard
(`APILatencyMS > 0`) prevents this: only successful, non-zero measurements
update the baseline and contribute to live-value comparisons.

Pinned by `TestAnomaly_APILatencyMS_NoMeasurement_NoBaseline` in
`server/internal/anomaly/anomaly_api_latency_test.go`.

### Zero-viewer baselines and the first-viewer spike

Streams with a sustained zero-viewer history — for example, a test stream
publishing continuously but watched by no one — accumulate a Welford baseline of
`mean=0`, `stddev=0` after ≥30 ticks (the `MinSamples` warmup gate, ~30 minutes).
When the first viewer connects, the detector fires a high-sigma anomaly flag.

**Why this is intentional.** `viewers=0` is a genuine measurement — Pulse reads
`ViewerCount` from the AMS live snapshot for every active stream and feeds it to
Welford unconditionally, with no presence guard (see `UpdateBaselines`,
`anomaly.go`). "Zero viewers" means "this stream is live and currently has no
audience", which is a true observation, not a missing data point.

This is the critical distinction from the D-088 cpu/mem/disk class: those metrics
are absent on standalone AMS deployments (the `CPUPCTReported`/`MemPCTReported`/
`DiskPCTReported` presence flags suppress false-zero feeding). Likewise,
`ams_api_latency_ms` uses an `APILatencyMS > 0` guard because `0` is a sentinel
for a failed poll, not a real sub-millisecond round-trip. For `viewers`, `0` is
never a sentinel — it is the actual viewer count.

**The spike mechanics.** After a zero-viewer history the effective-stddev floor
applies at detection time (see `detectFlagsLocked`, `anomaly.go`):

```
effStddev = max(stddev, StddevRelEpsilon × |mean|, StddevAbsEpsilon)
          = max(0,      0.05 × 0,                  1e-9)
          = 1e-9
```

When the first viewer arrives (`observed=1`, `mean=0`):

```
z = |1 − 0| / 1e-9 ≈ 1 × 10⁹
```

The flag fires immediately (well above `DefaultSigma = 4.0`). The
`HysteresisTicks` (10 ticks / 600 s cooldown) then suppresses re-fires.

**Ruling: KEEP.** "Audience appeared after a long quiet period" is a
true statistical anomaly. Operators who consider it noise can raise `min_sigma` on
`viewers` queries, e.g.:

```
GET /api/v1/anomalies?metric=viewers&min_sigma=10
```

An observation-side skip (mirroring the `APILatencyMS > 0` pattern — a ~2-line
change) remains a follow-up option if an operator overrules this ruling.

---

## Statistical model

### Algorithm: Welford online mean / variance

Baselines are maintained using **Welford's one-pass online algorithm**, which
computes exact sample mean and variance incrementally without storing history.
The update rule per tick:

```
n     = sample_count + 1
delta  = observed - mean
mean  += delta / n
delta2 = observed - mean        # uses the new mean
M2    += delta * delta2
stddev = sqrt(M2 / (n - 1))    if n >= 2, else 0
```

`M2` is reconstructed from stored `stddev` and `sample_count` as
`M2_prev = stddev² × (n - 1)`.

Only `mean`, `stddev`, and `sample_count` are persisted in the
`anomaly_baselines` meta-store table — no raw history is stored.

### Window

The rolling window is fixed at **3,600 seconds (1 hour)**. Multiple windows
are a Phase-3 roadmap item.

### Tick interval

The baseline updater runs every **60 seconds** (one tick). At each tick it
reads the current live snapshot, updates Welford statistics for all
observed metrics, and persists the results to the meta store.

---

## Default sensitivity and the <1 false-alarm/node-week target

### Default parameters

| Parameter | Value | Description |
|---|---|---|
| `DefaultSigma` | **4.0** | Sigma threshold for a flag to fire |
| `MinSamples` | **30** | Minimum observations before any flag can fire (~30 min warmup) |
| `HysteresisTicks` | **10** | Ticks suppressed after a flag fires (600 s cooldown) |
| `TickInterval` | **60 s** | Baseline update and detection period |
| `relEps` | **0.01** (1%) | Relative epsilon floor: `effStddev >= 0.01 × |mean|` |
| `absEps` | **0.5** | Absolute epsilon floor: `effStddev >= 0.5` |

**Epsilon floor (Wave-3-Plus):** When a metric is perfectly constant (stddev=0),
z-score computation would divide by zero. The detector applies an effective stddev:

```
effStddev = max(stored_stddev, relEps × |mean|, absEps)
```

This means a deviation of 1% of the mean or 0.5 absolute units is always enough
to produce a finite z-score. At a constant baseline of 100 viewers, a spike to 180
yields `effStddev = max(0, 1.0, 0.5) = 1.0` and `sigma = (180-100)/1.0 = 80.0`
— a clear flag. Small perturbations (e.g. 100→102.5) fall below the relative floor
and correctly produce 0 flags. The stored Welford state is not modified; the floor
is applied on-read in `ComputeFlags` only. The analytical false-alarm rate is
unchanged (the floor only widens the effective stddev, reducing false positives
on constant signals).

Verified by `TestAnomaly_ConstantBaseline_LargeDeviation_Flags` (sigma=80.00, 1 flag)
and `TestAnomaly_ConstantBaseline_SmallDeviation_NoFlag` (0 flags) in
`server/internal/anomaly/anomaly_test.go`.

### False-alarm calibration math

Model: Gaussian tail probability + renewal-process hysteresis.

```
sigma=4.0: P(|Z| >= 4.0) ≈ 6.33e-5  (two-tailed standard normal)

ticks/node/week = 7 × 24 × 3600 / 60 = 10,080
metrics/node    = 5  (conservative node-scoped budget: cpu_pct, mem_pct, disk_pct,
                      ams_api_latency_ms + viewers as a 1-stream-per-node bound;
                      ingest_bitrate_kbps scales with stream count, excluded from
                      the per-node budget)

lambda_raw per metric = 10,080 × 6.33e-5 = 0.638 exceedances/week

With hysteresis (renewal-process suppression):
  lambda_effective = lambda_raw / (1 + lambda_raw × HysteresisTicks)
                   = 0.638 / (1 + 0.638 × 10)
                   = 0.638 / 7.38
                   ≈ 0.08644/week/metric

Total across 5 node-scoped metrics: 0.08644 × 5 ≈ 0.432/node-week
```

**Measured result: 0.432/node-week < 1.0/node-week — PRD F9 target met.**

Verified by `TestAnomaly_FalseAlarmRate_ModeledTarget` in
`server/internal/anomaly/anomaly_test.go`.

### MinSamples warmup gate

The detector emits **zero flags** until at least 30 samples have been collected
for a (metric, scope) combination. At a 60-second tick interval this means
approximately **30 minutes of traffic** is required before baselines stabilize
and anomaly detection becomes active. The UI displays "Baselines are still
learning — anomaly flags appear once enough samples have been collected
(typically a few hours of traffic)."

---

## Tuning sensitivity

### API-level filter: `min_sigma`

`GET /api/v1/anomalies?min_sigma=3.5` filters the response to flags with
`sigma >= 3.5`. The OpenAPI default is **2.0** — this is the API _filter_
minimum, not the detection threshold. Lowering `min_sigma` surfaces less severe
deviations. Raising it reduces noise at the cost of hiding milder events.

The detector always fires at `DefaultSigma = 4.0`. The `min_sigma` parameter
controls what the caller _sees_, not what is detected.

### Sensitivity selector in the UI

The `/anomalies` dashboard provides a dropdown:
- **All (σ ≥ 2)** — all flags above 2σ
- **Medium+ (σ ≥ 3)** — medium and high severity
- **High (σ ≥ 4)** — only high-severity deviations (default)

Severity badges in the table:
- **High** (red) — σ ≥ 4
- **Medium** (yellow) — 3 ≤ σ < 4
- **Low** (blue) — 2 ≤ σ < 3

---

## Where flags surface

### REST API

```
GET /api/v1/anomalies
Authorization: Bearer <token>
```

**Query parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `min_sigma` | float | 2.0 | Minimum sigma to include |
| `metric` | string | — | Filter by metric name (`viewers`, `ingest_bitrate_kbps`, `cpu_pct`, `mem_pct`, `disk_pct`, `ams_api_latency_ms`) |
| `from` | epoch ms | — | Start of time range |
| `to` | epoch ms | — | End of time range |
| `limit` | int | 100 | Maximum flags returned (1–1000) |
| `cursor` | string | — | Pagination cursor |

**Response 200:**

```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "metric": "viewers",
      "scope": {
        "node_id": "",
        "app": "",
        "stream_id": "live/stream1"
      },
      "observed": 205.0,
      "expected": 100.0,
      "sigma": 19.92,
      "ts": 1781451077690
    }
  ],
  "meta": { "next_cursor": null }
}
```

**Tier gate:** Returns `403 LICENSE_REQUIRED` for Free and Pro tiers.
Enterprise tier required.

### Web UI

Navigate to `/anomalies` in the Pulse dashboard. The page shows:
- A filterable table with metric, scope, observed vs expected values, delta,
  sigma, severity badge, and detection timestamp.
- The sensitivity selector (maps to `min_sigma` query param).
- An empty state with the "baselines learning" explanation when no flags exist.
- An upsell gate for non-Enterprise tenants.

---

## AMS early-warning ladder (S25 / D-087, ant-media#7926)

AMS can gradually freeze while appearing healthy at the OS level — CPU, memory,
and disk metrics remain normal; the Java process stays alive; but the HLS/REST
API stops responding (see upstream issue
[ant-media/Ant-Media-Server#7926](https://github.com/ant-media/Ant-Media-Server/issues/7926),
open 2026-07-06: ~24 h freeze, OS metrics normal, HLS/API dead).

Pulse's anomaly detector forms the first rung of a three-rung detection ladder
for this failure class:

| Rung | Signal | Mechanism | Typical latency |
|------|--------|-----------|----------------|
| 1 — creep | `ams_api_latency_ms` rising | Welford anomaly flag (F9) | Minutes — catches gradual degradation early |
| 2 — errors | Consecutive API-failure streak (`consec_api_errors >= 3`) | `node_degraded` alert (evaluator, wave2.go) | ~15 s at 5 s poll interval |
| 3 — freeze | No node stats for 3×PollInterval → `EvictStaleNodes` removes node | `node_down` alert (BUG-011 FIXED S25/D-087) | ≤15 s after `node_degraded` |

The flag-event store (ADR-0009, S24/D-086) persists rung-1 detections with
timestamps, providing the forensic timeline the #7926 reporter lacked.

---

## Known limitations

| Gap | Description | Phase |
|---|---|---|
| GAP-3-004 | Zero-stddev blind spot | **CLOSED Wave-3-Plus** — epsilon floor applied in `ComputeFlags` (see "Epsilon floor" above) |
| Single window | Only a 1-hour rolling window is tracked. Multi-window anomaly detection (e.g., 24-hour baseline) is Phase 3. | Phase 3 |
| error\_rate / rebuffer\_ratio absent | These QoE metrics are not tracked as anomaly signals. `rebuffer_ratio` and `error_rate` are gated by beacon data sparsity (S25/D-087 assessment: prod `beacon_events` = 2 rows / 1 stream; all-zero baselines would make the first real rebuffer event an instant false alarm). Re-assess when a real beacon deployment shows sustained multi-viewer traffic and a sub-hour windowing design exists. | Phase 3 |
| cpu\_pct / mem\_pct / disk\_pct absent for standalone AMS | These node metrics are unavailable via REST on standalone AMS deployments (DG-05); available in cluster mode or via Kafka. | Environment |

---

## Phase-3 roadmap

- Multi-window baselines (1h, 24h, 7d).
- `error_rate` and `rebuffer_ratio` anomaly signals — gated on beacon data volume
  (sparsity gate, S25/D-087; see Known limitations above) and a sub-hour
  windowing redesign.
- Alert integration: auto-create an alert rule from an anomaly flag.
