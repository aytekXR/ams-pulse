// Package license_test — D-089 expiry lifecycle tests.
//
// TDD: these tests were written against the OLD code (no lazy expiry check) and
// are expected to be RED until the implementation in license.go is updated.
package license_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/license"
)

// ─── Log-capture helper ───────────────────────────────────────────────────────

// captureHandler is a minimal slog.Handler that collects warn-level messages.
type captureHandler struct {
	mu    sync.Mutex
	warns []string
}

func (h *captureHandler) Enabled(_ context.Context, l slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.mu.Lock()
		h.warns = append(h.warns, r.Message)
		h.mu.Unlock()
	}
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) WarnCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.warns)
}

func (h *captureHandler) WarnMessages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.warns))
	copy(out, h.warns)
	return out
}

// installCaptureLogger swaps in a capturing logger and restores slog.Default on cleanup.
func installCaptureLogger(t *testing.T) *captureHandler {
	t.Helper()
	h := &captureHandler{}
	license.SetLogger(slog.New(h))
	t.Cleanup(func() { license.SetLogger(slog.Default()) })
	return h
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestMidRunExpiry_DegradesToFree: mint key expiring in 1h, activate (assert pro tier
// + valid=true), swap now() to T+2h, assert Tier()==free, Valid()==false,
// ExpiresAt() retained non-nil, CheckDataAPI/CheckBeaconIngest now refuse,
// Warn log emitted exactly once.
func TestMidRunExpiry_DegradesToFree(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)

	h := installCaptureLogger(t)

	// Key expires in 1 hour from real now.
	expiresInMs := time.Now().Add(1 * time.Hour).UnixMilli()
	key := kf.signKey(t, map[string]interface{}{
		"tier":       "pro",
		"data_api":   true,
		"expires_at": expiresInMs,
	})

	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}

	// Pre-expiry: clock is real (< expires_at).
	if got := mgr.Tier(); got != license.TierPro {
		t.Fatalf("pre-expiry: want tier=%q got %q", license.TierPro, got)
	}
	if !mgr.Valid() {
		t.Error("pre-expiry: want valid=true")
	}
	if mgr.ExpiresAt() == nil {
		t.Fatal("pre-expiry: want non-nil ExpiresAt")
	}
	if h.WarnCount() != 0 {
		t.Errorf("pre-expiry: want 0 warn logs, got %d: %v", h.WarnCount(), h.WarnMessages())
	}

	// Advance clock to T+2h (past expires_at).
	license.SetNow(func() time.Time { return time.Now().Add(2 * time.Hour) })
	t.Cleanup(func() { license.SetNow(time.Now) })

	// Post-expiry: all reads must reflect downgraded state.
	if got := mgr.Tier(); got != license.TierFree {
		t.Errorf("post-expiry: Tier want %q got %q", license.TierFree, got)
	}
	if mgr.Valid() {
		t.Error("post-expiry: want valid=false")
	}
	if mgr.ExpiresAt() == nil {
		t.Error("post-expiry: ExpiresAt must be retained (non-nil)")
	}

	// Gated checks must refuse on the now-free tier.
	if err := mgr.CheckDataAPI(); err == nil {
		t.Error("post-expiry: CheckDataAPI must return error (free tier has no DataAPI)")
	}
	if err := mgr.CheckBeaconIngest(); err == nil {
		t.Error("post-expiry: CheckBeaconIngest must return error (free tier blocked)")
	}

	// Warn emitted exactly once.
	if got := h.WarnCount(); got != 1 {
		t.Errorf("want exactly 1 warn log, got %d: %v", got, h.WarnMessages())
	}
	if msgs := h.WarnMessages(); len(msgs) > 0 && msgs[0] != "license: expired — degraded to free tier" {
		t.Errorf("warn message: got %q want %q", msgs[0], "license: expired — degraded to free tier")
	}

	// Idempotency: subsequent reads must NOT emit another warn.
	_ = mgr.Tier()
	_ = mgr.Valid()
	if got := h.WarnCount(); got != 1 {
		t.Errorf("idempotency: want 1 warn log after repeated reads, got %d", got)
	}
}

// TestMidRunExpiry_ChecksAlsoDowngrade: expiry observed through a Check* method
// FIRST (not Tier) — proves every public reader triggers the lazy check.
func TestMidRunExpiry_ChecksAlsoDowngrade(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)

	_ = installCaptureLogger(t)

	expiresInMs := time.Now().Add(1 * time.Hour).UnixMilli()
	key := kf.signKey(t, map[string]interface{}{
		"tier":       "pro",
		"data_api":   true,
		"expires_at": expiresInMs,
	})

	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}

	// Confirm pre-expiry is pro (ensures the key was accepted).
	if got := mgr.Tier(); got != license.TierPro {
		t.Fatalf("pre-expiry: want %q got %q", license.TierPro, got)
	}

	// Advance clock.
	license.SetNow(func() time.Time { return time.Now().Add(2 * time.Hour) })
	t.Cleanup(func() { license.SetNow(time.Now) })

	// Observe downgrade via CheckDataAPI (NOT Tier) first.
	// On the old code this returns nil because the tier is still "pro" (never updated).
	if err := mgr.CheckDataAPI(); err == nil {
		t.Error("CheckDataAPI: must return error after expiry (free tier lacks DataAPI)")
	}

	// Verify consistent state after the check triggered expiry.
	if got := mgr.Tier(); got != license.TierFree {
		t.Errorf("after CheckDataAPI: want tier=%q got %q", license.TierFree, got)
	}
	if mgr.Valid() {
		t.Error("after CheckDataAPI: want valid=false")
	}
}

// TestBootExpiredKey_HonestState: New() with an already-expired key must produce
// tier=free, valid=false, ExpiresAt() retained non-nil.
// Replaces the comment "activate() returns an error for expired keys; New() fails
// open → Free tier" — the NEW semantics: activate succeeds, lazy check degrades on
// first read, expiresAt is preserved.
func TestBootExpiredKey_HonestState(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)

	_ = installCaptureLogger(t)

	pastMs := time.Now().Add(-24 * time.Hour).UnixMilli()
	key := kf.signKey(t, map[string]interface{}{
		"tier":       "pro",
		"data_api":   true,
		"expires_at": pastMs,
	})

	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New returned unexpected error (fail-open must be preserved): %v", err)
	}

	// First read triggers the lazy expiry check.
	if got := mgr.Tier(); got != license.TierFree {
		t.Errorf("boot expired key: Tier want %q got %q", license.TierFree, got)
	}
	if mgr.Valid() {
		t.Error("boot expired key: want valid=false (honest state, not silent free)")
	}
	if mgr.ExpiresAt() == nil {
		t.Error("boot expired key: ExpiresAt must be retained (non-nil)")
	}
	if exp := mgr.ExpiresAt(); exp != nil && !time.Now().After(*exp) {
		t.Errorf("boot expired key: ExpiresAt must be in the past, got %v", *exp)
	}
}

// TestNoKey_SemanticsUnchanged: pins current no-key behavior.
// KEEP: valid=true, expiresAt=nil, tier=free — the free tier is always valid.
func TestNoKey_SemanticsUnchanged(t *testing.T) {
	mgr, err := license.New("", "")
	if err != nil {
		t.Fatalf("license.New('','') error: %v", err)
	}

	if got := mgr.Tier(); got != license.TierFree {
		t.Errorf("no-key: want tier=%q got %q", license.TierFree, got)
	}
	if !mgr.Valid() {
		t.Error("no-key: want valid=true (free tier is always valid; semantics unchanged)")
	}
	if got := mgr.ExpiresAt(); got != nil {
		t.Errorf("no-key: want ExpiresAt=nil, got %v", *got)
	}
}

// TestPerpetualKey_NeverDegrades: a key with no expires_at must not degrade
// regardless of how far forward the clock is moved.
func TestPerpetualKey_NeverDegrades(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)

	_ = installCaptureLogger(t)

	key := kf.signKey(t, map[string]interface{}{
		"tier":     "pro",
		"data_api": true,
		// no expires_at → perpetual
	})

	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}

	// Move clock 10 years into the future.
	license.SetNow(func() time.Time { return time.Now().Add(10 * 365 * 24 * time.Hour) })
	t.Cleanup(func() { license.SetNow(time.Now) })

	if got := mgr.Tier(); got != license.TierPro {
		t.Errorf("perpetual key: want tier=%q got %q", license.TierPro, got)
	}
	if !mgr.Valid() {
		t.Error("perpetual key: want valid=true (no expiry in claims)")
	}
	if got := mgr.ExpiresAt(); got != nil {
		t.Errorf("perpetual key: want ExpiresAt=nil, got %v", *got)
	}
	// No warn should have been emitted.
	if h := installCaptureLogger(t); h.WarnCount() != 0 {
		t.Errorf("perpetual key: want 0 warn logs, got %d", h.WarnCount())
	}
}
