package anomaly

// s70_d132_internal_test.go — internal (white-box) tests for the D-132 anomaly
// cluster. These drive the unexported detection primitives directly so the
// hysteresis timing is exercised with a FIXED baseline (no Welford drift) and
// the scopeJSON/parseScopeJSON round-trip is checked byte-for-byte.
//
//	[16] detectFlagsLocked(setHysteresis=false) must NOT arm the shared cooldown.
//	[17] a fired flag suppresses exactly HysteresisTicks subsequent ticks.
//	[18] scopeJSON escapes ID fields; parseScopeJSON round-trips them.

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// internalFakeFlagStore is a minimal FlagEventStore for white-box tests that need
// to drive WarmHysteresis without ClickHouse. RecentFlagKeys returns keys verbatim.
type internalFakeFlagStore struct{ keys []FlagKey }

func (f *internalFakeFlagStore) InsertAnomalyFlagEvent(context.Context, AnomalyFlagEvent) error {
	return nil
}
func (f *internalFakeFlagStore) RecentFlagKeys(context.Context, int) ([]FlagKey, error) {
	return f.keys, nil
}

const d132Scope = `{"stream_id":"s1"}`

// fixedAnomalyInputs returns a baseline+live pair whose observed value sits at
// z≈10 (10σ) so detectFlagsLocked always fires when not in cooldown. The baseline
// is a fixed slice — never fed through Welford — so repeated detection passes see
// an unchanging z-score.
func fixedAnomalyInputs() ([]AnomalyBaselineRow, map[string]float64) {
	baselines := []AnomalyBaselineRow{
		{Metric: "viewers", Scope: d132Scope, Mean: 0, Stddev: 1, SampleCount: 100, WindowS: 3600},
	}
	live := map[string]float64{"viewers:" + d132Scope: 10}
	return baselines, live
}

// TestDetectFlagsLocked_CooldownSuppressesExactlyNTicks pins [17]. Simulating the
// UpdateBaselines tick order (decrementHysteresis THEN detect), a fire must be
// followed by exactly HysteresisTicks suppressed ticks before the next fire — so
// fires land (HysteresisTicks+1) ticks apart. The pre-fix code armed the counter
// to HysteresisTicks (not +1), suppressing only N-1 ticks; this test kills that.
func TestDetectFlagsLocked_CooldownSuppressesExactlyNTicks(t *testing.T) {
	const n = 3
	d := New(Config{DefaultSigma: 3.0, MinSamples: 1, HysteresisTicks: n, TickInterval: time.Second}, nil, nil, nil)
	baselines, live := fixedAnomalyInputs()

	const ticks = 3*n + 2 // 11 ticks → expect fires at 0, 4, 8
	var fireTicks []int
	for tick := 0; tick < ticks; tick++ {
		d.decrementHysteresis() // matches UpdateBaselines: decrement before detect
		d.mu.Lock()
		got := d.detectFlagsLocked(baselines, live, d.defaultSigma, true)
		d.mu.Unlock()
		if len(got) > 0 {
			fireTicks = append(fireTicks, tick)
		}
	}

	var want []int
	for tk := 0; tk < ticks; tk += n + 1 {
		want = append(want, tk)
	}
	if !reflect.DeepEqual(fireTicks, want) {
		t.Fatalf("fire ticks = %v, want %v (each fire must suppress exactly %d ticks)", fireTicks, want, n)
	}
	if len(fireTicks) >= 2 && fireTicks[1]-fireTicks[0] != n+1 {
		t.Fatalf("cooldown gap = %d ticks, want %d (D-132 [17] off-by-one)", fireTicks[1]-fireTicks[0], n+1)
	}
}

// TestDetectFlagsLocked_ReadPassDoesNotArmCooldown pins the [16] core: a
// ComputeFlags-style pass (setHysteresis=false) detects the anomaly but must NOT
// arm the shared cooldown, so the immediately-following tick pass (setHysteresis=
// true) still fires and persists. The pre-fix code armed unconditionally, which
// let an on-read poll suppress the tick path's audit-trail write.
func TestDetectFlagsLocked_ReadPassDoesNotArmCooldown(t *testing.T) {
	d := New(Config{DefaultSigma: 3.0, MinSamples: 1, HysteresisTicks: 5, TickInterval: time.Second}, nil, nil, nil)
	baselines, live := fixedAnomalyInputs()
	hk := hysteresisKey{metric: "viewers", scope: d132Scope}

	// Read pass: detects but must not arm.
	d.mu.Lock()
	readPass := d.detectFlagsLocked(baselines, live, d.defaultSigma, false)
	armedAfterRead := d.hysteresis[hk]
	d.mu.Unlock()
	if len(readPass) != 1 {
		t.Fatalf("read pass: expected 1 detected flag, got %d", len(readPass))
	}
	if armedAfterRead != 0 {
		t.Fatalf("read pass armed the cooldown (rem=%d); GET /anomalies must not arm it (D-132 [16])", armedAfterRead)
	}

	// Tick pass immediately after: must still fire (read pass left cooldown clear)
	// and now arm it.
	d.mu.Lock()
	tickPass := d.detectFlagsLocked(baselines, live, d.defaultSigma, true)
	armedAfterTick := d.hysteresis[hk]
	d.mu.Unlock()
	if len(tickPass) != 1 {
		t.Fatalf("tick pass after read: expected 1 flag (cooldown must be clear), got %d", len(tickPass))
	}
	if armedAfterTick != d.hysteresisTicks+1 {
		t.Fatalf("tick pass armed rem=%d, want %d (HysteresisTicks+1)", armedAfterTick, d.hysteresisTicks+1)
	}
}

// TestScopeJSON_RoundTripEscaping pins [18]: scopeJSON must emit valid JSON for
// IDs containing quotes/backslashes and parseScopeJSON must recover the exact
// original value. The pre-fix concat produced invalid JSON that the tolerant
// scan truncated at the first raw quote, mis-attributing anomaly events.
func TestScopeJSON_RoundTripEscaping(t *testing.T) {
	cases := []struct {
		name             string
		nodeID, app, sid string
	}{
		{"plain", "", "", "stream-123"},
		{"quote", "", "", `foo"bar`},
		{"backslash", "", "", `foo\bar`},
		{"quote-and-backslash", "", "", `a"b\c`},
		{"all-fields", "node-1", "LiveApp", `s"1`},
		{"node-quote", `n"1`, "", ""},
		{"app-quote", "", `Live"App`, ""},
		{"tab-control", "", "", "a\tb"},
		{"utf8", "", "", "naïve→ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			js := ScopeJSON(tc.nodeID, tc.app, tc.sid)
			if !json.Valid([]byte(js)) {
				t.Fatalf("ScopeJSON(%q,%q,%q) = %q — not valid JSON", tc.nodeID, tc.app, tc.sid, js)
			}
			got := parseScopeJSON(js)
			if got.NodeID != tc.nodeID {
				t.Errorf("NodeID round-trip: got %q, want %q (json=%q)", got.NodeID, tc.nodeID, js)
			}
			if got.App != tc.app {
				t.Errorf("App round-trip: got %q, want %q (json=%q)", got.App, tc.app, js)
			}
			if got.StreamID != tc.sid {
				t.Errorf("StreamID round-trip: got %q, want %q (json=%q)", got.StreamID, tc.sid, js)
			}
		})
	}
}

// TestScopeJSON_ByteIdentityForNormalIDs guards the upgrade path: an ID with no
// special characters MUST serialize byte-identically to the pre-D-132 format, or
// every persisted baseline key (metric:scope:window) would change on upgrade and
// reset the rolling statistics.
func TestScopeJSON_ByteIdentityForNormalIDs(t *testing.T) {
	cases := []struct{ nodeID, app, sid, want string }{
		{"", "", "s1", `{"stream_id":"s1"}`},
		{"node-1", "", "", `{"node_id":"node-1"}`},
		{"n1", "LiveApp", "s1", `{"node_id":"n1","app":"LiveApp","stream_id":"s1"}`},
		{"", "", "", "{}"},
	}
	for _, tc := range cases {
		if got := ScopeJSON(tc.nodeID, tc.app, tc.sid); got != tc.want {
			t.Errorf("ScopeJSON(%q,%q,%q) = %q, want %q", tc.nodeID, tc.app, tc.sid, got, tc.want)
		}
	}
}

// TestParseScopeJSON_LegacyFallback covers the tolerant fallback branch for
// pre-D-132 rows: a raw unescaped quote makes json.Unmarshal fail, and the scan
// still returns a best-effort (truncated) value without crashing. Well-formed
// rows parse identically through the JSON path.
func TestParseScopeJSON_LegacyFallback(t *testing.T) {
	got := parseScopeJSON(`{"stream_id":"live"stream"}`)
	if got.StreamID != "live" {
		t.Errorf("legacy fallback StreamID = %q, want %q (best-effort truncation)", got.StreamID, "live")
	}
	got2 := parseScopeJSON(`{"node_id":"n1","stream_id":"s1"}`)
	if got2.NodeID != "n1" || got2.StreamID != "s1" {
		t.Errorf("well-formed parse = %+v, want NodeID=n1 StreamID=s1", got2)
	}
}

// TestWarmHysteresis_ArmsConsistentlyWithFreshFire pins the D-132 review finding:
// the restart-dedup path must arm the cooldown to HysteresisTicks+1, identical to a
// fresh fire (detectFlagsLocked), so that exactly HysteresisTicks ticks are suppressed
// after a restart. Arming to plain HysteresisTicks suppresses only N-1 ticks (the
// decrement-before-detect cycle eats one) and re-fires early, writing a duplicate
// audit event — the very thing WarmHysteresis exists to prevent. The prior test
// TestFlagStore_WarmHysteresis_SuppressesRefire could not catch this (it never drives
// a tick, so any non-zero armed value passes); this asserts the armed value directly.
func TestWarmHysteresis_ArmsConsistentlyWithFreshFire(t *testing.T) {
	const n = 5
	d := New(Config{DefaultSigma: 3.0, MinSamples: 1, HysteresisTicks: n, TickInterval: time.Second}, nil, nil, nil)
	d.SetFlagStore(&internalFakeFlagStore{keys: []FlagKey{{Metric: "viewers", Scope: d132Scope}}})

	if err := d.WarmHysteresis(context.Background()); err != nil {
		t.Fatalf("WarmHysteresis: %v", err)
	}

	got := d.hysteresis[hysteresisKey{metric: "viewers", scope: d132Scope}]
	if got != n+1 {
		t.Fatalf("WarmHysteresis armed rem=%d, want %d (HysteresisTicks+1, matching a fresh fire — "+
			"so a restart suppresses exactly %d ticks, not %d, avoiding a duplicate re-fire)", got, n+1, n, n-1)
	}
}
