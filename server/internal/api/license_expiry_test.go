// Package api_test — D-089 RULE-1: three-state license distinguishability.
//
// Verifies that GET /admin/license returns the HONEST state (tier=free,
// valid=false, expires_at=<past non-nil>) for a license key that was already
// expired at boot — instead of the old silent setFree (valid=true, expires_at=nil).
//
// TDD: written RED against the old code; goes GREEN once license.go implements
// the lazy-expiry downgrade via maybeExpireLocked().
package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// makeExpiredProLicenseAPI mints a pro-tier license key with expires_at 24 h in
// the past and installs the matching public key via t.Setenv.
// Returns only the signed key; cleanup is handled by t.Setenv automatically.
func makeExpiredProLicenseAPI(t *testing.T) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 key pair: %v", err)
	}

	pastMs := time.Now().Add(-24 * time.Hour).UnixMilli()
	claims := map[string]any{
		"tier":       "pro",
		"data_api":   true,
		"expires_at": pastMs,
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	sig := ed25519.Sign(priv, claimsJSON)
	key := base64.StdEncoding.EncodeToString(claimsJSON) + "." +
		base64.StdEncoding.EncodeToString(sig)

	t.Setenv("PULSE_LICENSE_PUBKEY", hex.EncodeToString(pub))
	return key
}

// TestLicenseExpiry_GetLicense_HonestState verifies GET /admin/license returns:
//
//	{ "tier": "free", "valid": false, "expires_at": <non-null past value> }
//
// for a key that was already expired when the Manager was created.
// This is RULE-1 three-state distinguishability:
//
//	(free/valid=true/nil)   → no key (never had a license)
//	(pro/valid=true/future) → active trial / paid tier
//	(free/valid=false/past) → expired trial        ← this test
func TestLicenseExpiry_GetLicense_HonestState(t *testing.T) {
	expiredKey := makeExpiredProLicenseAPI(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)

	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Fatalf("meta DDL not found (repo-root mount required, D-028/D-064): %v", err)
	}

	ms, err := meta.New(ctx, "sqlite", ":memory:", "expiry-api-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { ms.Close() })

	adminToken := "plt_expiry_api_test_abc123"
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "expiry-api-admin",
		TokenHash: hashToken(adminToken),
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// license.New with an already-expired key — must not return an error (fail-open).
	lic, err := license.New(expiredKey, "")
	if err != nil {
		t.Fatalf("license.New with expired key: %v (fail-open must be preserved)", err)
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, ms, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/license", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", authHeader(adminToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/admin/license: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, rawBody)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	// RULE-1 assertions — three-state distinguishability.
	tier, _ := body["tier"].(string)
	if tier != "free" {
		t.Errorf("RULE-1 FAIL: tier want %q got %q", "free", tier)
	}

	valid, _ := body["valid"].(bool)
	if valid {
		t.Errorf("RULE-1 FAIL: valid want false got true")
	}

	if body["expires_at"] == nil {
		t.Error("RULE-1 FAIL: expires_at must be non-nil (past expiry must be retained, not nil'd by setFree)")
	}

	t.Logf("PASS RULE-1: GET /api/v1/admin/license → tier=%q valid=%v expires_at=%v",
		body["tier"], body["valid"], body["expires_at"])
}
