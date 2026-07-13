package alert_test

// wave3_d087_test.go — D-087 AMS early-warning ladder rung 1 tests.
// Tests for ams_api_latency_ms anomaly metric and the map+switch parity pin.
// Written RED-first: all tests fail until wave3.go is updated.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// snapWithNodeAPILatency returns a LiveSnapshot with one node that has APILatencyMS set.
func snapWithNodeAPILatency(nodeID string, apiLatencyMS float64) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			nodeID: {NodeID: nodeID, APILatencyMS: apiLatencyMS},
		},
	}
}

// ─── TestValidateAnomalyRule_AmsAPILatencyMS_Supported ───────────────────────

// ams_api_latency_ms must pass ValidateAnomalyRule (it is a supported anomaly metric).
// RED before map update: returns error "not supported".
func TestValidateAnomalyRule_AmsAPILatencyMS_Supported(t *testing.T) {
	rule := meta.AlertRuleRow{RuleType: "anomaly", Metric: "ams_api_latency_ms", WindowS: 3600}
	if err := alert.ValidateAnomalyRule(rule); err != nil {
		t.Errorf("ams_api_latency_ms should be a supported anomaly metric, got error: %v", err)
	}
}

// TestValidateAnomalyRule_SupportedMetrics_IncludesAmsLatency checks all 6 supported metrics.
// RED before map update: ams_api_latency_ms is missing → validation errors.
func TestValidateAnomalyRule_SupportedMetrics_IncludesAmsLatency(t *testing.T) {
	all := []string{"viewer_count", "ingest_bitrate_kbps", "cpu_pct", "mem_pct", "disk_pct", "ams_api_latency_ms"}
	for _, metric := range all {
		rule := meta.AlertRuleRow{RuleType: "anomaly", Metric: metric, WindowS: 3600}
		if err := alert.ValidateAnomalyRule(rule); err != nil {
			t.Errorf("metric %q: expected no validation error, got: %v", metric, err)
		}
	}
}

// TestValidateAnomalyRule_ErrorMsg_IncludesAmsLatency verifies the error message
// for an unsupported metric mentions ams_api_latency_ms in the supported list.
// RED before message update: the error message lists the old 5 metrics only.
func TestValidateAnomalyRule_ErrorMsg_IncludesAmsLatency(t *testing.T) {
	rule := meta.AlertRuleRow{RuleType: "anomaly", Metric: "bad_metric", WindowS: 3600}
	err := alert.ValidateAnomalyRule(rule)
	if err == nil {
		t.Fatal("expected error for unsupported metric, got nil")
	}
	var valErr *alert.AnomalyRuleValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *AnomalyRuleValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(valErr.Message, "ams_api_latency_ms") {
		t.Errorf("error message should mention 'ams_api_latency_ms', got: %q", valErr.Message)
	}
}

// ─── TestEvalAnomalyMetric_AmsAPILatencyMS_Fires ─────────────────────────────

// A node with APILatencyMS=500ms fires when baseline mean=50ms, stddev=10ms, sigma=2.0.
// z = (500-50)/max(10, max(0.05*50=2.5, 1e-9)) = 450/10 = 45 >> 2.0 → fires.
// RED before switch update: evalAnomalyMetric hits default case, reader never called.
func TestEvalAnomalyMetric_AmsAPILatencyMS_Fires(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: nodeBaseline("ams_api_latency_ms", "node-1", 50.0, 10.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "api-latency-fires", "ams_api_latency_ms", 2.0, 5)

	live.setSnap(snapWithNodeAPILatency("node-1", 500.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("expected alert to fire for ams_api_latency_ms anomaly (z=45 >> sigma=2.0), got 0 history entries")
	}
	if hist[0].Metric != "ams_api_latency_ms" {
		t.Errorf("expected metric=ams_api_latency_ms, got %q", hist[0].Metric)
	}
	if hist[0].State != "firing" {
		t.Errorf("expected state=firing, got %q", hist[0].State)
	}
	// Value = observed APILatencyMS.
	if hist[0].Value != 500.0 {
		t.Errorf("expected Value=500.0 (observed api_latency_ms), got %g", hist[0].Value)
	}
}

// ─── TestEvalAnomalyMetric_AmsAPILatencyMS_NoFireBelowSigma ─────────────────

// When APILatencyMS is close to baseline, no fire.
// mean=50, stddev=10; APILatencyMS=55 → z=(55-50)/10=0.5 < sigma=2.0 → no fire.
func TestEvalAnomalyMetric_AmsAPILatencyMS_NoFireBelowSigma(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: nodeBaseline("ams_api_latency_ms", "node-1", 50.0, 10.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "api-latency-no-fire", "ams_api_latency_ms", 2.0, 5)

	live.setSnap(snapWithNodeAPILatency("node-1", 55.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history (z=0.5 < sigma=2.0), got %d", len(hist))
	}
}

// ─── TestEvalAnomalyMetric_AmsAPILatencyMS_ZeroSkip ─────────────────────────

// When APILatencyMS==0 (the failure path — key absent per D-075 semantics),
// the node must be skipped entirely (reader NOT called for that node).
// This tests the presence guard: 0 means "not measured", not "0ms latency".
func TestEvalAnomalyMetric_AmsAPILatencyMS_ZeroSkip(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	calledWith := &[]string{}
	reader := &alert.FakeAnomalyBaselineReader{
		Row:        nodeBaseline("ams_api_latency_ms", "node-1", 50.0, 10.0, 30),
		CalledWith: calledWith,
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "api-latency-zero-skip", "ams_api_latency_ms", 2.0, 5)

	// APILatencyMS=0 means the last call failed (key-absent semantics).
	live.setSnap(snapWithNodeAPILatency("node-1", 0.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	reader.Mu.Lock()
	calls := *reader.CalledWith
	reader.Mu.Unlock()

	// Presence guard: reader must NOT be called when APILatencyMS==0.
	if len(calls) > 0 {
		t.Errorf("expected reader NOT called when APILatencyMS==0 (failure path), got calls: %v", calls)
	}
	// Also verify no history entry.
	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history entries when APILatencyMS==0, got %d", len(hist))
	}
}

// ─── TestAnomalyMetricMapSwitchParity ────────────────────────────────────────

// Parity test: every metric in supportedAnomalyMetrics must be handled by
// evalAnomalyMetric's dispatch switch. A metric in the map but missing from
// the switch hits the default case (Warn + nil), which silently skips the rule.
// This test detects that silent-nil trap by verifying the reader IS called.
// RED for ams_api_latency_ms before switch update.
func TestAnomalyMetricMapSwitchParity(t *testing.T) {
	type metricCase struct {
		metric    string
		readerKey string // expected baseline lookup key (after alias)
		snap      *domain.LiveSnapshot
		baseline  *anomaly.AnomalyBaselineRow
	}

	cases := []metricCase{
		{
			metric:    "viewer_count",
			readerKey: "viewers",
			snap:      snapWithStream("s1", 100),
			baseline:  streamBaseline("viewers", "s1", 10.0, 1.0, 30),
		},
		{
			metric:    "ingest_bitrate_kbps",
			readerKey: "ingest_bitrate_kbps",
			snap: &domain.LiveSnapshot{
				Streams: map[string]*domain.LiveStream{
					"s1": {StreamID: "s1", App: "live", Active: true, IngestBitrate: 1000.0},
				},
				Nodes: map[string]*domain.LiveNodeStats{},
			},
			baseline: streamBaseline("ingest_bitrate_kbps", "s1", 500.0, 50.0, 30),
		},
		{
			metric:    "cpu_pct",
			readerKey: "cpu_pct",
			snap:      snapWithNode("n1", 80.0, 50.0),
			baseline:  nodeBaseline("cpu_pct", "n1", 30.0, 5.0, 30),
		},
		{
			metric:    "mem_pct",
			readerKey: "mem_pct",
			snap:      snapWithNode("n1", 50.0, 90.0),
			baseline:  nodeBaseline("mem_pct", "n1", 40.0, 3.0, 30),
		},
		{
			metric:    "disk_pct",
			readerKey: "disk_pct",
			snap: &domain.LiveSnapshot{
				Streams: map[string]*domain.LiveStream{},
				Nodes: map[string]*domain.LiveNodeStats{
					// D-088: DiskPCTReported=true (cluster path emits disk_pct key).
					"n1": {NodeID: "n1", DiskPCT: 85.0, DiskPCTReported: true},
				},
			},
			baseline: nodeBaseline("disk_pct", "n1", 30.0, 5.0, 30),
		},
		{
			// ams_api_latency_ms: node-scoped, no alias, skip when value==0.
			metric:    "ams_api_latency_ms",
			readerKey: "ams_api_latency_ms",
			snap:      snapWithNodeAPILatency("n1", 500.0),
			baseline:  nodeBaseline("ams_api_latency_ms", "n1", 50.0, 10.0, 30),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.metric, func(t *testing.T) {
			store := openTestStore(t)
			live := newFakeLive()
			clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			ev, _ := newTestEvaluator(t, store, live, clock)

			calledWith := &[]string{}
			reader := &alert.FakeAnomalyBaselineReader{
				Row:        tc.baseline,
				CalledWith: calledWith,
			}
			ev.SetAnomalyBaselineReader(reader)

			ctx := context.Background()
			makeAnomalyRule(ctx, t, store, "parity-"+tc.metric, tc.metric, 1.0, 5)

			live.setSnap(tc.snap)
			clock.Advance(3601 * time.Second)
			ev.TickOnce(ctx)

			reader.Mu.Lock()
			calls := *reader.CalledWith
			reader.Mu.Unlock()

			if len(calls) == 0 {
				t.Errorf(
					"metric %q: GetAnomalyBaseline was never called — "+
						"metric is in supportedAnomalyMetrics but missing from evalAnomalyMetric switch (silent-nil trap)",
					tc.metric,
				)
				return
			}
			found := false
			for _, m := range calls {
				if m == tc.readerKey {
					found = true
				}
			}
			if !found {
				t.Errorf("metric %q: expected reader called with key %q, got: %v", tc.metric, tc.readerKey, calls)
			}
		})
	}
}
