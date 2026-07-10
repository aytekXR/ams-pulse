// Package alert_test — delivery retry + delivery_failure recording (P1 order 2).
//
// TDD: tests are written BEFORE the implementation.  They compile and fail
// (compile error or test failure) on the unmodified evaluator, then go green
// after the implementation is in place.
package alert_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── fakeFailChannel ──────────────────────────────────────────────────────────

// fakeFailChannel is a programmable Channel implementation for retry tests.
type fakeFailChannel struct {
	mu        sync.Mutex
	calls     int
	callTimes []time.Time
	failFirst int           // number of calls that return an error (0 = always succeed)
	failAll   bool          // if true, every Send returns an error
	sendDelay time.Duration // per-Send sleep; lets tests make synchronous delivery measurably slow
}

func (f *fakeFailChannel) Name() string { return "fake-fail" }

func (f *fakeFailChannel) Send(_ context.Context, _ []byte) error {
	f.mu.Lock()
	delay := f.sendDelay
	f.calls++
	n := f.calls
	f.callTimes = append(f.callTimes, time.Now())
	fail := f.failAll || n <= f.failFirst
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay) // outside the mutex so Calls()/CallTimes() never block on it
	}
	if fail {
		return fmt.Errorf("simulated send failure #%d", n)
	}
	return nil
}

func (f *fakeFailChannel) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeFailChannel) CallTimes() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]time.Time, len(f.callTimes))
	copy(cp, f.callTimes)
	return cp
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// newRetryEvaluator builds an Evaluator with the given channel registered and
// the supplied config. Use tiny base-delay / cap values so tests don't sleep.
func newRetryEvaluator(
	t *testing.T,
	store *meta.Store,
	live domain.LiveProvider,
	clock alert.Clock,
	channelID string,
	ch channels.Channel,
	cfg alert.Config,
) *alert.Evaluator {
	t.Helper()
	reg := channels.NewRegistry()
	reg.Register(channelID, ch)
	ev := alert.New(cfg, live, store, reg, clock, nil)
	return ev
}

// createOfflineRuleForChannel creates a stream_offline rule with WindowS=0
// (fires on the very first tick the condition is met) and ChannelIDs pointing
// at the given channel.
func createOfflineRuleForChannel(t *testing.T, store *meta.Store, channelID string) meta.AlertRuleRow {
	t.Helper()
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               "retry-test-rule",
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            0, // fires immediately on first matching tick
		ScopeJSON:          `{"stream_id": "retry-test-stream"}`,
		Severity:           "critical",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         fmt.Sprintf(`[%q]`, channelID),
	}
	created, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	return created
}

// offlineSnapForRetry returns a snapshot where "retry-test-stream" is absent
// (offline), which triggers the stream_offline rule.
func offlineSnapForRetry() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	}
}

// fastRetryCfg returns a Config with tiny retry delays for use in unit tests.
func fastRetryCfg() alert.Config {
	return alert.Config{
		TickInterval:     5 * time.Second,
		RetryBaseDelay:   1 * time.Millisecond,
		RetryCap:         10 * time.Millisecond,
		RetryMaxAttempts: 3,
	}
}

// ─── 5a: fail-twice-then-succeed ─────────────────────────────────────────────

// TestDelivery_RetrySucceeds verifies that when a channel fails on the first
// two attempts and succeeds on the third:
//   - Send is called exactly 3 times.
//   - No delivery_failure row is written to the history store.
func TestDelivery_RetrySucceeds(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	const chID = "fail2-then-ok"
	fake := &fakeFailChannel{failFirst: 2} // fails calls 1 and 2, succeeds on call 3
	ev := newRetryEvaluator(t, store, live, clock, chID, fake, fastRetryCfg())
	createOfflineRuleForChannel(t, store, chID)

	ctx := context.Background()
	live.setSnap(offlineSnapForRetry())

	// WindowS=0 → fires on the first tick.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	// Wait for all delivery goroutines to finish.
	ev.Stop()

	// Assert Send was called exactly 3 times (2 failures + 1 success).
	if got := fake.Calls(); got != 3 {
		t.Errorf("expected 3 Send calls (2 fail + 1 succeed), got %d", got)
	}

	// Assert no delivery_failure row was written.
	hist, err := store.ListAlertHistory(ctx, "", "delivery_failure", 0, 0, 10)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) > 0 {
		t.Errorf("expected 0 delivery_failure rows (delivery succeeded on 3rd try), got %d", len(hist))
	}
	t.Logf("PASS 5a: fail-twice-then-succeed → 3 Send calls, 0 delivery_failure rows")
}

// ─── 5b: always-fail → delivery_failure row ──────────────────────────────────

// TestDelivery_AllFail_RecordsDeliveryFailure verifies that when all 4 Send
// attempts (initial + 3 retries) fail:
//   - TickOnce/deliver() returns in <100ms (delivery is async / non-blocking).
//   - Exactly ONE delivery_failure history row is written.
//   - The row carries state="delivery_failure", the rule's metric, and the
//     channel_id embedded in the scope JSON.
func TestDelivery_AllFail_RecordsDeliveryFailure(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	const chID = "always-fail-ch"
	// sendDelay makes SYNCHRONOUS delivery measurably slow: 4 attempts × 500ms ≥ 2s,
	// so the 1s budget below discriminates sync vs async even under whole-suite -race
	// CPU contention (worst observed TickOnce overhead ~110ms — D-075 gate finding; the
	// old instant-Send fake + 100ms budget measured only scheduler noise, D-042 class).
	fake := &fakeFailChannel{failAll: true, sendDelay: 500 * time.Millisecond}
	ev := newRetryEvaluator(t, store, live, clock, chID, fake, fastRetryCfg())
	createOfflineRuleForChannel(t, store, chID)

	ctx := context.Background()
	live.setSnap(offlineSnapForRetry())

	// Measure that TickOnce returns promptly — delivery must be asynchronous.
	start := time.Now()
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("TickOnce took %v; expected <1s — delivery should be non-blocking (sync would take ≥2s)", elapsed)
	}
	t.Logf("TickOnce returned in %v (async check passed)", elapsed)

	// Wait for all delivery goroutines to finish (they exhaust retries with tiny delays).
	ev.Stop()

	// Assert exactly 1 delivery_failure row.
	hist, err := store.ListAlertHistory(ctx, "", "delivery_failure", 0, 0, 10)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 delivery_failure row, got %d", len(hist))
	}

	row := hist[0]
	if row.State != "delivery_failure" {
		t.Errorf("expected state=delivery_failure, got %q", row.State)
	}
	if row.Metric != "stream_offline" {
		t.Errorf("expected metric=stream_offline, got %q", row.Metric)
	}

	// channel_id must be embedded in the scope JSON.
	if row.ScopeJSON == "" {
		t.Error("scope JSON is empty; expected {channel_id: ...} to be present")
	}

	// Send was called 4 times (1 initial + 3 retries).
	if got := fake.Calls(); got != 4 {
		t.Errorf("expected 4 Send calls (1 initial + 3 retries), got %d", got)
	}

	t.Logf("PASS 5b: all-fail → 1 delivery_failure row, state=%s, metric=%s, scope=%s",
		row.State, row.Metric, row.ScopeJSON)
}

// ─── 5c: shutdown does not hang ──────────────────────────────────────────────

// TestDelivery_Shutdown_NoHang verifies that cancelling the evaluator context
// while a delivery goroutine is mid-retry causes Stop() to return within 2 s.
// This guards against goroutine leaks when the evaluator is shut down during
// active retries.
func TestDelivery_Shutdown_NoHang(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	const chID = "shutdown-ch"
	fake := &fakeFailChannel{failAll: true}

	// Larger base delay so the goroutine is sleeping when we cancel.
	cfg := alert.Config{
		TickInterval:     5 * time.Second,
		RetryBaseDelay:   200 * time.Millisecond, // noticeable sleep between retries
		RetryCap:         5 * time.Second,
		RetryMaxAttempts: 3,
	}
	ev := newRetryEvaluator(t, store, live, clock, chID, fake, cfg)
	createOfflineRuleForChannel(t, store, chID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	live.setSnap(offlineSnapForRetry())
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx) // spawns delivery goroutine(s) that will retry with 200ms backoff

	// Cancel context — delivery goroutines should abort their pending sleep.
	cancel()

	// Stop() must return within 2 s (goroutines must exit promptly on ctx cancel).
	done := make(chan struct{})
	go func() {
		ev.Stop()
		close(done)
	}()

	select {
	case <-done:
		t.Log("PASS 5c: Stop() returned promptly after context cancel (no goroutine leak)")
	case <-time.After(2 * time.Second):
		t.Fatal("FAIL 5c: Stop() did not return within 2 s — delivery goroutine leak detected")
	}
}

// ─── 5d: backoff shape ────────────────────────────────────────────────────────

// TestDelivery_BackoffShape verifies that the inter-attempt delays match the
// expected exponential-backoff-with-jitter formula:
//
//	delay[n] = min(base * 2^(n-1), cap) * jitter   where jitter ∈ [0.8, 1.2]
//
// The test uses an always-failing channel and injects a tiny base/cap so the
// total wall-clock time is < 500 ms regardless of backoff shape.
func TestDelivery_BackoffShape(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	const chID = "backoff-shape-ch"
	fake := &fakeFailChannel{failAll: true}

	base := 20 * time.Millisecond
	cap_ := 200 * time.Millisecond
	cfg := alert.Config{
		TickInterval:     5 * time.Second,
		RetryBaseDelay:   base,
		RetryCap:         cap_,
		RetryMaxAttempts: 3,
	}
	ev := newRetryEvaluator(t, store, live, clock, chID, fake, cfg)
	createOfflineRuleForChannel(t, store, chID)

	ctx := context.Background()
	live.setSnap(offlineSnapForRetry())

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	// Wait for all retries to finish.
	ev.Stop()

	times := fake.CallTimes()
	// 1 initial + 3 retries = 4 total calls.
	if len(times) != 4 {
		t.Fatalf("expected 4 Send calls (1 initial + 3 retries), got %d", len(times))
	}

	// Check inter-attempt delay for each retry n (1-indexed).
	// Expected raw delay before retry n: min(base * 2^(n-1), cap).
	// Jitter range: [0.8, 1.2] → allow generous [0.5, 2.0] window for OS scheduling.
	for n := 1; n <= 3; n++ {
		elapsed := times[n].Sub(times[n-1])
		rawDelay := time.Duration(float64(base) * math.Pow(2, float64(n-1)))
		if rawDelay > cap_ {
			rawDelay = cap_
		}
		minAllowed := time.Duration(float64(rawDelay) * 0.50)                     // generous lower bound
		maxAllowed := time.Duration(float64(rawDelay)*2.00) + 50*time.Millisecond // upper + OS slack

		if elapsed < minAllowed || elapsed > maxAllowed {
			t.Errorf("retry %d: elapsed=%v, expected [%v, %v] (rawDelay=%v)",
				n, elapsed, minAllowed, maxAllowed, rawDelay)
		} else {
			t.Logf("retry %d: elapsed=%v (rawDelay=%v) ✓", n, elapsed, rawDelay)
		}
	}
	t.Logf("PASS 5d: backoff shape verified for 3 retries (base=%v, cap=%v)", base, cap_)
}
