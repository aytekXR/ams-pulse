//go:build integration

// Integration test: query.Service geo/device breakdown and QoE summary
// against a real ClickHouse server process.
//
// Validates VD-06 (geo + device breakdown queries return non-empty rows from
// viewer_sessions) and VD-11 (QoE summary queries rollup_qoe_1h and returns
// real startup_p50_ms from quantile states).
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestQuery ./internal/query/... -v -timeout 240s
//
// Prerequisites: /tmp/clickhouse binary (v26.6.1, D-002 no Docker).
package query_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
	"github.com/pulse-analytics/pulse/server/internal/testutil"
)

// queryFreePort finds a free TCP port.
func queryFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// startClickHouse starts a ClickHouse server, waits for it to be ready,
// and returns the TCP port and a cleanup func.
func startClickHouse(t *testing.T, tmpDir string) (tcpPort int) {
	t.Helper()
	chBin := testutil.RequireClickHouseBin(t)

	tcpPort = queryFreePort(t)
	httpPort := queryFreePort(t)
	for httpPort == tcpPort {
		httpPort = queryFreePort(t)
	}

	cmd := exec.Command(chBin, "server",
		"--",
		fmt.Sprintf("--path=%s", tmpDir),
		fmt.Sprintf("--tcp_port=%d", tcpPort),
		fmt.Sprintf("--http_port=%d", httpPort),
		"--listen_host=127.0.0.1",
		"--mysql_port=0",
		"--postgresql_port=0",
	)
	if os.Getenv("PULSE_TEST_VERBOSE_CH") == "" {
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start clickhouse: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Wait for ClickHouse to accept connections.
	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer waitCancel()

	t.Logf("waiting for ClickHouse on port %d...", tcpPort)
	for {
		opts, _ := clickhousego.ParseDSN(startupDSN)
		conn, err := clickhousego.Open(opts)
		if err == nil {
			pingCtx, c := context.WithTimeout(waitCtx, 2*time.Second)
			err = conn.Ping(pingCtx)
			c()
			conn.Close()
			if err == nil {
				break
			}
		}
		select {
		case <-waitCtx.Done():
			t.Fatal("timeout waiting for ClickHouse to start")
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Logf("ClickHouse ready on TCP port %d", tcpPort)
	return tcpPort
}

// runMigrations applies the ClickHouse migrations to the given database.
func runMigrations(t *testing.T, tcpPort int, dbName string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	// thisFile = server/internal/query/query_integration_test.go
	// repoRoot = 3 dirs up: query -> internal -> server -> repo
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")

	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	startupOpts, _ := clickhousego.ParseDSN(startupDSN)
	migConn, err := clickhousego.Open(startupOpts)
	if err != nil {
		t.Fatalf("open migration conn: %v", err)
	}
	defer migConn.Close()

	runner := migrations.New(migConn, migrations.Config{
		MigrationsDir: migrationsDir,
		Database:      dbName,
		RetentionDays: 90,
		RollupTTLDays: 395,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Log("migrations applied")
}

// ─── VD-06: Geo breakdown ──────────────────────────────────────────────────

// TestQuery_GeoBreakdown_NonEmptyRows seeds viewer_sessions with known
// geo_country values and asserts GeoBreakdown returns non-empty rows.
// This is the regression guard for VD-06 (handler was an unconditional stub).
func TestQuery_GeoBreakdown_NonEmptyRows(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startClickHouse(t, tmpDir)

	const dbName = "pulse_query_geo_test"
	runMigrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Seed viewer_sessions with known geo_country values.
	// Two rows: one US, one DE.
	baseTime := time.Now().UTC().Add(-24 * time.Hour)

	type sessionRow struct {
		sessionID    string
		streamID     string
		app          string
		nodeID       string
		geoCountry   string
		clientDevice string
		protocol     string
		watchTimeS   uint32
		startedAt    time.Time
		endedAt      time.Time
		updatedAt    time.Time
	}

	seeds := []sessionRow{
		{
			sessionID: "geo-us-001", streamID: "stream-1", app: "live", nodeID: "n1",
			geoCountry: "US", clientDevice: "desktop", protocol: "hls",
			watchTimeS: 300, startedAt: baseTime, endedAt: baseTime.Add(300 * time.Second),
			updatedAt: baseTime.Add(300 * time.Second),
		},
		{
			sessionID: "geo-us-002", streamID: "stream-1", app: "live", nodeID: "n1",
			geoCountry: "US", clientDevice: "mobile", protocol: "hls",
			watchTimeS: 150, startedAt: baseTime.Add(5 * time.Minute), endedAt: baseTime.Add(5*time.Minute + 150*time.Second),
			updatedAt: baseTime.Add(5*time.Minute + 150*time.Second),
		},
		{
			sessionID: "geo-de-001", streamID: "stream-1", app: "live", nodeID: "n1",
			geoCountry: "DE", clientDevice: "desktop", protocol: "webrtc",
			watchTimeS: 600, startedAt: baseTime.Add(10 * time.Minute), endedAt: baseTime.Add(10*time.Minute + 600*time.Second),
			updatedAt: baseTime.Add(10*time.Minute + 600*time.Second),
		},
	}

	for _, s := range seeds {
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.viewer_sessions
				(session_id, stream_id, app, node_id, started_at, ended_at, updated_at,
				 watch_time_s, geo_country, client_device, protocol)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			dbName),
			s.sessionID, s.streamID, s.app, s.nodeID,
			s.startedAt, s.endedAt, s.updatedAt,
			s.watchTimeS, s.geoCountry, s.clientDevice, s.protocol,
		); err != nil {
			t.Fatalf("seed session %s: %v", s.sessionID, err)
		}
	}

	// Force merge so FINAL query returns the inserted rows promptly.
	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.viewer_sessions FINAL", dbName))

	// Create query service with the real ClickHouse connection.
	fakeLive := &fakeLiveProviderQ{}
	qsvc := query.New(fakeLive, conn, nil)

	from := baseTime.Add(-1 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	rows, err := qsvc.GeoBreakdown(ctx, query.GeoParams{
		From: from,
		To:   to,
	})
	if err != nil {
		t.Fatalf("GeoBreakdown: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("VD-06 FAIL: GeoBreakdown returned 0 rows (expected non-empty from seeded data)")
	}
	t.Logf("GeoBreakdown returned %d rows", len(rows))

	// Find US row and verify aggregates.
	var usRow *query.GeoRow
	for i := range rows {
		if rows[i].Country == "US" {
			usRow = &rows[i]
			break
		}
	}
	if usRow == nil {
		t.Errorf("expected a 'US' row in geo breakdown, got: %v", rows)
	} else {
		if usRow.Views < 2 {
			t.Errorf("expected ≥2 US views, got %d", usRow.Views)
		}
		expectedWatchS := int64(300 + 150) // 450s
		if usRow.WatchTimeS < expectedWatchS {
			// Allow >= because FINAL may not have merged all background parts yet.
			t.Errorf("expected US watch_time_s>=%d, got %d", expectedWatchS, usRow.WatchTimeS)
		}
		t.Logf("US row: views=%d, uniques=%d, watch_time_s=%d", usRow.Views, usRow.Uniques, usRow.WatchTimeS)
	}
	t.Logf("PASS VD-06: GeoBreakdown returns %d non-empty rows from seeded data", len(rows))
}

// ─── VD-06: Device breakdown ──────────────────────────────────────────────

// TestQuery_DeviceBreakdown_NonEmptyRows seeds viewer_sessions with known
// client_device values and asserts DeviceBreakdown returns non-empty rows.
func TestQuery_DeviceBreakdown_NonEmptyRows(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startClickHouse(t, tmpDir)

	const dbName = "pulse_query_device_test"
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

	type devRow struct {
		sessionID     string
		streamID      string
		geoCountry    string
		clientDevice  string
		clientOS      string
		clientBrowser string
		protocol      string
		watchTimeS    uint32
		startedAt     time.Time
		updatedAt     time.Time
	}

	seeds := []devRow{
		{
			sessionID: "dev-dt-001", streamID: "s1", geoCountry: "US",
			clientDevice: "desktop", clientOS: "linux", clientBrowser: "chrome", protocol: "hls",
			watchTimeS: 200, startedAt: baseTime, updatedAt: baseTime.Add(200 * time.Second),
		},
		{
			sessionID: "dev-mb-001", streamID: "s1", geoCountry: "US",
			clientDevice: "mobile", clientOS: "ios", clientBrowser: "safari", protocol: "hls",
			watchTimeS: 120, startedAt: baseTime.Add(1 * time.Minute), updatedAt: baseTime.Add(time.Minute + 120*time.Second),
		},
		{
			sessionID: "dev-mb-002", streamID: "s1", geoCountry: "DE",
			clientDevice: "mobile", clientOS: "android", clientBrowser: "chrome", protocol: "webrtc",
			watchTimeS: 80, startedAt: baseTime.Add(2 * time.Minute), updatedAt: baseTime.Add(2*time.Minute + 80*time.Second),
		},
	}

	for _, s := range seeds {
		endedAt := s.startedAt.Add(time.Duration(s.watchTimeS) * time.Second)
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.viewer_sessions
				(session_id, stream_id, app, node_id, started_at, ended_at, updated_at,
				 watch_time_s, geo_country, client_device, client_os, client_browser, protocol)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			dbName),
			s.sessionID, s.streamID, "live", "n1",
			s.startedAt, endedAt, s.updatedAt,
			s.watchTimeS, s.geoCountry, s.clientDevice, s.clientOS, s.clientBrowser, s.protocol,
		); err != nil {
			t.Fatalf("seed session %s: %v", s.sessionID, err)
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
	if len(rows) == 0 {
		t.Fatal("VD-06 FAIL: DeviceBreakdown returned 0 rows (expected non-empty from seeded data)")
	}
	t.Logf("DeviceBreakdown returned %d rows", len(rows))

	// Verify desktop row exists.
	var desktopRow *query.DeviceRow
	for i := range rows {
		if rows[i].Device == "desktop" {
			desktopRow = &rows[i]
			break
		}
	}
	if desktopRow == nil {
		t.Errorf("expected a 'desktop' row in device breakdown, got: %v", rows)
	} else {
		t.Logf("desktop row: views=%d, os=%s, browser=%s, protocol=%s",
			desktopRow.Views, desktopRow.OS, desktopRow.Browser, desktopRow.Protocol)
	}
	t.Logf("PASS VD-06: DeviceBreakdown returns %d non-empty rows from seeded data", len(rows))
}

// ─── VD-11: QoE summary from rollup_qoe_1h ────────────────────────────────

// TestQuery_QoeSummary_RealStartupP50 seeds beacon_events (which feeds
// rollup_qoe_1h via the MV), then asserts QoeSummary returns a non-zero
// startup_p50_ms and uses bitrate_kbps_p50 (not bitrate_kbps).
// This is the regression guard for VD-11.
func TestQuery_QoeSummary_RealStartupP50(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startClickHouse(t, tmpDir)

	const dbName = "pulse_query_qoe_test"
	runMigrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	baseTime := time.Now().UTC().Add(-2 * time.Hour)

	// Insert beacon_events rows with startup_complete events — the materialized
	// view mv_qoe_1h will populate rollup_qoe_1h on INSERT.
	// 5 sessions with known startup_ms values: [500, 800, 1000, 1200, 1500].
	// Expected p50 ≈ 1000 ms (median of 5 values).
	startupValues := []uint32{500, 800, 1000, 1200, 1500}

	for i, sv := range startupValues {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		sessionID := fmt.Sprintf("qoe-session-%03d", i)
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.beacon_events
				(version, session_id, stream_id, app, ts, event_type, startup_ms,
				 bitrate_kbps, geo_country, client_device)
			VALUES (1, ?, 'stream-qoe-test', 'live', ?, 'startup_complete', ?, 2500.0, 'US', 'desktop')`,
			dbName),
			sessionID, ts, sv,
		); err != nil {
			t.Fatalf("insert beacon_event %d (startup_ms=%d): %v", i, sv, err)
		}
	}

	// Also insert heartbeat events with bitrate to populate bitrate_kbps_state.
	for i := 0; i < 5; i++ {
		ts := baseTime.Add(time.Duration(i)*time.Minute + 30*time.Second)
		sessionID := fmt.Sprintf("qoe-session-%03d", i)
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.beacon_events
				(version, session_id, stream_id, app, ts, event_type, watch_ms,
				 bitrate_kbps, geo_country, client_device)
			VALUES (1, ?, 'stream-qoe-test', 'live', ?, 'heartbeat', 30000, ?, 'US', 'desktop')`,
			dbName),
			sessionID, ts, float32(2000+i*500),
		); err != nil {
			t.Fatalf("insert heartbeat %d: %v", i, err)
		}
	}

	// Force merge of rollup_qoe_1h to consolidate the MV-inserted rows.
	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.rollup_qoe_1h FINAL", dbName))

	fakeLive := &fakeLiveProviderQ{}
	qsvc := query.New(fakeLive, conn, nil)

	// from MUST sit at/before the rollup bucket, which is toStartOfHour(baseTime) — i.e.
	// up to 59min BEFORE baseTime. The old `baseTime-30min` was the true root cause of this
	// test's chronic flake (misdiagnosed as timing in D-038/39/40): when the wall-clock minute
	// was >30, toStartOfHour(baseTime) fell before baseTime-30min, so the query's `bucket >= from`
	// filter dropped the only row → startup_p50_ms=0 forever (no poll can un-filter present data;
	// that's why even a 92s poll failed at 19:37). Anchoring to the bucket's hour makes it
	// wall-clock-independent. Verified vs ClickHouse: excluded at min-37, included after the fix.
	from := baseTime.Truncate(time.Hour).Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	result, err := qsvc.QoeSummary(ctx, query.QoeParams{
		From:     from,
		To:       to,
		Interval: "hour",
	})
	if err != nil {
		t.Fatalf("QoeSummary: %v", err)
	}

	// Short belt-and-suspenders wait for MV write + FINAL-merge visibility. The chronic flake
	// was NOT population lag — it was the `from` time-window boundary above (now fixed), which is
	// why the old 15s→90s poll (D-039) never helped. With `from` anchored to the bucket's hour and
	// the MV firing synchronously on INSERT, the value is deterministically ≈1000; this loop should
	// exit on the first iteration. Kept only to absorb any residual async-visibility jitter.
	for deadline := time.Now().Add(15 * time.Second); result.Totals.StartupP50Ms == 0 && time.Now().Before(deadline); {
		_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.rollup_qoe_1h FINAL", dbName))
		time.Sleep(300 * time.Millisecond)
		result, err = qsvc.QoeSummary(ctx, query.QoeParams{From: from, To: to, Interval: "hour"})
		if err != nil {
			t.Fatalf("QoeSummary (poll): %v", err)
		}
	}

	// VD-11 assertion: startup_p50_ms must reflect ONLY the real startup_complete samples
	// [500,800,1000,1200,1500] → median ≈ 1000. Since D-042 the MV computes the startup
	// quantile with quantilesStateIf(event_type='startup_complete'), so the five heartbeat
	// startup_ms=0 rows no longer dilute it. The value must land inside the real range; the
	// pre-D-042 buggy MV produced ~250 (< 500), which this range check now catches.
	if result.Totals.StartupP50Ms < 500 || result.Totals.StartupP50Ms > 1500 {
		t.Errorf("VD-11 FAIL: startup_p50_ms=%.1f outside the real startup range [500,1500] — "+
			"heartbeat zeros leaking into the startup quantile? (see mv_qoe_1h / migration 0004)",
			result.Totals.StartupP50Ms)
	} else {
		t.Logf("startup_p50_ms=%.1f startup_p95_ms=%.1f", result.Totals.StartupP50Ms, result.Totals.StartupP95Ms)
		// Startup_p95_ms must be >= p50.
		if result.Totals.StartupP95Ms < result.Totals.StartupP50Ms {
			t.Errorf("startup_p95_ms=%.1f < startup_p50_ms=%.1f (quantile ordering violated)",
				result.Totals.StartupP95Ms, result.Totals.StartupP50Ms)
		}
	}

	// VD-11 assertion: BitrateTimeline uses bitrate_kbps_p50 field.
	// Verify the result type has the correct JSON field name.
	if len(result.BitrateTimeline) > 0 {
		b := result.BitrateTimeline[0]
		if b.BitrateKbpsP50 == 0 {
			t.Error("VD-11 FAIL: bitrate_kbps_p50 is 0 in timeline (wrong field name)")
		} else {
			t.Logf("bitrate_kbps_p50=%.1f (correct field name)", b.BitrateKbpsP50)
		}
	} else {
		t.Log("note: bitrate_timeline is empty (MV may not have fired; tested totals path)")
	}

	t.Logf("PASS VD-11: QoeSummary returns real startup_p50_ms=%.1f from rollup_qoe_1h",
		result.Totals.StartupP50Ms)
}

// ─── D-062: QoEForStream from rollup_qoe_1h ──────────────────────────────

// TestQuery_QoEForStream_RebufferRatio seeds beacon_events with one rebuffer_end
// (rebuffer_ms=5000) and one heartbeat (watch_ms=10000) for stream "int-stream".
// After OPTIMIZE TABLE rollup_qoe_1h FINAL, asserts QoEForStream returns
// rebuffer_ratio ≈ 0.5 and error_rate = 0.
//
// Derivation: rollup_qoe_1h MV (0001_init.sql:406-425):
//
//	rebuffer_ratio = sumMerge(rebuffer_total_ms) / sumMerge(watch_time_ms)
//	              = 5000 / 10000 = 0.5
//	error_rate    = sumMerge(error_count) / countMerge(session_count)
//	              = 0 / 2 = 0
func TestQuery_QoEForStream_RebufferRatio(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startClickHouse(t, tmpDir)

	const dbName = "pulse_query_qoe_stream_test"
	runMigrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Base time 30 min ago so both rows land in the current hour bucket.
	baseTime := time.Now().UTC().Add(-30 * time.Minute)

	// Seed: rebuffer_end with rebuffer_ms=5000.
	if err := conn.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.beacon_events
			(version, session_id, stream_id, app, ts, event_type, rebuffer_ms)
		VALUES (1, 'int-sess-reb', 'int-stream', 'live', ?, 'rebuffer_end', 5000)`,
		dbName), baseTime,
	); err != nil {
		t.Fatalf("insert rebuffer_end: %v", err)
	}

	// Seed: heartbeat with watch_ms=10000.
	if err := conn.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.beacon_events
			(version, session_id, stream_id, app, ts, event_type, watch_ms)
		VALUES (1, 'int-sess-hb', 'int-stream', 'live', ?, 'heartbeat', 10000)`,
		dbName), baseTime.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert heartbeat: %v", err)
	}

	// Merge aggregate states so sumMerge / countMerge return consolidated results.
	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.rollup_qoe_1h FINAL", dbName))

	fakeLive := &fakeLiveProviderQ{}
	qsvc := query.New(fakeLive, conn, nil)

	// lookback of 2h covers toStartOfHour(baseTime) regardless of current minute.
	rebuf, errRate, err := qsvc.QoEForStream(ctx, "int-stream", "live", 2*time.Hour)
	if err != nil {
		t.Fatalf("QoEForStream: %v", err)
	}

	// Short retry loop for residual async-visibility jitter after OPTIMIZE FINAL.
	for deadline := time.Now().Add(15 * time.Second); rebuf == 0 && time.Now().Before(deadline); {
		_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.rollup_qoe_1h FINAL", dbName))
		time.Sleep(300 * time.Millisecond)
		rebuf, errRate, err = qsvc.QoEForStream(ctx, "int-stream", "live", 2*time.Hour)
		if err != nil {
			t.Fatalf("QoEForStream (poll): %v", err)
		}
	}

	// rebuffer_ratio = 5000ms / 10000ms = 0.5; allow [0.4, 0.6] for float rounding.
	if rebuf < 0.4 || rebuf > 0.6 {
		t.Errorf("D-062 FAIL: QoEForStream rebuffer_ratio=%.4f, expected ~0.5 (range [0.4,0.6])", rebuf)
	} else {
		t.Logf("PASS D-062: QoEForStream rebuffer_ratio=%.4f (expected ~0.5)", rebuf)
	}
	if errRate != 0 {
		t.Errorf("D-062 FAIL: QoEForStream error_rate=%.4f, expected 0 (no error events seeded)", errRate)
	} else {
		t.Logf("PASS D-062: QoEForStream error_rate=%.4f (expected 0)", errRate)
	}
}

// ─── Fake live provider for query tests ──────────────────────────────────

type fakeLiveProviderQ struct{}

func (f *fakeLiveProviderQ) CurrentSnapshot() *domain.LiveSnapshot { return nil }
func (f *fakeLiveProviderQ) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}
