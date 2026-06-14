// Package api_test — Wave 3 interval validation test (needs enterprise tier to reach the validator).
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestProbe_IntervalValidation_422 verifies that interval_s < 30 returns 422 on enterprise tier.
// The free tier test (TestProbe_IntervalValidation) gets 403 before reaching the validator.
func TestProbe_IntervalValidation_422(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	body := map[string]any{
		"name":       "Bad Interval Probe",
		"url":        "http://example.com/stream.m3u8",
		"protocol":   "hls",
		"interval_s": 10, // < 30: invalid
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/probes", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /probes: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for interval_s<30, got %d: %s", resp.StatusCode, respBody)
	}
	var errResp map[string]any
	json.Unmarshal(respBody, &errResp)
	if code, ok := errResp["code"].(string); ok && code != "INVALID_PROBE" {
		t.Errorf("expected code=INVALID_PROBE, got %q", code)
	}
	t.Logf("PASS: interval_s=10 on enterprise tier → 422 INVALID_PROBE")
}

// TestProbe_ProtocolValidation_422 verifies that an invalid protocol returns 422.
func TestProbe_ProtocolValidation_422(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	body := map[string]any{
		"name":       "Bad Protocol Probe",
		"url":        "http://example.com/stream.m3u8",
		"protocol":   "srt", // not in enum
		"interval_s": 60,
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/probes", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /probes: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		body2, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 422 for invalid protocol, got %d: %s", resp.StatusCode, body2)
	}
	t.Logf("PASS: invalid protocol → 422 INVALID_PROBE")
}

// TestProbe_FullLifecycle verifies create → list (with last_result) → update → delete.
func TestProbe_FullLifecycle(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// Create.
	createBody := map[string]any{
		"name":       "Lifecycle Probe",
		"url":        "http://example.com/hls/live.m3u8",
		"protocol":   "hls",
		"interval_s": 60,
		"timeout_s":  15,
		"enabled":    true,
	}
	bodyBytes, _ := json.Marshal(createBody)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/probes", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /probes: %v", err)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %v", resp.StatusCode, created)
	}
	probeID := created["id"].(string)
	t.Logf("PASS: POST /probes → 201 (id=%q)", probeID)

	// List — probe should appear with last_result absent (no results yet).
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/probes", nil)
	req2.Header.Set("Authorization", authHeader(token))
	resp2, _ := http.DefaultClient.Do(req2)
	var listResp map[string]any
	json.NewDecoder(resp2.Body).Decode(&listResp)
	resp2.Body.Close()

	items := listResp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 probe in list, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["id"] != probeID {
		t.Errorf("expected probe ID %q in list, got %q", probeID, item["id"])
	}
	t.Logf("PASS: GET /probes → list includes created probe (id=%q)", probeID)

	// Update.
	updateBody := map[string]any{
		"name":       "Updated Lifecycle Probe",
		"interval_s": 120,
	}
	ub, _ := json.Marshal(updateBody)
	req3, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/probes/"+probeID, bytes.NewReader(ub))
	req3.Header.Set("Authorization", authHeader(token))
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req3)
	var updated map[string]any
	json.NewDecoder(resp3.Body).Decode(&updated)
	resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from PUT, got %d: %v", resp3.StatusCode, updated)
	}
	if updated["name"] != "Updated Lifecycle Probe" {
		t.Errorf("expected name=Updated Lifecycle Probe, got %v", updated["name"])
	}
	t.Logf("PASS: PUT /probes/%s → 200 (name=%v)", probeID, updated["name"])

	// Delete.
	req4, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/probes/"+probeID, nil)
	req4.Header.Set("Authorization", authHeader(token))
	resp4, _ := http.DefaultClient.Do(req4)
	resp4.Body.Close()

	if resp4.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 from DELETE, got %d", resp4.StatusCode)
	}
	t.Logf("PASS: DELETE /probes/%s → 204", probeID)

	// Verify gone.
	req5, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/probes", nil)
	req5.Header.Set("Authorization", authHeader(token))
	resp5, _ := http.DefaultClient.Do(req5)
	var listResp2 map[string]any
	json.NewDecoder(resp5.Body).Decode(&listResp2)
	resp5.Body.Close()

	items2 := listResp2["items"].([]any)
	if len(items2) != 0 {
		t.Errorf("expected 0 probes after delete, got %d", len(items2))
	}
	t.Logf("PASS: DELETE verified — 0 probes in list after deletion")
}
