// Package alert_test — S67 (D-129) alert-evaluator correctness cluster.
//
// Three findings from the S62 audit, each fixed here and covered TDD-style
// (RED against the pre-fix code, GREEN after):
//
//	[7] evalNodeMetric (evaluator.go): a node that did not report cpu/mem/disk
//	    was evaluated as 0 instead of skipped — mirrors the D-088 presence guard
//	    already in evalAnomalyNodes. Without it, `node_cpu lt 50` false-fires on
//	    an unreported 0.
//	[8] evalStreamOffline (evaluator.go): hardcoded value=0 and ok=!active,
//	    reporting a misleading 0 on a firing alert and ignoring the rule
//	    operator/threshold. Now a binary metric (1.0 offline / 0.0 online)
//	    evaluated via compare().
//	[9] evalLicenseExpiry (license_expiry.go): returned nil for a perpetual
//	    licence, so a previously-firing near-expiry alert never resolved
//	    (processEvaluation has no stale-state sweep). Now emits a resolving result.
package alert_test

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// s67ThresholdRule builds an enabled, immediate (window_s=0) threshold rule.
func s67ThresholdRule(name, metric, operator string, threshold float64) meta.AlertRuleRow {
	return meta.AlertRuleRow{
		Name:               name,
		Metric:             metric,
		Operator:           operator,
		Threshold:          threshold,
		WindowS:            0,
		ScopeJSON:          `{}`,
		Severity:           "warning",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
}

// s67RunTick creates an evaluator over one rule and one snapshot, ticks once
// (window_s=0 fires immediately), and returns the captured notifications.
func s67RunTick(t *testing.T, rule meta.AlertRuleRow, snap *domain.LiveSnapshot) []map[string]any {
	t.Helper()
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAlertRule(ctx, rule); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	live := newFakeLive()
	live.setSnap(snap)
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
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

	ev.TickOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	out := make([]map[string]any, len(notifs))
	copy(out, notifs)
	return out
}

func s67NodeSnap(n *domain.LiveNodeStats) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{n.NodeID: n},
	}
}

func s67StreamSnap(streams ...*domain.LiveStream) *domain.LiveSnapshot {
	m := make(map[string]*domain.LiveStream, len(streams))
	for _, s := range streams {
		m[s.StreamID] = s
	}
	return &domain.LiveSnapshot{Streams: m, Nodes: map[string]*domain.LiveNodeStats{}}
}

// ─── [7] evalNodeMetric presence guards ──────────────────────────────────────

// TestEvalNodeMetric_UnreportedCPU_NoFire_S67: a node that did not report cpu_pct
// (CPUPCTReported=false, CPUPCT=0) must be skipped. Without the guard, `node_cpu lt 50`
// fires on the phantom 0 (compare(0,"lt",50)=true) — a false alert for a standalone
// AMS 3.x node that never reports cpu_pct.
func TestEvalNodeMetric_UnreportedCPU_NoFire_S67(t *testing.T) {
	notifs := s67RunTick(t,
		s67ThresholdRule("s67-cpu-unreported", "node_cpu", "lt", 50),
		s67NodeSnap(&domain.LiveNodeStats{NodeID: "n1", CPUPCT: 0, CPUPCTReported: false}))
	if len(notifs) != 0 {
		t.Errorf("node_cpu with CPUPCTReported=false must not fire; got %d notifications "+
			"(unreported 0 was compared as a real reading)", len(notifs))
	}
}

// TestEvalNodeMetric_UnreportedMem_NoFire_S67: same guard for mem_pct.
func TestEvalNodeMetric_UnreportedMem_NoFire_S67(t *testing.T) {
	notifs := s67RunTick(t,
		s67ThresholdRule("s67-mem-unreported", "node_mem", "lt", 50),
		s67NodeSnap(&domain.LiveNodeStats{NodeID: "n1", MemPCT: 0, MemPCTReported: false}))
	if len(notifs) != 0 {
		t.Errorf("node_mem with MemPCTReported=false must not fire; got %d notifications", len(notifs))
	}
}

// TestEvalNodeMetric_UnreportedDisk_NoFire_S67: same guard for disk_pct.
func TestEvalNodeMetric_UnreportedDisk_NoFire_S67(t *testing.T) {
	notifs := s67RunTick(t,
		s67ThresholdRule("s67-disk-unreported", "node_disk", "lt", 50),
		s67NodeSnap(&domain.LiveNodeStats{NodeID: "n1", DiskPCT: 0, DiskPCTReported: false}))
	if len(notifs) != 0 {
		t.Errorf("node_disk with DiskPCTReported=false must not fire; got %d notifications", len(notifs))
	}
}

// TestEvalNodeMetric_ReportedCPU_Fires_S67 is the positive control: the guard must not
// block a node that genuinely reports the field. CPUPCTReported=true, CPUPCT=95, and a
// `node_cpu gt 90` rule → the alert fires with value 95.
func TestEvalNodeMetric_ReportedCPU_Fires_S67(t *testing.T) {
	notifs := s67RunTick(t,
		s67ThresholdRule("s67-cpu-reported", "node_cpu", "gt", 90),
		s67NodeSnap(&domain.LiveNodeStats{NodeID: "n1", CPUPCT: 95, CPUPCTReported: true}))
	if len(notifs) == 0 {
		t.Fatal("node_cpu with CPUPCTReported=true and CPUPCT=95 must fire on `gt 90`; " +
			"got 0 notifications (guard is over-broad)")
	}
	if v, _ := notifs[0]["value"].(float64); v != 95 {
		t.Errorf("notification value: got %v, want 95", notifs[0]["value"])
	}
}

// TestEvalNodeMetric_FieldStopsReporting_Resolves_S67 covers the D-129 review regression:
// a node_cpu alert that FIRED while the node reported CPU must RESOLVE (not stick) when the
// node later stops reporting cpu_pct (e.g. an AMS 5.x→3.x downgrade). The presence guard
// emits an ok=false result rather than skipping the node — otherwise processEvaluation (no
// stale-state sweep) would never see the groupKey again and the alert would stay firing.
func TestEvalNodeMetric_FieldStopsReporting_Resolves_S67(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAlertRule(ctx, s67ThresholdRule("s67-cpu-resolve", "node_cpu", "gt", 90)); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
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

	// Tick 1: node reports high CPU → fires.
	live.setSnap(s67NodeSnap(&domain.LiveNodeStats{NodeID: "n1", CPUPCT: 95, CPUPCTReported: true}))
	ev.TickOnce(ctx)

	// Tick 2: node stops reporting cpu_pct (downgrade) → the firing alert must resolve.
	live.setSnap(s67NodeSnap(&domain.LiveNodeStats{NodeID: "n1", CPUPCT: 0, CPUPCTReported: false}))
	clock.Advance(1 * time.Second)
	ev.TickOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(notifs) != 2 {
		t.Fatalf("expected 2 notifications (firing then resolved); got %d: %v", len(notifs), notifs)
	}
	if s, _ := notifs[1]["state"].(string); s != "resolved" {
		t.Errorf("second notification state: got %q, want resolved "+
			"(a node that stops reporting a field must resolve, not stick firing)", notifs[1]["state"])
	}
}

// ─── [8] evalStreamOffline binary value + compare ─────────────────────────────

// TestEvalStreamOffline_ScopedOffline_FiresValueOne_S67: a scoped stream absent from the
// snapshot is offline. With the default { eq, 1 } rule it fires, and the notified value
// must be 1.0 (was hardcoded 0 pre-D-129 — misleading on a firing alert, and inconsistent
// with the threshold of 1).
func TestEvalStreamOffline_ScopedOffline_FiresValueOne_S67(t *testing.T) {
	rule := s67ThresholdRule("s67-offline-scoped", "stream_offline", "eq", 1)
	rule.ScopeJSON = `{"stream_id":"s1"}`
	notifs := s67RunTick(t, rule, s67StreamSnap()) // s1 absent → offline
	if len(notifs) != 1 {
		t.Fatalf("scoped stream_offline for an absent stream must fire once; got %d", len(notifs))
	}
	if v, _ := notifs[0]["value"].(float64); v != 1 {
		t.Errorf("firing stream_offline value: got %v, want 1.0 (D-129 binary metric)", notifs[0]["value"])
	}
}

// TestEvalStreamOffline_ScopedOnline_NoFire_S67: a scoped stream present in the snapshot
// is online → value 0.0 → compare(0,"eq",1)=false → no fire.
func TestEvalStreamOffline_ScopedOnline_NoFire_S67(t *testing.T) {
	rule := s67ThresholdRule("s67-online-scoped", "stream_offline", "eq", 1)
	rule.ScopeJSON = `{"stream_id":"s1"}`
	notifs := s67RunTick(t, rule, s67StreamSnap(&domain.LiveStream{StreamID: "s1", Active: true}))
	if len(notifs) != 0 {
		t.Errorf("scoped stream_offline for a present stream must not fire; got %d", len(notifs))
	}
}

// TestEvalStreamOffline_WildcardInactive_FiresValueOne_S67: wildcard rule, a present
// stream with Active=false is offline → fires with value 1.0.
func TestEvalStreamOffline_WildcardInactive_FiresValueOne_S67(t *testing.T) {
	notifs := s67RunTick(t,
		s67ThresholdRule("s67-offline-wild", "stream_offline", "eq", 1),
		s67StreamSnap(&domain.LiveStream{StreamID: "s1", Active: false}))
	if len(notifs) != 1 {
		t.Fatalf("wildcard stream_offline for an inactive stream must fire once; got %d", len(notifs))
	}
	if v, _ := notifs[0]["value"].(float64); v != 1 {
		t.Errorf("firing stream_offline value: got %v, want 1.0", notifs[0]["value"])
	}
}

// TestEvalStreamOffline_CompareRespected_S67 is the discriminating test for the compare()
// path. Pre-D-129 the operator/threshold were ignored (ok hardcoded to !active), so an
// offline stream fired regardless of the rule. Under a rule whose predicate is NOT met by
// the offline value (`eq 0`), the offline stream must NOT fire once compare() is honored.
func TestEvalStreamOffline_CompareRespected_S67(t *testing.T) {
	notifs := s67RunTick(t,
		s67ThresholdRule("s67-offline-cmp", "stream_offline", "eq", 0),
		s67StreamSnap(&domain.LiveStream{StreamID: "s1", Active: false}))
	if len(notifs) != 0 {
		t.Errorf("offline stream under `eq 0` must not fire once compare() is honored; "+
			"got %d (operator/threshold ignored)", len(notifs))
	}
}

// ─── [9] evalLicenseExpiry resolve-on-perpetual ───────────────────────────────

// TestLicenseExpiry_PerpetualAfterFiring_Resolves_S67: a near-expiry licence alert that
// has fired must RESOLVE when the licence is renewed to perpetual. Pre-D-129
// evalLicenseExpiry returned nil for a perpetual licence, so processEvaluation (which has
// no stale-state sweep) never saw the "license" groupKey again and the alert stayed firing
// forever. The fix emits a resolving result (ok=false) keyed "license".
func TestLicenseExpiry_PerpetualAfterFiring_Resolves_S67(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAlertRule(ctx, licenseExpiryRule()); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	})
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
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

	// Tick 1: 10 days left (< 14) → fires.
	ev.SetLicenseChecker(alert.FakeLicenseChecker{Days: 10, HasExpiry: true})
	ev.TickOnce(ctx)

	// Tick 2: licence renewed to perpetual → the firing alert must resolve.
	ev.SetLicenseChecker(alert.FakeLicenseChecker{Days: 0, HasExpiry: false})
	clock.Advance(1 * time.Second)
	ev.TickOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(notifs) != 2 {
		t.Fatalf("expected 2 notifications (firing then resolved); got %d: %v", len(notifs), notifs)
	}
	if s, _ := notifs[0]["state"].(string); s != "firing" {
		t.Errorf("first notification state: got %q, want firing", notifs[0]["state"])
	}
	if s, _ := notifs[1]["state"].(string); s != "resolved" {
		t.Errorf("second notification state: got %q, want resolved "+
			"(a perpetual licence must resolve a previously-firing near-expiry alert)", notifs[1]["state"])
	}
	// D-129 review: the resolve value must stay within float32 range — the OpenAPI
	// alert value field is format:float, so math.MaxFloat64 would overflow to +Inf on
	// a strict float32 client and render as garbage in the history UI.
	if v, ok := notifs[1]["value"].(float64); !ok || v > math.MaxFloat32 {
		t.Errorf("resolve value %v must be float32-safe (OpenAPI format:float); "+
			"math.MaxFloat64 overflows to +Inf", notifs[1]["value"])
	}
}
