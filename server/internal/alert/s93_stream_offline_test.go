// Package alert_test — S93 (D-157): wildcard stream_offline fire+resolve coverage.
//
// D-156 found that a WILDCARD stream_offline rule (including the default critical
// "Stream offline (default)" rule, ScopeJSON "{}") never fired: the old evaluator
// looked for a snap.Streams entry with Active==false, but the aggregator removes an
// ended stream from the snapshot BEFORE marking it inactive (onPublishEnd), so that
// state is unreachable. The former s67 wildcard test masked this by injecting the
// impossible Active:false snapshot entry directly.
//
// D-157 makes wildcard offline a present→gone EDGE detected across ticks. These
// tests drive the REAL state machine (and, in the capstone, the REAL aggregator) to
// prove BOTH the fire and the resolve — the two edges a mutation must not break.
package alert_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/collector/aggregator"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// s93Clock returns a fake clock anchored at a fixed instant (tests advance it).
func s93Clock() *alert.FakeClock {
	return alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

// offlineNotifCapture collects notification payloads and counts them by state.
type offlineNotifCapture struct {
	mu     sync.Mutex
	notifs []map[string]any
}

func (c *offlineNotifCapture) sink(payload []byte) {
	var n map[string]any
	_ = json.Unmarshal(payload, &n)
	c.mu.Lock()
	c.notifs = append(c.notifs, n)
	c.mu.Unlock()
}

func (c *offlineNotifCapture) countState(state string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, m := range c.notifs {
		if m["state"] == state {
			n++
		}
	}
	return n
}

func streamPresentSnap(streamID, app, nodeID string) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			streamID: {StreamID: streamID, App: app, NodeID: nodeID, Active: true},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func emptySnap() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	}
}

// TestStreamOffline_Wildcard_FireThenResolve_S93 is the core D-157 regression: a
// wildcard rule fires when a previously-present stream goes offline (present→gone
// edge held long enough to satisfy WindowS), then AUTO-RESOLVES after the hold
// window (proving it does not stick firing forever — the framework has no stale
// sweep, so the evaluator must emit a resolving 0.0 itself).
func TestStreamOffline_Wildcard_FireThenResolve_S93(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)

	// Wildcard (streamID="") stream_offline rule: window 10s, cooldown 300s.
	createStreamOfflineRule(t, store, "", 10, 300)

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)
	ctx := context.Background()

	// t=0: stream present → establishes it in the tracker; must NOT fire.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx)
	if got := nc.countState("firing"); got != 0 {
		t.Fatalf("no fire expected while the stream is present; got %d firing", got)
	}

	// Stream goes offline (removed from the snapshot — as the aggregator does).
	live.setSnap(emptySnap())

	// hold = window(10) + max(10, 2*tick=10) = 20s from the first offline tick (t=5).
	// Fire at pendingSince(t5)+window(10) = t15.
	tick(t, ctx, ev, clock, 3) // t=5,10,15
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("wildcard stream_offline must fire once after WindowS offline; got %d firing", got)
	}
	if got := nc.countState("resolved"); got != 0 {
		t.Fatalf("must not resolve before the hold window elapses; got %d resolved", got)
	}

	// Resolve at offlineAt(t5)+hold(20) = t25.
	tick(t, ctx, ev, clock, 2) // t=20,25
	if got := nc.countState("resolved"); got != 1 {
		t.Fatalf("wildcard stream_offline must AUTO-RESOLVE after the hold window; got %d resolved "+
			"(regression: a fired critical alert would stick firing forever)", got)
	}
	// And it must not re-fire or double-resolve on further ticks.
	tick(t, ctx, ev, clock, 3) // t=30,35,40
	if got := nc.countState("firing"); got != 1 {
		t.Errorf("no re-fire expected (stream stays gone); got %d firing", got)
	}
	if got := nc.countState("resolved"); got != 1 {
		t.Errorf("exactly one resolve expected; got %d resolved", got)
	}
}

// TestStreamOffline_Wildcard_NoFalseFireOnFirstTick_S93: a stream that was never
// observed present (absent from the very first tick) is NOT "offline" — the edge
// detector has no prior-presence record, so it must not fire (no false alarm on
// startup or for streams that never existed).
func TestStreamOffline_Wildcard_NoFalseFireOnFirstTick_S93(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	createStreamOfflineRule(t, store, "", 10, 300)

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)
	ctx := context.Background()

	// Snapshot is empty from the start; nothing was ever present.
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 6) // t=5..30, well past window+hold
	if got := nc.countState("firing"); got != 0 {
		t.Fatalf("a never-present stream must not produce an offline alert; got %d firing", got)
	}
}

// TestStreamOffline_Wildcard_RecoveryResolves_S93: a stream that goes offline
// (fires) and then RETURNS before the hold window elapses resolves via recovery
// (the returning stream emits 0.0), not only via the hold timeout.
func TestStreamOffline_Wildcard_RecoveryResolves_S93(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	createStreamOfflineRule(t, store, "", 10, 300)

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)
	ctx := context.Background()

	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx) // t=0 present
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 3) // t=5,10,15 → fire at t15
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("expected 1 firing after offline; got %d", got)
	}

	// Stream returns at t=20 (before hold=20 elapses from t5 → t25).
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	tick(t, ctx, ev, clock, 1) // t=20
	if got := nc.countState("resolved"); got != 1 {
		t.Fatalf("a returning stream must resolve its offline alert; got %d resolved", got)
	}
}

// TestStreamOffline_Wildcard_RealAggregatorFlow_S93 is the capstone: it drives the
// REAL aggregator (publish_start → publish_end) instead of a hand-built snapshot,
// proving the fix works against the exact snapshot the aggregator produces in
// production — the flow the masked s67 test never exercised. The sanity assertions
// also document the root cause: after publish_end the ended stream is GONE from the
// snapshot (not present-but-inactive).
func TestStreamOffline_Wildcard_RealAggregatorFlow_S93(t *testing.T) {
	agg := aggregator.New(3*time.Minute, nil, nil)

	// Stream goes live.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishStart, TS: 1,
		Source: domain.SourceRestPoll, NodeID: "n1", App: "live", StreamID: "live1",
		Data: map[string]any{"publish_type": "rtmp"},
	})
	if _, ok := agg.CurrentSnapshot().Streams["live1"]; !ok {
		t.Fatal("sanity: publish_start should make live1 present in the snapshot")
	}

	store := openTestStore(t)
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, agg, clock) // aggregator IS the live provider
	createStreamOfflineRule(t, store, "", 10, 300)

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)
	ctx := context.Background()

	ev.TickOnce(ctx) // t=0: live1 present via the real aggregator → no fire
	if got := nc.countState("firing"); got != 0 {
		t.Fatalf("no fire while live1 is publishing; got %d firing", got)
	}

	// Stream ends — the aggregator removes it from the snapshot entirely.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishEnd, TS: 2,
		Source: domain.SourceRestPoll, NodeID: "n1", App: "live", StreamID: "live1",
	})
	if _, ok := agg.CurrentSnapshot().Streams["live1"]; ok {
		t.Fatal("root-cause sanity: after publish_end live1 must be GONE from the snapshot " +
			"(the aggregator removes it before marking inactive) — this is why the old " +
			"present-but-inactive wildcard check could never match")
	}

	tick(t, ctx, ev, clock, 3) // t=5,10,15 → fire at t15
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("wildcard stream_offline must fire when a real aggregator stream ends; got %d firing "+
			"(this is the D-156 defect)", got)
	}
}

// TestStreamOffline_Wildcard_DisabledReEnabled_NoSpuriousFire_S93 (D-157 review
// fix): a wildcard offline rule that is disabled, has its watched stream end while
// disabled, then is re-enabled must NOT fire for that already-ended stream — its
// edge-detection tracker is pruned on disable and rebuilt fresh (empty prevPresent)
// on re-enable, so the stale present record cannot fabricate a present→gone edge.
func TestStreamOffline_Wildcard_DisabledReEnabled_NoSpuriousFire_S93(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	rule := createStreamOfflineRule(t, store, "", 0, 300) // wildcard, WindowS=0
	ctx := context.Background()

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	// t=0: stream present while enabled → tracker records prevPresent={live1}.
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx)

	// Disable the rule → it is skipped and its tracker pruned.
	rule.Enabled = false
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	tick(t, ctx, ev, clock, 1)

	// Stream ends while the rule is disabled.
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 1)

	// Re-enable → tracker rebuilds fresh; the already-ended stream is not a new edge.
	rule.Enabled = true
	if err := store.UpdateAlertRule(ctx, rule); err != nil {
		t.Fatalf("re-enable rule: %v", err)
	}
	tick(t, ctx, ev, clock, 3)
	if got := nc.countState("firing"); got != 0 {
		t.Fatalf("re-enabling a wildcard offline rule must not fire for a stream that ended "+
			"while it was disabled; got %d firing (stale-tracker regression)", got)
	}
}

// TestStreamOffline_Wildcard_GroupByApp_RecoveryResolves_S93 (D-157 review fix):
// a wildcard offline rule with group_by=app must not orphan-stick-fire when a
// stream recovers within the hold window. group_by does not collapse wildcard
// offline (offline streams are absent from the snapshot, so applyGroupBy would
// re-key them inconsistently vs. their online state); it stays one alert per stream
// and therefore fires and resolves against the same stream-id key.
func TestStreamOffline_Wildcard_GroupByApp_RecoveryResolves_S93(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := s93Clock()
	ev, _ := newTestEvaluator(t, store, live, clock)
	ctx := context.Background()

	rule := meta.AlertRuleRow{
		Name: "wild-offline-groupby", Metric: "stream_offline", Operator: "eq", Threshold: 1,
		WindowS: 0, ScopeJSON: `{}`, GroupBy: sql.NullString{String: "app", Valid: true},
		Severity: "critical", CooldownS: 300, Enabled: true, Muted: false,
		MaintenanceWindows: "[]", ChannelIDs: `["test-channel"]`,
	}
	if _, err := store.CreateAlertRule(ctx, rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	nc := &offlineNotifCapture{}
	ev.SetNotifySink(nc.sink)

	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	ev.TickOnce(ctx) // t=0 present
	live.setSnap(emptySnap())
	tick(t, ctx, ev, clock, 1) // t=5: live1 gone → fires (WindowS=0)
	if got := nc.countState("firing"); got != 1 {
		t.Fatalf("expected 1 firing after offline; got %d", got)
	}

	// Recovery within the hold window (hold = 0+max(0,2*5)=10 from t5 → t15).
	live.setSnap(streamPresentSnap("live1", "app1", "n1"))
	tick(t, ctx, ev, clock, 1) // t=10 present again
	if got := nc.countState("resolved"); got != 1 {
		t.Fatalf("group_by=app wildcard offline must RESOLVE on recovery, not orphan-stick-fire; "+
			"got %d resolved", got)
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

// tick advances the fake clock by one TickInterval (5s) and evaluates, n times.
func tick(t *testing.T, ctx context.Context, ev interface{ TickOnce(context.Context) }, clock *alert.FakeClock, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		clock.Advance(5 * time.Second)
		ev.TickOnce(ctx)
	}
}
