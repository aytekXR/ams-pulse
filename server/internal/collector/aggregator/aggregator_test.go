// Package aggregator — unit tests for VD-20a, VD-03.
package aggregator

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// mockEdgeChecker implements EdgeStreamChecker for tests.
type mockEdgeChecker struct {
	edgeStreams map[string]bool
	roles       map[string]string
}

func (m *mockEdgeChecker) IsEdgeStream(streamID string) bool {
	return m.edgeStreams[streamID]
}
func (m *mockEdgeChecker) NodeRole(nodeID string) string {
	return m.roles[nodeID]
}

// TestAggregator_HealthScore_NonZero verifies that LiveStream.HealthScore is
// non-zero after processing an ingest_stats event (VD-20a).
// Before the fix, UpdateIngestHealth() had zero callers and HealthScore was
// always 0.0 regardless of ingest metrics.
func TestAggregator_HealthScore_NonZero(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Seed a stream (publish_start).
	agg.OnServerEvent(domain.ServerEvent{
		Version:  1,
		Type:     domain.EventStreamPublishStart,
		TS:       time.Now().UnixMilli(),
		Source:   domain.SourceRestPoll,
		NodeID:   "node-1",
		StreamID: "stream-1",
		App:      "live",
		Data:     map[string]any{"publish_type": "rtmp"},
	})

	// Deliver ingest_stats with healthy values.
	agg.OnServerEvent(domain.ServerEvent{
		Version:  1,
		Type:     domain.EventIngestStats,
		TS:       time.Now().UnixMilli(),
		Source:   domain.SourceRestPoll,
		NodeID:   "node-1",
		StreamID: "stream-1",
		App:      "live",
		Data: map[string]any{
			"bitrate_kbps":        2000.0,
			"fps":                 30.0,
			"keyframe_interval_s": 2.0,
			"packet_loss_pct":     0.0,
			"jitter_ms":           0.0,
		},
	})

	snap := agg.CurrentSnapshot()
	s, ok := snap.Streams["stream-1"]
	if !ok {
		t.Fatal("stream-1 not in snapshot")
	}

	if s.HealthScore == 0.0 {
		t.Errorf("HealthScore = 0.0 after ingest_stats with healthy values; want > 0 (VD-20a)")
	}
	if s.HealthScore < 0.5 {
		t.Errorf("HealthScore = %.3f; want >= 0.5 for healthy ingest (VD-20a)", s.HealthScore)
	}
	if s.Health != domain.StreamHealthGood {
		t.Errorf("Health = %q; want %q for score=%.3f (VD-20a)", s.Health, domain.StreamHealthGood, s.HealthScore)
	}
	t.Logf("PASS VD-20a: HealthScore=%.3f Health=%q after ingest_stats", s.HealthScore, s.Health)
}

// TestAggregator_HealthScore_DegradedBitrate verifies that low bitrate produces
// a sub-good health score (not "good") (VD-20a extension).
func TestAggregator_HealthScore_DegradedBitrate(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamPublishStart, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "n", StreamID: "s", App: "live",
		Data: map[string]any{"publish_type": "rtmp"},
	})
	// Very low bitrate, no FPS.
	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventIngestStats, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "n", StreamID: "s", App: "live",
		Data: map[string]any{
			"bitrate_kbps":    10.0, // very low
			"fps":             0.0,
			"packet_loss_pct": 50.0, // severe
		},
	})

	snap := agg.CurrentSnapshot()
	s, ok := snap.Streams["s"]
	if !ok {
		t.Fatal("stream not in snapshot")
	}
	if s.HealthScore == 0.0 {
		t.Error("HealthScore must not be exactly 0.0 for non-zero bitrate")
	}
	if s.Health == domain.StreamHealthGood {
		t.Errorf("Health should not be Good for severely degraded ingest (VD-20a); got %q (score=%.3f)", s.Health, s.HealthScore)
	}
	t.Logf("PASS: degraded ingest HealthScore=%.3f Health=%q", s.HealthScore, s.Health)
}

// TestAggregator_EdgeDedup_ViewerCount verifies that when an edge node is active,
// the origin node's viewer_count is excluded from aggregation (VD-03).
// Before the fix, IsEdgeStream() returned false unconditionally, causing
// double-counting of viewers in origin+edge cluster deployments.
func TestAggregator_EdgeDedup_ViewerCount(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Wire an edge checker: stream-1 is served by an edge node.
	checker := &mockEdgeChecker{
		edgeStreams: map[string]bool{"stream-1": true},
		roles:       map[string]string{"origin-node": "origin", "edge-node": "edge"},
	}
	agg.SetEdgeChecker(checker)

	// Publish start from the origin.
	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamPublishStart, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "origin-node", StreamID: "stream-1", App: "live",
		Data: map[string]any{"publish_type": "rtmp"},
	})
	// Publish start from the edge (same stream).
	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamPublishStart, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "edge-node", StreamID: "stream-1", App: "live",
		Data: map[string]any{"publish_type": "rtmp"},
	})

	// Origin reports 100 viewers (this includes edge viewers — should be SKIPPED).
	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamStats, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "origin-node", StreamID: "stream-1", App: "live",
		Data: map[string]any{"viewer_count": 100, "bitrate_kbps": 2000.0},
	})

	// Edge reports 50 viewers (actual edge viewers — should be COUNTED).
	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamStats, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "edge-node", StreamID: "stream-1", App: "live",
		Data: map[string]any{"viewer_count": 50, "bitrate_kbps": 2000.0},
	})

	snap := agg.CurrentSnapshot()

	// Origin's viewer count (100) should be discarded; edge's (50) should count.
	// TotalViewers in the snapshot should reflect edge count (50), not 150.
	if snap.TotalViewers == 150 {
		t.Errorf("TotalViewers = 150: origin/edge double-counting detected (VD-03). Expected 50.")
	}
	if snap.TotalViewers == 0 {
		t.Errorf("TotalViewers = 0: edge viewers not counted (VD-03)")
	}
	// The edge stream entry should have its viewer count set (50).
	edgeKey := "stream-1"
	if s, ok := snap.Streams[edgeKey]; ok {
		if s.ViewerCount == 100 {
			t.Errorf("edge stream has ViewerCount=100 from origin (double-count), expected 50 (VD-03)")
		}
	}
	t.Logf("PASS VD-03: TotalViewers=%d (no double-count; origin discarded)", snap.TotalViewers)
}

// TestAggregator_NoEdgeChecker_PassThrough verifies that without an edge checker,
// all viewer counts pass through unchanged (standalone deployment).
func TestAggregator_NoEdgeChecker_PassThrough(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)
	// No SetEdgeChecker — standalone mode.

	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamPublishStart, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "n1", StreamID: "s1", App: "live",
		Data: map[string]any{"publish_type": "rtmp"},
	})
	agg.OnServerEvent(domain.ServerEvent{
		Type: domain.EventStreamStats, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "n1", StreamID: "s1", App: "live",
		Data: map[string]any{"viewer_count": 42, "bitrate_kbps": 1500.0},
	})

	snap := agg.CurrentSnapshot()
	if snap.TotalViewers != 42 {
		t.Errorf("standalone TotalViewers = %d, want 42", snap.TotalViewers)
	}
}

// TestAggregator_NodeStats_Version verifies that the version field from
// node_stats events is propagated to the snapshot (VD-40).
func TestAggregator_NodeStats_Version(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	agg.OnServerEvent(domain.ServerEvent{
		Type:   domain.EventNodeStats,
		TS:     time.Now().UnixMilli(),
		Source: domain.SourceRestPoll,
		NodeID: "my-node",
		Data: map[string]any{
			"cpu_pct": 30.0,
			"mem_pct": 50.0,
			"version": "2.8.3",
		},
	})

	snap := agg.CurrentSnapshot()
	node, ok := snap.Nodes["my-node"]
	if !ok {
		t.Fatal("my-node not in snapshot after node_stats event")
	}
	if node.Version != "2.8.3" {
		t.Errorf("Version = %q, want %q (VD-40)", node.Version, "2.8.3")
	}
	t.Logf("PASS VD-40: LiveNodeStats.Version=%q", node.Version)
}

// reentrantAggSink calls back into the aggregator (CurrentSnapshot → a.mu.RLock)
// from inside WriteServerEvent — the production sink (Fanout) likewise fans
// EvictStale's publish_end back to this same aggregator's OnServerEvent. If
// EvictStale emits while holding a.mu, this RLock self-deadlocks.
type reentrantAggSink struct {
	a     *Aggregator
	calls int
}

func (s *reentrantAggSink) WriteServerEvent(domain.ServerEvent) {
	_ = s.a.CurrentSnapshot() // RLock a.mu — must not be held by EvictStale()
	s.calls++
}
func (s *reentrantAggSink) WriteBeaconEvent(domain.BeaconEvent)     {}
func (s *reentrantAggSink) WriteViewerSession(domain.ViewerSession) {}

// TestAggregator_EvictStaleDoesNotHoldLockDuringSinkEmit is a regression guard:
// EvictStale must emit eviction events only AFTER releasing a.mu, or the sink's
// fan-back into OnServerEvent (a.mu.Lock) self-deadlocks (the same class of bug
// as the cluster.Discovery.poll AB→BA deadlock that wedged the live dashboard).
func TestAggregator_EvictStaleDoesNotHoldLockDuringSinkEmit(t *testing.T) {
	agg := New(time.Minute, nil, nil)
	sink := &reentrantAggSink{a: agg}
	agg.sink = sink

	// Seed an Active stream with an old LastSeenAt (TS in the past) so it is
	// immediately stale relative to the 1-minute threshold.
	agg.OnServerEvent(domain.ServerEvent{
		Version:  1,
		Type:     domain.EventStreamPublishStart,
		TS:       time.Now().Add(-time.Hour).UnixMilli(),
		Source:   domain.SourceRestPoll,
		NodeID:   "n1",
		StreamID: "s1",
		App:      "live",
		Data:     map[string]any{"publish_type": "rtmp"},
	})

	done := make(chan struct{})
	go func() { agg.EvictStale(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("EvictStale() deadlocked: it emitted to the sink while holding a.mu")
	}
	if sink.calls == 0 {
		t.Fatal("expected EvictStale to emit a publish_end event for the stale stream")
	}
}

// TestAggregator_CrossAppStreamID_NoCollision verifies that two AMS applications
// hosting a stream with the SAME streamId on the SAME node do not collide: a
// publish_end for one app must not delete the live stream in the other app.
// Regression for the real-AMS multi-app bug (D-029): test.antmedia.io serves
// "test123" live in LiveApp while a stale per-app poll emitted a publish_end that
// — under the old node-only key — deleted the live stream from the snapshot.
func TestAggregator_CrossAppStreamID_NoCollision(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// LiveApp/test123 goes live.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishStart, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "node-1", App: "LiveApp", StreamID: "test123",
		Data: map[string]any{"publish_type": "rtmp"},
	})
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1, Type: domain.EventStreamStats, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "node-1", App: "LiveApp", StreamID: "test123",
		Data: map[string]any{"viewer_count": 0},
	})

	// A different app on the same node ends a (different) stream that happens to
	// share the streamId. This must NOT delete LiveApp/test123.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishEnd, TS: time.Now().UnixMilli(),
		Source: domain.SourceRestPoll, NodeID: "node-1", App: "PetarTest2", StreamID: "test123",
		Data: map[string]any{"reason": "disappeared"},
	})

	snap := agg.CurrentSnapshot()
	if snap.ActiveStreams != 1 {
		t.Fatalf("ActiveStreams = %d after cross-app publish_end; want 1 (LiveApp/test123 still live)", snap.ActiveStreams)
	}
	s, ok := snap.Streams["test123"]
	if !ok || !s.Active || s.App != "LiveApp" {
		t.Fatalf("LiveApp/test123 missing/inactive after PetarTest2 publish_end: ok=%v stream=%+v", ok, s)
	}
}
