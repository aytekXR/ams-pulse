// Package license_test — D-133 (S71) tests for the license cluster:
//
//	[12] New() logs (no longer silently discards) activation/read failures.
//	[23] activate() rejects an unknown tier → New falls open to Free.
//	[24] the dev-mode pubkey fallback wraps the real GenerateKey error (err2).
package license_test

import (
	"crypto/ed25519"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/license"
)

func warnsContain(warns []string, sub string) bool {
	for _, w := range warns {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

// TestNew_InvalidInlineKey_LogsWarning pins [12]: a rejected inline key must be
// logged at Warn (so the operator can tell "key rejected" from "no key"), not
// silently discarded, while New still fails open to Free.
func TestNew_InvalidInlineKey_LogsWarning(t *testing.T) {
	h := installCaptureLogger(t)

	mgr, err := license.New("not.aValidSignedKey", "") // signature verification fails
	if err != nil {
		t.Fatalf("New must fail open (nil error), got %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Errorf("tier after invalid key = %q, want free", mgr.Tier())
	}
	if !warnsContain(h.WarnMessages(), "activation failed") {
		t.Errorf("expected a Warn about activation failure, got %v", h.WarnMessages())
	}
}

// TestNew_OfflineFile_LogsWarning pins [12] for both offline sub-branches:
// unreadable file, and a readable file whose contents fail activation.
func TestNew_OfflineFile_LogsWarning(t *testing.T) {
	t.Run("unreadable", func(t *testing.T) {
		h := installCaptureLogger(t)
		mgr, err := license.New("", filepath.Join(t.TempDir(), "does-not-exist.key"))
		if err != nil {
			t.Fatalf("New must fail open, got %v", err)
		}
		if mgr.Tier() != license.TierFree {
			t.Errorf("tier = %q, want free", mgr.Tier())
		}
		if !warnsContain(h.WarnMessages(), "unreadable") {
			t.Errorf("expected a Warn about the unreadable offline file, got %v", h.WarnMessages())
		}
	})

	t.Run("bad-contents", func(t *testing.T) {
		h := installCaptureLogger(t)
		f := filepath.Join(t.TempDir(), "bad.key")
		if err := os.WriteFile(f, []byte("garbage-not-a-key"), 0o600); err != nil {
			t.Fatalf("write temp key: %v", err)
		}
		mgr, err := license.New("", f)
		if err != nil {
			t.Fatalf("New must fail open, got %v", err)
		}
		if mgr.Tier() != license.TierFree {
			t.Errorf("tier = %q, want free", mgr.Tier())
		}
		if !warnsContain(h.WarnMessages(), "activation failed") {
			t.Errorf("expected a Warn about offline activation failure, got %v", h.WarnMessages())
		}
	})
}

// TestActivate_UnknownTier_FallsBackToFree pins [23]: a validly-signed key whose
// tier is not one of the four known values must be rejected by activate() so New
// falls open to Free — not trusted as a privileged unknown tier with unlimited
// capacity. (The signature is valid; this models a vendor-side tier typo.)
func TestActivate_UnknownTier_FallsBackToFree(t *testing.T) {
	kf := generateKeys(t)
	kf.install(t)
	key := kf.signKey(t, map[string]interface{}{
		"tier":     "enterprise_lite", // unknown → must be rejected
		"data_api": true,
		// max_nodes / retention_days omitted → would map to -1 (unlimited) if trusted
	})

	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("New must fail open, got %v", err)
	}
	if mgr.Tier() != license.TierFree {
		t.Fatalf("unknown-tier key: tier = %q, want free (activate must reject it)", mgr.Tier())
	}
	// Capacity must be the Free defaults, not the unlimited (-1) grant.
	if ent := mgr.Entitlements(); ent.MaxNodes != 1 {
		t.Errorf("unknown-tier key: MaxNodes = %d, want 1 (free) — unlimited grant leaked", ent.MaxNodes)
	}
	if err := mgr.CheckProbes(); err == nil {
		t.Error("unknown-tier key: CheckProbes must refuse (free tier)")
	}
	if err := mgr.CheckBeaconIngest(); err == nil {
		t.Error("unknown-tier key: CheckBeaconIngest must refuse (free tier)")
	}
}

// TestNew_PubkeyFallbackGenerateKeyFailure_WrapsErr2 pins [24]: when
// PULSE_LICENSE_PUBKEY decodes to the wrong length (err==nil) and the dev-mode
// GenerateKey fallback then fails, New must wrap that real error (err2), not the
// nil hex-decode error — otherwise the diagnostic is "init public key: <nil>".
func TestNew_PubkeyFallbackGenerateKeyFailure_WrapsErr2(t *testing.T) {
	t.Setenv("PULSE_LICENSE_PUBKEY", "abcd") // decodes to 2 bytes (≠ 32) → err==nil, length mismatch

	sentinel := errors.New("injected generatekey failure")
	license.SetGenerateKey(func(io.Reader) (ed25519.PublicKey, ed25519.PrivateKey, error) {
		return nil, nil, sentinel
	})
	t.Cleanup(func() { license.SetGenerateKey(ed25519.GenerateKey) })

	_, err := license.New("", "")
	if err == nil {
		t.Fatal("expected New to fail when the pubkey fallback GenerateKey fails")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped err2 %v, got %v (regression: wraps the nil hex-decode err)", sentinel, err)
	}
}
