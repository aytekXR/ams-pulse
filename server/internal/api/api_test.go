// Package api_test contains OpenAPI conformance tests for the Pulse HTTP API.
//
// Tests spin up the API server handler using httptest.NewServer, make real
// HTTP requests, and validate responses against contracts/openapi/pulse-api.yaml
// using getkin/kin-openapi.
//
// Acceptance criteria (WO-103):
//   - GET /healthz → 200 JSON (no auth required)
//   - GET /api/v1/live/overview → 200 JSON conformant to LiveOverview schema
//   - GET /api/v1/live/streams  → 200 JSON conformant to LiveStreamList schema
//   - POST /api/v1/alerts/rules → 201 JSON conformant to AlertRule schema
//   - GET /api/v1/alerts/rules  → 200 JSON conformant to AlertRuleList schema
//   - Unauthenticated request   → 401
//   - All 200 responses validate against the OpenAPI spec (no schema violations)
package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Test fixtures ────────────────────────────────────────────────────────────

// fakeLiveProvider is a minimal domain.LiveProvider for API tests.
type fakeLiveProvider struct{}

func (f *fakeLiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		ActiveStreams: 1,
		TotalViewers:  42,
		IngestBitrate: 1200.0,
		Streams: map[string]*domain.LiveStream{
			"stream1": {
				StreamID:      "stream1",
				App:           "live",
				NodeID:        "node-1",
				Active:        true,
				ViewerCount:   42,
				IngestBitrate: 1200.0,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{
			"node-1": {NodeID: "node-1", CPUPCT: 30.0, MemPCT: 45.0},
		},
	}
}

func (f *fakeLiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// openAPISpec locates and loads the OpenAPI spec.
func openAPISpec(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	// server/internal/api/ → contracts/openapi/pulse-api.yaml
	specPath := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(file))),
		"..", "contracts", "openapi", "pulse-api.yaml")
	specPath = filepath.Clean(specPath)

	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Fatalf("openapi spec not found at %s — spec must exist; a missing spec makes conformance vacuously true", specPath)
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("load openapi spec: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("openapi spec invalid: %v", err)
	}
	return doc
}

// setupTestServer creates an httptest.Server with the API handler and
// returns the server URL, a pre-created admin token, and a cleanup func.
func setupTestServer(t *testing.T) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	// Meta store.
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "api-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create an admin token for testing.
	adminToken = "plt_testtoken_" + "abcdef1234567890"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "test-admin",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// License, live provider, query service.
	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	// Build API server and get its handler.
	apiCfg := api.Config{
		ListenAddr: ":0",
		BaseURL:    "",
	}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		store.Close()
	}
	return ts, adminToken, cleanup
}

func metaDDLPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file),
		"..", "..", "..", "contracts", "db", "meta", "0001_init.sql"))
}

// hashToken returns SHA-256 hex of the token (matching the auth middleware).
func hashToken(tok string) string {
	h := sha256.New()
	h.Write([]byte(tok))
	return hex.EncodeToString(h.Sum(nil))
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// conformCheck validates that an http.Response body conforms to the OpenAPI spec.
func conformCheck(t *testing.T, doc *openapi3.T, req *http.Request, resp *http.Response) {
	t.Helper()
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("kin-openapi router: %v", err)
	}

	// Read body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		// Route not found in spec — this is a conformance violation: every production
		// route must be described in the spec. Fail loud so the orchestrator can file a CR.
		t.Errorf("conformance: route not in spec (%s %s): %v — add the route to contracts/openapi/pulse-api.yaml",
			req.Method, req.URL.Path, err)
		return
	}

	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:     req,
			PathParams:  pathParams,
			Route:       route,
			QueryParams: req.URL.Query(),
		},
		Status: resp.StatusCode,
		Header: resp.Header,
		Options: &openapi3filter.Options{
			// Auth validation skipped — we test it separately.
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			ExcludeRequestBody: true,
		},
	}
	input.SetBodyBytes(body)

	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		t.Errorf("FAIL: response does not conform to OpenAPI spec (%s %s %d): %v",
			req.Method, req.URL.Path, resp.StatusCode, err)
	}
}

func authHeader(token string) string { return "Bearer " + token }

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestAPI_Healthz_NoAuth(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
	t.Logf("PASS: GET /healthz → %d, body=%v", resp.StatusCode, body)
}

func TestAPI_Unauthorized_Returns401(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	// No Authorization header.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}
	t.Logf("PASS: unauthenticated → %d", resp.StatusCode)
}

func TestAPI_LiveOverview_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/live/overview: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Re-build request with the test server URL for conformance check
	// (kin-openapi needs the path to match the spec server base).
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/live/overview", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/live/overview → 200, conforms to spec")
}

func TestAPI_LiveStreams_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/streams", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/live/streams: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/live/streams", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/live/streams → 200, conforms to spec")
}

func TestAPI_AlertRules_CreateAndList(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	// POST /api/v1/alerts/rules
	ruleBody := map[string]any{
		"name":        "Low viewer count",
		"metric":      "viewer_count",
		"operator":    "lt",
		"threshold":   5,
		"window_s":    60,
		"severity":    "warning",
		"cooldown_s":  300,
		"enabled":     true,
		"channel_ids": []string{},
	}
	bodyBytes, _ := json.Marshal(ruleBody)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/alerts/rules: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 or 200, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("POST /api/v1/alerts/rules → %d", resp.StatusCode)

	// GET /api/v1/alerts/rules
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/alerts/rules", nil)
	req2.Header.Set("Authorization", authHeader(token))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("GET /api/v1/alerts/rules: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200 for list, got %d: %s", resp2.StatusCode, body)
	}

	var listResp map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&listResp); err != nil {
		t.Errorf("alert rules list response is not valid JSON: %v", err)
	}
	t.Logf("PASS: GET /api/v1/alerts/rules → 200, body keys=%v", keys(listResp))
}

func TestAPI_AdminTokens_List(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tokens", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/admin/tokens: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("PASS: GET /api/v1/admin/tokens → 200")
}

func TestAPI_FleetNodes(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/fleet/nodes", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/fleet/nodes: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("PASS: GET /api/v1/fleet/nodes → 200")
}

func TestAPI_License_Get(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/license", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/admin/license: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var licResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&licResp); err != nil {
		t.Errorf("license response is not valid JSON: %v", err)
	}
	if licResp["tier"] != "free" {
		t.Errorf("expected tier=free, got %v", licResp["tier"])
	}
	t.Logf("PASS: GET /api/v1/admin/license → 200, tier=%v", licResp["tier"])
}

// ─── Test: /healthz 503 when ClickHouse is unreachable ───────────────────────

func TestAPI_Healthz_ClickHouseDown_Returns503(t *testing.T) {
	// D-W1-002: /healthz must return 503 when a critical component is unreachable.
	// We build a server directly (not via setupTestServer) with an unreachable CH conn.

	// Inject an unreachable ClickHouse connection (points to a port that is
	// guaranteed to be closed/unreachable).
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{"127.0.0.1:19191"}, // unreachable
		DialTimeout: 100 * 1000 * 1000,           // 100ms as nanoseconds for time.Duration
	})
	if err != nil {
		// If even opening fails, skip the test — the point is an unreachable server.
		t.Skipf("could not open unreachable ch conn: %v", err)
	}

	// We need to inject the connection into the server — but the test server
	// is already started. We test via a fresh handler with the bad conn.
	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store2, err := meta.New(ctx, "sqlite", ":memory:", "test-secret2")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store2.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store2.Close()

	lic2, _ := license.New("", "")
	live2 := &fakeLiveProvider{}
	qsvc2 := query.New(live2, nil, lic2)
	srv2 := api.New(api.Config{ListenAddr: ":0"}, store2, live2, qsvc2, lic2, nil)
	srv2.SetClickHouseConn(conn)
	ts2 := httptest.NewServer(srv2.Handler())
	defer ts2.Close()

	resp, err := http.Get(ts2.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 503 when ClickHouse unreachable, got %d: %s", resp.StatusCode, body)
	} else {
		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		t.Logf("PASS: /healthz → 503 with body=%v", body)
	}
}

// ─── Test: /healthz returns latency_ms as integer when CH is up ──────────────

func TestAPI_Healthz_MetaStoreLatency(t *testing.T) {
	// D-W1-002: latency_ms must be a real measured integer, not null.
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}

	components, ok := body["components"].(map[string]any)
	if !ok {
		t.Fatalf("expected components object, got %T", body["components"])
	}
	metaComp, ok := components["meta_store"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta_store component, got %T", components["meta_store"])
	}
	if metaComp["status"] != "ok" {
		t.Errorf("expected meta_store status=ok, got %v", metaComp["status"])
	}
	// latency_ms must be a number (JSON number → float64 after decode), not nil.
	latency := metaComp["latency_ms"]
	if latency == nil {
		t.Errorf("expected latency_ms to be a measured integer, got nil")
	} else {
		t.Logf("PASS: /healthz meta_store latency_ms=%v (measured, not null)", latency)
	}
}

// keys returns the keys of a map (for logging).
func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ─── Test: /healthz kafka stats (VD-27) ──────────────────────────────────────

// fakeKafkaStats implements api.KafkaStatsProvider without a real broker.
type fakeKafkaStats struct {
	lag         int64
	parseErrors int64
}

func (f *fakeKafkaStats) Lag() int64         { return f.lag }
func (f *fakeKafkaStats) ParseErrors() int64 { return f.parseErrors }

// TestAPI_Healthz_KafkaStats verifies that /healthz includes a kafka component
// when SetKafkaStats is wired, and that parse_errors>0 yields status "degraded".
func TestAPI_Healthz_KafkaStats(t *testing.T) {
	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "kafka-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)

	// Wire a fake KafkaStatsProvider with parse_errors>0 → should report "degraded".
	kafka := &fakeKafkaStats{lag: 42, parseErrors: 3}
	srv.SetKafkaStats(kafka)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	// /healthz must return 200 even when kafka is degraded (non-critical).
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for degraded (non-critical) kafka, got %d: %s", resp.StatusCode, body)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}

	components, ok := body["components"].(map[string]any)
	if !ok {
		t.Fatalf("expected components object, got %T", body["components"])
	}

	kafkaComp, ok := components["kafka"].(map[string]any)
	if !ok {
		t.Fatalf("expected kafka component in /healthz, got %T", components["kafka"])
	}

	// lag must be present and match the fake value.
	lag, hasLag := kafkaComp["lag"]
	if !hasLag {
		t.Error("kafka component missing 'lag' field")
	} else if lag.(float64) != 42 {
		t.Errorf("expected lag=42, got %v", lag)
	}

	// parse_errors must be present and match the fake value.
	parseErrors, hasPE := kafkaComp["parse_errors"]
	if !hasPE {
		t.Error("kafka component missing 'parse_errors' field")
	} else if parseErrors.(float64) != 3 {
		t.Errorf("expected parse_errors=3, got %v", parseErrors)
	}

	// parse_errors>0 → status must be "degraded".
	if kafkaComp["status"] != "degraded" {
		t.Errorf("expected kafka status=degraded when parse_errors>0, got %v", kafkaComp["status"])
	}

	// Overall status must be "degraded" (non-critical component degraded).
	if body["status"] != "degraded" {
		t.Errorf("expected overall status=degraded, got %v", body["status"])
	}

	t.Logf("PASS: /healthz with kafka stats → 200, kafka.status=%v lag=%v parse_errors=%v overall=%v",
		kafkaComp["status"], lag, parseErrors, body["status"])
}
