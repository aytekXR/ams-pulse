package alert_test

// wildcard_node_down_test.go — Fix B regression: wildcard node_down rule fires
// and resolves correctly when a node is evicted from the live snapshot.
//
// Before Fix B, a wildcard node_down rule (scope.NodeID == "") produced zero
// evalResults on every tick — identical to the original comment "// TODO: wildcard
// node_down — no results" — so it could never fire even when a node disappeared.
// Fix B adds a nodeDownTracker that diffs snap.Nodes across ticks, mirrors the
// offlineTracker pattern used for stream_offline.
//
// Test scenario:
//
//	t=0   (baseline): both nodeA and nodeB present — tracker initialised, no fire
//	t=5   (tick 1):   both present — 0.0, pendingSince not set
//	t=10  (tick 2):   nodeB evicted — tracker detects edge; nodeB emits 1.0, pendingSince=t10
//	t=15  (tick 3):   nodeB still absent — now-pendingSince=5 s < 10 s window → no fire
//	t=20  (tick 4):   nodeB still absent — now-pendingSince=10 s == windowS → FIRE
//	t=25  (tick 5):   nodeB returns to snap — emits 0.0 → RESOLVE

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// nodeDownClock returns a fake clock anchored at an epoch convenient for this test.
func nodeDownClock() *alert.FakeClock {
	return alert.NewFakeClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
}

// snapWithNodes builds a LiveSnapshot containing exactly the given node IDs.
func snapWithNodes(nodeIDs ...string) *domain.LiveSnapshot {
	nodes := make(map[string]*domain.LiveNodeStats, len(nodeIDs))
	for _, nid := range nodeIDs {
		nodes[nid] = &domain.LiveNodeStats{NodeID: nid}
	}
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   nodes,
	}
}

// createWildcardNodeDownRule inserts a wildcard node_down rule and returns it.
// operator=eq, threshold=1 (fires when value==1.0, i.e. node is down).
func createWildcardNodeDownRule(t *testing.T, store *meta.Store, windowS, cooldownS int) meta.AlertRuleRow {
	t.Helper()
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               "test-wildcard-node-down",
		Metric:             "node_down",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            windowS,
		ScopeJSON:          `{}`, // wildcard: no node_id constraint
		Severity:           "critical",
		CooldownS:          cooldownS,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	created, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	return created
}

// ─── Core regression ─────────────────────────────────────────────────────────

// TestWildcardNodeDown_FiresWhenNodeEvicted_FixB is the primary Fix B regression:
// a wildcard node_down rule fires when a node disappears from snap.Nodes for
// long enough to satisfy WindowS.
func TestWildcardNodeDown_FiresWhenNodeEvicted_FixB(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := nodeDownClock()

	createWildcardNodeDownRule(t, store, 10, 60) // window=10 s, cooldown=60 s

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	var mu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		mu.Lock()
		notifs = append(notifs, n)
		mu.Unlock()
	})

	countState := func(state string) int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, m := range notifs {
			if m["state"] == state {
				n++
			}
		}
		return n
	}

	// t=0: baseline — both nodes present; tick to prime the tracker's prevPresent.
	live.setSnap(snapWithNodes("nodeA", "nodeB"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=5: both nodes → 0.0 for both; no pendingSince

	// t=5 → t=10: evict nodeB from snapshot.
	live.setSnap(snapWithNodes("nodeA")) // nodeB gone
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=10: nodeB detected absent → emits 1.0; pendingSince=t10; no fire yet (0 s < 10 s)

	if n := countState("firing"); n != 0 {
		t.Fatalf("tick t=10: expected 0 firing notifications, got %d (fired too early)", n)
	}

	// t=10 → t=15: nodeB still absent (window accumulating).
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=15: 15-10=5 s < 10 s → no fire

	if n := countState("firing"); n != 0 {
		t.Fatalf("tick t=15: expected 0 firing notifications, got %d (fired too early)", n)
	}

	// t=15 → t=20: window elapsed.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=20: 20-10=10 s >= 10 s → FIRE for nodeB

	if n := countState("firing"); n == 0 {
		t.Fatal("tick t=20: expected at least 1 firing notification after window elapsed, got 0 — Fix B regression")
	}

	// Confirm the notification targets nodeB.
	mu.Lock()
	var nodeDownNotif map[string]any
	for _, m := range notifs {
		if m["state"] == "firing" {
			nodeDownNotif = m
			break
		}
	}
	mu.Unlock()
	if nodeDownNotif == nil {
		t.Fatal("no firing notification found")
	}
	if nodeDownNotif["metric"] != "node_down" {
		t.Errorf("expected metric=node_down, got %v", nodeDownNotif["metric"])
	}
	if gk := nodeDownNotif["group_key"]; gk != "nodeB" {
		t.Errorf("expected group_key=nodeB, got %v", gk)
	}
	t.Logf("Fix B: wildcard node_down fired for group_key=%v at t=20", nodeDownNotif["group_key"])
}

// TestWildcardNodeDown_ResolvesWhenNodeReturns_FixB proves that once a wildcard
// node_down fires, it resolves when the evicted node reappears in snap.Nodes.
func TestWildcardNodeDown_ResolvesWhenNodeReturns_FixB(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := nodeDownClock()

	createWildcardNodeDownRule(t, store, 10, 300) // long cooldown so only one fire

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	var mu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		mu.Lock()
		notifs = append(notifs, n)
		mu.Unlock()
	})

	countState := func(state string) int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, m := range notifs {
			if m["state"] == state {
				n++
			}
		}
		return n
	}

	// Prime the tracker.
	live.setSnap(snapWithNodes("nodeA", "nodeB"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=5: both present

	// Evict nodeB.
	live.setSnap(snapWithNodes("nodeA"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=10: nodeB down detected
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=15: 5 s < 10 s
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=20: 10 s >= 10 s → FIRE

	if countState("firing") == 0 {
		t.Fatal("expected firing notification before testing resolve — Fix B regression in fire path")
	}
	firingCount := countState("firing")

	// Return nodeB to snapshot → evalWildcardNodeDown emits 0.0 and deletes from downSince.
	live.setSnap(snapWithNodes("nodeA", "nodeB"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=25: nodeB present → 0.0 → RESOLVE

	if countState("resolved") == 0 {
		t.Fatal("expected resolved notification when nodeB returned — Fix B resolve path broken")
	}
	t.Logf("Fix B: fired=%d, resolved=%d (node_down alert resolved on node return)", firingCount, countState("resolved"))
}

// TestWildcardNodeDown_NilScopeFiresForAnyNode_FixB verifies that a rule with no
// scope (ScopeJSON="{}") applies to all nodes, not just a named one.
func TestWildcardNodeDown_NilScopeFiresForAnyNode_FixB(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := nodeDownClock()

	createWildcardNodeDownRule(t, store, 0, 60) // windowS=0: fire immediately on first miss

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	var mu sync.Mutex
	firedGroupKeys := make(map[string]bool)
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		mu.Lock()
		if n["state"] == "firing" {
			if gk, ok := n["group_key"].(string); ok {
				firedGroupKeys[gk] = true
			}
		}
		mu.Unlock()
	})

	// Prime tracker with three nodes.
	live.setSnap(snapWithNodes("n1", "n2", "n3"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	// Evict n2 and n3.
	live.setSnap(snapWithNodes("n1"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // windowS=0 → fires immediately for n2 and n3

	mu.Lock()
	defer mu.Unlock()
	if !firedGroupKeys["n2"] {
		t.Errorf("expected firing for n2, got keys=%v", firedGroupKeys)
	}
	if !firedGroupKeys["n3"] {
		t.Errorf("expected firing for n3, got keys=%v", firedGroupKeys)
	}
	if firedGroupKeys["n1"] {
		t.Errorf("n1 should NOT fire (still present), got keys=%v", firedGroupKeys)
	}
	t.Logf("Fix B: wildcard rule fired for all evicted nodes: %v", firedGroupKeys)
}

// TestWildcardNodeDown_PreviouslyInert_FixB is the direct "before/after" regression:
// in a single tick with both nodes present followed by a tick where one is absent,
// the evaluator must produce AT LEAST ONE result for the missing node (the old code
// returned [] — permanently inert). This test verifies that the state machine
// actually has an entry for nodeB to advance.
func TestWildcardNodeDown_PreviouslyInert_FixB(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := nodeDownClock()

	createWildcardNodeDownRule(t, store, 0, 60) // fire immediately

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	// Prime tracker.
	live.setSnap(snapWithNodes("nodeA", "nodeB"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	// Evict nodeB: in old code, evalNodeUpDown returned [] so states map never grew.
	live.setSnap(snapWithNodes("nodeA"))
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	// With Fix B, at least one state entry for nodeB must exist.
	count := ev.StateCount()
	if count == 0 {
		t.Fatal("Fix B regression: StateCount=0 after node eviction — evalWildcardNodeDown returned no results (old inert behavior)")
	}
	t.Logf("Fix B: StateCount=%d after node eviction (expected ≥1)", count)
}
