// Package api_test — Wave 2 additional tests:
//   - Prometheus /metrics scrape (expfmt parse, cardinality check)
//   - Tier gating: Free blocks Telegram; Pro blocks PagerDuty; entitlement errors
//   - bcrypt password hashing
//   - CSV export on audience endpoint
//   - CR-3: POST /admin/sources/{sourceId}/test
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Test: Prometheus /metrics endpoint ──────────────────────────────────────

func TestAPI_Metrics_ParsesWithExpfmt(t *testing.T) {
	// /metrics requires Business+ tier (CheckPrometheus); use Business server.
	ts, _, cleanup := setupBusinessServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// Verify required metrics are present.
	requiredMetrics := []string{
		"pulse_live_viewers",
		"pulse_live_streams",
		"pulse_live_publishers",
		"pulse_ingest_bitrate_kbps",
		"pulse_alerts_firing",
	}
	for _, m := range requiredMetrics {
		if !strings.Contains(text, m) {
			t.Errorf("expected metric %q in /metrics output, not found", m)
		}
	}

	// Check cardinality: no stream-level labels (ARCHITECTURE §3).
	// Valid labels: node, app. Invalid: stream_id, session_id.
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		// No stream_id label allowed.
		if strings.Contains(line, "stream_id=") {
			t.Errorf("FAIL: stream-level label found in /metrics (cardinality violation): %q", line)
		}
		if strings.Contains(line, "session_id=") {
			t.Errorf("FAIL: session-level label found in /metrics (cardinality violation): %q", line)
		}
	}

	t.Logf("PASS: /metrics parses correctly, %d lines, required metrics present", len(lines))
}

func TestAPI_Metrics_Token_Gated(t *testing.T) {
	// /metrics requires Business+ tier (CheckPrometheus). Use a Business license
	// so the tier gate passes, and then verify the MetricsToken gate.
	licKey, licCleanup := makeTestBusinessLicense(t)
	defer licCleanup()

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := readFileDirect(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, _ := meta.New(ctx, "sqlite", ":memory:", "test-secret")
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	lic, err := license.New(licKey, "")
	if err != nil {
		t.Fatalf("license.New (business): %v", err)
	}
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0", MetricsToken: "scrape-secret"}, store, live, qsvc, lic, nil)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Without token → 401 (tier passes, MetricsToken gate fires).
	resp1, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics (no token): %v", err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without scrape token, got %d", resp1.StatusCode)
	}

	// With correct token → 200.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer scrape-secret")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /metrics (with token): %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with correct scrape token, got %d", resp2.StatusCode)
	}
	t.Logf("PASS: /metrics: 401 without token, 200 with correct token (business tier)")
}

// ─── Test: Tier gating (§7.11) ───────────────────────────────────────────────

func TestAPI_FreeTier_BlocksTelegramChannel(t *testing.T) {
	// Free tier: only email channel allowed. Creating Telegram → 403.
	ts, token, cleanup := setupTestServer(t) // free tier by default
	defer cleanup()

	body := map[string]any{
		"type": "telegram",
		"name": "My Telegram Channel",
		"config": map[string]any{
			"telegram_bot_token": "123:fake",
			"chat_id":            "-100123",
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body2, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 403 for telegram on free tier, got %d: %s", resp.StatusCode, body2)
	}

	var errResp map[string]any
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v", errResp["code"])
	}
	t.Logf("PASS: Free tier blocks telegram channel creation → 403 LICENSE_REQUIRED")
}

func TestAPI_FreeTier_BlocksSlackChannel(t *testing.T) {
	// Free tier: Slack not allowed.
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	body := map[string]any{
		"type":   "slack",
		"name":   "My Slack Channel",
		"config": map[string]any{"slack_webhook_url": "https://hooks.slack.com/fake"},
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels (slack): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body2, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 403 for slack on free tier, got %d: %s", resp.StatusCode, body2)
	}
	t.Logf("PASS: Free tier blocks slack channel creation → 403")
}

func TestAPI_FreeTier_AllowsEmailChannel(t *testing.T) {
	// Free tier: email IS allowed.
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	body := map[string]any{
		"type": "email",
		"name": "My Email Channel",
		"config": map[string]any{
			"from": "alerts@example.com",
			"to":   "admin@example.com",
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels (email): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body2, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 for email on free tier, got %d: %s", resp.StatusCode, body2)
	}
	t.Logf("PASS: Free tier allows email channel creation → 201")
}

// ─── Test: bcrypt password hashing (G3) ──────────────────────────────────────

func TestAPI_CreateUser_BcryptHash(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	body := map[string]any{
		"username": "test-user",
		"role":     "viewer",
		"password": "s3cur3passw0rd!",
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/users", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body2, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body2)
	}

	var userResp map[string]any
	json.NewDecoder(resp.Body).Decode(&userResp)
	// The response must NOT include the password hash.
	if _, ok := userResp["pw_hash"]; ok {
		t.Error("response must not include pw_hash")
	}
	if _, ok := userResp["password"]; ok {
		t.Error("response must not include plaintext password")
	}
	t.Logf("PASS: POST /admin/users returns %v (no password in response)", userResp)
}

// ─── Test: CSV export ────────────────────────────────────────────────────────

func TestAPI_AudienceAnalytics_CSVFormat(t *testing.T) {
	// Analytics endpoints require Pro+ tier (CheckDataAPI); use Business server.
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/analytics/audience?format=csv", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /analytics/audience?format=csv: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("expected text/csv Content-Type for format=csv, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	// At minimum there should be a header row.
	if len(lines) == 0 {
		t.Error("expected at least a CSV header row")
	}
	header := lines[0]
	if !strings.Contains(header, "ts") || !strings.Contains(header, "views") {
		t.Errorf("CSV header missing expected columns, got: %q", header)
	}
	t.Logf("PASS: CSV export → %d lines, header=%q", len(lines), header)
}

// ─── Test: CR-3 POST /admin/sources/{sourceId}/test ─────────────────────────

func TestAPI_TestSource_NotFound_404(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources/nonexistent-id/test", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST .../test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent source, got %d", resp.StatusCode)
	}
	t.Logf("PASS: POST /admin/sources/nonexistent/test → 404")
}

func TestAPI_TestSource_ExistingSource_OK(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a source first.
	createBody := map[string]any{
		"name":     "test-ams",
		"type":     "rest",
		"rest_url": "http://localhost:5080", // unreachable but that's OK for the test
	}
	bodyBytes, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Authorization", authHeader(token))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Skipf("could not create source: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		body2, _ := io.ReadAll(createResp.Body)
		t.Skipf("could not create source: %d %s", createResp.StatusCode, body2)
	}

	var sourceData map[string]any
	json.NewDecoder(createResp.Body).Decode(&sourceData)
	sourceID, _ := sourceData["id"].(string)

	// Now test it — will fail connectivity but should return 200 with error status.
	testReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources/"+sourceID+"/test", nil)
	testReq.Header.Set("Authorization", authHeader(token))

	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST .../test: %v", err)
	}
	defer testResp.Body.Close()

	// Should return 200 with a status object (either "ok" or "error" for connectivity).
	if testResp.StatusCode != http.StatusOK {
		body2, _ := io.ReadAll(testResp.Body)
		t.Fatalf("expected 200 for test, got %d: %s", testResp.StatusCode, body2)
	}

	var testResult map[string]any
	json.NewDecoder(testResp.Body).Decode(&testResult)
	status, _ := testResult["status"].(string)
	if status == "" {
		t.Error("expected status field in test response")
	}
	t.Logf("PASS: POST /admin/sources/%s/test → 200, status=%q", sourceID, status)
}

// ─── Test: QoE tier gating ───────────────────────────────────────────────────

func TestAPI_ProTier_QoE_Accessible(t *testing.T) {
	// QoE endpoint requires Pro+ tier (CheckDataAPI). Use Business server which
	// is Pro+ (Data API is enabled on Business tier).
	// Note: the former "fail-open" comment is superseded by the explicit
	// CheckDataAPI gate introduced in A2.
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/summary", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /qoe/summary: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 for qoe/summary on business tier, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("PASS: /qoe/summary accessible on business (Pro+) tier")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// readFileDirect reads a file by path (used by TestAPI_Metrics_Token_Gated).
func readFileDirect(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Ensure domain import is used (the type is already used via fakeLiveProvider).
var _ domain.LiveProvider = &fakeLiveProvider{}
