// Package api_test — alert-rule spec validation gate (fast-follow item 5, D-165).
//
// Before this fix, POST and PUT /api/v1/alerts/rules accepted any metric name,
// any operator, any severity, and out-of-range window_s values, returning HTTP
// 201/200 for rules that silently never fired.  The handlers now call
// alert.ValidateRuleSpec for threshold rules and reject invalid specs with HTTP
// 422 INVALID_RULE before touching the store.
//
// JSON cannot represent IEEE 754 NaN or ±Infinity as number literals.
// Go's encoding/json decoder rejects a payload containing these values and
// returns a decode error before the handler body runs — the request receives
// HTTP 400 INVALID_JSON rather than 422 INVALID_RULE.  There is no test case
// for threshold=NaN/Inf because such a payload cannot be constructed with
// standard JSON encoding.
package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// baseValidRule returns a minimal valid threshold-rule body that
// ValidateRuleSpec, ValidateAnomalyRule, and the store all accept.
func baseValidRule(name string) map[string]any {
	return map[string]any{
		"name":       name,
		"metric":     "viewer_count",
		"operator":   "lt",
		"threshold":  5.0,
		"window_s":   60.0,
		"severity":   "warning",
		"cooldown_s": 300.0,
		"enabled":    true,
	}
}

// postAlertRule sends POST /api/v1/alerts/rules with the given body.
// Returns the HTTP response; caller closes body.
func postAlertRule(t *testing.T, ts *httptest.Server, token string, body map[string]any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/alerts/rules: %v", err)
	}
	return resp
}

// putAlertRule sends PUT /api/v1/alerts/rules/{id} with the given body.
func putAlertRule(t *testing.T, ts *httptest.Server, token, id string, body map[string]any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/"+id, bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/v1/alerts/rules/%s: %v", id, err)
	}
	return resp
}

// mutateBody returns a shallow copy of base with the specified key overwritten.
func mutateBody(base map[string]any, key string, val any) map[string]any {
	m := make(map[string]any, len(base))
	for k, v := range base {
		m[k] = v
	}
	m[key] = val
	return m
}

// TestAlertRuleCreate_InvalidSpec_Returns422 covers every hostile field class
// identified in the D-165 review: unknown metric, bad operator, negative / zero
// window_s, invalid severity, over-cap window_s, and the deprecated
// viewer_drop_pct alias (must still be accepted).
func TestAlertRuleCreate_InvalidSpec_Returns422(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	cases := []struct {
		name       string
		bodyMut    func(map[string]any) map[string]any
		wantStatus int
		wantCode   string
		wantInMsg  string // substring expected in the error message
	}{
		{
			name:       "unknown_metric",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "metric", "packets_from_mars") },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_RULE",
			wantInMsg:  "metric",
		},
		{
			name:       "operator_banana",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "operator", "banana") },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_RULE",
			wantInMsg:  "operator",
		},
		{
			name:       "window_s_negative",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "window_s", -3600.0) },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_RULE",
			wantInMsg:  "window_s",
		},
		{
			name:       "window_s_zero",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "window_s", 0.0) },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_RULE",
			wantInMsg:  "window_s",
		},
		{
			name:       "severity_apocalypse",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "severity", "apocalypse") },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_RULE",
			wantInMsg:  "severity",
		},
		{
			// 604801 > 7 days (604800 s cap in alert.maxWindowS)
			name:       "window_s_over_cap",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "window_s", 604801.0) },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_RULE",
			wantInMsg:  "window_s",
		},
		{
			// threshold=1e308 is finite; ValidateRuleSpec rejects only NaN/±Inf.
			// A very large finite threshold is valid — the rule just never fires.
			name:       "threshold_1e308_is_valid",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "threshold", 1e308) },
			wantStatus: http.StatusCreated,
			wantCode:   "",
			wantInMsg:  "",
		},
		{
			// viewer_drop_pct is the deprecated alias for viewer_count_floor (Fix D).
			// It must still be accepted at the API boundary.
			name:       "deprecated_viewer_drop_pct_accepted",
			bodyMut:    func(b map[string]any) map[string]any { return mutateBody(b, "metric", "viewer_drop_pct") },
			wantStatus: http.StatusCreated,
			wantCode:   "",
			wantInMsg:  "",
		},
		{
			// Fully valid baseline rule — control case.
			name:       "valid_rule_gets_201",
			bodyMut:    func(b map[string]any) map[string]any { return b },
			wantStatus: http.StatusCreated,
			wantCode:   "",
			wantInMsg:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.bodyMut(baseValidRule(fmt.Sprintf("test-rule-%s", tc.name)))
			resp := postAlertRule(t, ts, adminToken, body)
			defer resp.Body.Close()

			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("want %d, got %d; body=%s", tc.wantStatus, resp.StatusCode, raw)
				return
			}
			if tc.wantCode == "" {
				// Success case: no error body to check.
				return
			}
			var errBody map[string]any
			if err := json.Unmarshal(raw, &errBody); err != nil {
				t.Fatalf("422 body is not JSON: %v; raw=%s", err, raw)
			}
			if code, _ := errBody["code"].(string); code != tc.wantCode {
				t.Errorf("want code=%q, got %q; body=%s", tc.wantCode, code, raw)
			}
			if tc.wantInMsg != "" {
				msg, _ := errBody["message"].(string)
				if !strings.Contains(msg, tc.wantInMsg) {
					t.Errorf("want message to contain %q, got %q", tc.wantInMsg, msg)
				}
			}
		})
	}
}

// TestAlertRuleUpdate_InvalidSpec_Returns422 verifies the same validation gate
// on the PUT handler.  A valid rule is created first, then an update with an
// invalid spec is submitted to confirm the update path also rejects it.
func TestAlertRuleUpdate_InvalidSpec_Returns422(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a valid rule to get an ID to update.
	createResp := postAlertRule(t, ts, adminToken, baseValidRule("update-baseline"))
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(createResp.Body)
		t.Fatalf("setup: expected 201, got %d: %s", createResp.StatusCode, raw)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	ruleID, _ := created["id"].(string)
	if ruleID == "" {
		t.Fatal("setup: no id in create response")
	}

	cases := []struct {
		name       string
		field      string
		val        any
		wantStatus int
		wantCode   string
		wantInMsg  string
	}{
		{"unknown_metric", "metric", "packets_from_mars", http.StatusUnprocessableEntity, "INVALID_RULE", "metric"},
		{"operator_banana", "operator", "banana", http.StatusUnprocessableEntity, "INVALID_RULE", "operator"},
		{"window_s_negative", "window_s", -3600.0, http.StatusUnprocessableEntity, "INVALID_RULE", "window_s"},
		{"window_s_zero", "window_s", 0.0, http.StatusUnprocessableEntity, "INVALID_RULE", "window_s"},
		{"severity_apocalypse", "severity", "apocalypse", http.StatusUnprocessableEntity, "INVALID_RULE", "severity"},
		{"window_s_over_cap", "window_s", 604801.0, http.StatusUnprocessableEntity, "INVALID_RULE", "window_s"},
		// Valid update — confirms the guard does not block a well-formed body.
		{"valid_update", "threshold", 10.0, http.StatusOK, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := mutateBody(baseValidRule("update-baseline"), tc.field, tc.val)
			resp := putAlertRule(t, ts, adminToken, ruleID, body)
			defer resp.Body.Close()

			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("want %d, got %d; body=%s", tc.wantStatus, resp.StatusCode, raw)
				return
			}
			if tc.wantCode == "" {
				return
			}
			var errBody map[string]any
			if err := json.Unmarshal(raw, &errBody); err != nil {
				t.Fatalf("422 body is not JSON: %v; raw=%s", err, raw)
			}
			if code, _ := errBody["code"].(string); code != tc.wantCode {
				t.Errorf("want code=%q, got %q; body=%s", tc.wantCode, code, raw)
			}
			if tc.wantInMsg != "" {
				msg, _ := errBody["message"].(string)
				if !strings.Contains(msg, tc.wantInMsg) {
					t.Errorf("want message to contain %q, got %q", tc.wantInMsg, msg)
				}
			}
		})
	}
}
