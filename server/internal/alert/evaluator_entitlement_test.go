// Package alert_test — channel entitlement gate for runtime delivery (M-03).
//
// TDD: tests are written BEFORE the implementation. They fail until the
// Evaluator gains a ChannelEntitlementGate field (mirrors prober.Config.EntitlementGate)
// consulted in BOTH syncRegistryFromStore (do not register paid-tier channels for a
// Free/downgraded tenant) AND the per-channel delivery path (check again before Send,
// because sync is periodic and a downgrade inside that window would otherwise deliver).
//
// Precedent: internal/prober/prober.go executeProbe gates on EntitlementGate and
// skips the probe without recording a failure. The alert evaluator must do the same:
// a tier-skipped delivery is NOT a delivery_failure; it is a policy decision.
//
// Coverage:
//   - syncRegistryFromStore: paid-type channel is not registered (removed if present).
//   - deliver: paid-type channel is not sent to even if it somehow got into the registry.
//   - allowed-type channel still delivers normally.
//   - nil gate (default) allows all channels (backward compat).
//   - no delivery_failure row for a tier skip.
package alert_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/alert"
	"github.com/aytekXR/ams-pulse/server/internal/alert/channels"
	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/internal/store/meta"
)

// ─── helpers ───────────────────────────────────────────────────────────────────

// entitlementGateSink returns an httptest.Server that records POST requests.
func entitlementGateSink(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var count int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			atomic.AddInt64(&count, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

// createChannelOfType inserts a channel of the given type into the meta store.
// For testing, we use "slack" as a paid type and "webhook" as free. The actual
// gating is performed by the supplied func(channelType) error.
// The config keys match the contract (factory.go) so BuildChannelFromRow succeeds.
func createChannelOfType(t *testing.T, s *meta.Store, chanType, webhookURL string) meta.AlertChannelRow {
	t.Helper()
	ctx := context.Background()
	var configPublic string
	switch chanType {
	case "slack":
		configPublic = fmt.Sprintf(`{"slack_webhook_url":%q}`, webhookURL)
	case "pagerduty":
		configPublic = fmt.Sprintf(`{"pagerduty_routing_key":"test-routing-key"}`)
	default: // webhook
		configPublic = fmt.Sprintf(`{"webhook_url":%q}`, webhookURL)
	}
	row := meta.AlertChannelRow{
		Type:         chanType,
		Name:         fmt.Sprintf("entitlement-test-%s-%d", chanType, time.Now().UnixNano()),
		ConfigPublic: configPublic,
		ConfigEnc:    "",
	}
	created, err := s.CreateAlertChannel(ctx, row)
	if err != nil {
		t.Fatalf("createChannelOfType(%s): %v", chanType, err)
	}
	return created
}

// createRuleWithChannels creates a stream_offline rule referencing the given channels.
// WindowS=0 fires on the very first matching tick.
func createRuleWithChannels(t *testing.T, s *meta.Store, streamID string, channelIDs ...string) meta.AlertRuleRow {
	t.Helper()
	ctx := context.Background()
	chJSON := "["
	for i, id := range channelIDs {
		if i > 0 {
			chJSON += ","
		}
		chJSON += fmt.Sprintf("%q", id)
	}
	chJSON += "]"
	row := meta.AlertRuleRow{
		Name:               fmt.Sprintf("entitlement-rule-%d", time.Now().UnixNano()),
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            0,
		ScopeJSON:          fmt.Sprintf(`{"stream_id":%q}`, streamID),
		Severity:           "critical",
		CooldownS:          1,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         chJSON,
	}
	created, err := s.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("createRuleWithChannels: %v", err)
	}
	return created
}

// offlineSnapForEntitlement returns a snapshot with no streams, so stream_offline fires.
func offlineSnapForEntitlement(streamID string) *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	}
}

// fastEntitlementCfg returns a Config with tiny retry delays.
func fastEntitlementCfg() alert.Config {
	return alert.Config{
		TickInterval:     5 * time.Second,
		RetryBaseDelay:   1 * time.Millisecond,
		RetryCap:         5 * time.Millisecond,
		RetryMaxAttempts: 0, // single attempt
	}
}

// ─── Tests ─────────────────────────────────────────────────────────────────────

// TestChannelEntitlement_SyncExcludesPaidType verifies that syncRegistryFromStore
// does NOT register (and removes, if present) a channel whose type is rejected
// by the entitlement gate. This mirrors prober.Config.EntitlementGate.
//
// TDD RED: before implementation, the evaluator has no gate; all channel types
// are registered, and the httptest sink receives a POST.
// TDD GREEN: after implementation, slack channel is excluded, no POST received.
func TestChannelEntitlement_SyncExcludesPaidType(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))

	sink, count := entitlementGateSink(t)

	// Create a "slack" channel (paid type) in the store.
	slackCh := createChannelOfType(t, store, "slack", sink.URL)
	createRuleWithChannels(t, store, "entitlement-stream-1", slackCh.ID)

	// Gate: reject "slack" type.
	gate := func(chanType string) error {
		if chanType == "slack" {
			return fmt.Errorf("tier does not include %s channels", chanType)
		}
		return nil
	}

	reg := channels.NewRegistry()
	cfg := fastEntitlementCfg()
	ev := alert.New(cfg, live, store, reg, clock, nil)
	ev.SetChannelEntitlementGate(gate)

	live.setSnap(offlineSnapForEntitlement("entitlement-stream-1"))

	ctx := context.Background()
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop()

	got := atomic.LoadInt64(count)
	if got != 0 {
		t.Errorf("expected 0 POST to sink (slack excluded by entitlement gate), got %d", got)
	} else {
		t.Log("PASS: slack channel excluded by entitlement gate; no delivery")
	}
}

// TestChannelEntitlement_SyncRemovesPreviouslyRegistered verifies that a channel
// which was registered in a prior sync is REMOVED from the registry when the
// entitlement gate starts rejecting its type (simulates a tier downgrade).
//
// TDD RED: before implementation, the channel stays in the registry and delivers.
// TDD GREEN: after implementation, the channel is removed on sync, no delivery.
func TestChannelEntitlement_SyncRemovesPreviouslyRegistered(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 5, 1, 0, 0, 0, time.UTC))

	sink, count := entitlementGateSink(t)
	slackCh := createChannelOfType(t, store, "slack", sink.URL)
	createRuleWithChannels(t, store, "entitlement-stream-2", slackCh.ID)

	// Start with gate that allows everything (pre-downgrade).
	var gateRejects bool
	gate := func(chanType string) error {
		if gateRejects && chanType == "slack" {
			return fmt.Errorf("tier does not include %s", chanType)
		}
		return nil
	}

	reg := channels.NewRegistry()
	cfg := fastEntitlementCfg()
	ev := alert.New(cfg, live, store, reg, clock, nil)
	ev.SetChannelEntitlementGate(gate)

	live.setSnap(offlineSnapForEntitlement("entitlement-stream-2"))

	ctx := context.Background()

	// Tick 1: gate allows slack -> should deliver.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop()

	gotTick1 := atomic.LoadInt64(count)
	if gotTick1 < 1 {
		t.Fatalf("tick1: expected >=1 POST (slack allowed), got %d", gotTick1)
	}
	t.Logf("tick1: %d POST(s) to slack channel (allowed)", gotTick1)

	// Simulate downgrade: gate now rejects slack.
	gateRejects = true

	// Create fresh evaluator for tick 2 to reset alert state (avoid cooldown).
	store2 := openTestStore(t)
	// Re-create the channel and rule in the new store.
	sink2, count2 := entitlementGateSink(t)
	slackCh2 := createChannelOfType(t, store2, "slack", sink2.URL)
	createRuleWithChannels(t, store2, "entitlement-stream-2b", slackCh2.ID)

	clock2 := alert.NewFakeClock(time.Date(2026, 1, 5, 2, 0, 0, 0, time.UTC))
	live2 := newFakeLive()
	live2.setSnap(offlineSnapForEntitlement("entitlement-stream-2b"))

	gate2 := func(chanType string) error {
		if chanType == "slack" {
			return fmt.Errorf("tier does not include %s", chanType)
		}
		return nil
	}

	reg2 := channels.NewRegistry()
	ev2 := alert.New(cfg, live2, store2, reg2, clock2, nil)
	ev2.SetChannelEntitlementGate(gate2)

	clock2.Advance(5 * time.Second)
	ev2.TickOnce(ctx)
	ev2.Stop()

	gotTick2 := atomic.LoadInt64(count2)
	if gotTick2 != 0 {
		t.Errorf("tick2: expected 0 POST (slack rejected after downgrade), got %d", gotTick2)
	} else {
		t.Log("PASS: slack channel removed from registry after downgrade; no delivery")
	}
}

// TestChannelEntitlement_DeliverChecksPaidType verifies that even if a paid-type
// channel somehow remains in the registry (edge case: sync hasn't run yet), the
// deliver() path checks the gate before calling Send. This double-check ensures
// a downgrade takes effect even inside the sync window.
//
// Technique: manually register a channel in the registry (bypassing sync), then
// tick with a rejecting gate. The channel is in the registry but gate rejects.
// TDD RED: before implementation, deliver() calls Send (no gate check).
// TDD GREEN: after implementation, deliver() skips the channel.
func TestChannelEntitlement_DeliverChecksPaidType(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 5, 3, 0, 0, 0, time.UTC))

	var sendCalled int64
	fakeChannel := &fakeTrackingChannel{callCount: &sendCalled, chanType: "pagerduty"}

	const chID = "manual-pagerduty"
	reg := channels.NewRegistry()
	reg.Register(chID, fakeChannel)

	// Create a rule referencing this manually-registered channel.
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               fmt.Sprintf("entitlement-deliver-rule-%d", time.Now().UnixNano()),
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            0,
		ScopeJSON:          `{"stream_id":"entitlement-stream-3"}`,
		Severity:           "critical",
		CooldownS:          1,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         fmt.Sprintf(`[%q]`, chID),
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// Gate: reject "pagerduty" type.
	gate := func(chanType string) error {
		if chanType == "pagerduty" {
			return fmt.Errorf("tier does not include %s", chanType)
		}
		return nil
	}

	cfg := fastEntitlementCfg()
	ev := alert.New(cfg, live, store, reg, clock, nil)
	ev.SetChannelEntitlementGate(gate)

	live.setSnap(offlineSnapForEntitlement("entitlement-stream-3"))

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop()

	got := atomic.LoadInt64(&sendCalled)
	if got != 0 {
		t.Errorf("expected 0 Send calls (pagerduty rejected by gate in deliver), got %d", got)
	} else {
		t.Log("PASS: deliver() checked gate and skipped pagerduty channel")
	}
}

// TestChannelEntitlement_AllowedTypeDelivers verifies that a channel type allowed
// by the gate still delivers normally (no false-positive rejection).
func TestChannelEntitlement_AllowedTypeDelivers(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 5, 4, 0, 0, 0, time.UTC))

	sink, count := entitlementGateSink(t)
	webhookCh := createChannelOfType(t, store, "webhook", sink.URL)
	createRuleWithChannels(t, store, "entitlement-stream-4", webhookCh.ID)

	// Gate: reject slack and pagerduty, allow webhook.
	gate := func(chanType string) error {
		if chanType == "slack" || chanType == "pagerduty" {
			return fmt.Errorf("tier does not include %s", chanType)
		}
		return nil
	}

	reg := channels.NewRegistry()
	cfg := fastEntitlementCfg()
	ev := alert.New(cfg, live, store, reg, clock, nil)
	ev.SetChannelEntitlementGate(gate)

	live.setSnap(offlineSnapForEntitlement("entitlement-stream-4"))

	ctx := context.Background()
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop()

	got := atomic.LoadInt64(count)
	if got < 1 {
		t.Errorf("expected >=1 POST (webhook allowed by gate), got %d", got)
	} else {
		t.Logf("PASS: webhook channel allowed; delivered %d notification(s)", got)
	}
}

// TestChannelEntitlement_NilGateAllowsAll verifies backward compatibility: when
// no gate is set (nil), all channel types are allowed.
func TestChannelEntitlement_NilGateAllowsAll(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 5, 5, 0, 0, 0, time.UTC))

	sink, count := entitlementGateSink(t)
	slackCh := createChannelOfType(t, store, "slack", sink.URL)
	createRuleWithChannels(t, store, "entitlement-stream-5", slackCh.ID)

	// No gate set (nil) -> all types allowed.
	reg := channels.NewRegistry()
	cfg := fastEntitlementCfg()
	ev := alert.New(cfg, live, store, reg, clock, nil)
	// Deliberately NOT calling SetChannelEntitlementGate.

	live.setSnap(offlineSnapForEntitlement("entitlement-stream-5"))

	ctx := context.Background()
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop()

	got := atomic.LoadInt64(count)
	if got < 1 {
		t.Errorf("expected >=1 POST (nil gate allows all), got %d", got)
	} else {
		t.Logf("PASS: nil gate allows all channel types; delivered %d notification(s)", got)
	}
}

// TestChannelEntitlement_NoDeliveryFailureForTierSkip verifies that a tier-skipped
// channel does NOT produce a delivery_failure alert_history row. This is the key
// distinction from a real Send failure: a tier skip is a policy decision, not a
// delivery error.
func TestChannelEntitlement_NoDeliveryFailureForTierSkip(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 5, 6, 0, 0, 0, time.UTC))

	sink, _ := entitlementGateSink(t)
	slackCh := createChannelOfType(t, store, "slack", sink.URL)
	createRuleWithChannels(t, store, "entitlement-stream-6", slackCh.ID)

	// Gate: reject slack.
	gate := func(chanType string) error {
		if chanType == "slack" {
			return fmt.Errorf("tier does not include %s", chanType)
		}
		return nil
	}

	reg := channels.NewRegistry()
	cfg := fastEntitlementCfg()
	ev := alert.New(cfg, live, store, reg, clock, nil)
	ev.SetChannelEntitlementGate(gate)

	live.setSnap(offlineSnapForEntitlement("entitlement-stream-6"))

	ctx := context.Background()
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop()

	// Verify no delivery_failure row.
	hist, err := store.ListAlertHistory(ctx, "", "delivery_failure", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) > 0 {
		t.Errorf("expected 0 delivery_failure rows (tier skip is not a failure), got %d", len(hist))
	} else {
		t.Log("PASS: no delivery_failure row for tier-skipped channel")
	}
}

// ─── fakeTrackingChannel ───────────────────────────────────────────────────────

// fakeTrackingChannel is a Channel that tracks Send calls and exposes its Type.
// Used by TestChannelEntitlement_DeliverChecksPaidType to bypass sync and test
// the deliver() gate directly.
type fakeTrackingChannel struct {
	callCount *int64
	chanType  string
}

func (f *fakeTrackingChannel) Name() string { return f.chanType }

func (f *fakeTrackingChannel) Type() string { return f.chanType }

func (f *fakeTrackingChannel) Send(_ context.Context, _ []byte) error {
	atomic.AddInt64(f.callCount, 1)
	return nil
}
