// Package api — WO-4 internal tests for unexported helpers.
// Tests in this file use package api (not api_test) to access unexported symbols.
package api

import (
	"testing"
	"time"
)

// ─── checkPassword ────────────────────────────────────────────────────────────

// TestCheckPassword_Bcrypt verifies checkPassword against a bcrypt hash.
func TestCheckPassword_Bcrypt(t *testing.T) {
	pw := "my-secure-password"
	stored := hashPassword(pw)

	if stored == "" {
		t.Fatal("hashPassword returned empty string")
	}

	// Correct password must match.
	if !checkPassword(pw, stored) {
		t.Errorf("checkPassword returned false for correct bcrypt password")
	}
	t.Logf("PASS: checkPassword (bcrypt) → correct password matches stored hash")

	// Wrong password must not match.
	if checkPassword("wrong-password", stored) {
		t.Errorf("checkPassword returned true for wrong password")
	}
	t.Logf("PASS: checkPassword (bcrypt) → wrong password correctly rejected")
}

// TestCheckPassword_SHA256Legacy verifies checkPassword against legacy sha256: hashes.
func TestCheckPassword_SHA256Legacy(t *testing.T) {
	pw := "legacy-password"
	// Manually construct a sha256: hash (legacy format).
	stored := sha256Hex(pw)
	legacyStored := "sha256:" + stored

	if !checkPassword(pw, legacyStored) {
		t.Errorf("checkPassword returned false for correct sha256 password")
	}
	t.Logf("PASS: checkPassword (sha256) → correct password matches")

	if checkPassword("other-password", legacyStored) {
		t.Errorf("checkPassword returned true for wrong password against sha256 hash")
	}
	t.Logf("PASS: checkPassword (sha256) → wrong password correctly rejected")
}

// TestCheckPassword_Empty verifies behavior with empty passwords.
func TestCheckPassword_Empty(t *testing.T) {
	// Empty password + empty stored → match.
	if !checkPassword("", "") {
		t.Errorf("checkPassword: empty vs empty should match")
	}
	// Non-empty password + empty stored → no match.
	if checkPassword("something", "") {
		t.Errorf("checkPassword: non-empty vs empty should not match")
	}
	t.Logf("PASS: checkPassword empty cases work correctly")
}

// ─── hashPassword ─────────────────────────────────────────────────────────────

// TestHashPassword_Bcrypt verifies hashPassword produces a bcrypt hash.
func TestHashPassword_Bcrypt(t *testing.T) {
	pw := "test-password"
	h := hashPassword(pw)
	if h == "" {
		t.Fatal("hashPassword returned empty string")
	}
	// bcrypt hashes start with $2a$ or $2b$.
	if len(h) < 4 || (h[0] != '$') {
		t.Errorf("expected bcrypt hash starting with $, got %q", h[:min(10, len(h))])
	}
	t.Logf("PASS: hashPassword produced bcrypt hash (len=%d)", len(h))
}

// TestHashPassword_Empty verifies empty password returns empty hash.
func TestHashPassword_Empty(t *testing.T) {
	h := hashPassword("")
	if h != "" {
		t.Errorf("expected empty hash for empty password, got %q", h)
	}
	t.Logf("PASS: hashPassword('') → ''")
}

// ─── Rate-limit eviction ──────────────────────────────────────────────────────

// TestRateLimitEviction exercises startEviction and evictOnce.
func TestRateLimitEviction(t *testing.T) {
	kl := newKeyedLimiter(100, 200)

	// Allow some keys to prime the limiter's internal map.
	kl.Allow("key-a")
	kl.Allow("key-b")
	kl.Allow("key-a") // second access

	// Run eviction directly with a very short idle window: all keys idle
	// relative to lastUsed < now.Add(-0) which should NOT evict anything
	// immediately (zero window).
	kl.evictOnce(0) // idleWindow=0 → all idle
	t.Logf("PASS: evictOnce(0) ran without panic")

	// startEviction: launch background goroutine and immediately stop it.
	stop := kl.startEviction(5*time.Millisecond, 1*time.Millisecond)
	// Let the goroutine tick at least once.
	time.Sleep(20 * time.Millisecond)
	stop()
	t.Logf("PASS: startEviction goroutine started and stopped cleanly")
}

// ─── Utility helpers ──────────────────────────────────────────────────────────

// TestJsonOrEmpty verifies jsonOrEmpty handles edge cases.
func TestJsonOrEmpty(t *testing.T) {
	// Empty string → empty map.
	v := jsonOrEmpty("")
	if m, ok := v.(map[string]any); !ok || len(m) != 0 {
		t.Errorf("jsonOrEmpty('') expected empty map{}, got %v", v)
	}
	// Valid JSON → parsed value.
	v2 := jsonOrEmpty(`{"k":"v"}`)
	m2, ok := v2.(map[string]any)
	if !ok {
		t.Errorf("jsonOrEmpty valid JSON expected map, got %T", v2)
	} else if m2["k"] != "v" {
		t.Errorf("jsonOrEmpty valid JSON: expected k=v, got %v", m2["k"])
	}
	// Invalid JSON → empty map.
	v3 := jsonOrEmpty(`{bad`)
	if m3, ok := v3.(map[string]any); !ok || len(m3) != 0 {
		t.Errorf("jsonOrEmpty invalid JSON expected empty map{}, got %v", v3)
	}
	t.Logf("PASS: jsonOrEmpty handles empty, valid, and invalid JSON")
}

// TestNullableInt verifies nullableInt returns nil for negative values.
func TestNullableInt(t *testing.T) {
	if nullableInt(-1) != nil {
		t.Errorf("nullableInt(-1) should be nil")
	}
	if nullableInt(0) != 0 {
		t.Errorf("nullableInt(0) should be 0")
	}
	if nullableInt(5) != 5 {
		t.Errorf("nullableInt(5) should be 5")
	}
	t.Logf("PASS: nullableInt works correctly")
}

// TestParseTimeRange verifies time range parsing edge cases.
func TestParseTimeRange(t *testing.T) {
	// Both empty → defaults (last 7 days).
	from, to := parseTimeRange("", "")
	if from.IsZero() || to.IsZero() {
		t.Error("expected non-zero defaults for empty from/to")
	}
	if !to.After(from) {
		t.Error("expected to > from for defaults")
	}

	// Valid epoch ms.
	from2, to2 := parseTimeRange("1000000", "2000000")
	if from2.UnixMilli() != 1000000 {
		t.Errorf("expected from=1000000ms, got %d", from2.UnixMilli())
	}
	if to2.UnixMilli() != 2000000 {
		t.Errorf("expected to=2000000ms, got %d", to2.UnixMilli())
	}

	// RFC3339 strings.
	from3, _ := parseTimeRange("2026-01-01T00:00:00Z", "")
	if from3.Year() != 2026 {
		t.Errorf("expected year=2026 from RFC3339 from, got %d", from3.Year())
	}

	t.Logf("PASS: parseTimeRange handles empty, epoch-ms, and RFC3339")
}

// min returns the smaller of a and b (used in TestHashPassword_Bcrypt).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
