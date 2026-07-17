// Gap-closure test (E2E-validation doc §5.10, gap G-09): the leave-without-join
// early-return branch in stitcher.go handleLeave (a viewer_leave for a viewer_id
// that never joined) had no covering test. The heartbeat-creates-session and
// EvictIdle branches are already covered by stitcher_test.go; this pins the
// remaining branch so a refactor cannot start fabricating phantom sessions or
// corrupting the active-viewer count on a stray leave.
package sessions

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

func TestStitcher_LeaveWithoutJoin_NoSessionNoActiveCount(t *testing.T) {
	sink := &captureSink{}
	st := New(Config{IdleTimeout: 5 * time.Minute}, sink, nil)

	// A viewer_leave whose viewer_id was never seen in a join.
	st.OnServerEvent(domain.ServerEvent{
		Version:  1,
		Type:     domain.EventViewerLeave,
		TS:       time.Now().UnixMilli(),
		StreamID: "stream1",
		Data: map[string]any{
			"viewer_id":    "never-joined",
			"watch_time_s": 5.0,
		},
	})

	if got := st.ActiveCount(); got != 0 {
		t.Fatalf("ActiveCount after leave-without-join = %d, want 0", got)
	}
	if len(sink.sessions) != 0 {
		t.Fatalf("leave-without-join wrote %d viewer session(s), want 0 (must not fabricate a session)",
			len(sink.sessions))
	}
}

// An empty viewer_id must also be a no-op (guarded before the map lookup).
func TestStitcher_LeaveEmptyViewerID_NoOp(t *testing.T) {
	sink := &captureSink{}
	st := New(Config{IdleTimeout: 5 * time.Minute}, sink, nil)

	st.OnServerEvent(domain.ServerEvent{
		Version:  1,
		Type:     domain.EventViewerLeave,
		TS:       time.Now().UnixMilli(),
		StreamID: "stream1",
		Data:     map[string]any{"viewer_id": ""},
	})

	if got := st.ActiveCount(); got != 0 {
		t.Fatalf("ActiveCount after empty-viewer_id leave = %d, want 0", got)
	}
	if len(sink.sessions) != 0 {
		t.Fatalf("empty-viewer_id leave wrote %d session(s), want 0", len(sink.sessions))
	}
}
