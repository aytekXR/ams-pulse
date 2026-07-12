//go:build integration

// Integration test: mv_recording_1d materialized view populates rollup_usage_1d.recording_bytes.
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestIntegration_MvRecording \
//	    ./internal/store/clickhouse/... -v -timeout 300s
//
// Prerequisites: /tmp/clickhouse binary available.
package clickhouse_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
	"github.com/pulse-analytics/pulse/server/internal/testutil"
)

// fixedEventTS is a fixed epoch-ms constant used in all MV recording assertions.
// UTC calendar date: 2026-07-11 (pinned for TS-to-bucket semantics test).
const fixedEventTS = int64(1783770838091)

// fixedEventSizeBytes is the recording size used for each event in the MV test.
const fixedEventSizeBytes = int64(12_345_678)

// TestIntegration_MvRecording verifies that inserting recording_ready server events
// via the store causes mv_recording_1d to populate rollup_usage_1d.recording_bytes.
//
// Assertions:
//
//	(a) sum(recording_bytes) in rollup_usage_1d equals 2 * fixedEventSizeBytes
//	(b) the bucket date equals the UTC calendar date of fixedEventTS
func TestIntegration_MvRecording(t *testing.T) {
	chBin := testutil.RequireClickHouseBin(t)

	tcpPort := freePort(t)
	httpPort := freePort(t)
	for httpPort == tcpPort {
		httpPort = freePort(t)
	}

	tmpDir := t.TempDir()

	// 1. Start ClickHouse server.
	cmd := exec.Command(chBin, "server",
		"--",
		fmt.Sprintf("--path=%s", tmpDir),
		fmt.Sprintf("--tcp_port=%d", tcpPort),
		fmt.Sprintf("--http_port=%d", httpPort),
		"--listen_host=127.0.0.1",
		"--mysql_port=0",
		"--postgresql_port=0",
	)
	// Suppress noise unless verbose mode is requested.
	if envStr := "PULSE_TEST_VERBOSE_CH"; envStr == "" {
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
	const dbName = "pulse_mv_rec_test"
	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)

	// 2. Wait for ClickHouse to accept connections (up to 45s).
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

	// 3. Locate contracts/db/clickhouse via runtime.Caller.
	// thisFile = server/internal/store/clickhouse/recording_mv_integration_test.go
	// repoRoot = 4 levels up
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")

	// 4. Apply ALL migrations (including 0009_recording_mv.sql when it exists).
	startupOpts, err := clickhousego.ParseDSN(startupDSN)
	if err != nil {
		t.Fatalf("parse startup DSN for migrations: %v", err)
	}
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

	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migCancel()

	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Log("migrations applied")

	// 5. Open Store with short FlushInterval for test speed.
	store, err := clickhouse.New(context.Background(), clickhouse.Config{
		DSN:           dsn,
		Database:      dbName,
		BatchSize:     100,
		FlushInterval: 500 * time.Millisecond,
		MaxRetries:    1,
	}, nil)
	if err != nil {
		t.Fatalf("clickhouse.New: %v", err)
	}
	defer store.Close()

	insertCtx, insertCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer insertCancel()

	store.Start(insertCtx)

	// 6. Insert TWO recording_ready ServerEvents with the fixed TS and size.
	t.Logf("inserting 2 recording_ready events (size_bytes=%d, ts=%d)...", fixedEventSizeBytes, fixedEventTS)
	for i := 0; i < 2; i++ {
		store.OnServerEvent(domain.ServerEvent{
			Version:  1,
			Type:     domain.EventRecordingReady,
			TS:       fixedEventTS,
			Source:   domain.SourceRestPoll,
			NodeID:   "node-1",
			App:      "pulse-test",
			StreamID: "stream-1",
			Data: map[string]any{
				"path":       "rec.mp4",
				"size_bytes": fixedEventSizeBytes,
			},
		})
	}

	// 7. Wait for store.Metrics() to confirm both events were flushed (10s deadline).
	deadline := time.Now().Add(10 * time.Second)
	var inserted int64
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		inserted, _ = store.Metrics()
		if inserted >= 2 {
			break
		}
	}
	if inserted < 2 {
		t.Fatalf("timed out waiting for flush: only %d events flushed (expected >= 2)", inserted)
	}
	t.Logf("flushed %d events", inserted)

	// 8. Open a direct verify connection to query rollup_usage_1d.
	verifyOpts, err := clickhousego.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse verify DSN: %v", err)
	}
	verifyConn, err := clickhousego.Open(verifyOpts)
	if err != nil {
		t.Fatalf("open verify conn: %v", err)
	}
	defer verifyConn.Close()

	qCtx, qCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer qCancel()

	// (a) Assert sum(recording_bytes) == 2 * fixedEventSizeBytes.
	// Use sum() to aggregate across possibly-unmerged SummingMergeTree parts.
	var totalBytes uint64
	sumRow := verifyConn.QueryRow(qCtx,
		fmt.Sprintf("SELECT sum(recording_bytes) FROM %s.rollup_usage_1d", dbName))
	if err := sumRow.Scan(&totalBytes); err != nil {
		t.Fatalf("sum(recording_bytes) query: %v", err)
	}
	want := uint64(2 * fixedEventSizeBytes)
	t.Logf("rollup_usage_1d sum(recording_bytes) = %d (want %d)", totalBytes, want)
	if totalBytes != want {
		t.Errorf("sum(recording_bytes) = %d, want %d (check that 0009_recording_mv.sql exists and was applied)",
			totalBytes, want)
	}

	// (b) Assert the bucket date equals the UTC calendar date of fixedEventTS.
	expectedBucket := time.UnixMilli(fixedEventTS).UTC().Format("2006-01-02")
	var bucketStr string
	bucketRow := verifyConn.QueryRow(qCtx,
		fmt.Sprintf("SELECT toString(min(bucket)) FROM %s.rollup_usage_1d WHERE recording_bytes > 0", dbName))
	if err := bucketRow.Scan(&bucketStr); err != nil {
		t.Fatalf("bucket date query: %v", err)
	}
	t.Logf("bucket date in rollup_usage_1d = %q (want %q)", bucketStr, expectedBucket)
	if bucketStr != expectedBucket {
		t.Errorf("bucket = %q, want %q (TS-to-bucket semantics)", bucketStr, expectedBucket)
	}

	t.Logf("PASS: recording_bytes sum=%d (2x%d), bucket=%s", totalBytes, fixedEventSizeBytes, bucketStr)
}

// TestIntegration_MvRecording_IdempotentRun verifies that 0009_recording_mv.sql
// participates in TestIntegration_Migrations_IdempotentRun cleanly.
// This test simply applies migrations twice to the same instance and confirms
// the second run is a no-op (consistent with A11).
func TestIntegration_MvRecording_IdempotentRun(t *testing.T) {
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
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("start clickhouse: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	const dbName = "pulse_mv_idem_test"

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer waitCancel()

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

	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migCancel()

	// First run.
	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	t.Log("first migration run complete")

	// Count migrations after first run.
	var count1 uint64
	row1 := migConn.QueryRow(migCtx, fmt.Sprintf("SELECT count() FROM %s.schema_migrations", dbName))
	if err := row1.Scan(&count1); err != nil {
		t.Fatalf("count schema_migrations after first run: %v", err)
	}
	t.Logf("schema_migrations after first run: %d", count1)

	// Second run — must be a no-op (A11).
	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("A11 VIOLATION — second Run returned error: %v", err)
	}
	t.Log("second migration run: no-op (A11 satisfied)")

	var count2 uint64
	row2 := migConn.QueryRow(migCtx, fmt.Sprintf("SELECT count() FROM %s.schema_migrations", dbName))
	if err := row2.Scan(&count2); err != nil {
		t.Fatalf("count schema_migrations after second run: %v", err)
	}
	if count2 != count1 {
		t.Errorf("A11: migration count changed from %d to %d — second run is not idempotent", count1, count2)
	}
	t.Logf("PASS: %d migrations, A11 satisfied (0009 idempotent)", count1)
}
