// Package query_test — unit test for LiveStreams cursor pagination (BUG-009).
//
// TDD RED: before the fix, page 2 returns page 1 again (cursor ignored).
// TDD GREEN: after the fix, cursor advances to the next page.
package query_test

import (
	"context"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/query"
)

// TestLiveStreams_Cursor verifies that the cursor parameter advances pagination
// in LiveStreams. Without the fix, page 2 returns the same items as page 1.
func TestLiveStreams_Cursor(t *testing.T) {
	snap := &domain.LiveSnapshot{
		ActiveStreams: 3,
		Streams: map[string]*domain.LiveStream{
			"s1": {StreamID: "s1", App: "live", NodeID: "node-1", Active: true},
			"s2": {StreamID: "s2", App: "live", NodeID: "node-1", Active: true},
			"s3": {StreamID: "s3", App: "live", NodeID: "node-1", Active: true},
		},
		AppViewers: map[string]int{},
		Nodes:      map[string]*domain.LiveNodeStats{},
	}

	live := &mockLiveProvider{snap: snap}
	svc := query.New(live, nil, nil)
	ctx := context.Background()

	// Page 1: no cursor, limit=2 → 2 items, next_cursor must be non-nil.
	r1, err := svc.LiveStreams(ctx, "", "", "", 2, "")
	if err != nil {
		t.Fatalf("page1: LiveStreams: %v", err)
	}
	if len(r1.Items) != 2 {
		t.Fatalf("page1: want 2 items, got %d", len(r1.Items))
	}
	if r1.Meta.NextCursor == nil {
		t.Fatal("page1: next_cursor must be non-nil when more items exist")
	}
	if *r1.Meta.NextCursor != "2" {
		t.Errorf("page1: next_cursor=%q, want %q", *r1.Meta.NextCursor, "2")
	}
	t.Logf("PASS page1: %d items, next_cursor=%q", len(r1.Items), *r1.Meta.NextCursor)

	// Page 2: cursor="2", limit=2 → 1 remaining item, next_cursor must be nil.
	r2, err := svc.LiveStreams(ctx, "", "", "", 2, *r1.Meta.NextCursor)
	if err != nil {
		t.Fatalf("page2: LiveStreams: %v", err)
	}
	if len(r2.Items) != 1 {
		t.Fatalf("page2: want 1 item, got %d (cursor not advancing?)", len(r2.Items))
	}
	if r2.Meta.NextCursor != nil {
		t.Errorf("page2: next_cursor must be nil when no more items, got %q", *r2.Meta.NextCursor)
	}
	t.Logf("PASS page2: %d item, next_cursor=nil", len(r2.Items))

	// Verify page 2 item is NOT in page 1 items.
	p1IDs := map[string]bool{}
	for _, it := range r1.Items {
		p1IDs[it.StreamID] = true
	}
	for _, it := range r2.Items {
		if p1IDs[it.StreamID] {
			t.Errorf("cursor not advancing: stream_id %q appears on both page1 and page2", it.StreamID)
		} else {
			t.Logf("PASS distinct: page2 stream_id=%q not in page1", it.StreamID)
		}
	}
}

// TestLiveStreams_StaleCursorNoPanic verifies that a cursor beyond the current
// snapshot length (e.g. streams went offline between pages, or a fabricated
// numeric cursor) returns an empty page with nil next_cursor and does NOT panic.
//
// TDD RED: before the fix, items[start:end] panics when start > len(items).
// TDD GREEN: after clamping start to len(items), the slice is always valid.
func TestLiveStreams_StaleCursorNoPanic(t *testing.T) {
	snap := &domain.LiveSnapshot{
		ActiveStreams: 2,
		Streams: map[string]*domain.LiveStream{
			"s1": {StreamID: "s1", App: "live", NodeID: "node-1", Active: true},
			"s2": {StreamID: "s2", App: "live", NodeID: "node-1", Active: true},
		},
		AppViewers: map[string]int{},
		Nodes:      map[string]*domain.LiveNodeStats{},
	}

	live := &mockLiveProvider{snap: snap}
	svc := query.New(live, nil, nil)
	ctx := context.Background()

	// cursor="10" is beyond len(items)==2 — would panic without the clamp fix.
	result, err := svc.LiveStreams(ctx, "", "", "", 50, "10")
	if err != nil {
		t.Fatalf("LiveStreams with stale cursor: unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("stale cursor: want 0 items, got %d", len(result.Items))
	}
	if result.Meta.NextCursor != nil {
		t.Errorf("stale cursor: want nil next_cursor, got %q", *result.Meta.NextCursor)
	}
	t.Logf("PASS: stale cursor \"10\" with 2 streams returns empty page, no panic")
}
