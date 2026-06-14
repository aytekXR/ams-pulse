# Anomaly Detection (F9)

**Status: Shipped (Wave 3-MVP)**
**Tier: Enterprise only**

Pulse detects statistical deviations in stream and node metrics using rolling
baselines. When a metric diverges beyond a configurable sigma threshold, the
system emits an anomaly flag visible via `GET /api/v1/anomalies` and in the
Anomalies dashboard at `/anomalies`.

---

## What F9 flags

The detector tracks three metrics per entity:

| Metric | Scope | Description |
|---|---|---|
| `viewers` | per-stream | Current viewer count for a stream |
| `cpu_pct` | per-node | Node CPU utilisation percentage |
| `mem_pct` | per-node | Node memory utilisation percentage |

A flag is emitted when the current observed value deviates from the stored
rolling mean by more than `sigma` standard deviations. The `sigma` value is
reported in the flag and in the UI (e.g. "19.92σ").

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

### False-alarm calibration math

Model: Gaussian tail probability + renewal-process hysteresis.

```
sigma=4.0: P(|Z| >= 4.0) ≈ 6.33e-5  (two-tailed standard normal)

ticks/node/week = 7 × 24 × 3600 / 60 = 10,080
metrics/node    = 3 (viewers, cpu_pct, mem_pct)

lambda_raw per metric = 10,080 × 6.33e-5 = 0.638 exceedances/week

With hysteresis (renewal-process suppression):
  lambda_effective = lambda_raw / (1 + lambda_raw × HysteresisTicks)
                   = 0.638 / (1 + 0.638 × 10)
                   = 0.638 / 7.38
                   ≈ 0.086/week/metric

Total across 3 metrics: 0.086 × 3 = 0.259 false alarms/node/week
```

**Measured result: 0.259/node-week < 1.0/node-week — PRD F9 target met.**

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
| `metric` | string | — | Filter by metric name (`viewers`, `cpu_pct`, `mem_pct`) |
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

## Known limitations (Wave 3-MVP, D-001)

| Gap | Description | Phase |
|---|---|---|
| GAP-3-004 | Zero-stddev blind spot: perfectly constant metrics cannot produce a z-score. Real production streams have natural variance; this is non-blocking for MVP. | Phase 3: epsilon floor |
| Single window | Only a 1-hour rolling window is tracked. Multi-window anomaly detection (e.g., 24-hour baseline) is Phase 3. | Phase 3 |
| 3 metrics only | `viewers`, `cpu_pct`, `mem_pct` are tracked. Additional metrics (e.g., `error_rate`, `rebuffer_ratio`) are Phase 3 extensions. | Phase 3 |

---

## Phase-3 roadmap

- Multi-window baselines (1h, 24h, 7d).
- Additional metrics: `error_rate`, `rebuffer_ratio`, `ingest_bitrate_floor`.
- Epsilon floor for zero-stddev detection.
- Flag persistence in a dedicated table (currently computed on-read from live snapshot + baselines).
- Alert integration: auto-create an alert rule from an anomaly flag.
