// api_streak_test.go — D-087 TDD: API RTT measurement and consecutive-error streak.
//
// Rung 1 & 2 of the AMS early-warning detection ladder (ant-media/Ant-Media-Server#7926):
//
//	rung 1: ams_api_latency_ms node metric → Welford anomaly flag
//	rung 2: consecutive API-failure streak → node_degraded alert
//
// RED-first pins (before implementation they all fail):
//
//	Test 1: node_stats success includes api_latency_ms (positive float64)
//	Test 2: failure-streak event has api_unreachable=true, NO api_latency_ms key
//	Test 3: consec_api_errors increments on each consecutive failing poll cycle
//	Test 4: streak resets to 0.0 on first success after failures
//	Test 5: broadcast/app poll failures do NOT touch the streak counter
package restpoller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// drainNodeStats collects all node_stats events already in the sink.
func drainNodeStats(sink *mockEventSink) []domain.ServerEvent {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	var out []domain.ServerEvent
	for _, ev := range sink.events {
		if ev.Type == domain.EventNodeStats {
			out = append(out, ev)
		}
	}
	return out
}

// clearEvents removes all events from the sink.
func clearEvents(sink *mockEventSink) {
	sink.mu.Lock()
	sink.events = sink.events[:0]
	sink.mu.Unlock()
}

// waitNodeStats waits until at least n node_stats events with pred are collected.
// Returns collected events or nil on timeout.
func waitNodeStats(sink *mockEventSink, n int,
	pred func(domain.ServerEvent) bool,
	deadline time.Duration,
) []domain.ServerEvent {
	dl := time.NewTimer(deadline)
	defer dl.Stop()
	var got []domain.ServerEvent
	for {
		select {
		case <-dl.C:
			return got
		case <-sink.notify:
			sink.mu.Lock()
			for _, ev := range sink.events {
				if ev.Type == domain.EventNodeStats && pred(ev) {
					got = append(got, ev)
				}
			}
			sink.mu.Unlock()
			if len(got) >= n {
				return got
			}
		}
	}
}

// standaloneAMSServer returns a test server that simulates a standalone AMS node.
// sysStatsHandler controls what /rest/v2/system-status returns.
func standaloneAMSServer(t *testing.T, sysStatsHandler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{"applications": []any{}}) //nolint:errcheck
		case "/rest/v2/cluster/nodes":
			// Standalone AMS: 404 → client maps to nil,nil
			w.WriteHeader(http.StatusNotFound)
		case "/rest/v2/system-status":
			sysStatsHandler(w, r)
		case "/rest/v2/version":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// clusterAMSServer returns a test server that simulates a clustered AMS node.
// clusterNodesHandler controls what /rest/v2/cluster/nodes returns.
func clusterAMSServer(t *testing.T, clusterNodesHandler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{"applications": []any{}}) //nolint:errcheck
		case "/rest/v2/cluster/nodes":
			clusterNodesHandler(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// makePoller builds a Poller for the given server URL with a fast poll interval.
func makePoller(t *testing.T, serverURL string) (*restpoller.Poller, *mockEventSink) {
	t.Helper()
	client := amsclient.New(amsclient.Config{BaseURL: serverURL, Timeout: 2 * time.Second})
	sink := newMockSink()
	p := restpoller.New(restpoller.Config{
		NodeID:       "streak-test-node",
		PollInterval: 60 * time.Millisecond,
	}, client, sink, nil)
	return p, sink
}

// runPoller launches the poller in background and returns a cancel func.
func runPoller(p *restpoller.Poller) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go func() { _ = p.Run(ctx) }()
	return ctx, cancel
}

// ─── Test 1: RTT present on success ─────────────────────────────────────────

// TestAPIStreak_RTTPresentOnSuccess verifies that a successful standalone SystemStats
// call produces a node_stats event with api_latency_ms (positive float64) and
// consec_api_errors=0.0. D-087, rung 1 data feed.
func TestAPIStreak_RTTPresentOnSuccess(t *testing.T) {
	srv := standaloneAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"osName": "Linux"}) //nolint:errcheck
	})
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Wait for a node_stats event that is NOT a failure event (no api_unreachable).
	events := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		_, isFailure := ev.Data["api_unreachable"]
		return !isFailure
	}, 3*time.Second)

	if len(events) == 0 {
		t.Fatal("FAIL: no successful node_stats event received within 3s")
	}
	ev := events[0]

	// api_latency_ms must be present and positive.
	rtt, ok := ev.Data["api_latency_ms"].(float64)
	if !ok {
		t.Errorf("api_latency_ms key absent or wrong type in successful event; Data=%v", ev.Data)
	} else if rtt < 0 {
		t.Errorf("api_latency_ms = %v, want >= 0 (round-trip time)", rtt)
	} else {
		t.Logf("PASS: api_latency_ms = %.3f ms on success", rtt)
	}

	// consec_api_errors must be 0.0.
	errs, ok := ev.Data["consec_api_errors"].(float64)
	if !ok {
		t.Errorf("consec_api_errors key absent or wrong type on success; Data=%v", ev.Data)
	} else if errs != 0 {
		t.Errorf("consec_api_errors = %v, want 0.0 on first successful poll", errs)
	} else {
		t.Logf("PASS: consec_api_errors = 0 on success")
	}
}

// ─── Test 2: RTT absent on failure ──────────────────────────────────────────

// TestAPIStreak_RTTAbsentOnFailure verifies that a failed SystemStats call produces
// a failure-streak node_stats event with api_unreachable=true, consec_api_errors=1,
// and NO api_latency_ms key. D-087 contract: key-absent = not measured.
func TestAPIStreak_RTTAbsentOnFailure(t *testing.T) {
	srv := standaloneAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// Simulate API failure
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Wait for a failure-streak event.
	events := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		unreachable, _ := ev.Data["api_unreachable"].(bool)
		return unreachable
	}, 3*time.Second)

	if len(events) == 0 {
		t.Fatal("FAIL: no failure-streak node_stats event received within 3s")
	}
	ev := events[0]

	// api_unreachable must be true.
	if unreachable, _ := ev.Data["api_unreachable"].(bool); !unreachable {
		t.Errorf("api_unreachable = %v, want true on failure", ev.Data["api_unreachable"])
	}

	// api_latency_ms must be ABSENT on failure (D-075 key-absent semantics).
	if _, present := ev.Data["api_latency_ms"]; present {
		t.Errorf("api_latency_ms present on failure event (must be absent); value=%v", ev.Data["api_latency_ms"])
	} else {
		t.Logf("PASS: api_latency_ms absent on failure")
	}

	// consec_api_errors must be 1.0 (first failure).
	errs, ok := ev.Data["consec_api_errors"].(float64)
	if !ok {
		t.Errorf("consec_api_errors key absent or wrong type on failure; Data=%v", ev.Data)
	} else if errs < 1 {
		t.Errorf("consec_api_errors = %v, want >= 1 on first failure", errs)
	} else {
		t.Logf("PASS: consec_api_errors = %.0f on first failure", errs)
	}
}

// ─── Test 3: Streak increments on consecutive failures ───────────────────────

// TestAPIStreak_IncrementsOnConsecutiveFailures verifies that consec_api_errors
// increments on each consecutive failing poll cycle. D-087, rung 2 data feed.
func TestAPIStreak_IncrementsOnConsecutiveFailures(t *testing.T) {
	srv := standaloneAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Collect 3 consecutive failure events.
	var failEvents []domain.ServerEvent
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()

	for len(failEvents) < 3 {
		select {
		case <-sink.notify:
			sink.mu.Lock()
			for _, ev := range sink.events {
				if ev.Type == domain.EventNodeStats {
					if u, _ := ev.Data["api_unreachable"].(bool); u {
						// Avoid duplicates: check if we already have this event by consec count.
						n := len(failEvents)
						want := float64(n + 1)
						if c, _ := ev.Data["consec_api_errors"].(float64); c == want {
							failEvents = append(failEvents, ev)
						}
					}
				}
			}
			sink.mu.Unlock()
		case <-deadline.C:
			t.Fatalf("FAIL: only collected %d consecutive failure events (want 3) within 4s",
				len(failEvents))
		}
	}

	// Verify consec_api_errors = 1, 2, 3.
	for i, ev := range failEvents {
		want := float64(i + 1)
		got, _ := ev.Data["consec_api_errors"].(float64)
		if got != want {
			t.Errorf("failure event #%d: consec_api_errors = %v, want %v", i+1, got, want)
		} else {
			t.Logf("PASS: failure event #%d has consec_api_errors = %.0f", i+1, got)
		}
	}
}

// ─── Test 4: Streak resets on success ────────────────────────────────────────

// TestAPIStreak_ResetsOnSuccess verifies that after a streak of failures,
// the first successful poll emits consec_api_errors=0.0 and api_latency_ms present.
// D-087: streak counter resets on success.
func TestAPIStreak_ResetsOnSuccess(t *testing.T) {
	failCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{"applications": []any{}}) //nolint:errcheck
		case "/rest/v2/cluster/nodes":
			w.WriteHeader(http.StatusNotFound)
		case "/rest/v2/system-status":
			if failCount < 2 {
				failCount++
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"osName": "Linux"}) //nolint:errcheck
			}
		case "/rest/v2/version":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Collect: 2 failure events, then 1 success event with consec=0.
	var successWithReset *domain.ServerEvent
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	var seenFailures int

	for successWithReset == nil {
		select {
		case <-sink.notify:
			sink.mu.Lock()
			for _, ev := range sink.events {
				if ev.Type != domain.EventNodeStats {
					continue
				}
				if u, _ := ev.Data["api_unreachable"].(bool); u {
					seenFailures++
					continue
				}
				// Success event — check if streak was reset.
				if seenFailures >= 2 {
					if c, _ := ev.Data["consec_api_errors"].(float64); c == 0 {
						cp := ev
						successWithReset = &cp
					}
				}
			}
			sink.mu.Unlock()
		case <-deadline.C:
			t.Fatalf("FAIL: did not observe reset (failures seen=%d, success found=%v)", seenFailures, successWithReset != nil)
		}
	}

	ev := successWithReset

	// After reset: consec_api_errors must be 0.
	errs, _ := ev.Data["consec_api_errors"].(float64)
	if errs != 0 {
		t.Errorf("consec_api_errors = %v after success, want 0.0 (streak must reset)", errs)
	} else {
		t.Logf("PASS: consec_api_errors reset to 0 on success after %d failures", seenFailures)
	}

	// api_latency_ms must be present again.
	if _, present := ev.Data["api_latency_ms"]; !present {
		t.Errorf("api_latency_ms absent on success after reset (must be present)")
	} else {
		t.Logf("PASS: api_latency_ms present on success after reset")
	}
}

// ─── Test 5: Broadcast failures don't touch streak ───────────────────────────

// TestAPIStreak_BroadcastFailure_NoStreakIncrement verifies that when broadcast/app
// polling fails (e.g. the app list endpoint returns 500), the API streak counter
// is NOT incremented — only SystemStats/ClusterNodes calls drive the streak.
// D-087 contract: "only the node-stats call does" increment the streak.
func TestAPIStreak_BroadcastFailure_NoStreakIncrement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			// Return one app so pollApp is called.
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"applications": []string{"live"},
			})
		case "/rest/v2/cluster/nodes":
			w.WriteHeader(http.StatusNotFound) // standalone
		case "/rest/v2/system-status":
			// SystemStats succeeds → streak should be 0.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"osName": "Linux"}) //nolint:errcheck
		case "/rest/v2/version":
			w.WriteHeader(http.StatusNotFound)
		case "/live/rest/v2/broadcasts/list/0/200":
			// Broadcast listing fails — this must NOT affect the streak.
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Wait for a successful node_stats event (SystemStats succeeds).
	events := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return !u
	}, 3*time.Second)

	if len(events) == 0 {
		t.Fatal("FAIL: no successful node_stats event received within 3s")
	}
	ev := events[0]

	// consec_api_errors must be 0 despite broadcast listing failure.
	errs, _ := ev.Data["consec_api_errors"].(float64)
	if errs != 0 {
		t.Errorf("consec_api_errors = %v, want 0.0 — broadcast failure must NOT increment streak", errs)
	} else {
		t.Logf("PASS: consec_api_errors = 0 (broadcast failure did not touch streak)")
	}

	// api_latency_ms must be present.
	if _, present := ev.Data["api_latency_ms"]; !present {
		t.Errorf("api_latency_ms absent on successful node_stats (should be present)")
	}
}

// ─── Test 6: Cluster path RTT ────────────────────────────────────────────────

// TestAPIStreak_ClusterPath_RTTOnSuccess verifies that a successful ClusterNodes call
// (cluster mode) also produces api_latency_ms in the emitted node_stats event.
func TestAPIStreak_ClusterPath_RTTOnSuccess(t *testing.T) {
	srv := clusterAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return one cluster node.
		json.NewEncoder(w).Encode([]map[string]any{{ //nolint:errcheck
			"nodeId":      "node-cluster-1",
			"ip":          "10.0.0.1",
			"cpuUsage":    20.0,
			"memoryUsage": 40.0,
		}})
	})
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	events := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return !u
	}, 3*time.Second)

	if len(events) == 0 {
		t.Fatal("FAIL: no node_stats event from cluster path within 3s")
	}
	ev := events[0]

	if _, present := ev.Data["api_latency_ms"]; !present {
		t.Errorf("api_latency_ms absent on cluster-path success; Data=%v", ev.Data)
	} else {
		t.Logf("PASS: api_latency_ms present on cluster-path success")
	}
	if c, _ := ev.Data["consec_api_errors"].(float64); c != 0 {
		t.Errorf("consec_api_errors = %v, want 0 on cluster-path success", c)
	}
}

// TestAPIStreak_ClusterPath_FailureEvent verifies that a ClusterNodes error (non-404)
// produces a failure-streak node_stats event with api_unreachable=true.
func TestAPIStreak_ClusterPath_FailureEvent(t *testing.T) {
	srv := clusterAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// 500 error (not 404) — treated as a real failure, not standalone.
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	events := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return u
	}, 3*time.Second)

	if len(events) == 0 {
		t.Fatal("FAIL: no failure-streak event from cluster-path failure within 3s")
	}
	ev := events[0]

	if u, _ := ev.Data["api_unreachable"].(bool); !u {
		t.Errorf("api_unreachable = %v, want true on cluster-path failure", ev.Data["api_unreachable"])
	}
	if _, present := ev.Data["api_latency_ms"]; present {
		t.Errorf("api_latency_ms present on cluster-path failure (must be absent)")
	}
	t.Logf("PASS: cluster-path failure produces api_unreachable=true, no RTT")
}

// ─── Test 8: post-recovery failure starts a FRESH streak (D-087 verify M4) ───

// TestAPIStreak_PostRecoveryFailure_StartsAtOne is the discriminating pin the
// S25 verify pass demanded: a success event emitting a hardcoded 0 would make
// Test 4 pass even if the internal counter were never reset. The only
// observable proof of the reset is the FIRST failure AFTER a recovery: its
// consec_api_errors must be 1 (fresh streak), not oldStreak+1.
//
// Sequence driven via the fake AMS: fail ×2 → succeed ×1 → fail forever.
func TestAPIStreak_PostRecoveryFailure_StartsAtOne(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{"applications": []any{}}) //nolint:errcheck
		case "/rest/v2/cluster/nodes":
			w.WriteHeader(http.StatusNotFound)
		case "/rest/v2/system-status":
			call++
			if call <= 2 || call >= 4 {
				w.WriteHeader(http.StatusInternalServerError)
			} else { // call == 3: the one recovery success
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"osName": "Linux"}) //nolint:errcheck
			}
		case "/rest/v2/version":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-sink.notify:
			// Stateless positional scan on every notify: the verdict event is the
			// first failure event AFTER the buffer position of the recovery
			// success. (A latched-flag rescan from index 0 would pick a
			// PRE-recovery failure and pass vacuously — the first draft of this
			// test did exactly that; caught by the D-087 M4 re-derivation.)
			sink.mu.Lock()
			var nodeEvents []domain.ServerEvent
			for _, ev := range sink.events {
				if ev.Type == domain.EventNodeStats {
					nodeEvents = append(nodeEvents, ev)
				}
			}
			sink.mu.Unlock()
			recoveryIdx := -1
			for i, ev := range nodeEvents {
				if u, _ := ev.Data["api_unreachable"].(bool); !u {
					recoveryIdx = i
					break
				}
			}
			if recoveryIdx < 0 {
				continue // recovery success not yet observed
			}
			for _, ev := range nodeEvents[recoveryIdx+1:] {
				if u, _ := ev.Data["api_unreachable"].(bool); !u {
					continue
				}
				// The verdict event: first failure after the recovery.
				c, _ := ev.Data["consec_api_errors"].(float64)
				if c != 1 {
					t.Fatalf("FAIL: post-recovery failure carries consec_api_errors=%v, want 1 — the internal streak counter was not reset on success", c)
				}
				t.Logf("PASS: post-recovery failure starts a fresh streak (consec_api_errors=1)")
				return
			}
		case <-deadline.C:
			t.Fatal("FAIL: timed out — expected a failure event after the recovery success")
		}
	}
}
