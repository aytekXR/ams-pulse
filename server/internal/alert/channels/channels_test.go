package channels_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
)

// testPayload returns a minimal alert notification JSON payload for tests.
func testPayload(state string) []byte {
	n := map[string]any{
		"version":   1,
		"alert_id":  "test-alert-id-123",
		"rule_id":   "rule-456",
		"state":     state,
		"severity":  "warning",
		"ts":        int64(1700000000000),
		"title":     "Test alert: stream offline",
		"metric":    "stream_offline",
		"value":     0.0,
		"threshold": 0.0,
		"scope": map[string]any{
			"stream_id": "auction-stream",
			"app":       "live",
		},
		"test":           true,
		"cooldown_until": nil,
		"group_key":      "auction-stream",
		"dashboard_url":  "http://localhost:8090/alerts",
	}
	b, _ := json.Marshal(n)
	return b
}

// ─── Noop channel ─────────────────────────────────────────────────────────────

func TestNoopChannel_Send(t *testing.T) {
	noop := &channels.NoopChannel{}
	payload := testPayload("firing")
	if err := noop.Send(context.Background(), payload); err != nil {
		t.Fatalf("noop.Send: %v", err)
	}
	if len(noop.Received) != 1 {
		t.Errorf("expected 1 received payload, got %d", len(noop.Received))
	}
	t.Logf("PASS: NoopChannel.Send records payload, len=%d", len(noop.Received))
}

// ─── Slack channel ────────────────────────────────────────────────────────────

func TestSlackChannel_Send_HTTPTest(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ch := channels.NewSlackChannel(channels.SlackConfig{
		WebhookURL: srv.URL,
		Channel:    "#alerts",
	})
	payload := testPayload("firing")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("slack.Send: %v", err)
	}
	if len(received) == 0 {
		t.Error("expected Slack webhook to receive a POST body")
	}
	var body map[string]any
	if err := json.Unmarshal(received, &body); err != nil {
		t.Fatal("slack body not JSON:", err)
	}
	if _, ok := body["text"]; !ok {
		t.Error("expected 'text' field in Slack message")
	}
	t.Logf("PASS: Slack channel sent POST to webhook, body=%s", strings.TrimSpace(string(received))[:80])
}

func TestSlackChannel_TestFire(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ch := channels.NewSlackChannel(channels.SlackConfig{WebhookURL: srv.URL})
	// Use TestFireChannel from alert evaluator-level
	payload := channels.BuildTestPayload("rule-test-fire")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("slack test fire: %v", err)
	}
	if len(received) == 0 {
		t.Error("expected test-fire to send to Slack")
	}
	t.Logf("PASS: Slack test-fire delivered to fake webhook")
}

// ─── Telegram channel ─────────────────────────────────────────────────────────

func TestTelegramChannel_Send_HTTPTest(t *testing.T) {
	var receivedBody []byte
	var receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 42}})
	}))
	defer srv.Close()

	// Override the Telegram API URL via a custom transport (test-only):
	// We create a channel with fake token and patch its internal URL via the
	// test server's URL as bot token prefix.
	// Since TelegramChannel uses the BotToken in the URL, we craft the URL.
	// For testing, we intercept by using a custom round-tripper is complex.
	// Instead, use a test approach: set BotToken to "fake" and verify the URL structure.

	// Build a testable Telegram channel by pointing to our test server.
	// The Telegram channel POSTs to https://api.telegram.org/bot<token>/sendMessage.
	// We can't easily override this without changing the struct. So we use a test
	// that validates the channel is built correctly, and use a fake HTTP client.
	ch := channels.NewTelegramChannel(channels.TelegramConfig{
		BotToken: "123456789:FAKE_TOKEN_FOR_TEST",
		ChatID:   "-100123456789",
	})
	// The channel will fail to connect to the real Telegram API in tests.
	// That's expected — we verify the channel structure is correct.
	// For a proper integration-like test, we set the client's transport.
	// Use the provided test server approach:
	chTest := channels.NewTelegramChannelWithURL(channels.TelegramConfig{
		BotToken: "fake-token",
		ChatID:   "-100123",
	}, srv.URL+"/bot%s/sendMessage")

	payload := testPayload("firing")
	if err := chTest.Send(context.Background(), payload); err != nil {
		t.Fatalf("telegram.Send: %v", err)
	}
	_ = receivedPath
	if len(receivedBody) == 0 {
		t.Error("expected Telegram channel to POST a body")
	}
	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatal("telegram body not JSON:", err)
	}
	if _, ok := body["text"]; !ok {
		t.Error("expected 'text' field in Telegram message")
	}
	if _, ok := body["chat_id"]; !ok {
		t.Error("expected 'chat_id' field in Telegram message")
	}
	t.Logf("PASS: Telegram channel POSTed to fake API, path=%s text_len=%d",
		receivedPath, len(body["text"].(string)))
	_ = ch // suppress unused variable warning
}

// ─── PagerDuty channel ────────────────────────────────────────────────────────

func TestPagerDutyChannel_Trigger_HTTPTest(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"status":    "success",
			"message":   "Event processed",
			"dedup_key": "alert-id-123",
		})
	}))
	defer srv.Close()

	ch := channels.NewPagerDutyChannel(channels.PagerDutyConfig{
		RoutingKey: "fake-routing-key-32chars000000000",
	})
	ch.SetAPIURL(srv.URL)

	payload := testPayload("firing")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("pagerduty.Send (trigger): %v", err)
	}
	if len(receivedBody) == 0 {
		t.Error("expected PagerDuty channel to POST a body")
	}
	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatal("pagerduty body not JSON:", err)
	}
	if body["event_action"] != "trigger" {
		t.Errorf("expected event_action=trigger for firing, got %v", body["event_action"])
	}
	if body["routing_key"] != "fake-routing-key-32chars000000000" {
		t.Errorf("expected routing_key in body, got %v", body["routing_key"])
	}
	t.Logf("PASS: PagerDuty trigger: event_action=%v dedup_key=%v", body["event_action"], body["dedup_key"])
}

func TestPagerDutyChannel_Resolve_HTTPTest(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"status": "success"})
	}))
	defer srv.Close()

	ch := channels.NewPagerDutyChannel(channels.PagerDutyConfig{
		RoutingKey: "fake-routing-key",
	})
	ch.SetAPIURL(srv.URL)

	payload := testPayload("resolved")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("pagerduty.Send (resolve): %v", err)
	}
	var body map[string]any
	_ = json.Unmarshal(receivedBody, &body)
	if body["event_action"] != "resolve" {
		t.Errorf("expected event_action=resolve for resolved, got %v", body["event_action"])
	}
	t.Logf("PASS: PagerDuty resolve: event_action=%v", body["event_action"])
}

// ─── Webhook channel ─────────────────────────────────────────────────────────

func TestWebhookChannel_Send_WithHMAC(t *testing.T) {
	secret := "my-webhook-secret-abc"
	var receivedBody []byte
	var receivedSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedSig = r.Header.Get("X-Pulse-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := channels.NewWebhookChannel(channels.WebhookConfig{
		URL:    srv.URL,
		Secret: secret,
	})

	payload := testPayload("firing")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("webhook.Send: %v", err)
	}

	if len(receivedBody) == 0 {
		t.Error("expected webhook to receive a body")
	}
	if receivedSig == "" {
		t.Error("expected X-Pulse-Signature header")
	}
	if !strings.HasPrefix(receivedSig, "sha256=") {
		t.Errorf("signature must start with 'sha256=', got %q", receivedSig)
	}

	// Verify HMAC matches.
	valid := channels.VerifyWebhookSignature(secret, receivedBody, receivedSig)
	if !valid {
		t.Errorf("HMAC signature verification FAILED for received payload")
	}
	t.Logf("PASS: webhook HMAC verified: sig=%s (len=%d)", receivedSig[:20]+"...", len(receivedSig))
}

func TestWebhookChannel_Send_NoSecret(t *testing.T) {
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Pulse-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := channels.NewWebhookChannel(channels.WebhookConfig{
		URL:    srv.URL,
		Secret: "", // no secret
	})

	payload := testPayload("firing")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("webhook.Send: %v", err)
	}
	if receivedSig != "" {
		t.Errorf("expected no signature header when secret is empty, got %q", receivedSig)
	}
	t.Logf("PASS: webhook without secret: no X-Pulse-Signature header sent")
}

func TestWebhookChannel_HMAC_Verification(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test":"data","value":42}`)

	// Generate the correct signature using a real webhook send to a test server.
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Pulse-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := channels.NewWebhookChannel(channels.WebhookConfig{URL: srv.URL, Secret: secret})
	_ = ch.Send(context.Background(), payload)
	sig := receivedSig

	// Correct signature → true.
	if !channels.VerifyWebhookSignature(secret, payload, sig) {
		t.Error("expected valid signature to verify")
	}

	// Tampered payload → false.
	tampered := []byte(`{"test":"TAMPERED","value":42}`)
	if channels.VerifyWebhookSignature(secret, tampered, sig) {
		t.Error("expected tampered payload to fail verification")
	}

	// Wrong secret → false.
	if channels.VerifyWebhookSignature("wrong-secret", payload, sig) {
		t.Error("expected wrong secret to fail verification")
	}

	// Malformed signature → false.
	if channels.VerifyWebhookSignature(secret, payload, "not-a-sig") {
		t.Error("expected malformed signature to fail verification")
	}

	t.Logf("PASS: HMAC verification: correct=true, tampered=false, wrong-secret=false, malformed=false")
}

func TestWebhookChannel_TestFire(t *testing.T) {
	secret := "test-secret"
	var receivedBody []byte
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedSig = r.Header.Get("X-Pulse-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := channels.NewWebhookChannel(channels.WebhookConfig{URL: srv.URL, Secret: secret})
	payload := channels.BuildTestPayload("rule-webhook-test")
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("webhook test-fire: %v", err)
	}
	if !channels.VerifyWebhookSignature(secret, receivedBody, receivedSig) {
		t.Error("test-fire HMAC verification failed")
	}
	t.Logf("PASS: webhook test-fire with HMAC verified")
}

// computeTestHMAC is not needed — HMAC tests use channels.VerifyWebhookSignature
// as the authoritative checker together with the channel's own Send to generate the sig.
// The TestWebhookChannel_HMAC_Verification test uses a real webhook Send call to get
// a valid sig, then tests that VerifyWebhookSignature accepts/rejects correctly.
