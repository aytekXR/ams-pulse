//go:build integration

// Package api_test — VD-24 qoe/ingest seeded ClickHouse integration test.
//
// VD-24: the existing TestVD21_IngestHealth_TimeseriesAndDropEventsPresent uses
// a nil qsvc conn so IngestTimeseries always returns [] (empty). This test
// closes that gap by:
//   - Starting a real ClickHouse instance (D-002: /tmp/clickhouse, no Docker).
//   - Seeding server_events ingest_stats rows for a known stream.
//   - Wiring a real query.Service via the existing api.Server setter.
//   - GETting /api/v1/qoe/ingest with a live snapshot containing the stream.
//   - Asserting HTTP 200 and streams[0].timeseries is a NON-empty array
//     (closes the gap: previously timeseries was always [] due to nil conn).
//
// Run with:
//
//	CGO_ENABLED=0 go test -tags integration -run "TestVD24" ./internal/api/... -v -timeout 300s
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

// startCHForVD24 starts a ClickHouse server for VD-24 and returns the TCP port.
func startCHForVD24(t *testing.T, tmpDir string) int {
	t.Helper()
	chBin := "/tmp/clickhouse"
	if _, err := os.Stat(chBin); err != nil {
		t.Skipf("clickhouse binary not found at %s: %v", chBin, err)
	}

	tcpPort := vd24FreePort(t)
	httpPort := vd24FreePort(t)
	for httpPort == tcpPort {
		httpPort = vd24FreePort(t)
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

	startupDSN := fmt.Sprintf("clickhouse://127.0.0.1:%d/default", tcpPort)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer waitCancel()

	t.Logf("VD-24: waiting for ClickHouse on port %d...", tcpPort)
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
	t.Logf("VD-24: ClickHouse ready on TCP port %d", tcpPort)
	return tcpPort
}

func vd24FreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// runVD24Migrations runs ClickHouse migrations for the VD-24 test database.
func runVD24Migrations(t *testing.T, tcpPort int, dbName string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source file path")
	}
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
	t.Log("VD-24: migrations applied")
}

// vd24LiveProvider is a live provider that returns a known active stream.
// This ensures the handleIngestHealth handler builds the streams list and
// calls qsvc.IngestTimeseries for the seeded stream.
type vd24LiveProvider struct {
	streamID string
	app      string
	nodeID   string
}

func (v *vd24LiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		ActiveStreams: 1,
		TotalViewers:  3,
		IngestBitrate: 1200.0,
		Streams: map[string]*domain.LiveStream{
			v.streamID: {
				StreamID:      v.streamID,
				App:           v.app,
				NodeID:        v.nodeID,
				Active:        true,
				ViewerCount:   3,
				IngestBitrate: 1200.0,
				FPS:           30.0,
				HealthScore:   0.9,
				Health:        domain.StreamHealthGood,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{
			v.nodeID: {NodeID: v.nodeID, CPUPCT: 20.0, MemPCT: 35.0},
		},
	}
}

func (v *vd24LiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// TestVD24_IngestQoE_TimeseriesNonEmpty guards VD-24:
// GET /api/v1/qoe/ingest must return streams[0].timeseries as a NON-empty array
// when server_events has been seeded with ingest_stats rows for the live stream.
// Previously the existing test used nil conn so timeseries was always [].
func TestVD24_IngestQoE_TimeseriesNonEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	tcpPort := startCHForVD24(t, tmpDir)
	const dbName = "pulse_vd24_ingest_test"
	runVD24Migrations(t, tcpPort, dbName)

	dsn := fmt.Sprintf("clickhouse://127.0.0.1:%d/%s", tcpPort, dbName)
	opts, _ := clickhousego.ParseDSN(dsn)
	conn, err := clickhousego.Open(opts)
	if err != nil {
		t.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Use stream-ingest-1 in the live snapshot and seed matching server_events.
	const streamID = "stream-ingest-1"
	const app = "live"
	const nodeID = "node-1"

	// Seed server_events ingest_stats rows.
	// The IngestTimeseries query filters on event_type = 'ingest_stats'.
	// Seed multiple rows spanning 3+ minutes so the bucketed timeseries is non-empty.
	now := time.Now().UTC()
	baseTs := now.Add(-10 * time.Minute)

	type ingestSeedRow struct {
		eventType   string
		ts          time.Time
		app         string
		streamID    string
		nodeID      string
		bitrateKbps float32
		fps         float32
	}
	ingestSeeds := []ingestSeedRow{
		{eventType: "ingest_stats", ts: baseTs.Add(0 * time.Minute), app: app, streamID: streamID, nodeID: nodeID, bitrateKbps: 1200.0, fps: 30.0},
		{eventType: "ingest_stats", ts: baseTs.Add(1 * time.Minute), app: app, streamID: streamID, nodeID: nodeID, bitrateKbps: 1250.0, fps: 30.0},
		{eventType: "ingest_stats", ts: baseTs.Add(2 * time.Minute), app: app, streamID: streamID, nodeID: nodeID, bitrateKbps: 1180.0, fps: 29.5},
		{eventType: "ingest_stats", ts: baseTs.Add(3 * time.Minute), app: app, streamID: streamID, nodeID: nodeID, bitrateKbps: 1230.0, fps: 30.0},
	}

	for _, s := range ingestSeeds {
		if err := conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.server_events
				(event_type, ts, app, stream_id, node_id, bitrate_kbps, fps)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, dbName),
			s.eventType, s.ts, s.app, s.streamID, s.nodeID, s.bitrateKbps, s.fps,
		); err != nil {
			t.Fatalf("seed ingest_stats row: %v", err)
		}
	}
	t.Logf("VD-24: seeded %d ingest_stats rows for stream %s", len(ingestSeeds), streamID)

	// Set up the API server with the real CH conn and the live provider with the seeded stream.
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "vd24-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	adminToken := "plt_vd24_testtoken_abcdef"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "vd24-admin",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// GET /qoe/ingest is gated behind CheckDataAPI (Pro+) since D-041; provision a Pro license.
	proKey, licCleanup := makeTestProLicense(t)
	defer licCleanup()
	lic, err := license.New(proKey, "")
	if err != nil {
		t.Fatalf("license.New (pro): %v", err)
	}
	live := &vd24LiveProvider{streamID: streamID, app: app, nodeID: nodeID}
	// Wire real CH conn so IngestTimeseries queries real data.
	qsvc := query.New(live, conn, lic)

	apiCfg := api.Config{ListenAddr: ":0"}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest", nil)
	req.Header.Set("Authorization", authHeader(adminToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/qoe/ingest: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("VD-24: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("VD-24: decode response: %v", err)
	}

	streamsRaw, ok := result["streams"]
	if !ok {
		t.Fatal("VD-24 FAIL: response missing 'streams' key")
	}
	streams, ok := streamsRaw.([]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("VD-24 FAIL: expected at least 1 stream in response, got %v", streamsRaw)
	}

	streamMap, ok := streams[0].(map[string]any)
	if !ok {
		t.Fatalf("VD-24 FAIL: stream entry is not an object: %T", streams[0])
	}

	timeseriesRaw, hasTimeseries := streamMap["timeseries"]
	if !hasTimeseries {
		t.Fatalf("VD-24 FAIL: stream missing 'timeseries' field")
	}
	timeseries, isSlice := timeseriesRaw.([]any)
	if !isSlice {
		t.Fatalf("VD-24 FAIL: 'timeseries' must be array, got %T", timeseriesRaw)
	}

	// KEY ASSERTION: timeseries must be NON-empty (seeded 4 ingest_stats rows → ≥1 bucket).
	if len(timeseries) == 0 {
		t.Fatalf("VD-24 FAIL: timeseries is empty [] — IngestTimeseries did not query seeded data (real CH conn not wired or query returned no rows)")
	}

	t.Logf("PASS VD-24: timeseries has %d bucket(s) from seeded ingest_stats (non-empty, real CH conn verified)", len(timeseries))
}
