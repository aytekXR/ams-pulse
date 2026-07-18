// Package api_test — S89 contract-drift regression guards (D-151).
//
// These pin the two client/server contract mismatches an S89 adversarial sweep
// confirmed against the code (both were live drift, not style):
//   - handleTestSource emitted the failure detail under `message`, but the
//     AmsSourceStatus schema (and the web OnboardingWizard) use `error`.
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// createSourceAt creates an AMS rest_poll source with a caller-chosen rest_url and
// returns its ID. Unlike createSource (fixed URL), it lets a test point the source
// at a deterministically-closed port so the connectivity test fails predictably.
func createSourceAt(t *testing.T, baseURL, token, name, restURL string) string {
	t.Helper()
	body := map[string]any{"name": name, "type": "rest_poll", "rest_url": restURL}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/sources", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createSourceAt POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("createSourceAt: expected 201, got %d: %s", resp.StatusCode, bd)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatal("createSourceAt: empty id in response")
	}
	return id
}

// TestS89_TestSource_ErrorDetail_UsesContractKey guards the S89/D-151 fix:
// POST /admin/sources/{id}/test must surface the failure reason under the OpenAPI
// AmsSourceStatus `error` key — never the undocumented `message` key the handler
// used to emit. The web OnboardingWizard reads `status.error`, so emitting `message`
// made every failed connectivity test render the generic "Source unreachable"
// fallback instead of the real reason.
func TestS89_TestSource_ErrorDetail_UsesContractKey(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	// Point the source at the loopback discard port (:9) — nothing listens there,
	// so the test dial fails fast with a connection error → reachable=false with a
	// human-readable detail.
	srcID := createSourceAt(t, ts.URL, token, "s89-drift-src", "http://127.0.0.1:9")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources/"+srcID+"/test", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bd)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode test result: %v", err)
	}

	if reachable, _ := result["reachable"].(bool); reachable {
		t.Fatalf("expected reachable=false for a closed port, got result=%v", result)
	}

	// The bug: the detail was carried under `message`, absent from AmsSourceStatus.
	// The contract key `error` must carry it (this is the mutation target — reverting
	// the handler to `message` drops the `error` key and fails here).
	errDetail, ok := result["error"].(string)
	if !ok || errDetail == "" {
		t.Errorf("AmsSourceStatus.error missing/empty on failure; got keys=%v "+
			"(regression: failure detail emitted under the wrong key)", mapKeys(result))
	}
	// And the undocumented `message` key must not return.
	if _, hasMessage := result["message"]; hasMessage {
		t.Errorf("handler re-introduced the undocumented `message` key (not in AmsSourceStatus); result=%v", result)
	}
}
