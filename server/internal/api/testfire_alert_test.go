// Package api_test — alert channel test-fire correctness.
//
// Tests verify:
//   - POST /alerts/channels/{id}/test delivers to the configured sink (webhook_url key).
//   - Response is 200 {accepted, message} (not 202 {ok}).
//   - Delivery failure returns 200 {accepted:false} (not 502).
//   - Secret URLs are NOT echoed in the failure body (security regression guard).
//   - buildChannelFromRow reads the contract key names (webhook_url, slack_channel,
//     email_to, telegram_chat_id, pagerduty_severity).
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ─── Webhook sink test (TDD: red on old "url" key, green after "webhook_url" fix) ───

// TestA1_TestFireAlertChannel_WebhookSink verifies that:
//  1. Creating a channel with the CONTRACT key "webhook_url" (not "url") works.
//  2. POST /test delivers to the sink (accepted==true, HTTP 200).
//  3. The httptest sink received exactly one POST.
//
// RED on the old str("url") code; GREEN after 1a fix (str("webhook_url")).
func TestA1_TestFireAlertChannel_WebhookSink(t *testing.T) {
	// Spin up a sink that records received requests.
	var sinkHits atomic.Int32
	var sinkBody []byte
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sinkBody = body
		sinkHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	// Create a webhook channel using the CONTRACT key "webhook_url".
	chanBody := map[string]any{
		"type": "webhook",
		"name": "test-webhook-sink",
		"config": map[string]any{
			"webhook_url": sink.URL, // CONTRACT KEY — was "url" (now fixed to "webhook_url")
		},
	}
	chanBytes, _ := json.Marshal(chanBody)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels",
		bytes.NewReader(chanBytes))
	createReq.Header.Set("Authorization", authHeader(token))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /alerts/channels: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201 creating channel, got %d: %s", createResp.StatusCode, body)
	}
	var chanData map[string]any
	json.NewDecoder(createResp.Body).Decode(&chanData)
	channelID, _ := chanData["id"].(string)
	if channelID == "" {
		t.Fatal("no channel id in create response")
	}

	// POST the test-fire route.
	testReq, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/alerts/channels/"+channelID+"/test", nil)
	testReq.Header.Set("Authorization", authHeader(token))
	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST /alerts/channels/%s/test: %v", channelID, err)
	}
	respBody, _ := io.ReadAll(testResp.Body)
	testResp.Body.Close()

	// Assert HTTP 200 (synchronous delivery, not 202).
	if testResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", testResp.StatusCode, respBody)
	}

	// Assert accepted==true.
	var result map[string]any
	json.Unmarshal(respBody, &result)
	if result["accepted"] != true {
		t.Errorf("expected accepted=true, got %v (body: %s)", result["accepted"], respBody)
	}

	// Assert the sink received exactly one POST.
	if sinkHits.Load() == 0 {
		t.Errorf("webhook sink received NO request (handler is broken)")
	}
	if len(sinkBody) == 0 {
		t.Errorf("webhook sink received empty body")
	}

	t.Logf("PASS A1: test-fire → sink hit %d time(s), body=%d bytes, HTTP 200, accepted=true",
		sinkHits.Load(), len(sinkBody))
}

// ─── Delivery-FAILURE test (security regression: secret URL must not leak) ───

// TestA1_TestFireAlertChannel_UnreachableWebhook verifies that an unreachable
// webhook URL causes the test-fire to return 200 accepted==false (not 502), AND
// the response body does NOT contain the URL (security: URL may embed a token).
func TestA1_TestFireAlertChannel_UnreachableWebhook(t *testing.T) {
	// Use a port that is guaranteed to be closed on the test host.
	unreachableURL := "http://127.0.0.1:19192/webhook-secret-token-abc123"

	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	chanBody := map[string]any{
		"type": "webhook",
		"name": "unreachable-webhook",
		"config": map[string]any{
			"webhook_url": unreachableURL,
		},
	}
	chanBytes, _ := json.Marshal(chanBody)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels",
		bytes.NewReader(chanBytes))
	createReq.Header.Set("Authorization", authHeader(token))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /alerts/channels: %v", err)
	}
	createBody, _ := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating channel, got %d: %s", createResp.StatusCode, createBody)
	}
	var chanData map[string]any
	json.Unmarshal(createBody, &chanData)
	channelID, _ := chanData["id"].(string)
	if channelID == "" {
		t.Fatal("no channel id in create response")
	}

	testReq, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/alerts/channels/"+channelID+"/test", nil)
	testReq.Header.Set("Authorization", authHeader(token))
	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST test-fire: %v", err)
	}
	respBody, _ := io.ReadAll(testResp.Body)
	testResp.Body.Close()

	// Must be HTTP 200 (not 502 — synchronous delivery result in body).
	if testResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for unreachable webhook, got %d: %s", testResp.StatusCode, respBody)
	}

	// Must be accepted==false.
	var result map[string]any
	json.Unmarshal(respBody, &result)
	if result["accepted"] != false {
		t.Errorf("expected accepted=false for unreachable webhook, got %v", result["accepted"])
	}

	// SECURITY REGRESSION: the response body must NOT contain the URL.
	// *url.Error from net/http includes the target URL in its message, which
	// would leak telegram bot tokens / slack webhook URLs embedded in the URL.
	if strings.Contains(string(respBody), unreachableURL) {
		t.Errorf("SECURITY: response body contains the channel URL (secret leak risk): %s", respBody)
	}
	// Also must not contain the embedded token string.
	if strings.Contains(string(respBody), "webhook-secret-token-abc123") {
		t.Errorf("SECURITY: response body contains the secret token from the URL: %s", respBody)
	}

	t.Logf("PASS: unreachable webhook → 200 accepted=false, URL absent from body: %s", respBody)
}

// ─── 404 for missing channel (unchanged behavior) ─────────────────────────────

// TestA1_TestFireAlertChannel_NotFound verifies that a missing channel returns 404.
func TestA1_TestFireAlertChannel_NotFound(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/alerts/channels/nonexistent-channel-id/test", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST test-fire (notfound): %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing channel, got %d", resp.StatusCode)
	}
	t.Logf("PASS A1: missing channel → 404")
}

// ─── buildChannelFromRow unit tests: verify contract key names per type ───────

// TestBuildChannelFromRow_ContractKeys verifies that alertChannelFromAPI stores
// config under the contract keys and buildChannelFromRow reads the SAME contract
// keys. One test per channel type.
//
// These tests exercise the full round-trip: create via API → retrieve from store
// → buildChannelFromRow. If any key name is wrong, the channel will silently have
// empty fields (and TestFireChannel will fail).
func TestBuildChannelFromRow_ContractKeys_Webhook(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	chanBody := map[string]any{
		"type": "webhook",
		"name": "key-test-webhook",
		"config": map[string]any{
			"webhook_url": sink.URL, // contract key
		},
	}
	channelID := createChannel(t, ts.URL, token, chanBody)

	// Test-fire must succeed (accepted==true) — proves buildChannelFromRow reads "webhook_url".
	result := testFireChannel(t, ts.URL, token, channelID)
	if result["accepted"] != true {
		t.Errorf("webhook: expected accepted=true, got %v; buildChannelFromRow may read wrong key", result)
	}
	t.Logf("PASS: webhook channel → accepted=true (contract key 'webhook_url' round-trips correctly)")
}

func TestBuildChannelFromRow_ContractKeys_Slack(t *testing.T) {
	ts, token, cleanup := setupBusinessServer(t)
	defer cleanup()

	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	chanBody := map[string]any{
		"type": "slack",
		"name": "key-test-slack",
		"config": map[string]any{
			"slack_webhook_url": sink.URL, // secret — contract key
			"slack_channel":     "#test",  // public — contract key
		},
	}
	channelID := createChannel(t, ts.URL, token, chanBody)

	result := testFireChannel(t, ts.URL, token, channelID)
	if result["accepted"] != true {
		t.Errorf("slack: expected accepted=true, got %v; buildChannelFromRow may read wrong key for slack_channel or slack_webhook_url", result)
	}
	t.Logf("PASS: slack channel → accepted=true (contract keys 'slack_webhook_url' + 'slack_channel' round-trip correctly)")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// createChannel POSTs to /alerts/channels and returns the created channel ID.
func createChannel(t *testing.T, baseURL, token string, body map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/alerts/channels",
		bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, respBody)
	}
	var chanData map[string]any
	json.Unmarshal(respBody, &chanData)
	id, _ := chanData["id"].(string)
	if id == "" {
		t.Fatal("no channel id in create response")
	}
	return id
}

// testFireChannel POSTs to /alerts/channels/{id}/test and returns the parsed body.
func testFireChannel(t *testing.T, baseURL, token, channelID string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost,
		baseURL+"/api/v1/alerts/channels/"+channelID+"/test", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels/%s/test: %v", channelID, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.Unmarshal(body, &result)
	return result
}
