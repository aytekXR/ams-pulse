# ADR 0007: Anomaly detection uses Welford online algorithm (not EWMA)

**Status:** Accepted · **Date:** 2026-06-14 · **Wave:** 3-MVP

## Context

F9 anomaly detection (PRD §7.9) requires rolling baseline statistics per
(metric, scope, window) to detect deviations. Two common algorithms were
considered:

1. **Exponentially Weighted Moving Average (EWMA)** — maintains a single
   exponential smoothed mean. Adding variance requires a parallel EWMA for
   the squared deviation.
2. **Welford's online algorithm** — computes exact sample mean and variance
   (population or sample) in a single pass without storing history, using
   numerically stable recurrence relations.

## Decision

Use **Welford's online algorithm** for rolling baseline statistics.

Only `mean`, `stddev`, and `sample_count` are persisted per
(metric, scope, window_s) in the `anomaly_baselines` meta-store table.
M2 (the variance accumulator) is re-derived on each update as
`M2_prev = stddev² × (n-1)` to avoid storing a fourth column.

## Rationale

- **Exact statistics.** Welford gives exact sample mean and variance for the
  observed data, which simplifies the false-alarm calibration math: the
  |Z| >= sigma threshold has a direct Gaussian interpretation.
- **No decay parameter.** EWMA requires a tunable α (decay constant) that
  affects both false-alarm rate and detection sensitivity in coupled ways.
  Welford's sample count serves as the natural stabilization gate instead
  (MinSamples = 30).
- **Numerically stable.** The two-pass Welford formula avoids catastrophic
  cancellation in variance computation, unlike the naive E[X²] - E[X]² form.
- **Minimal storage.** Three columns (mean, stddev, sample_count) per row.
  No ring buffer or history table is needed.

## False-alarm calibration

With Welford statistics and σ=4.0 threshold, the Gaussian tail probability
P(|Z| ≥ 4.0) ≈ 6.33e-5 per tick. With hysteresis suppression (10 ticks
cooldown after each flag), the renewal-process effective rate is
0.259 false alarms/node-week across 3 metrics — below the PRD target of
< 1/node-week.

See `docs/guides/anomaly-detection.md` §Sensitivity for the full derivation.
Measured by: `TestAnomaly_FalseAlarmRate_ModeledTarget` in
`server/internal/anomaly/anomaly_test.go`.

## Consequences

- **Zero-stddev blind spot.** When a metric stream is perfectly constant,
  Welford converges to stddev=0 and |Z| is undefined (guarded — no flag
  fires). Real production streams have natural variance; this is acceptable
  for MVP. Phase-3 mitigation: epsilon floor on stddev. (GAP-3-004)
- **No window expiry.** The current implementation is an ever-growing sample.
  "Rolling window" refers to the single 3600 s window_s tag on the baseline
  row, not to evicting old samples from the Welford accumulator. Phase-3:
  time-windowed Welford or reservoir sampling.
- **Phase-3 EWMA alternative.** If a continuous stream with a slowly drifting
  mean is needed (e.g., for multi-hour trend detection), EWMA may be
  re-evaluated. The `BaselineStore` interface (`ListAnomalyBaselines` /
  `UpsertAnomalyBaseline`) is the abstraction boundary; swapping the algorithm
  requires only replacing the update logic in `anomaly.Detector.UpdateBaselines`.

## Amendments

### D-087 (2026-07-12): ams_api_latency_ms metric + budget update

**Metric count and budget update.**
The historical '3 metrics' text (viewers, error_rate, rebuffer_ratio) in the
original calibration section is now stale: `rebuffer_ratio` and `error_rate`
are excluded by the wave-3 sparsity gate (pinned in `wave3_test.go:709`
`TestValidateAnomalyRule_UnsupportedMetric`). The updated budget is:

| Scope       | Metrics                                          | Count |
|-------------|--------------------------------------------------|-------|
| node-scoped | cpu_pct, mem_pct, disk_pct, ams_api_latency_ms   | 4     |
| as-if-node  | viewers (1-stream conservative bound)            | +1    |
| stream-only | ingest_bitrate_kbps (scales with streams, excl.) | —     |

Conservative 5-metric budget: `0.08644 × 5 = 0.4322/node-week < 1.0 PRD target`.
Pinned by `TestAnomaly_FalseAlarmRate_ModeledTarget` (`metricsPerNode=5`).

**Semantic novelty: ams_api_latency_ms is the first Pulse-measured metric.**
All previous metrics (cpu_pct, mem_pct, disk_pct, viewers, ingest_bitrate_kbps)
are values *reported by AMS* and forwarded by the poller. `ams_api_latency_ms`
is the round-trip time of Pulse's own `SystemStats`/`ClusterNodes` HTTP call —
it monitors the monitoring path itself, enabling early detection of the
AMS-freeze failure class (ant-media/Ant-Media-Server#7926, D-087 rung 1).

**Honest caveat:** because this latency is measured at the Pulse host, a
network event or Pulse-side CPU spike can also move it independently of AMS
health. This is acceptable for an early-warning signal: a genuine AMS freeze
produces a sustained latency rise that persists across rung-1 (anomaly), rung-2
(consecutive error streak), and rung-3 (eviction timeout) — transient Pulse-side
slowdowns typically do not. The signal is complementary, not definitive.

**Key-absent semantics.** `APILatencyMS==0` signals that the last stats call
*failed* (or the node has never been polled). Feeding 0 to Welford would poison
the baseline toward zero and make normal latency look anomalous. The presence
guard (`if n.APILatencyMS > 0`) is pinned by `TestAnomaly_APILatencyMS_NoMeasurement_NoBaseline`.
