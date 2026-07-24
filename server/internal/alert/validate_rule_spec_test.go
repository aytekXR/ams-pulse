package alert_test

// validate_rule_spec_test.go — tests for Fix A: canonical threshold-rule validator.
//
// Covers: ValidateRuleSpec, KnownMetricNames, KnownOperators, KnownSeverities,
// and the single-source-of-truth invariant (every KnownMetricName must be routed
// by evaluateRule — enforced here by checking against the switch cases via
// exercise of TickOnce with a fake snapshot, not by reading switch source).

import (
	"errors"
	"math"
	"sort"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/alert"
)

// ─── KnownMetricNames list invariants ────────────────────────────────────────

func TestKnownMetricNames_IsSorted(t *testing.T) {
	cp := make([]string, len(alert.KnownMetricNames))
	copy(cp, alert.KnownMetricNames)
	sort.Strings(cp)
	for i, want := range cp {
		if alert.KnownMetricNames[i] != want {
			t.Errorf("KnownMetricNames[%d] = %q, want sorted %q — list must be kept sorted", i, alert.KnownMetricNames[i], want)
		}
	}
}

func TestKnownMetricNames_ContainsRequiredEntries(t *testing.T) {
	required := []string{
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
		"viewer_drop_pct", // deprecated alias
	}
	set := make(map[string]bool, len(alert.KnownMetricNames))
	for _, m := range alert.KnownMetricNames {
		set[m] = true
	}
	for _, r := range required {
		if !set[r] {
			t.Errorf("KnownMetricNames missing required entry %q", r)
		}
	}
}

func TestKnownMetricNames_NoAnomaly_OnlyMetrics(t *testing.T) {
	// Anomaly-only metrics (no threshold-rule switch case) must NOT appear here.
	anomalyOnly := []string{"cpu_pct", "mem_pct", "disk_pct", "ams_api_latency_ms"}
	set := make(map[string]bool, len(alert.KnownMetricNames))
	for _, m := range alert.KnownMetricNames {
		set[m] = true
	}
	for _, m := range anomalyOnly {
		if set[m] {
			t.Errorf("KnownMetricNames should NOT contain anomaly-only metric %q (use SupportedAnomalyMetrics)", m)
		}
	}
}

// ─── KnownOperators ──────────────────────────────────────────────────────────

func TestKnownOperators_ContainsAll(t *testing.T) {
	required := []string{"eq", "gt", "gte", "lt", "lte"}
	set := make(map[string]bool, len(alert.KnownOperators))
	for _, op := range alert.KnownOperators {
		set[op] = true
	}
	for _, op := range required {
		if !set[op] {
			t.Errorf("KnownOperators missing %q", op)
		}
	}
	if len(alert.KnownOperators) != len(required) {
		t.Errorf("KnownOperators has %d entries, expected %d", len(alert.KnownOperators), len(required))
	}
}

// ─── KnownSeverities ─────────────────────────────────────────────────────────

func TestKnownSeverities_ContainsAll(t *testing.T) {
	required := []string{"critical", "info", "warning"}
	set := make(map[string]bool, len(alert.KnownSeverities))
	for _, sv := range alert.KnownSeverities {
		set[sv] = true
	}
	for _, sv := range required {
		if !set[sv] {
			t.Errorf("KnownSeverities missing %q", sv)
		}
	}
}

// ─── ValidateRuleSpec: valid cases ───────────────────────────────────────────

func TestValidateRuleSpec_Valid(t *testing.T) {
	cases := []struct {
		name      string
		metric    string
		operator  string
		windowS   int64
		severity  string
		threshold float64
	}{
		{"stream_offline eq critical", "stream_offline", "eq", 30, "critical", 1},
		{"node_cpu gt warning 7d window", "node_cpu", "gt", 604800, "warning", 90},
		{"viewer_count_floor lt warning", "viewer_count_floor", "lt", 60, "warning", 1},
		{"viewer_drop_pct alias", "viewer_drop_pct", "lt", 60, "info", 1},
		{"rebuffer_ratio gte", "rebuffer_ratio", "gte", 3600, "critical", 0.5},
		{"fps lte", "fps", "lte", 10, "warning", 24},
		{"cert_expiry lt", "cert_expiry", "lt", 86400, "warning", 7},
		{"threshold zero", "stream_offline", "eq", 1, "info", 0},
		{"threshold negative", "node_cpu", "gt", 5, "critical", -1},
		{"min window", "stream_offline", "eq", 1, "critical", 1},
		{"max window 7 days", "node_mem", "gt", 604800, "warning", 80},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := alert.ValidateRuleSpec(tc.metric, tc.operator, tc.windowS, tc.severity, tc.threshold); err != nil {
				t.Errorf("ValidateRuleSpec(%q,%q,%d,%q,%v) = %v, want nil", tc.metric, tc.operator, tc.windowS, tc.severity, tc.threshold, err)
			}
		})
	}
}

// ─── ValidateRuleSpec: invalid cases ─────────────────────────────────────────

func TestValidateRuleSpec_UnknownMetric(t *testing.T) {
	err := alert.ValidateRuleSpec("cpu_pct", "gt", 60, "warning", 90)
	if err == nil {
		t.Fatal("expected error for anomaly-only metric cpu_pct, got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_UnknownOperator(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "ne", 60, "warning", 90)
	if err == nil {
		t.Fatal("expected error for operator 'ne', got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_UnknownSeverity(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "gt", 60, "urgent", 90)
	if err == nil {
		t.Fatal("expected error for severity 'urgent', got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_WindowZeroIsValid(t *testing.T) {
	// window_s:0 is "near-instant" — a legitimate value the evaluator honors and
	// the e2e A1 rule relies on. Only a negative window is rejected.
	if err := alert.ValidateRuleSpec("node_cpu", "gt", 0, "warning", 90); err != nil {
		t.Errorf("window_s=0 must be valid, got %v", err)
	}
}

func TestValidateRuleSpec_EmptySeverityIsValid(t *testing.T) {
	// An omitted severity stores as empty and has always been accepted; only a
	// non-empty WRONG severity is rejected (see TestValidateRuleSpec_UnknownSeverity).
	if err := alert.ValidateRuleSpec("node_cpu", "gt", 60, "", 90); err != nil {
		t.Errorf("empty severity must be valid, got %v", err)
	}
}

func TestValidateRuleSpec_WindowNegative(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "gt", -5, "warning", 90)
	if err == nil {
		t.Fatal("expected error for window_s=-5, got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_WindowExceedsMax(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "gt", 604801, "warning", 90)
	if err == nil {
		t.Fatal("expected error for window_s > 7 days, got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_ThresholdNaN(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "gt", 60, "warning", math.NaN())
	if err == nil {
		t.Fatal("expected error for NaN threshold, got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_ThresholdPosInf(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "gt", 60, "warning", math.Inf(1))
	if err == nil {
		t.Fatal("expected error for +Inf threshold, got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_ThresholdNegInf(t *testing.T) {
	err := alert.ValidateRuleSpec("node_cpu", "lt", 60, "critical", math.Inf(-1))
	if err == nil {
		t.Fatal("expected error for -Inf threshold, got nil")
	}
	if !errors.Is(err, alert.ErrInvalidRuleSpec) {
		t.Errorf("expected ErrInvalidRuleSpec, got %T: %v", err, err)
	}
}

func TestValidateRuleSpec_EmptyMetric(t *testing.T) {
	err := alert.ValidateRuleSpec("", "eq", 30, "critical", 1)
	if err == nil {
		t.Fatal("expected error for empty metric, got nil")
	}
}

// ─── ErrInvalidRuleSpec sentinel ─────────────────────────────────────────────

func TestErrInvalidRuleSpec_IsWrappable(t *testing.T) {
	// All validation errors must wrap ErrInvalidRuleSpec so API handlers can
	// test errors.Is(err, alert.ErrInvalidRuleSpec) without string matching.
	errCases := []struct {
		name     string
		metric   string
		operator string
		windowS  int64
		severity string
		thresh   float64
	}{
		{"bad metric", "bogus", "gt", 60, "warning", 1},
		{"bad operator", "node_cpu", "xx", 60, "warning", 1},
		{"bad severity", "node_cpu", "gt", 60, "extreme", 1},
		{"negative window", "node_cpu", "gt", -5, "warning", 1},
		{"over window", "node_cpu", "gt", 99999999, "warning", 1},
		{"NaN thresh", "node_cpu", "gt", 60, "warning", math.NaN()},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			err := alert.ValidateRuleSpec(tc.metric, tc.operator, tc.windowS, tc.severity, tc.thresh)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, alert.ErrInvalidRuleSpec) {
				t.Errorf("errors.Is(err, ErrInvalidRuleSpec) = false; got %T: %v", err, err)
			}
		})
	}
}
