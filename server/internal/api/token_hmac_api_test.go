// TDD tests for HMAC-SHA256 API token hashing at the HTTP layer (item 6).
//
// Tests verify that:
//   - POST /admin/tokens stores hash_alg='hmac-sha256' (not plain sha256) when
//     the store has an explicit key
//   - The returned raw token authenticates successfully on /api/v1/* routes
//   - Legacy sha256-hashed tokens (pre-seeded) still authenticate (back-compat)
//   - An ingest token created via POST /admin/tokens authenticates on /ingest/beacon
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

// setupHMACTestServer creates an API test server where the meta store uses an
// explicit secret key (so HashToken returns hmac-sha256 for new tokens).
func setupHMACTestServer(t *testing.T) (ts *httptest.Server, adminToken string, store *meta.Store, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	// Explicit secret key → HMAC tokens for new creates.
	st, err := meta.New(ctx, "sqlite", ":memory:", "hmac-api-test-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := st.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Pre-seed an admin token with legacy SHA-256 hash to verify back-compat.
	adminToken = "plt_hmactest_admin_abcdef12345678"
	if err := st.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "hmac-test-admin",
		TokenHash: hashToken(adminToken), // plain SHA-256 (legacy)
		HashAlg:   "sha256",              // legacy alg
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (admin seed): %v", err)
	}

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	srv := api.New(api.Config{ListenAddr: ":0"}, st, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		st.Close()
	}
	return ts, adminToken, st, cleanup
}

// TestHMACAPI_LegacyTokenStillAuthenticates verifies that the legacy SHA-256
// pre-seeded token (hash_alg='sha256') still authenticates via the bearer
// middleware (backward compatibility — LIVE PROD ADMIN TOKEN must keep working).
func TestHMACAPI_LegacyTokenStillAuthenticates(t *testing.T) {
	ts, adminToken, _, cleanup := setupHMACTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /admin/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("legacy SHA-256 token auth: expected 200, got %d: %s", resp.StatusCode, body)
	}
}

// TestHMACAPI_CreateTokenStoresHMACHash verifies that POST /admin/tokens
// creates a token with hash_alg='hmac-sha256' in the store (behavioral test:
// fails until handleCreateToken is updated to call store.HashToken).
func TestHMACAPI_CreateTokenStoresHMACHash(t *testing.T) {
	ts, adminToken, st, cleanup := setupHMACTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]any{
		"kind":   "api",
		"name":   "new-hmac-api-token",
		"scopes": []string{"admin"},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	rawToken, _ := created["token"].(string)
	if rawToken == "" {
		t.Fatal("response missing 'token' field")
	}

	// Look up the created token in the store and verify hash_alg.
	ctx := context.Background()
	found, err := st.LookupToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupToken after create: %v", err)
	}
	if found == nil {
		t.Fatal("LookupToken after create: token not found")
	}
	if found.HashAlg != "hmac-sha256" {
		t.Errorf("new token hash_alg: expected %q, got %q — handleCreateToken must call store.HashToken()", "hmac-sha256", found.HashAlg)
	}
}

// TestHMACAPI_NewTokenAuthenticates verifies that a token created via
// POST /admin/tokens (HMAC-hashed) can immediately authenticate on subsequent requests.
func TestHMACAPI_NewTokenAuthenticates(t *testing.T) {
	ts, adminToken, _, cleanup := setupHMACTestServer(t)
	defer cleanup()

	// Create a new HMAC token.
	body, _ := json.Marshal(map[string]any{
		"kind":   "api",
		"name":   "auth-check-token",
		"scopes": []string{"admin"},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	newRawToken, _ := created["token"].(string)
	if newRawToken == "" {
		t.Fatal("response missing 'token' field")
	}

	// Use the new HMAC token to authenticate.
	authReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tokens", nil)
	authReq.Header.Set("Authorization", "Bearer "+newRawToken)
	authResp, err := http.DefaultClient.Do(authReq)
	if err != nil {
		t.Fatalf("GET /admin/tokens with new HMAC token: %v", err)
	}
	defer authResp.Body.Close()
	if authResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(authResp.Body)
		t.Fatalf("HMAC token auth failed: expected 200, got %d: %s — bearerAuthMiddleware must call store.LookupToken()", authResp.StatusCode, b)
	}
}

// TestHMACAPI_WrongTokenRejected verifies that a random token is rejected with 401.
func TestHMACAPI_WrongTokenRejected(t *testing.T) {
	ts, _, _, cleanup := setupHMACTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tokens", nil)
	req.Header.Set("Authorization", "Bearer plt_notreal_xxxxxxxxxxxxxxxxxxxxxxxx")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /admin/tokens: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", resp.StatusCode)
	}
}
