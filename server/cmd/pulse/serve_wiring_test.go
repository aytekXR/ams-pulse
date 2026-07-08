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
	"testing"

	anomaly "github.com/pulse-analytics/pulse/server/internal/anomaly"
	beaconingest "github.com/pulse-analytics/pulse/server/internal/collector/beacon"
	"github.com/pulse-analytics/pulse/server/internal/domain"
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
