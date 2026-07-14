// Package api_test — role-enforcement tests for requireWriteScope.
//
// Confirmed privilege-escalation path: a "viewer" OIDC token had full write
// access because bearerAuthMiddleware never inspected tok.Scopes. The fix adds
// requireWriteScope after bearerAuthMiddleware on the /api/v1 group.
//
// Test matrix:
//  1. viewer + POST /api/v1/alerts/rules              → 403
//  2. viewer + DELETE /api/v1/admin/tokens/{id}       → 403
//  3. viewer + POST /api/v1/admin/tokens              → 403 (privilege-escalation path)
//  4. viewer + GET  /api/v1/live/overview             → not 403 (viewers can read)
//  5. read   + POST /api/v1/admin/tokens              → 403 (the REAL escalation path — see below)
//  6. read   + GET  /api/v1/live/overview             → not 403
//  7. legacy (nil Scopes) + POST /api/v1/admin/tokens → not 403 (lockout-prevention guarantee)
//  8. admin  + POST /api/v1/admin/tokens              → not 403
//
// Cases 5 and 6 exist because the first cut of requireWriteScope denied only the
// scope string "viewer" — while the Settings UI mints its tokens with scope
// "read". Every token a real deployment could produce sailed through, and the
// suite was green. Enforce on what the product actually issues, not on the role
// name the design doc happens to use.
package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// setupAuthzServer spins up the API handler with one pre-seeded token per scope
// class so authz tests can exercise them all against a single server.
func setupAuthzServer(t *testing.T) (
	ts *httptest.Server,
	viewerTok, readTok, legacyTok, adminTok string,
	cleanup func(),
) {
	t.Helper()
	ctx := context.Background()

	ddl, err := readMetaDDL(t)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "authz-test-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	viewerTok = "plt_viewertoken_authz1234567890"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "authz-viewer",
		TokenHash: hashToken(viewerTok),
		Scopes:    []string{"viewer"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken viewer: %v", err)
	}

	// readTok carries scope "read" — what the Settings UI actually mints
	// (SettingsPage.tsx createApiToken). The first cut of requireWriteScope only
	// blocked "viewer", so this token — the only kind the UI could produce — kept
	// full write access and could mint itself an admin token. Nothing here may
	// pass unless a "read" token is denied writes.
	readTok = "plt_readtoken_authz12345678901"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "authz-read",
		TokenHash: hashToken(readTok),
		Scopes:    []string{"read"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken read: %v", err)
	}

	// legacyTok has nil Scopes, matching tokens minted via the API before
	// requireWriteScope was introduced.
	legacyTok = "plt_legacytoken_authz1234567890"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "authz-legacy",
		TokenHash: hashToken(legacyTok),
		Scopes:    nil,
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken legacy: %v", err)
	}

	adminTok = "plt_admintoken_authz1234567890x"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "authz-admin",
		TokenHash: hashToken(adminTok),
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken admin: %v", err)
	}

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
	}
	return ts, viewerTok, readTok, legacyTok, adminTok, cleanup
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestAuthz_Viewer_PostAlertsRules_Forbidden verifies that a viewer token
// cannot create alert rules (POST is a mutating method).
func TestAuthz_Viewer_PostAlertsRules_Forbidden(t *testing.T) {
	ts, viewerTok, _, _, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules",
		bytes.NewBufferString(`{"name":"x","metric":"bitrate","condition":{"op":"gt","threshold":1},"severity":"warning"}`))
	req.Header.Set("Authorization", "Bearer "+viewerTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer POST /api/v1/alerts/rules: want 403, got %d", resp.StatusCode)
	}
}

// TestAuthz_Viewer_DeleteAdminToken_Forbidden verifies that a viewer token
// cannot delete API tokens.
func TestAuthz_Viewer_DeleteAdminToken_Forbidden(t *testing.T) {
	ts, viewerTok, _, _, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/tokens/some-token-id", nil)
	req.Header.Set("Authorization", "Bearer "+viewerTok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer DELETE /api/v1/admin/tokens/{id}: want 403, got %d", resp.StatusCode)
	}
}

// TestAuthz_Viewer_MintAdminToken_Forbidden_PrivilegeEscalationPath is the
// key privilege-escalation regression test: a viewer token must not be able
// to call POST /api/v1/admin/tokens and mint a permanent admin token for itself.
func TestAuthz_Viewer_MintAdminToken_Forbidden_PrivilegeEscalationPath(t *testing.T) {
	ts, viewerTok, _, _, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewBufferString(`{"kind":"api","name":"evil-token","scopes":["admin"]}`))
	req.Header.Set("Authorization", "Bearer "+viewerTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer POST /api/v1/admin/tokens (privilege-escalation path): want 403, got %d", resp.StatusCode)
	}
}

// TestAuthz_Viewer_GetLiveOverview_Allowed verifies that a viewer token can
// still read data — read-only access must not be broken.
func TestAuthz_Viewer_GetLiveOverview_Allowed(t *testing.T) {
	ts, viewerTok, _, _, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer "+viewerTok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("viewer GET /api/v1/live/overview: got 403, viewers must be allowed to read")
	}
}

// TestAuthz_ReadScope_MintAdminToken_Forbidden_PrivilegeEscalationPath is the
// test the first cut of this feature lacked, and the reason it shipped useless.
// "read" — not "viewer" — is the scope the Settings UI mints, so this is the
// privilege-escalation path an actual deployment exposes: a read-only token
// calling POST /api/v1/admin/tokens to mint itself a permanent admin token.
func TestAuthz_ReadScope_MintAdminToken_Forbidden_PrivilegeEscalationPath(t *testing.T) {
	ts, _, readTok, _, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewBufferString(`{"kind":"api","name":"escalated","scopes":["admin"]}`))
	req.Header.Set("Authorization", "Bearer "+readTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("read-scope POST /api/v1/admin/tokens (privilege-escalation path): want 403, got %d", resp.StatusCode)
	}
}

// TestAuthz_ReadScope_GetLiveOverview_Allowed verifies the other half of the
// contract: a read token is read-only, not useless.
func TestAuthz_ReadScope_GetLiveOverview_Allowed(t *testing.T) {
	ts, _, readTok, _, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/live/overview", nil)
	req.Header.Set("Authorization", "Bearer "+readTok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("read-scope GET /api/v1/live/overview: got 403, read tokens must be allowed to read")
	}
}

// TestAuthz_LegacyEmptyScope_PostAllowed_LockoutPrevention guards the
// backward-compatibility guarantee: tokens with nil/empty Scopes (every token
// minted before requireWriteScope existed) must not be blocked from mutating
// requests. Breaking this would lock operators out of production.
func TestAuthz_LegacyEmptyScope_PostAllowed_LockoutPrevention(t *testing.T) {
	ts, _, _, legacyTok, _, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewBufferString(`{"kind":"api","name":"new-token"}`))
	req.Header.Set("Authorization", "Bearer "+legacyTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("legacy (nil-scope) POST /api/v1/admin/tokens: got 403, must not block legacy tokens (lockout-prevention)")
	}
}

// TestAuthz_Admin_PostAllowed verifies that an explicit "admin" scope token
// can call mutating endpoints without restriction.
func TestAuthz_Admin_PostAllowed(t *testing.T) {
	ts, _, _, _, adminTok, cleanup := setupAuthzServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewBufferString(`{"kind":"api","name":"another-token"}`))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("admin POST /api/v1/admin/tokens: got 403, admin tokens must have full write access")
	}
}
