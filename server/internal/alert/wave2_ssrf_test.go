package alert_test

// wave2_ssrf_test.go — SSRF guard on CertChecker TLS dialer.
//
// CertChecker.DaysUntilExpiry dials a host derived from an alert rule scope
// (scope.StreamID used as host:port). This is API-supplied. Without
// ssrfguard.DialControl on the net.Dialer, setting the scope to
// 169.254.169.254:443 reaches the cloud metadata service.
//
// These tests prove the guard is wired: dialing a link-local address (IMDSv4)
// must return an error, while loopback remains allowed.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/alert"
)

// TestCertChecker_RefusesIMDS_IPv4 verifies that CertChecker refuses to dial
// the cloud metadata address (169.254.169.254). This would fail before the fix.
func TestCertChecker_RefusesIMDS_IPv4(t *testing.T) {
	checker := alert.NewCertChecker(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := checker.DaysUntilExpiry(ctx, "169.254.169.254:443")
	if err == nil {
		t.Fatal("expected error when dialing IMDSv4 address 169.254.169.254:443, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing" — if it's a timeout or connection
	// error, the guard is NOT installed and the dial was actually attempted.
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("CertChecker IMDS refusal error (expected): %v", err)
}

// TestCertChecker_RefusesIMDS_ZeroAddr verifies that CertChecker refuses to
// dial the unspecified address 0.0.0.0.
func TestCertChecker_RefusesIMDS_ZeroAddr(t *testing.T) {
	checker := alert.NewCertChecker(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := checker.DaysUntilExpiry(ctx, "0.0.0.0:443")
	if err == nil {
		t.Fatal("expected error when dialing unspecified address 0.0.0.0:443, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing" — if it's a connection
	// error, the guard is NOT installed and the dial was actually attempted.
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("CertChecker 0.0.0.0 refusal error (expected): %v", err)
}

// TestCertChecker_AllowsLoopback verifies that ssrfguard does not block loopback
// addresses (127.x). The test expects a connection or TLS error (no TLS server
// listening), not an ssrfguard refusal.
func TestCertChecker_AllowsLoopback(t *testing.T) {
	checker := alert.NewCertChecker(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := checker.DaysUntilExpiry(ctx, "127.0.0.1:19997")
	// We expect a connection-refused or TLS error, but NOT an ssrfguard refusal.
	if err != nil && strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Errorf("ssrfguard should NOT block loopback; got: %v", err)
	}
	t.Logf("CertChecker loopback result (expected non-guard error): %v", err)
}

// TestCertCheckerWithTLSConfig_RefusesIMDS_IPv4 verifies that the alternate
// constructor NewCertCheckerWithTLSConfig also has the SSRF guard installed.
func TestCertCheckerWithTLSConfig_RefusesIMDS_IPv4(t *testing.T) {
	checker := alert.NewCertCheckerWithTLSConfig(nil, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := checker.DaysUntilExpiry(ctx, "169.254.169.254:443")
	if err == nil {
		t.Fatal("expected error when dialing IMDSv4 via NewCertCheckerWithTLSConfig, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing".
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("CertCheckerWithTLSConfig IMDS refusal error (expected): %v", err)
}
