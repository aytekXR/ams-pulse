// Gap-closure test (E2E-validation doc §5.10, gap G-20): drive the unexported
// poll() synchronously against a non-empty AMS broadcasts/list and assert the
// broadcasts→domain-events path. Existing restpoller tests either call poll()
// with an EMPTY broadcasts list (pollApp is a no-op) or exercise the poller only
// via Run() in a goroutine; neither pins the direct, deterministic single-poll
// path that turns a live broadcast into publish_start + stream_stats events.
package restpoller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

func TestPoll_EmitsEventsFromNonEmptyBroadcastList(t *testing.T) {
	t.Parallel()
	const streamID = "poll-direct-1"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/testapp/rest/v2/broadcasts/list/0/200":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"streamId":    streamID,
					"status":      "broadcasting",
					"publishType": "rtmp",
					"appName":     "testapp",
					"bitrate":     624000.0, // bits/sec on the AMS 3.0.3 wire → 624 kbps after /1000
				},
			})
		default:
			// cluster/nodes → 404 (ClusterNodes yields nil,nil); system-status/version
			// 404 → warn-and-continue. poll() must still succeed and emit app events.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	sink := newMockVodSink()
	p := buildPollerFor(srv, newFakeVodState(), sink)

	// First synchronous poll: prevStatus is unknown → a new broadcast emits publish_start.
	if err := p.poll(context.Background()); err != nil {
		t.Fatalf("poll(): %v", err)
	}

	var gotPublishStart, gotStats bool
	for _, ev := range sink.copyEvents() {
		if ev.StreamID != streamID {
			continue
		}
		switch ev.Type {
		case domain.EventStreamPublishStart:
			gotPublishStart = true
			if pt, _ := ev.Data["publish_type"].(string); pt != "rtmp" {
				t.Errorf("publish_start publish_type=%q, want rtmp", pt)
			}
		case domain.EventStreamStats:
			gotStats = true
			// The single bits/sec → kbps normalization boundary: 624000/1000 = 624.
			if kbps, _ := ev.Data["bitrate_kbps"].(float64); kbps != 624 {
				t.Errorf("stream_stats bitrate_kbps=%v, want 624 (624000 bits/s ÷ 1000)", kbps)
			}
		}
	}
	if !gotPublishStart {
		t.Fatalf("poll() emitted no publish_start for %q (total events=%d)", streamID, sink.countAll())
	}
	if !gotStats {
		t.Fatalf("poll() emitted no stream_stats for %q", streamID)
	}
}
