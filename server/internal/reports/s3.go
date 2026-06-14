// Package reports — S3-compatible CSV export (F8, WO-204 item 6).
//
// Implementation: AWS Signature Version 4 by hand (no external dep).
// Justification: minio-go adds ~3 MB to the binary; hand-rolled SigV4 PUT
// adds ~200 lines and zero external deps. The PUT-object use case is simple
// enough that the full SDK is unnecessary.
//
// Config:
//   PULSE_S3_ENDPOINT  — S3-compatible endpoint URL (e.g. https://s3.amazonaws.com)
//   PULSE_S3_BUCKET    — bucket name
//   PULSE_S3_PREFIX    — object key prefix (e.g. "pulse-reports/")
//   PULSE_S3_REGION    — AWS region (default: us-east-1)
//   PULSE_S3_ACCESS_KEY_ID / PULSE_S3_SECRET_ACCESS_KEY — credentials via env ref
//
// Never store credentials plaintext in meta store: creds come from env vars only.
// Config fields for endpoint/bucket/prefix/region are stored in schedule config JSON.
// Retry: 3 attempts with 2 s backoff; alert on repeated failure.
package reports

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// S3Config holds S3-compatible upload configuration.
// Credentials are always sourced from environment variables (never stored plaintext).
type S3Config struct {
	Endpoint        string // e.g. "https://s3.amazonaws.com" or "http://minio:9000"
	Bucket          string // bucket name
	Prefix          string // object key prefix
	Region          string // AWS region (default: us-east-1)
	// AccessKeyEnvRef and SecretKeyEnvRef name the env vars holding credentials.
	// If empty, fall back to AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
	AccessKeyEnvRef string
	SecretKeyEnvRef string
}

// S3Uploader uploads objects to an S3-compatible store using SigV4.
type S3Uploader struct {
	cfg    S3Config
	logger *slog.Logger
	client *http.Client
}

// NewS3Uploader creates an S3Uploader.
func NewS3Uploader(cfg S3Config, logger *slog.Logger) *S3Uploader {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &S3Uploader{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Upload uploads data to S3 at the given key, with retries.
// Returns an error if all retries are exhausted.
func (u *S3Uploader) Upload(ctx context.Context, key, contentType string, data []byte) error {
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := u.uploadOnce(ctx, key, contentType, data); err != nil {
			lastErr = err
			u.logger.Warn("reports: S3 upload attempt failed",
				"attempt", attempt,
				"key", key,
				"error", err)
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Duration(attempt) * time.Second):
				}
			}
			continue
		}
		u.logger.Info("reports: S3 upload succeeded", "key", key, "bytes", len(data))
		return nil
	}
	return fmt.Errorf("S3 upload failed after %d attempts: %w", maxRetries, lastErr)
}

// uploadOnce performs a single PUT-object request with SigV4 signing.
func (u *S3Uploader) uploadOnce(ctx context.Context, key, contentType string, data []byte) error {
	accessKey := u.resolveEnv(u.cfg.AccessKeyEnvRef, "AWS_ACCESS_KEY_ID")
	secretKey := u.resolveEnv(u.cfg.SecretKeyEnvRef, "AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("S3 credentials not configured (set AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)")
	}

	endpoint := strings.TrimRight(u.cfg.Endpoint, "/")
	url := fmt.Sprintf("%s/%s/%s", endpoint, u.cfg.Bucket, key)

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	// Compute content hash.
	bodyHash := sha256Hex(data)

	// Build canonical request.
	headers := map[string]string{
		"host":                  hostFromURL(endpoint),
		"x-amz-content-sha256": bodyHash,
		"x-amz-date":           amzDate,
		"content-type":         contentType,
	}
	signedHeaders, canonicalHeaders := buildCanonicalHeaders(headers)
	canonicalRequest := strings.Join([]string{
		"PUT",
		"/" + u.cfg.Bucket + "/" + key,
		"", // query string (empty)
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	// Build string to sign.
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, u.cfg.Region)
	strToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	// Derive signing key.
	signingKey := deriveSigningKey(secretKey, dateStamp, u.cfg.Region, "s3")
	signature := hexHMACSHA256(signingKey, strToSign)

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Amz-Content-SHA256", bodyHash)
	req.Header.Set("X-Amz-Date", amzDate)
	req.ContentLength = int64(len(data))

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("PUT %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 300 {
		return fmt.Errorf("PUT %s: HTTP %d: %s", url, resp.StatusCode, string(body))
	}
	return nil
}

func (u *S3Uploader) resolveEnv(envRef, fallback string) string {
	if envRef != "" {
		if v := os.Getenv(envRef); v != "" {
			return v
		}
	}
	return os.Getenv(fallback)
}

// ─── SigV4 helpers ────────────────────────────────────────────────────────────

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hexSHA256(data []byte) string {
	return sha256Hex(data)
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func hexHMACSHA256(key []byte, data string) string {
	return hex.EncodeToString(hmacSHA256(key, data))
}

func deriveSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	return kSigning
}

// buildCanonicalHeaders returns (signedHeaders, canonicalHeadersBlock).
func buildCanonicalHeaders(headers map[string]string) (string, string) {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)

	var canonical strings.Builder
	for _, k := range keys {
		canonical.WriteString(k)
		canonical.WriteString(":")
		// Trim spaces in value.
		canonical.WriteString(strings.TrimSpace(headers[k]))
		canonical.WriteString("\n")
	}
	return strings.Join(keys, ";"), canonical.String()
}

func hostFromURL(endpoint string) string {
	// Strip scheme.
	h := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	// Strip trailing path.
	if idx := strings.Index(h, "/"); idx >= 0 {
		h = h[:idx]
	}
	return h
}

// S3FakeServer is a minimal in-memory S3-compatible handler for tests.
// Registers PUT /{bucket}/{key} and verifies SigV4 Authorization header presence.
type S3FakeServer struct {
	Uploads map[string][]byte // key → body
}

// NewS3FakeServer creates a test fake.
func NewS3FakeServer() *S3FakeServer {
	return &S3FakeServer{Uploads: make(map[string][]byte)}
}

// ServeHTTP handles PUT /{bucket}/{key}.
func (f *S3FakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Verify SigV4 Authorization header is present.
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/")
	f.Uploads[key] = body
	w.WriteHeader(http.StatusOK)
}
