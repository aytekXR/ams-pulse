package main

// D-058 regression suite: dedicated beacon ingest listener wiring.
//
// Two defects found live during D-058 staging verify:
//
//   (a) The dedicated listener was never started outside CI: PULSE_INGEST_LISTEN_ADDR
//       was read but the newServer() call that creates the listener was absent from the
//       path exercised in production, so /beacon on port :8091 returned 502.
//
//   (b) VD-15: the dedicated listener was constructed with License=nil (fail-open).
//       Free-tier deployments received HTTP 202 on the beacon ingest endpoint instead
//       of 403; the API-mux path correctly returned 403 because it wired the license
//       manager, but the dedicated listener did not.
//
// Fix: beaconListenerConfig extracts the config-building step so it can be pinned
// independently of the full newServer() construction (which requires ClickHouse).
//
// TDD layout:
//   TestBeaconListenerConfig_AddrSet      — D-058 pin (a): config produced when addr set
//   TestBeaconListenerConfig_AddrEmpty    — D-058 pin (a): no config when addr empty
//   TestBeaconListenerConfig_LicenseNonNil — D-058 pin (b)/VD-15: License always wired
//   TestAnomalyBridge_ComputeFlags_NilSnapshot — bridge ComputeFlags smoke
//
// Red evidence: initial wrong assertion `wantLicenseNil = true` caused
// TestBeaconListenerConfig_LicenseNonNil to FAIL ("License nil = expected non-nil").
// Corrected to `wantLicenseNil = false`.

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	anomaly "github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/collector/aggregator"
	beaconingest "github.com/pulse-analytics/pulse/server/internal/collector/beacon"
	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Beacon listener config helpers ──────────────────────────────────────────

// testLicenseChecker is a minimal stub satisfying beaconingest.LicenseChecker.
type testLicenseChecker struct{ failOpen bool }

func (t *testLicenseChecker) CheckBeaconIngest() error { return nil }

// ─── D-058 pin (a): config is built when PULSE_INGEST_LISTEN_ADDR is set ────

// TestBeaconListenerConfig_AddrSet verifies that beaconListenerConfig returns
// a config (ok=true) and the correct listen address when IngestListenAddr is set.
// This pins D-058 defect (a): the dedicated listener must be constructed when the
// env var is present.
func TestBeaconListenerConfig_AddrSet(t *testing.T) {
	const addr = ":8091"
	lic := &testLicenseChecker{}

	cfg, ok := beaconListenerConfig(addr, lic)

	if !ok {
		t.Fatal("beaconListenerConfig: expected ok=true when listenAddr is non-empty (D-058 pin a)")
	}
	if cfg.ListenAddr != addr {
		t.Errorf("beaconListenerConfig: ListenAddr = %q, want %q", cfg.ListenAddr, addr)
	}
}

// TestBeaconListenerConfig_AddrEmpty verifies that beaconListenerConfig returns
// ok=false when listenAddr is empty, so no dedicated listener is constructed.
func TestBeaconListenerConfig_AddrEmpty(t *testing.T) {
	lic := &testLicenseChecker{}

	_, ok := beaconListenerConfig("", lic)

	if ok {
		t.Fatal("beaconListenerConfig: expected ok=false when listenAddr is empty")
	}
}

// TestBeaconListenerConfig_LicenseNonNil is the binding VD-15 / D-058 pin (b):
// the dedicated beacon listener's Config.License MUST be non-nil.
//
// When License is nil, the beacon handler operates fail-open: Free-tier deployments
// receive HTTP 202 on the dedicated beacon port (skipping the license gate in
// beacon.Handler.Handle). This was the live defect found in D-058 staging verify.
//
// This test MUST FAIL if beaconListenerConfig is ever changed to pass License=nil.
func TestBeaconListenerConfig_LicenseNonNil(t *testing.T) {
	lic := &testLicenseChecker{}

	cfg, ok := beaconListenerConfig(":8091", lic)
	if !ok {
		t.Fatal("beaconListenerConfig: expected ok=true")
	}

	// VD-15 binding assertion: License must be exactly the object we passed in.
	// A nil License means the handler skips the license check (fail-open).
	if cfg.License == nil {
		t.Fatal("VD-15 D-058: beaconListenerConfig returned Config.License=nil — " +
			"the dedicated beacon listener will be fail-open (Free tier gets 202)")
	}
	if cfg.License != beaconingest.LicenseChecker(lic) {
		t.Error("VD-15: beaconListenerConfig: License is non-nil but is not the object passed in (wiring broken)")
	}
}

// TestBeaconListenerConfig_RateLimitsNonZero verifies that rate limit fields
// are populated with sensible non-zero defaults by beaconListenerConfig so the
// beacon handler does not use the package defaults for those fields.
func TestBeaconListenerConfig_RateLimitsNonZero(t *testing.T) {
	cfg, _ := beaconListenerConfig(":8091", &testLicenseChecker{})

	if cfg.RateLimitPerTokenRPS <= 0 {
		t.Errorf("beaconListenerConfig: RateLimitPerTokenRPS = %v, want > 0", cfg.RateLimitPerTokenRPS)
	}
	if cfg.RateBurst <= 0 {
		t.Errorf("beaconListenerConfig: RateBurst = %v, want > 0", cfg.RateBurst)
	}
}

// ─── anomalyDetectorBridge.ComputeFlags smoke ─────────────────────────────────

// stubBaselineStore is a no-op BaselineStore for the bridge smoke test.
type stubBaselineStore struct{}

func (s *stubBaselineStore) ListAnomalyBaselines(_ context.Context) ([]anomaly.AnomalyBaselineRow, error) {
	return nil, nil
}

func (s *stubBaselineStore) UpsertAnomalyBaseline(_ context.Context, _ anomaly.AnomalyBaselineRow) error {
	return nil
}

// stubLiveProvider returns a nil snapshot so ComputeFlags short-circuits.
type stubLiveProvider struct{}

func (s *stubLiveProvider) CurrentSnapshot() *domain.LiveSnapshot { return nil }

func (s *stubLiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot)
	return ch, func() { close(ch) }
}

// TestAnomalyBridge_ComputeFlags_NilSnapshot verifies that anomalyDetectorBridge
// delegates to anomaly.Detector.ComputeFlags and returns an empty (non-nil) slice
// when the live snapshot is nil (no active streams).
func TestAnomalyBridge_ComputeFlags_NilSnapshot(t *testing.T) {
	det := anomaly.New(anomaly.Config{}, &stubBaselineStore{}, &stubLiveProvider{}, nil)
	bridge := &anomalyDetectorBridge{det: det}

	flags, err := bridge.ComputeFlags(context.Background(), 4.0)
	if err != nil {
		t.Fatalf("anomalyDetectorBridge.ComputeFlags: unexpected error: %v", err)
	}
	// Nil snapshot → no flags; the bridge must return a non-nil empty slice (not nil).
	if flags == nil {
		t.Error("anomalyDetectorBridge.ComputeFlags: returned nil slice for nil snapshot, want empty slice")
	}
	if len(flags) != 0 {
		t.Errorf("anomalyDetectorBridge.ComputeFlags: len=%d, want 0 for nil snapshot", len(flags))
	}
}

// ─── D-086 wiring pin: flagHistoryBridge ─────────────────────────────────────

// stubFlagQueryer is a test double satisfying the flagQueryer interface used
// by flagHistoryBridge (serve.go). It returns pre-configured data without a
// real ClickHouse connection.
type stubFlagQueryer struct {
	events     []anomaly.AnomalyFlagEvent
	nextCursor string
	err        error
}

func (s *stubFlagQueryer) QueryFlagHistory(_ context.Context, _, _ time.Time, _, _, _ string, _ float64, _ int, _ string) ([]anomaly.AnomalyFlagEvent, string, error) {
	return s.events, s.nextCursor, s.err
}

// TestFlagHistoryBridge_QueryFlagHistory verifies that flagHistoryBridge correctly
// converts anomaly.AnomalyFlagEvent rows to api.AnomalyFlagAPI values (D-086,
// ADR-0009 §6 read path). The test exercises scope field mapping
// (NodeID/App/StreamID → Scope), TS conversion (DetectedAt.UnixMilli()), and
// NextCursor passthrough.
//
// Compile-time catch: referencing &flagHistoryBridge{} here causes a compile
// failure until serve.go defines the type — that is the RED evidence for this pin.
func TestFlagHistoryBridge_QueryFlagHistory(t *testing.T) {
	detectedAt := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	stub := &stubFlagQueryer{
		events: []anomaly.AnomalyFlagEvent{{
			ID:         "evt-1",
			Metric:     "ingest_bitrate_kbps",
			NodeID:     "node-1",
			App:        "live",
			StreamID:   "s1",
			Scope:      `{"node_id":"node-1","app":"live","stream_id":"s1"}`,
			Observed:   150.0,
			Expected:   100.0,
			Sigma:      3.5,
			DetectedAt: detectedAt,
		}},
		nextCursor: "abc123",
	}

	bridge := &flagHistoryBridge{store: stub}
	page, err := bridge.QueryFlagHistory(context.Background(), time.Time{}, time.Time{}, "", "", "", 0, 50, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory: unexpected error: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("page.Items: got %d items, want 1", len(page.Items))
	}
	got := page.Items[0]
	if got.ID != "evt-1" {
		t.Errorf("ID: got %q, want evt-1", got.ID)
	}
	if got.Metric != "ingest_bitrate_kbps" {
		t.Errorf("Metric: got %q, want ingest_bitrate_kbps", got.Metric)
	}
	if got.Scope.NodeID != "node-1" {
		t.Errorf("Scope.NodeID: got %q, want node-1", got.Scope.NodeID)
	}
	if got.Scope.App != "live" {
		t.Errorf("Scope.App: got %q, want live", got.Scope.App)
	}
	if got.Scope.StreamID != "s1" {
		t.Errorf("Scope.StreamID: got %q, want s1", got.Scope.StreamID)
	}
	if got.Observed != 150.0 {
		t.Errorf("Observed: got %v, want 150.0", got.Observed)
	}
	if got.Expected != 100.0 {
		t.Errorf("Expected: got %v, want 100.0", got.Expected)
	}
	if got.Sigma != 3.5 {
		t.Errorf("Sigma: got %v, want 3.5", got.Sigma)
	}
	wantTS := detectedAt.UnixMilli()
	if got.TS != wantTS {
		t.Errorf("TS: got %d, want %d (DetectedAt.UnixMilli)", got.TS, wantTS)
	}
	if page.NextCursor != "abc123" {
		t.Errorf("NextCursor: got %q, want abc123", page.NextCursor)
	}
}

// ─── BUG-011 wiring pin: wireNodeEviction ────────────────────────────────────

// TestBUG011_NodeEviction_Wired verifies that wireNodeEviction starts a goroutine
// that calls EvictStaleNodes, so a node whose stats stop flowing is removed from
// the snapshot within the stale threshold (BUG-011 D-087).
//
// Compile-time catch: referencing wireNodeEviction here causes a compile failure
// until serve.go defines the function — that is the RED evidence for this pin
// (analogous to beaconListenerConfig / VD-15).
func TestBUG011_NodeEviction_Wired(t *testing.T) {
	// Use a very short poll interval so the test runs fast.
	// threshold = 3×pollInterval = 15ms; cadence = threshold/2 = 7.5ms.
	const pollInterval = 5 * time.Millisecond

	agg := aggregator.New(3*time.Minute, nil, nil)

	// Seed the node with a successful stats event.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "vanishing-node",
		Data:    map[string]any{"consec_api_errors": 0.0},
	})

	if _, ok := agg.CurrentSnapshot().Nodes["vanishing-node"]; !ok {
		t.Fatal("vanishing-node not in snapshot after seeding")
	}

	// Stats stop flowing — sleep past the threshold so the node becomes stale.
	time.Sleep(20 * time.Millisecond) // 20ms > threshold (15ms)

	// Start the eviction goroutine (the BUG-011 fix). It should fire within cadence
	// and evict the node since LastSeenAt is already past the threshold.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	wireNodeEviction(ctx, agg, pollInterval)

	// Give the goroutine time to fire (2× cadence should be enough).
	time.Sleep(20 * time.Millisecond)

	if _, ok := agg.CurrentSnapshot().Nodes["vanishing-node"]; ok {
		t.Error("BUG-011 FAIL: vanishing-node still in snapshot after stale threshold elapsed — " +
			"wireNodeEviction did not call EvictStaleNodes (or was not called from serve.Start)")
	} else {
		t.Log("PASS BUG-011: vanishing-node evicted by wireNodeEviction goroutine (rung 3 now fires)")
	}
}

// ─── D-062 wiring pin: wireAlertQoEReader ────────────────────────────────────

// qoeWiringFakeLive is a minimal LiveProvider that returns one active stream.
// Used by TestWireAlertQoEReader_ReaderWired to exercise the QoE evaluation path.
type qoeWiringFakeLive struct{}

func (l *qoeWiringFakeLive) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"s1": {StreamID: "s1", App: "live", Active: true},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func (l *qoeWiringFakeLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// TestWireAlertQoEReader_ReaderWired is the D-062 wiring-seam pin.
//
// It verifies that wireAlertQoEReader (serve.go) correctly wires the QoE reader
// to the alert evaluator: with a FakeQoEReader returning 0.0, a
// "rebuffer_ratio >=0" rule FIRES (0.0 >= 0.0 = true); without the reader the
// rule is silently skipped and no notification fires.
//
// Compile-time catch: if wireAlertQoEReader is deleted from serve.go this file
// fails to compile — that is the m3 RED (analogous to beaconListenerConfig / VD-15).
func TestWireAlertQoEReader_ReaderWired(t *testing.T) {
	ctx := context.Background()

	// Minimal in-memory meta store.
	store, err := meta.New(ctx, "sqlite", ":memory:", "test-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ddlBytes, readErr := os.ReadFile("../../../contracts/db/meta/0001_init.sql")
	if readErr != nil {
		t.Skipf("meta DDL not found (run from repo root): %v", readErr)
	}
	if err := store.MigrateEmbedded(ctx, string(ddlBytes)); err != nil {
		t.Fatalf("MigrateEmbedded: %v", err)
	}

	// Rule: rebuffer_ratio >= 0.0 — fires when reader returns 0.0 (0.0 >= 0.0 = true).
	// Without a reader (nil) the rule is silently skipped → no notification.
	_, err = store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name: "wiring-pin-rebuf-gte0", Metric: "rebuffer_ratio", Operator: "gte",
		Threshold: 0.0, WindowS: 5, ScopeJSON: "{}", Severity: "warning",
		CooldownS: 300, Enabled: true, Muted: false,
		MaintenanceWindows: "[]", ChannelIDs: `["c1"]`,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	noop := &channels.NoopChannel{}
	reg := channels.NewRegistry()
	reg.Register("c1", noop)

	clock := alert.NewFakeClock(time.Now().UTC())
	ev := alert.New(alert.Config{
		TickInterval: 5 * time.Second,
		BaseURL:      "http://localhost",
	}, &qoeWiringFakeLive{}, store, reg, clock, nil)

	// D-062 wiring: FakeQoEReader returns 0.0 rebuffer_ratio.
	// 0.0 >= 0.0 is true → rule fires → notification sent.
	wireAlertQoEReader(ev, &alert.FakeQoEReader{RebufferRatio: 0.0})

	var mu sync.Mutex
	var notifs [][]byte
	ev.SetNotifySink(func(p []byte) {
		mu.Lock()
		notifs = append(notifs, p)
		mu.Unlock()
	})

	// Three ticks so the window (5s) elapses: tick1 sets pendingSince, tick2 fires.
	for i := 0; i < 3; i++ {
		clock.Advance(5 * time.Second)
		ev.TickOnce(ctx)
	}

	mu.Lock()
	n := len(notifs)
	mu.Unlock()

	// D-062 wiring pin: with reader wired, rebuffer_ratio=0.0 >= 0.0 fires.
	// Without wireAlertQoEReader (or if it forgets to call SetQoEReader), n=0.
	if n == 0 {
		t.Error("D-062 wiring pin FAIL: rebuffer_ratio rule did not fire — " +
			"wireAlertQoEReader did not call SetQoEReader, or was not called from serve.go")
	} else {
		t.Logf("PASS D-062 wiring pin: rebuffer_ratio rule fired (reader=FakeQoEReader{0.0}, threshold=0.0)")
	}
}

// ─── D-101 wiring pin: wireAlertLicenseExpiry ────────────────────────────────

// TestWireAlertLicenseExpiry_CheckerWired is the D-101 wiring-seam pin.
//
// It verifies that wireAlertLicenseExpiry (serve.go) wires the licence-expiry
// checker into the alert evaluator: with a FakeLicenseChecker reporting 10 days
// until expiry, a "license_expiry lt 14" rule FIRES (10 < 14); without the checker
// the rule is silently skipped and no notification fires. The three unit tests in
// license_expiry_test.go call ev.SetLicenseChecker directly, so only this pin
// catches serve.go forgetting to wire the checker into the real evaluator.
//
// Compile-time catch: if wireAlertLicenseExpiry is deleted from serve.go this file
// fails to compile — the m3 RED, analogous to TestWireAlertQoEReader_ReaderWired.
func TestWireAlertLicenseExpiry_CheckerWired(t *testing.T) {
	ctx := context.Background()

	store, err := meta.New(ctx, "sqlite", ":memory:", "test-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ddlBytes, readErr := os.ReadFile("../../../contracts/db/meta/0001_init.sql")
	if readErr != nil {
		t.Skipf("meta DDL not found (run from repo root): %v", readErr)
	}
	if err := store.MigrateEmbedded(ctx, string(ddlBytes)); err != nil {
		t.Fatalf("MigrateEmbedded: %v", err)
	}

	// Rule: license_expiry < 14 days — fires when the checker reports 10 days left.
	// Without a checker (nil) the rule is silently skipped → no notification.
	_, err = store.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name: "wiring-pin-license-lt14", Metric: "license_expiry", Operator: "lt",
		Threshold: 14, WindowS: 0, ScopeJSON: "{}", Severity: "critical",
		CooldownS: 86400, Enabled: true, Muted: false,
		MaintenanceWindows: "[]", ChannelIDs: `["c1"]`,
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	reg := channels.NewRegistry()
	reg.Register("c1", &channels.NoopChannel{})

	clock := alert.NewFakeClock(time.Now().UTC())
	ev := alert.New(alert.Config{
		TickInterval: 5 * time.Second,
		BaseURL:      "http://localhost",
	}, &qoeWiringFakeLive{}, store, reg, clock, nil)

	// D-101 wiring: FakeLicenseChecker reports 10 days until expiry (< 14 → fires).
	wireAlertLicenseExpiry(ev, alert.FakeLicenseChecker{Days: 10, HasExpiry: true})

	var mu sync.Mutex
	var notifs [][]byte
	ev.SetNotifySink(func(p []byte) {
		mu.Lock()
		notifs = append(notifs, p)
		mu.Unlock()
	})

	// WindowS=0 → tick1 sets pendingSince, tick2 fires (window already elapsed).
	for i := 0; i < 2; i++ {
		clock.Advance(1 * time.Second)
		ev.TickOnce(ctx)
	}

	mu.Lock()
	n := len(notifs)
	mu.Unlock()

	if n == 0 {
		t.Error("D-101 wiring pin FAIL: license_expiry rule did not fire — " +
			"wireAlertLicenseExpiry did not call SetLicenseChecker, or was not called from serve.go")
	} else {
		t.Logf("PASS D-101 wiring pin: license_expiry rule fired (checker=FakeLicenseChecker{10d}, threshold=14)")
	}
}

// TestNodeEvictionThreshold_ThreeTimesPollInterval pins the 3× multiplier
// (D-087 verify M8): the freeze-detection lead time (~15 s at the 5 s default)
// depends on it, and no behavioral test discriminates the exact value.
func TestNodeEvictionThreshold_ThreeTimesPollInterval(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{5 * time.Second, 15 * time.Second},
		{10 * time.Second, 30 * time.Second},
		{0, 3 * restpoller.DefaultPollInterval},  // fallback to the poller default
		{-1, 3 * restpoller.DefaultPollInterval}, // negative treated as unset
	}
	for _, c := range cases {
		if got := nodeEvictionThreshold(c.in); got != c.want {
			t.Errorf("nodeEvictionThreshold(%v) = %v, want %v (3×PollInterval contract)", c.in, got, c.want)
		}
	}
}
