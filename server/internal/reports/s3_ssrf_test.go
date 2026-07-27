package reports_test

// s3_ssrf_test.go — SSRF guard on S3Uploader HTTP client.
//
// S3Uploader builds a plain http.Client with no guarded transport; the endpoint
// comes from PULSE_S3_ENDPOINT (environment). While lower severity than API-supplied
// addresses, the same ssrfguard.DialControl is required to prevent SSRF via a
// misconfigured S3 endpoint pointing to the cloud metadata service.
//
// These tests prove the guard is wired: dialing a link-local address (IMDSv4)
// must return an error, while loopback remains allowed.

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/reports"
)

// TestS3Uploader_RefusesIMDS_IPv4 verifies that S3Uploader refuses to dial
// the cloud metadata address (169.254.169.254). This would fail before the fix.
func TestS3Uploader_RefusesIMDS_IPv4(t *testing.T) {
	// Set dummy credentials so the uploader doesn't fail on missing creds.
	os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	uploader := reports.NewS3Uploader(reports.S3Config{
		Endpoint: "http://169.254.169.254",
		Bucket:   "test-bucket",
		Prefix:   "reports/",
		Region:   "us-east-1",
	}, slog.Default())

	// The guard fails immediately (no network I/O), but the Upload retry loop adds
	// backoff delays (2s, 4s, 6s between 3 attempts). Give enough time for all retries
	// to complete with ssrfguard errors so the final wrapped error contains the guard
	// message. Total time: 3 fast failures + 2s + 4s backoff = ~7s max.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := uploader.Upload(ctx, "test.csv", "text/csv", []byte("test,data"))
	if err == nil {
		t.Fatal("expected error when dialing IMDSv4 address 169.254.169.254, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing" — if it's a timeout or connection
	// error, the guard is NOT installed and the dial was actually attempted.
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("S3Uploader IMDS refusal error (expected): %v", err)
}

// TestS3Uploader_RefusesIMDS_ZeroAddr verifies that S3Uploader refuses to
// dial the unspecified address 0.0.0.0.
func TestS3Uploader_RefusesIMDS_ZeroAddr(t *testing.T) {
	os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	uploader := reports.NewS3Uploader(reports.S3Config{
		Endpoint: "http://0.0.0.0",
		Bucket:   "test-bucket",
		Prefix:   "reports/",
		Region:   "us-east-1",
	}, slog.Default())

	// Same as IPv4 test — give enough time for all retries to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := uploader.Upload(ctx, "test.csv", "text/csv", []byte("test,data"))
	if err == nil {
		t.Fatal("expected error when dialing unspecified address 0.0.0.0, got nil")
	}
	// The guard error MUST contain "ssrfguard: refusing" — if it's a connection
	// error, the guard is NOT installed and the dial was actually attempted.
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("expected ssrfguard refusal error, got (dial was attempted): %v", err)
	}
	t.Logf("S3Uploader 0.0.0.0 refusal error (expected): %v", err)
}

// TestS3Uploader_AllowsLoopback verifies that ssrfguard does not block loopback
// addresses (127.x). The test expects a connection-refused error (nothing listening
// on that port), not an ssrfguard refusal.
func TestS3Uploader_AllowsLoopback(t *testing.T) {
	os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	}()

	uploader := reports.NewS3Uploader(reports.S3Config{
		Endpoint: "http://127.0.0.1:19996",
		Bucket:   "test-bucket",
		Prefix:   "reports/",
		Region:   "us-east-1",
	}, slog.Default())

	// Loopback is allowed, so connection-refused should be fast on each attempt.
	// Total retry time: 3 fast failures + 2s + 4s backoff = ~7s max.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := uploader.Upload(ctx, "test.csv", "text/csv", []byte("test,data"))
	// We expect a connection-refused or similar OS error, but NOT an ssrfguard refusal.
	if err != nil && strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Errorf("ssrfguard should NOT block loopback; got: %v", err)
	}
	t.Logf("S3Uploader loopback result (expected non-guard error): %v", err)
}
