package main

// BUG-2 regression suite: beacon ingest token adapter vs HMAC-stored tokens.
//
// Since D-052, meta.Store.HashToken uses HMAC-SHA256 when PULSE_SECRET_KEY is
// set (now mandatory). The old adapter called GetIngestTokenByHash(sha256(raw))
// which never matched an HMAC-stored token → always 401.
//
// The fix: adapter calls LookupToken(ctx, rawToken) which tries HMAC first then
// falls back to plain SHA-256 for legacy rows.
//
// TDD layout (tests written BEFORE the adapter was changed):
//   TestBeaconAdapter_HMACToken_Authenticates  — FAILS before fix (compile error on LookupIngestToken)
//   TestBeaconAdapter_LegacyTokenBackCompat    — pin the back-compat promise
//   TestBeaconAdapter_WrongKindRejected        — non-ingest token must be rejected
//   TestBeaconAdapter_WrongTokenRejected       — unknown raw token must be rejected

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// adapterDDLPath returns the path to the meta DDL relative to this file.
func adapterDDLPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file),
		"..", "..", "..", "contracts", "db", "meta", "0001_init.sql"))
}

// newTestMetaStore creates an in-memory meta.Store with an explicit key so that
// new tokens are HMAC-hashed (the D-052 default on every real boot path).
func newTestMetaStore(t *testing.T, key string) *meta.Store {
	t.Helper()
	ctx := context.Background()
	ddlPath := adapterDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found at %s: %v", ddlPath, err)
	}
	st, err := meta.New(ctx, "sqlite", ":memory:", key)
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := st.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("MigrateEmbedded: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// sha256HexTest is a local helper for pre-seeding legacy rows.
func sha256HexTest(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// TestBeaconAdapter_HMACToken_Authenticates verifies that an ingest token
// created via the normal store path (HMAC hash when key is set) authenticates
// via the adapter with the raw token.
//
// This test MUST FAIL against the pre-fix code (compile error: LookupIngestToken
// does not exist on the old TokenStore interface / metaIngestTokenStore).
func TestBeaconAdapter_HMACToken_Authenticates(t *testing.T) {
	ctx := context.Background()
	st := newTestMetaStore(t, "test-secret-key-beacon-adapter-16x")

	rawToken := "plt_ingest_hmac_test_rawtoken_xyz"
	tokenHash, hashAlg := st.HashToken(rawToken) // will be hmac-sha256 (key is set)
	if hashAlg != "hmac-sha256" {
		t.Fatalf("expected hmac-sha256, got %q; test setup is wrong", hashAlg)
	}

	if err := st.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "hmac-ingest-test",
		TokenHash: tokenHash,
		HashAlg:   hashAlg,
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Build the adapter under test.
	adapter := &metaIngestTokenStore{store: st}

	// LookupIngestToken is the NEW method — compile-fails on old code (red state).
	tokenID, ok, err := adapter.LookupIngestToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupIngestToken returned error: %v", err)
	}
	if !ok {
		t.Fatal("LookupIngestToken: expected ok=true for valid HMAC-stored ingest token, got false (BUG-2)")
	}
	if tokenID == "" {
		t.Fatal("LookupIngestToken: expected non-empty tokenID")
	}
	t.Logf("PASS BUG-2: HMAC-stored ingest token authenticated; tokenID=%s", tokenID)
}

// TestBeaconAdapter_LegacyTokenBackCompat verifies that a legacy row stored
// with plain SHA-256 + hash_alg='sha256' still authenticates via the adapter.
// This pins the D-052 back-compat promise for ingest tokens.
func TestBeaconAdapter_LegacyTokenBackCompat(t *testing.T) {
	ctx := context.Background()
	// Use an explicit key — but the legacy row is pre-seeded with sha256 hash.
	st := newTestMetaStore(t, "test-secret-key-beacon-legacy-abc1")

	rawToken := "plt_ingest_legacy_sha256_rawtoken"
	legacyHash := sha256HexTest(rawToken)

	if err := st.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "legacy-sha256-ingest",
		TokenHash: legacyHash,
		HashAlg:   "sha256", // legacy algorithm
		CreatedAt: 999,
	}); err != nil {
		t.Fatalf("CreateToken (legacy): %v", err)
	}

	adapter := &metaIngestTokenStore{store: st}

	tokenID, ok, err := adapter.LookupIngestToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupIngestToken (legacy): error: %v", err)
	}
	if !ok {
		t.Fatal("LookupIngestToken (legacy): expected ok=true for legacy SHA-256 ingest token, got false")
	}
	if tokenID == "" {
		t.Fatal("LookupIngestToken (legacy): expected non-empty tokenID")
	}
	t.Logf("PASS back-compat: legacy SHA-256 ingest token authenticated; tokenID=%s", tokenID)
}

// TestBeaconAdapter_WrongKindRejected verifies that a token with kind!="ingest"
// is rejected (returns ok=false) even when the raw token matches.
func TestBeaconAdapter_WrongKindRejected(t *testing.T) {
	ctx := context.Background()
	st := newTestMetaStore(t, "test-secret-key-beacon-kind-xyz1")

	rawToken := "plt_api_admin_shouldnotwork"
	tokenHash, hashAlg := st.HashToken(rawToken)
	if err := st.CreateToken(ctx, meta.APIToken{
		Kind:      "api", // NOT "ingest"
		Name:      "api-token-not-ingest",
		TokenHash: tokenHash,
		HashAlg:   hashAlg,
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (api kind): %v", err)
	}

	adapter := &metaIngestTokenStore{store: st}

	_, ok, err := adapter.LookupIngestToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupIngestToken (wrong kind): unexpected error: %v", err)
	}
	if ok {
		t.Fatal("LookupIngestToken (wrong kind): expected ok=false for non-ingest token kind, got true")
	}
	t.Logf("PASS: kind!=\"ingest\" token correctly rejected")
}

// TestBeaconAdapter_WrongTokenRejected verifies that an unknown raw token
// returns ok=false (no match in the store).
func TestBeaconAdapter_WrongTokenRejected(t *testing.T) {
	ctx := context.Background()
	st := newTestMetaStore(t, "test-secret-key-beacon-wrong-tok1")

	// No token seeded — any lookup should return false.
	adapter := &metaIngestTokenStore{store: st}

	_, ok, err := adapter.LookupIngestToken(ctx, "plt_nonexistent_token_xyz_9999")
	if err != nil {
		t.Fatalf("LookupIngestToken (wrong token): unexpected error: %v", err)
	}
	if ok {
		t.Fatal("LookupIngestToken (wrong token): expected ok=false for unknown token, got true")
	}
	t.Logf("PASS: unknown token correctly rejected")
}

// TestBeaconAdapter_ExpiredTokenRejected verifies that an ingest token with an
// ExpiresAt in the past is rejected (ok=false) by the adapter.
//
// This is the regression guard for the blocker identified in the fix-round-1
// review: LookupIngestToken checked tok.Kind but not tok.ExpiresAt, allowing
// expired ingest tokens to authenticate on the beacon endpoint.
//
// TDD: this test MUST FAIL before the expiry check is added to
// metaIngestTokenStore.LookupIngestToken (it will return ok=true for an expired
// token). After the fix it must return ok=false.
func TestBeaconAdapter_ExpiredTokenRejected(t *testing.T) {
	ctx := context.Background()
	st := newTestMetaStore(t, "test-secret-key-beacon-expiry-x1y")

	rawToken := "plt_ingest_expired_test_rawtoken"
	tokenHash, hashAlg := st.HashToken(rawToken)

	// ExpiresAt = 1 ms (well in the past).
	pastMS := int64(1)
	if err := st.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "expired-ingest-token",
		TokenHash: tokenHash,
		HashAlg:   hashAlg,
		ExpiresAt: &pastMS,
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (expired): %v", err)
	}

	adapter := &metaIngestTokenStore{store: st}

	_, ok, err := adapter.LookupIngestToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupIngestToken (expired): unexpected error: %v", err)
	}
	// Must be rejected: expired token must never authenticate.
	if ok {
		t.Fatal("LookupIngestToken (expired): expected ok=false for expired ingest token, got true (BLOCKER)")
	}
	t.Logf("PASS: expired ingest token correctly rejected by adapter")
}

// TestBeaconAdapter_NotYetExpiredTokenAccepted verifies that a token with a
// future ExpiresAt is still accepted (the expiry check is not over-aggressive).
func TestBeaconAdapter_NotYetExpiredTokenAccepted(t *testing.T) {
	ctx := context.Background()
	st := newTestMetaStore(t, "test-secret-key-beacon-noexp-abc1")

	rawToken := "plt_ingest_future_expiry_rawtoken"
	tokenHash, hashAlg := st.HashToken(rawToken)

	// ExpiresAt far in the future (year 2099 in ms).
	futureMS := int64(4102444800000)
	if err := st.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "future-expiry-ingest-token",
		TokenHash: tokenHash,
		HashAlg:   hashAlg,
		ExpiresAt: &futureMS,
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (future expiry): %v", err)
	}

	adapter := &metaIngestTokenStore{store: st}

	tokenID, ok, err := adapter.LookupIngestToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupIngestToken (future expiry): unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("LookupIngestToken (future expiry): expected ok=true for non-expired ingest token, got false")
	}
	if tokenID == "" {
		t.Fatal("LookupIngestToken (future expiry): expected non-empty tokenID")
	}
	t.Logf("PASS: non-expired ingest token correctly accepted; tokenID=%s", tokenID)
}
