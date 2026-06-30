//go:build integration

// Package api_test — VD-19 geo/device HTTP-level integration test.
//
// VD-19: handleGeoAnalytics / handleDeviceAnalytics have no HTTP-level test.
// This file adds one that:
//   - Starts a real ClickHouse instance (D-002: /tmp/clickhouse, no Docker).
//   - Seeds viewer_sessions with >=2 distinct geo_country and >=2 distinct
//     client_device rows (views > 0).
//   - Wires a real query.Service via the existing API server wiring.
//   - GETs /api/v1/analytics/geo and /api/v1/analytics/devices.
//   - Asserts HTTP 200 and a non-empty `rows` array with at least one
//     non-empty country/device entry and views > 0.
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run "TestVD19" ./internal/api/... -v -timeout 300s
//
// Prerequisites: /tmp/clickhouse binary (D-002).
package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clickhousego "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse/migrations"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// startCHForVD19 starts a ClickHouse server for VD-19 tests and returns the TCP port.
func startCHForVD19(t *testing.T, tmpDir string) int {
	t.Helper()
	chBin := "/tmp/clickhouse"
	if _, err := os.Stat(chBin); err != nil {
		t.Skipf("clickhouse binary not found at %s: %v", chBin, err)
	}

	tcpPort := vd19FreePort(t)
	httpPort := vd19FreePort(t)
	for httpPort == tcpPort {
		httpPort = vd19FreePort(t)
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

	t.Logf("VD-19: waiting for ClickHouse on port %d...", tcpPort)
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
	t.Logf("VD-19: ClickHouse ready on TCP port %d", tcpPort)
	return tcpPort
}

func vd19FreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// runVD19Migrations runs ClickHouse migrations for the VD-19 test database.
func runVD19Migrations(t *testing.T, tcpPort int, dbName string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
	// thisFile = server/internal/api/vd19_geo_device_test.go
	// repoRoot = 3 dirs up: api -> internal -> server -> repo
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
	t.Log("VD-19: migrations applied")
}

// vd19LiveProvider is a minimal live provider for VD-19 API tests.
type vd19LiveProvider struct{}

func (v *vd19LiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	}
}
func (v *vd19LiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// setupVD19Server creates an httptest.Server with a real ClickHouse-backed
// query.Service. Returns the server, an admin token, the CH conn, and cleanup.
func setupVD19Server(t *testing.T, conn clickhousego.Conn) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "vd19-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_vd19_testtoken_abcdef"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "vd19-admin",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// /analytics/geo and /analytics/devices are gated behind CheckDataAPI
	// (Pro+) since D-041, so provision a Pro license here.
	proKey, licCleanup := makeTestProLicense(t)
	lic, err := license.New(proKey, "")
	if err != nil {
		t.Fatalf("license.New (pro): %v", err)
	}
	live := &vd19LiveProvider{}
	// Wire real CH conn so GeoBreakdown / DeviceBreakdown queries real data.
	qsvc := query.New(live, conn, lic)

	apiCfg := api.Config{ListenAddr: ":0"}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
		licCleanup()
	}
	return ts, adminToken, cleanup
}

// TestVD19_GeoAnalytics_NonEmptyRows guards VD-19:
// GET /api/v1/analytics/geo must return HTTP 200 and a non-empty `rows` array
// with at least one entry whose country is non-empty and views > 0,
// when viewer_sessions has been seeded with >=2 distinct geo_country values.
func TestVD19_GeoAnalytics_NonEmptyRows(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startCHForVD19(t, tmpDir)
	const dbName = "pulse_vd19_geo_test"
	runVD19Migrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	baseTime := time.Now().UTC().Add(-24 * time.Hour)

	// Seed >=2 distinct geo_country rows.
	geoSeeds := []struct {
		sid, stream, app, nodeID, country, device, protocol string
		watchS                                               uint32
		startedAt, endedAt, updatedAt                       time.Time
	}{
		{
			sid: "geo-us-1", stream: "s1", app: "live", nodeID: "n1",
			country: "US", device: "desktop", protocol: "hls", watchS: 300,
			startedAt: baseTime, endedAt: baseTime.Add(300 * time.Second),
			updatedAt: baseTime.Add(300 * time.Second),
		},
		{
			sid: "geo-us-2", stream: "s1", app: "live", nodeID: "n1",
			country: "US", device: "mobile", protocol: "hls", watchS: 120,
			startedAt: baseTime.Add(5 * time.Minute), endedAt: baseTime.Add(5*time.Minute + 120*time.Second),
			updatedAt: baseTime.Add(5*time.Minute + 120*time.Second),
		},
		{
			sid: "geo-de-1", stream: "s1", app: "live", nodeID: "n1",
			country: "DE", device: "desktop", protocol: "webrtc", watchS: 600,
			startedAt: baseTime.Add(10 * time.Minute), endedAt: baseTime.Add(10*time.Minute + 600*time.Second),
			updatedAt: baseTime.Add(10*time.Minute + 600*time.Second),
		},
	}
	for _, s := range geoSeeds {
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.viewer_sessions
				(session_id, stream_id, app, node_id, started_at, ended_at, updated_at,
				 watch_time_s, geo_country, client_device, protocol)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, dbName),
			s.sid, s.stream, s.app, s.nodeID,
			s.startedAt, s.endedAt, s.updatedAt,
			s.watchS, s.country, s.device, s.protocol,
		); err != nil {
			t.Fatalf("seed geo session %s: %v", s.sid, err)
		}
	}
	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.viewer_sessions FINAL", dbName))

	ts, token, cleanup := setupVD19Server(t, conn)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/geo", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/analytics/geo: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("VD-19 geo: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("VD-19 geo: decode response: %v", err)
	}

	rowsRaw, ok := result["rows"]
	if !ok {
		t.Fatal("VD-19 geo FAIL: response missing 'rows' key")
	}
	rows, ok := rowsRaw.([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("VD-19 geo FAIL: expected non-empty rows array, got %v", rowsRaw)
	}

	// Assert at least one row has a non-empty country and views > 0.
	for _, rowRaw := range rows {
		row, ok := rowRaw.(map[string]any)
		if !ok {
			continue
		}
		country, _ := row["country"].(string)
		views, _ := row["views"].(float64)
		if country != "" && views > 0 {
			t.Logf("PASS VD-19 geo: country=%q views=%.0f (non-empty, views>0)", country, views)
			return
		}
	}
	t.Errorf("VD-19 geo FAIL: no row with non-empty country and views>0 in %d rows: %v", len(rows), rows)
}

// TestVD19_DeviceAnalytics_NonEmptyRows guards VD-19:
// GET /api/v1/analytics/devices must return HTTP 200 and a non-empty `rows` array
// with at least one entry whose device is non-empty and views > 0,
// when viewer_sessions has been seeded with >=2 distinct client_device values.
func TestVD19_DeviceAnalytics_NonEmptyRows(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startCHForVD19(t, tmpDir)
	const dbName = "pulse_vd19_device_test"
	runVD19Migrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	baseTime := time.Now().UTC().Add(-24 * time.Hour)

	// Seed >=2 distinct client_device rows.
	devSeeds := []struct {
		sid, stream, app, nodeID, country, device, os, browser, protocol string
		watchS                                                             uint32
		startedAt, endedAt, updatedAt                                     time.Time
	}{
		{
			sid: "dev-desktop-1", stream: "s1", app: "live", nodeID: "n1",
			country: "US", device: "desktop", os: "linux", browser: "chrome", protocol: "hls",
			watchS: 400,
			startedAt: baseTime, endedAt: baseTime.Add(400 * time.Second),
			updatedAt: baseTime.Add(400 * time.Second),
		},
		{
			sid: "dev-desktop-2", stream: "s1", app: "live", nodeID: "n1",
			country: "GB", device: "desktop", os: "windows", browser: "firefox", protocol: "hls",
			watchS: 200,
			startedAt: baseTime.Add(5 * time.Minute), endedAt: baseTime.Add(5*time.Minute + 200*time.Second),
			updatedAt: baseTime.Add(5*time.Minute + 200*time.Second),
		},
		{
			sid: "dev-mobile-1", stream: "s1", app: "live", nodeID: "n1",
			country: "US", device: "mobile", os: "android", browser: "chrome", protocol: "hls",
			watchS: 150,
			startedAt: baseTime.Add(10 * time.Minute), endedAt: baseTime.Add(10*time.Minute + 150*time.Second),
			updatedAt: baseTime.Add(10*time.Minute + 150*time.Second),
		},
	}
	for _, s := range devSeeds {
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.viewer_sessions
				(session_id, stream_id, app, node_id, started_at, ended_at, updated_at,
				 watch_time_s, geo_country, client_device, client_os, client_browser, protocol)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, dbName),
			s.sid, s.stream, s.app, s.nodeID,
			s.startedAt, s.endedAt, s.updatedAt,
			s.watchS, s.country, s.device, s.os, s.browser, s.protocol,
		); err != nil {
			t.Fatalf("seed device session %s: %v", s.sid, err)
		}
	}
	_ = conn.Exec(ctx, fmt.Sprintf("OPTIMIZE TABLE %s.viewer_sessions FINAL", dbName))

	ts, token, cleanup := setupVD19Server(t, conn)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/devices", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/analytics/devices: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("VD-19 devices: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("VD-19 devices: decode response: %v", err)
	}

	rowsRaw, ok := result["rows"]
	if !ok {
		t.Fatal("VD-19 devices FAIL: response missing 'rows' key")
	}
	rows, ok := rowsRaw.([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("VD-19 devices FAIL: expected non-empty rows array, got %v", rowsRaw)
	}

	// Assert at least one row has a non-empty device and views > 0.
	for _, rowRaw := range rows {
		row, ok := rowRaw.(map[string]any)
		if !ok {
			continue
		}
		device, _ := row["device"].(string)
		views, _ := row["views"].(float64)
		if device != "" && views > 0 {
			t.Logf("PASS VD-19 devices: device=%q views=%.0f (non-empty, views>0)", device, views)
			return
		}
	}
	t.Errorf("VD-19 devices FAIL: no row with non-empty device and views>0 in %d rows: %v", len(rows), rows)
}
