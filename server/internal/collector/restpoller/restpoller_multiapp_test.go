// restpoller_multiapp_test.go — D-029 regression test.
//
// Verifies that the multi-app keying fix in detectEnded (D-029) holds:
// when two AMS apps host a stream with the same stream-ID, a stream ending
// in app-B must NOT emit a false publish_end for app-A's stream.
//
// The pre-fix bug keyed prevStatus by "nodeID/streamID" (no app), so
// detectEnded("PetarTest2", []) would also evict "LiveApp/test123" from
// prevStatus and emit a spurious publish_end for a genuinely-live stream.
// The fix uses "nodeID/app/streamID" as the key, scoping each app's end
// detection to its own prefix.
package restpoller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// TestRestPoller_MultiApp_NoFalseEnd is the D-029 regression test.
//
// Scenario:
//   - AMS node serves two apps: "LiveApp" (app-A) and "PetarTest2" (app-B).
//   - Both apps host stream "test123" in the first poll cycle (both broadcasting).
//   - From the second poll cycle onwards, app-B's "test123" is gone; app-A's stays live.
//
// Assertions:
//  1. No publish_end is ever emitted for app-A / "test123" — the stream is still live
//     and the poller must not falsely end it (D-029 multi-app keying bug).
//  2. A publish_end IS emitted for app-B / "test123" — proves detectEnded ran and
//     that the negative assertion above is meaningful, not vacuous.
//  3. A publish_start IS emitted for app-A / "test123" — proves the stream was seen
//     as live, so the absence of publish_end is not due to it never being tracked.
func TestRestPoller_MultiApp_NoFalseEnd(t *testing.T) {
	const (
		nodeID         = "node-1"
		sharedStreamID = "test123"
		appA           = "LiveApp"    // stream stays live throughout the test
		appB           = "PetarTest2" // stream disappears after the first poll
		pollInterval   = 100 * time.Millisecond
	)

	// appBPollCount is incremented each time the mock serves app-B's broadcast list.
	// On the first call (count==1) the stream is present; after that the list is empty,
	// causing detectEnded to emit publish_end for app-B/test123.
	var appBPollCount atomic.Int32

	mockAMS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {

		// Standalone AMS: 404 on cluster/nodes → poller falls back to SystemStats path.
		// SystemStats also 404s (default case), so the poller logs a warning and continues.
		case "/rest/v2/cluster/nodes":
			w.WriteHeader(http.StatusNotFound)

		// app-A: "test123" is always broadcasting — never disappears.
		case "/" + appA + "/rest/v2/broadcasts/list/0/200":
			json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
				{
					"streamId":    sharedStreamID,
					"status":      "broadcasting",
					"publishType": "rtmp",
					"appName":     appA,
					"bitrate":     624000.0, // bits/sec (real AMS 3.0.3 unit)
				},
			})

		// app-B: "test123" is live on the first poll, then gone.
		case "/" + appB + "/rest/v2/broadcasts/list/0/200":
			count := appBPollCount.Add(1)
			if count <= 1 {
				json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
					{
						"streamId":    sharedStreamID,
						"status":      "broadcasting",
						"publishType": "rtmp",
						"appName":     appB,
						"bitrate":     624000.0,
					},
				})
			} else {
				// Stream gone: empty list triggers detectEnded.
				json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockAMS.Close()

	client := amsclient.New(amsclient.Config{
		BaseURL: mockAMS.URL,
		Timeout: 3 * time.Second,
	})
	sink := newMockSink()
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       nodeID,
			PollInterval: pollInterval,
			// Explicit app list: resolveApps returns this directly without calling
			// ListApplications, making the mock server simpler.
			Applications: []string{appA, appB},
		},
		client,
		sink,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = poller.Run(ctx)
	}()

	// ── Phase 1: wait for the poller to detect that app-B's stream ended ──────────
	//
	// publish_end for appB/sharedStreamID confirms that:
	//   (a) the poller ran at least two poll cycles,
	//   (b) detectEnded fired for app-B after its stream disappeared.
	//
	// This makes the negative assertion in Phase 2 non-vacuous: if the poller
	// hadn't run enough cycles, both assertions would trivially pass.
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()

waitForAppBEnd:
	for {
		select {
		case <-sink.notify:
			sink.mu.Lock()
			var found bool
			for _, ev := range sink.events {
				if ev.Type == domain.EventStreamPublishEnd &&
					ev.App == appB && ev.StreamID == sharedStreamID {
					found = true
					break
				}
			}
			sink.mu.Unlock()
			if found {
				break waitForAppBEnd
			}

		case <-deadline.C:
			t.Fatalf("timed out waiting for publish_end for %s/%s — poller did not detect disappeared stream",
				appB, sharedStreamID)

		case <-ctx.Done():
			t.Fatalf("context done before publish_end for %s/%s", appB, sharedStreamID)
		}
	}

	// Stop the poller; snapshot events collected so far.
	cancel()

	sink.mu.Lock()
	allEvents := make([]domain.ServerEvent, len(sink.events))
	copy(allEvents, sink.events)
	sink.mu.Unlock()

	// ── Phase 2: D-029 regression assertion ───────────────────────────────────────
	//
	// app-A's stream is still live. The poller must NOT have emitted publish_end
	// for it. Before the D-029 fix, detectEnded used "nodeID/streamID" (no app),
	// so processing app-B's empty broadcast list would evict "node-1/test123" from
	// prevStatus regardless of which app it belonged to, and emit a false
	// publish_end for LiveApp/test123.
	for _, ev := range allEvents {
		if ev.Type == domain.EventStreamPublishEnd &&
			ev.App == appA && ev.StreamID == sharedStreamID {
			t.Errorf("D-029 regression: false publish_end emitted for %s/%s — "+
				"app-A stream is still live; multi-app keying bug reintroduced "+
				"(detectEnded must use nodeID/app/streamID prefix, not nodeID/streamID)",
				appA, sharedStreamID)
		}
	}

	// ── Phase 3: confirm app-A stream was tracked as active ───────────────────────
	//
	// If publish_start was never emitted for app-A/test123, the test above would
	// pass trivially (the stream was never tracked, so no false end could occur).
	// This assertion makes Phase 2 meaningful.
	var appAStartSeen bool
	for _, ev := range allEvents {
		if ev.Type == domain.EventStreamPublishStart &&
			ev.App == appA && ev.StreamID == sharedStreamID {
			appAStartSeen = true
			break
		}
	}
	if !appAStartSeen {
		t.Errorf("expected stream_publish_start for %s/%s but none received — "+
			"test is misconfigured (app-A stream must be seen as broadcasting)",
			appA, sharedStreamID)
	}
}
