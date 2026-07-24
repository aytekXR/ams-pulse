package alert

// validate.go — canonical threshold-rule spec validator (Fix A).
//
// This file owns the single-source lists for valid metric names, operators, and
// severities. The evaluateRule switch in evaluator.go and evalGenericMetric MUST
// stay in sync with KnownMetricNames; comments at those switch sites cross-reference
// this file. Anomaly rules use the separate SupportedAnomalyMetrics list in wave3.go.

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// maxWindowS is the upper bound for alert rule window_s (7 days = 604800 s).
// Keeps evaluation windows human-auditable and within typical ClickHouse
// retention policies; a 7-day window is far beyond any reasonable alert latency
// budget (PRD F5 §4 cap is 30 s).
const maxWindowS int64 = 7 * 24 * 60 * 60 // 604800

// KnownMetricNames is the authoritative sorted list of valid metric names for
// threshold-rule evaluation. Every name here must be handled by the
// evaluateRule switch in evaluator.go (directly or via evalGenericMetric).
//
// viewer_count_floor is the canonical name for the per-stream viewer floor
// metric; viewer_drop_pct is its deprecated alias (accepted, not preferred — Fix D).
// Anomaly-only metrics (cpu_pct, mem_pct, disk_pct, ams_api_latency_ms) are
// absent here; use SupportedAnomalyMetrics() in wave3.go for anomaly rules.
var KnownMetricNames = func() []string {
	m := []string{
		"cert_expiry",
		"error_rate",
		"fps",
		"ingest_bitrate_floor",
		"ingest_bitrate_kbps",
		"license_expiry",
		"node_cpu",
		"node_degraded",
		"node_disk",
		"node_down",
		"node_mem",
		"rebuffer_ratio",
		"stream_offline",
		"viewer_count",
		"viewer_count_floor",
		"viewer_drop_pct", // deprecated alias for viewer_count_floor
	}
	sort.Strings(m)
	return m
}()

// knownMetricSet is KnownMetricNames indexed for O(1) lookup.
var knownMetricSet = func() map[string]bool {
	s := make(map[string]bool, len(KnownMetricNames))
	for _, m := range KnownMetricNames {
		s[m] = true
	}
	return s
}()

// KnownOperators is the authoritative list of valid threshold-rule operators.
// They map 1-to-1 with the compare() function cases in evaluator.go.
var KnownOperators = []string{"eq", "gt", "gte", "lt", "lte"}

// knownOperatorSet is KnownOperators indexed for O(1) lookup.
var knownOperatorSet = func() map[string]bool {
	s := make(map[string]bool, len(KnownOperators))
	for _, op := range KnownOperators {
		s[op] = true
	}
	return s
}()

// KnownSeverities is the authoritative list of valid alert rule severity levels.
var KnownSeverities = []string{"critical", "info", "warning"}

// knownSeveritySet is KnownSeverities indexed for O(1) lookup.
var knownSeveritySet = func() map[string]bool {
	s := make(map[string]bool, len(KnownSeverities))
	for _, sv := range KnownSeverities {
		s[sv] = true
	}
	return s
}()

// ErrInvalidRuleSpec is the sentinel error returned by ValidateRuleSpec.
// Callers may use errors.Is to test for this class of error.
var ErrInvalidRuleSpec = errors.New("invalid alert rule spec")

// ValidateRuleSpec checks that a threshold-rule specification is self-consistent
// and within accepted bounds. A follow-up agent wires this into the API
// create/update handler; this function only validates, never persists.
//
// Validation rules:
//   - metric must be in KnownMetricNames (see also: SupportedAnomalyMetrics for anomaly rules)
//   - operator must be in KnownOperators
//   - severity, when non-empty, must be in KnownSeverities. An EMPTY severity is
//     accepted: the create handler treats an omitted severity as "unspecified"
//     and has always stored it as such (the seeded rules set one explicitly, but
//     the API never required it). We reject a WRONG severity, not a missing one.
//   - windowSeconds: 0 ≤ windowSeconds ≤ maxWindowS. Zero is a legitimate value —
//     the evaluator treats window_s:0 as "near-instant" (evaluate the latest
//     sample with no lookback), which the e2e A1 rule relies on. Only a NEGATIVE
//     window is nonsense (the review's -3600 payload).
//   - threshold must be finite: NaN and ±Inf are rejected because compare() with
//     a non-finite threshold silently always returns false or true, producing
//     permanently-stuck alerts that are indistinguishable from real firing ones
func ValidateRuleSpec(metric, operator string, windowSeconds int64, severity string, threshold float64) error {
	if !knownMetricSet[metric] {
		return fmt.Errorf("%w: unknown metric %q (see alert.KnownMetricNames)", ErrInvalidRuleSpec, metric)
	}
	if !knownOperatorSet[operator] {
		return fmt.Errorf("%w: unknown operator %q (valid: eq, gt, gte, lt, lte)", ErrInvalidRuleSpec, operator)
	}
	if severity != "" && !knownSeveritySet[severity] {
		return fmt.Errorf("%w: unknown severity %q (valid: critical, warning, info)", ErrInvalidRuleSpec, severity)
	}
	if windowSeconds < 0 {
		return fmt.Errorf("%w: window_s must be >= 0, got %d", ErrInvalidRuleSpec, windowSeconds)
	}
	if windowSeconds > maxWindowS {
		return fmt.Errorf("%w: window_s must be <= %d (7 days), got %d", ErrInvalidRuleSpec, maxWindowS, windowSeconds)
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return fmt.Errorf("%w: threshold must be finite, got %v", ErrInvalidRuleSpec, threshold)
	}
	return nil
}
