// api_anomaly_contract_test.go — S11 WO-B contract tests for anomaly alert rules.
//
// RED pass: these tests FAIL before alertRuleToAPI encodes rule_type/sigma/min_samples
// and before handleCreateAlertRule calls ValidateAnomalyRule.
// GREEN pass: all assertions pass after WO-B api/server.go changes are applied.
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestContractAlertRuleIncludesRuleType verifies that POST /api/v1/alerts/rules with
// rule_type=anomaly returns a 201 response body with rule_type, sigma, min_samples.
func TestContractAlertRuleIncludesRuleType(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	body := map[string]any{
		"name":        "contract-anomaly-1",
		"metric":      "viewer_count",
		"rule_type":   "anomaly",
		"window_s":    3600,
		"sigma":       2.5,
		"min_samples": 5,
		"severity":    "warning",
		"operator":    "gt",
		"threshold":   0,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/alerts/rules: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Assert rule_type is returned.
	if rt, _ := out["rule_type"].(string); rt != "anomaly" {
		t.Errorf("expected rule_type=anomaly in response, got %q (full body: %v)", rt, out)
	}
	// Assert sigma is returned (JSON numbers decode as float64).
	if sigma, _ := out["sigma"].(float64); sigma != 2.5 {
		t.Errorf("expected sigma=2.5 in response, got %v", sigma)
	}
	// Assert min_samples is returned.
	if ms, _ := out["min_samples"].(float64); ms != 5 {
		t.Errorf("expected min_samples=5 in response, got %v", ms)
	}
}

// TestContractAlertRule_AnomalyBadMetric400 verifies that POST /api/v1/alerts/rules
// with rule_type=anomaly and an unsupported metric returns HTTP 400 INVALID_ANOMALY_RULE.
func TestContractAlertRule_AnomalyBadMetric400(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	body := map[string]any{
		"name":        "contract-anomaly-bad-metric",
		"metric":      "ingest_bitrate_kbps",
		"rule_type":   "anomaly",
		"window_s":    3600,
		"sigma":       2.0,
		"min_samples": 5,
		"severity":    "warning",
		"operator":    "gt",
		"threshold":   0,
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/alerts/rules: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported anomaly metric, got %d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if code, _ := out["code"].(string); code != "INVALID_ANOMALY_RULE" {
		t.Errorf("expected code=INVALID_ANOMALY_RULE, got %q (full body: %v)", code, out)
	}
}
