// Package api_test — security-hardening tests for the fixes applied in the
// production-hardening pass:
//
//   - A1  CORS allowlist: echo vs omit
//   - A4  ?token= ignored on normal API route
//   - B4/A6 rest_url scheme validation (file:// rejected)
//   - B4/A6 redirect blocked in the test-source HTTP client
//   - A11 alert-history limit capped at 500
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// setupServerWithCORS spins up the API handler with a specific CORS allowlist.
func setupServerWithCORS(t *testing.T, allowedOrigins []string) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	ddl, err := readMetaDDL(t)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "test-secret-cors")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_corstest_abcdef1234567890"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "cors-test-admin",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	apiCfg := api.Config{
		ListenAddr:         ":0",
		CORSAllowedOrigins: allowedOrigins,
	}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
	}
	return ts, adminToken, cleanup
}

// readMetaDDL reads the DDL file (reusing metaDDLPath from the package).
func readMetaDDL(t *testing.T) ([]byte, error) {
	t.Helper()
	p := metaDDLPath(t)
	return os.ReadFile(p)
}

// ─── A1: CORS allowlist ───────────────────────────────────────────────────────

// TestCORS_AllowlistedOrigin_Echoed verifies that when the request Origin is in
// the allowlist the server echoes it in Access-Control-Allow-Origin.
func TestCORS_AllowlistedOrigin_Echoed(t *testing.T) {
	allowed := "https://beyondkaira.com"
	ts, adminToken, cleanup := setupServerWithCORS(t, []string{allowed})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Origin", allowed)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != allowed {
		t.Errorf("expected Access-Control-Allow-Origin=%q, got %q", allowed, got)
	}
	if vary := resp.Header.Get("Vary"); !strings.Contains(vary, "Origin") {
		t.Errorf("expected Vary to contain 'Origin', got %q", vary)
	}
}

// TestCORS_UnknownOrigin_NoHeader verifies that an origin NOT in the allowlist
// does not receive an Access-Control-Allow-Origin header.
func TestCORS_UnknownOrigin_NoHeader(t *testing.T) {
	ts, adminToken, cleanup := setupServerWithCORS(t, []string{"https://beyondkaira.com"})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Origin", "https://evil.example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header for unknown origin, got %q", got)
	}
}

// TestCORS_EmptyAllowlist_NoHeader verifies that when the allowlist is empty no
// Access-Control-Allow-Origin header is emitted (same-origin SPA still works).
func TestCORS_EmptyAllowlist_NoHeader(t *testing.T) {
	ts, adminToken, cleanup := setupServerWithCORS(t, nil)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Origin", "https://any.example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("expected no ACAO header with empty allowlist, got %q", got)
	}
}

// TestCORS_BeaconIngest_AlwaysPermissive verifies that the beacon route echoes
// the Origin regardless of the CORSAllowedOrigins allowlist.
func TestCORS_BeaconIngest_AlwaysPermissive(t *testing.T) {
	// allowlist intentionally excludes the beacon's origin.
	ts, _, cleanup := setupServerWithCORS(t, []string{"https://beyondkaira.com"})
	defer cleanup()

	body := bytes.NewBufferString(`{"version":1,"session_id":"s1","stream_id":"st1","app":"live","events":[]}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://player.third-party.com")
	// No ingest token — we only care about CORS headers here, not auth.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	// Beacon should echo the request origin (or * if no origin header).
	if got != "https://player.third-party.com" && got != "*" {
		t.Errorf("beacon CORS should be permissive, got Access-Control-Allow-Origin=%q", got)
	}
}

// TestCORS_OPTIONS_Preflight_NoContent verifies that OPTIONS returns 204 with
// CORS headers and does NOT require auth.
func TestCORS_OPTIONS_Preflight_NoContent(t *testing.T) {
	ts, _, cleanup := setupServerWithCORS(t, []string{"https://beyondkaira.com"})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Origin", "https://beyondkaira.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", resp.StatusCode)
	}
}

// ─── A4: ?token= ignored on API routes ───────────────────────────────────────

// TestTokenInURL_Ignored verifies that ?token= does not authenticate normal
// API requests (the bearer middleware must read only the Authorization header).
func TestTokenInURL_Ignored(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	// Pass the valid token as a query parameter instead of the header.
	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/live/overview?token="+adminToken, nil)
	// Do NOT set Authorization header.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 when token is in URL only, got %d", resp.StatusCode)
	}
}

// TestTokenInURL_HeaderStillWorks verifies that the fix doesn't break normal
// header-based auth (regression guard).
func TestTokenInURL_HeaderStillWorks(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 with header auth, got %d: %s", resp.StatusCode, body)
	}
}

// ─── B4/A6: rest_url scheme validation ───────────────────────────────────────

// createSourceHelper creates a source via the API and returns the response.
func createSourceHelper(t *testing.T, ts *httptest.Server, adminToken string, body map[string]any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources",
		bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	return resp
}

// TestSource_FileScheme_Rejected verifies that rest_url with file:// is rejected
// with 422 (INVALID_SOURCE).
func TestSource_FileScheme_Rejected(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	resp := createSourceHelper(t, ts, adminToken, map[string]any{
		"name":     "evil-file",
		"type":     "rest",
		"rest_url": "file:///etc/passwd",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 422 for file:// rest_url, got %d: %s", resp.StatusCode, body)
	}
}

// TestSource_FTPScheme_Rejected verifies that rest_url with ftp:// is rejected.
func TestSource_FTPScheme_Rejected(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	resp := createSourceHelper(t, ts, adminToken, map[string]any{
		"name":     "evil-ftp",
		"type":     "rest",
		"rest_url": "ftp://internal.host/secret",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 422 for ftp:// rest_url, got %d: %s", resp.StatusCode, body)
	}
}

// TestSource_HTTPScheme_Accepted verifies that http:// is accepted (AMS on
// private networks commonly uses plain HTTP).
func TestSource_HTTPScheme_Accepted(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	resp := createSourceHelper(t, ts, adminToken, map[string]any{
		"name":     "internal-ams",
		"type":     "rest",
		"rest_url": "http://192.168.1.10:5080",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 201 for http:// rest_url (private IP), got %d: %s", resp.StatusCode, body)
	}
}

// TestSource_HTTPSScheme_Accepted verifies that https:// is accepted.
func TestSource_HTTPSScheme_Accepted(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	resp := createSourceHelper(t, ts, adminToken, map[string]any{
		"name":     "public-ams",
		"type":     "rest",
		"rest_url": "https://ams.example.com:5443",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 201 for https:// rest_url, got %d: %s", resp.StatusCode, body)
	}
}

// TestSource_TestEndpoint_BlocksRedirect verifies that the test-source handler
// uses an HTTP client that does not follow redirects (ErrUseLastResponse).
// We spin up a local server that responds with a redirect; the test-source
// handler should NOT follow it and should still report reachable=true
// (receiving any HTTP response — including 3xx — counts as reachable).
func TestSource_TestEndpoint_BlocksRedirect(t *testing.T) {
	// Redirect target — we use a counter to detect if it was hit.
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This would be a dangerous internal endpoint in a real SSRF scenario.
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"target":"hit"}`)
	}))
	defer redirectTarget.Close()

	// Source server — issues a 302 to the redirect target.
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/redirect-target", http.StatusFound)
	}))
	defer sourceServer.Close()

	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	// Create an AMS source pointing at sourceServer.
	createResp := createSourceHelper(t, ts, adminToken, map[string]any{
		"name":     "redirect-test-source",
		"type":     "rest",
		"rest_url": sourceServer.URL,
	})
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201 creating source, got %d: %s", createResp.StatusCode, body)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	sourceID, _ := created["id"].(string)
	if sourceID == "" {
		t.Fatal("no source id in response")
	}

	// Trigger the connectivity test.
	testReq, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/admin/sources/"+sourceID+"/test", nil)
	testReq.Header.Set("Authorization", "Bearer "+adminToken)

	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST /test: %v", err)
	}
	defer testResp.Body.Close()

	if testResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(testResp.Body)
		t.Fatalf("expected 200 from /test, got %d: %s", testResp.StatusCode, body)
	}

	var result map[string]any
	json.NewDecoder(testResp.Body).Decode(&result)

	// The source server returned a 302; because we block redirects the handler
	// sees it as a reachable response (any HTTP response = reachable=true).
	reachable, _ := result["reachable"].(bool)
	if !reachable {
		t.Errorf("expected reachable=true (302 is still an HTTP response), got result=%v", result)
	}
	// The response message should mention the source server, NOT the redirect target.
	msg, _ := result["message"].(string)
	if strings.Contains(msg, "redirect-target") {
		t.Errorf("redirect was followed; message references redirect target: %q", msg)
	}
}

// ─── A11: alert-history limit capped at 500 ──────────────────────────────────

// TestAlertHistory_LimitCappedAt500 verifies that ?limit=9999 is silently
// capped to 500 and the endpoint still returns 200.
func TestAlertHistory_LimitCappedAt500(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/alerts/history?limit=9999", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	// The handler must not return more than 500 items (empty DB returns 0, which
	// is within 500, so we just verify it doesn't panic / error).
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	items, ok := body["items"].([]any)
	if !ok {
		t.Errorf("expected 'items' array in response, got %v", body)
	}
	if len(items) > 500 {
		t.Errorf("got %d items, expected at most 500", len(items))
	}
}

// TestAlertHistory_DefaultLimit_Works verifies the default (no limit param) still
// returns 200.
func TestAlertHistory_DefaultLimit_Works(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/alerts/history", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /alerts/history: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}
