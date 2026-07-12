//go:build integration

// Integration test: anomaly_flag_events ClickHouse store (ADR-0009 BUG-008 phase 2).
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestIntegration_AnomalyFlagEvents \
//	    ./internal/store/clickhouse/... -v -timeout 300s -v /tmp/clickhouse:/tmp/clickhouse
//
// Prerequisites: /tmp/clickhouse binary available.
package clickhouse_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
)

// newFlagEvent builds an AnomalyFlagEvent with all fields populated.
func newFlagEvent(metric, nodeID, app, streamID string, sigma float64, detectedAt time.Time) anomaly.AnomalyFlagEvent {
	scope := buildScope(nodeID, app, streamID)
	return anomaly.AnomalyFlagEvent{
		ID:         uuid.New().String(),
		Metric:     metric,
		NodeID:     nodeID,
		App:        app,
		StreamID:   streamID,
		Scope:      scope,
		Observed:   sigma * 10.0, // synthetic: z*10 as observed
		Expected:   50.0,
		Sigma:      sigma,
		DetectedAt: detectedAt.UTC().Truncate(time.Millisecond),
	}
}

// buildScope builds the canonical raw JSON scope string.
// Only non-empty fields are included (matching scopeJSON in anomaly.go).
func buildScope(nodeID, app, streamID string) string {
	var parts []string
	if nodeID != "" {
		parts = append(parts, `"node_id":"`+nodeID+`"`)
	}
	if app != "" {
		parts = append(parts, `"app":"`+app+`"`)
	}
	if streamID != "" {
		parts = append(parts, `"stream_id":"`+streamID+`"`)
	}
	if len(parts) == 0 {
		return "{}"
	}
	out := "{"
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	out += "}"
	return out
}

// TestIntegration_AnomalyFlagEvents verifies the anomaly_flag_events ClickHouse store:
//
//  1. Migration 0010 applies cleanly via the existing runner.
//  2. Insert → QueryFlagHistory round-trip: all fields incl. scope bytes + DateTime64 ms precision.
//  3. from/to window filtering.
//  4. metric / app / stream_id / minSigma column filters.
//  5. Keyset pagination across ≥2 pages: cursor continuity, no overlap, no gap.
//  6. RecentFlagKeys returns only in-window keys.
//  7. Malformed cursor → typed ErrInvalidCursor (no panic).
func TestIntegration_AnomalyFlagEvents(t *testing.T) {
	store, _, dbName := startClickHouseForProbes(t, "pulse_anomaly_test")
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ─── 1. Round-trip: all fields incl. scope bytes + DateTime64 ms precision ──

	baseTime := time.Now().UTC().Truncate(time.Millisecond).Add(-10 * time.Minute)

	// Event with stream scope.
	ev1 := newFlagEvent("viewers", "", "live", "stream1", 5.5, baseTime)
	// Event with node scope — note the raw scope JSON must be stored byte-identical.
	ev2 := newFlagEvent("cpu_pct", "node-1", "", "", 4.2, baseTime.Add(time.Minute))
	// Event with mixed scope.
	ev3 := newFlagEvent("ingest_bitrate_kbps", "node-1", "live", "stream1", 6.1, baseTime.Add(2*time.Minute))

	t.Logf("inserting 3 events into %s.anomaly_flag_events...", dbName)
	for _, ev := range []anomaly.AnomalyFlagEvent{ev1, ev2, ev3} {
		if err := store.InsertAnomalyFlagEvent(ctx, ev); err != nil {
			t.Fatalf("InsertAnomalyFlagEvent(%q): %v", ev.Metric, err)
		}
	}

	// Give ClickHouse a moment to flush.
	time.Sleep(500 * time.Millisecond)

	// Query all events — no filters.
	events, nextCursor, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "", "", "", 0, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (all): %v", err)
	}
	if nextCursor != "" {
		t.Errorf("nextCursor should be empty (all 3 fit in limit=100), got %q", nextCursor)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	t.Logf("QueryFlagHistory returned %d events (expected 3)", len(events))

	// Verify round-trip of all fields for ev1.
	found1 := findEvent(events, ev1.ID)
	if found1 == nil {
		t.Fatalf("ev1 (id=%s) not found in results", ev1.ID)
	}
	assertEventEqual(t, "ev1", ev1, *found1)

	// Verify round-trip for ev2 (node scope — byte-identical scope JSON).
	found2 := findEvent(events, ev2.ID)
	if found2 == nil {
		t.Fatalf("ev2 (id=%s) not found in results", ev2.ID)
	}
	assertEventEqual(t, "ev2", ev2, *found2)

	// Verify round-trip for ev3.
	found3 := findEvent(events, ev3.ID)
	if found3 == nil {
		t.Fatalf("ev3 (id=%s) not found in results", ev3.ID)
	}
	assertEventEqual(t, "ev3", ev3, *found3)

	t.Log("PASS: all-fields round-trip (scope byte-identical, DateTime64 ms precision)")

	// ─── 2. Time-window filtering ─────────────────────────────────────────────

	// from/to that includes only ev1.
	fromTime := baseTime.Add(-time.Second)
	toTime := baseTime.Add(30 * time.Second)
	window1, _, err := store.QueryFlagHistory(ctx, fromTime, toTime, "", "", "", 0, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (window1): %v", err)
	}
	if len(window1) != 1 || window1[0].ID != ev1.ID {
		t.Errorf("expected only ev1 in [%v, %v] window, got %d events: %v",
			fromTime, toTime, len(window1), eventIDs(window1))
	}
	t.Logf("PASS: from/to filtering → 1 event in narrow window")

	// to-only filter (all events up to ev2).
	toOnly := baseTime.Add(90 * time.Second)
	window2, _, err := store.QueryFlagHistory(ctx, time.Time{}, toOnly, "", "", "", 0, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (to-only): %v", err)
	}
	if len(window2) != 2 {
		t.Errorf("expected 2 events with to-only filter, got %d: %v", len(window2), eventIDs(window2))
	}
	t.Logf("PASS: to-only filter → 2 events")

	// ─── 3. Column filters: metric ────────────────────────────────────────────

	metricFilter, _, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "cpu_pct", "", "", 0, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (metric filter): %v", err)
	}
	if len(metricFilter) != 1 || metricFilter[0].Metric != "cpu_pct" {
		t.Errorf("metric filter: expected 1 cpu_pct event, got %d: %v", len(metricFilter), eventIDs(metricFilter))
	}
	t.Logf("PASS: metric filter → 1 cpu_pct event")

	// ─── 4. Column filters: app ───────────────────────────────────────────────

	appFilter, _, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "", "live", "", 0, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (app filter): %v", err)
	}
	// ev1 and ev3 have app="live".
	if len(appFilter) != 2 {
		t.Errorf("app filter: expected 2 events with app=live, got %d: %v", len(appFilter), eventIDs(appFilter))
	}
	t.Logf("PASS: app filter → 2 events with app=live")

	// ─── 5. Column filters: stream_id ─────────────────────────────────────────

	streamFilter, _, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "", "", "stream1", 0, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (stream filter): %v", err)
	}
	// ev1 and ev3 have stream_id="stream1".
	if len(streamFilter) != 2 {
		t.Errorf("stream filter: expected 2 events with stream1, got %d: %v", len(streamFilter), eventIDs(streamFilter))
	}
	t.Logf("PASS: stream_id filter → 2 events")

	// ─── 6. Column filters: minSigma ─────────────────────────────────────────

	sigmaFilter, _, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "", "", "", 5.6, 100, "")
	if err != nil {
		t.Fatalf("QueryFlagHistory (minSigma filter): %v", err)
	}
	// Only ev3 has sigma=6.1 >= 5.6; ev1=5.5 < 5.6; ev2=4.2 < 5.6.
	if len(sigmaFilter) != 1 || sigmaFilter[0].ID != ev3.ID {
		t.Errorf("minSigma filter: expected only ev3 (sigma=6.1), got %d events: %v",
			len(sigmaFilter), eventIDs(sigmaFilter))
	}
	t.Logf("PASS: minSigma filter → 1 event (sigma≥5.6)")

	t.Log("PASS: all column filters verified")
}

// TestIntegration_AnomalyFlagEvents_Pagination verifies keyset pagination
// across ≥2 pages with cursor continuity, no overlap, and no gap.
func TestIntegration_AnomalyFlagEvents_Pagination(t *testing.T) {
	store, _, _ := startClickHouseForProbes(t, "pulse_anomaly_page_test")
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Insert 7 events at 250 ms spacing from a second-aligned base so that every
	// page boundary falls INSIDE a shared wall-clock second. This makes the test
	// structurally sensitive to the cursor-precision bug fixed in D-086: a keyset
	// comparison that truncates the cursor to second precision (clickhouse-go
	// sends time.Time params as DateTime) re-admits same-second rows and
	// duplicates them across page boundaries, deterministically.
	const total = 7
	baseTime := time.Now().UTC().Truncate(time.Second).Add(-15 * time.Minute)
	inserted := make([]anomaly.AnomalyFlagEvent, total)
	for i := 0; i < total; i++ {
		ev := newFlagEvent("viewers", "", "", "stream-page", float64(i+1), baseTime.Add(time.Duration(i)*250*time.Millisecond))
		inserted[i] = ev
		if err := store.InsertAnomalyFlagEvent(ctx, ev); err != nil {
			t.Fatalf("insert[%d]: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// Paginate with limit=3: expect pages of 3, 3, 1.
	const pageSize = 3
	var collected []anomaly.AnomalyFlagEvent
	cursor := ""
	page := 0
	for {
		page++
		evs, next, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "", "", "", 0, pageSize, cursor)
		if err != nil {
			t.Fatalf("page %d QueryFlagHistory: %v", page, err)
		}
		t.Logf("page %d: %d events, nextCursor=%q", page, len(evs), next)
		if len(evs) == 0 {
			break
		}
		collected = append(collected, evs...)
		cursor = next
		if cursor == "" {
			break
		}
		if page > total {
			t.Fatalf("too many pages (> total events): cursor loop")
		}
	}

	if len(collected) != total {
		t.Errorf("expected %d events across all pages, got %d", total, len(collected))
	}

	// Verify no duplicates (all IDs unique).
	seen := make(map[string]int)
	for i, ev := range collected {
		if prev, ok := seen[ev.ID]; ok {
			t.Errorf("duplicate event %s at page positions %d and %d", ev.ID, prev, i)
		}
		seen[ev.ID] = i
	}

	// Verify no gaps: all inserted IDs appear in collected.
	for _, ins := range inserted {
		if _, ok := seen[ins.ID]; !ok {
			t.Errorf("inserted event %s not found in paginated results", ins.ID)
		}
	}

	// Verify time order: each event.DetectedAt >= previous.
	for i := 1; i < len(collected); i++ {
		if collected[i].DetectedAt.Before(collected[i-1].DetectedAt) {
			t.Errorf("result not time-ordered at index %d: %v < %v",
				i, collected[i].DetectedAt, collected[i-1].DetectedAt)
		}
	}

	t.Logf("PASS: %d events paginated across %d pages, no overlap/gap, time-ordered", total, page)
}

// TestIntegration_AnomalyFlagEvents_RecentFlagKeys verifies that RecentFlagKeys
// returns only (metric, scope) pairs within the specified window.
func TestIntegration_AnomalyFlagEvents_RecentFlagKeys(t *testing.T) {
	store, _, _ := startClickHouseForProbes(t, "pulse_anomaly_recent_test")
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	now := time.Now().UTC()

	// Recent event: 30s ago — well within a 600s window.
	recentEv := newFlagEvent("viewers", "", "", "s1", 5.0, now.Add(-30*time.Second).Truncate(time.Millisecond))
	// Old event: 700s ago — outside the 600s default window.
	oldEv := newFlagEvent("cpu_pct", "node-1", "", "", 4.0, now.Add(-700*time.Second).Truncate(time.Millisecond))

	for _, ev := range []anomaly.AnomalyFlagEvent{recentEv, oldEv} {
		if err := store.InsertAnomalyFlagEvent(ctx, ev); err != nil {
			t.Fatalf("insert(%q): %v", ev.Metric, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// Query with windowSecs=600 (the default WarmHysteresis window).
	keys, err := store.RecentFlagKeys(ctx, 600)
	if err != nil {
		t.Fatalf("RecentFlagKeys(600): %v", err)
	}
	t.Logf("RecentFlagKeys(600) returned %d keys", len(keys))

	// Only the recent event should appear.
	foundRecent := false
	foundOld := false
	for _, k := range keys {
		if k.Metric == recentEv.Metric && k.Scope == recentEv.Scope {
			foundRecent = true
		}
		if k.Metric == oldEv.Metric && k.Scope == oldEv.Scope {
			foundOld = true
		}
	}
	if !foundRecent {
		t.Errorf("recent event (metric=%q scope=%q) not in RecentFlagKeys(600)",
			recentEv.Metric, recentEv.Scope)
	}
	if foundOld {
		t.Errorf("old event (700s ago) should NOT be in RecentFlagKeys(600) window")
	}
	t.Logf("PASS: RecentFlagKeys(600) → recent event in window, old event excluded")
}

// TestIntegration_AnomalyFlagEvents_MalformedCursor verifies that a malformed
// cursor returns ErrInvalidCursor (no panic, HTTP-400-mappable error).
func TestIntegration_AnomalyFlagEvents_MalformedCursor(t *testing.T) {
	store, _, _ := startClickHouseForProbes(t, "pulse_anomaly_cursor_test")
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cases := []struct {
		name   string
		cursor string
	}{
		{"not-base64", "!!!not-base64!!!"},
		{"missing-colon", "bm9jb2xvbg=="}, // base64("nocolon")
		{"bad-timestamp", "eHh4OmlkMTIz"}, // base64("xxx:id123")
		{"empty-id", "MTIzOg=="},          // base64("123:")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := store.QueryFlagHistory(ctx, time.Time{}, time.Time{}, "", "", "", 0, 10, tc.cursor)
			if err == nil {
				t.Errorf("expected error for malformed cursor %q, got nil", tc.cursor)
				return
			}
			if !errors.Is(err, clickhouse.ErrInvalidCursor) {
				t.Errorf("expected ErrInvalidCursor for cursor %q, got: %v", tc.cursor, err)
			}
			t.Logf("PASS: cursor %q → ErrInvalidCursor", tc.name)
		})
	}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func findEvent(events []anomaly.AnomalyFlagEvent, id string) *anomaly.AnomalyFlagEvent {
	for i := range events {
		if events[i].ID == id {
			return &events[i]
		}
	}
	return nil
}

func assertEventEqual(t *testing.T, label string, want, got anomaly.AnomalyFlagEvent) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s ID: got %q, want %q", label, got.ID, want.ID)
	}
	if got.Metric != want.Metric {
		t.Errorf("%s Metric: got %q, want %q", label, got.Metric, want.Metric)
	}
	if got.NodeID != want.NodeID {
		t.Errorf("%s NodeID: got %q, want %q", label, got.NodeID, want.NodeID)
	}
	if got.App != want.App {
		t.Errorf("%s App: got %q, want %q", label, got.App, want.App)
	}
	if got.StreamID != want.StreamID {
		t.Errorf("%s StreamID: got %q, want %q", label, got.StreamID, want.StreamID)
	}
	// Scope must be byte-identical — this is the key invariant for WarmHysteresis
	// key identity (ADR-0009 §3: raw JSON stored byte-for-byte, not re-serialized).
	if got.Scope != want.Scope {
		t.Errorf("%s Scope: got %q, want %q (must be byte-identical)", label, got.Scope, want.Scope)
	}
	// Float64 round-trip via CH.
	if got.Observed != want.Observed {
		t.Errorf("%s Observed: got %v, want %v", label, got.Observed, want.Observed)
	}
	if got.Expected != want.Expected {
		t.Errorf("%s Expected: got %v, want %v", label, got.Expected, want.Expected)
	}
	if got.Sigma != want.Sigma {
		t.Errorf("%s Sigma: got %v, want %v", label, got.Sigma, want.Sigma)
	}
	// DetectedAt: DateTime64(3) = ms precision; must round-trip exactly.
	if !got.DetectedAt.Equal(want.DetectedAt) {
		t.Errorf("%s DetectedAt: got %v, want %v", label, got.DetectedAt, want.DetectedAt)
	}
}

func eventIDs(events []anomaly.AnomalyFlagEvent) []string {
	ids := make([]string, len(events))
	for i, ev := range events {
		ids[i] = ev.ID
	}
	return ids
}
