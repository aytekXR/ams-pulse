// Package api_test — D-130 [21]: probe-URL SSRF validation at the API boundary.
// Needs enterprise tier to reach the validator (free tier 403s at the license gate).
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// postProbe POSTs a create body and returns (status, decoded-error-code).
func postProbe(t *testing.T, baseURL, token, url string) (int, string) {
	t.Helper()
	return probeReq(t, http.MethodPost, baseURL+"/api/v1/probes", token, map[string]any{
		"name":       "ssrf probe",
		"url":        url,
		"protocol":   "hls",
		"interval_s": 60,
	})
}

func probeReq(t *testing.T, method, endpoint, token string, body map[string]any) (int, string) {
	t.Helper()
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, endpoint, bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	json.Unmarshal(raw, &m)
	code, _ := m["code"].(string)
	return resp.StatusCode, code
}

// TestProbe_SSRF_RejectedAtCreate_S68 verifies SSRF-prone create URLs → 422 INVALID_PROBE.
func TestProbe_SSRF_RejectedAtCreate_S68(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	for _, tc := range []struct {
		name string
		url  string
	}{
		{"file-scheme", "file:///etc/passwd"},
		{"gopher-scheme", "gopher://169.254.169.254/"},
		{"imdsv4-metadata", "http://169.254.169.254/latest/meta-data/"},
		{"imdsv4-bare", "http://169.254.169.254"},
		{"userinfo-trick", "http://legit.example.com@169.254.169.254/"},
		{"imdsv6", "http://[fd00:ec2::254]/"},
		{"unspecified", "http://0.0.0.0/"},
	} {
		status, code := postProbe(t, ts.URL, token, tc.url)
		if status != http.StatusUnprocessableEntity || code != "INVALID_PROBE" {
			t.Errorf("%s: url=%q → status=%d code=%q, want 422 INVALID_PROBE", tc.name, tc.url, status, code)
		}
	}
}

// TestProbe_SSRF_PrivateAllowedAtCreate_S68 verifies the deliberate deviation from
// the audit's literal fix: a private (RFC-1918) AMS URL is ACCEPTED (201), because
// self-hosted AMS is routinely on an internal network (B4/A6 ruling). Reddening
// this test would signal an over-broad denial that breaks the primary use case.
func TestProbe_SSRF_PrivateAllowedAtCreate_S68(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	for _, u := range []string{
		"http://10.0.0.5:5080/LiveApp/streams/test.m3u8",
		"http://192.168.1.20:5080/live.m3u8",
		"http://127.0.0.1:5080/live.m3u8",
	} {
		status, _ := postProbe(t, ts.URL, token, u)
		if status != http.StatusCreated {
			t.Errorf("private URL %q → status=%d, want 201 (private AMS must be allowed)", u, status)
		}
	}
}

// TestProbe_SSRF_RejectedAtUpdate_S68 verifies the update path validates a supplied
// URL (the merge would otherwise store an unvalidated one).
func TestProbe_SSRF_RejectedAtUpdate_S68(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// Create a valid probe first.
	status, _ := postProbe(t, ts.URL, token, "http://example.com/live.m3u8")
	if status != http.StatusCreated {
		t.Fatalf("setup create → %d, want 201", status)
	}
	// Recover its ID via list.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/probes", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, _ := http.DefaultClient.Do(req)
	var list map[string]any
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	items := list["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no probe found after create")
	}
	id := items[0].(map[string]any)["id"].(string)

	// PUT a metadata URL → must 422, not silently merge.
	status2, code2 := probeReq(t, http.MethodPut, ts.URL+"/api/v1/probes/"+id, token, map[string]any{
		"url": "http://169.254.169.254/latest/meta-data/",
	})
	if status2 != http.StatusUnprocessableEntity || code2 != "INVALID_PROBE" {
		t.Errorf("update to metadata URL → status=%d code=%q, want 422 INVALID_PROBE", status2, code2)
	}
}
