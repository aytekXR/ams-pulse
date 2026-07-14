// Package api_test — tests for GET /api/v1/reports/export (CSV export, tier gate).
//
// RED-PROVEN requirement: each test was observed to fail after a deliberate
// mutation of the handler (broken tier gate, wrong header row) before the
// mutation was reverted. See verifiedHow in the task report.
package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestReportExport_CSV_200 verifies that a Business-tier request returns 200
// with Content-Type text/csv and a correct header row.
func TestReportExport_CSV_200(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/reports/export?format=csv", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/export?format=csv: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("expected Content-Type text/csv, got %q", ct)
	}

	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("expected Content-Disposition attachment, got %q", cd)
	}

	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least a CSV header row, got empty body")
	}
	header := lines[0]
	wantHeader := "app,stream_id,tenant,viewer_minutes,peak_concurrency,egress_gb,recording_gb,egress_method"
	if header != wantHeader {
		t.Errorf("CSV header mismatch:\n  want: %q\n   got: %q", wantHeader, header)
	}
	t.Logf("PASS: GET /reports/export?format=csv → 200, header=%q, %d rows", header, len(lines)-1)
}

// TestReportExport_FreeTier_403 verifies that the tier gate rejects Free-tier tokens.
func TestReportExport_FreeTier_403(t *testing.T) {
	ts, token, cleanup := setupTestServer(t) // free tier
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/reports/export?format=csv", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/export (free tier): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 403 for free tier, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("PASS: GET /reports/export blocked on free tier → 403")
}

// TestReportExport_TokenQueryParam_200 verifies that auth via ?token= query param
// works (required for browser downloads that cannot set Authorization headers).
func TestReportExport_TokenQueryParam_200(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	// No Authorization header — use ?token= only.
	url := ts.URL + "/api/v1/reports/export?format=csv&token=" + token
	req, _ := http.NewRequest(http.MethodGet, url, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/export?token=...: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 with ?token= auth, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("expected text/csv Content-Type, got %q", ct)
	}
	t.Logf("PASS: GET /reports/export?token=... (no Authorization header) → 200")
}

// TestReportExport_MissingToken_401 verifies that requests without any auth token
// are rejected with 401.
func TestReportExport_MissingToken_401(t *testing.T) {
	ts, _, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/reports/export?format=csv", nil)
	// No auth.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/export (no auth): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 without auth, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("PASS: GET /reports/export without auth → 401")
}

// TestReportExport_PDF_501 verifies that format=pdf returns 501 NOT_IMPLEMENTED.
func TestReportExport_PDF_501(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/reports/export?format=pdf", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/export?format=pdf: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 501 for format=pdf, got %d: %s", resp.StatusCode, body)
	}
	t.Logf("PASS: GET /reports/export?format=pdf → 501 NOT_IMPLEMENTED")
}

// TestReportExport_DefaultFormat_CSV verifies that omitting ?format= defaults to CSV.
func TestReportExport_DefaultFormat_CSV(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/v1/reports/export", nil) // no format param
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/export (no format): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 when format omitted, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("expected text/csv when format omitted, got %q", ct)
	}
	t.Logf("PASS: GET /reports/export (no format) → 200 text/csv")
}
