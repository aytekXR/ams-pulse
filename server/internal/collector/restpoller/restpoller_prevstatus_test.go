// restpoller_prevstatus_test.go — S54 regression test (D-116) for S48 finding [9].
//
// pollApp records EVERY broadcast's status in p.prevStatus (idle, created,
// broadcasting…), but detectEnded used to evict only keys whose status was
// "broadcasting". A non-broadcasting stream that disappeared from AMS therefore
// leaked its prevStatus entry forever — the map grew without bound. The fix
// evicts every disappeared key of the app (any status) while still emitting
// publish_end only for the ones that were broadcasting.
//
// Internal test (package restpoller): calls the unexported detectEnded directly
// and inspects p.prevStatus.
package restpoller

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

func TestDetectEnded_EvictsDisappearedNonBroadcasting(t *testing.T) {
	sink := newMockVodSink()
	// detectEnded never touches the client; a throwaway is fine.
	client := amsclient.New(amsclient.Config{BaseURL: "http://127.0.0.1:0"})
	p := New(Config{NodeID: "n1", Applications: []string{"live"}}, client, sink, nil)

	const prefix = "n1/live/"
	p.mu.Lock()
	p.prevStatus[prefix+"idlecam"] = "created"      // non-broadcasting, will disappear
	p.prevStatus[prefix+"livecam"] = "broadcasting" // broadcasting, will disappear
	p.prevStatus["n1/other/keep"] = "broadcasting"  // different app — must be untouched
	p.mu.Unlock()

	// Both "live" streams are gone this poll (empty current list for app "live").
	p.detectEnded("live", nil)

	p.mu.Lock()
	_, idleStill := p.prevStatus[prefix+"idlecam"]
	_, liveStill := p.prevStatus[prefix+"livecam"]
	_, otherStill := p.prevStatus["n1/other/keep"]
	p.mu.Unlock()

	if idleStill {
		t.Error("non-broadcasting stream that disappeared was NOT evicted from prevStatus (unbounded map leak — finding [9])")
	}
	if liveStill {
		t.Error("broadcasting stream that disappeared was not evicted from prevStatus")
	}
	if !otherStill {
		t.Error("a different app's key was wrongly evicted — detectEnded must stay app-scoped (prefix)")
	}

	// publish_end must be emitted ONLY for the broadcasting stream, never the idle one.
	var ends []domain.ServerEvent
	for _, ev := range sink.copyEvents() {
		if ev.Type == domain.EventStreamPublishEnd {
			ends = append(ends, ev)
		}
	}
	if len(ends) != 1 || ends[0].App != "live" || ends[0].StreamID != "livecam" {
		t.Fatalf("want exactly 1 publish_end for live/livecam, got %+v", ends)
	}
}
