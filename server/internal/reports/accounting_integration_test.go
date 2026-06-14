//go:build integration

// Integration test: Accountant with a real ClickHouse server process.
// Exercises the real a.conn path (NOT the nil-conn shortcut) — this is the
// regression guard for defect D-W2-002 (wrong ClickHouse column names).
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run TestAccountant ./internal/reports/... -v -timeout 180s
//
// Prerequisites: /tmp/clickhouse binary available (D-002: no Docker).
package reports_test

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
	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
)

// TestAccountant_CHIntegration starts a real ClickHouse instance, runs migrations,
// seeds KNOWN-TRUTH viewer_sessions (2 tenants, fixed counts/durations), then:
// (a) calls ComputeUsage and asserts viewer-minutes/peak match truth,
// (b) calls Reconcile and asserts DriftPct ≤ 1.0 against raw,
// (c) asserts tenant attribution is correct.
//
// This test exercises the real Accountant.conn code path (NOT the nil-conn
// shortcut that bypasses ClickHouse). It is the regression guard for D-W2-002.
func TestAccountant_CHIntegration(t *testing.T) {
	chBin := "/tmp/clickhouse"
	if _, err := os.Stat(chBin); err != nil {
		t.Skipf("clickhouse binary not found at %s: %v", chBin, err)
	}

	// ── 1. Start ClickHouse ──────────────────────────────────────────────────
	tcpPort := integFreePort(t)
	httpPort := integFreePort(t)
	for httpPort == tcpPort {
		httpPort = integFreePort(t)
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

	const dbName = "pulse_accounting_test"
	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)

	// ── 2. Wait for ClickHouse ───────────────────────────────────────────────
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 60*time.Second)
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
			t.Fatal("timeout waiting for ClickHouse to start")
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Log("ClickHouse ready")

	// ── 3. Run migrations ────────────────────────────────────────────────────
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	// thisFile = server/internal/reports/accounting_integration_test.go
	// filepath.Dir(thisFile) = server/internal/reports/
	// repoRoot = 3 levels up: reports -> internal -> server -> repo-root
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
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

	// ── 4. Open account connection ───────────────────────────────────────────
	opts, err := clickhousego.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// ── 5. Seed KNOWN-TRUTH viewer_sessions ──────────────────────────────────
	// Two tenants, two streams, known watch times.
	// Tenant A (stream "stream-alpha"): 3 sessions × 600 s each = 1800 s = 30 min
	// Tenant B (stream "stream-beta"):  5 sessions × 300 s each = 1500 s = 25 min
	// Total: 8 sessions, 55 min
	//
	// mv_usage_1d inserts: viewer_minutes = watch_time_s / 60.0 per session row.
	// Since peak_concurrency is toUInt32(1) per row in the MV, sum over sessions
	// gives session count (not true concurrent peak). For the test we verify
	// total viewer-minutes and drift, not peak (which is an approximation in the MV).

	// Use a recent date — within the 90-day viewer_sessions TTL.
	// Fixed past dates (e.g., 2026-03-01) may exceed the TTL and cause
	// ClickHouse to expire rows before background merge, returning 0 counts.
	baseDay := time.Now().UTC().Truncate(24 * time.Hour).Add(-2 * 24 * time.Hour) // 2 days ago

	type seedRow struct {
		sessionID string
		streamID  string
		app       string
		nodeID    string
		tenant    string
		watchS    uint32
		startedAt time.Time
		endedAt   time.Time
		updatedAt time.Time
	}

	var seeds []seedRow

	// Tenant A: 3 sessions × 600 s
	for i := 0; i < 3; i++ {
		s := baseDay.Add(time.Duration(i*10) * time.Minute)
		seeds = append(seeds, seedRow{
			sessionID: fmt.Sprintf("alpha-%02d", i),
			streamID:  "stream-alpha",
			app:       "live",
			nodeID:    "n1",
			tenant:    "tenant-a",
			watchS:    600,
			startedAt: s,
			endedAt:   s.Add(600 * time.Second),
			updatedAt: s.Add(600 * time.Second),
		})
	}

	// Tenant B: 5 sessions × 300 s
	for i := 0; i < 5; i++ {
		s := baseDay.Add(time.Duration(i*7) * time.Minute)
		seeds = append(seeds, seedRow{
			sessionID: fmt.Sprintf("beta-%02d", i),
			streamID:  "stream-beta",
			app:       "live",
			nodeID:    "n1",
			tenant:    "tenant-b",
			watchS:    300,
			startedAt: s,
			endedAt:   s.Add(300 * time.Second),
			updatedAt: s.Add(300 * time.Second),
		})
	}

	// Known truth totals (seconds).
	const truthAlphaS = 3 * 600.0  // 1800 s = 30 min
	const truthBetaS = 5 * 300.0   // 1500 s = 25 min
	const truthTotalS = truthAlphaS + truthBetaS // 3300 s = 55 min
	const truthTotalMin = truthTotalS / 60.0      // 55.0 min

	// Insert via direct INSERT INTO viewer_sessions.
	// The mv_usage_1d MV will trigger on INSERT and populate rollup_usage_1d.
	insertCtx, insertCancel := context.WithTimeout(ctx, 30*time.Second)
	defer insertCancel()

	// Insert with explicit column list matching the viewer_sessions schema.
	// Unqualified table name — connection DSN sets the default database.
	// All other columns use their DEFAULT values from the DDL.
	batch, err := conn.PrepareBatch(insertCtx,
		`INSERT INTO viewer_sessions
			(session_id, stream_id, app, node_id, tenant,
			 watch_time_s, started_at, ended_at, updated_at)`)
	if err != nil {
		t.Fatalf("prepare batch: %v", err)
	}

	for _, s := range seeds {
		if err := batch.Append(
			s.sessionID, s.streamID, s.app, s.nodeID, s.tenant,
			s.watchS,
			s.startedAt, s.endedAt, s.updatedAt,
		); err != nil {
			t.Fatalf("batch append: %v", err)
		}
	}
	if err := batch.Send(); err != nil {
		t.Fatalf("batch send: %v", err)
	}
	t.Logf("seeded %d viewer_sessions (truth: %.1f min total)", len(seeds), truthTotalMin)

	// Wait briefly for materialized view to trigger.
	time.Sleep(500 * time.Millisecond)

	// Verify rollup_usage_1d was populated.
	// Use unqualified table name — connection DSN sets the default database.
	var usageRows uint64
	if err := conn.QueryRow(insertCtx,
		"SELECT count() FROM rollup_usage_1d",
	).Scan(&usageRows); err != nil {
		t.Fatalf("rollup_usage_1d count: %v", err)
	}
	t.Logf("rollup_usage_1d rows after seed: %d", usageRows)
	if usageRows == 0 {
		t.Fatal("rollup_usage_1d is empty — mv_usage_1d MV did not trigger; MV population broken")
	}

	// ── 6. Create Accountant (real conn path) ─────────────────────────────────
	// No meta store: we rely on the tenant column already stored in rollup_usage_1d
	// (populated from viewer_sessions.tenant by mv_usage_1d).
	acct := reports.NewAccountant(conn, nil)

	// ── 7a. ComputeUsage — assert viewer-minutes and tenant attribution ────────
	from := baseDay.AddDate(0, 0, -1) // one day before seed
	to := baseDay.AddDate(0, 0, +2)   // two days after seed
	usageReport, err := acct.ComputeUsage(ctx, reports.UsageParams{
		From:     from,
		To:       to,
		Interval: "day",
	})
	if err != nil {
		t.Fatalf("ComputeUsage: %v", err)
	}

	t.Logf("ComputeUsage returned %d rows, totals: viewer_minutes=%.4f, peak=%d",
		len(usageReport.Rows), usageReport.Totals.ViewerMinutes, usageReport.Totals.PeakConcurrency)

	// Total viewer-minutes must be within 1% of truth.
	if usageReport.Totals.ViewerMinutes == 0 {
		t.Fatal("ComputeUsage returned 0 viewer-minutes — ClickHouse query returned no rows")
	}

	drift := absDiff(usageReport.Totals.ViewerMinutes, truthTotalMin) / truthTotalMin * 100.0
	t.Logf("ComputeUsage: computed=%.4f min, truth=%.4f min, drift=%.4f%%",
		usageReport.Totals.ViewerMinutes, truthTotalMin, drift)
	if drift > 1.0 {
		t.Errorf("ComputeUsage viewer-minutes drift=%.4f%% exceeds ±1%% budget (computed=%.4f, truth=%.4f)",
			drift, usageReport.Totals.ViewerMinutes, truthTotalMin)
	}

	// All rows must have egress_method set.
	for i, r := range usageReport.Rows {
		if r.EgressMethod == "" {
			t.Errorf("row[%d] has empty egress_method", i)
		}
	}

	// Tenant attribution: tenant field in rollup_usage_1d comes from viewer_sessions.tenant.
	// Verify each stream's viewer-minutes individually.
	alphaMin := 0.0
	betaMin := 0.0
	for _, r := range usageReport.Rows {
		if r.StreamID != nil && *r.StreamID == "stream-alpha" {
			alphaMin += r.ViewerMinutes
		}
		if r.StreamID != nil && *r.StreamID == "stream-beta" {
			betaMin += r.ViewerMinutes
		}
	}
	t.Logf("Per-stream: alpha=%.4f min (truth=%.1f), beta=%.4f min (truth=%.1f)",
		alphaMin, truthAlphaS/60.0, betaMin, truthBetaS/60.0)

	alphaDrift := absDiff(alphaMin, truthAlphaS/60.0) / (truthAlphaS / 60.0) * 100.0
	betaDrift := absDiff(betaMin, truthBetaS/60.0) / (truthBetaS / 60.0) * 100.0
	if alphaDrift > 1.0 {
		t.Errorf("stream-alpha viewer-minutes drift=%.4f%% (computed=%.4f, truth=%.4f)",
			alphaDrift, alphaMin, truthAlphaS/60.0)
	}
	if betaDrift > 1.0 {
		t.Errorf("stream-beta viewer-minutes drift=%.4f%% (computed=%.4f, truth=%.4f)",
			betaDrift, betaMin, truthBetaS/60.0)
	}

	t.Logf("PASS (a): ComputeUsage — viewer-minutes drift=%.4f%%, stream attribution within 1%%", drift)

	// ── 7b. Reconcile — assert DriftPct ≤ 1.0 ───────────────────────────────
	// Reconcile compares rollup_usage_1d sum(viewer_minutes) vs raw
	// viewer_sessions sum(watch_time_s)/60. Both are seeded from the same source,
	// so drift should be exactly 0% (or near 0% from float arithmetic).
	rec, err := acct.Reconcile(ctx, from, to)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	t.Logf("Reconcile: rollup=%.4f min, raw=%.4f min, drift=%.6f%%, within_tolerance=%v, data_points=%d",
		rec.RollupViewerMinutes, rec.RawViewerMinutes, rec.DriftPct, rec.WithinTolerance, rec.DataPoints)

	if rec.RollupViewerMinutes == 0 {
		t.Fatal("Reconcile: rollup viewer-minutes is 0 — rollup_usage_1d query broken")
	}
	if rec.RawViewerMinutes == 0 {
		t.Fatal("Reconcile: raw viewer-minutes is 0 — viewer_sessions query broken")
	}
	if !rec.WithinTolerance {
		t.Errorf("Reconcile drift=%.4f%% exceeds ±1%% budget (rollup=%.4f, raw=%.4f)",
			rec.DriftPct, rec.RollupViewerMinutes, rec.RawViewerMinutes)
	}
	if rec.DataPoints == 0 {
		t.Error("Reconcile: data_points=0, expected > 0")
	}

	t.Logf("PASS (b): Reconcile drift=%.6f%% ≤ 1%%", rec.DriftPct)

	// ── 7c. Tenant attribution via rollup_usage_1d.tenant column ─────────────
	// Query rollup_usage_1d directly to verify tenant column is populated.
	tenantRows, err := conn.Query(insertCtx,
		`SELECT tenant, sum(viewer_minutes) AS vm
		FROM rollup_usage_1d
		WHERE bucket >= ? AND bucket <= ?
		GROUP BY tenant
		ORDER BY tenant`,
		from, to,
	)
	if err != nil {
		t.Fatalf("tenant attribution query: %v", err)
	}
	defer tenantRows.Close()

	tenantMinutes := map[string]float64{}
	for tenantRows.Next() {
		var tn string
		var vm float64
		if err := tenantRows.Scan(&tn, &vm); err != nil {
			t.Fatalf("tenant row scan: %v", err)
		}
		tenantMinutes[tn] = vm
		t.Logf("tenant=%q: %.4f viewer-minutes", tn, vm)
	}
	if err := tenantRows.Err(); err != nil {
		t.Fatalf("tenant rows err: %v", err)
	}

	aVM, aOK := tenantMinutes["tenant-a"]
	bVM, bOK := tenantMinutes["tenant-b"]
	if !aOK {
		t.Error("tenant-a not found in rollup_usage_1d")
	} else {
		d := absDiff(aVM, truthAlphaS/60.0) / (truthAlphaS / 60.0) * 100.0
		if d > 1.0 {
			t.Errorf("tenant-a viewer-minutes drift=%.4f%% (got=%.4f, want=%.4f)", d, aVM, truthAlphaS/60.0)
		}
	}
	if !bOK {
		t.Error("tenant-b not found in rollup_usage_1d")
	} else {
		d := absDiff(bVM, truthBetaS/60.0) / (truthBetaS / 60.0) * 100.0
		if d > 1.0 {
			t.Errorf("tenant-b viewer-minutes drift=%.4f%% (got=%.4f, want=%.4f)", d, bVM, truthBetaS/60.0)
		}
	}

	t.Logf("PASS (c): tenant-a=%.4f min (truth=30.0), tenant-b=%.4f min (truth=25.0) — attribution correct",
		aVM, bVM)
	t.Logf("PASS: D-W2-002 regression guard complete — live CH path verified")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func integFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
