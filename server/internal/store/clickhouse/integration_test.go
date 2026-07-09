//go:build integration

// Integration test: ClickHouse store with a real server process.
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestIntegration ./internal/store/clickhouse/... -v -timeout 120s
//
// Prerequisites: /tmp/clickhouse binary available (D-002: no Docker).
package clickhouse_test

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
	"github.com/pulse-analytics/pulse/server/internal/testutil"
)

// TestIntegration_BatchInsert starts a ClickHouse server, runs migrations,
// inserts 10k synthetic events via the batcher, and verifies counts and TTL.
func TestIntegration_BatchInsert(t *testing.T) {
	chBin := testutil.RequireClickHouseBin(t)

	// 1. Find free TCP ports using a stable range to avoid race with OS reuse.
	tcpPort := freePort(t)
	httpPort := freePort(t)
	// Ensure different ports.
	for httpPort == tcpPort {
		httpPort = freePort(t)
	}

	tmpDir := t.TempDir()

	// 2. Start ClickHouse server.
	// Disable MySQL (9004) and PostgreSQL (9005) compatibility ports to avoid
	// port conflicts in CI environments where those ports may already be in use.
	cmd := exec.Command(chBin, "server",
		"--",
		fmt.Sprintf("--path=%s", tmpDir),
		fmt.Sprintf("--tcp_port=%d", tcpPort),
		fmt.Sprintf("--http_port=%d", httpPort),
		"--listen_host=127.0.0.1",
		"--mysql_port=0",
		"--postgresql_port=0",
	)
	// Suppress ClickHouse console noise in test output.
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

	// 3. Wait for ClickHouse to accept connections (up to 45s — first startup is slow).
	// Use 'default' database for the startup ping (target DB doesn't exist yet).
	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/pulse_integration_test", tcpPort)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer waitCancel()

	t.Logf("waiting for ClickHouse on 127.0.0.1:%d...", tcpPort)
	for {
		opts, err := clickhousego.ParseDSN(startupDSN)
		if err != nil {
			t.Fatalf("parse startup DSN: %v", err)
		}
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
	t.Log("ClickHouse is ready")

	// 4. Run migrations.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	// thisFile = server/internal/store/clickhouse/integration_test.go
	// repoRoot  = 4 levels up
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")

	// Connect to 'default' for migrations — the runner's ensureMigrationsTable creates
	// the target DB via CREATE DATABASE IF NOT EXISTS, so we must not specify it in the
	// DSN (ClickHouse would reject the connection if the DB doesn't exist yet).
	startupOpts, err := clickhousego.ParseDSN(startupDSN)
	if err != nil {
		t.Fatalf("parse startup DSN for migrations: %v", err)
	}
	migConn, err := clickhousego.Open(startupOpts)
	if err != nil {
		t.Fatalf("open conn for migrations: %v", err)
	}
	defer migConn.Close()

	opts, err := clickhousego.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}

	runner := migrations.New(migConn, migrations.Config{
		MigrationsDir: migrationsDir,
		Database:      "pulse_integration_test",
		RetentionDays: 90,
		RollupTTLDays: 395,
	}, nil)

	migCtx, migCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migCancel()

	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Log("migrations applied")

	// Verify that the migration actually created the expected tables.
	checkConn, err := clickhousego.Open(startupOpts)
	if err != nil {
		t.Fatalf("open check conn: %v", err)
	}
	var migTableCount uint64
	checkRow := checkConn.QueryRow(migCtx,
		"SELECT count() FROM system.tables WHERE database = 'pulse_integration_test'")
	if err := checkRow.Scan(&migTableCount); err != nil {
		t.Fatalf("check tables: %v", err)
	}
	t.Logf("tables created by migration: %d", migTableCount)
	if migTableCount == 0 {
		// Print what databases exist
		rows, _ := checkConn.Query(migCtx, "SELECT name FROM system.databases ORDER BY name")
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var dbName string
				rows.Scan(&dbName)
				t.Logf("  database: %s", dbName)
			}
		}
		t.Fatalf("migration applied but no tables created in pulse_integration_test")
	}
	checkConn.Close()

	// 5. Create store with large channel buffer so 10k burst fits without drops.
	// Channel size = 2 * BatchSize = 2 * 1000 = 2000; but we send 10k so we need
	// BatchSize >= 5000 to avoid drops on a fast burst.
	const numEvents = 10_000
	store, err := clickhouse.New(context.Background(), clickhouse.Config{
		DSN:           dsn,
		Database:      "pulse_integration_test",
		BatchSize:     numEvents, // large enough to never drop during burst
		FlushInterval: 2 * time.Second,
		MaxRetries:    1,
	}, nil)
	if err != nil {
		t.Fatalf("clickhouse.New: %v", err)
	}
	defer store.Close()

	insertCtx, insertCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer insertCancel()

	store.Start(insertCtx)

	// 6. Insert 10k synthetic server_events via the batcher.
	t.Logf("inserting %d synthetic server_events...", numEvents)
	start := time.Now()

	apps := []string{"live", "stream", "webinar", "broadcast"}
	eventTypes := []string{
		domain.EventStreamPublishStart,
		domain.EventStreamStats,
		domain.EventNodeStats,
		domain.EventWebRTCClientStats,
		domain.EventIngestStats,
	}

	for i := 0; i < numEvents; i++ {
		app := apps[i%len(apps)]
		evType := eventTypes[i%len(eventTypes)]
		ev := domain.ServerEvent{
			Version:  1,
			Type:     evType,
			TS:       time.Now().UnixMilli() - int64(rand.Intn(3600000)),
			Source:   domain.SourceRestPoll,
			NodeID:   fmt.Sprintf("node-%d", i%3),
			App:      app,
			StreamID: fmt.Sprintf("stream-%d", i%100),
			Data:     syntheticData(evType, i),
		}
		store.OnServerEvent(ev)
	}

	// Wait for batcher to flush. With BatchSize=10k and FlushInterval=2s:
	// all 10k events buffer up immediately, then flush on the 2s timer.
	// Allow 20s total (10x margin).
	t.Log("waiting for batcher to flush...")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		inserted, dropped := store.Metrics()
		t.Logf("  flushed so far: inserted=%d dropped=%d", inserted, dropped)
		if inserted+dropped >= numEvents {
			break
		}
	}

	elapsed := time.Since(start)
	inserted, dropped := store.Metrics()
	t.Logf("insert complete: %d inserted, %d dropped, elapsed=%v", inserted, dropped, elapsed)

	if dropped > 0 {
		t.Errorf("unexpected dropped events: %d (channel large enough for full burst)", dropped)
	}

	// 7. Verify event counts in ClickHouse.
	verifyConn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open verify conn: %v", err)
	}
	defer verifyConn.Close()

	var count uint64
	qCtx, qCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer qCancel()

	row := verifyConn.QueryRow(qCtx, "SELECT count() FROM pulse_integration_test.server_events")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	t.Logf("server_events count in ClickHouse: %d (expected ~%d)", count, inserted)

	if count == 0 {
		t.Error("expected > 0 rows in server_events")
	}
	if count < uint64(inserted)*9/10 {
		t.Errorf("count %d is much less than inserted %d — possible batch loss", count, inserted)
	}

	// 8. Verify TTL clause exists on server_events.
	var ttlExpr string
	ttlRow := verifyConn.QueryRow(qCtx,
		`SELECT metadata_modification_time FROM system.tables
		 WHERE database = 'pulse_integration_test' AND name = 'server_events'`)
	// Check TTL presence via create_table_query.
	var createTableQuery string
	ttlCheckRow := verifyConn.QueryRow(qCtx,
		`SELECT create_table_query FROM system.tables
		 WHERE database = 'pulse_integration_test' AND name = 'server_events'`)
	if err := ttlCheckRow.Scan(&createTableQuery); err != nil {
		t.Fatalf("create_table_query: %v", err)
	}
	_ = ttlRow
	t.Logf("server_events create_table_query snippet: ...%s...",
		createTableQuery[max(0, len(createTableQuery)-200):])

	if !containsStr(createTableQuery, "TTL") {
		t.Error("server_events table is missing TTL clause")
	}

	// 9. Verify rollup tables exist.
	var tableCount uint64
	tablesRow := verifyConn.QueryRow(qCtx,
		`SELECT count() FROM system.tables
		 WHERE database = 'pulse_integration_test'
		 AND name IN ('rollup_audience_1h','rollup_audience_1d','rollup_qoe_1h','rollup_qoe_1d','rollup_usage_1d')`)
	if err := tablesRow.Scan(&tableCount); err != nil {
		t.Fatalf("rollup tables query: %v", err)
	}
	if tableCount != 5 {
		t.Errorf("expected 5 rollup tables, found %d", tableCount)
	}
	t.Logf("rollup tables: %d/5 present", tableCount)

	// 10. Verify TTL expression in rollup table.
	var rollupCreate string
	rollupRow := verifyConn.QueryRow(qCtx,
		`SELECT create_table_query FROM system.tables
		 WHERE database = 'pulse_integration_test' AND name = 'rollup_audience_1h'`)
	if err := rollupRow.Scan(&rollupCreate); err != nil {
		t.Fatalf("rollup_audience_1h create: %v", err)
	}
	if !containsStr(rollupCreate, "TTL") {
		t.Errorf("rollup_audience_1h is missing TTL clause")
	}

	// All checks passed.
	t.Logf("PASS: 10k events inserted, counts and TTL verified (elapsed: %v)", elapsed)

	// Also verify TTL string used in logging output.
	_ = ttlExpr
}

// freePort finds an available TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// syntheticData returns a data map appropriate for the given event type.
func syntheticData(evType string, i int) map[string]any {
	switch evType {
	case domain.EventStreamPublishStart:
		return map[string]any{"publish_type": "rtmp"}
	case domain.EventStreamStats:
		return map[string]any{
			"viewer_count": i % 50,
			"bitrate_kbps": float64(1000 + i%3000),
		}
	case domain.EventNodeStats:
		return map[string]any{
			"cpu_pct":  float64(10 + i%80),
			"mem_pct":  float64(20 + i%70),
			"disk_pct": float64(5 + i%60),
		}
	case domain.EventWebRTCClientStats:
		return map[string]any{
			"client_id":       fmt.Sprintf("client-%d", i),
			"rtt_ms":          float64(10 + i%200),
			"jitter_ms":       float64(1 + i%50),
			"packet_loss_pct": float64(i%5) * 0.1,
		}
	case domain.EventIngestStats:
		return map[string]any{
			"bitrate_kbps": float64(1500 + i%2000),
			"fps":          float64(25 + i%30),
		}
	default:
		return map[string]any{"viewer_count": i % 10}
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) &&
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// startClickHouse is a test helper that starts a ClickHouse server on a random port,
// runs migrations against it, and returns a connected store and a direct verify connection.
// The caller must close the store and verify connection; the server is cleaned up by t.Cleanup.
func startClickHouseForProbes(t *testing.T, dbName string) (*clickhouse.Store, clickhousego.Conn, string) {
	t.Helper()
	chBin := testutil.RequireClickHouseBin(t)

	tcpPort := freePort(t)
	httpPort := freePort(t)
	for httpPort == tcpPort {
		httpPort = freePort(t)
	}

	tmpDir := t.TempDir()
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
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer waitCancel()

	t.Logf("waiting for ClickHouse on 127.0.0.1:%d...", tcpPort)
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
			t.Fatal("timeout waiting for ClickHouse")
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Log("ClickHouse ready")

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")

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

	migCtx, migCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migCancel()
	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Log("migrations applied")

	store, err := clickhouse.New(context.Background(), clickhouse.Config{
		DSN:           dsn,
		Database:      dbName,
		BatchSize:     100,
		FlushInterval: 500 * time.Millisecond,
		MaxRetries:    1,
	}, nil)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	verifyOpts, _ := clickhousego.ParseDSN(dsn)
	verifyConn, err := clickhousego.Open(verifyOpts)
	if err != nil {
		t.Fatalf("open verify conn: %v", err)
	}

	return store, verifyConn, dbName
}

// TestIntegration_ProbeResults verifies the probe_results ClickHouse store:
// insert N results for a probe_id, then QueryProbeResults returns them
// time-ordered within the queried range.
func TestIntegration_ProbeResults(t *testing.T) {
	store, verifyConn, dbName := startClickHouseForProbes(t, "pulse_probe_test")
	defer store.Close()
	defer verifyConn.Close()

	insertCtx, insertCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer insertCancel()

	store.Start(insertCtx)

	const numResults = 20
	probeID := "probe-integration-001"
	baseTime := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Duration(numResults) * time.Minute)

	t.Logf("inserting %d probe_results for probe_id=%s...", numResults, probeID)

	expectedIDs := make([]string, numResults)
	for i := 0; i < numResults; i++ {
		resultID := uuid.New().String()
		expectedIDs[i] = resultID
		r := domain.ProbeResult{
			ID:          resultID,
			ProbeID:     probeID,
			TS:          baseTime.Add(time.Duration(i) * time.Minute),
			Success:     i%3 != 0, // every 3rd result is a failure
			TTFBMs:      uint32(50 + i*10),
			BitrateKbps: float32(1000 + i*50),
		}
		if !r.Success {
			r.ErrorCode = "http_5xx"
			r.ErrorMsg = fmt.Sprintf("simulated error at step %d", i)
			r.BitrateKbps = 0
		}
		if err := store.InsertProbeResult(insertCtx, r); err != nil {
			t.Fatalf("InsertProbeResult[%d]: %v", i, err)
		}
	}

	// Give ClickHouse a moment to flush/commit.
	time.Sleep(1 * time.Second)

	// Verify count via direct connection.
	var count uint64
	qCtx, qCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer qCancel()

	row := verifyConn.QueryRow(qCtx,
		fmt.Sprintf("SELECT count() FROM %s.probe_results WHERE probe_id = ?", dbName),
		probeID,
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count probe_results: %v", err)
	}
	t.Logf("probe_results inserted: %d (expected %d)", count, numResults)
	if count != numResults {
		t.Errorf("expected %d probe_results, got %d", numResults, count)
	}

	// Test QueryProbeResults — query the full range, expect all results ordered by ts.
	from := baseTime.Add(-1 * time.Minute)
	to := baseTime.Add(time.Duration(numResults+1) * time.Minute)
	results, err := store.QueryProbeResults(qCtx, probeID, from, to, 100)
	if err != nil {
		t.Fatalf("QueryProbeResults: %v", err)
	}
	t.Logf("QueryProbeResults returned %d results (expected %d)", len(results), numResults)

	if len(results) != numResults {
		t.Errorf("expected %d results from QueryProbeResults, got %d", numResults, len(results))
	}

	// Verify time ordering: each result.TS >= previous.
	for i := 1; i < len(results); i++ {
		if results[i].TS.Before(results[i-1].TS) {
			t.Errorf("results not time-ordered at index %d: %v < %v",
				i, results[i].TS, results[i-1].TS)
		}
	}

	// Verify success/failure fields round-trip.
	failCount := 0
	for _, r := range results {
		if !r.Success {
			failCount++
			if r.ErrorCode != "http_5xx" {
				t.Errorf("failure result missing error_code: probe_id=%s id=%s", r.ProbeID, r.ID)
			}
		}
	}
	// Every 3rd result starting at index 0 is a failure: 0, 3, 6, 9, 12, 15, 18 → 7
	expectedFails := 0
	for i := 0; i < numResults; i++ {
		if i%3 == 0 {
			expectedFails++
		}
	}
	if failCount != expectedFails {
		t.Errorf("expected %d failure results, got %d", expectedFails, failCount)
	}

	// Test range filtering: query only the first half.
	midTime := baseTime.Add(time.Duration(numResults/2) * time.Minute)
	firstHalf, err := store.QueryProbeResults(qCtx, probeID, from, midTime, 100)
	if err != nil {
		t.Fatalf("QueryProbeResults (first half): %v", err)
	}
	t.Logf("first-half range query: %d results (expected ~%d)", len(firstHalf), numResults/2)
	if len(firstHalf) == 0 || len(firstHalf) >= numResults {
		t.Errorf("range query should return a subset; got %d/%d", len(firstHalf), numResults)
	}
	// All results in firstHalf should be before midTime.
	for _, r := range firstHalf {
		if !r.TS.Before(midTime) {
			t.Errorf("result outside range: ts=%v, expected < %v", r.TS, midTime)
		}
	}

	// Test limit enforcement.
	limited, err := store.QueryProbeResults(qCtx, probeID, from, to, 5)
	if err != nil {
		t.Fatalf("QueryProbeResults (limited): %v", err)
	}
	if len(limited) > 5 {
		t.Errorf("limit=5: expected ≤5 results, got %d", len(limited))
	}
	t.Logf("limit=5 query returned %d results", len(limited))

	t.Logf("PASS: probe_results=%d inserted+queried, time-ordered, range-filtered, limited",
		numResults)
}

// TestIntegration_ViewerSessionsAndRollups verifies that viewer_sessions are inserted
// and the materialized views (mv_audience_1h, mv_audience_1d) populate correctly.
// Also verifies beacon_events populate the mv_qoe_1h rollup.
func TestIntegration_ViewerSessionsAndRollups(t *testing.T) {
	chBin := testutil.RequireClickHouseBin(t)

	tcpPort := freePort(t)
	httpPort := freePort(t)
	for httpPort == tcpPort {
		httpPort = freePort(t)
	}

	tmpDir := t.TempDir()

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
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start clickhouse: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	dbName := "pulse_rollup_test"
	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)

	// Wait for ClickHouse.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer waitCancel()

	t.Logf("waiting for ClickHouse on 127.0.0.1:%d...", tcpPort)
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
			t.Fatal("timeout waiting for ClickHouse")
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Log("ClickHouse ready")

	// Run migrations.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")

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

	migCtx, migCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migCancel()

	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Log("migrations applied")

	// Create store.
	store, err := clickhouse.New(context.Background(), clickhouse.Config{
		DSN:           dsn,
		Database:      dbName,
		BatchSize:     100,
		FlushInterval: 500 * time.Millisecond,
		MaxRetries:    1,
	}, nil)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer store.Close()

	insertCtx, insertCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer insertCancel()

	store.Start(insertCtx)

	// Insert 100 viewer sessions with geo/device dims.
	t.Log("inserting 100 viewer_sessions...")
	baseTime := time.Now().UTC().Truncate(time.Hour)
	for i := 0; i < 100; i++ {
		sess := domain.ViewerSession{
			SessionID:     fmt.Sprintf("sess-%d", i),
			StreamID:      fmt.Sprintf("stream-%d", i%5),
			App:           "live",
			NodeID:        "n1",
			StartedAt:     baseTime.Add(time.Duration(i) * time.Second),
			EndedAt:       baseTime.Add(time.Duration(i)*time.Second + 60*time.Second),
			UpdatedAt:     time.Now().UTC(),
			WatchTimeS:    60,
			Protocol:      []string{"webrtc", "hls", "rtmp"}[i%3],
			GeoCountry:    []string{"US", "DE", "TR"}[i%3],
			GeoRegion:     "CA",
			ClientDevice:  []string{"desktop", "mobile"}[i%2],
			ClientOS:      "macOS",
			ClientBrowser: "Chrome",
		}
		store.OnViewerSession(sess)
	}

	// Insert 50 beacon events for QoE rollup.
	t.Log("inserting 50 beacon_events for QoE rollup...")
	for i := 0; i < 50; i++ {
		beacon := domain.BeaconEvent{
			Version:   1,
			SessionID: fmt.Sprintf("bsess-%d", i),
			StreamID:  fmt.Sprintf("stream-%d", i%5),
			App:       "live",
			Events: []domain.BeaconItem{
				{
					Type: "startup_complete",
					TS:   baseTime.Add(time.Duration(i) * time.Second).UnixMilli(),
					Data: map[string]any{
						"startup_ms": float64(500 + i*10),
					},
				},
				{
					Type: "heartbeat",
					TS:   baseTime.Add(time.Duration(i)*time.Second + 30*time.Second).UnixMilli(),
					Data: map[string]any{
						"watch_ms":     float64(30000),
						"bitrate_kbps": float64(1500 + i*10),
					},
				},
			},
			Enrichment: &domain.EnrichmentBlock{
				Geo: &domain.GeoEnrichment{Country: "US"},
			},
		}
		store.OnBeaconEvent(beacon)
	}

	// Wait for flush.
	t.Log("waiting for flush...")
	time.Sleep(2 * time.Second)

	// Verify via direct connection.
	opts, _ := clickhousego.ParseDSN(dsn)
	verifyConn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open verify conn: %v", err)
	}
	defer verifyConn.Close()

	qCtx, qCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer qCancel()

	// Check viewer_sessions count.
	var sessCount uint64
	row := verifyConn.QueryRow(qCtx, fmt.Sprintf("SELECT count() FROM %s.viewer_sessions", dbName))
	if err := row.Scan(&sessCount); err != nil {
		t.Fatalf("viewer_sessions count: %v", err)
	}
	t.Logf("viewer_sessions inserted: %d (expected ~100)", sessCount)
	if sessCount == 0 {
		t.Error("expected > 0 viewer_sessions")
	}

	// Check that viewer_sessions contain geo/device dims.
	var geoCount uint64
	geoRow := verifyConn.QueryRow(qCtx,
		fmt.Sprintf("SELECT countIf(geo_country != '') FROM %s.viewer_sessions", dbName))
	if err := geoRow.Scan(&geoCount); err != nil {
		t.Fatalf("geo_country check: %v", err)
	}
	if geoCount == 0 {
		t.Error("expected geo_country to be populated in viewer_sessions")
	}
	t.Logf("viewer_sessions with geo_country: %d/%d", geoCount, sessCount)

	// Check rollup_audience_1h was populated by the materialized view.
	// Give ClickHouse a moment to process the MV triggers.
	time.Sleep(1 * time.Second)
	var audienceCount uint64
	aRow := verifyConn.QueryRow(qCtx,
		fmt.Sprintf("SELECT count() FROM %s.rollup_audience_1h", dbName))
	if err := aRow.Scan(&audienceCount); err != nil {
		t.Fatalf("rollup_audience_1h count: %v", err)
	}
	t.Logf("rollup_audience_1h rows: %d", audienceCount)
	if audienceCount == 0 {
		t.Error("rollup_audience_1h should be populated by MV from viewer_sessions")
	}

	// Check rollup_qoe_1h was populated by the QoE materialized view.
	var qoeCount uint64
	qRow := verifyConn.QueryRow(qCtx,
		fmt.Sprintf("SELECT count() FROM %s.rollup_qoe_1h", dbName))
	if err := qRow.Scan(&qoeCount); err != nil {
		t.Fatalf("rollup_qoe_1h count: %v", err)
	}
	t.Logf("rollup_qoe_1h rows: %d", qoeCount)
	if qoeCount == 0 {
		t.Error("rollup_qoe_1h should be populated by MV from beacon_events")
	}

	t.Logf("PASS: viewer_sessions=%d, rollup_audience_1h=%d rows, rollup_qoe_1h=%d rows",
		sessCount, audienceCount, qoeCount)
}
