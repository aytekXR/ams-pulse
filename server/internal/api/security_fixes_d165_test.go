// Package api_test — D-165 security-fix tests covering:
//
//   - A: SOURCE-TEST SSRF — DialControl blocks link-local/IMDS addresses; loopback allowed
//   - B: NIL-SCOPE TOKEN = SILENT ADMIN — scopeless api token defaults to "read"
//   - C: MAIN-PORT BEACON VALIDATION SKIP — /ingest/beacon now validates like the beacon port
package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── A: Source-test SSRF ──────────────────────────────────────────────────────

// TestSourceTest_SSRF_LinkLocalRefused verifies that calling the source-test
// endpoint for a source whose rest_url is a link-local (IMDS) address results
// in reachable=false rather than a successful outbound connection.
// This exercises the IP-literal boundary check (ssrfguard.IsDenied on the parsed
// hostname) and the DialControl guard on the transport.
func TestSourceTest_SSRF_LinkLocalRefused(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a source whose rest_url is the AWS IMDSv4 address.
	// amsSourceFromAPI only rejects non-http/https schemes; it accepts IP literals,
	// so this create succeeds and exercises the guard at test time.
	createBody, _ := json.Marshal(map[string]any{
		"name":     "imds-test-source",
		"type":     "rest",
		"rest_url": "http://169.254.169.254",
	})
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources",
		bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create source: expected 201, got %d: %s", createResp.StatusCode, raw)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	sourceID, _ := created["id"].(string)
	if sourceID == "" {
		t.Fatal("no id in create response")
	}

	// Trigger the connectivity test.
	testReq, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/admin/sources/"+sourceID+"/test", nil)
	testReq.Header.Set("Authorization", "Bearer "+adminToken)
	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST /test: %v", err)
	}
	defer testResp.Body.Close()

	if testResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(testResp.Body)
		t.Fatalf("test endpoint returned %d (want 200): %s", testResp.StatusCode, raw)
	}
	var result map[string]any
	json.NewDecoder(testResp.Body).Decode(&result)

	// The test must report reachable=false — not a successful connection to 169.254.169.254.
	if reachable, _ := result["reachable"].(bool); reachable {
		t.Errorf("expected reachable=false for IMDS address, got result=%v", result)
	}
}

// TestSourceTest_Loopback_Allowed verifies that the ssrfguard installation does
// NOT block a source whose rest_url points at the test's own loopback server.
// Loopback must remain allowed because AMS is often on 127.x.x.x in dev (B4/A6 ruling).
func TestSourceTest_Loopback_Allowed(t *testing.T) {
	// Spin up a lightweight server that mimics an AMS /rest/v2/version response.
	fakAMS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"version":"3.0.3"}`)
	}))
	defer fakAMS.Close()

	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	createBody, _ := json.Marshal(map[string]any{
		"name":     "loopback-ams",
		"type":     "rest",
		"rest_url": fakAMS.URL, // 127.0.0.1:<port>
	})
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources",
		bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create source: expected 201, got %d: %s", createResp.StatusCode, raw)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	sourceID, _ := created["id"].(string)
	if sourceID == "" {
		t.Fatal("no id in create response")
	}

	testReq, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/admin/sources/"+sourceID+"/test", nil)
	testReq.Header.Set("Authorization", "Bearer "+adminToken)
	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST /test: %v", err)
	}
	defer testResp.Body.Close()
	if testResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(testResp.Body)
		t.Fatalf("test endpoint returned %d (want 200): %s", testResp.StatusCode, raw)
	}
	var result map[string]any
	json.NewDecoder(testResp.Body).Decode(&result)

	// Loopback is in the ALLOWED range — the connection to fakAMS must succeed.
	reachable, _ := result["reachable"].(bool)
	if !reachable {
		t.Errorf("expected reachable=true for loopback AMS, got result=%v (loopback must be allowed)", result)
	}
}

// ─── B: Nil-scope token = silent admin ────────────────────────────────────────

// TestCreateToken_NoScopes_DefaultsToRead verifies that POSTing an api token
// without a scopes field stores scope "read" and that the token cannot perform
// write operations (e.g. delete a user).
func TestCreateToken_NoScopes_DefaultsToRead(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a token without specifying scopes.
	createBody, _ := json.Marshal(map[string]any{
		"kind": "api",
		"name": "no-scopes-token",
	})
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /admin/tokens: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201, got %d: %s", createResp.StatusCode, raw)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)

	// The stored scopes must be ["read"], not nil/empty.
	scopesRaw, ok := created["scopes"]
	if !ok {
		t.Fatal("response has no 'scopes' field")
	}
	scopes, _ := scopesRaw.([]any)
	if len(scopes) != 1 {
		t.Errorf("expected 1 scope, got %v", scopesRaw)
	} else if scopes[0] != "read" {
		t.Errorf("expected scope 'read', got %v", scopes[0])
	}

	// Recover the raw token from the create response (only time it is visible).
	rawToken, _ := created["token"].(string)
	if rawToken == "" {
		t.Fatal("no raw token in response")
	}

	// Attempt a write operation (create another token) using the read-only token.
	writeBody, _ := json.Marshal(map[string]any{"kind": "api", "name": "should-fail"})
	writeReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(writeBody))
	writeReq.Header.Set("Authorization", "Bearer "+rawToken)
	writeReq.Header.Set("Content-Type", "application/json")
	writeResp, err := http.DefaultClient.Do(writeReq)
	if err != nil {
		t.Fatalf("POST /admin/tokens with read-only token: %v", err)
	}
	defer writeResp.Body.Close()
	if writeResp.StatusCode != http.StatusForbidden {
		raw, _ := io.ReadAll(writeResp.Body)
		t.Errorf("expected 403 for write with read-only token, got %d: %s", writeResp.StatusCode, raw)
	}
}

// TestCreateToken_ExplicitAdminScope_Works verifies that explicitly requesting
// scope "admin" still produces a fully privileged token.
func TestCreateToken_ExplicitAdminScope_Works(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	createBody, _ := json.Marshal(map[string]any{
		"kind":   "api",
		"name":   "explicit-admin-token",
		"scopes": []string{"admin"},
	})
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /admin/tokens: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201, got %d: %s", createResp.StatusCode, raw)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)

	rawToken, _ := created["token"].(string)
	if rawToken == "" {
		t.Fatal("no raw token in response")
	}

	// The admin-scoped token must be able to create another token (write operation).
	writeBody, _ := json.Marshal(map[string]any{"kind": "api", "name": "created-by-admin"})
	writeReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(writeBody))
	writeReq.Header.Set("Authorization", "Bearer "+rawToken)
	writeReq.Header.Set("Content-Type", "application/json")
	writeResp, err := http.DefaultClient.Do(writeReq)
	if err != nil {
		t.Fatalf("POST /admin/tokens with admin token: %v", err)
	}
	defer writeResp.Body.Close()
	if writeResp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(writeResp.Body)
		t.Errorf("expected 201 for write with admin token, got %d: %s", writeResp.StatusCode, raw)
	}
}

// ─── C: Main-port beacon validation ──────────────────────────────────────────

// validIngestToken creates an ingest token in the given server's store and
// returns the raw token string.
func makeIngestToken(t *testing.T, ts *httptest.Server, adminToken string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"kind": "ingest", "name": "test-ingest"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create ingest token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 creating ingest token, got %d: %s", resp.StatusCode, raw)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	tok, _ := m["token"].(string)
	if tok == "" {
		t.Fatal("no token in create response")
	}
	return tok
}

// TestMainPortBeacon_InvalidBatch_Rejected verifies that the main-port
// /ingest/beacon route rejects a batch that fails schema validation
// (bad version, missing session_id/stream_id, unknown event type).
// Before D-165 this was silently accepted; now it must return 422 SCHEMA_ERROR.
// An enterprise license is required to reach validation (Pro+ license gate).
func TestMainPortBeacon_InvalidBatch_Rejected(t *testing.T) {
	ts, adminToken, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	ingestTok := makeIngestToken(t, ts, adminToken)

	// Invalid batch: version=2, no session_id/stream_id, bad event type.
	invalidBatch := map[string]any{
		"version":    2, // must be 1
		"session_id": "",
		"stream_id":  "",
		"events": []any{
			map[string]any{
				"type": "unknown_type",
				"ts":   1700000000000,
			},
		},
	}
	body, _ := json.Marshal(invalidBatch)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon",
		bytes.NewReader(body))
	req.Header.Set("X-Pulse-Ingest-Token", ingestTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		raw, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 422 for invalid batch, got %d: %s", resp.StatusCode, raw)
		return
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if code, _ := result["code"].(string); code != "SCHEMA_ERROR" {
		t.Errorf("expected code=SCHEMA_ERROR, got %q (result=%v)", code, result)
	}
}

// TestMainPortBeacon_ValidBatch_Accepted verifies that a well-formed batch
// passes schema validation and reaches the next processing stage.
// Uses enterprise license so the license gate does not intercept before validation.
func TestMainPortBeacon_ValidBatch_Accepted(t *testing.T) {
	ts, adminToken, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	ingestTok := makeIngestToken(t, ts, adminToken)

	validBatch := map[string]any{
		"version":    1,
		"session_id": "sess-1",
		"stream_id":  "stream-1",
		"app":        "live",
		"events": []any{
			map[string]any{
				"type": "session_start",
				"ts":   1700000000000,
				"data": map[string]any{},
			},
		},
	}
	body, _ := json.Marshal(validBatch)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon",
		bytes.NewReader(body))
	req.Header.Set("X-Pulse-Ingest-Token", ingestTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	// A valid batch must not be rejected by schema validation (422 would be wrong).
	if resp.StatusCode == http.StatusUnprocessableEntity {
		raw, _ := io.ReadAll(resp.Body)
		t.Errorf("valid batch was rejected by schema validation (want 202): %s", raw)
	}
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 202 Accepted for valid batch, got %d: %s", resp.StatusCode, raw)
	}
}
