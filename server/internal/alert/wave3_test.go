package alert_test

// wave3_test.go — TDD tests for S11 WO-B anomaly alert rule engine.
// Layer 1 tests: evalAnomalyMetric, SetAnomalyBaselineReader, ValidateAnomalyRule.
// All tests were written first (RED phase) before wave3.go existed.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// makeAnomalyRule creates an anomaly AlertRuleRow for test use.
func makeAnomalyRule(ctx context.Context, t *testing.T, store *meta.Store, name, metric string, sigma float64, minSamples int) meta.AlertRuleRow {
	t.Helper()
	row, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               name,
		Metric:             metric,
		RuleType:           "anomaly",
		WindowS:            3600,
		Sigma:              sigma,
		MinSamples:         minSamples,
		Severity:           "warning",
		Operator:           "gt",
		Threshold:          0,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule(%s): %v", name, err)
	}
	return row
}

// streamBaseline returns an AnomalyBaselineRow for a stream-level metric.
func streamBaseline(metric, streamID string, mean, stddev float64, samples int) *anomaly.AnomalyBaselineRow {
	return &anomaly.AnomalyBaselineRow{
		Metric:      metric,
		Scope:       fmt.Sprintf(`{"stream_id":"%s"}`, streamID),
		WindowS:     3600,
		Mean:        mean,
		Stddev:      stddev,
		SampleCount: samples,
		LastUpdated: time.Now().UnixMilli(),
	}
}

// nodeBaseline returns an AnomalyBaselineRow for a node-level metric.
func nodeBaseline(metric, nodeID string, mean, stddev float64, samples int) *anomaly.AnomalyBaselineRow {
	return &anomaly.AnomalyBaselineRow{
		Metric:      metric,
		Scope:       fmt.Sprintf(`{"node_id":"%s"}`, nodeID),
		WindowS:     3600,
		Mean:        mean,
		Stddev:      stddev,
		SampleCount: samples,
		LastUpdated: time.Now().UnixMilli(),
	}
}

// snapWithStream returns a LiveSnapshot containing one stream.
func snapWithStream(streamID string, viewerCount int) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			streamID: {StreamID: streamID, App: "live", Active: true, ViewerCount: viewerCount},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

// snapWithNode returns a LiveSnapshot containing one node.
func snapWithNode(nodeID string, cpuPct, memPct float64) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			nodeID: {NodeID: nodeID, CPUPCT: cpuPct, MemPCT: memPct},
		},
	}
}

// ─── TestEvalAnomalyMetric_NoReader ──────────────────────────────────────────

// When anomalyReader is nil, no history rows should be created (WARN logged).
func TestEvalAnomalyMetric_NoReader(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)
	// No anomalyReader set — should skip anomaly rules silently with WARN.

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "no-reader-rule", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("expected 0 history entries with no anomaly reader, got %d", len(hist))
	}
}

// ─── TestEvalAnomalyMetric_BelowMinSamples ───────────────────────────────────

// When SampleCount < effectiveMinSamples, no fire.
func TestEvalAnomalyMetric_BelowMinSamples(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// Baseline has only 2 samples; rule requires 30.
	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 2),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	_, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "below-min-samples",
		Metric:             "viewer_count",
		RuleType:           "anomaly",
		WindowS:            3600,
		Sigma:              0.5,
		MinSamples:         30, // requires 30; baseline has only 2
		Severity:           "warning",
		Operator:           "gt",
		Threshold:          0,
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history (below min_samples=30, got %d samples), got %d rows", 2, len(hist))
	}
}

// ─── TestEvalAnomalyMetric_NoBaseline ────────────────────────────────────────

// When GetAnomalyBaseline returns (nil, nil), no fire, no error.
func TestEvalAnomalyMetric_NoBaseline(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{Row: nil, Err: nil}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "no-baseline-rule", "viewer_count", 0.5, 2)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history (no baseline), got %d", len(hist))
	}
}

// ─── TestEvalAnomalyMetric_FiresAboveSigma ───────────────────────────────────

// When z-score exceeds effectiveSigma, the rule fires.
func TestEvalAnomalyMetric_FiresAboveSigma(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// mean=10, stddev=1 → effStddev = max(1.0, max(0.05*10=0.5, 1e-9)) = 1.0
	// viewer_count=10000 → z = (10000-10)/1.0 = 9990 >> 2.0 → fires
	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "fires-above-sigma", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("expected firing history entry, got 0")
	}
	if hist[0].Metric != "viewer_count" {
		t.Errorf("expected metric=viewer_count, got %q", hist[0].Metric)
	}
	if hist[0].State != "firing" {
		t.Errorf("expected state=firing, got %q", hist[0].State)
	}
	// S11 WO-B: history value = observed value (10000).
	if hist[0].Value != 10000.0 {
		t.Errorf("expected Value=10000 (observed), got %g", hist[0].Value)
	}
	// S11 WO-B: history threshold = baseline mean (10.0).
	if hist[0].Threshold != 10.0 {
		t.Errorf("expected Threshold=10.0 (baseline mean), got %g", hist[0].Threshold)
	}
}

// ─── TestEvalAnomalyMetric_NotifContainsExpectedAndSigma ─────────────────────

// Notification payload for anomaly rules includes expected and sigma_multiplier.
func TestEvalAnomalyMetric_NotifContainsExpectedAndSigma(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	var notifs []map[string]any
	var notifMu sync.Mutex
	ev.SetNotifySink(func(payload []byte) {
		var n map[string]any
		if err := json.Unmarshal(payload, &n); err == nil {
			notifMu.Lock()
			notifs = append(notifs, n)
			notifMu.Unlock()
		}
	})

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "notif-anomaly-fields", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	defer notifMu.Unlock()
	if len(notifs) == 0 {
		t.Fatal("expected notification, got 0")
	}
	n := notifs[0]
	if n["expected"] == nil {
		t.Error("notification missing 'expected' field for anomaly rule")
	}
	if expectedVal, ok := n["expected"].(float64); !ok || expectedVal != 10.0 {
		t.Errorf("expected 'expected'=10.0, got %v", n["expected"])
	}
	if n["sigma_multiplier"] == nil {
		t.Error("notification missing 'sigma_multiplier' field for anomaly rule")
	}
	// threshold in notification = baseline mean
	if thresh, ok := n["threshold"].(float64); !ok || thresh != 10.0 {
		t.Errorf("expected threshold=10.0 (baseline mean) in anomaly notification, got %v", n["threshold"])
	}
}

// ─── TestEvalAnomalyMetric_NoFireBelowSigma ──────────────────────────────────

// When z-score < effectiveSigma, no fire.
func TestEvalAnomalyMetric_NoFireBelowSigma(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// mean=10, stddev=1; viewer_count=11 → z=(11-10)/1.0=1.0 < 5.0 → no fire
	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "no-fire-below-sigma", "viewer_count", 5.0, 5)

	live.setSnap(snapWithStream("s1", 11))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history (z=1.0 < sigma=5.0), got %d", len(hist))
	}
}

// ─── TestEvalAnomalyMetric_StddevZero ────────────────────────────────────────

// stddev=0 baseline uses epsilon floor; a large deviation still fires.
func TestEvalAnomalyMetric_StddevZero(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// stddev=0, mean=10
	// effStddev = max(0, max(0.05*10=0.5, 1e-9)) = 0.5
	// viewer_count=20 → z = |20-10|/0.5 = 20 > 2.0 → fires
	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 0.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "stddev-zero", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 20))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("expected firing with stddev=0 and deviation=10 (z=20 > sigma=2.0)")
	}
}

// ─── TestEvalAnomalyMetric_ViewerCountAlias ──────────────────────────────────

// metric "viewer_count" maps to baseline key "viewers".
// Verified by using a CalledWith-tracking reader.
func TestEvalAnomalyMetric_ViewerCountAlias(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row:        nil,
		CalledWith: &[]string{},
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "alias-test", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 100))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	reader.Mu.Lock()
	calls := *reader.CalledWith
	reader.Mu.Unlock()

	found := false
	for _, m := range calls {
		if m == "viewers" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GetAnomalyBaseline called with metric='viewers', got: %v", calls)
	}
	// Must NOT be called with "viewer_count" (that's the alias key, not the storage key)
	for _, m := range calls {
		if m == "viewer_count" {
			t.Errorf("GetAnomalyBaseline should NOT be called with 'viewer_count' (use alias 'viewers'), got: %v", calls)
		}
	}
}

// ─── TestEvalAnomalyMetric_DefaultSigmaApplied ───────────────────────────────

// rule.Sigma=0 falls back to anomaly.DefaultSigma (4.0).
// mean=10, stddev=1, viewer_count=15 → z=5 > DefaultSigma=4.0 → fires.
func TestEvalAnomalyMetric_DefaultSigmaApplied(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	_, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "default-sigma",
		Metric:             "viewer_count",
		RuleType:           "anomaly",
		WindowS:            3600,
		Sigma:              0, // use DefaultSigma=4.0
		MinSamples:         5,
		Severity:           "warning",
		Operator:           "gt",
		Threshold:          0,
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// z = (15-10)/1.0 = 5 > DefaultSigma=4.0 → fires
	live.setSnap(snapWithStream("s1", 15))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("expected firing with z=5 > DefaultSigma=4.0")
	}
}

// ─── TestEvalAnomalyMetric_NodePath_CpuPct ───────────────────────────────────

// cpu_pct anomaly rule iterates snap.Nodes, groupKey = node_id.
func TestEvalAnomalyMetric_NodePath_CpuPct(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// mean=30, stddev=5; cpu=80 → z=(80-30)/5=10 > 2.0 → fires
	reader := &alert.FakeAnomalyBaselineReader{
		Row: nodeBaseline("cpu_pct", "node-1", 30.0, 5.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "cpu-anomaly", "cpu_pct", 2.0, 5)

	live.setSnap(snapWithNode("node-1", 80.0, 50.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("expected firing history for cpu_pct anomaly node rule")
	}
	if hist[0].Metric != "cpu_pct" {
		t.Errorf("expected metric=cpu_pct, got %q", hist[0].Metric)
	}
}

// ─── TestEvalAnomalyMetric_NodePath_MemPct ───────────────────────────────────

// mem_pct anomaly rule iterates snap.Nodes.
func TestEvalAnomalyMetric_NodePath_MemPct(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// mean=40, stddev=3; mem=90 → z=(90-40)/max(3,max(2,1e-9))=50/3≈16.7 > 2.0 → fires
	reader := &alert.FakeAnomalyBaselineReader{
		Row: nodeBaseline("mem_pct", "node-1", 40.0, 3.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "mem-anomaly", "mem_pct", 2.0, 5)

	live.setSnap(snapWithNode("node-1", 50.0, 90.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Fatal("expected firing history for mem_pct anomaly node rule")
	}
	if hist[0].Metric != "mem_pct" {
		t.Errorf("expected metric=mem_pct, got %q", hist[0].Metric)
	}
}

// ─── TestEvalAnomalyMetric_BaselineReaderError ───────────────────────────────

// Reader returns error → stream skipped, no panic, no history.
func TestEvalAnomalyMetric_BaselineReaderError(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: nil,
		Err: errors.New("transient store error"),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "reader-error", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx) // must not panic

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history when reader errors, got %d", len(hist))
	}
}

// ─── TestEvaluator_AnomalyRuleDispatch ───────────────────────────────────────

// RuleType="anomaly" dispatches to evalAnomalyMetric, NOT threshold path.
// Verified by notification firing on deviation from baseline.
func TestEvaluator_AnomalyRuleDispatch(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifs []map[string]any
	var mu sync.Mutex
	ev.SetNotifySink(func(payload []byte) {
		var n map[string]any
		_ = json.Unmarshal(payload, &n)
		mu.Lock()
		notifs = append(notifs, n)
		mu.Unlock()
	})

	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "dispatch-test", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	mu.Lock()
	n := len(notifs)
	mu.Unlock()
	if n == 0 {
		t.Fatal("expected notification from anomaly dispatch, got 0")
	}
	mu.Lock()
	notif := notifs[0]
	mu.Unlock()
	if notif["metric"] != "viewer_count" {
		t.Errorf("expected metric=viewer_count, got %v", notif["metric"])
	}
	if notif["state"] != "firing" {
		t.Errorf("expected state=firing, got %v", notif["state"])
	}
}

// ─── TestEvaluator_ThresholdRuleUnchanged ────────────────────────────────────

// RuleType="threshold" still evaluates via existing switch path.
func TestEvaluator_ThresholdRuleUnchanged(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	_, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "threshold-compat",
		Metric:             "viewer_count",
		RuleType:           "threshold",
		Operator:           "gt",
		Threshold:          5,
		WindowS:            0,
		Severity:           "warning",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live.setSnap(snapWithStream("s1", 10))
	clock.Advance(1 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("expected threshold rule to still fire with viewer_count=10 > threshold=5")
	}
}

// ─── TestEvaluator_RuleTypeEmpty_BehavesAsThreshold ──────────────────────────

// RuleType="" behaves as threshold (backward compat) — test-pinned.
func TestEvaluator_RuleTypeEmpty_BehavesAsThreshold(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	_, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "empty-ruletype-compat",
		Metric:             "viewer_count",
		RuleType:           "", // empty → behaves as threshold
		Operator:           "gt",
		Threshold:          5,
		WindowS:            0,
		Severity:           "warning",
		Enabled:            true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live.setSnap(snapWithStream("s1", 10))
	clock.Advance(1 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("expected empty RuleType rule to fire as threshold (backward compat)")
	}
}

// ─── TestEvaluator_SetAnomalyBaselineReader ──────────────────────────────────

// SetAnomalyBaselineReader stores the reader; evaluateRule sees it.
func TestEvaluator_SetAnomalyBaselineReader(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{Row: nil}
	ev.SetAnomalyBaselineReader(reader) // must compile and not panic
}

// ─── TestEvaluator_AnomalyRulePersistsHistory ────────────────────────────────

// End-to-end: anomaly rule with FakeAnomalyBaselineReader → TickOnce → history row.
func TestEvaluator_AnomalyRulePersistsHistory(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "persist-history", "viewer_count", 2.0, 5)

	live.setSnap(snapWithStream("s1", 10000))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("expected alert_history row, got 0")
	}
	if hist[0].Metric != "viewer_count" {
		t.Errorf("expected metric=viewer_count, got %q", hist[0].Metric)
	}
	if hist[0].State != "firing" {
		t.Errorf("expected state=firing, got %q", hist[0].State)
	}
}

// ─── ValidateAnomalyRule tests ────────────────────────────────────────────────

func TestValidateAnomalyRule_SupportedMetrics(t *testing.T) {
	for _, metric := range []string{"viewer_count", "ingest_bitrate_kbps", "cpu_pct", "mem_pct", "disk_pct"} {
		rule := meta.AlertRuleRow{RuleType: "anomaly", Metric: metric, WindowS: 3600}
		if err := alert.ValidateAnomalyRule(rule); err != nil {
			t.Errorf("metric %q: expected no error, got %v", metric, err)
		}
	}
}

func TestValidateAnomalyRule_UnsupportedMetric(t *testing.T) {
	// rebuffer_ratio is excluded (beacon QoE — not in LiveSnapshot until U3).
	rule := meta.AlertRuleRow{RuleType: "anomaly", Metric: "rebuffer_ratio", WindowS: 3600}
	err := alert.ValidateAnomalyRule(rule)
	if err == nil {
		t.Fatal("expected error for unsupported metric")
	}
	var valErr *alert.AnomalyRuleValidationError
	if !errors.As(err, &valErr) {
		t.Errorf("expected *alert.AnomalyRuleValidationError, got %T", err)
	}
	if valErr.Field != "metric" {
		t.Errorf("expected Field='metric', got %q", valErr.Field)
	}
}

func TestValidateAnomalyRule_WrongWindowS(t *testing.T) {
	rule := meta.AlertRuleRow{RuleType: "anomaly", Metric: "viewer_count", WindowS: 300}
	err := alert.ValidateAnomalyRule(rule)
	if err == nil {
		t.Fatal("expected error for window_s != 3600")
	}
	var valErr *alert.AnomalyRuleValidationError
	if !errors.As(err, &valErr) {
		t.Errorf("expected *alert.AnomalyRuleValidationError, got %T", err)
	}
	if valErr.Field != "window_s" {
		t.Errorf("expected Field='window_s', got %q", valErr.Field)
	}
}

func TestValidateAnomalyRule_ThresholdRulePassthrough(t *testing.T) {
	// Non-anomaly rules always pass, regardless of metric/window.
	cases := []meta.AlertRuleRow{
		{RuleType: "threshold", Metric: "viewer_count", WindowS: 300},
		{RuleType: "", Metric: "unknown_metric", WindowS: 0},
	}
	for _, r := range cases {
		if err := alert.ValidateAnomalyRule(r); err != nil {
			t.Errorf("RuleType=%q: expected no error, got %v", r.RuleType, err)
		}
	}
}
