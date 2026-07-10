//go:build integration

// Integration test: probe_results TTL responds to RetentionDays configuration.
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestIntegration_ProbeResultsTTL \
//	    ./internal/store/clickhouse/... -v -timeout 300s
//
// Prerequisites: /tmp/clickhouse binary available (D-002: no Docker-in-Docker).
// Validates D-072 verifier finding #6: 0001_init.sql hardcoded toIntervalDay(90)
// for probe_results instead of using the {retention_days} placeholder, making the
// retention_days config knob a no-op for that table.
package clickhouse_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
	"github.com/pulse-analytics/pulse/server/internal/testutil"
)

// TestIntegration_ProbeResultsTTL verifies that the migration set applies a
// probe_results TTL expression that matches the configured RetentionDays value.
//
// This is a regression test for D-072 verifier finding #6: probe_results had a
// hardcoded toIntervalDay(90) in 0001_init.sql, so any non-default RetentionDays
// was silently ignored for that table while all other raw tables correctly used
// {retention_days}.
//
// GREEN requires two changes:
//  1. 0001_init.sql line ~225: toIntervalDay(90) → toIntervalDay({retention_days})
//  2. 0006_probe_results_ttl.sql: ALTER TABLE MODIFY TTL for existing deployments
func TestIntegration_ProbeResultsTTL(t *testing.T) {
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

	const dbName = "pulse_ttl_test"
	const retentionDays = 33 // deliberately non-default (runner default is 90)

	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)

	// Wait for ClickHouse to accept TCP connections (first startup can take 45s).
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

	// Locate contracts/db/clickhouse relative to this source file.
	// thisFile is server/internal/store/clickhouse/migrations_ttl_integration_test.go
	// repoRoot is 4 levels up.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path via runtime.Caller")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")

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
		RetentionDays: retentionDays,
		RollupTTLDays: 395,
	}, nil)

	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migCancel()

	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Log("migrations applied")

	// Query system.tables for probe_results DDL — this is what ClickHouse
	// actually used to build the table (substitutions already applied by runner).
	var createTableQuery string
	row := migConn.QueryRow(migCtx,
		"SELECT create_table_query FROM system.tables WHERE database = ? AND name = 'probe_results'",
		dbName,
	)
	if err := row.Scan(&createTableQuery); err != nil {
		t.Fatalf("create_table_query for probe_results: %v", err)
	}
	t.Logf("probe_results DDL (tail-300): ...%s",
		createTableQuery[max(0, len(createTableQuery)-300):])

	// Assert the TTL expression encodes the configured retentionDays value.
	// GREEN requires: 0001 uses {retention_days} AND 0006 MODIFY TTL is present.
	wantTTL := fmt.Sprintf("toIntervalDay(%d)", retentionDays)
	if !containsStr(createTableQuery, wantTTL) {
		t.Errorf("probe_results TTL does not reflect RetentionDays=%d:\n"+
			"  want create_table_query to contain: %q\n"+
			"  actual create_table_query: %s",
			retentionDays, wantTTL, createTableQuery)
	}

	// Also verify 0006 was recorded in schema_migrations (i.e. the ALTER ran).
	var migNames []string
	rows, err := migConn.Query(migCtx,
		fmt.Sprintf("SELECT name FROM %s.schema_migrations ORDER BY name", dbName))
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan migration name: %v", err)
		}
		migNames = append(migNames, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	t.Logf("applied migrations: %v", migNames)

	foundTTLMig := false
	for _, n := range migNames {
		if n == "0006_probe_results_ttl.sql" {
			foundTTLMig = true
			break
		}
	}
	if !foundTTLMig {
		t.Errorf("0006_probe_results_ttl.sql not recorded in schema_migrations — migration was not applied")
	}

	t.Logf("PASS: probe_results TTL uses toIntervalDay(%d) as configured", retentionDays)
}
