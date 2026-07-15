// Package api_test — S37 (D-099) entitlement-gate tests.
//
// Backs the S37 audit finding "entitlements are enforced, not decorative". Covers
// the HTTP gates added in S37:
//   - Alert-channel UPDATE and TEST-fire must gate the channel type by tier
//     (create was already gated; update/test were the escape hatches).
//   - Report-schedule create/update must gate the white-label header on the
//     white_label entitlement (reports themselves are only Business).
//   - OIDC login/callback/status must gate on the Enterprise SSO entitlement.
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

// ─── Pro-tier server (email/slack/telegram; NO pagerduty/webhook) ─────────────

func setupProTierServer(t *testing.T) (base string, store *meta.Store, token string, cleanup func()) {
	t.Helper()
	licKey, licCleanup := makeTestProLicense(t)

	ctx := context.Background()
	ddl, err := os.ReadFile(metaDDLPath(t))
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found (repo-root mount required): %v", err)
	}
	ms, err := meta.New(ctx, "sqlite", ":memory:", "pro-s37-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	token = "plt_pro_s37_token"
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind: "api", Name: "pro-admin", TokenHash: hashToken(token),
		Scopes: []string{"admin"}, CreatedAt: 1000,
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
	if lic.Tier() != license.TierPro {
		ms.Close()
		licCleanup()
		t.Fatalf("expected pro tier, got %q", lic.Tier())
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, ms, live, qsvc, lic, nil)
	httpSrv := httptest.NewServer(srv.Handler())

	cleanup = func() {
		httpSrv.Close()
		ms.Close()
		licCleanup()
	}
	return httpSrv.URL, ms, token, cleanup
}

// ─── Alert-channel type gate: UPDATE ──────────────────────────────────────────

// TestAlertChannel_UpdateToUnlicensedType_Blocked: a Pro tenant may create an
// email channel but must not be able to UPDATE it to a Business-only type
// (webhook). Mutation proof: removing the CheckChannelAllowed gate in
// handleUpdateAlertChannel returns 200 instead of 403.
func TestAlertChannel_UpdateToUnlicensedType_Blocked(t *testing.T) {
	base, _, token, cleanup := setupProTierServer(t)
	defer cleanup()
	client := http.DefaultClient

	// Create an allowed (email) channel first.
	created := doJSON(t, client, http.MethodPost, base+"/api/v1/alerts/channels", token, map[string]any{
		"type": "email", "name": "ops-email", "config": map[string]any{"to": "ops@example.com"},
	})
	if created.status != http.StatusCreated {
		t.Fatalf("create email channel: expected 201, got %d: %s", created.status, created.body)
	}
	id, _ := created.json["id"].(string)
	if id == "" {
		t.Fatalf("no id in create response: %s", created.body)
	}

	// Update it to a webhook (Business+): must be 403 LICENSE_REQUIRED.
	upd := doJSON(t, client, http.MethodPut, base+"/api/v1/alerts/channels/"+id, token, map[string]any{
		"type": "webhook", "name": "ops-email",
		"config": map[string]any{"webhook_url": "http://example.com/hook"},
	})
	if upd.status != http.StatusForbidden {
		t.Fatalf("update to webhook on pro tier: expected 403, got %d: %s", upd.status, upd.body)
	}
	if upd.json["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v", upd.json["code"])
	}
}

// ─── Alert-channel type gate: TEST-fire ───────────────────────────────────────

// TestAlertChannel_TestFireUnlicensedType_Blocked: firing a test delivery to a
// paid channel type must be gated. We seed a webhook channel directly (bypassing
// the create gate) then POST .../test on a Pro tenant. Mutation proof: removing
// the CheckChannelAllowed gate in handleTestAlertChannel returns 200.
func TestAlertChannel_TestFireUnlicensedType_Blocked(t *testing.T) {
	base, store, token, cleanup := setupProTierServer(t)
	defer cleanup()
	client := http.DefaultClient

	seeded, err := store.CreateAlertChannel(context.Background(), meta.AlertChannelRow{
		Type: "webhook", Name: "seeded-webhook", ConfigPublic: `{"webhook_url":"http://example.com/hook"}`,
	})
	if err != nil {
		t.Fatalf("seed webhook channel: %v", err)
	}

	res := doJSON(t, client, http.MethodPost, base+"/api/v1/alerts/channels/"+seeded.ID+"/test", token, nil)
	if res.status != http.StatusForbidden {
		t.Fatalf("test-fire webhook on pro tier: expected 403, got %d: %s", res.status, res.body)
	}
	if res.json["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v", res.json["code"])
	}
}

// ─── White-label report-schedule gate ─────────────────────────────────────────

// TestReportSchedule_WhiteLabelHeader_BlockedOnBusiness: reports ARE licensed on
// Business, but white-label branding is Enterprise-only. A schedule WITHOUT a
// header succeeds (201); the same schedule WITH a whitelabel_header is 403.
// Mutation proof: removing the CheckWhiteLabel gate in handleCreateReportSchedule
// makes the header variant return 201.
func TestReportSchedule_WhiteLabelHeader_BlockedOnBusiness(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	client := http.DefaultClient

	// Positive control: plain schedule (no header) is allowed on Business.
	plain := doJSON(t, client, http.MethodPost, ts.URL+"/api/v1/reports/schedules", token, map[string]any{
		"cron": "0 0 *", "format": "csv",
	})
	if plain.status != http.StatusCreated {
		t.Fatalf("plain schedule on business: expected 201, got %d: %s", plain.status, plain.body)
	}

	// White-label header must be rejected on Business.
	wl := doJSON(t, client, http.MethodPost, ts.URL+"/api/v1/reports/schedules", token, map[string]any{
		"cron": "0 0 *", "format": "csv",
		"whitelabel_header": map[string]any{"name": "ACME Corp"},
	})
	if wl.status != http.StatusForbidden {
		t.Fatalf("white-label schedule on business: expected 403, got %d: %s", wl.status, wl.body)
	}
	if wl.json["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v", wl.json["code"])
	}
}

// TestReportSchedule_WhiteLabelHeader_AllowedOnEnterprise is the positive control:
// the same white-label schedule succeeds when the tier licenses white-label.
func TestReportSchedule_WhiteLabelHeader_AllowedOnEnterprise(t *testing.T) {
	ts, token, cleanup := setupEnterpriseAnomalyServer(t) // enterprise tier (white_label:true)
	defer cleanup()

	wl := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/reports/schedules", token, map[string]any{
		"cron": "0 0 *", "format": "csv",
		"whitelabel_header": map[string]any{"name": "ACME Corp"},
	})
	if wl.status != http.StatusCreated {
		t.Fatalf("white-label schedule on enterprise: expected 201, got %d: %s", wl.status, wl.body)
	}
}

// ─── SSO/OIDC Enterprise gate ─────────────────────────────────────────────────

// TestOIDCGate_LoginBlockedOnFreeTier: an OIDC-configured server on a non-Enterprise
// tier must 403 the login endpoint (SSO is Enterprise-only). Mutation proof:
// removing the CheckSSO gate in handleOIDCLogin yields a 302 redirect instead.
func TestOIDCGate_LoginBlockedOnFreeTier(t *testing.T) {
	freeLic, _ := license.New("", "")
	env := setupOIDCTestServerTier(t, "viewer", nil, "http://example.com/auth/oidc/callback", freeLic)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(env.srv.URL + "/auth/oidc/login")
	if err != nil {
		t.Fatalf("GET /auth/oidc/login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login on free tier: expected 403, got %d: %s", resp.StatusCode, body)
	}
}

// TestOIDCGate_CallbackBlockedOnFreeTier: the OIDC CALLBACK endpoint must also 403
// on a non-Enterprise tier. This is the gap the S37 adversarial review caught — the
// gate existed in handleOIDCCallback but no test exercised it, so deleting it failed
// zero tests. Without the CheckSSO gate the callback would fall through to its inner
// MISSING_STATE (400) check; with it, an unlicensed callback is 403 LICENSE_REQUIRED.
// Concrete risk: an admin starts login on Enterprise (state cookie, 10-min TTL), the
// license is downgraded, then the IdP redirect completes — the gate must refuse it.
func TestOIDCGate_CallbackBlockedOnFreeTier(t *testing.T) {
	freeLic, _ := license.New("", "")
	env := setupOIDCTestServerTier(t, "viewer", nil, "http://example.com/auth/oidc/callback", freeLic)

	// No state cookie / code: without the gate this is 400 MISSING_STATE; with it, 403.
	resp, err := http.Get(env.srv.URL + "/auth/oidc/callback")
	if err != nil {
		t.Fatalf("GET /auth/oidc/callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("callback on free tier: expected 403, got %d: %s", resp.StatusCode, body)
	}
	var er map[string]any
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &er)
	if er["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v (%s)", er["code"], body)
	}
}

// TestOIDCGate_StatusFalseOnFreeTier: /auth/oidc/status must report enabled=false
// on a non-Enterprise tier even when OIDC is fully configured, so the UI hides the
// SSO button. Mutation proof: reverting handleOIDCStatus to `s.oidc != nil` alone
// reports enabled=true.
func TestOIDCGate_StatusFalseOnFreeTier(t *testing.T) {
	freeLic, _ := license.New("", "")
	env := setupOIDCTestServerTier(t, "viewer", nil, "http://example.com/auth/oidc/callback", freeLic)

	resp, err := http.Get(env.srv.URL + "/auth/oidc/status")
	if err != nil {
		t.Fatalf("GET /auth/oidc/status: %v", err)
	}
	defer resp.Body.Close()
	var status struct {
		Enabled bool `json:"enabled"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatalf("parse status JSON %q: %v", body, err)
	}
	if status.Enabled {
		t.Errorf("free tier with OIDC configured: expected enabled=false, got true (%s)", body)
	}
}

// ─── small JSON helpers ───────────────────────────────────────────────────────

type jsonResp struct {
	status int
	body   string
	json   map[string]any
}

func doJSON(t *testing.T, client *http.Client, method, url, token string, payload any) jsonResp {
	t.Helper()
	var reader io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := jsonResp{status: resp.StatusCode, body: string(raw)}
	_ = json.Unmarshal(raw, &out.json)
	return out
}
