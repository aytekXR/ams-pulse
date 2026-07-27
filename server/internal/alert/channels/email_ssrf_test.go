package channels_test

// email_ssrf_test.go — SSRF guard on EmailChannel SMTP dialer.
//
// EmailChannel.Send dials cfg.SMTPAddr which is API-supplied via the alert-channel
// config key smtp_addr. Without ssrfguard.DialControl on the dialer, setting
// SMTPAddr to 169.254.169.254:25 reaches the cloud metadata service.
//
// These tests prove the guard is wired: dialing a link-local address (IMDSv4) must
// return an error, while loopback remains allowed.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/alert/channels"
)

// TestEmailChannel_RefusesIMDS_IPv4 verifies that EmailChannel refuses to dial
// the cloud metadata address (169.254.169.254). This would fail before the fix.
func TestEmailChannel_RefusesIMDS_IPv4(t *testing.T) {
	ch := channels.NewEmailChannel(channels.EmailConfig{
		SMTPAddr: "169.254.169.254:25",
		From:     "test@pulse.local",
		To:       "admin@example.com",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	if err == nil {
		t.Fatal("expected error when dialing IMDSv4 address 169.254.169.254:25, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing" — if it's a timeout or connection
	// error, the guard is NOT installed and the dial was actually attempted.
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("EmailChannel IMDS refusal error (expected): %v", err)
}

// TestEmailChannel_RefusesIMDS_ZeroAddr verifies that EmailChannel refuses to
// dial the unspecified address 0.0.0.0.
func TestEmailChannel_RefusesIMDS_ZeroAddr(t *testing.T) {
	ch := channels.NewEmailChannel(channels.EmailConfig{
		SMTPAddr: "0.0.0.0:25",
		From:     "test@pulse.local",
		To:       "admin@example.com",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	if err == nil {
		t.Fatal("expected error when dialing unspecified address 0.0.0.0:25, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing" — if it's a connection refused
	// error, the guard is NOT installed and the dial was actually attempted.
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("EmailChannel 0.0.0.0 refusal error (expected): %v", err)
}

// TestEmailChannel_AllowsLoopback verifies that ssrfguard does not block loopback
// addresses (127.x). The test expects a connection-refused error (nothing listening
// on that port), not an ssrfguard refusal.
func TestEmailChannel_AllowsLoopback(t *testing.T) {
	ch := channels.NewEmailChannel(channels.EmailConfig{
		SMTPAddr: "127.0.0.1:19998", // unlikely to have SMTP running
		From:     "test@pulse.local",
		To:       "admin@example.com",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := ch.Send(ctx, channels.BuildTestPayload("test-rule"))
	// We expect a connection-refused or similar OS error, but NOT an ssrfguard refusal.
	if err != nil && strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Errorf("ssrfguard should NOT block loopback; got: %v", err)
	}
	t.Logf("EmailChannel loopback result (expected non-guard error): %v", err)
}
