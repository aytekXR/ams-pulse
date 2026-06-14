# F9 Anomaly Detection — Adversarial Validation Findings

**Validator:** QA adversarial subagent  
**Date:** 2026-06-14  
**Area:** F9 anomaly detection  
**Verdict:** PARTIAL — PRD acceptance criterion met on paper but two major gaps undercut the claim

---

## PRD requirement (§7.9)

> F9 (Phase 3). Anomaly Detection  
> Baseline-deviation flags on viewers, errors and rebuffering ("this Tuesday looks wrong"), simple statistical models first, no ML theater. Acceptance: fewer than 1 false alarm per node-week at default sensitivity.

Tier: Enterprise only (§7.11 pricing table).

---

## Budget check

| Budget | Source | Claimed | Verified | Status |
|---|---|---|---|---|
| < 1 false alarm/node-week at default sensitivity | PRD F9 | 0.2594/node-week | Model math correct (Gaussian); PRD target met on formula | PASS (model only) |
| Flags on viewers | PRD F9 | viewers tracked | Implemented | PASS |
| Flags on errors | PRD F9 | NOT tracked | Only cpu_pct in code | FAIL |
| Flags on rebuffering | PRD F9 | NOT tracked | Only mem_pct in code | FAIL |

---

## FINDING-1 [major, high confidence]: PRD metric mismatch — errors and rebuffering not tracked

PRD F9: "baseline-deviation flags on viewers, errors and rebuffering"

Implementation tracks: viewers (per stream), cpu_pct (per node), mem_pct (per node).
error_rate and rebuffer_ratio are not computed by UpdateBaselines. They are not in domain.LiveStream or domain.LiveNodeStats in a form the anomaly detector reads.

Evidence: server/internal/anomaly/anomaly.go lines 226-246 only collect viewers, cpu_pct, mem_pct.
server/internal/domain/types.go line 279: LiveStream has no error_rate field.
anomaly.go line 42 (package comment) says "3 metrics (viewers, error_rate, rebuffer_ratio)" but this is wrong — actual metrics are viewers + cpu_pct + mem_pct.

The frontend test data (sampleFlags in AnomaliesPage.test.tsx lines 107-123) includes error_rate and rebuffer_ratio as mock data, but these will never appear from the real backend.

**Owner:** BE-02

---

## FINDING-2 [major, high confidence]: False-alarm rate test is a pure model calculation, not a simulation

TestAnomaly_FalseAlarmRate_ModeledTarget does not feed random Gaussian noise to UpdateBaselines, call ComputeFlags, and count false alarms. It computes 0.2594/node-week by plugging P(|Z|>=4.0)=6.33e-5 into the renewal-process formula. The test would PASS even if ComputeFlags never fired a flag (e.g., if stddev were always 0 due to GAP-3-004).

The gate report labels this MEASURED in Architecture §4. It is a theoretical model assertion.

Additional note: the code uses Welford sample stddev (divides by n-1). The test statistic (x - sample_mean)/sample_stddev follows t(n-1), not N(0,1). At n=30 (MinSamples), P(|T|>=4.0 | df=29) is approximately 3.9e-4 vs 6.33e-5 for normal — approximately 6x larger. After hysteresis the rate is still below 1.0/node-week (~0.29/node-week), so the PRD target is met, but the 0.2594 figure is underestimated by approximately 6x.

**Owner:** BE-02

---

## FINDING-3 [major, medium confidence]: OpenAPI spec declares 4 unimplemented query params for GET /anomalies

The GET /anomalies spec (contracts/openapi/pulse-api.yaml lines 667-672) declares: from, to, app, stream, limit, cursor.

The handler (server/internal/api/wave3.go handleAnomalies) only processes min_sigma and metric. The four other params are silently ignored. The kin-openapi conformance test only validates the response shape, not param handling. next_cursor is always null — real pagination is not implemented.

**Owner:** BE-02 / INT-01

---

## FINDING-4 [minor, high confidence]: OpenAPI spec missing 403 response for GET /anomalies

GET /anomalies returns HTTP 403 for non-Enterprise tiers (tested). The spec (pulse-api.yaml lines 685-694) lists only 200, 400, 401, 500 — no 403. API clients following the spec will not handle 403 correctly.

**Owner:** INT-01

---

## FINDING-5 [minor, high confidence]: Rolling window is not rolling — Welford uses unbounded history

window_s=3600 is stored on the baseline row but never used to expire or weight old observations. Every tick adds to the ever-growing accumulator. After 10,080 samples (one week), the baseline reflects all history, not just the last hour. A stream with 1000 viewers for 6 months followed by a 2× surge will barely move the z-score because the 26,000+ sample history dominates. ADR-0007 documents this as a known limitation.

**Owner:** BE-02

---

## FINDING-6 [minor, medium confidence]: GAP-3-004 zero-stddev severity understated

A node consistently at 5.0% CPU for 30+ minutes builds stddev=0. The next spike to 80% produces no flag because ComputeFlags guards `if b.Stddev <= 0 { continue }` (anomaly.go line 358). The most useful anomaly scenario — idle to spike — is precisely the zero-stddev case. The gap note says "Real production streams have natural variance" but AMS REST polls rounded floats that CAN be perfectly constant.

The epsilon floor fix is a one-liner and warrants higher priority than Phase 3.

**Owner:** BE-02

---

## FINDING-7 [cosmetic, high confidence]: Config.DefaultSigma comment says 3.5 but fallback is 4.0

anomaly.go line 134: "DefaultSigma is the default sigma threshold. 0 → DefaultSigma (3.5)."
Actual fallback at line 150: uses DefaultSigma = 4.0. Stale comment from prior design iteration.

**Owner:** BE-02

---

## FINDING-8 [minor, medium confidence]: Pro-tier anomaly gate not directly tested

TestLicense_CheckProbes_CheckAnomalies tests free → blocked, enterprise → allowed. No test verifies Pro → blocked for anomalies. setupProServer falls back to free tier. CheckAnomalies() code is correct (requires TierEnterprise) but the Pro-tier block path is untested (analogous to GAP-3-006 for probes).

**Owner:** BE-02

---

## Passing checks

- Welford M2 reconstruction math: CORRECT. M2 = stddev^2 * (old_count - 1) matches the invariant.
- Enterprise tier gating: CORRECT. CheckAnomalies() requires TierEnterprise; handler calls it first.
- Hysteresis cooldown: CORRECT. UpdateBaselines decrements per tick; ComputeFlags blocks within cooldown.
- MinSamples gate: CORRECT. Suppresses flags until 30 observations accumulated.
- kin-openapi conformance of 200 response shape: PASS (TestAnomalies_Conforms_OpenAPI).
