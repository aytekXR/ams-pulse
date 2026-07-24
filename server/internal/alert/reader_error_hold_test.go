package alert_test

// reader_error_hold_test.go — Fix C regression: QoE reader errors must hold
// alert states, not resolve them.
//
// Before Fix C, when the QoE reader returned an error the affected stream was
// skipped via `continue`, producing no evalResult for it. processEvaluation
// therefore never saw the stream's entry this tick, so pruneStaleStates could
// evict it — and a firing alert that lost its state entry would never emit a
// resolved notification (permanently stuck or silently dropped). Worse, on a
// single-stream scan a reader error caused the evaluator to produce [] results,
// which meant every firing alert for that stream transitioned to resolved on the
// next tick after recovery (a false resolve during the outage window).
//
// Fix C changes the reader-error path to emit evalResult{hold: true} for each
// affected stream. processEvaluation updates lastCheck (preventing eviction) but
// skips all state transitions — the alert stays in exactly the state it was in
// before the reader error and cannot spuriously resolve.
//
// Test scenarios
//
//  1. Hold-during-outage: fire an alert, then inject a reader error; verify no
//     resolve notification is sent while the reader is erroring.
//  2. Recovery: after the reader recovers, verify normal evaluation resumes (the
//     alert resolves when the condition is no longer met).
//  3. State-count invariant: StateCount does not drop to 0 during a reader error
//     (proves the state entry is preserved, not evicted by pruneStaleStates).

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// readerHoldClock returns a fake clock for these tests.
func readerHoldClock() *alert.FakeClock {
	return alert.NewFakeClock(time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC))
}

// createRebufferRatioRule inserts a rebuffer_ratio rule and returns it.
// operator=gt, threshold=0.1 — fires when rebuffer_ratio > 0.1.
func createRebufferRatioRule(t *testing.T, store *meta.Store, windowS, cooldownS int) meta.AlertRuleRow {
	t.Helper()
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               "test-rebuffer-ratio",
		Metric:             "rebuffer_ratio",
		Operator:           "gt",
		Threshold:          0.1,
		WindowS:            windowS,
		ScopeJSON:          `{}`,
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

// snapWithStream builds a LiveSnapshot with a single active stream.
func snapWithSingleStream(streamID string) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			streamID: {StreamID: streamID, App: "live", Active: true},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

// ─── Test 1: hold-during-outage ──────────────────────────────────────────────

// TestReaderErrorHold_NoSpuriousResolve_FixC fires a rebuffer_ratio alert then
// injects a QoE reader error. Verifies that no resolved notification is sent
// while the reader is in error mode — the alert must be held, not resolved.
func TestReaderErrorHold_NoSpuriousResolve_FixC(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := readerHoldClock()

	createRebufferRatioRule(t, store, 10, 300) // window=10 s, long cooldown

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	// Wire a QoE reader that fires the rebuffer_ratio threshold.
	qoeReader := &alert.FakeQoEReader{RebufferRatio: 0.9, Err: nil}
	ev.SetQoEReader(qoeReader)

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

	live.setSnap(snapWithSingleStream("stream1"))

	// Advance until the alert fires (window=10 s, tick=5 s → fire at t=20 after 3 ticks).
	//
	//  t=5:  pendingSince = t5; elapsed=0 s < 10 s
	//  t=10: elapsed=5 s < 10 s
	//  t=15: elapsed=10 s >= 10 s → FIRE
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	if countState("firing") == 0 {
		t.Fatal("setup failed: expected firing notification before injecting reader error")
	}
	t.Logf("firing count before error injection: %d", countState("firing"))

	// Inject a reader error: the QoE reader now returns an error for every stream.
	qoeReader.Err = errors.New("clickhouse: connection refused")

	// Run several ticks with the reader erroring.
	// Fix C: the hold path must prevent any resolved notification.
	resolvedBefore := countState("resolved")
	for i := 0; i < 4; i++ {
		clock.Advance(5 * time.Second)
		ev.TickOnce(ctx)
	}

	resolvedAfter := countState("resolved")
	if resolvedAfter > resolvedBefore {
		t.Errorf("Fix C regression: got %d resolved notification(s) during reader error — spurious resolve (old 'continue' behaviour)", resolvedAfter-resolvedBefore)
	}
	t.Logf("Fix C: resolved count held at %d during reader error (no spurious resolve)", resolvedAfter)
}

// ─── Test 2: recovery ─────────────────────────────────────────────────────────

// TestReaderErrorHold_Recovery_FixC verifies that after a reader error the
// evaluator resumes normal evaluation on recovery. Specifically: when the reader
// recovers with a below-threshold value, the alert resolves.
func TestReaderErrorHold_Recovery_FixC(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := readerHoldClock()

	createRebufferRatioRule(t, store, 10, 0) // window=10 s, cooldownS=0 so re-fire allowed

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	qoeReader := &alert.FakeQoEReader{RebufferRatio: 0.9, Err: nil}
	ev.SetQoEReader(qoeReader)

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

	live.setSnap(snapWithSingleStream("stream1"))

	// Fire the alert.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=15: FIRE

	if countState("firing") == 0 {
		t.Fatal("setup: alert did not fire before injecting reader error")
	}

	// Inject error for 2 ticks.
	qoeReader.Err = errors.New("clickhouse timeout")
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=20: hold
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=25: hold

	// Recover the reader with a value below threshold (rebuffer=0.0 → not firing).
	qoeReader.Err = nil
	qoeReader.RebufferRatio = 0.0

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=30: condition not met → RESOLVE

	if countState("resolved") == 0 {
		t.Error("Fix C regression: no resolved notification after reader recovery and condition cleared")
	}
	t.Logf("Fix C recovery: firing=%d, resolved=%d", countState("firing"), countState("resolved"))
}

// ─── Test 3: StateCount invariant ────────────────────────────────────────────

// TestReaderErrorHold_StateNotEvicted_FixC proves that StateCount does not drop
// to 0 while the reader is in error mode. The old 'continue' path produced no
// evalResult, so pruneStaleStates could evict the entry (lastCheck not updated).
// Fix C stamps lastCheck via the hold path, preventing eviction.
func TestReaderErrorHold_StateNotEvicted_FixC(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := readerHoldClock()

	createRebufferRatioRule(t, store, 10, 0)

	ctx := context.Background()
	ev, _ := newTestEvaluator(t, store, live, clock)

	qoeReader := &alert.FakeQoEReader{RebufferRatio: 0.9, Err: nil}
	ev.SetQoEReader(qoeReader)

	live.setSnap(snapWithSingleStream("stream1"))

	// Prime the state machine so stream1 has an active state entry.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // t=5: pendingSince = t5

	stateAfterPrime := ev.StateCount()
	if stateAfterPrime == 0 {
		t.Fatal("setup: StateCount=0 after first tick — no state created for stream1")
	}

	// Inject reader error.
	qoeReader.Err = errors.New("reader error")

	// Multiple ticks: pruneStaleStates runs each tick. With the old code it would
	// evict the entry (lastCheck not updated by hold path → treated as stale).
	for i := 0; i < 3; i++ {
		clock.Advance(5 * time.Second)
		ev.TickOnce(ctx)
	}

	stateAfterError := ev.StateCount()
	if stateAfterError == 0 {
		t.Error("Fix C regression: StateCount=0 during reader error — state entry evicted (old behaviour: no lastCheck update)")
	}
	t.Logf("Fix C: StateCount before=%d, after=%d reader error ticks (entry preserved)", stateAfterPrime, stateAfterError)
}
