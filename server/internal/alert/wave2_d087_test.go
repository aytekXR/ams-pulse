package alert_test

// wave2_d087_test.go — D-087 AMS early-warning ladder rung 2 tests.
// Tests for node_degraded ConsecAPIErrors>=3 extension and node_down regression pin.
// Written RED-first: ConsecAPIErrors tests fail until wave2.go is updated.

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// makeNodeDegradedRule creates a node_degraded alert rule.
func makeNodeDegradedRule(ctx context.Context, t *testing.T, store *meta.Store, scopeNodeID string) meta.AlertRuleRow {
	t.Helper()
	scopeJSON := `{}`
	if scopeNodeID != "" {
		scopeJSON = `{"node_id":"` + scopeNodeID + `"}`
	}
	row, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "node-degraded-test",
		Metric:             "node_degraded",
		Operator:           "gt",
		Threshold:          0,
		WindowS:            5,
		ScopeJSON:          scopeJSON,
		Severity:           "warning",
		CooldownS:          60,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	})
	if err != nil {
		t.Fatalf("makeNodeDegradedRule: %v", err)
	}
	return row
}

// snapWithNodeConsecErrors returns a LiveSnapshot with one node whose ConsecAPIErrors is set.
// CPU and mem are explicitly 0 to model a standalone AMS node (they never report OS metrics).
func snapWithNodeConsecErrors(nodeID string, consecErrors int) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			nodeID: {
				NodeID:          nodeID,
				CPUPCT:          0, // standalone AMS — OS metrics never reported
				MemPCT:          0,
				ConsecAPIErrors: consecErrors,
			},
		},
	}
}

// ─── TestNodeDegraded_ConsecAPIErrors_Three_Fires ─────────────────────────────

// A standalone node (cpu=mem=0) with ConsecAPIErrors=3 must fire node_degraded.
// This is rung-2 of the early-warning ladder.
// RED before wave2.go update: n.CPUPCT>90||n.MemPCT>90 is always false for these nodes.
func TestNodeDegraded_ConsecAPIErrors_Three_Fires(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	makeNodeDegradedRule(ctx, t, store, "")

	// ConsecAPIErrors=3, cpu=mem=0 (standalone AMS never reports OS metrics).
	live.setSnap(snapWithNodeConsecErrors("node-1", 3))

	// Advance past window_s=5s.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Error("node_degraded should fire when ConsecAPIErrors=3 (rung-2 of early-warning ladder)")
	} else if hist[0].State != "firing" {
		t.Errorf("expected state=firing, got %q", hist[0].State)
	} else {
		t.Logf("PASS: node_degraded fired with ConsecAPIErrors=3, cpu=0, mem=0")
	}
}

// ─── TestNodeDegraded_ConsecAPIErrors_Two_NoFire ──────────────────────────────

// ConsecAPIErrors=2 must NOT fire (threshold is >=3, not >=2).
func TestNodeDegraded_ConsecAPIErrors_Two_NoFire(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	makeNodeDegradedRule(ctx, t, store, "")

	live.setSnap(snapWithNodeConsecErrors("node-1", 2))

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("node_degraded must NOT fire when ConsecAPIErrors=2 (threshold is >=3), got %d history entries", len(hist))
	} else {
		t.Logf("PASS: node_degraded correctly does not fire with ConsecAPIErrors=2")
	}
}

// ─── TestNodeDegraded_ConsecAPIErrors_Zero_NoFire ────────────────────────────

// ConsecAPIErrors=0 (recovered) must NOT fire.
func TestNodeDegraded_ConsecAPIErrors_Zero_NoFire(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	makeNodeDegradedRule(ctx, t, store, "")

	live.setSnap(snapWithNodeConsecErrors("node-1", 0))

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) != 0 {
		t.Errorf("node_degraded must NOT fire when ConsecAPIErrors=0 (recovered), got %d history entries", len(hist))
	} else {
		t.Logf("PASS: node_degraded correctly does not fire with ConsecAPIErrors=0")
	}
}

// ─── TestNodeDegraded_HighCPU_StillFires ─────────────────────────────────────

// The original CPU>90 path must still fire (behavioral pin — keep old logic unchanged).
func TestNodeDegraded_HighCPU_StillFires(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	makeNodeDegradedRule(ctx, t, store, "")

	// CPU=95>90 → fires via existing path.
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"node-1": {NodeID: "node-1", CPUPCT: 95.0, MemPCT: 0, ConsecAPIErrors: 0},
		},
	})

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("node_degraded must still fire when CPUPCT=95>90 (behavioral pin)")
	} else {
		t.Logf("PASS: node_degraded still fires via CPUPCT>90 path")
	}
}

// ─── TestNodeDegraded_HighMem_StillFires ─────────────────────────────────────

// The original Mem>90 path must still fire (behavioral pin).
func TestNodeDegraded_HighMem_StillFires(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	makeNodeDegradedRule(ctx, t, store, "")

	// Mem=95>90 → fires via existing path.
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"node-1": {NodeID: "node-1", CPUPCT: 0, MemPCT: 95.0, ConsecAPIErrors: 0},
		},
	})

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("node_degraded must still fire when MemPCT=95>90 (behavioral pin)")
	} else {
		t.Logf("PASS: node_degraded still fires via MemPCT>90 path")
	}
}

// ─── TestNodeDown_AbsentNode_StillFires ──────────────────────────────────────

// Regression pin: node_down must still fire when the node is absent from the snapshot.
// This test is a duplicate of VD-30 but placed here explicitly as a D-087 regression guard.
func TestNodeDown_AbsentNode_StillFires_D087(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	ctx := context.Background()
	_, err := store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name:               "node-down-regression",
		Metric:             "node_down",
		Operator:           "gt",
		Threshold:          0,
		WindowS:            5,
		ScopeJSON:          `{"node_id":"node-missing"}`,
		Severity:           "critical",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// Snapshot: node-missing is absent.
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{}, // node-missing not present
	})

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	hist, _ := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if len(hist) == 0 {
		t.Error("D-087 regression: node_down must still fire when node is absent from snapshot")
	} else {
		t.Logf("PASS D-087 regression: node_down fired when node absent (group_key=%v)", hist[0].GroupKey)
	}
}
