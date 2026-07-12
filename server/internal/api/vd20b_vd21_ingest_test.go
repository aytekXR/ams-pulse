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
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

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
		// A missing DDL is test-infrastructure failure, not a legitimate skip:
		// silent skips are the D-028 false-green class. Fail loud.
		t.Fatalf("meta DDL not found: %v", err)
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

// ─── BUG-004 regression tests ──────────────────────────────────────────────────
//
// handleIngestHealth ignored every declared OpenAPI query parameter: from, to,
// app, stream, node.  These tests are the TDD specification for the fix.

// captureIngestQsvc implements api.IngestQuerier and records every
// IngestTimeseriesParams it receives.  Used to assert that handleIngestHealth
// correctly plumbs from/to and app/stream/node params through to the query layer.
type captureIngestQsvc struct {
	captured []query.IngestTimeseriesParams
}

func (c *captureIngestQsvc) IngestTimeseries(_ context.Context, p query.IngestTimeseriesParams) (*query.IngestTimeseriesResult, error) {
	c.captured = append(c.captured, p)
	return &query.IngestTimeseriesResult{
		Timeseries: []query.IngestBucket{},
		DropEvents: []query.DropEvent{},
	}, nil
}

// setupIngestCaptureServer mirrors setupHealthyTestServer but injects the given
// IngestQuerier double so tests can observe IngestTimeseriesParams.
func setupIngestCaptureServer(t *testing.T, iq api.IngestQuerier) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	licKey, licCleanup := makeTestBusinessLicense(t)

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		// A missing DDL is test-infrastructure failure, not a legitimate skip:
		// silent skips are the D-028 false-green class. Fail loud.
		t.Fatalf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "bug004-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_bug004_testtoken_abcdef"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "bug004-admin",
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
	qsvc := query.New(live, nil, lic)

	apiCfg := api.Config{ListenAddr: ":0"}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	srv.SetIngestTracker(&fakeIngestTracker{})
	if iq != nil {
		srv.SetIngestQuerier(iq)
	}

	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
		licCleanup()
	}
	return ts, adminToken, cleanup
}

// TestBUG004_IngestHealth_HonorsTimeRange proves that from/to query params
// reach IngestTimeseriesParams.From / .To (BUG-004 core regression).
// Table-driven: epoch-ms, RFC3339, only-from, only-to, and absent (back-compat
// zero time → no time filter) cases.
func TestBUG004_IngestHealth_HonorsTimeRange(t *testing.T) {
	epochMs1 := int64(1_700_000_000_000) // 2023-11-14T22:13:20Z
	epochMs2 := int64(1_700_100_000_000)
	rfc1 := time.UnixMilli(epochMs1).UTC().Format(time.RFC3339)
	rfc2 := time.UnixMilli(epochMs2).UTC().Format(time.RFC3339)

	cases := []struct {
		name      string
		fromParam string
		toParam   string
		wantFrom  time.Time
		wantTo    time.Time
		wantCalls int
	}{
		{
			name:      "epoch-ms both",
			fromParam: strconv.FormatInt(epochMs1, 10),
			toParam:   strconv.FormatInt(epochMs2, 10),
			wantFrom:  time.UnixMilli(epochMs1),
			wantTo:    time.UnixMilli(epochMs2),
			wantCalls: 1,
		},
		{
			name:      "RFC3339 both",
			fromParam: rfc1,
			toParam:   rfc2,
			wantFrom:  time.UnixMilli(epochMs1).UTC(),
			wantTo:    time.UnixMilli(epochMs2).UTC(),
			wantCalls: 1,
		},
		{
			name:      "absent both – back-compat zero",
			fromParam: "",
			toParam:   "",
			wantFrom:  time.Time{}, // zero → no time filter passed to IngestTimeseries
			wantTo:    time.Time{},
			wantCalls: 1,
		},
		{
			name:      "only from provided",
			fromParam: strconv.FormatInt(epochMs1, 10),
			toParam:   "",
			wantFrom:  time.UnixMilli(epochMs1),
			wantTo:    time.Time{}, // zero
			wantCalls: 1,
		},
		{
			name:      "only to provided",
			fromParam: "",
			toParam:   strconv.FormatInt(epochMs2, 10),
			wantFrom:  time.Time{}, // zero
			wantTo:    time.UnixMilli(epochMs2),
			wantCalls: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureIngestQsvc{}
			ts, token, cleanup := setupIngestCaptureServer(t, cap)
			defer cleanup()

			vals := url.Values{}
			if tc.fromParam != "" {
				vals.Set("from", tc.fromParam)
			}
			if tc.toParam != "" {
				vals.Set("to", tc.toParam)
			}
			rawURL := ts.URL + "/api/v1/qoe/ingest"
			if len(vals) > 0 {
				rawURL += "?" + vals.Encode()
			}

			req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
			req.Header.Set("Authorization", authHeader(token))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET /qoe/ingest: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
			}

			if len(cap.captured) != tc.wantCalls {
				t.Fatalf("BUG-004: want %d IngestTimeseries call(s), got %d (handler may not be using iqsvc or not plumbing params)",
					tc.wantCalls, len(cap.captured))
			}
			if tc.wantCalls == 0 {
				return
			}
			got := cap.captured[0]
			if !got.From.Equal(tc.wantFrom) {
				t.Errorf("BUG-004: IngestTimeseriesParams.From = %v, want %v", got.From, tc.wantFrom)
			}
			if !got.To.Equal(tc.wantTo) {
				t.Errorf("BUG-004: IngestTimeseriesParams.To = %v, want %v", got.To, tc.wantTo)
			}
			if t.Failed() {
				return
			}
			t.Logf("PASS %s: From=%v To=%v", tc.name, got.From, got.To)
		})
	}
}

// TestBUG004_IngestHealth_AppStreamNodeFilter proves that app/stream/node query
// params filter which streams from the live snapshot appear in the response.
// fakeHealthyLiveProvider has one stream: id="healthy-stream-1", app="live", node="node-1".
func TestBUG004_IngestHealth_AppStreamNodeFilter(t *testing.T) {
	cases := []struct {
		name        string
		queryStr    string
		wantStreams int // streams array length in JSON response
		wantCalls   int // IngestTimeseries capture calls
	}{
		{name: "no filter", queryStr: "", wantStreams: 1, wantCalls: 1},
		{name: "app match", queryStr: "app=live", wantStreams: 1, wantCalls: 1},
		{name: "app no match", queryStr: "app=other", wantStreams: 0, wantCalls: 0},
		{name: "stream match", queryStr: "stream=healthy-stream-1", wantStreams: 1, wantCalls: 1},
		{name: "stream no match", queryStr: "stream=other-stream", wantStreams: 0, wantCalls: 0},
		{name: "node match", queryStr: "node=node-1", wantStreams: 1, wantCalls: 1},
		{name: "node no match", queryStr: "node=other-node", wantStreams: 0, wantCalls: 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureIngestQsvc{}
			ts, token, cleanup := setupIngestCaptureServer(t, cap)
			defer cleanup()

			rawURL := ts.URL + "/api/v1/qoe/ingest"
			if tc.queryStr != "" {
				rawURL += "?" + tc.queryStr
			}

			req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
			req.Header.Set("Authorization", authHeader(token))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
			}

			var result map[string]any
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatalf("decode: %v", err)
			}
			streams, _ := result["streams"].([]any)
			if len(streams) != tc.wantStreams {
				t.Errorf("BUG-004 filter %q: want %d stream(s) in response, got %d",
					tc.queryStr, tc.wantStreams, len(streams))
			}
			if len(cap.captured) != tc.wantCalls {
				t.Errorf("BUG-004 filter %q: want %d IngestTimeseries call(s), got %d",
					tc.queryStr, tc.wantCalls, len(cap.captured))
			}
			if !t.Failed() {
				t.Logf("PASS %s: streams=%d calls=%d", tc.name, len(streams), len(cap.captured))
			}
		})
	}
}

// TestBUG005_IngestHealth_HonorsBucketInterval proves that the `interval` query
// param is parsed by parseBucketInterval and reaches IngestTimeseriesParams.BucketSeconds
// (BUG-005 regression guard).
//
// F4 deviation note: absent `interval` → BucketSeconds=0 → IngestTimeseries uses
// its internal 60-second bucket default, preserving PRD F4's 15-second visibility
// requirement. The OpenAPI default of "day" (86400 s) is deliberately NOT applied
// when the param is absent, because a 24-hour bucket hides sub-minute degradation.
func TestBUG005_IngestHealth_HonorsBucketInterval(t *testing.T) {
	epochMs1 := int64(1_700_000_000_000) // 2023-11-14T22:13:20Z
	epochMs2 := int64(1_700_100_000_000)

	cases := []struct {
		name           string
		queryStr       string
		wantBucketSecs int
		wantFrom       time.Time
		wantTo         time.Time
		wantCalls      int
	}{
		{
			name:           "hour",
			queryStr:       "interval=hour",
			wantBucketSecs: 3600,
			wantCalls:      1,
		},
		{
			name:           "day",
			queryStr:       "interval=day",
			wantBucketSecs: 86400,
			wantCalls:      1,
		},
		{
			name:           "absent",
			queryStr:       "",
			wantBucketSecs: 0, // F4 deviation: absent → 0 → IngestTimeseries uses 60s default
			wantCalls:      1,
		},
		{
			name:           "invalid-week",
			queryStr:       "interval=week",
			wantBucketSecs: 0, // lenient: unknown value → 0
			wantCalls:      1,
		},
		{
			name:           "combined",
			queryStr:       "interval=hour&from=" + strconv.FormatInt(epochMs1, 10) + "&to=" + strconv.FormatInt(epochMs2, 10),
			wantBucketSecs: 3600,
			wantFrom:       time.UnixMilli(epochMs1),
			wantTo:         time.UnixMilli(epochMs2),
			wantCalls:      1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Fresh captureIngestQsvc per subtest to avoid slice accumulation.
			cap := &captureIngestQsvc{}
			ts, token, cleanup := setupIngestCaptureServer(t, cap)
			defer cleanup()

			rawURL := ts.URL + "/api/v1/qoe/ingest"
			if tc.queryStr != "" {
				rawURL += "?" + tc.queryStr
			}

			req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
			req.Header.Set("Authorization", authHeader(token))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET /qoe/ingest: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("BUG-005: want 200, got %d: %s", resp.StatusCode, body)
			}

			if len(cap.captured) != tc.wantCalls {
				t.Fatalf("BUG-005: want %d IngestTimeseries call(s), got %d (interval plumbing broken?)",
					tc.wantCalls, len(cap.captured))
			}
			if tc.wantCalls == 0 {
				return
			}
			got := cap.captured[0]

			t.Logf("BUG-005 fix check [%s]: got BucketSeconds=%d, want %d",
				tc.name, got.BucketSeconds, tc.wantBucketSecs)
			if got.BucketSeconds != tc.wantBucketSecs {
				t.Errorf("BUG-005: IngestTimeseriesParams.BucketSeconds = %d, want %d (interval=%q)",
					got.BucketSeconds, tc.wantBucketSecs, tc.queryStr)
			}

			// For the "combined" case, also assert From and To are plumbed.
			if tc.name == "combined" {
				if !got.From.Equal(tc.wantFrom) {
					t.Errorf("BUG-005 combined: From = %v, want %v", got.From, tc.wantFrom)
				}
				if !got.To.Equal(tc.wantTo) {
					t.Errorf("BUG-005 combined: To = %v, want %v", got.To, tc.wantTo)
				}
			}

			if !t.Failed() {
				t.Logf("PASS BUG-005 [%s]: BucketSeconds=%d as expected", tc.name, got.BucketSeconds)
			}
		})
	}
}

// TestBUG004_IngestHealth_BackCompat_NoParams proves byte-identical back-compat:
// absent from/to → zero time.Time → no time filter in IngestTimeseries; all
// active streams from the live snapshot are still returned.
func TestBUG004_IngestHealth_BackCompat_NoParams(t *testing.T) {
	cap := &captureIngestQsvc{}
	ts, token, cleanup := setupIngestCaptureServer(t, cap)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	streams, _ := result["streams"].([]any)
	if len(streams) != 1 {
		t.Errorf("back-compat: want 1 active stream in response, got %d", len(streams))
	}
	if len(cap.captured) != 1 {
		t.Fatalf("back-compat: want 1 IngestTimeseries call, got %d (iqsvc not wired?)", len(cap.captured))
	}
	got := cap.captured[0]
	if !got.From.IsZero() {
		t.Errorf("back-compat: want zero From (no time filter), got %v", got.From)
	}
	if !got.To.IsZero() {
		t.Errorf("back-compat: want zero To (no time filter), got %v", got.To)
	}
	if !t.Failed() {
		t.Logf("PASS back-compat: From=%v (zero) To=%v (zero) — no time filter applied", got.From, got.To)
	}
}
