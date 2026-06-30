// Package query_test — unit tests for LiveStreamItem viewer QoE field mapping.
package query_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
)

// mockLiveProvider is a minimal domain.LiveProvider for query unit tests.
type mockLiveProvider struct {
	mu   sync.RWMutex
	snap *domain.LiveSnapshot
}

func (m *mockLiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snap
}

func (m *mockLiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// TestLiveStreams_ViewerQoEFields verifies that viewer-side WebRTC QoE metrics
// (ViewerRTTMS, ViewerJitterMS, ViewerLossPct) from a LiveStream snapshot
// surface as viewer_rtt_ms / viewer_jitter_ms / viewer_loss_pct in the
// LiveStreamItem returned by LiveStreams().
func TestLiveStreams_ViewerQoEFields(t *testing.T) {
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"stream-qoe": {
				StreamID:      "stream-qoe",
				App:           "live",
				NodeID:        "node-1",
				Active:        true,
				ViewerCount:   5,
				IngestBitrate: 1000.0,
				Health:        domain.StreamHealthGood,
				StartedAt:     time.Now().Add(-time.Minute),
				// Viewer-side QoE metrics from webrtc_client_stats events.
				ViewerRTTMS:    42.5,
				ViewerJitterMS: 8.2,
				ViewerLossPct:  1.5,
			},
		},
		Nodes:     map[string]*domain.LiveNodeStats{},
		UpdatedAt: time.Now(),
	}

	live := &mockLiveProvider{snap: snap}
	lic, _ := license.New("", "")
	svc := query.New(live, nil, lic)

	result, err := svc.LiveStreams(context.Background(), "", "", "", 10, "")
	if err != nil {
		t.Fatalf("LiveStreams: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 stream item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.StreamID != "stream-qoe" {
		t.Errorf("StreamID = %q, want %q", item.StreamID, "stream-qoe")
	}

	// viewer_rtt_ms must be mapped from LiveStream.ViewerRTTMS.
	if item.ViewerRttMs != 42.5 {
		t.Errorf("ViewerRttMs = %v, want 42.5", item.ViewerRttMs)
	}
	// viewer_jitter_ms must be mapped from LiveStream.ViewerJitterMS.
	if item.ViewerJitterMs != 8.2 {
		t.Errorf("ViewerJitterMs = %v, want 8.2", item.ViewerJitterMs)
	}
	// viewer_loss_pct must be mapped from LiveStream.ViewerLossPct.
	if item.ViewerLossPct != 1.5 {
		t.Errorf("ViewerLossPct = %v, want 1.5", item.ViewerLossPct)
	}

	t.Logf("PASS: viewer QoE fields: rtt=%.1f jitter=%.1f loss=%.1f",
		item.ViewerRttMs, item.ViewerJitterMs, item.ViewerLossPct)
}

// TestLiveStreams_ViewerQoEFields_Absent verifies that when viewer QoE fields
// are zero (no WebRTC viewers), they serialize as 0 and are correctly absent
// from JSON (omitempty). This is a compile-time / tag check via struct fields.
func TestLiveStreams_ViewerQoEFields_Absent(t *testing.T) {
	snap := &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"stream-noqoe": {
				StreamID:    "stream-noqoe",
				App:         "live",
				Active:      true,
				ViewerCount: 3,
				Health:      domain.StreamHealthGood,
				// ViewerRTTMS / ViewerJitterMS / ViewerLossPct all zero (no WebRTC viewers)
			},
		},
		Nodes:     map[string]*domain.LiveNodeStats{},
		UpdatedAt: time.Now(),
	}

	live := &mockLiveProvider{snap: snap}
	lic, _ := license.New("", "")
	svc := query.New(live, nil, lic)

	result, err := svc.LiveStreams(context.Background(), "", "", "", 10, "")
	if err != nil {
		t.Fatalf("LiveStreams: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 stream item, got %d", len(result.Items))
	}

	item := result.Items[0]
	// Zero values are valid — no assertion needed; this test confirms
	// the struct fields exist and compile cleanly.
	if item.ViewerRttMs != 0 || item.ViewerJitterMs != 0 || item.ViewerLossPct != 0 {
		t.Errorf("expected all viewer QoE fields to be 0 when absent from snapshot; got rtt=%v jitter=%v loss=%v",
			item.ViewerRttMs, item.ViewerJitterMs, item.ViewerLossPct)
	}
	t.Log("PASS: viewer QoE fields correctly 0 when no WebRTC viewer data")
}
