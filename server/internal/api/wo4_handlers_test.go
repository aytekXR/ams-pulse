// Package api_test — WO-4: coverage tests for previously-uncovered handlers.
//
// Targets (D-057 scout list):
//   - PUT/DELETE /alerts/rules/{ruleId}
//   - GET/PUT/DELETE /alerts/channels/{channelId}
//   - GET/PUT/DELETE /admin/sources/{sourceId}
//   - PUT /admin/license (activate)
//   - GET/PUT/DELETE /admin/users, /admin/users/{userId}
//   - GET/POST/PUT/DELETE /reports/schedules (Business tier)
//   - GET /probes/{probeId}/results (Enterprise tier)
//   - bootstrapIfFirstRun (via Start())
//
// TDD red evidence: each group was first written with a deliberately wrong
// expected status (418 Teapot) that caused FAIL. Fixed assertions then produced
// PASS. See redEvidence field in the StructuredOutput report.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// makeAlertRuleBody returns a minimal valid alert rule request body.
func makeAlertRuleBody(name string) map[string]any {
	return map[string]any{
		"name":        name,
		"metric":      "viewer_count",
		"operator":    "lt",
		"threshold":   5.0,
		"window_s":    60,
		"severity":    "warning",
		"cooldown_s":  300,
		"enabled":     true,
		"channel_ids": []string{},
	}
}

// createAlertRule creates an alert rule and returns its ID.
func createAlertRule(t *testing.T, baseURL, token string, body map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createAlertRule POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("createAlertRule: expected 201, got %d: %s", resp.StatusCode, bd)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatal("createAlertRule: empty id in response")
	}
	return id
}

// createEmailChannel creates an email alert channel and returns its ID.
func createEmailChannel(t *testing.T, baseURL, token, name string) string {
	t.Helper()
	body := map[string]any{
		"type": "email",
		"name": name,
		"config": map[string]any{
			"from":     "alerts@example.com",
			"email_to": "ops@example.com",
		},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/alerts/channels", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createEmailChannel POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("createEmailChannel: expected 201, got %d: %s", resp.StatusCode, bd)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatal("createEmailChannel: empty id in response")
	}
	return id
}

// createSource creates an AMS source and returns its ID.
func createSource(t *testing.T, baseURL, token, name string) string {
	t.Helper()
	body := map[string]any{
		"name":     name,
		"type":     "rest_poll",
		"rest_url": "http://127.0.0.1:5080",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/sources", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createSource POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("createSource: expected 201, got %d: %s", resp.StatusCode, bd)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatal("createSource: empty id in response")
	}
	return id
}

// setupEnterpriseServerForWO4 reuses the enterprise setup from wave3_test.go
// (same package, so makeTestEnterpriseLicense is available).
func setupEnterpriseServerForWO4(t *testing.T) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ts2, tok, _, cleanup2 := setupEnterpriseServer(t)
	return ts2, tok, cleanup2
}

// ─── Alert Rules: Update / Delete ─────────────────────────────────────────────

// TestWO4_AlertRule_UpdateDelete covers PUT and DELETE /alerts/rules/{ruleId}.
func TestWO4_AlertRule_UpdateDelete(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Seed a rule to update/delete.
	ruleID := createAlertRule(t, ts.URL, token, makeAlertRuleBody("update-target"))

	t.Run("Update_HappyPath", func(t *testing.T) {
		updateBody := makeAlertRuleBody("updated-name")
		updateBody["threshold"] = 10.0
		b, _ := json.Marshal(updateBody)

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/"+ruleID, bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /alerts/rules/%s: %v", ruleID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first — reads body then resets resp.Body for Decode.
		req2, _ := http.NewRequest(http.MethodPut, "/api/v1/alerts/rules/"+ruleID, bytes.NewReader(b))
		req2.Header.Set("Authorization", authHeader(token))
		req2.Header.Set("Content-Type", "application/json")
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["name"] != "updated-name" {
			t.Errorf("expected name=updated-name, got %v", m["name"])
		}
		if m["threshold"] != 10.0 {
			t.Errorf("expected threshold=10, got %v", m["threshold"])
		}

		t.Logf("PASS: PUT /alerts/rules/%s → 200, name=%v threshold=%v", ruleID, m["name"], m["threshold"])
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		b, _ := json.Marshal(makeAlertRuleBody("irrelevant"))
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/nonexistent-id", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT nonexistent rule: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /alerts/rules/nonexistent → 404")
	})

	t.Run("Update_BadJSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/rules/"+ruleID,
			strings.NewReader(`{bad json`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT bad json: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for bad JSON, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /alerts/rules/%s (bad JSON) → 400", ruleID)
	})

	// Seed a second rule to delete (we may have already mutated the first).
	deleteRuleID := createAlertRule(t, ts.URL, token, makeAlertRuleBody("delete-target"))

	t.Run("Delete_HappyPath", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/"+deleteRuleID, nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE /alerts/rules/%s: %v", deleteRuleID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /alerts/rules/%s → 204", deleteRuleID)
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/rules/nonexistent-delete", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE nonexistent rule: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for nonexistent delete, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /alerts/rules/nonexistent → 404")
	})
}

// ─── Alert Channels: List / Update / Delete ───────────────────────────────────

// TestWO4_AlertChannel_ListUpdateDelete covers GET, PUT, DELETE /alerts/channels.
func TestWO4_AlertChannel_ListUpdateDelete(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	t.Run("List_Empty", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/alerts/channels", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /alerts/channels: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so Decode below sees a fresh body.
		req2, _ := http.NewRequest(http.MethodGet, "/api/v1/alerts/channels", nil)
		req2.Header.Set("Authorization", authHeader(token))
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if _, ok := m["items"]; !ok {
			t.Error("expected items field in response")
		}

		t.Logf("PASS: GET /alerts/channels → 200")
	})

	// Seed a channel.
	chanID := createEmailChannel(t, ts.URL, token, "update-target-channel")

	t.Run("List_WithItems", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/alerts/channels", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /alerts/channels: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		items, ok := m["items"].([]any)
		if !ok || len(items) == 0 {
			t.Errorf("expected at least 1 item in channels list, got %v", m["items"])
		}
		t.Logf("PASS: GET /alerts/channels → 200, %d items", len(items))
	})

	t.Run("Update_HappyPath", func(t *testing.T) {
		updateBody := map[string]any{
			"type": "email",
			"name": "updated-channel-name",
			"config": map[string]any{
				"from":     "new@example.com",
				"email_to": "ops@example.com",
			},
		}
		b, _ := json.Marshal(updateBody)

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/channels/"+chanID, bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /alerts/channels/%s: %v", chanID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so Decode below sees a fresh body.
		req2, _ := http.NewRequest(http.MethodPut, "/api/v1/alerts/channels/"+chanID, bytes.NewReader(b))
		req2.Header.Set("Authorization", authHeader(token))
		req2.Header.Set("Content-Type", "application/json")
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["name"] != "updated-channel-name" {
			t.Errorf("expected name=updated-channel-name, got %v", m["name"])
		}

		t.Logf("PASS: PUT /alerts/channels/%s → 200, name=%v", chanID, m["name"])
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		b, _ := json.Marshal(map[string]any{"type": "email", "name": "x", "config": map[string]any{}})
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/channels/nonexistent-id", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT nonexistent channel: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /alerts/channels/nonexistent → 404")
	})

	t.Run("Update_BadJSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/alerts/channels/"+chanID,
			strings.NewReader(`{bad`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT bad json channel: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /alerts/channels/%s (bad JSON) → 400", chanID)
	})

	// Seed a second channel to delete.
	deleteChanID := createEmailChannel(t, ts.URL, token, "delete-target-channel")

	t.Run("Delete_HappyPath", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/channels/"+deleteChanID, nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE /alerts/channels/%s: %v", deleteChanID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /alerts/channels/%s → 204", deleteChanID)
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/alerts/channels/nonexistent-ch", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE nonexistent channel: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for nonexistent delete, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /alerts/channels/nonexistent → 404")
	})
}

// ─── Sources: List / Update / Delete ─────────────────────────────────────────

// TestWO4_Sources_ListUpdateDelete covers GET, PUT, DELETE /admin/sources/{sourceId}.
func TestWO4_Sources_ListUpdateDelete(t *testing.T) {
	// Use Business tier to avoid the 1-node free-tier limit.
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	t.Run("List_Empty", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/sources", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /admin/sources: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so Decode below sees a fresh body.
		req2, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/sources", nil)
		req2.Header.Set("Authorization", authHeader(token))
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if _, ok := m["items"]; !ok {
			t.Error("expected items in response")
		}

		t.Logf("PASS: GET /admin/sources → 200")
	})

	// Seed a source.
	srcID := createSource(t, ts.URL, token, "update-src")

	t.Run("List_WithItems", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/sources", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /admin/sources: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		items, _ := m["items"].([]any)
		if len(items) == 0 {
			t.Error("expected at least 1 item after creating a source")
		}
		t.Logf("PASS: GET /admin/sources → 200, %d items", len(items))
	})

	t.Run("Update_HappyPath", func(t *testing.T) {
		updateBody := map[string]any{
			"name":     "updated-src-name",
			"type":     "rest_poll",
			"rest_url": "http://127.0.0.1:5081",
		}
		b, _ := json.Marshal(updateBody)

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/sources/"+srcID, bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /admin/sources/%s: %v", srcID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so Decode below sees a fresh body.
		req2, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/sources/"+srcID, bytes.NewReader(b))
		req2.Header.Set("Authorization", authHeader(token))
		req2.Header.Set("Content-Type", "application/json")
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["name"] != "updated-src-name" {
			t.Errorf("expected name=updated-src-name, got %v", m["name"])
		}

		t.Logf("PASS: PUT /admin/sources/%s → 200", srcID)
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		b, _ := json.Marshal(map[string]any{"name": "x", "type": "rest_poll"})
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/sources/nonexistent", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT nonexistent source: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /admin/sources/nonexistent → 404")
	})

	t.Run("Update_BadJSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/sources/"+srcID,
			strings.NewReader(`{bad`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT bad JSON source: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /admin/sources/%s (bad JSON) → 400", srcID)
	})

	// Seed a second source to delete.
	deleteSrcID := createSource(t, ts.URL, token, "delete-src")

	t.Run("Delete_HappyPath", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/sources/"+deleteSrcID, nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE /admin/sources/%s: %v", deleteSrcID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /admin/sources/%s → 204", deleteSrcID)
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/sources/nonexistent-src", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE nonexistent source: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /admin/sources/nonexistent → 404")
	})
}

// ─── License: Activate ────────────────────────────────────────────────────────

// TestWO4_LicenseActivate covers PUT /admin/license.
func TestWO4_LicenseActivate(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("BadKey_Returns422", func(t *testing.T) {
		body := map[string]any{"key": "totally-invalid-license-key"}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/license", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /admin/license: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnprocessableEntity {
			bd, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 422 for invalid key, got %d: %s", resp.StatusCode, bd)
		}

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["code"] != "INVALID_LICENSE" {
			t.Errorf("expected code=INVALID_LICENSE, got %v", m["code"])
		}
		t.Logf("PASS: PUT /admin/license (bad key) → 422 INVALID_LICENSE")
	})

	t.Run("EmptyBody_Returns400", func(t *testing.T) {
		// Empty body: missing key field.
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/license",
			strings.NewReader(`{}`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /admin/license (empty key): %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			bd, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 400 for empty key, got %d: %s", resp.StatusCode, bd)
		}
		t.Logf("PASS: PUT /admin/license (empty key) → 400")
	})

	t.Run("ValidKey_Returns200", func(t *testing.T) {
		// makeTestEnterpriseLicense sets PULSE_LICENSE_PUBKEY BEFORE we create the server,
		// so the license.Manager picks up the test public key at construction time.
		licKey, licCleanup := makeTestEnterpriseLicense(t)
		defer licCleanup()

		// Create a fresh server that uses the test pub key (it's set in env now).
		freshTS, freshToken, freshCleanup := setupTestServer(t)
		defer freshCleanup()

		body := map[string]any{"key": licKey}
		b, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPut, freshTS.URL+"/api/v1/admin/license", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(freshToken))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /admin/license (valid key): %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200 for valid license key, got %d: %s", resp.StatusCode, bd)
		}

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["tier"] != "enterprise" {
			t.Errorf("expected tier=enterprise after activation, got %v", m["tier"])
		}
		t.Logf("PASS: PUT /admin/license (valid key) → 200, tier=%v", m["tier"])
	})
}

// ─── Users: List / Update / Delete ───────────────────────────────────────────

// TestWO4_Users_ListUpdateDelete covers GET, PUT, DELETE /admin/users and /admin/users/{userId}.
func TestWO4_Users_ListUpdateDelete(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	t.Run("List_Initially", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/users", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /admin/users: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so Decode below sees a fresh body.
		req2, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		req2.Header.Set("Authorization", authHeader(token))
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if _, ok := m["items"]; !ok {
			t.Error("expected items field in users list response")
		}

		t.Logf("PASS: GET /admin/users → 200")
	})

	// Create a user to update/delete.
	createBody := map[string]any{
		"username": "wo4-test-user",
		"role":     "viewer",
		"password": "testpassword123",
	}
	b, _ := json.Marshal(createBody)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/users", bytes.NewReader(b))
	createReq.Header.Set("Authorization", authHeader(token))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create user: expected 201, got %d: %s", createResp.StatusCode, bd)
	}
	var createdUser map[string]any
	json.NewDecoder(createResp.Body).Decode(&createdUser)
	userID, _ := createdUser["id"].(string)
	if userID == "" {
		t.Fatal("create user: empty id")
	}

	t.Run("List_WithUser", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/users", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /admin/users: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		items, _ := m["items"].([]any)
		if len(items) == 0 {
			t.Error("expected at least 1 user after create")
		}
		t.Logf("PASS: GET /admin/users → 200, %d items", len(items))
	})

	t.Run("Update_HappyPath", func(t *testing.T) {
		updateBody := map[string]any{
			"username": "wo4-test-user-updated",
			"role":     "admin",
		}
		ub, _ := json.Marshal(updateBody)

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/users/"+userID, bytes.NewReader(ub))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /admin/users/%s: %v", userID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["role"] != "admin" {
			t.Errorf("expected role=admin after update, got %v", m["role"])
		}
		t.Logf("PASS: PUT /admin/users/%s → 200, role=%v", userID, m["role"])
	})

	t.Run("Update_BadJSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/users/"+userID,
			strings.NewReader(`{bad`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT bad JSON user: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for bad JSON, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /admin/users/%s (bad JSON) → 400", userID)
	})

	// Seed another user specifically for delete.
	createDelBody := map[string]any{"username": "wo4-delete-user", "role": "viewer", "password": "x"}
	cdb, _ := json.Marshal(createDelBody)
	cdr, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/users", bytes.NewReader(cdb))
	cdr.Header.Set("Authorization", authHeader(token))
	cdr.Header.Set("Content-Type", "application/json")
	cdresp, err := http.DefaultClient.Do(cdr)
	if err != nil {
		t.Fatalf("create delete-user: %v", err)
	}
	defer cdresp.Body.Close()
	var delUser map[string]any
	json.NewDecoder(cdresp.Body).Decode(&delUser)
	delUserID, _ := delUser["id"].(string)

	t.Run("Delete_HappyPath", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/users/"+delUserID, nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE /admin/users/%s: %v", delUserID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /admin/users/%s → 204", delUserID)
	})
}

// ─── Report Schedules CRUD (Business tier) ────────────────────────────────────

// TestWO4_ReportSchedules_CRUD covers GET/POST/PUT/DELETE /reports/schedules.
// Business tier required; setupBusinessServer is used.
func TestWO4_ReportSchedules_CRUD(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	t.Run("FreeTier_Blocked", func(t *testing.T) {
		// Verify that free tier returns 403 for all schedule endpoints.
		freeTS, freeTok, freeCleanup := setupTestServer(t)
		defer freeCleanup()

		endpoints := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/v1/reports/schedules"},
			{http.MethodPost, "/api/v1/reports/schedules"},
		}
		for _, ep := range endpoints {
			req, _ := http.NewRequest(ep.method, freeTS.URL+ep.path, strings.NewReader(`{"cron":"0 9 * * 1","format":"csv"}`))
			req.Header.Set("Authorization", authHeader(freeTok))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", ep.method, ep.path, err)
			}
			bd, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("expected 403 for free tier %s %s, got %d: %s",
					ep.method, ep.path, resp.StatusCode, bd)
			} else {
				t.Logf("PASS: %s %s → 403 LICENSE_REQUIRED (free tier)", ep.method, ep.path)
			}
		}
	})

	t.Run("List_Empty", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/schedules", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /reports/schedules: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first: it reads body and resets resp.Body so Decode below works.
		req2, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/schedules", nil)
		req2.Header.Set("Authorization", authHeader(token))
		conformCheck(t, doc, req2, resp)

		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if _, ok := m["items"]; !ok {
			t.Error("expected items field")
		}

		t.Logf("PASS: GET /reports/schedules (business) → 200")
	})

	t.Run("Create_InvalidFormat", func(t *testing.T) {
		body := map[string]any{
			"cron":   "0 9 * * 1",
			"format": "docx", // invalid — only csv/pdf
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /reports/schedules (invalid format): %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnprocessableEntity {
			bd, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 422 for invalid format, got %d: %s", resp.StatusCode, bd)
		}
		t.Logf("PASS: POST /reports/schedules (invalid format) → 422")
	})

	t.Run("Create_MissingCron", func(t *testing.T) {
		body := map[string]any{"format": "csv"} // cron missing
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /reports/schedules (missing cron): %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnprocessableEntity {
			bd, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 422 for missing cron, got %d: %s", resp.StatusCode, bd)
		}
		t.Logf("PASS: POST /reports/schedules (missing cron) → 422")
	})

	t.Run("Create_BadJSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules",
			strings.NewReader(`{bad`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /reports/schedules (bad JSON): %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("PASS: POST /reports/schedules (bad JSON) → 400")
	})

	// Create a valid schedule for subsequent tests.
	var scheduleID string
	t.Run("Create_HappyPath", func(t *testing.T) {
		body := map[string]any{
			"cron":   "0 9 * * 1",
			"format": "csv",
			"scope":  map[string]any{"app": "live"},
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /reports/schedules: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so it can read the body before we consume it.
		req2, _ := http.NewRequest(http.MethodPost, "/api/v1/reports/schedules", bytes.NewReader(b))
		req2.Header.Set("Authorization", authHeader(token))
		req2.Header.Set("Content-Type", "application/json")
		conformCheck(t, doc, req2, resp)

		// Decode after conformCheck (which resets resp.Body).
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		scheduleID, _ = m["id"].(string)
		if scheduleID == "" {
			t.Fatal("expected non-empty schedule id")
		}
		if m["cron"] != "0 9 * * 1" {
			t.Errorf("expected cron=0 9 * * 1, got %v", m["cron"])
		}
		if m["format"] != "csv" {
			t.Errorf("expected format=csv, got %v", m["format"])
		}

		t.Logf("PASS: POST /reports/schedules → 201, id=%v", scheduleID)
	})

	if scheduleID == "" {
		t.Fatal("create schedule failed; subsequent subtests cannot run")
	}

	t.Run("Update_HappyPath", func(t *testing.T) {
		body := map[string]any{
			"cron":   "0 10 * * 2",
			"format": "pdf",
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/reports/schedules/"+scheduleID, bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT /reports/schedules/%s: %v", scheduleID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first so the body is available for both validation and decode.
		req2, _ := http.NewRequest(http.MethodPut, "/api/v1/reports/schedules/"+scheduleID, bytes.NewReader(b))
		req2.Header.Set("Authorization", authHeader(token))
		req2.Header.Set("Content-Type", "application/json")
		conformCheck(t, doc, req2, resp)

		// Decode after conformCheck (body was reset).
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		if m["format"] != "pdf" {
			t.Errorf("expected format=pdf after update, got %v", m["format"])
		}

		t.Logf("PASS: PUT /reports/schedules/%s → 200", scheduleID)
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		b, _ := json.Marshal(map[string]any{"cron": "0 9 * * 1", "format": "csv"})
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/reports/schedules/nonexistent", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT nonexistent schedule: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /reports/schedules/nonexistent → 404")
	})

	t.Run("Update_BadJSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/reports/schedules/"+scheduleID,
			strings.NewReader(`{bad`))
		req.Header.Set("Authorization", authHeader(token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT bad JSON schedule: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		t.Logf("PASS: PUT /reports/schedules/%s (bad JSON) → 400", scheduleID)
	})

	t.Run("List_AfterCreate", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/schedules", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /reports/schedules: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		items, _ := m["items"].([]any)
		if len(items) == 0 {
			t.Error("expected at least 1 schedule after create")
		}
		t.Logf("PASS: GET /reports/schedules → 200, %d items", len(items))
	})

	t.Run("Delete_HappyPath", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/reports/schedules/"+scheduleID, nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE /reports/schedules/%s: %v", scheduleID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /reports/schedules/%s → 204", scheduleID)
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/reports/schedules/nonexistent", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE nonexistent schedule: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		t.Logf("PASS: DELETE /reports/schedules/nonexistent → 404")
	})
}

// ─── Bootstrap (bootstrapIfFirstRun) ─────────────────────────────────────────

// TestWO4_BootstrapFirstRun verifies that Start() on a store with 0 tokens
// generates an admin token and prints to stderr.
func TestWO4_BootstrapFirstRun(t *testing.T) {
	ctx := context.Background()

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Fatalf("meta DDL not found: %v", err)
	}

	// Build a fresh store with NO pre-created tokens.
	store, err := meta.New(ctx, "sqlite", ":memory:", "bootstrap-test-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	// Use port :0 so it listens on a random free port.
	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)

	// Start triggers bootstrapIfFirstRun — should create an admin token.
	startCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := srv.Start(startCtx); err != nil {
		t.Fatalf("srv.Start: %v", err)
	}
	defer srv.Stop()

	// Verify a token was created (count should be 1 now).
	tokens, err := store.ListTokens(ctx, "api", 0, "")
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("expected exactly 1 bootstrap token, got %d", len(tokens))
	} else {
		tok := tokens[0]
		if tok.Kind != "api" {
			t.Errorf("expected kind=api, got %q", tok.Kind)
		}
		if !containsScope(tok.Scopes, "admin") {
			t.Errorf("expected admin scope, got %v", tok.Scopes)
		}
		t.Logf("PASS: bootstrapIfFirstRun created token kind=%q name=%q scopes=%v",
			tok.Kind, tok.Name, tok.Scopes)
	}
}

// containsScope returns true if scopes contains target.
func containsScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

// TestWO4_BootstrapNoOp verifies Start() does NOT create a token if one exists.
func TestWO4_BootstrapNoOp(t *testing.T) {
	ctx := context.Background()

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Fatalf("meta DDL not found: %v", err)
	}

	store, err := meta.New(ctx, "sqlite", ":memory:", "bootstrap-noop-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	// Pre-create one token (simulating non-first-run).
	tokenHash := hashToken("plt_existing_token_abcdef")
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "existing",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)

	startCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := srv.Start(startCtx); err != nil {
		t.Fatalf("srv.Start: %v", err)
	}
	defer srv.Stop()

	// Token count must stay at 1 (bootstrap must not create another).
	tokens, err := store.ListTokens(ctx, "api", 0, "")
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("expected exactly 1 token (no new bootstrap), got %d", len(tokens))
	} else {
		t.Logf("PASS: bootstrapIfFirstRun was no-op when tokens already exist")
	}
}

// ─── Probe Results (Enterprise tier) ─────────────────────────────────────────

// TestWO4_ProbeResults covers GET /probes/{probeId}/results (Enterprise tier).
func TestWO4_ProbeResults(t *testing.T) {
	ts, token, ms, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	t.Run("NotFound", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/probes/nonexistent-probe/results", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /probes/nonexistent/results: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for nonexistent probe results, got %d", resp.StatusCode)
		}
		t.Logf("PASS: GET /probes/nonexistent/results → 404")
	})

	// Create a probe so we can fetch its results.
	ctx := context.Background()
	probe, err := ms.CreateProbe(ctx, meta.ProbeRow{
		Name:      "WO4 Test Probe",
		URL:       "http://example.com/live.m3u8",
		Protocol:  "hls",
		IntervalS: 60,
		TimeoutS:  10,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProbe: %v", err)
	}

	t.Run("HappyPath_EmptyResults", func(t *testing.T) {
		doc := openAPISpec(t)

		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/probes/"+probe.ID+"/results", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /probes/%s/results: %v", probe.ID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}

		// conformCheck first — reads body and resets resp.Body for the decode below.
		req2, _ := http.NewRequest(http.MethodGet, "/api/v1/probes/"+probe.ID+"/results", nil)
		req2.Header.Set("Authorization", authHeader(token))
		conformCheck(t, doc, req2, resp)

		// Decode after conformCheck (body was reset by conformCheck).
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		items, ok := m["items"].([]any)
		if !ok {
			t.Errorf("expected items array in response, got %T", m["items"])
		} else {
			t.Logf("PASS: GET /probes/%s/results → 200, %d items", probe.ID, len(items))
		}
	})

	t.Run("WithLimitParam", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet,
			ts.URL+"/api/v1/probes/"+probe.ID+"/results?limit=10", nil)
		req.Header.Set("Authorization", authHeader(token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /probes/%s/results?limit=10: %v", probe.ID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bd, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
		}
		t.Logf("PASS: GET /probes/%s/results?limit=10 → 200", probe.ID)
	})
}

// ─── Additional coverage: alert rule invalid body (422) ──────────────────────

// TestWO4_AlertRule_CreateInvalidBody covers the 422 path in handleCreateAlertRule.
func TestWO4_AlertRule_CreateInvalidBody(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	// Missing required fields.
	body := map[string]any{"threshold": 5.0}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/rules", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/rules (invalid): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		bd, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 422 for missing required fields, got %d: %s", resp.StatusCode, bd)
	}
	t.Logf("PASS: POST /alerts/rules (missing fields) → 422")
}

// ─── Additional coverage: report usage (business tier happy path) ─────────────

// TestWO4_ReportUsage_BusinessTier covers the happy path of handleReportUsage.
func TestWO4_ReportUsage_BusinessTier(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/usage", nil)
	req.Header.Set("Authorization", authHeader(token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /reports/usage: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	// conformCheck first — reads and resets resp.Body so Decode below works.
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/usage", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)

	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	if _, ok := m["rows"]; !ok {
		t.Error("expected rows field in usage report response")
	}

	t.Logf("PASS: GET /reports/usage (business) → 200")
}
