// TDD tests for HMAC-SHA256 API token hashing (item 6 — P2 hardening batch).
//
// Behavioral coverage:
//   - New tokens created with an explicit secret key are stored as HMAC-SHA256
//   - LookupToken finds HMAC tokens by raw value
//   - Legacy SHA-256 rows (hash_alg 'sha256' or NULL) still authenticate
//   - Revocation by ID works for both hash types
//   - Wrong tokens are rejected
//   - Empty cipher key (no PULSE_SECRET_KEY) falls back to sha256 alg
//   - Migration upgrade: old-schema DB gains hash_alg column; legacy row survives
package meta_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// openStoreWithKey creates an in-memory store with an explicit secret key.
// The explicit key causes HashToken to return hmac-sha256.
func openStoreWithKey(t *testing.T, secretKey string) *meta.Store {
	t.Helper()
	ctx := context.Background()
	s, err := meta.New(ctx, "sqlite", ":memory:", secretKey)
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	ddl := readMetaDDL(t)
	if err := s.MigrateEmbedded(ctx, ddl); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// legacySHA256Hex returns the plain SHA-256 hex of a string (legacy format).
func legacySHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// TestHMACToken_HashTokenReturnsHMACAlg verifies HashToken returns "hmac-sha256"
// when an explicit secret key is set, and "sha256" when no key is set.
func TestHMACToken_HashTokenReturnsHMACAlg(t *testing.T) {
	ctx := context.Background()

	// With explicit key → hmac-sha256.
	sKeyed, err := meta.New(ctx, "sqlite", ":memory:", "explicit-test-key-001")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	defer sKeyed.Close()

	rawToken := "plt_hmactest_abcdef1234567890"
	hash, alg := sKeyed.HashToken(rawToken)
	if alg != "hmac-sha256" {
		t.Errorf("HashToken: expected alg=%q, got %q", "hmac-sha256", alg)
	}
	if hash == "" {
		t.Error("HashToken: expected non-empty hash")
	}
	// Hash must NOT equal the plain SHA-256 (HMAC key-separates from bare SHA-256).
	if hash == legacySHA256Hex(rawToken) {
		t.Error("HashToken: HMAC hash must differ from plain SHA-256 hash")
	}

	// Without explicit key (empty string → random ephemeral key on :memory:) → sha256.
	sNoKey, err := meta.New(ctx, "sqlite", ":memory:", "")
	if err != nil {
		t.Fatalf("meta.New (no key): %v", err)
	}
	defer sNoKey.Close()

	_, algNoKey := sNoKey.HashToken(rawToken)
	if algNoKey != "sha256" {
		t.Errorf("HashToken (no key): expected alg=%q, got %q", "sha256", algNoKey)
	}
}

// TestHMACToken_CreateAndLookup verifies that a token created via CreateToken
// with hmac-sha256 is found by LookupToken using the raw value.
func TestHMACToken_CreateAndLookup(t *testing.T) {
	s := openStoreWithKey(t, "explicit-test-key-002")
	ctx := context.Background()

	rawToken := "plt_lookup_abcdef1234567890"
	hash, alg := s.HashToken(rawToken)

	tok := meta.APIToken{
		Kind:      "api",
		Name:      "hmac-lookup-test",
		TokenHash: hash,
		HashAlg:   alg,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}
	if err := s.CreateToken(ctx, tok); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	found, err := s.LookupToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupToken: %v", err)
	}
	if found == nil {
		t.Fatal("LookupToken: expected token, got nil")
	}
	if found.Kind != "api" || found.Name != "hmac-lookup-test" {
		t.Errorf("LookupToken: wrong token returned: %+v", found)
	}
	if found.HashAlg != "hmac-sha256" {
		t.Errorf("LookupToken: expected HashAlg=%q, got %q", "hmac-sha256", found.HashAlg)
	}
}

// TestHMACToken_LegacySHA256Lookup verifies that a pre-existing row with
// hash_alg='sha256' is still found by LookupToken (backward compat).
func TestHMACToken_LegacySHA256Lookup(t *testing.T) {
	s := openStoreWithKey(t, "explicit-test-key-003")
	ctx := context.Background()

	rawToken := "plt_legacy_abcdef1234567890"
	sha256Hash := legacySHA256Hex(rawToken)

	// Insert a legacy row with explicit hash_alg='sha256'.
	legacyTok := meta.APIToken{
		Kind:      "api",
		Name:      "legacy-sha256-row",
		TokenHash: sha256Hash,
		HashAlg:   "sha256",
		Scopes:    []string{"admin"},
		CreatedAt: 2000,
	}
	if err := s.CreateToken(ctx, legacyTok); err != nil {
		t.Fatalf("CreateToken (legacy): %v", err)
	}

	// LookupToken must find it via SHA-256 fallback path.
	found, err := s.LookupToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupToken (legacy sha256): %v", err)
	}
	if found == nil {
		t.Fatal("LookupToken (legacy sha256): expected token, got nil — backward compat broken")
	}
	if found.Name != "legacy-sha256-row" {
		t.Errorf("LookupToken (legacy sha256): wrong row: %+v", found)
	}
}

// TestHMACToken_LegacyNullHashAlgLookup verifies that rows with HashAlg=""
// (treated as 'sha256' default) are found by LookupToken.
func TestHMACToken_LegacyNullHashAlgLookup(t *testing.T) {
	s := openStoreWithKey(t, "explicit-test-key-004")
	ctx := context.Background()

	rawToken := "plt_nullalg_abcdef1234567890"
	sha256Hash := legacySHA256Hex(rawToken)

	// Insert a row with HashAlg="" (stored as 'sha256' default by CreateToken).
	nullAlgTok := meta.APIToken{
		Kind:      "api",
		Name:      "null-alg-row",
		TokenHash: sha256Hash,
		HashAlg:   "", // empty → CreateToken stores 'sha256'
		Scopes:    []string{"viewer"},
		CreatedAt: 3000,
	}
	if err := s.CreateToken(ctx, nullAlgTok); err != nil {
		t.Fatalf("CreateToken (null alg): %v", err)
	}

	found, err := s.LookupToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupToken (null alg): %v", err)
	}
	if found == nil {
		t.Fatal("LookupToken (null alg): expected token, got nil")
	}
	if found.Name != "null-alg-row" {
		t.Errorf("LookupToken (null alg): wrong row: %+v", found)
	}
}

// TestHMACToken_WrongTokenRejected verifies that LookupToken returns nil for
// a token that was never stored.
func TestHMACToken_WrongTokenRejected(t *testing.T) {
	s := openStoreWithKey(t, "explicit-test-key-005")
	ctx := context.Background()

	found, err := s.LookupToken(ctx, "plt_doesnotexist_xxxxxxxxxxxxxxxx")
	if err != nil {
		t.Fatalf("LookupToken (wrong): %v", err)
	}
	if found != nil {
		t.Errorf("LookupToken (wrong): expected nil, got %+v", found)
	}
}

// TestHMACToken_RevocationWorks verifies that DeleteToken removes a token by ID
// for both HMAC and legacy SHA-256 rows.
func TestHMACToken_RevocationWorks(t *testing.T) {
	s := openStoreWithKey(t, "explicit-test-key-006")
	ctx := context.Background()

	// Create an HMAC token.
	rawHMAC := "plt_revoke_hmac_abcdef1234567890"
	hmacHash, hmacAlg := s.HashToken(rawHMAC)
	hmacTok := meta.APIToken{
		Kind:      "api",
		Name:      "hmac-revoke",
		TokenHash: hmacHash,
		HashAlg:   hmacAlg,
		Scopes:    []string{"admin"},
		CreatedAt: 4000,
	}
	if err := s.CreateToken(ctx, hmacTok); err != nil {
		t.Fatalf("CreateToken (hmac revoke): %v", err)
	}

	// Create a legacy SHA-256 token.
	rawLegacy := "plt_revoke_legacy_abcdef1234567890"
	legacyTok := meta.APIToken{
		Kind:      "api",
		Name:      "legacy-revoke",
		TokenHash: legacySHA256Hex(rawLegacy),
		HashAlg:   "sha256",
		Scopes:    []string{"admin"},
		CreatedAt: 5000,
	}
	if err := s.CreateToken(ctx, legacyTok); err != nil {
		t.Fatalf("CreateToken (legacy revoke): %v", err)
	}

	// Find and delete HMAC token.
	foundHMAC, _ := s.LookupToken(ctx, rawHMAC)
	if foundHMAC == nil {
		t.Fatal("setup: HMAC token not found before revocation")
	}
	if err := s.DeleteToken(ctx, foundHMAC.ID); err != nil {
		t.Fatalf("DeleteToken (hmac): %v", err)
	}
	afterRevoke, _ := s.LookupToken(ctx, rawHMAC)
	if afterRevoke != nil {
		t.Error("HMAC token still found after revocation")
	}

	// Find and delete legacy token.
	foundLegacy, _ := s.LookupToken(ctx, rawLegacy)
	if foundLegacy == nil {
		t.Fatal("setup: legacy token not found before revocation")
	}
	if err := s.DeleteToken(ctx, foundLegacy.ID); err != nil {
		t.Fatalf("DeleteToken (legacy): %v", err)
	}
	afterRevokeLegacy, _ := s.LookupToken(ctx, rawLegacy)
	if afterRevokeLegacy != nil {
		t.Error("legacy token still found after revocation")
	}
}

// TestHMACToken_EmptyKeyFallback verifies that a store opened with empty secret
// key creates tokens with hash_alg='sha256' (dev-mode fallback).
func TestHMACToken_EmptyKeyFallback(t *testing.T) {
	ctx := context.Background()
	s, err := meta.New(ctx, "sqlite", ":memory:", "")
	if err != nil {
		t.Fatalf("meta.New (empty key): %v", err)
	}
	defer s.Close()
	if err := s.MigrateEmbedded(ctx, readMetaDDL(t)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rawToken := "plt_devmode_abcdef1234567890"
	hash, alg := s.HashToken(rawToken)
	if alg != "sha256" {
		t.Errorf("EmptyKeyFallback: expected alg='sha256', got %q", alg)
	}
	// The hash must equal the plain SHA-256.
	if hash != legacySHA256Hex(rawToken) {
		t.Error("EmptyKeyFallback: expected plain SHA-256 hash for empty-key store")
	}
}

// TestHMACToken_MigrationUpgrade verifies the ALTER TABLE upgrade path:
//  1. Create a SQLite file from the OLD 0001_init.sql (no hash_alg column).
//  2. Insert a legacy token row via raw SQL (simulates a live prod row).
//  3. Run MigrateEmbedded with the NEW DDL (triggers applySchemaUpgrades).
//  4. Assert: LookupToken finds the legacy row; HashAlg defaults to 'sha256'.
func TestHMACToken_MigrationUpgrade(t *testing.T) {
	// Build old DDL (no hash_alg) by stripping the column from current DDL.
	currentDDL := readMetaDDL(t)
	oldDDL := stripHashAlgColumn(currentDDL)
	// Skip if the hash_alg COLUMN DECLARATION is still present (strip failed).
	if hasHashAlgColumn(oldDDL) {
		t.Skip("could not build old DDL without hash_alg column for migration test")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "upgrade_test.db")
	ctx := context.Background()

	rawToken := "plt_oldprod_abcdef1234567890"
	legacyHash := legacySHA256Hex(rawToken)

	// Step 1: Set up old-schema DB using raw sql.Open (bypasses meta.New
	// so we can apply old DDL without triggering the new code paths).
	func() {
		rawDB, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
		if err != nil {
			t.Fatalf("sql.Open (old schema): %v", err)
		}
		defer rawDB.Close()

		// Apply old DDL.
		for _, stmt := range splitSQLStatements(oldDDL) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
				// Tolerate PRAGMA errors (not supported on all backends).
				if strings.Contains(err.Error(), "PRAGMA") {
					continue
				}
				t.Fatalf("apply old DDL stmt %q: %v", stmt[:min(len(stmt), 60)], err)
			}
		}

		// Insert a legacy token row without hash_alg column.
		_, err = rawDB.ExecContext(ctx,
			`INSERT INTO api_tokens (id, user_id, kind, name, token_hash, scopes, created_at)
			 VALUES ('legacy-id-001', NULL, 'api', 'prod-admin', ?, '["admin"]', 9000)`,
			legacyHash)
		if err != nil {
			t.Fatalf("insert legacy row: %v", err)
		}
	}()

	// Step 2: Re-open with meta.New + apply NEW DDL (triggers ALTER TABLE upgrade).
	newStore, err := meta.New(ctx, "sqlite", dbPath, "upgrade-secret-key")
	if err != nil {
		t.Fatalf("meta.New (re-open): %v", err)
	}
	defer newStore.Close()

	if err := newStore.MigrateEmbedded(ctx, currentDDL); err != nil {
		t.Fatalf("MigrateEmbedded (new DDL): %v — migration upgrade failed", err)
	}

	// Step 3: Assert that LookupToken finds the legacy row.
	// If the hash_alg column was NOT added, the SELECT would fail at runtime.
	found, err := newStore.LookupToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("LookupToken (legacy after upgrade): %v", err)
	}
	if found == nil {
		t.Fatal("LookupToken (legacy after upgrade): expected token, got nil — legacy row lost or migration broken")
	}
	if found.Name != "prod-admin" {
		t.Errorf("wrong token returned after upgrade: %+v", found)
	}
	// The default hash_alg for the pre-existing row should be 'sha256'.
	if found.HashAlg != "sha256" {
		t.Errorf("expected HashAlg=%q for upgraded legacy row, got %q", "sha256", found.HashAlg)
	}
}

// ─── test helpers ─────────────────────────────────────────────────────────────

// stripHashAlgColumn removes the hash_alg column declaration and any comment
// lines that reference only hash_alg from the api_tokens DDL block to simulate
// a pre-migration schema file.
//
// It matches only lines where hash_alg is the COLUMN NAME (trimmed line starts
// with "hash_alg"), not lines that merely mention hash_alg in a comment.
func stripHashAlgColumn(ddl string) string {
	var out strings.Builder
	for _, line := range strings.Split(ddl, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip the hash_alg column declaration line.
		if strings.HasPrefix(trimmed, "hash_alg") {
			continue
		}
		// Skip comment-only lines that are solely about hash_alg
		// (e.g. "-- hash_alg: 'hmac-sha256'...").
		if strings.HasPrefix(trimmed, "--") && strings.Contains(trimmed, "hash_alg") &&
			!strings.Contains(trimmed, "token_hash") {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// splitSQLStatements splits a SQL script on semicolons (simplified).
func splitSQLStatements(script string) []string {
	var stmts []string
	var buf strings.Builder
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if strings.HasSuffix(strings.TrimSpace(line), ";") {
			stmts = append(stmts, buf.String())
			buf.Reset()
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// hasHashAlgColumn reports whether the DDL contains a hash_alg column
// declaration (trimmed line starting with "hash_alg ").
func hasHashAlgColumn(ddl string) bool {
	for _, line := range strings.Split(ddl, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "hash_alg ") || strings.HasPrefix(trimmed, "hash_alg\t") {
			return true
		}
	}
	return false
}
