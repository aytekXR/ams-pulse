package channels

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookConfig is the configuration for a generic webhook notification channel.
// The payload is the raw alert-notification.schema.json JSON body, signed with
// HMAC-SHA256 in the X-Pulse-Signature header.
//
// Signature format (documented for downstream verification):
//
//	X-Pulse-Signature: sha256=<hex(HMAC-SHA256(secret, body))>
//
// Consumers must verify this signature before processing the webhook payload.
type WebhookConfig struct {
	// URL is the webhook endpoint to POST to.
	URL string
	// Secret is the shared HMAC-SHA256 signing secret. SENSITIVE: encrypted at rest.
	// If empty, no signature header is sent.
	Secret string
	// Headers is an optional map of extra HTTP headers to include in the request.
	Headers map[string]string
}

// WebhookChannel sends alert notifications to an arbitrary HTTP endpoint.
// The payload is the exact alert-notification JSON (per contracts/events/alert-notification.schema.json).
// The HMAC-SHA256 signature is in X-Pulse-Signature: sha256=<hex>.
type WebhookChannel struct {
	cfg    WebhookConfig
	client *http.Client
}

// NewWebhookChannel creates a new generic webhook channel.
func NewWebhookChannel(cfg WebhookConfig) *WebhookChannel {
	return &WebhookChannel{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns "webhook".
func (w *WebhookChannel) Name() string { return "webhook" }

// Send posts the alert notification to the configured URL with HMAC-SHA256 signature.
// The payload is the raw alert-notification.schema.json JSON.
func (w *WebhookChannel) Send(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook channel: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pulse-webhook/1.0")

	// Sign payload with HMAC-SHA256 if secret is configured.
	if w.cfg.Secret != "" {
		sig := computeHMACSHA256(w.cfg.Secret, payload)
		req.Header.Set("X-Pulse-Signature", "sha256="+sig)
	}

	// Add any custom headers.
	for k, v := range w.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook channel: send: %w", err)
	}
	defer resp.Body.Close()

	// Accept any 2xx response.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook channel: endpoint returned %d", resp.StatusCode)
	}
	return nil
}

// computeHMACSHA256 returns the HMAC-SHA256 hex signature for a payload and secret.
// This is the authoritative signing function — consumers must use the same algorithm:
//
//	HMAC-SHA256(key=secret, message=payload_bytes)
//	signature = "sha256=" + hex(mac)
func computeHMACSHA256(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhookSignature verifies the X-Pulse-Signature header from a received webhook.
// Returns true if the signature is valid.
// This is exported for use in SDK/documentation examples.
func VerifyWebhookSignature(secret string, payload []byte, signature string) bool {
	const prefix = "sha256="
	if len(signature) <= len(prefix) {
		return false
	}
	expected := prefix + computeHMACSHA256(secret, payload)
	// Constant-time comparison to prevent timing attacks.
	return hmac.Equal([]byte(signature), []byte(expected))
}

// ─── Test helper ──────────────────────────────────────────────────────────────

// BuildTestPayload builds a minimal alert notification payload for testing.
func BuildTestPayload(ruleID string) []byte {
	n := map[string]any{
		"version":        1,
		"alert_id":       "test-alert-id",
		"rule_id":        ruleID,
		"state":          "firing",
		"severity":       "info",
		"ts":             time.Now().UnixMilli(),
		"title":          "Pulse test notification",
		"metric":         "test_fire",
		"value":          0.0,
		"threshold":      0.0,
		"scope":          map[string]any{},
		"test":           true,
		"cooldown_until": nil,
		"group_key":      nil,
	}
	b, _ := json.Marshal(n)
	return b
}
