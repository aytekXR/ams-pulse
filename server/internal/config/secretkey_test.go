package config

import (
	"strings"
	"testing"
)

// ValidateSecretKey is the single implementation of PULSE_SECRET_KEY validation.
//
// It exists because the same checks were copy-pasted into FOUR places
// (cmd/pulse/main.go runMigrate, cmd/pulse/main.go runReconcile,
// cmd/pulse/serve.go newServer, and config.validate) and had already drifted:
// two of the four never gained the "changeme" placeholder check, so a
// placeholder key that `pulse serve` rejected was accepted by `pulse reconcile`.
//
// It also carries the mitigation for CodeQL alert #6
// (go/weak-sensitive-data-hashing, meta.go:1649). meta.deriveKey uses a
// 64-hex-char PULSE_SECRET_KEY DIRECTLY as the AES-256-GCM key, but SHA-256
// hashes anything else. SHA-256 is a fast hash, so a low-entropy passphrase
// that merely clears the 16-byte floor yields a brute-forceable key if the meta
// database is ever exfiltrated.
//
// The fix is deliberately a WARNING, not an error, and deliberately not a
// change to deriveKey:
//   - Erroring would refuse to boot deployments that are already running on a
//     non-hex key, and their data is encrypted under exactly that derivation.
//   - Switching deriveKey to a slow KDF would change the derived key and make
//     every existing credential_enc / webhook_secret_enc / config_enc blob
//     undecryptable. There is no `pulse rekey` command yet (ADR-0004 defers it).
//
// So: warn, name the consequence, and leave every existing deployment working.

func TestValidateSecretKey_Errors(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		dsn     string
		wantErr string
	}{
		{"empty is rejected", "", "/var/lib/pulse/pulse.db", "must be set"},
		{"under 16 bytes is rejected", "short", "/var/lib/pulse/pulse.db", "too short"},
		{"exactly 15 bytes is rejected", strings.Repeat("a", 15), "/var/lib/pulse/pulse.db", "too short"},
		{"placeholder is rejected", "changeme-generate-with-openssl", "/var/lib/pulse/pulse.db", "placeholder"},
		{"placeholder is case-insensitive", "ChangeMe-Generate-With-Openssl", "/var/lib/pulse/pulse.db", "placeholder"},
		// This is the drift that motivated extracting the function: two of the
		// four copies accepted a placeholder outright.
		{"placeholder embedded mid-string is rejected", "prefixCHANGEMEsuffix-padded", "/var/lib/pulse/pulse.db", "placeholder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSecretKey(tt.key, tt.dsn)
			if err == nil {
				t.Fatalf("ValidateSecretKey(%q) = nil error, want error containing %q", tt.key, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateSecretKey_MemoryDSNIsExempt(t *testing.T) {
	// :memory: is the dev/test path — no persistence, so no key requirement.
	for _, key := range []string{"", "short", "changeme"} {
		if _, err := ValidateSecretKey(key, ":memory:"); err != nil {
			t.Errorf("ValidateSecretKey(%q, \":memory:\") = %v, want nil", key, err)
		}
	}
}

func TestValidateSecretKey_CanonicalHexIsClean(t *testing.T) {
	// 64 hex chars — what `openssl rand -hex 32` produces and what
	// deploy/quickstart/install.sh generates. meta.deriveKey uses these bytes
	// DIRECTLY, so SHA-256 is never involved and there is nothing to warn about.
	key := "a3f1" + strings.Repeat("0", 60)
	warnings, err := ValidateSecretKey(key, "/var/lib/pulse/pulse.db")
	if err != nil {
		t.Fatalf("canonical hex key rejected: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("canonical hex key produced warnings %v, want none", warnings)
	}
}

func TestValidateSecretKey_NonHexWarnsAboutSHA256Derivation(t *testing.T) {
	// The CodeQL #6 mitigation. A 20-char passphrase clears the 16-byte floor
	// and is NOT rejected — existing deployments must keep booting — but the
	// operator is told that it goes through SHA-256 and what that costs them.
	warnings, err := ValidateSecretKey("correcthorsebattery1", "/var/lib/pulse/pulse.db")
	if err != nil {
		t.Fatalf("non-hex key must WARN, not error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("non-hex key produced no warning; the SHA-256 derivation path is unflagged")
	}
	joined := strings.Join(warnings, " ")
	for _, want := range []string{"SHA-256", "openssl rand -hex 32"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warning %q does not mention %q — an operator cannot act on it", joined, want)
		}
	}
}

func TestValidateSecretKey_HexOfWrongLengthStillWarns(t *testing.T) {
	// Hex characters but not 64 of them: deriveKey's hex branch requires
	// len == 64 exactly, so this falls through to SHA-256 like any other string.
	// Getting this wrong would tell an operator they are safe when they are not.
	warnings, err := ValidateSecretKey(strings.Repeat("ab", 20), "/var/lib/pulse/pulse.db") // 40 hex chars
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("40-char hex key produced no warning, but deriveKey SHA-256s anything that is not exactly 64 hex chars")
	}
}

func TestValidateSecretKey_SixtyFourNonHexCharsStillWarns(t *testing.T) {
	// 64 characters that are NOT valid hex also fall through to SHA-256,
	// because deriveKey requires hex.DecodeString to succeed.
	warnings, err := ValidateSecretKey(strings.Repeat("z", 64), "/var/lib/pulse/pulse.db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("64 non-hex chars produced no warning, but hex.DecodeString fails on them so deriveKey uses SHA-256")
	}
}

func TestValidateSecretKey_UppercaseHexIsAccepted(t *testing.T) {
	// hex.DecodeString accepts uppercase, so deriveKey uses it directly and
	// there is nothing to warn about. Warning here would be a false alarm.
	warnings, err := ValidateSecretKey(strings.Repeat("AB", 32), "/var/lib/pulse/pulse.db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("uppercase hex produced warnings %v; hex.DecodeString accepts it, so deriveKey uses it directly", warnings)
	}
}
