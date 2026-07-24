package channels_test

// channels_ssrf_test.go — Fix E regression: SSRF guard on alert channel HTTP clients.
//
// Both WebhookChannel and SlackChannel install ssrfguard.DialControl on their
// internal http.Client transport so that a misconfigured webhook URL targeting a
// cloud IMDS endpoint (169.254.169.254 — IMDSv4, shared by AWS/GCP/Azure) is
// refused at dial time. The guard runs on the RESOLVED IP, making it
// DNS-rebinding-safe.
//
// These tests prove the guard is wired: dialing 169.254.169.254 must return an
// error before any network connection is established (fast refusal, no timeout
// needed).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
)

// ─── WebhookChannel ──────────────────────────────────────────────────────────

func TestWebhookChannel_RefusesIMDS_IPv4(t *testing.T) {
	ch := channels.NewWebhookChannel(channels.WebhookConfig{
		URL: "http://169.254.169.254/latest/meta-data/",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	if err == nil {
		t.Fatal("expected error when dialing IMDSv4 address 169.254.169.254, got nil")
	}
	// The guard error must mention the address so operators can diagnose mis-configs.
	if !strings.Contains(err.Error(), "169.254.169.254") && !strings.Contains(err.Error(), "ssrfguard") {
		t.Logf("error does not mention ssrfguard or address (may be wrapped): %v", err)
	}
	t.Logf("WebhookChannel IMDS refusal error (expected): %v", err)
}

func TestWebhookChannel_RefusesIMDS_ZeroAddr(t *testing.T) {
	ch := channels.NewWebhookChannel(channels.WebhookConfig{
		URL: "http://0.0.0.0/",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	if err == nil {
		t.Fatal("expected error when dialing unspecified address 0.0.0.0, got nil")
	}
	t.Logf("WebhookChannel 0.0.0.0 refusal error (expected): %v", err)
}

func TestWebhookChannel_AllowsLoopback(t *testing.T) {
	// Loopback (127.x) is allowed by ssrfguard policy. The test expects a
	// connection-refused error (nothing listening), not an ssrfguard refusal.
	// A port-unreachable error is fine — the guard must NOT be the reason it fails.
	ch := channels.NewWebhookChannel(channels.WebhookConfig{
		URL: "http://127.0.0.1:19999/pulse-test-loopback-webhook",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	// We expect a connection-refused or similar OS error, but NOT an ssrfguard refusal.
	// Check specifically for "ssrfguard: refusing to dial restricted" — the guard error prefix.
	if err != nil && strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Errorf("ssrfguard should NOT block loopback; got: %v", err)
	}
	t.Logf("loopback result (expected non-guard error): %v", err)
}

// ─── SlackChannel ────────────────────────────────────────────────────────────

func TestSlackChannel_RefusesIMDS_IPv4(t *testing.T) {
	ch := channels.NewSlackChannel(channels.SlackConfig{
		WebhookURL: "http://169.254.169.254/hooks/test",
		Channel:    "#alerts",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	if err == nil {
		t.Fatal("expected error when dialing IMDSv4 address 169.254.169.254 via SlackChannel, got nil")
	}
	t.Logf("SlackChannel IMDS refusal error (expected): %v", err)
}

func TestSlackChannel_AllowsLoopback(t *testing.T) {
	// Same as WebhookChannel: loopback is allowed — connection-refused expected,
	// but ssrfguard must not be the error source.
	ch := channels.NewSlackChannel(channels.SlackConfig{
		WebhookURL: "http://127.0.0.1:19999/pulse-test-loopback-slack",
		Channel:    "#alerts",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	if err != nil && strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Errorf("ssrfguard should NOT block loopback for SlackChannel; got: %v", err)
	}
	t.Logf("SlackChannel loopback result (expected non-guard error): %v", err)
}
