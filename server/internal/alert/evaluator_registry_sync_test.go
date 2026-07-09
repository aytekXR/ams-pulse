// Package alert_test — registry sync from meta store (P1 registry fix).
//
// TDD: tests are written BEFORE the implementation. They fail (Send never called)
// against the current evaluator (empty registry, never populated from store),
// then go green once evaluate() syncs the registry on every tick.
//
// Coverage:
//   - Main: channel stored in meta store, never Register()ed → Send() is attempted.
//   - Update: channel URL updated in store → next tick delivers to new URL.
//   - Delete: channel removed from store → no further delivery, no panic.
//   - Corrupt: undecryptable/unknown-type row skipped, other channels still delivered.
package alert_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// httpSink returns an httptest.Server that records POST requests and always
// responds 200 OK. The returned *int64 is the atomic request counter.
func httpSink(t *testing.T) (*httptest.Server, *int64) {
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

// createWebhookInStore inserts a webhook channel into the meta store only —
// it is NOT registered in any evaluator registry. This replicates the production
// path where a user creates a channel via the API; the API writes to the store
// but (before P1 fix) the evaluator never learns about it.
func createWebhookInStore(t *testing.T, s *meta.Store, webhookURL string) meta.AlertChannelRow {
	t.Helper()
	ctx := context.Background()
	row := meta.AlertChannelRow{
		Type:         "webhook",
		Name:         fmt.Sprintf("sync-test-webhook-%d", time.Now().UnixNano()),
		ConfigPublic: fmt.Sprintf(`{"webhook_url":%q}`, webhookURL),
		ConfigEnc:    "", // webhook_url is not a secret field
	}
	created, err := s.CreateAlertChannel(ctx, row)
	if err != nil {
		t.Fatalf("createWebhookInStore: %v", err)
	}
	return created
}

// createOfflineRuleWithChannels creates a stream_offline rule referencing the
// given channel IDs. WindowS=0 fires on the very first matching tick.
func createOfflineRuleWithChannels(t *testing.T, s *meta.Store, channelIDs ...string) meta.AlertRuleRow {
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
		Name:               fmt.Sprintf("sync-rule-%d", time.Now().UnixNano()),
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            0,
		ScopeJSON:          `{"stream_id":"sync-test-stream"}`,
		Severity:           "critical",
		CooldownS:          1,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         chJSON,
	}
	created, err := s.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("createOfflineRuleWithChannels: %v", err)
	}
	return created
}

// offlineSnapForSync returns a snapshot that has NO active streams, so the
// stream_offline rule for "sync-test-stream" fires immediately.
func offlineSnapForSync() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	}
}

// newSyncEvaluator builds an Evaluator with an EMPTY registry (no channels
// pre-registered). This is the key difference from newRetryEvaluator — the
// evaluator must populate its registry from the store on each tick.
func newSyncEvaluator(t *testing.T, s *meta.Store, live domain.LiveProvider, clock alert.Clock, cfg alert.Config) *alert.Evaluator {
	t.Helper()
	reg := channels.NewRegistry() // intentionally empty — tests the sync path
	ev := alert.New(cfg, live, s, reg, clock, nil)
	return ev
}

// fastSyncCfg returns a Config with tiny retry delays so tests do not sleep long.
func fastSyncCfg() alert.Config {
	return alert.Config{
		TickInterval:     5 * time.Second,
		RetryBaseDelay:   1 * time.Millisecond,
		RetryCap:         5 * time.Millisecond,
		RetryMaxAttempts: 0, // single attempt — no retries needed for success tests
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestRegistrySync_ChannelInStore_SendAttempted is the primary RED→GREEN test.
//
// A webhook channel exists in the meta store only (never Register()ed).
// A rule references it. The evaluator ticks.
//
//   - RED (before fix): registry is empty → registry.Get returns false → no Send call.
//     The httptest server records 0 requests → test FAILS.
//   - GREEN (after fix): evaluate() syncs registry from store → webhook channel built →
//     Send() called → httptest server records ≥1 request → test PASSES.
func TestRegistrySync_ChannelInStore_SendAttempted(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	sink, count := httpSink(t)
	ch := createWebhookInStore(t, store, sink.URL)
	createOfflineRuleWithChannels(t, store, ch.ID)

	ev := newSyncEvaluator(t, store, live, clock, fastSyncCfg())
	live.setSnap(offlineSnapForSync())

	ctx := context.Background()
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	ev.Stop() // wait for delivery goroutines

	got := atomic.LoadInt64(count)
	if got < 1 {
		t.Errorf("expected ≥1 POST to webhook sink, got %d; "+
			"evaluator did not sync registry from store (registry gap bug)", got)
	} else {
		t.Logf("PASS: httptest sink received %d POST(s) — registry sync works", got)
	}
}

// TestRegistrySync_UpdatePropagates verifies that when a channel's webhook URL
// is updated in the meta store, the next evaluator tick delivers to the NEW URL.
//
// Uses two fresh evaluators (one per tick) to avoid cooldown state complexity.
func TestRegistrySync_UpdatePropagates(t *testing.T) {
	store := openTestStore(t)

	sinkA, countA := httpSink(t)
	sinkB, countB := httpSink(t)

	ch := createWebhookInStore(t, store, sinkA.URL)

	// Tick 1: rule fires → deliver to sinkA.
	{
		live := newFakeLive()
		clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		createOfflineRuleWithChannels(t, store, ch.ID)
		ev := newSyncEvaluator(t, store, live, clock, fastSyncCfg())
		live.setSnap(offlineSnapForSync())
		clock.Advance(5 * time.Second)
		ev.TickOnce(context.Background())
		ev.Stop()
		if got := atomic.LoadInt64(countA); got < 1 {
			t.Fatalf("tick1: expected sinkA to receive ≥1 POST, got %d (registry sync bug)", got)
		}
		t.Logf("tick1 OK: sinkA=%d", atomic.LoadInt64(countA))
	}

	// Update channel URL to sinkB.
	ctx := context.Background()
	updated, err := store.GetAlertChannel(ctx, ch.ID)
	if err != nil || updated == nil {
		t.Fatalf("GetAlertChannel: %v", err)
	}
	updated.ConfigPublic = fmt.Sprintf(`{"webhook_url":%q}`, sinkB.URL)
	if err := store.UpdateAlertChannel(ctx, *updated); err != nil {
		t.Fatalf("UpdateAlertChannel: %v", err)
	}

	// Tick 2 (fresh evaluator, fresh state): rule fires again → deliver to sinkB.
	{
		live := newFakeLive()
		clock := alert.NewFakeClock(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)) // different time
		ev := newSyncEvaluator(t, store, live, clock, fastSyncCfg())
		live.setSnap(offlineSnapForSync())
		clock.Advance(5 * time.Second)
		ev.TickOnce(context.Background())
		ev.Stop()
		if got := atomic.LoadInt64(countB); got < 1 {
			t.Fatalf("tick2: expected sinkB to receive ≥1 POST after URL update, got %d", got)
		}
		t.Logf("tick2 OK: sinkB=%d — updated URL propagated", atomic.LoadInt64(countB))
	}
}

// TestRegistrySync_DeleteStopsDelivery verifies that when a channel is removed
// from the meta store, the evaluator no longer delivers to it.
//
// Uses two fresh evaluators to avoid cooldown state complexity.
func TestRegistrySync_DeleteStopsDelivery(t *testing.T) {
	store := openTestStore(t)

	sink, count := httpSink(t)
	ch := createWebhookInStore(t, store, sink.URL)

	// Tick 1: channel present → delivery attempted.
	{
		live := newFakeLive()
		clock := alert.NewFakeClock(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
		createOfflineRuleWithChannels(t, store, ch.ID)
		ev := newSyncEvaluator(t, store, live, clock, fastSyncCfg())
		live.setSnap(offlineSnapForSync())
		clock.Advance(5 * time.Second)
		ev.TickOnce(context.Background())
		ev.Stop()
		if got := atomic.LoadInt64(count); got < 1 {
			t.Fatalf("tick1: expected ≥1 POST before deletion, got %d (registry sync bug)", got)
		}
		t.Logf("tick1 OK: sink=%d (channel existed)", atomic.LoadInt64(count))
	}

	// Delete the channel from the store.
	if err := store.DeleteAlertChannel(context.Background(), ch.ID); err != nil {
		t.Fatalf("DeleteAlertChannel: %v", err)
	}
	afterTick1 := atomic.LoadInt64(count)

	// Tick 2 (fresh evaluator, channel gone from store): no further delivery.
	{
		live := newFakeLive()
		clock := alert.NewFakeClock(time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC))
		ev := newSyncEvaluator(t, store, live, clock, fastSyncCfg())
		live.setSnap(offlineSnapForSync())
		clock.Advance(5 * time.Second)
		ev.TickOnce(context.Background())
		ev.Stop()
		if got := atomic.LoadInt64(count); got != afterTick1 {
			t.Errorf("tick2: sink received %d additional POST(s) after channel deletion; "+
				"expected 0 (channel was removed from store)", got-afterTick1)
		} else {
			t.Logf("tick2 OK: sink count unchanged at %d — deleted channel not delivered", got)
		}
	}
}

// TestRegistrySync_CorruptRowSkipped_OtherDelivered verifies that a channel
// row with an undecryptable ConfigEnc is silently skipped and does NOT prevent
// other valid channels from receiving their delivery.
func TestRegistrySync_CorruptRowSkipped_OtherDelivered(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// Good channel: normal webhook with an httptest sink.
	goodSink, goodCount := httpSink(t)
	goodCh := createWebhookInStore(t, store, goodSink.URL)

	// Corrupt channel: valid row structure but ConfigEnc is garbage (cannot decrypt).
	ctx := context.Background()
	corruptRow := meta.AlertChannelRow{
		Type:         "webhook",
		Name:         "corrupt-channel",
		ConfigPublic: `{}`,
		ConfigEnc:    "not-valid-encrypted-data", // will fail Decrypt
	}
	corruptCh, err := store.CreateAlertChannel(ctx, corruptRow)
	if err != nil {
		t.Fatalf("CreateAlertChannel (corrupt): %v", err)
	}

	// Rule referencing both channels.
	createOfflineRuleWithChannels(t, store, goodCh.ID, corruptCh.ID)

	ev := newSyncEvaluator(t, store, live, clock, fastSyncCfg())
	live.setSnap(offlineSnapForSync())
	clock.Advance(5 * time.Second)

	// Must not panic.
	ev.TickOnce(ctx)
	ev.Stop()

	// Good channel must have received at least one delivery.
	if got := atomic.LoadInt64(goodCount); got < 1 {
		t.Errorf("good channel: expected ≥1 POST, got %d; "+
			"corrupt row must not block valid channel delivery", got)
	} else {
		t.Logf("PASS corrupt-skip: goodSink=%d, no panic", got)
	}
	// The corrupt channel was skipped — no panic, test did not hang.
}
