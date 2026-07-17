package alert_test

// s72_d134_test.go — D-134 (S72) tests for [22]: an already-expired TLS cert must
// yield the documented -1 sentinel (nil error) so a `cert_expiry lt 0` rule fires.
// Two reachable paths are covered:
//   - production (verification on): a trusted-CA leaf that has expired fails the
//     handshake with reason Expired; DaysUntilExpiry detects that and returns -1;
//   - a caller supplying InsecureSkipVerify reaches the leaf directly and the
//     now.After(NotAfter) branch returns -1.
// Reuses generateTestCert (wave2_test.go).

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
)

func startTLSServerWithCert(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// TestCertExpiry_TrustedCAExpired_ReturnsNegative covers the common production path:
// verification is ON, the leaf chains to a trusted anchor but has EXPIRED, so the
// handshake fails specifically for expiry. DaysUntilExpiry must surface that as
// (-1, nil) rather than a generic error that evalCertExpiry would skip.
func TestCertExpiry_TrustedCAExpired_ReturnsNegative(t *testing.T) {
	cert := generateTestCert(t, -2) // expired self-signed leaf (NotAfter 2 days ago)
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	// Trust the leaf as a root so the chain is valid and expiry is the ONLY failure.
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	hostPort := startTLSServerWithCert(t, cert)
	checker := alert.NewCertCheckerWithTLSConfig(&tls.Config{RootCAs: pool}, 5*time.Second)

	days, err := checker.DaysUntilExpiry(context.Background(), hostPort)
	if err != nil {
		t.Fatalf("expired trusted-CA cert: got error %v, want (-1, nil) via expiry detection", err)
	}
	if days >= 0 {
		t.Errorf("expired cert: days = %.2f, want < 0 (so `cert_expiry lt 0` fires)", days)
	}
}

// TestCertExpiry_SkipVerifyConfig_Expired_ReturnsNegative covers the in-config
// expiry branch: a caller that disables verification reaches the leaf directly, and
// now.After(NotAfter) must return the -1 sentinel (the pre-fix code returned 0).
func TestCertExpiry_SkipVerifyConfig_Expired_ReturnsNegative(t *testing.T) {
	cert := generateTestCert(t, -2)
	hostPort := startTLSServerWithCert(t, cert)
	// InsecureSkipVerify is test-only here (CodeQL's go/disabled-certificate-check
	// flags production code only); it lets the dial reach the NotAfter comparison.
	checker := alert.NewCertCheckerWithTLSConfig(&tls.Config{InsecureSkipVerify: true}, 5*time.Second)

	days, err := checker.DaysUntilExpiry(context.Background(), hostPort)
	if err != nil {
		t.Fatalf("DaysUntilExpiry: %v", err)
	}
	if days >= 0 {
		t.Errorf("expired cert (skip-verify): days = %.2f, want < 0", days)
	}
}
