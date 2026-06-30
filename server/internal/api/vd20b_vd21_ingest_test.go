// Package api_test — VD-20b + VD-21 acceptance tests.
//
// VD-20b: GET /qoe/ingest returns health_score > 0 when the live snapshot has
// a stream with a non-zero HealthScore (wired by BE-01 VD-20a).
//
// VD-21: GET /qoe/ingest response includes `timeseries` and `drop_events` per
// the OpenAPI IngestStream schema (both are always present; timeseries may be
// empty when ClickHouse is unavailable, but the keys must exist).
//
// VD-23: SetIngestTracker wired from serve.go; interface Snapshot() return type
// matches ingest.HealthTracker. This test verifies the interface is satisfied.
package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/collector/ingest"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// fakeHealthyLiveProvider returns a snapshot with a stream whose HealthScore > 0.
// This simulates the state after BE-01 VD-20a: aggregator.onIngestStats now
// calls ingest.ComputeHealthScore, so LiveStream.HealthScore is non-zero.
type fakeHealthyLiveProvider struct{}

func (f *fakeHealthyLiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		ActiveStreams: 1,
		TotalViewers:  5,
		IngestBitrate: 1500.0,
		Streams: map[string]*domain.LiveStream{
			"healthy-stream-1": {
				StreamID:          "healthy-stream-1",
				App:               "live",
				NodeID:            "node-1",
				Active:            true,
				ViewerCount:       5,
				IngestBitrate:     1500.0,
				FPS:               30.0,
				HealthScore:       0.95, // non-zero: VD-20b
				Health:            domain.StreamHealthGood,
				PacketLossPct:     0.1,
				JitterMS:          5.0,
				KeyframeIntervalS: 2.0,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{
			"node-1": {NodeID: "node-1", CPUPCT: 20.0, MemPCT: 40.0},
		},
	}
}

func (f *fakeHealthyLiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// fakeIngestTracker satisfies the api.IngestTracker interface (VD-23 type fix).
// Snapshot() returns map[string]ingest.PublisherState — matching HealthTracker.
type fakeIngestTracker struct{}

func (f *fakeIngestTracker) Snapshot() map[string]ingest.PublisherState {
	return map[string]ingest.PublisherState{
		"node-1/live/healthy-stream-1": {
			StreamID:    "healthy-stream-1",
			App:         "live",
			NodeID:      "node-1",
			BitrateKbps: 1500.0,
			FPS:         30.0,
			HealthScore: 0.95,
			Health:      domain.StreamHealthGood,
		},
	}
}

// setupHealthyTestServer creates an httptest.Server with a non-zero HealthScore snapshot.
// Uses a Business-tier license so that CheckDataAPI passes for /qoe/ingest (Pro+ gate).
func setupHealthyTestServer(t *testing.T) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	licKey, licCleanup := makeTestBusinessLicense(t)

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "vd20b-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_vd20b_testtoken_abcdef"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "vd20b-admin",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("CreateToken: %v", err)
	}

	lic, err := license.New(licKey, "")
	if err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("license.New (business): %v", err)
	}
	live := &fakeHealthyLiveProvider{}
	qsvc := query.New(live, nil, lic) // nil conn = ClickHouse not configured; IngestTimeseries returns []

	apiCfg := api.Config{ListenAddr: ":0"}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)

	// VD-23: SetIngestTracker is now called; fakeIngestTracker satisfies the
	// updated interface (Snapshot() → map[string]ingest.PublisherState).
	srv.SetIngestTracker(&fakeIngestTracker{})

	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
		licCleanup()
	}
	return ts, adminToken, cleanup
}

// TestVD20b_IngestHealth_HealthScoreNonZero guards VD-20b:
// GET /api/v1/qoe/ingest must return health_score > 0 when the live snapshot
// has a stream with HealthScore > 0 (set by aggregator.onIngestStats via BE-01 VD-20a).
func TestVD20b_IngestHealth_HealthScoreNonZero(t *testing.T) {
	ts, token, cleanup := setupHealthyTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /qoe/ingest: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("VD-20b: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	streamsRaw, ok := result["streams"]
	if !ok {
		t.Fatal("VD-20b FAIL: response missing 'streams' key")
	}
	streams, ok := streamsRaw.([]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("VD-20b FAIL: expected at least 1 stream in response, got %v", streamsRaw)
	}

	streamMap, ok := streams[0].(map[string]any)
	if !ok {
		t.Fatalf("VD-20b FAIL: stream entry is not an object: %T", streams[0])
	}

	healthScore, ok := streamMap["health_score"].(float64)
	if !ok {
		t.Fatalf("VD-20b FAIL: health_score field missing or wrong type in stream: %v", streamMap)
	}
	if healthScore <= 0 {
		t.Errorf("VD-20b FAIL: health_score = %v, want > 0 (LiveStream.HealthScore=0.95 should scale to 95.0)",
			healthScore)
	} else {
		t.Logf("PASS VD-20b: health_score = %v > 0 (raw=0.95 × 100 = 95.0)", healthScore)
	}
}

// TestVD21_IngestHealth_TimeseriesAndDropEventsPresent guards VD-21:
// GET /api/v1/qoe/ingest response must include `timeseries` and `drop_events`
// keys per the OpenAPI IngestStream schema. Both fields must always be present
// (even if empty) — the UI renders stream.timeseries.map() and stream.drop_events.
func TestVD21_IngestHealth_TimeseriesAndDropEventsPresent(t *testing.T) {
	ts, token, cleanup := setupHealthyTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /qoe/ingest: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("VD-21: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	streams, _ := result["streams"].([]any)
	if len(streams) == 0 {
		t.Fatal("VD-21 FAIL: no streams in response")
	}

	streamMap, _ := streams[0].(map[string]any)

	// `timeseries` must be present (may be empty array when ClickHouse not configured).
	timeseriesRaw, hasTimeseries := streamMap["timeseries"]
	if !hasTimeseries {
		t.Errorf("VD-21 FAIL: stream missing required 'timeseries' field; keys=%v", mapKeys(streamMap))
	} else {
		_, isSlice := timeseriesRaw.([]any)
		if !isSlice {
			t.Errorf("VD-21 FAIL: 'timeseries' must be array, got %T", timeseriesRaw)
		} else {
			t.Logf("PASS VD-21: 'timeseries' present (len=%d)", len(timeseriesRaw.([]any)))
		}
	}

	// `drop_events` must be present (may be empty array).
	dropEventsRaw, hasDropEvents := streamMap["drop_events"]
	if !hasDropEvents {
		t.Errorf("VD-21 FAIL: stream missing 'drop_events' field; keys=%v", mapKeys(streamMap))
	} else {
		_, isSlice := dropEventsRaw.([]any)
		if !isSlice {
			t.Errorf("VD-21 FAIL: 'drop_events' must be array, got %T", dropEventsRaw)
		} else {
			t.Logf("PASS VD-21: 'drop_events' present (len=%d)", len(dropEventsRaw.([]any)))
		}
	}
}

// TestVD23_IngestTracker_InterfaceConformance guards VD-23:
// api.IngestTracker.Snapshot() return type must match ingest.HealthTracker.Snapshot(),
// i.e., map[string]ingest.PublisherState. The fakeIngestTracker above satisfies
// this interface; if the interface were still map[string]interface{} the
// fakeIngestTracker would not compile (type mismatch).
//
// This test verifies SetIngestTracker can be called with a *ingest.HealthTracker
// (which satisfies the updated interface), asserting compile-time compatibility.
func TestVD23_IngestTracker_InterfaceConformance(t *testing.T) {
	// Compile-time assertion: fakeIngestTracker must satisfy api.IngestTracker.
	// If Snapshot() return type is wrong, this will not compile.
	var _ api.IngestTracker = &fakeIngestTracker{}

	// Also verify ingest.HealthTracker satisfies the interface (the real impl).
	ht := ingest.New(ingest.Config{}, nil)
	var _ api.IngestTracker = ht

	t.Logf("PASS VD-23: ingest.HealthTracker satisfies api.IngestTracker (Snapshot() → map[string]ingest.PublisherState)")
}
