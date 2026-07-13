package alert_test

// wave3_presence_flags_test.go — D-088 TDD: presence-flag guard in evalAnomalyNodes.
//
// Covers:
//   - evalAnomalyNodes skips a node for cpu_pct when CPUPCTReported=false
//     (standalone AMS 3.x path — normalize.go omits cpu/mem/disk keys).
//
// RED: current evalAnomalyNodes has no CPUPCTReported guard — it reads n.CPUPCT=0
// unconditionally and calls the reader with val=0. The test verifies reader is NOT
// called, but with unmodified code it IS called → behavioral FAIL.
//
// GREEN: after adding the guard (`if !n.CPUPCTReported { continue }` for cpu_pct),
// the reader is never called for this node.

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// snapWithUnreportedCPU returns a LiveSnapshot with one node that has CPUPCT=80
// but CPUPCTReported=false (standalone path: normalize.go did not emit cpu_pct key).
func snapWithUnreportedCPU(nodeID string, cpuPCT float64) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			nodeID: {
				NodeID:         nodeID,
				CPUPCT:         cpuPCT,
				CPUPCTReported: false, // key was absent → standalone node
			},
		},
	}
}

// snapWithReportedCPU returns a LiveSnapshot with one node that has
// CPUPCTReported=true (cluster path: cpu_pct key was present in the event).
func snapWithReportedCPU(nodeID string, cpuPCT float64) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			nodeID: {
				NodeID:         nodeID,
				CPUPCT:         cpuPCT,
				CPUPCTReported: true,
			},
		},
	}
}

// TestEvalAnomalyNodes_UnreportedCPU_NoEval verifies that evalAnomalyNodes does
// not call the baseline reader for a node with CPUPCTReported=false.
//
// Background: standalone AMS 3.x /rest/v2/system-status does not return cpu_pct.
// normalize.go honestly omits the key. The aggregator leaves CPUPCTReported=false.
// If evalAnomalyNodes reads CPUPCT=0 and compares to a real baseline (mean=50,
// stddev=5), it fires a false alarm (z=|0-50|/5=10 >> sigma). The guard prevents this.
//
// RED: current code has no CPUPCTReported check — reader IS called with val=0 →
// alert fires for the standalone node → test fails (history entries present).
func TestEvalAnomalyNodes_UnreportedCPU_NoEval(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// Provide a warmed-up baseline so minSamples check passes.
	calledWith := &[]string{}
	reader := &alert.FakeAnomalyBaselineReader{
		Row:        nodeBaseline("cpu_pct", "standalone-node", 50.0, 5.0, 30),
		CalledWith: calledWith,
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "cpu-unreported-guard", "cpu_pct", 2.0, 5)

	// Standalone node: CPUPCT has a value (80) but CPUPCTReported=false.
	// The guard must prevent evaluation — this node's cpu_pct was never measured.
	live.setSnap(snapWithUnreportedCPU("standalone-node", 80.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	reader.Mu.Lock()
	calls := *reader.CalledWith
	reader.Mu.Unlock()

	if len(calls) > 0 {
		t.Errorf("evalAnomalyNodes: baseline reader was called with metric=%v for a node "+
			"with CPUPCTReported=false — the presence guard must skip this node "+
			"(standalone AMS 3.x never reports cpu_pct)", calls)
	}

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("expected 0 alert history entries for unreported cpu_pct, got %d "+
			"(the node with CPUPCTReported=false must not trigger an alert)", len(hist))
	}
	t.Logf("PASS: evalAnomalyNodes skips cpu_pct for node with CPUPCTReported=false")
}

// TestEvalAnomalyNodes_ReportedCPU_Fires verifies that the presence guard does NOT
// block evaluation when CPUPCTReported=true. This is the positive control: a node
// that genuinely reports cpu_pct must still fire when anomalous.
func TestEvalAnomalyNodes_ReportedCPU_Fires(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		// mean=30, stddev=5; CPUPCT=80 → z=(80-30)/max(5,1.5,1e-9)=50/5=10 >> 2.0
		Row: nodeBaseline("cpu_pct", "cluster-node", 30.0, 5.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "cpu-reported-fires", "cpu_pct", 2.0, 5)

	// Cluster node: CPUPCTReported=true, anomalous value.
	live.setSnap(snapWithReportedCPU("cluster-node", 80.0))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Error("expected alert to fire for cluster node with CPUPCTReported=true and " +
			"anomalous cpu_pct, got 0 history entries (presence guard must allow reported nodes)")
	}
	t.Logf("PASS: evalAnomalyNodes fires for cpu_pct when CPUPCTReported=true")
}

// TestEvalAnomalyNodes_UnreportedMem_NoEval is the same guard for mem_pct.
func TestEvalAnomalyNodes_UnreportedMem_NoEval(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: nodeBaseline("mem_pct", "standalone-2", 60.0, 5.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "mem-unreported-guard", "mem_pct", 2.0, 5)

	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"standalone-2": {
				NodeID:         "standalone-2",
				MemPCT:         70.0,
				MemPCTReported: false,
			},
		},
	})
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history for MemPCTReported=false node, got %d", len(hist))
	}
	t.Logf("PASS: evalAnomalyNodes skips mem_pct for node with MemPCTReported=false")
}

// TestEvalAnomalyNodes_UnreportedDisk_NoEval is the same guard for disk_pct.
func TestEvalAnomalyNodes_UnreportedDisk_NoEval(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeAnomalyBaselineReader{
		Row: nodeBaseline("disk_pct", "standalone-3", 40.0, 5.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "disk-unreported-guard", "disk_pct", 2.0, 5)

	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"standalone-3": {
				NodeID:          "standalone-3",
				DiskPCT:         50.0,
				DiskPCTReported: false,
			},
		},
	})
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("expected 0 history for DiskPCTReported=false node, got %d", len(hist))
	}
	t.Logf("PASS: evalAnomalyNodes skips disk_pct for node with DiskPCTReported=false")
}

// makeAnomalyRuleCustom is a local helper for non-anomaly rule tests that need
// to avoid duplicate rule names when multiple sub-tests run in the same store.
func makeAnomalyRuleCustom(ctx context.Context, t *testing.T, store *meta.Store, name, metric string) meta.AlertRuleRow {
	t.Helper()
	row, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               name,
		Metric:             metric,
		RuleType:           "anomaly",
		WindowS:            3600,
		Sigma:              2.0,
		MinSamples:         5,
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
