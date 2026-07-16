// Package api — S47 (D-109) internal test for the password-hashing hardening.
// Uses package api (not api_test) to reach the unexported hashPassword.
package api

import (
	"strings"
	"testing"
)

// TestHashPassword_NoSHA256Fallback: hashPassword must never emit a fast SHA-256
// digest for a password (CWE-916, flagged by CodeQL on the S47 diff). bcrypt
// rejects inputs over 72 bytes; on that error hashPassword now fails closed with
// "" rather than a crackable "sha256:" hash. checkPassword keeps verifying legacy
// sha256: rows already in the DB (tested in wo4_internal_test.go).
//
// Mutation: restore the SHA-256 fallback in hashPassword → h becomes "sha256:..."
// for the >72-byte input → this test goes RED.
func TestHashPassword_NoSHA256Fallback(t *testing.T) {
	long := strings.Repeat("a", 100) // > 72 bytes → bcrypt returns an error
	h := hashPassword(long)
	if strings.HasPrefix(h, "sha256:") {
		t.Errorf("hashPassword fell back to a SHA-256 password hash: %q", h)
	}
	if h != "" {
		t.Errorf("hashPassword(>72 bytes) = %q; want \"\" (fail closed, no weak hash)", h[:min(20, len(h))])
	}
	// A normal password still yields a bcrypt hash ($2a/$2b...).
	if bh := hashPassword("normal-password"); len(bh) < 4 || bh[0] != '$' {
		t.Errorf("expected a bcrypt hash for a normal password, got %q", bh)
	}
}
