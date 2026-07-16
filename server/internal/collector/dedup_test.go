// dedup_test.go — S49 regression test for the cross-app StreamID collision (D-111).
//
// AMS stream identity is (app, streamId): two applications on one node may each
// host a stream with the same bare streamId. Before D-111 the dedupKey omitted
// App, so the second app's publish_start within the same window collided with
// the first and IsDuplicate returned true, silently dropping it. See S48 finding
// [1] (agents/handoffs/S48-AUDIT-FINDINGS.md).
package collector

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// TestDeduplicator_CrossAppSameStreamID_NotDuplicate proves that two apps sharing
// a bare StreamID in the same dedup window are NOT treated as duplicates.
//
// Mutation target: removing `app` from dedupKey (reverting the fix) makes the
// second call collide with the first → IsDuplicate returns true → this test fails.
func TestDeduplicator_CrossAppSameStreamID_NotDuplicate(t *testing.T) {
	d := NewDeduplicator(10 * time.Second)

	// Fixed TS so both events land in the SAME window bucket (ts/windowMs equal) —
	// this is the collision-prone case the fix must survive.
	const ts = int64(1_700_000_000_000)

	evA := domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishStart, TS: ts,
		NodeID: "node-1", App: "LiveApp", StreamID: "test123",
	}
	evB := domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishStart, TS: ts,
		NodeID: "node-1", App: "PetarTest2", StreamID: "test123",
	}

	if d.IsDuplicate(evA) {
		t.Fatal("first app's publish_start reported as duplicate on first sight")
	}
	if d.IsDuplicate(evB) {
		t.Fatal("second app's publish_start dropped as duplicate — dedupKey must " +
			"include App (AMS identity is app/streamId; cross-app same StreamID is distinct)")
	}
}

// TestDeduplicator_SameApp_IsDuplicate is the positive control: an actual
// duplicate (same app, node, streamID, window) IS still dropped, so the fix did
// not defeat deduplication itself.
func TestDeduplicator_SameApp_IsDuplicate(t *testing.T) {
	d := NewDeduplicator(10 * time.Second)
	const ts = int64(1_700_000_000_000)

	ev := domain.ServerEvent{
		Version: 1, Type: domain.EventStreamPublishStart, TS: ts,
		NodeID: "node-1", App: "LiveApp", StreamID: "test123",
	}
	if d.IsDuplicate(ev) {
		t.Fatal("first sighting reported as duplicate")
	}
	if !d.IsDuplicate(ev) {
		t.Fatal("identical repeat within the window must be a duplicate — dedup is broken")
	}
}
