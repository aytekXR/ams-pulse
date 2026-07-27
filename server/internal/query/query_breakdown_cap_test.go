//go:build integration

// Integration test: J-02 fix — top-N row cap with tail aggregation for
// DeviceBreakdown and GeoBreakdown (region=true path).
//
// These tests MUST run against a real ClickHouse server to exercise the SQL
// path. They seed viewer_sessions with MORE distinct group tuples than the
// cap, then assert:
//  1. The result set length is <= cap + 1 (cap rows + optional tail row)
//  2. When a tail row exists, it is labelled with sentinel values
//  3. The tail row aggregates are mathematically consistent (sum of tail
//     views + capped views = total views across all rows)
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestQuery.*RowCap ./internal/query/... -v -timeout 300s
//
// Prerequisites: /tmp/clickhouse binary (v26.6.1, D-002 no Docker).
package query_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/aytekXR/ams-pulse/server/internal/query"
)

// ─── J-02 Fix: DeviceBreakdown top-N cap ──────────────────────────────────────

// TestQuery_DeviceBreakdown_RowCap seeds 120 distinct device combos (> 100 cap)
// and asserts DeviceBreakdown returns at most 101 rows (100 capped + 1 tail).
func TestQuery_DeviceBreakdown_RowCap(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startClickHouse(t, tmpDir)

	const dbName = "pulse_query_device_cap_test"
	runMigrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	baseTime := time.Now().UTC().Add(-24 * time.Hour)

	// Seed 120 distinct (device, os, browser, protocol) combos.
	// Each combo gets one session with views=1, uniques=1, watch_time_s=10.
	const numCombos = 120
	const cap = 100

	for i := 0; i < numCombos; i++ {
		sessionID := fmt.Sprintf("dev-cap-sess-%03d", i)
		device := fmt.Sprintf("device%03d", i)
		os := fmt.Sprintf("os%03d", i)
		browser := fmt.Sprintf("browser%03d", i)
		protocol := "hls"
		startedAt := baseTime.Add(time.Duration(i) * time.Minute)
		endedAt := startedAt.Add(10 * time.Second)
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.viewer_sessions
				(session_id, stream_id, app, node_id, started_at, ended_at, updated_at,
				 watch_time_s, geo_country, client_device, client_os, client_browser, protocol)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			dbName),
			sessionID, "s1", "live", "n1",
			startedAt, endedAt, endedAt,
			uint32(10), "US", device, os, browser, protocol,
		); err != nil {
			t.Fatalf("seed session %s: %v", sessionID, err)
		}
	}

	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.viewer_sessions FINAL", dbName))

	fakeLive := &fakeLiveProviderQ{}
	qsvc := query.New(fakeLive, conn, nil)

	from := baseTime.Add(-1 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	rows, err := qsvc.DeviceBreakdown(ctx, query.DeviceParams{
		From: from,
		To:   to,
	})
	if err != nil {
		t.Fatalf("DeviceBreakdown: %v", err)
	}

	// ASSERTION 1: result set is capped at 100 + 1 (tail row).
	maxAllowed := cap + 1
	if len(rows) > maxAllowed {
		t.Errorf("J-02 FAIL: DeviceBreakdown returned %d rows, expected at most %d (cap=%d + 1 tail)",
			len(rows), maxAllowed, cap)
	} else {
		t.Logf("DeviceBreakdown returned %d rows (cap=%d + tail)", len(rows), cap)
	}

	// ASSERTION 2: if there's a tail row, it must be labelled with sentinel "other".
	var tailRow *query.DeviceRow
	for i := range rows {
		if rows[i].Device == "other" && rows[i].OS == "other" && rows[i].Browser == "other" {
			tailRow = &rows[i]
			break
		}
	}
	if numCombos > cap {
		if tailRow == nil {
			t.Error("J-02 FAIL: expected a tail row with Device/OS/Browser='other' when combos exceed cap")
		} else {
			t.Logf("Tail row found: views=%d, uniques=%d, watch_time_s=%d",
				tailRow.Views, tailRow.Uniques, tailRow.WatchTimeS)
		}
	}

	// ASSERTION 3: tail aggregates + capped rows' sum = total seeded.
	// Total seeded: 120 sessions, each with views=1, watch_time_s=10.
	expectedTotalViews := int64(numCombos)
	expectedTotalWatchS := int64(numCombos * 10)

	var actualTotalViews, actualTotalWatchS int64
	for _, r := range rows {
		actualTotalViews += r.Views
		actualTotalWatchS += r.WatchTimeS
	}

	if actualTotalViews != expectedTotalViews {
		t.Errorf("J-02 FAIL: total views mismatch: got %d, expected %d (tail aggregation error)",
			actualTotalViews, expectedTotalViews)
	} else {
		t.Logf("Total views correct: %d", actualTotalViews)
	}
	if actualTotalWatchS != expectedTotalWatchS {
		t.Errorf("J-02 FAIL: total watch_time_s mismatch: got %d, expected %d",
			actualTotalWatchS, expectedTotalWatchS)
	}

	t.Logf("PASS J-02: DeviceBreakdown row cap enforced with honest tail aggregation")
}

// ─── J-02 Fix: GeoBreakdown (region=true) top-N cap ───────────────────────────

// TestQuery_GeoBreakdown_RegionRowCap seeds 120 distinct (country, region)
// combos and asserts GeoBreakdown(Region=true) returns at most 101 rows.
func TestQuery_GeoBreakdown_RegionRowCap(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startClickHouse(t, tmpDir)

	const dbName = "pulse_query_geo_cap_test"
	runMigrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	baseTime := time.Now().UTC().Add(-24 * time.Hour)

	// Seed 120 distinct (country, region) combos.
	// Each combo gets one session with views=1, uniques=1, watch_time_s=10.
	const numCombos = 120
	const cap = 100

	for i := 0; i < numCombos; i++ {
		sessionID := fmt.Sprintf("geo-cap-sess-%03d", i)
		country := fmt.Sprintf("C%02d", i%30) // 30 countries
		region := fmt.Sprintf("R%03d", i)     // 120 unique regions
		startedAt := baseTime.Add(time.Duration(i) * time.Minute)
		endedAt := startedAt.Add(10 * time.Second)
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.viewer_sessions
				(session_id, stream_id, app, node_id, started_at, ended_at, updated_at,
				 watch_time_s, geo_country, geo_region, client_device, protocol)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			dbName),
			sessionID, "s1", "live", "n1",
			startedAt, endedAt, endedAt,
			uint32(10), country, region, "desktop", "hls",
		); err != nil {
			t.Fatalf("seed session %s: %v", sessionID, err)
		}
	}

	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.viewer_sessions FINAL", dbName))

	fakeLive := &fakeLiveProviderQ{}
	qsvc := query.New(fakeLive, conn, nil)

	from := baseTime.Add(-1 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	rows, err := qsvc.GeoBreakdown(ctx, query.GeoParams{
		From:   from,
		To:     to,
		Region: true, // Region=true path multiplies key space
	})
	if err != nil {
		t.Fatalf("GeoBreakdown: %v", err)
	}

	// ASSERTION 1: result set is capped at 100 + 1 (tail row).
	maxAllowed := cap + 1
	if len(rows) > maxAllowed {
		t.Errorf("J-02 FAIL: GeoBreakdown(Region=true) returned %d rows, expected at most %d (cap=%d + 1 tail)",
			len(rows), maxAllowed, cap)
	} else {
		t.Logf("GeoBreakdown(Region=true) returned %d rows (cap=%d + tail)", len(rows), cap)
	}

	// ASSERTION 2: if there's a tail row, Country must be sentinel "other".
	var tailRow *query.GeoRow
	for i := range rows {
		if rows[i].Country == "other" {
			tailRow = &rows[i]
			break
		}
	}
	if numCombos > cap {
		if tailRow == nil {
			t.Error("J-02 FAIL: expected a tail row with Country='other' when combos exceed cap")
		} else {
			regionLabel := ""
			if tailRow.Region != nil {
				regionLabel = *tailRow.Region
			}
			t.Logf("Tail row found: country=%s, region=%s, views=%d, uniques=%d, watch_time_s=%d",
				tailRow.Country, regionLabel, tailRow.Views, tailRow.Uniques, tailRow.WatchTimeS)
		}
	}

	// ASSERTION 3: tail + capped rows' sum = total seeded.
	expectedTotalViews := int64(numCombos)
	expectedTotalWatchS := int64(numCombos * 10)

	var actualTotalViews, actualTotalWatchS int64
	for _, r := range rows {
		actualTotalViews += r.Views
		actualTotalWatchS += r.WatchTimeS
	}

	if actualTotalViews != expectedTotalViews {
		t.Errorf("J-02 FAIL: total views mismatch: got %d, expected %d",
			actualTotalViews, expectedTotalViews)
	} else {
		t.Logf("Total views correct: %d", actualTotalViews)
	}
	if actualTotalWatchS != expectedTotalWatchS {
		t.Errorf("J-02 FAIL: total watch_time_s mismatch: got %d, expected %d",
			actualTotalWatchS, expectedTotalWatchS)
	}

	t.Logf("PASS J-02: GeoBreakdown(Region=true) row cap enforced with honest tail aggregation")
}

// NOTE: fakeLiveProviderQ is defined in query_integration_test.go (same package, same build tag).
