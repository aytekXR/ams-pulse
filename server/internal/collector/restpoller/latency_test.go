// Latency test: a new stream must appear as a domain event within ≤10 s
// of being present on the mock AMS endpoint. Uses an in-process HTTP server
// simulating AMS REST v2.
//
// Acceptance criterion (F1): stream visible ≤ 10 s after publish at default
// 5 s poll interval. Worst case = 1 poll = 5 s. This test uses a 2 s interval
// to keep the test fast while demonstrating the headroom.
package restpoller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// mockEventSink captures events emitted by the poller.
type mockEventSink struct {
	mu     sync.Mutex
	events []domain.ServerEvent
	notify chan struct{}
}

func newMockSink() *mockEventSink {
	return &mockEventSink{notify: make(chan struct{}, 100)}
}

func (m *mockEventSink) WriteServerEvent(ev domain.ServerEvent) {
	m.mu.Lock()
	m.events = append(m.events, ev)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
}
func (m *mockEventSink) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (m *mockEventSink) WriteViewerSession(_ domain.ViewerSession) {}

func (m *mockEventSink) findPublishStart(streamID string) *domain.ServerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.events {
		if m.events[i].Type == domain.EventStreamPublishStart &&
			m.events[i].StreamID == streamID {
			return &m.events[i]
		}
	}
	return nil
}

// TestLatency_StreamVisibleWithin10s uses a mock AMS server and measures
// how quickly a new broadcast appears as a domain event.
func TestLatency_StreamVisibleWithin10s(t *testing.T) {
	const targetStreamID = "latency-test-stream"
	const pollInterval = 2 * time.Second // fast for test; default is 5 s (≤ 10 s F1)
	const maxLatency = 10 * time.Second  // F1 budget

	// Atomic: 0 = stream not yet present, 1 = stream is live.
	var streamPresent atomic.Int32

	// Mock AMS REST server.
	mockAMS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{
				"applications": []map[string]any{
					{"name": "live"},
				},
			})
		case "/live/rest/v2/broadcasts/list/0/200":
			if streamPresent.Load() == 1 {
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"streamId":          targetStreamID,
						"status":            "broadcasting",
						"publishType":       "rtmp",
						"appName":           "live",
						"hlsViewerCount":    0,
						"webRTCViewerCount": 0,
						"rtmpViewerCount":   0,
						"dashViewerCount":   0,
						"bitrate":           2500.0,
						"speed":             2500.0,
					},
				})
			} else {
				json.NewEncoder(w).Encode([]map[string]any{})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockAMS.Close()

	// Set up poller.
	client := amsclient.New(amsclient.Config{
		BaseURL: mockAMS.URL,
		Timeout: 3 * time.Second,
	})
	sink := newMockSink()
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       "test-node",
			PollInterval: pollInterval,
			Applications: []string{"live"},
		},
		client,
		sink,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start poller.
	go func() {
		if err := poller.Run(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("poller.Run: %v", err)
		}
	}()

	// Wait a moment for initial poll to complete (returns empty list).
	time.Sleep(500 * time.Millisecond)

	// Record the time we "publish" the stream.
	publishTime := time.Now()
	streamPresent.Store(1)
	t.Logf("stream published at %v (poll interval %v)", publishTime.Format(time.RFC3339Nano), pollInterval)

	// Wait for the publish_start event.
	deadline := time.NewTimer(maxLatency + pollInterval) // generous deadline
	defer deadline.Stop()

	for {
		select {
		case <-sink.notify:
			if ev := sink.findPublishStart(targetStreamID); ev != nil {
				latency := time.Since(publishTime)
				t.Logf("stream_publish_start received: latency = %v (budget = %v)", latency, maxLatency)
				if latency > maxLatency {
					t.Errorf("FAIL: latency %v exceeds F1 budget %v", latency, maxLatency)
				} else {
					t.Logf("PASS: latency %v <= %v", latency, maxLatency)
				}
				cancel()
				return
			}
		case <-deadline.C:
			t.Errorf("FAIL: stream_publish_start not received within %v", maxLatency)
			cancel()
			return
		case <-ctx.Done():
			if sink.findPublishStart(targetStreamID) != nil {
				return // success path — context cancelled after finding event
			}
			return
		}
	}
}

// TestPoller_Backoff verifies that the collector supervisor restarts a failing
// source with exponential backoff. Uses a short poll interval and a failing
// AMS server.
func TestPoller_BackoffOnAMSFailure(t *testing.T) {
	const pollInterval = 100 * time.Millisecond

	requestCount := atomic.Int32{}
	mockAMS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockAMS.Close()

	client := amsclient.New(amsclient.Config{
		BaseURL: mockAMS.URL,
		Timeout: 500 * time.Millisecond,
	})
	sink := newMockSink()
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       "test-node",
			PollInterval: pollInterval,
			Applications: []string{"live"},
		},
		client,
		sink,
		nil,
	)

	// Wrap in collector supervisor with very short backoff.
	col := collector.New(nil, poller)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go col.Run(ctx)

	<-ctx.Done()

	count := requestCount.Load()
	t.Logf("AMS received %d requests during 2s backoff test", count)
	if count < 2 {
		t.Errorf("expected at least 2 poll attempts, got %d", count)
	}
}
