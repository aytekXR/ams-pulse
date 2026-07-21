// Package alert_test — S96 (D-160): bound Evaluator.states growth.
//
// e.states (the per-(rule,group) firing-state map) had NO delete site: every unique
// (rule, stream_id) that ever produced an evalResult left a permanent entry, an
// unbounded leak on high-stream-churn systems (found S95/D-159 #5). pruneStaleStates
// now evicts entries whose stream vanished (no evalResult this tick) once they are
// behaviorally INERT (resolved / idle-pending) and their cooldown has lapsed —
// provably identical to keeping them, since the next evalResult recreates a fresh
// entry. These tests prove the eviction happens AND does not change fire/resolve/
// re-fire behavior. (The whole existing s93/s95 + evaluator suite is the broader
// behavior-preservation proof.)
package alert_test

import (
	"context"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// TestStates_ResolvedEntryEvictedAfterCooldown_S96 is the mutation target: after a
// wildcard offline stream fires, resolves, and its cooldown lapses, its ruleState is
// evicted — StateCount returns to 0 instead of leaking one entry forever.
func TestStates_ResolvedEntryEvictedAfterCooldown_S96(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	createStreamOfflineRule(t, store, "", 0, 10) // wildcard, WindowS=0, cooldown=10s
	ctx := context.Background()

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	// t=0 present → creates one pending state for live1.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx)
	if got := ev.StateCount(); got != 1 {
		t.Fatalf("expected 1 state after live1 seen; got %d", got)
	}

	// t=5 offline → fire (WindowS=0); hold = 0+max(0,2*tick)=10 → resolve at t15;
	// cooldownUntil = firedAt(t5)+10 = t15.
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 1) // t=5 fire
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("expected fire at t5; got %d firing", got)
	}

	// t=10 (still firing), t=15 (resolve), t=20 (evict: stale + resolved + cooldown lapsed).
	tick(t, ctx, ev, clock, 3) // t=10,15,20
	if got := nc.countState("resolved"); got != 1 {
		t.Fatalf("expected resolve after the hold; got %d resolved", got)
	}
	if got := ev.StateCount(); got != 0 {
		t.Fatalf("a resolved entry whose stream vanished and whose cooldown lapsed must be "+
			"evicted; got %d states (unbounded-growth regression, D-160 #5)", got)
	}
}

// TestStates_EvictionPreservesRefire_S96 proves eviction is behavior-preserving: after
// an offline event fires, resolves, and is evicted, the SAME stream going offline again
// is a NEW event that still pages. (Passes with and without the sweep — its job is to
// guard against the eviction silently breaking re-fire.)
func TestStates_EvictionPreservesRefire_S96(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	createStreamOfflineRule(t, store, "", 0, 10) // WindowS=0, cooldown=10s
	ctx := context.Background()

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	// First offline event: fire at t5, resolve at t15, evict at t20.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx) // t=0 present
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 4) // t=5 fire, t=10, t=15 resolve, t=20 evict
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("expected 1 fire from the first offline event; got %d", got)
	}
	if got := ev.StateCount(); got != 0 {
		t.Fatalf("first event's state must be evicted before re-test; got %d", got)
	}

	// Stream returns, then goes offline again — a distinct event that must page again.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	tick(t, ctx, ev, clock, 1) // t=25 present (fresh pending entry)
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 1) // t=30 offline edge → fire again
	if got := nc.countState("firing"); got != 2 {
		t.Fatalf("a second, distinct offline event must page again after the first entry was "+
			"evicted; got %d firing (eviction broke re-fire)", got)
	}
}

// TestStates_ResolvedWithPendingProgressNotEvicted_S96 pins the D-160 review fix: a
// "resolved" entry can carry a NON-zero pendingSince — its condition re-met and is
// accumulating toward a re-fire that a still-active cooldown suppresses. If its node
// then vanishes and the cooldown lapses, that entry must NOT be evicted: dropping it
// would discard the accumulated window progress and delay/miss the re-fire on the
// node's return. Only entries with pendingSince==zero are behaviorally inert.
func TestStates_ResolvedWithPendingProgressNotEvicted_S96(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	ctx := context.Background()

	// node_cpu gt 90, WindowS=30s (must hold 30s to fire), cooldown=60s.
	rule := meta.AlertRuleRow{
		Name: "s96-cpu", Metric: "node_cpu", Operator: "gt", Threshold: 90,
		WindowS: 30, ScopeJSON: `{}`, Severity: "warning", CooldownS: 60,
		Enabled: true, Muted: false, MaintenanceWindows: "[]", ChannelIDs: `["test-channel"]`,
	}
	if _, err := store.CreateAlertRule(ctx, rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	hiCPU := &domain.LiveNodeStats{NodeID: "n1", CPUPCT: 95, CPUPCTReported: true} // >90 → met
	loCPU := &domain.LiveNodeStats{NodeID: "n1", CPUPCT: 50, CPUPCTReported: true} // <90 → not met

	// T=0..30: CPU high → window elapses → fires at T=30 (cooldownUntil = T30+60 = T90).
	live.setSnap(s67NodeSnap(hiCPU))
	ev.TickOnce(ctx)           // T=0 pendingSince=T0
	tick(t, ctx, ev, clock, 6) // T=5..30 → fire at T30
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("expected fire at T30; got %d firing", got)
	}
	// T=35: CPU drops → firing resolves, pendingSince reset to zero.
	live.setSnap(s67NodeSnap(loCPU))
	tick(t, ctx, ev, clock, 1) // T=35 resolve
	if got := nc.countState("resolved"); got != 1 {
		t.Fatalf("expected resolve at T35; got %d resolved", got)
	}
	// T=40: CPU high again → the RESOLVED entry accumulates pendingSince=T40 (window not
	// yet elapsed → no re-fire; cooldown until T90 would suppress it anyway).
	live.setSnap(s67NodeSnap(hiCPU))
	tick(t, ctx, ev, clock, 1) // T=40 (state=resolved, pendingSince=T40)

	// The node then VANISHES; ticks run past the cooldown expiry (T90).
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 12) // T=45..100 (past cooldownUntil=T90)

	if got := ev.StateCount(); got != 1 {
		t.Fatalf("a resolved entry with accumulated re-fire window progress (non-zero "+
			"pendingSince) must NOT be evicted; got %d states (D-160 review regression — "+
			"evicting it would delay/miss the re-fire on the node's return)", got)
	}
}
