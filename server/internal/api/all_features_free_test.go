package api_test

// D-194 (v0.5.0): Pulse is free — every feature, for everyone. cmd/pulse turns
// the license manager's all-features-free policy on, so a keyless server must
// serve every formerly gated endpoint and must say so on GET /admin/license.
//
// Each endpoint below is first proven to be gated on an enforcing keyless server
// (the control), so this test cannot pass by picking endpoints that were never
// gated in the first place.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aytekXR/ams-pulse/server/internal/license"
)

type gatedRequest struct {
	method, path, body string
}

var formerlyGated = []gatedRequest{
	{http.MethodGet, "/metrics", ""},
	{http.MethodGet, "/api/v1/qoe/summary", ""},
	{http.MethodGet, "/api/v1/reports/schedules", ""},
	{http.MethodGet, "/api/v1/admin/tenants", ""},
	{http.MethodGet, "/api/v1/anomalies", ""},
	{http.MethodGet, "/api/v1/probes", ""},
	{http.MethodGet, "/api/v1/reports/export?type=usage&format=csv", ""},
	// Slack is not a Free-tier channel under enforcement.
	{http.MethodPost, "/api/v1/alerts/channels",
		`{"name":"ops","type":"slack","config":{"webhook_url":"https://hooks.slack.com/services/T0/B0/x"}}`},
}

func allFreeLicense(t *testing.T) *license.Manager {
	t.Helper()
	lic, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	lic.SetAllFeaturesFree(true)
	return lic
}

// doReq sends the request with the admin token and returns status + error code.
func doReq(t *testing.T, base, token string, r gatedRequest) (int, string) {
	t.Helper()
	var body io.Reader
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	req, err := http.NewRequest(r.method, base+r.path, body)
	if err != nil {
		t.Fatalf("NewRequest %s %s: %v", r.method, r.path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if r.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", r.method, r.path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &e)
	return resp.StatusCode, e.Code
}

func TestAllFeaturesFree_FormerlyGatedEndpointsOpenWithoutKey(t *testing.T) {
	enforcing, enfTok, enfCleanup := setupTestServer(t)
	defer enfCleanup()
	free, freeTok, freeCleanup := setupTestServerWithLicense(t, allFreeLicense(t))
	defer freeCleanup()

	for _, r := range formerlyGated {
		name := r.method + " " + r.path
		t.Run(name, func(t *testing.T) {
			// Control: under tier enforcement a keyless server refuses it.
			if status, code := doReq(t, enforcing.URL, enfTok, r); status != http.StatusForbidden || code != "LICENSE_REQUIRED" {
				t.Fatalf("control: enforcing keyless server returned %d %q, want 403 LICENSE_REQUIRED — "+
					"this endpoint is not gated, so it proves nothing here", status, code)
			}
			// v0.5.0: the same request on an all-features-free keyless server must pass the gate.
			if status, code := doReq(t, free.URL, freeTok, r); code == "LICENSE_REQUIRED" {
				t.Errorf("all-features-free server still refused it: %d %q", status, code)
			}
		})
	}
}

func TestAllFeaturesFree_LicenseEndpointReportsPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lic      func(t *testing.T) *license.Manager
		wantFree bool
	}{
		{"policy on (v0.5.0 server)", allFreeLicense, true},
		{"policy off (enforcing)", func(t *testing.T) *license.Manager { l, _ := license.New("", ""); return l }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, tok, cleanup := setupTestServerWithLicense(t, tc.lic(t))
			defer cleanup()
			req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/license", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET /admin/license: %v", err)
			}
			defer resp.Body.Close()
			var got struct {
				Tier            string         `json:"tier"`
				Valid           bool           `json:"valid"`
				AllFeaturesFree *bool          `json:"all_features_free"`
				Limits          map[string]any `json:"limits"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.AllFeaturesFree == nil {
				t.Fatal("all_features_free is missing — v0.5.0 servers must always send it")
			}
			if *got.AllFeaturesFree != tc.wantFree {
				t.Errorf("all_features_free = %v, want %v", *got.AllFeaturesFree, tc.wantFree)
			}
			if got.Tier != "free" || !got.Valid {
				t.Errorf("tier/valid = %q/%v, want free/true (no key is loaded)", got.Tier, got.Valid)
			}
			if tc.wantFree {
				for _, k := range []string{"max_nodes", "max_streams", "retention_days"} {
					if got.Limits[k] != nil {
						t.Errorf("limits.%s = %v, want null (unlimited)", k, got.Limits[k])
					}
				}
				if got.Limits["data_api"] != true || got.Limits["white_label"] != true {
					t.Errorf("limits data_api/white_label = %v/%v, want true/true",
						got.Limits["data_api"], got.Limits["white_label"])
				}
			}
		})
	}
}
