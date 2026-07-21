// Package alert_test — S95 (D-159): wildcard stream_offline SUSPEND/RESUME + hold
// correctness. Follow-up to the D-157 edge-detection fix, surfaced by an adversarial
// re-verification of that never-independently-swept critical-alert code.
//
// Three defects, all where the D-157 tracker interacts with a rule edit mid-event:
//
//	#1 missed-fire  — an offline edge detected while ENABLED, then a brief disable /
//	                  maintenance window before WindowS elapses, then re-enable while
//	                  the stream stays offline: the old prune wiped offlineAt and the
//	                  fresh empty tracker could not re-detect an already-gone stream,
//	                  so the genuine offline event never paged.
//	#3/#4 stuck-fire — the same brief-disable AFTER the alert fired left the ruleState
//	                  stuck "firing" forever (the empty tracker produced no result to
//	                  resolve it).
//	#2 retro-hold   — the hold was recomputed per tick from rule.WindowS, so shrinking
//	                  WindowS mid-event retroactively expired an in-flight offline
//	                  event and swallowed its page.
//
// The fix preserves offlineAt/holdUntil across a SUSPEND (disable / maintenance) while
// resetting prevPresent (no spurious edges), and freezes the hold deadline at detection.
// These tests drive the REAL state machine; each pins one edge a mutation must not break.
package alert_test

import (
	"context"
	"testing"
)

// TestStreamOffline_Wildcard_OfflineSurvivesBriefDisable_Fires_S95 pins #1: a genuine
// offline event whose WindowS spans a brief disable must still page on resume.
func TestStreamOffline_Wildcard_OfflineSurvivesBriefDisable_Fires_S95(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	rule := createStreamOfflineRule(t, store, "", 10, 300) // wildcard, WindowS=10s
	ctx := context.Background()

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	// t=0: present while ENABLED → tracker records prevPresent={live1}.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx)

	// t=5: stream goes offline while the rule is ENABLED → edge detected
	// (offlineAt[live1]=t5, pendingSince=t5), but WindowS(10) not yet elapsed → no fire.
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 1) // t=5
	if got := nc.countState("firing"); got != 0 {
		t.Fatalf("must not fire before WindowS elapses; got %d firing", got)
	}

	// Briefly DISABLE (operator toggle or maintenance window) for one tick, stream
	// stays offline. The in-flight offlineAt must survive the suspend.
	rule.Enabled = false
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	tick(t, ctx, ev, clock, 1) // t=10 (rule skipped; tracker SUSPENDED, offlineAt preserved)

	// Re-ENABLE; the stream is STILL offline. The preserved offline event now satisfies
	// WindowS and must FIRE (before the fix it silently never paged).
	rule.Enabled = true
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("re-enable rule: %v", err)
	}
	tick(t, ctx, ev, clock, 2) // t=15,20
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("a genuine offline event that spans a brief disable must still page once "+
			"the window elapses; got %d firing (missed-fire regression, D-159 #1)", got)
	}
}

// TestStreamOffline_Wildcard_FiredThenBriefDisable_Resolves_S95 pins #3/#4: an alert
// that already fired and is briefly disabled must still AUTO-RESOLVE at its hold —
// not stick "firing" forever.
func TestStreamOffline_Wildcard_FiredThenBriefDisable_Resolves_S95(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	rule := createStreamOfflineRule(t, store, "", 0, 300) // wildcard, WindowS=0 → fires immediately
	ctx := context.Background()

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	// t=0 present, t=5 offline → fires immediately (WindowS=0). hold = t5 + max(0,2*tick) = t15.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx)
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 1) // t=5 → FIRE
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("WindowS=0 offline must fire immediately; got %d firing", got)
	}

	// Disable for one tick while FIRING and the stream stays offline.
	rule.Enabled = false
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	tick(t, ctx, ev, clock, 1) // t=10 (suspended; offlineAt/holdUntil preserved)

	// Re-enable; stream still offline. The preserved hold (t15) must let the fired
	// alert AUTO-RESOLVE rather than stick firing forever.
	rule.Enabled = true
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("re-enable rule: %v", err)
	}
	tick(t, ctx, ev, clock, 2) // t=15,20 → resolve at t15 (now reaches holdUntil)
	if got := nc.countState("resolved"); got != 1 {
		t.Fatalf("a fired wildcard offline alert that spans a brief disable must still "+
			"auto-resolve at its hold; got %d resolved (stuck-fire regression, D-159 #3/#4)", got)
	}
}

// TestStreamOffline_Wildcard_WindowDecreaseKeepsHold_S95 pins #2: shrinking WindowS
// mid-event must not retroactively expire an in-flight offline event. The hold is
// frozen at detection, so the event survives the edit and fires under the new window.
func TestStreamOffline_Wildcard_WindowDecreaseKeepsHold_S95(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	rule := createStreamOfflineRule(t, store, "", 60, 300) // WindowS=60 → hold frozen at 120s
	ctx := context.Background()

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	// t=0 present, t=5 offline → offlineAt[live1]=t5, holdUntil frozen at t5+120.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx)
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 2) // t=5,10 → offline, not yet fired (WindowS=60)
	if got := nc.countState("firing"); got != 0 {
		t.Fatalf("must not fire before WindowS=60; got %d firing", got)
	}

	// Operator DECREASES WindowS to 0 mid-event. A per-tick hold recompute would make
	// hold=streamOfflineHold(0,tick)=10s and, since now-offlineAt already exceeds 10s,
	// retroactively expire the event (emit 0.0 → pendingSince reset → page swallowed).
	// The frozen holdUntil prevents that; the event survives and now fires (WindowS=0).
	rule.WindowS = 0
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("shrink window: %v", err)
	}
	tick(t, ctx, ev, clock, 1) // t=15
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("shrinking WindowS mid-event must not retro-expire the in-flight offline "+
			"event; got %d firing (retro-hold-expiry regression, D-159 #2)", got)
	}
}
