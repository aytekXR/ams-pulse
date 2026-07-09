// B7 TDD tests — amsSourceFromAPI / amsSourceToAPI for per-source webhook
// secret (D-062 WO-3). Written FIRST (red) before the implementation existed.
//
// Coverage:
//   - amsSourceFromAPI encrypts body["webhook_secret"] → row.WebhookSecretEnc
//   - amsSourceToAPI emits webhook_secret_set: true/false in the response body
//
// These are integration-style tests that go through the full HTTP stack (POST
// /admin/sources and GET /admin/sources/{id}) to stay consistent with the
// style used by wo4_handlers_test.go.
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestAMSSource_WebhookSecretSet_FromAPI verifies the full round-trip:
//  1. POST /admin/sources with webhook_secret in the body → 201
//  2. GET the created source → webhook_secret_set: true in response
//  3. webhook_secret itself is NOT echoed back (write-only)
func TestAMSSource_WebhookSecretSet_FromAPI(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create a source with a webhook_secret.
	body := map[string]any{
		"name":           "b7-webhook-src",
		"type":           "webhook",
		"webhook_secret": "super-secret-per-source",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /admin/sources: expected 201, got %d: %s", resp.StatusCode, bd)
	}

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	srcID, _ := created["id"].(string)
	if srcID == "" {
		t.Fatal("POST /admin/sources: no id in response")
	}

	// 2. The creation response should already include webhook_secret_set: true.
	wsSet, hasProp := created["webhook_secret_set"]
	if !hasProp {
		t.Fatalf("POST /admin/sources response missing webhook_secret_set field; got %v", created)
	}
	if wsSet != true {
		t.Errorf("webhook_secret_set: expected true (secret was provided), got %v", wsSet)
	}

	// 3. webhook_secret must NOT be echoed (write-only).
	if _, has := created["webhook_secret"]; has {
		t.Error("webhook_secret should not appear in response body (write-only)")
	}

	// 4. GET the source and confirm webhook_secret_set is stable.
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/sources", nil)
	req2.Header.Set("Authorization", authHeader(token))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("GET /admin/sources: %v", err)
	}
	defer resp2.Body.Close()
	var listResp map[string]any
	json.NewDecoder(resp2.Body).Decode(&listResp)
	items, _ := listResp["items"].([]any)
	var found map[string]any
	for _, item := range items {
		m, _ := item.(map[string]any)
		if m["id"] == srcID {
			found = m
			break
		}
	}
	if found == nil {
		t.Fatalf("GET /admin/sources: source %s not found in list", srcID)
	}
	if found["webhook_secret_set"] != true {
		t.Errorf("GET /admin/sources: webhook_secret_set should be true, got %v", found["webhook_secret_set"])
	}
}

// TestAMSSource_WebhookSecretNotSet_FromAPI verifies that a source created
// without webhook_secret has webhook_secret_set: false.
func TestAMSSource_WebhookSecretNotSet_FromAPI(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	body := map[string]any{
		"name": "b7-nosecret-src",
		"type": "webhook",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /admin/sources: expected 201, got %d: %s", resp.StatusCode, bd)
	}

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	if created["webhook_secret_set"] != false {
		t.Errorf("webhook_secret_set: expected false (no secret), got %v", created["webhook_secret_set"])
	}
}
