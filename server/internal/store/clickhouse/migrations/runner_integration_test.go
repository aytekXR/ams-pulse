//go:build integration

// Integration tests for the migrations runner.
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestIntegration_Migrations \
//	    ./internal/store/clickhouse/migrations/... -v -timeout 300s
//
// Prerequisites: /tmp/clickhouse binary available (D-002: no Docker-in-Docker).
// The test starts a real ClickHouse server, applies all migrations from
// contracts/db/clickhouse/, and retires assumption A11 by asserting the
// second Run is a no-op returning nil.
package migrations

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
	"github.com/pulse-analytics/pulse/server/internal/testutil"
)

// TestIntegration_Migrations_IdempotentRun exercises the full Run() path
// against a real ClickHouse process and retires assumption A11.
//
// Steps:
//  1. Start a ClickHouse server on a random port.
//  2. Apply all migration files (first run) — must succeed.
//  3. Verify schema_migrations table was populated.
//  4. Apply migrations again (second run) — A11: MUST return nil (no-op).
//  5. Verify migration count is unchanged (no duplicate rows).
func TestIntegration_Migrations_IdempotentRun(t *testing.T) {
	chBin := testutil.RequireClickHouseBin(t)

	tcpPort := migFreePort(t)
	httpPort := migFreePort(t)
	for httpPort == tcpPort {
		httpPort = migFreePort(t)
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
	// thisFile is inside server/internal/store/clickhouse/migrations/; repo root is 5 levels up.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path via runtime.Caller")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../../../..")
	migrationsDir := filepath.Join(repoRoot, "contracts/db/clickhouse")
	t.Logf("migrations dir: %s", migrationsDir)

	// Verify the directory exists (sanity-check the path arithmetic).
	if _, err := os.Stat(migrationsDir); err != nil {
		t.Fatalf("migrations dir not accessible (%s): %v — check repo mount", migrationsDir, err)
	}

	const dbName = "pulse_migrations_test"

	// Open connection to 'default' — runner.ensureMigrationsTable creates the
	// target database itself, so the DSN must NOT specify it yet.
	startupOpts, err := clickhousego.ParseDSN(startupDSN)
	if err != nil {
		t.Fatalf("parse startup DSN: %v", err)
	}
	migConn, err := clickhousego.Open(startupOpts)
	if err != nil {
		t.Fatalf("open migration conn: %v", err)
	}
	defer migConn.Close()

	runner := New(migConn, Config{
		MigrationsDir: migrationsDir,
		Database:      dbName,
		RetentionDays: 90,
		RollupTTLDays: 395,
	}, nil)

	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migCancel()

	// ─── First Run ──────────────────────────────────────────────────────────────
	t.Log("first migration run...")
	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	t.Log("first migration run complete")

	// Verify schema_migrations was populated.
	count1 := migCount(t, migCtx, migConn, dbName)
	t.Logf("schema_migrations rows after first run: %d", count1)
	if count1 == 0 {
		t.Fatal("schema_migrations is empty after first run — migrations were not tracked")
	}

	// ─── A11 (BINDING): Second Run must be a no-op ──────────────────────────────
	// Assumption A11 stated that idempotency was unverified; this test retires it.
	// A failing assertion here is a regression blocker: the runner MUST skip
	// already-applied migrations and return nil.
	t.Log("A11: running migrations a SECOND time against the same instance...")
	if err := runner.Run(migCtx); err != nil {
		t.Fatalf("A11 VIOLATION — second Run returned error: %v\n"+
			"The runner must be idempotent: applied migrations are tracked in schema_migrations "+
			"and must be skipped on subsequent calls.", err)
	}
	t.Log("A11 SATISFIED: second Run returned nil (idempotent no-op)")

	// Verify migration count is unchanged (no duplicate insertions).
	count2 := migCount(t, migCtx, migConn, dbName)
	t.Logf("schema_migrations rows after second run: %d (was %d)", count2, count1)
	if count2 != count1 {
		t.Errorf("A11: schema_migrations count changed from %d to %d after second Run — "+
			"migrations must not be re-applied or re-inserted", count1, count2)
	}

	// Verify the target database tables were created.
	tableCount := migTableCount(t, migCtx, migConn, dbName)
	t.Logf("tables in %s: %d", dbName, tableCount)
	if tableCount == 0 {
		t.Errorf("no tables found in %s after migrations — DDL may not have executed", dbName)
	}

	t.Logf("PASS: %d migrations applied, A11 retired (idempotent), %d tables created",
		count1, tableCount)
}

// migCount returns the number of rows in schema_migrations for the given database.
func migCount(t *testing.T, ctx context.Context, conn clickhousego.Conn, db string) uint64 {
	t.Helper()
	row := conn.QueryRow(ctx,
		fmt.Sprintf("SELECT count() FROM %s.schema_migrations", db))
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	return n
}

// migTableCount returns the number of tables in the given database.
func migTableCount(t *testing.T, ctx context.Context, conn clickhousego.Conn, db string) uint64 {
	t.Helper()
	row := conn.QueryRow(ctx,
		"SELECT count() FROM system.tables WHERE database = ?", db)
	var n uint64
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count tables in %s: %v", db, err)
	}
	return n
}

// migFreePort finds an available TCP port.
func migFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}
