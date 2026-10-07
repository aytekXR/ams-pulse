package license_test

// All-features-free policy (D-194, v0.5.0). The operator made Pulse free for
// everyone — every feature, commercial use included — for at least the first
// year. Every release from v0.5.0 runs with every entitlement gate open.
//
// The tier machinery stays: keys still parse and verify, Tier() still names the
// loaded key, and New() on its own still enforces tiers (the existing tier tests
// keep guarding that path). A future paid model is then a policy flip, not a
// rewrite. These tests pin both halves — the policy opens everything, and the
// enforcing default is untouched — so neither can drift silently.

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/license"
)

// refusedGates runs every entitlement check and returns the names of the ones
// that refused. A new Check* method must be added here, or the policy tests
// below stop covering it.
func refusedGates(m *license.Manager) []string {
	checks := map[string]func() error{
		"CheckNodeLimit(1000)":           func() error { return m.CheckNodeLimit(1000) },
		"CheckChannelAllowed(email)":     func() error { return m.CheckChannelAllowed("email") },
		"CheckChannelAllowed(slack)":     func() error { return m.CheckChannelAllowed("slack") },
		"CheckChannelAllowed(telegram)":  func() error { return m.CheckChannelAllowed("telegram") },
		"CheckChannelAllowed(pagerduty)": func() error { return m.CheckChannelAllowed("pagerduty") },
		"CheckChannelAllowed(webhook)":   func() error { return m.CheckChannelAllowed("webhook") },
		"CheckDataAPI":                   m.CheckDataAPI,
		"CheckProbes":                    m.CheckProbes,
		"CheckAnomalies":                 m.CheckAnomalies,
		"CheckMultiTenant":               m.CheckMultiTenant,
		"CheckReports":                   m.CheckReports,
		"CheckBeaconIngest":              m.CheckBeaconIngest,
		"CheckPrometheus":                m.CheckPrometheus,
		"CheckSSO":                       m.CheckSSO,
		"CheckWhiteLabel":                m.CheckWhiteLabel,
	}
	var refused []string
	for name, check := range checks {
		if check() != nil {
			refused = append(refused, name)
		}
	}
	sort.Strings(refused)
	return refused
}

func newKeyless(t *testing.T) *license.Manager {
	t.Helper()
	m, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	return m
}

func assertEverythingOpen(t *testing.T, m *license.Manager) {
	t.Helper()
	if refused := refusedGates(m); len(refused) != 0 {
		t.Fatalf("all-features-free must open every gate; still refused: %v", refused)
	}
	if got := m.CheckRetention(36500); got != 36500 {
		t.Errorf("CheckRetention(36500) = %d, want 36500 (no retention cap)", got)
	}
	ent := m.Entitlements()
	if ent.MaxNodes != -1 || ent.MaxStreams != -1 || ent.RetentionDays != -1 {
		t.Errorf("limits must all be unlimited (-1); got nodes=%d streams=%d retention=%d",
			ent.MaxNodes, ent.MaxStreams, ent.RetentionDays)
	}
	if !ent.DataAPI || !ent.WhiteLabel {
		t.Errorf("DataAPI and WhiteLabel must be true; got DataAPI=%v WhiteLabel=%v", ent.DataAPI, ent.WhiteLabel)
	}
}

func TestAllFeaturesFree_KeylessOpensEveryGate(t *testing.T) {
	m := newKeyless(t)
	m.SetAllFeaturesFree(true)

	if !m.AllFeaturesFree() {
		t.Fatal("AllFeaturesFree() = false after SetAllFeaturesFree(true)")
	}
	assertEverythingOpen(t, m)
	// The tier stays honest: no key is loaded, so it is still "free".
	if m.Tier() != license.TierFree {
		t.Errorf("Tier() = %q, want %q (the policy must not invent a key)", m.Tier(), license.TierFree)
	}
	if !m.Valid() {
		t.Error("a keyless manager must stay valid")
	}
}

// New on its own must keep enforcing, so the existing tier tests still test a
// live path and switching the policy off would really restore the tiers.
func TestAllFeaturesFree_NewAloneStillEnforces(t *testing.T) {
	m := newKeyless(t)
	if m.AllFeaturesFree() {
		t.Fatal("New must not enable the policy itself; cmd/pulse opts in explicitly")
	}
	if len(refusedGates(m)) == 0 {
		t.Fatal("a keyless manager without the policy must still enforce Free-tier gates")
	}
}

// A paid key must never unlock less than having no key at all.
func TestAllFeaturesFree_PaidKeyGetsEverythingToo(t *testing.T) {
	m := newMgr(t, map[string]interface{}{
		"tier": "pro", "data_api": true, "max_nodes": 10, "retention_days": 90,
	})
	m.SetAllFeaturesFree(true)
	assertEverythingOpen(t, m)
	if m.Tier() != license.TierPro {
		t.Errorf("Tier() = %q, want %q (informational tier kept)", m.Tier(), license.TierPro)
	}
}

// The policy is per manager and must survive everything that rewrites the
// manager's tier state: lazy expiry, Refresh to no key, Refresh to a new key.
func TestAllFeaturesFree_SurvivesExpiryAndRefresh(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)
	expired := time.Now().Add(-time.Hour).UnixMilli()
	m, err := license.New(kf.signKey(t, map[string]interface{}{
		"tier": "pro", "data_api": true, "expires_at": expired,
	}), "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	m.SetAllFeaturesFree(true)

	if m.Valid() {
		t.Error("an expired key must still report invalid — the policy must not hide expiry")
	}
	assertEverythingOpen(t, m)

	if err := m.Refresh(context.Background(), ""); err != nil {
		t.Fatalf("Refresh to no key: %v", err)
	}
	assertEverythingOpen(t, m)

	if err := m.Refresh(context.Background(), kf.signKey(t, map[string]interface{}{"tier": "business"})); err != nil {
		t.Fatalf("Refresh to a business key: %v", err)
	}
	assertEverythingOpen(t, m)
}

func TestAllFeaturesFree_TurningItOffRestoresEnforcement(t *testing.T) {
	m := newKeyless(t)
	m.SetAllFeaturesFree(true)
	m.SetAllFeaturesFree(false)

	if len(refusedGates(m)) == 0 {
		t.Fatal("switching the policy off must restore Free-tier enforcement")
	}
	if got := m.Entitlements().MaxNodes; got != 1 {
		t.Errorf("Free-tier MaxNodes = %d after switching off, want 1", got)
	}
}

// Gate checks run on request goroutines while the flag could be read; run under
// -race to prove the flag is synchronised.
func TestAllFeaturesFree_ConcurrentToggleAndChecks(t *testing.T) {
	m := newKeyless(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(on bool) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				m.SetAllFeaturesFree(on)
			}
		}(i%2 == 0)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = m.CheckProbes()
				_ = m.Entitlements()
				_ = m.AllFeaturesFree()
			}
		}()
	}
	wg.Wait()
}
