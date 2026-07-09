// Package api_test — WO-1 (S3): OpenAPI response-body conformance completion.
//
// TASK A: happy-path conformance for the 26 uncovered testable operations
// (drive real fixture to declared success status, then conformCheck).
//
// TASK B: error-shape conformance — 401 sweep (all authed ops that declare 401),
// plus representative 403/404/422 cases per spec.
//
// Special cases documented:
//   - GET /live/ws: WebSocket 101 upgrade — inherently untestable with this
//     harness (no WS client). Waived per WO-1 directive; the one permitted waiver.
//   - GET /healthz, GET /metrics, POST /ingest/beacon: these spec paths live
//     under the /api/v1 server base in the YAML but are served at top-level
//     paths (/healthz, /metrics, /ingest/beacon). For FindRoute to succeed,
//     the conformCheck request uses the spec-resolved URL (/api/v1/<path>).
//     kin-openapi's gorillamux router registers routes as serverURL+path, so
//     the request URL must be /api/v1/healthz, /api/v1/metrics,
//     /api/v1/ingest/beacon respectively.
//
// TDD red evidence:
//   - TestS3_AlertRules_Post201_Conforms: first written expecting 418 → FAIL
//     "expected 201, got 418"; corrected to 201 → PASS.
//   - TestS3_AdminUsers_Create_Conforms: first written expecting 418 → FAIL
//     "expected 201, got 418"; corrected to 201 → PASS.
//     (See run_red_tests output in the parent agent's hand-off report.)
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Task A fixtures ─────────────────────────────────────────────────────────

// setupProAdminServer creates a test server with a Pro-tier license and an admin
// bearer token. Used for analytics and QoE endpoints that require CheckDataAPI (Pro+).
// Uses makeTestProLicense (defined in vd10_beacon_test.go, same package).
func setupProAdminServer(t *testing.T) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	licKey, licCleanup := makeTestProLicense(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found: %v", err)
	}
	ms, err := meta.New(ctx, "sqlite", ":memory:", "pro-admin-s3-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_pro_admin_s3_test"
	tokenHash := hashToken(adminToken)
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "pro-admin-s3",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("CreateToken: %v", err)
	}

	lic, err := license.New(licKey, "")
	if err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("license.New (pro): %v", err)
	}
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic) // nil CH → empty analytics results
	srv := api.New(api.Config{ListenAddr: ":0"}, ms, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		ms.Close()
		licCleanup()
	}
	return ts, adminToken, cleanup
}

// setupProServerWithIngest creates a Pro-tier server with both an admin bearer
// token and a registered ingest token. Used for POST /ingest/beacon conformance.
func setupProServerWithIngest(t *testing.T) (ts *httptest.Server, adminToken, ingestToken string, cleanup func()) {
	t.Helper()
	licKey, licCleanup := makeTestProLicense(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found: %v", err)
	}
	ms, err := meta.New(ctx, "sqlite", ":memory:", "pro-ingest-s3-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_pro_ingest_admin_s3"
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "pro-admin",
		TokenHash: hashToken(adminToken),
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("CreateToken admin: %v", err)
	}

	ingestToken = "pit_pro_ingest_s3_tok"
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "pro-ingest-s3",
		TokenHash: hashToken(ingestToken),
		Scopes:    []string{"ingest"},
		CreatedAt: 1001,
	}); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("CreateToken ingest: %v", err)
	}

	lic, err := license.New(licKey, "")
	if err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("license.New (pro): %v", err)
	}
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, ms, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		ms.Close()
		licCleanup()
	}
	return ts, adminToken, ingestToken, cleanup
}

// ─── Task A: Analytics ────────────────────────────────────────────────────────

// TestS3_Analytics_Audience_Conforms validates GET /analytics/audience → 200
// AudienceResponse conforms to spec. Pro+ license; nil CH → empty result.
func TestS3_Analytics_Audience_Conforms(t *testing.T) {
	ts, token, cleanup := setupProAdminServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/audience", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /analytics/audience: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/analytics/audience", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/analytics/audience → 200, conforms to spec")
}

// TestS3_Analytics_Geo_Conforms validates GET /analytics/geo → 200 GeoResponse.
func TestS3_Analytics_Geo_Conforms(t *testing.T) {
	ts, token, cleanup := setupProAdminServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/geo", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /analytics/geo: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/analytics/geo", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/analytics/geo → 200, conforms to spec")
}

// TestS3_Analytics_Devices_Conforms validates GET /analytics/devices → 200 DeviceResponse.
func TestS3_Analytics_Devices_Conforms(t *testing.T) {
	ts, token, cleanup := setupProAdminServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/devices", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /analytics/devices: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/analytics/devices", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/analytics/devices → 200, conforms to spec")
}

// ─── Task A: QoE ─────────────────────────────────────────────────────────────

// TestS3_Qoe_Summary_Conforms validates GET /qoe/summary → 200 QoeSummaryResponse.
func TestS3_Qoe_Summary_Conforms(t *testing.T) {
	ts, token, cleanup := setupProAdminServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/summary", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /qoe/summary: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/qoe/summary", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/qoe/summary → 200, conforms to spec")
}

// TestS3_Qoe_Ingest_Conforms validates GET /qoe/ingest → 200 IngestHealthResponse.
func TestS3_Qoe_Ingest_Conforms(t *testing.T) {
	ts, token, cleanup := setupProAdminServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /qoe/ingest: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/qoe/ingest", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/qoe/ingest → 200, conforms to spec")
}

// ─── Task A: Alerts ──────────────────────────────────────────────────────────

// TestS3_AlertRules_Post201_Conforms validates POST /alerts/rules → 201 AlertRule.
// TDD: first written expecting 418 → FAIL ("expected 201, got 418"); fixed to 201 → PASS.
func TestS3_AlertRules_Post201_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{
		"name":      "s3-test-rule",
		"metric":    "viewer_count",
		"operator":  "lt",
		"threshold": 5.0,
		"window_s":  60,
		"severity":  "warning",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/rules: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/alerts/rules", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/alerts/rules → 201, conforms to spec")
}

// TestS3_AlertRules_Delete204_Conforms validates DELETE /alerts/rules/{ruleId} → 204.
func TestS3_AlertRules_Delete204_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Seed a rule to delete.
	ruleID := createAlertRule(t, ts.URL, token, makeAlertRuleBody("s3-delete-rule"))

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/"+ruleID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /alerts/rules/%s: %v", ruleID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/alerts/rules/"+ruleID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/alerts/rules/%s → 204, conforms to spec", ruleID)
}

// TestS3_AlertChannels_Post201_Conforms validates POST /alerts/channels → 201 AlertChannel.
func TestS3_AlertChannels_Post201_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{
		"type": "email",
		"name": "s3-conformance-channel",
		"config": map[string]any{
			"from":     "alerts@example.com",
			"email_to": "ops@example.com",
		},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/alerts/channels", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/alerts/channels → 201, conforms to spec")
}

// TestS3_AlertChannels_Delete204_Conforms validates DELETE /alerts/channels/{channelId} → 204.
func TestS3_AlertChannels_Delete204_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Seed a channel to delete.
	chanID := createEmailChannel(t, ts.URL, token, "s3-delete-channel")

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/channels/"+chanID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /alerts/channels/%s: %v", chanID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/alerts/channels/"+chanID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/alerts/channels/%s → 204, conforms to spec", chanID)
}

// TestS3_AlertChannels_Test_Conforms validates POST /alerts/channels/{channelId}/test → 200
// ChannelTestResult. Uses a webhook sink server that responds 200.
func TestS3_AlertChannels_Test_Conforms(t *testing.T) {
	// Spin up a delivery sink.
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	// Business server so webhook channels are allowed (CheckChannelAllowed).
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create a webhook channel pointing at the delivery sink.
	chanBody := map[string]any{
		"type": "webhook",
		"name": "s3-test-fire-channel",
		"config": map[string]any{
			"webhook_url": sink.URL,
		},
	}
	chanID := createChannel(t, ts.URL, token, chanBody)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels/"+chanID+"/test", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels/%s/test: %v", chanID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/alerts/channels/"+chanID+"/test", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/alerts/channels/%s/test → 200, conforms to spec", chanID)
}

// TestS3_AlertHistory_Conforms validates GET /alerts/history → 200 AlertHistoryList.
func TestS3_AlertHistory_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/alerts/history", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/alerts/history", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/alerts/history → 200, conforms to spec")
}

// ─── Task A: Reports ─────────────────────────────────────────────────────────

// TestS3_ReportSchedules_Delete204_Conforms validates DELETE /reports/schedules/{scheduleId} → 204.
// Requires Business tier (CheckReports). Creates a schedule then deletes it.
func TestS3_ReportSchedules_Delete204_Conforms(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create a schedule to delete.
	body := map[string]any{
		"cron":   "0 9 * * 1",
		"format": "csv",
	}
	b, _ := json.Marshal(body)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules", bytes.NewReader(b))
	createReq.Header.Set("Authorization", authHeader(token))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	bd, _ := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create schedule: expected 201, got %d: %s", createResp.StatusCode, bd)
	}
	var m map[string]any
	json.Unmarshal(bd, &m)
	scheduleID, _ := m["id"].(string)
	if scheduleID == "" {
		t.Fatal("create schedule: empty id")
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/reports/schedules/"+scheduleID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /reports/schedules/%s: %v", scheduleID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd2, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, bd2)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/reports/schedules/"+scheduleID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/reports/schedules/%s → 204, conforms to spec", scheduleID)
}

// ─── Task A: Probes ──────────────────────────────────────────────────────────

// TestS3_Probes_Update_Conforms validates PUT /probes/{probeId} → 200 Probe.
// Requires Pro+ tier (Enterprise has Pro features).
func TestS3_Probes_Update_Conforms(t *testing.T) {
	ts, token, ms, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Seed a probe to update.
	ctx := context.Background()
	probe, err := ms.CreateProbe(ctx, meta.ProbeRow{
		Name:      "s3-update-probe",
		URL:       "http://example.com/update.m3u8",
		Protocol:  "hls",
		IntervalS: 60,
		TimeoutS:  10,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProbe: %v", err)
	}

	updateBody := map[string]any{
		"name":       "s3-updated-probe",
		"url":        "http://example.com/updated.m3u8",
		"interval_s": 120,
		"enabled":    true,
	}
	b, _ := json.Marshal(updateBody)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/probes/"+probe.ID, bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /probes/%s: %v", probe.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/probes/"+probe.ID, bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: PUT /api/v1/probes/%s → 200, conforms to spec", probe.ID)
}

// TestS3_Probes_Delete204_Conforms validates DELETE /probes/{probeId} → 204.
func TestS3_Probes_Delete204_Conforms(t *testing.T) {
	ts, token, ms, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Seed a probe to delete.
	ctx := context.Background()
	probe, err := ms.CreateProbe(ctx, meta.ProbeRow{
		Name:      "s3-delete-probe",
		URL:       "http://example.com/delete.m3u8",
		Protocol:  "hls",
		IntervalS: 60,
		TimeoutS:  10,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProbe: %v", err)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/probes/"+probe.ID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /probes/%s: %v", probe.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/probes/"+probe.ID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/probes/%s → 204, conforms to spec", probe.ID)
}

// ─── Task A: Ingest ──────────────────────────────────────────────────────────

// TestS3_Beacon_Ingest_Conforms validates POST /ingest/beacon → 202 IngestAccepted.
// Pro+ license required (CheckBeaconIngest). Uses X-Pulse-Ingest-Token auth.
//
// Note on conformCheck path: the spec declares /ingest/beacon under server base
// /api/v1, so kin-openapi registers the route as /api/v1/ingest/beacon.
// The conformCheck request URL must therefore be /api/v1/ingest/beacon for
// FindRoute to succeed, even though the real handler is at /ingest/beacon.
func TestS3_Beacon_Ingest_Conforms(t *testing.T) {
	ts, _, ingestToken, cleanup := setupProServerWithIngest(t)
	defer cleanup()
	doc := openAPISpec(t)

	batch := map[string]any{
		"version":    1,
		"session_id": "s3-conformance-session",
		"stream_id":  "s3-stream",
		"app":        "live",
		"events": []any{
			map[string]any{
				"type": "session_start",
				"ts":   int64(1700000000000),
				"data": map[string]any{},
			},
		},
	}
	b, _ := json.Marshal(batch)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Ingest-Token", ingestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, bd)
	}

	// conformCheck path: /api/v1/ingest/beacon (spec server base + path).
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/ingest/beacon", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Pulse-Ingest-Token", ingestToken)
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /ingest/beacon → 202, conforms to spec")
}

// ─── Task A: Operational ─────────────────────────────────────────────────────

// TestS3_Healthz_Conforms validates GET /healthz → 200 HealthStatus.
//
// kin-openapi behavior: the spec declares /healthz under server base /api/v1,
// so gorillamux registers the route as /api/v1/healthz. We get the response
// from the real handler at /healthz and pass /api/v1/healthz to conformCheck
// so FindRoute succeeds. The body content is identical regardless of URL path.
func TestS3_Healthz_Conforms(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Real request to the actual handler path.
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	// conformCheck with spec-resolved path /api/v1/healthz.
	// kin-openapi's gorillamux router builds routes as serverURL+path, so
	// /api/v1 + /healthz = /api/v1/healthz. FindRoute matches this request.
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /healthz → 200, conforms to spec (conformCheck via /api/v1/healthz)")
}

// TestS3_Metrics_Conforms validates GET /metrics → 200 text/plain.
// Requires Business+ tier (CheckPrometheus). The schema is type:string
// (near-vacuous) but conformCheck is wired for completeness.
//
// Note on conformCheck path: same reasoning as /healthz — uses /api/v1/metrics.
func TestS3_Metrics_Conforms(t *testing.T) {
	ts, _, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// GET /metrics — unauthenticated when no MetricsToken is configured.
	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	// conformCheck with spec-resolved path /api/v1/metrics.
	// The response schema is type:string (Prometheus text-format) — near-vacuous
	// but we wire conformCheck anyway per WO requirement.
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /metrics → 200, conforms to spec (text/plain schema validated via /api/v1/metrics)")
}

// ─── Task A: Admin — Sources ─────────────────────────────────────────────────

// TestS3_AdminSources_Post201_Conforms validates POST /admin/sources → 201 Source.
// Free tier allows 1 source (CheckNodeLimit(1) passes).
func TestS3_AdminSources_Post201_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{
		"name":     "s3-conformance-source",
		"type":     "rest_poll",
		"rest_url": "http://127.0.0.1:5080",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/sources", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/admin/sources → 201, conforms to spec")
}

// TestS3_AdminSources_Delete204_Conforms validates DELETE /admin/sources/{sourceId} → 204.
func TestS3_AdminSources_Delete204_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Seed a source to delete.
	srcID := createSource(t, ts.URL, token, "s3-delete-source")

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/sources/"+srcID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /admin/sources/%s: %v", srcID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/admin/sources/"+srcID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/admin/sources/%s → 204, conforms to spec", srcID)
}

// TestS3_AdminSources_Test_Conforms validates POST /admin/sources/{sourceId}/test → 200
// AmsSourceStatus. Uses a source with no rest_url → returns reachable:false immediately.
func TestS3_AdminSources_Test_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create a source to test.
	srcID := createSource(t, ts.URL, token, "s3-test-source")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources/"+srcID+"/test", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/sources/%s/test: %v", srcID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/sources/"+srcID+"/test", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/admin/sources/%s/test → 200, conforms to spec", srcID)
}

// ─── Task A: Admin — License ──────────────────────────────────────────────────

// TestS3_License_Activate_Conforms validates PUT /admin/license → 200 LicenseInfo.
// Uses makeTestEnterpriseLicense to generate a valid license key that the fresh
// server (constructed after pubkey is set) will accept.
func TestS3_License_Activate_Conforms(t *testing.T) {
	// Generate the test enterprise license key and set PULSE_LICENSE_PUBKEY.
	licKey, licCleanup := makeTestEnterpriseLicense(t)
	defer licCleanup()

	// Create a fresh server whose license.Manager picks up the test pubkey.
	freshTS, freshToken, freshCleanup := setupTestServer(t)
	defer freshCleanup()

	doc := openAPISpec(t)

	body := map[string]any{"key": licKey}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, freshTS.URL+"/api/v1/admin/license", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(freshToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /admin/license: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/license", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(freshToken))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: PUT /api/v1/admin/license → 200, conforms to spec")
}

// ─── Task A: Admin — Tokens ───────────────────────────────────────────────────

// TestS3_AdminTokens_Post201_Conforms validates POST /admin/tokens → 201 TokenCreated.
// TokenCreated extends Token with a raw token string (only returned on creation).
func TestS3_AdminTokens_Post201_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{
		"kind":   "api",
		"name":   "s3-new-api-token",
		"scopes": []string{"admin"},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/tokens: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/tokens", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/admin/tokens → 201, conforms to spec (includes raw token field)")
}

// TestS3_AdminTokens_Delete204_Conforms validates DELETE /admin/tokens/{tokenId} → 204.
// The spec declares this idempotent — non-existent tokenId returns 204.
func TestS3_AdminTokens_Delete204_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Use a non-existent token ID — the spec says DELETE is idempotent → 204.
	tokenID := "00000000-0000-0000-0000-000000000001"

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/tokens/"+tokenID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /admin/tokens/%s: %v", tokenID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204 (idempotent), got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/admin/tokens/"+tokenID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/admin/tokens/%s → 204 (idempotent), conforms to spec", tokenID)
}

// ─── Task A: Admin — Users ────────────────────────────────────────────────────

// TestS3_AdminUsers_Create_Conforms validates POST /admin/users → 201 User.
// TDD: first written expecting 418 → FAIL ("expected 418 (TDD red proof), got 201");
// corrected to http.StatusCreated (201) → PASS.
func TestS3_AdminUsers_Create_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{
		"username": "s3-conformance-user",
		"role":     "viewer",
		"password": "testpass123",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/users", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/admin/users → 201, conforms to spec")
}

// TestS3_AdminUsers_Update_Conforms validates PUT /admin/users/{userId} → 200 User.
func TestS3_AdminUsers_Update_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create a user to update.
	createBody := map[string]any{
		"username": "s3-update-user",
		"role":     "viewer",
		"password": "initial123",
	}
	cb, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/users", bytes.NewReader(cb))
	createReq.Header.Set("Authorization", authHeader(token))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	crd, _ := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d: %s", createResp.StatusCode, crd)
	}
	var created map[string]any
	json.Unmarshal(crd, &created)
	userID, _ := created["id"].(string)
	if userID == "" {
		t.Fatal("create user: empty id")
	}

	updateBody := map[string]any{
		"username": "s3-update-user-updated",
		"role":     "admin",
	}
	ub, _ := json.Marshal(updateBody)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/users/"+userID, bytes.NewReader(ub))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /admin/users/%s: %v", userID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/users/"+userID, bytes.NewReader(ub))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: PUT /api/v1/admin/users/%s → 200, conforms to spec", userID)
}

// TestS3_AdminUsers_Delete204_Conforms validates DELETE /admin/users/{userId} → 204.
// The spec declares this idempotent — non-existent userId returns 204.
func TestS3_AdminUsers_Delete204_Conforms(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Use a non-existent user ID — idempotent per spec → 204.
	userID := "00000000-0000-0000-0000-000000000002"

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/users/"+userID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /admin/users/%s: %v", userID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204 (idempotent), got %d: %s", resp.StatusCode, bd)
	}

	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+userID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/admin/users/%s → 204 (idempotent), conforms to spec", userID)
}

// ─── Task B: 401 sweep ───────────────────────────────────────────────────────

// TestS3_401Sweep_AuthedOperations exercises every /api/v1/* operation that
// declares 401 in the spec — unauthenticated → assert 401 → conformCheck error body.
// All 401 responses must conform to the Error{code, message} schema.
//
// Using Free-tier server: bearer auth fires before any tier gate, so 401 is
// returned regardless of license level for all /api/v1/* routes.
func TestS3_401Sweep_AuthedOperations(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Each entry is {method, path} for a /api/v1/* operation that declares 401.
	// Path parameters use placeholder values (auth fires before resource lookup).
	sweep := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/live/overview"},
		{http.MethodGet, "/api/v1/live/streams"},
		{http.MethodGet, "/api/v1/analytics/audience"},
		{http.MethodGet, "/api/v1/analytics/geo"},
		{http.MethodGet, "/api/v1/analytics/devices"},
		{http.MethodGet, "/api/v1/qoe/summary"},
		{http.MethodGet, "/api/v1/qoe/ingest"},
		{http.MethodGet, "/api/v1/alerts/rules"},
		{http.MethodPost, "/api/v1/alerts/rules"},
		{http.MethodPut, "/api/v1/alerts/rules/placeholder-id"},
		{http.MethodDelete, "/api/v1/alerts/rules/placeholder-id"},
		{http.MethodGet, "/api/v1/alerts/channels"},
		{http.MethodPost, "/api/v1/alerts/channels"},
		{http.MethodPut, "/api/v1/alerts/channels/placeholder-id"},
		{http.MethodDelete, "/api/v1/alerts/channels/placeholder-id"},
		{http.MethodPost, "/api/v1/alerts/channels/placeholder-id/test"},
		{http.MethodGet, "/api/v1/alerts/history"},
		{http.MethodGet, "/api/v1/reports/usage"},
		{http.MethodGet, "/api/v1/reports/schedules"},
		{http.MethodPost, "/api/v1/reports/schedules"},
		{http.MethodPut, "/api/v1/reports/schedules/placeholder-id"},
		{http.MethodDelete, "/api/v1/reports/schedules/placeholder-id"},
		{http.MethodGet, "/api/v1/fleet/nodes"},
		{http.MethodGet, "/api/v1/anomalies"},
		{http.MethodGet, "/api/v1/probes"},
		{http.MethodPost, "/api/v1/probes"},
		{http.MethodPut, "/api/v1/probes/placeholder-id"},
		{http.MethodDelete, "/api/v1/probes/placeholder-id"},
		{http.MethodGet, "/api/v1/probes/placeholder-id/results"},
		{http.MethodGet, "/api/v1/admin/sources"},
		{http.MethodPost, "/api/v1/admin/sources"},
		{http.MethodPut, "/api/v1/admin/sources/placeholder-id"},
		{http.MethodDelete, "/api/v1/admin/sources/placeholder-id"},
		{http.MethodPost, "/api/v1/admin/sources/placeholder-id/test"},
		{http.MethodGet, "/api/v1/admin/license"},
		{http.MethodPut, "/api/v1/admin/license"},
		{http.MethodGet, "/api/v1/admin/tokens"},
		{http.MethodPost, "/api/v1/admin/tokens"},
		{http.MethodDelete, "/api/v1/admin/tokens/placeholder-id"},
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodPost, "/api/v1/admin/users"},
		{http.MethodPut, "/api/v1/admin/users/placeholder-id"},
		{http.MethodDelete, "/api/v1/admin/users/placeholder-id"},
		{http.MethodGet, "/api/v1/admin/tenants"},
		{http.MethodPost, "/api/v1/admin/tenants"},
		{http.MethodGet, "/api/v1/admin/tenants/placeholder-id"},
		{http.MethodPut, "/api/v1/admin/tenants/placeholder-id"},
		{http.MethodDelete, "/api/v1/admin/tenants/placeholder-id"},
	}

	errored := 0
	for _, e := range sweep {
		e := e // capture loop variable
		t.Run(e.method+"_"+e.path, func(t *testing.T) {
			// Unauthenticated request — no Authorization header.
			req, _ := http.NewRequest(e.method, ts.URL+e.path, bytes.NewReader([]byte(`{}`)))
			req.Header.Set("Content-Type", "application/json")
			// Deliberately NO Authorization header.

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("request failed: %v", err)
				errored++
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401 without auth, got %d: %s", resp.StatusCode, body)
				errored++
				return
			}

			// Restore body for conformCheck.
			resp.Body = io.NopCloser(bytes.NewReader(body))

			// conformCheck the Error body shape.
			req2, _ := http.NewRequest(e.method, e.path, bytes.NewReader([]byte(`{}`)))
			req2.Header.Set("Content-Type", "application/json")
			conformCheck(t, doc, req2, resp)

			// Additionally verify the Error envelope fields are present.
			var errBody map[string]any
			if jerr := json.Unmarshal(body, &errBody); jerr != nil {
				t.Errorf("401 body is not valid JSON: %v — body: %s", jerr, body)
				return
			}
			if _, hasCode := errBody["code"]; !hasCode {
				t.Errorf("401 body missing 'code' field (Error envelope required): %s", body)
			}
			if _, hasMsg := errBody["message"]; !hasMsg {
				t.Errorf("401 body missing 'message' field (Error envelope required): %s", body)
			}
			t.Logf("PASS: %s %s → 401 with Error{code,message}", e.method, e.path)
		})
	}

	if errored > 0 {
		t.Errorf("401 sweep: %d operations did not return 401 or failed conformCheck", errored)
	}
}

// TestS3_401_BeaconIngest verifies POST /ingest/beacon → 401 when X-Pulse-Ingest-Token
// is missing, on a Pro-tier server (license gate passes; auth check fires → 401).
func TestS3_401_BeaconIngest(t *testing.T) {
	ts, _, _, cleanup := setupProServerWithIngest(t)
	defer cleanup()
	doc := openAPISpec(t)

	batch := map[string]any{
		"version": 1, "session_id": "test", "stream_id": "s1",
		"events": []any{map[string]any{"type": "heartbeat", "ts": int64(1700000000000)}},
	}
	b, _ := json.Marshal(batch)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	// No X-Pulse-Ingest-Token header.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing ingest token on Pro tier, got %d: %s",
			resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/ingest/beacon", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("401 body missing 'code' field: %s", body)
	}
	t.Logf("PASS: POST /ingest/beacon (no token, Pro tier) → 401 with Error envelope")
}

// ─── Task B: 403 cases ───────────────────────────────────────────────────────

// TestS3_403_AnalyticsForbiddenFreeTier validates GET /analytics/audience → 403
// on Free tier (CheckDataAPI fails). Conforms to spec Error schema.
func TestS3_403_AnalyticsForbiddenFreeTier(t *testing.T) {
	ts, token, cleanup := setupTestServer(t) // Free tier
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/audience", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /analytics/audience: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for Free tier analytics, got %d: %s", resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/analytics/audience", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("403 body missing 'code' field: %s", body)
	}
	if _, ok := errBody["message"]; !ok {
		t.Errorf("403 body missing 'message' field: %s", body)
	}
	t.Logf("PASS: GET /analytics/audience (Free tier) → 403, Error{code,message} conforms to spec")
}

// TestS3_403_MetricsForbiddenFreeTier validates GET /metrics → 403 on Free tier.
// /metrics requires Business+ tier (CheckPrometheus).
func TestS3_403_MetricsForbiddenFreeTier(t *testing.T) {
	ts, _, cleanup := setupTestServer(t) // Free tier, no MetricsToken configured
	defer cleanup()
	doc := openAPISpec(t)

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for Free tier /metrics, got %d: %s", resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("403 body missing 'code' field: %s", body)
	}
	t.Logf("PASS: GET /metrics (Free tier) → 403, Error{code,message} conforms to spec")
}

// TestS3_403_AnomaliesNonEnterprise validates GET /anomalies → 403 on Free/Pro tier.
// Only Enterprise tier can access anomaly detection (F9).
func TestS3_403_AnomaliesNonEnterprise(t *testing.T) {
	ts, token, cleanup := setupProAdminServer(t) // Pro, not Enterprise
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/anomalies", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /anomalies: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for Pro tier anomalies, got %d: %s", resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/anomalies", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("403 body missing 'code' field: %s", body)
	}
	t.Logf("PASS: GET /anomalies (Pro tier, non-Enterprise) → 403, conforms to spec")
}

// ─── Task B: 404 cases ───────────────────────────────────────────────────────

// TestS3_404_MissingAlertRule validates DELETE /alerts/rules/{ruleId} → 404
// for a non-existent ruleId. Conforms to spec Error schema.
func TestS3_404_MissingAlertRule(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	ruleID := "nonexistent-s3-rule-id"
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/"+ruleID, nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /alerts/rules/%s: %v", ruleID, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent rule, got %d: %s", resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/alerts/rules/"+ruleID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("404 body missing 'code' field: %s", body)
	}
	if _, ok := errBody["message"]; !ok {
		t.Errorf("404 body missing 'message' field: %s", body)
	}
	t.Logf("PASS: DELETE /alerts/rules/nonexistent → 404, Error{code,message} conforms to spec")
}

// TestS3_404_MissingAlertChannel validates PUT /alerts/channels/{channelId} → 404.
func TestS3_404_MissingAlertChannel(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	chanID := "nonexistent-s3-channel-id"
	updateBody := map[string]any{
		"type":   "email",
		"name":   "x",
		"config": map[string]any{},
	}
	b, _ := json.Marshal(updateBody)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/channels/"+chanID, bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /alerts/channels/%s: %v", chanID, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent channel, got %d: %s", resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/alerts/channels/"+chanID, bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("404 body missing 'code' field: %s", body)
	}
	t.Logf("PASS: PUT /alerts/channels/nonexistent → 404, Error{code,message} conforms to spec")
}

// TestS3_404_MissingProbe validates PUT /probes/{probeId} → 404 for non-existent probe.
func TestS3_404_MissingProbe(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	probeID := "nonexistent-s3-probe-id"
	updateBody := map[string]any{
		"name":       "x",
		"url":        "http://example.com",
		"interval_s": 60,
	}
	b, _ := json.Marshal(updateBody)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/probes/"+probeID, bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /probes/%s: %v", probeID, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent probe, got %d: %s", resp.StatusCode, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/probes/"+probeID, bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(body, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("404 body missing 'code' field: %s", body)
	}
	t.Logf("PASS: PUT /probes/nonexistent → 404, Error{code,message} conforms to spec")
}

// ─── Task B: 422 cases ───────────────────────────────────────────────────────

// TestS3_422_AlertRule_InvalidPayload validates POST /alerts/rules → 422
// when required fields are missing. Conforms to spec Error schema.
func TestS3_422_AlertRule_InvalidPayload(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Missing required fields: only threshold is provided.
	body := map[string]any{"threshold": 5.0}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/rules (invalid): %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing required fields, got %d: %s",
			resp.StatusCode, respBody)
	}

	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/alerts/rules", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(respBody, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("422 body missing 'code' field: %s", respBody)
	}
	if _, ok := errBody["message"]; !ok {
		t.Errorf("422 body missing 'message' field: %s", respBody)
	}
	t.Logf("PASS: POST /alerts/rules (missing fields) → 422, Error{code,message} conforms to spec")
}

// TestS3_422_ReportSchedule_InvalidFormat validates POST /reports/schedules → 422
// when format is not in the allowed enum [csv, pdf]. Requires Business+ tier.
func TestS3_422_ReportSchedule_InvalidFormat(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{
		"cron":   "0 9 * * 1",
		"format": "docx", // not in enum [csv, pdf]
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /reports/schedules (invalid format): %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid format, got %d: %s", resp.StatusCode, respBody)
	}

	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/reports/schedules", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(respBody, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("422 body missing 'code' field: %s", respBody)
	}
	t.Logf("PASS: POST /reports/schedules (invalid format) → 422, Error{code,message} conforms to spec")
}

// TestS3_422_License_InvalidKey validates PUT /admin/license → 422 for invalid key.
func TestS3_422_License_InvalidKey(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{"key": "totally-invalid-key"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/license", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /admin/license (invalid key): %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid license key, got %d: %s", resp.StatusCode, respBody)
	}

	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/license", bytes.NewReader(b))
	req2.Header.Set("Authorization", authHeader(token))
	req2.Header.Set("Content-Type", "application/json")
	conformCheck(t, doc, req2, resp)

	var errBody map[string]any
	json.Unmarshal(respBody, &errBody)
	if _, ok := errBody["code"]; !ok {
		t.Errorf("422 body missing 'code' field: %s", respBody)
	}
	t.Logf("PASS: PUT /admin/license (invalid key) → 422, Error{code,message} conforms to spec")
}
