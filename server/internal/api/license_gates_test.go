// Package api_test — A2: license gate tests.
//
// These tests verify that the 3 license gates (CheckDataAPI, CheckPrometheus,
// CheckNodeLimit) actually block requests on the FREE tier. Today the gates are
// defined in license.Manager but are never called, so these tests are RED.
//
// Pattern mirrors v3b_guard_test.go.
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
)

// ─── A2-1: Data API gate (CheckDataAPI) ──────────────────────────────────────

// TestLicenseGate_DataAPI_FreeTier_Blocks verifies that all data-API analytics
// endpoints (including qoe/ingest) require Pro+ tier. FREE tier must receive
// 403 LICENSE_REQUIRED.
//
// RED today for /qoe/ingest: handler returns 200 without calling s.lic.CheckDataAPI().
// GREEN after 2a fix (CheckDataAPI gate added to handleIngestHealth).
func TestLicenseGate_DataAPI_FreeTier_Blocks(t *testing.T) {
	ts, token, cleanup := setupTestServer(t) // free tier (license.New("",""))
	defer cleanup()

	endpoints := []string{
		"/api/v1/analytics/audience",
		"/api/v1/analytics/geo",
		"/api/v1/analytics/devices",
		"/api/v1/qoe/summary",
		"/api/v1/qoe/ingest", // F4: ingest health requires Pro+ (was leaking to Free)
	}
	for _, ep := range endpoints {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+ep, nil)
		req.Header.Set("Authorization", authHeader(token))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", ep, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("A2 FAIL: expected 403 for %s on free tier, got %d: %s",
				ep, resp.StatusCode, body)
		} else {
			var errResp map[string]any
			json.Unmarshal(body, &errResp)
			if errResp["code"] != "LICENSE_REQUIRED" {
				t.Errorf("expected code=LICENSE_REQUIRED for %s, got %v", ep, errResp["code"])
			}
			t.Logf("PASS A2: %s → 403 LICENSE_REQUIRED on free tier", ep)
		}
	}
}

// ─── A2-2: Prometheus gate (CheckPrometheus) ─────────────────────────────────

// TestLicenseGate_Prometheus_FreeTier_Blocks verifies that /metrics requires
// Business+ tier. FREE tier must receive 403 LICENSE_REQUIRED.
//
// RED today: handleMetrics returns 200 without calling s.lic.CheckPrometheus().
func TestLicenseGate_Prometheus_FreeTier_Blocks(t *testing.T) {
	ts, _, cleanup := setupTestServer(t) // free tier
	defer cleanup()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("A2 FAIL: expected 403 for /metrics on free tier, got %d: %s",
			resp.StatusCode, body)
	}
	var errResp map[string]any
	json.Unmarshal(body, &errResp)
	if errResp["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v", errResp["code"])
	}
	t.Logf("PASS A2: /metrics → 403 LICENSE_REQUIRED on free tier")
}

// TestLicenseGate_Prometheus_BusinessTier_Allows verifies that Business+ tier
// can access /metrics (returns 200).
func TestLicenseGate_Prometheus_BusinessTier_Allows(t *testing.T) {
	ts, _, cleanup := setupBusinessServer(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /metrics on business tier, got %d: %s",
			resp.StatusCode, body)
	}
	t.Logf("PASS A2: /metrics → 200 on business tier")
}

// ─── A2-3: Node limit gate (CheckNodeLimit) ──────────────────────────────────

// TestLicenseGate_NodeLimit_ExceedsFreeTier_Blocks verifies that creating
// sources beyond the Free tier node limit (MaxNodes=1) returns 403.
//
// RED today: handleCreateSource never calls s.lic.CheckNodeLimit().
func TestLicenseGate_NodeLimit_ExceedsFreeTier_Blocks(t *testing.T) {
	ts, token, cleanup := setupTestServer(t) // free tier (MaxNodes=1)
	defer cleanup()

	createSource := func(name string) (int, []byte) {
		body := map[string]any{
			"name":     name,
			"type":     "rest",
			"rest_url": "http://127.0.0.1:5080",
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources",
			bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /admin/sources (%s): %v", name, err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, respBody
	}

	// First source: Free tier allows 1 → must be 201.
	status1, body1 := createSource("source-1")
	if status1 != http.StatusCreated {
		t.Fatalf("expected 201 for first source (free MaxNodes=1), got %d: %s", status1, body1)
	}
	t.Logf("PASS A2: first source → 201 (within free tier limit)")

	// Second source: would be 2 nodes → must be 403.
	// RED today: returns 201 (no limit check).
	status2, body2 := createSource("source-2")
	if status2 != http.StatusForbidden {
		t.Fatalf("A2 FAIL: expected 403 for second source (exceeds free MaxNodes=1), got %d: %s",
			status2, body2)
	}
	var errResp map[string]any
	json.Unmarshal(body2, &errResp)
	if errResp["code"] != "LICENSE_REQUIRED" {
		t.Errorf("expected code=LICENSE_REQUIRED, got %v", errResp["code"])
	}
	t.Logf("PASS A2: second source → 403 LICENSE_REQUIRED (exceeds free tier limit)")
}

// TestLicenseGate_NodeLimit_ConcurrentCreatesSerialized guards the D-041 TOCTOU
// fix: many concurrent POST /admin/sources on a Free-tier server (MaxNodes=1) must
// yield EXACTLY ONE 201; the rest must be 403. Without the sourceMu serialization
// several creates could observe the same pre-create count of 0 and all succeed.
func TestLicenseGate_NodeLimit_ConcurrentCreatesSerialized(t *testing.T) {
	ts, token, cleanup := setupTestServer(t) // free tier (MaxNodes=1)
	defer cleanup()

	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{
				"name":     "race-source",
				"type":     "rest",
				"rest_url": "http://127.0.0.1:5080",
			})
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources",
				bytes.NewReader(body))
			req.Header.Set("Authorization", authHeader(token))
			req.Header.Set("Content-Type", "application/json")
			<-start // release all goroutines together to maximize contention
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				codes[idx] = -1
				return
			}
			resp.Body.Close()
			codes[idx] = resp.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()

	created, forbidden := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusForbidden:
			forbidden++
		}
	}
	if created != 1 {
		t.Fatalf("TOCTOU: expected exactly 1 created (free MaxNodes=1), got %d (codes=%v)", created, codes)
	}
	if forbidden != n-1 {
		t.Fatalf("expected %d forbidden, got %d (codes=%v)", n-1, forbidden, codes)
	}
	t.Logf("PASS: %d concurrent creates → exactly 1×201, %d×403", n, forbidden)
}
