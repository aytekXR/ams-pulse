// Package alert_test — S39 (D-101) license_expiry alert tests.
//
// Mirrors the cert_expiry tests: drive TickOnce with a license_expiry rule and a
// FakeLicenseChecker, assert firing via the notify sink. A rule is
// { metric: "license_expiry", operator: "lt", threshold: 14 }.
package alert_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// licenseExpiryRule builds an enabled license_expiry rule firing when < 14 days remain.
func licenseExpiryRule() meta.AlertRuleRow {
	return meta.AlertRuleRow{
		Name:               "license-expiry-rule",
		Metric:             "license_expiry",
		Operator:           "lt",
		Threshold:          14, // fire if < 14 days left
		WindowS:            0,  // immediate
		ScopeJSON:          `{}`,
		Severity:           "critical",
		CooldownS:          86400,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
}

// runLicenseExpiry sets up an evaluator with the given checker and rule, ticks twice
// (window_s=0 → immediate), and returns the captured notifications.
func runLicenseExpiry(t *testing.T, checker alert.LicenseExpiryChecker) []map[string]any {
	t.Helper()
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

	clock := alert.NewFakeClock(time.Now())
	ev, _ := newTestEvaluator(t, store, live, clock)
	ev.SetLicenseChecker(checker)

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
	clock.Advance(1 * time.Second)
	ev.TickOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	out := make([]map[string]any, len(notifs))
	copy(out, notifs)
	return out
}

// TestLicenseExpiry_NearExpiry_Fires: 10 days left (< 14 threshold) → fires.
func TestLicenseExpiry_NearExpiry_Fires(t *testing.T) {
	notifs := runLicenseExpiry(t, alert.FakeLicenseChecker{Days: 10, HasExpiry: true})
	if len(notifs) == 0 {
		t.Fatal("expected a license_expiry notification when the licence expires in 10 days (< 14)")
	}
	if v, _ := notifs[0]["value"].(float64); v != 10 {
		t.Errorf("notification value: got %v, want 10", notifs[0]["value"])
	}
}

// TestLicenseExpiry_Safe_DoesNotFire: 90 days left → does not fire.
func TestLicenseExpiry_Safe_DoesNotFire(t *testing.T) {
	notifs := runLicenseExpiry(t, alert.FakeLicenseChecker{Days: 90, HasExpiry: true})
	if len(notifs) != 0 {
		t.Errorf("expected 0 notifications with 90 days left (threshold < 14), got %d", len(notifs))
	}
}

// TestLicenseExpiry_Perpetual_NoFire: a licence with NO expiry must never fire, even
// though Days=0 would satisfy the threshold — ok=false means "nothing to warn about".
// This is the non-vacuous guard: without the `if !ok { return nil }` check, a
// perpetual/free licence (Days 0) would false-alarm every tick.
func TestLicenseExpiry_Perpetual_NoFire(t *testing.T) {
	notifs := runLicenseExpiry(t, alert.FakeLicenseChecker{Days: 0, HasExpiry: false})
	if len(notifs) != 0 {
		t.Errorf("expected 0 notifications for a perpetual licence (no expiry), got %d", len(notifs))
	}
}
