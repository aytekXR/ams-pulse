// Package sessions — session stitcher tests.
//
// Tests cover: join/heartbeat/leave sequence, join+timeout sequence.
package sessions

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// captureSink collects viewer sessions written to it.
type captureSink struct {
	sessions []domain.ViewerSession
}

func (c *captureSink) WriteServerEvent(_ domain.ServerEvent)     {}
func (c *captureSink) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (c *captureSink) WriteViewerSession(s domain.ViewerSession) { c.sessions = append(c.sessions, s) }

// lastSession returns the last written session or zero.
func (c *captureSink) lastSession() (domain.ViewerSession, bool) {
	if len(c.sessions) == 0 {
		return domain.ViewerSession{}, false
	}
	return c.sessions[len(c.sessions)-1], true
}

// findSessionID returns all sessions with the given session_id.
func (c *captureSink) findSessionID(id string) []domain.ViewerSession {
	var out []domain.ViewerSession
	for _, s := range c.sessions {
		if s.SessionID == id {
			out = append(out, s)
		}
	}
	return out
}

// ─── join/heartbeat/leave test ────────────────────────────────────────────────

// TestStitcher_JoinHeartbeatLeave verifies the full lifecycle produces correct session rows.
func TestStitcher_JoinHeartbeatLeave(t *testing.T) {
	sink := &captureSink{}
	st := New(Config{IdleTimeout: 5 * time.Minute}, sink, nil)

	baseTime := time.Now().Truncate(time.Second)

	// 1. Viewer join.
	st.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventViewerJoin,
		TS:       baseTime.UnixMilli(),
		StreamID: "stream1",
		App:      "live",
		NodeID:   "n1",
		Data: map[string]any{
			"viewer_id": "viewer-abc",
			"protocol":  "webrtc",
		},
	})

	if st.ActiveCount() != 1 {
		t.Errorf("after join: active sessions = %d, want 1", st.ActiveCount())
	}

	// Join should have written an initial session row.
	if len(sink.sessions) != 1 {
		t.Fatalf("after join: expected 1 session write, got %d", len(sink.sessions))
	}
	joinRow := sink.sessions[0]
	if joinRow.SessionID != "viewer-abc" {
		t.Errorf("join row: session_id = %q, want viewer-abc", joinRow.SessionID)
	}
	if joinRow.Protocol != "webrtc" {
		t.Errorf("join row: protocol = %q, want webrtc", joinRow.Protocol)
	}

	// 2. Beacon heartbeat at t+30s.
	heartbeatTime := baseTime.Add(30 * time.Second)
	st.OnBeaconEvent(domain.BeaconEvent{
		SessionID: "viewer-abc",
		StreamID:  "stream1",
		App:       "live",
		Events: []domain.BeaconItem{
			{
				Type: "heartbeat",
				TS:   heartbeatTime.UnixMilli(),
				Data: map[string]any{
					"watch_ms": float64(30000),
				},
			},
		},
	})

	// Heartbeat writes an upsert.
	if len(sink.sessions) < 2 {
		t.Fatalf("after heartbeat: expected ≥2 session writes, got %d", len(sink.sessions))
	}
	hbRow := sink.sessions[len(sink.sessions)-1]
	if hbRow.WatchTimeS != 30 {
		t.Errorf("heartbeat row: watch_time_s = %d, want 30", hbRow.WatchTimeS)
	}

	// 3. Viewer leave at t+65s.
	leaveTime := baseTime.Add(65 * time.Second)
	st.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventViewerLeave,
		TS:       leaveTime.UnixMilli(),
		StreamID: "stream1",
		App:      "live",
		NodeID:   "n1",
		Data: map[string]any{
			"viewer_id":    "viewer-abc",
			"protocol":     "webrtc",
			"watch_time_s": float64(65),
		},
	})

	if st.ActiveCount() != 0 {
		t.Errorf("after leave: active sessions = %d, want 0", st.ActiveCount())
	}

	// Leave should have written the final row with updated watch time.
	finalRows := sink.findSessionID("viewer-abc")
	if len(finalRows) < 2 {
		t.Fatalf("after leave: expected ≥2 rows for viewer-abc, got %d", len(finalRows))
	}
	finalRow := finalRows[len(finalRows)-1]
	if finalRow.WatchTimeS != 65 {
		t.Errorf("final row: watch_time_s = %d, want 65", finalRow.WatchTimeS)
	}
	if finalRow.EndedAt.IsZero() {
		t.Error("final row: ended_at is zero")
	}
	if !finalRow.EndedAt.Equal(leaveTime.UTC()) {
		t.Errorf("final row: ended_at = %v, want %v", finalRow.EndedAt, leaveTime.UTC())
	}
}

// TestStitcher_JoinTimeout verifies that an idle session is closed by eviction.
func TestStitcher_JoinTimeout(t *testing.T) {
	sink := &captureSink{}
	// Use a very short idle timeout for the test.
	shortTimeout := 50 * time.Millisecond
	st := New(Config{IdleTimeout: shortTimeout}, sink, nil)

	baseTime := time.Now()

	// Viewer join.
	st.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventViewerJoin,
		TS:       baseTime.UnixMilli(),
		StreamID: "stream2",
		App:      "live",
		NodeID:   "n1",
		Data: map[string]any{
			"viewer_id": "viewer-timeout",
			"protocol":  "hls",
		},
	})

	if st.ActiveCount() != 1 {
		t.Fatalf("after join: active = %d, want 1", st.ActiveCount())
	}

	// Wait for the idle timeout to elapse.
	time.Sleep(shortTimeout + 20*time.Millisecond)

	// Run eviction sweep.
	evicted := st.EvictIdle()
	if evicted != 1 {
		t.Errorf("EvictIdle: evicted = %d, want 1", evicted)
	}
	if st.ActiveCount() != 0 {
		t.Errorf("after eviction: active = %d, want 0", st.ActiveCount())
	}

	// The eviction should have written a final session row.
	rows := sink.findSessionID("viewer-timeout")
	if len(rows) < 2 {
		t.Fatalf("after eviction: expected ≥2 rows, got %d", len(rows))
	}
	finalRow := rows[len(rows)-1]
	if finalRow.EndedAt.IsZero() {
		t.Error("timeout row: ended_at is zero")
	}
	// ended_at should be after started_at.
	if !finalRow.EndedAt.After(finalRow.StartedAt) && !finalRow.EndedAt.Equal(finalRow.StartedAt) {
		t.Errorf("timeout row: ended_at %v should be >= started_at %v", finalRow.EndedAt, finalRow.StartedAt)
	}
	// Note: WatchTimeS may be 0 for very short test sessions (< 1s); this is
	// acceptable — the row is still written correctly with proper timestamps.
}

// TestStitcher_MultipleViewers verifies independent sessions are tracked correctly.
func TestStitcher_MultipleViewers(t *testing.T) {
	sink := &captureSink{}
	st := New(Config{IdleTimeout: 5 * time.Minute}, sink, nil)

	now := time.Now().UnixMilli()

	// Join 3 viewers.
	for i := 0; i < 3; i++ {
		st.OnServerEvent(domain.ServerEvent{
			Type:     domain.EventViewerJoin,
			TS:       now,
			StreamID: "stream3",
			Data: map[string]any{
				"viewer_id": []string{"v1", "v2", "v3"}[i],
				"protocol":  "webrtc",
			},
		})
	}

	if st.ActiveCount() != 3 {
		t.Errorf("after 3 joins: active = %d, want 3", st.ActiveCount())
	}

	// Leave viewer v2.
	st.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventViewerLeave,
		TS:       now + 10000,
		StreamID: "stream3",
		Data: map[string]any{
			"viewer_id":    "v2",
			"protocol":     "webrtc",
			"watch_time_s": float64(10),
		},
	})

	if st.ActiveCount() != 2 {
		t.Errorf("after v2 leave: active = %d, want 2", st.ActiveCount())
	}
}

// TestStitcher_WatchTimeFromTimestamps verifies watch_time is derived from join/leave
// timestamps when AMS does not send watch_time_s.
func TestStitcher_WatchTimeFromTimestamps(t *testing.T) {
	sink := &captureSink{}
	st := New(Config{IdleTimeout: 5 * time.Minute}, sink, nil)

	joinTime := time.Now()
	leaveTime := joinTime.Add(90 * time.Second)

	st.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventViewerJoin,
		TS:       joinTime.UnixMilli(),
		StreamID: "s4",
		Data: map[string]any{
			"viewer_id": "v-ts",
			"protocol":  "rtmp",
		},
	})

	// Leave without explicit watch_time_s.
	st.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventViewerLeave,
		TS:       leaveTime.UnixMilli(),
		StreamID: "s4",
		Data: map[string]any{
			"viewer_id": "v-ts",
			"protocol":  "rtmp",
			// watch_time_s absent
		},
	})

	rows := sink.findSessionID("v-ts")
	finalRow := rows[len(rows)-1]
	if finalRow.WatchTimeS < 89 || finalRow.WatchTimeS > 91 {
		t.Errorf("watch_time_s derived from timestamps: got %d, want ~90", finalRow.WatchTimeS)
	}
}

// TestStitcher_BeaconHeartbeatCreatesSession verifies that a beacon heartbeat
// for an unknown session creates a new session entry.
func TestStitcher_BeaconHeartbeatCreatesSession(t *testing.T) {
	sink := &captureSink{}
	st := New(Config{IdleTimeout: 5 * time.Minute}, sink, nil)

	now := time.Now().UnixMilli()

	// Beacon heartbeat without prior viewer_join (beacon-first flow).
	st.OnBeaconEvent(domain.BeaconEvent{
		SessionID: "beacon-session-xyz",
		StreamID:  "s5",
		App:       "live",
		Events: []domain.BeaconItem{
			{
				Type: "heartbeat",
				TS:   now,
				Data: map[string]any{
					"watch_ms": float64(60000),
				},
			},
		},
	})

	if st.ActiveCount() != 1 {
		t.Errorf("after beacon heartbeat: active = %d, want 1", st.ActiveCount())
	}

	rows := sink.findSessionID("beacon-session-xyz")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].WatchTimeS != 60 {
		t.Errorf("watch_time_s = %d, want 60", rows[0].WatchTimeS)
	}
}
