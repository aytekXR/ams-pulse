// aggregator_node_streak_test.go — D-087 TDD: aggregator failure-path handling.
//
// ORCH design ruling (frozen contract):
//
//	Normal-path event (no api_unreachable): full replace + LastSeenAt=now.
//	Failure-path event (api_unreachable=true): IN-PLACE ConsecAPIErrors update
//	on EXISTING entry ONLY — create nothing, touch no other field.
//
// Pins (all RED before implementation):
//
//	(a) During a failure streak the snapshot's ConsecAPIErrors rises while
//	    LastSeenAt stays FROZEN at the last-success timestamp.
//	(b) With failure events continuing, EvictStaleNodes(threshold) still evicts
//	    the node (rung 3 unbroken by rung 2).
//	(c) A failure event for an UNKNOWN node creates nothing.
package aggregator

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── (a) LastSeenAt frozen during failure streak ─────────────────────────────

// TestAggregator_FailureStreak_LastSeenAtFrozen verifies that failure-streak events
// (api_unreachable=true) do NOT update the node's LastSeenAt timestamp.
// This is load-bearing: if LastSeenAt is refreshed, rung 3 (EvictStaleNodes) can
// never fire and node_down is permanently suppressed (BUG-011 root cause variant).
func TestAggregator_FailureStreak_LastSeenAtFrozen(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Establish the node with a successful event.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "frozen-node",
		Data: map[string]any{
			"cpu_pct":           10.0,
			"consec_api_errors": 0.0,
			"api_latency_ms":    5.0,
		},
	})

	snap1 := agg.CurrentSnapshot()
	node1, ok := snap1.Nodes["frozen-node"]
	if !ok {
		t.Fatal("frozen-node not in snapshot after initial successful event")
	}
	initialLastSeen := node1.LastSeenAt
	if initialLastSeen.IsZero() {
		t.Fatal("LastSeenAt is zero after initial event — aggregator did not set it")
	}
	t.Logf("initial LastSeenAt = %v", initialLastSeen)

	// Sleep to make the failure event's wall-clock meaningfully later.
	time.Sleep(10 * time.Millisecond)

	// Send a failure-streak event.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "frozen-node",
		Data: map[string]any{
			"api_unreachable":   true,
			"consec_api_errors": 1.0,
		},
	})

	snap2 := agg.CurrentSnapshot()
	node2, ok := snap2.Nodes["frozen-node"]
	if !ok {
		t.Fatal("frozen-node disappeared from snapshot after failure event — failure event must NOT delete the node")
	}

	// (a) KEY ASSERTION: LastSeenAt must NOT have changed.
	if !node2.LastSeenAt.Equal(initialLastSeen) {
		t.Errorf("FAIL: LastSeenAt was updated by failure event — rung 3 (node eviction) will never fire\n"+
			"  initial: %v\n  after failure: %v\n  delta: %v",
			initialLastSeen, node2.LastSeenAt, node2.LastSeenAt.Sub(initialLastSeen))
	} else {
		t.Logf("PASS (a): LastSeenAt frozen at %v (not updated by failure event)", initialLastSeen)
	}

	// ConsecAPIErrors must have been updated to 1.
	if node2.ConsecAPIErrors != 1 {
		t.Errorf("ConsecAPIErrors = %d, want 1 after first failure event", node2.ConsecAPIErrors)
	} else {
		t.Logf("PASS: ConsecAPIErrors = 1 after first failure event")
	}
}

// ─── (b) EvictStaleNodes fires despite failure-streak events ─────────────────

// TestAggregator_FailureStreak_EvictStillWorks verifies that with failure events
// continuing to flow (and LastSeenAt frozen), EvictStaleNodes(threshold) still evicts
// the node when the threshold is exceeded. This is rung 3 of the early-warning ladder.
//
// Before the fix: failure events refresh LastSeenAt (existing onNodeStats code), so
// the node is never stale. After the fix: failure events leave LastSeenAt unchanged.
func TestAggregator_FailureStreak_EvictStillWorks(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Establish node with a successful event.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "doomed-node",
		Data: map[string]any{
			"consec_api_errors": 0.0,
			"api_latency_ms":    3.5,
		},
	})

	if _, ok := agg.CurrentSnapshot().Nodes["doomed-node"]; !ok {
		t.Fatal("doomed-node not in snapshot after initial event")
	}

	// Sleep so the node's LastSeenAt becomes stale relative to a short threshold.
	time.Sleep(10 * time.Millisecond)

	// Simulate an ongoing failure streak — these events must NOT refresh LastSeenAt.
	for i := 1; i <= 3; i++ {
		agg.OnServerEvent(domain.ServerEvent{
			Version: 1,
			Type:    domain.EventNodeStats,
			TS:      time.Now().UnixMilli(),
			Source:  domain.SourceRestPoll,
			NodeID:  "doomed-node",
			Data: map[string]any{
				"api_unreachable":   true,
				"consec_api_errors": float64(i),
			},
		})
	}

	// Verify ConsecAPIErrors was updated (in-place).
	snap := agg.CurrentSnapshot()
	if n, ok := snap.Nodes["doomed-node"]; ok {
		if n.ConsecAPIErrors != 3 {
			t.Errorf("ConsecAPIErrors after 3 failure events = %d, want 3", n.ConsecAPIErrors)
		}
	}

	// EvictStaleNodes with threshold=5ms — well within the elapsed time (≥10ms).
	agg.EvictStaleNodes(5 * time.Millisecond)

	// (b) KEY ASSERTION: node must be gone.
	if _, ok := agg.CurrentSnapshot().Nodes["doomed-node"]; ok {
		t.Error("FAIL (b): doomed-node still in snapshot after EvictStaleNodes — " +
			"failure events must NOT refresh LastSeenAt (rung 3 broken)")
	} else {
		t.Log("PASS (b): doomed-node evicted despite ongoing failure-streak events (rung 3 intact)")
	}
}

// ─── (c) Failure event for unknown node creates nothing ──────────────────────

// TestAggregator_FailureStreak_UnknownNodeCreatesNothing verifies that a failure-streak
// event for a node that was never seen before does NOT create a new entry.
// This prevents phantom node entries from API-failure events for nodes we've never
// successfully seen (e.g. from a wrong NodeID config or a split-brain scenario).
func TestAggregator_FailureStreak_UnknownNodeCreatesNothing(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Send failure event for a node that was never seen.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "ghost-node",
		Data: map[string]any{
			"api_unreachable":   true,
			"consec_api_errors": 1.0,
		},
	})

	snap := agg.CurrentSnapshot()
	if _, ok := snap.Nodes["ghost-node"]; ok {
		t.Error("FAIL (c): ghost-node appeared in snapshot after failure event for unknown node — " +
			"failure event must NOT create new entries")
	} else {
		t.Log("PASS (c): ghost-node correctly not created by failure event for unknown node")
	}
}

// ─── (d) Normal-path events still extract new fields ─────────────────────────

// TestAggregator_NormalPath_ExtractsNewFields verifies that normal-path (successful)
// node_stats events populate APILatencyMS and ConsecAPIErrors on the LiveNodeStats struct.
func TestAggregator_NormalPath_ExtractsNewFields(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "healthy-node",
		Data: map[string]any{
			"cpu_pct":           25.0,
			"mem_pct":           50.0,
			"api_latency_ms":    12.5,
			"consec_api_errors": 0.0,
		},
	})

	snap := agg.CurrentSnapshot()
	node, ok := snap.Nodes["healthy-node"]
	if !ok {
		t.Fatal("healthy-node not in snapshot")
	}

	if node.APILatencyMS != 12.5 {
		t.Errorf("APILatencyMS = %v, want 12.5", node.APILatencyMS)
	}
	if node.ConsecAPIErrors != 0 {
		t.Errorf("ConsecAPIErrors = %d, want 0", node.ConsecAPIErrors)
	}
	t.Logf("PASS (d): APILatencyMS=%.1f ConsecAPIErrors=%d", node.APILatencyMS, node.ConsecAPIErrors)
}

// TestAggregator_ConsecAPIErrors_PropagatesViaFailurePath verifies that repeated
// failure events in-place update ConsecAPIErrors to 1, 2, 3 without changing
// any other field on the existing LiveNodeStats entry.
func TestAggregator_ConsecAPIErrors_PropagatesViaFailurePath(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Establish node with known values.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "tracked-node",
		Data: map[string]any{
			"cpu_pct":           55.0,
			"mem_pct":           70.0,
			"api_latency_ms":    8.0,
			"consec_api_errors": 0.0,
		},
	})

	snap0 := agg.CurrentSnapshot()
	n0, ok := snap0.Nodes["tracked-node"]
	if !ok {
		t.Fatal("tracked-node not in snapshot after initial event")
	}
	// Verify initial state.
	if n0.CPUPCT != 55.0 {
		t.Errorf("initial CPUPCT = %v, want 55.0", n0.CPUPCT)
	}

	// Send 3 failure events.
	for i := 1; i <= 3; i++ {
		agg.OnServerEvent(domain.ServerEvent{
			Version: 1,
			Type:    domain.EventNodeStats,
			TS:      time.Now().UnixMilli(),
			Source:  domain.SourceRestPoll,
			NodeID:  "tracked-node",
			Data: map[string]any{
				"api_unreachable":   true,
				"consec_api_errors": float64(i),
			},
		})

		snap := agg.CurrentSnapshot()
		n, ok := snap.Nodes["tracked-node"]
		if !ok {
			t.Fatalf("tracked-node disappeared after failure event #%d", i)
		}

		// ConsecAPIErrors must match the event value.
		if n.ConsecAPIErrors != i {
			t.Errorf("after failure #%d: ConsecAPIErrors = %d, want %d", i, n.ConsecAPIErrors, i)
		}

		// CPUPCT must NOT be zeroed (failure event must not replace the struct).
		if n.CPUPCT != 55.0 {
			t.Errorf("after failure #%d: CPUPCT = %v, want 55.0 (failure event must not replace struct)", i, n.CPUPCT)
		}
		t.Logf("PASS: after failure #%d — ConsecAPIErrors=%d, CPUPCT=%.1f (preserved)", i, n.ConsecAPIErrors, n.CPUPCT)
	}
}
