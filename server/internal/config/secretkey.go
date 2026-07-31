package config

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// canonicalSecretKeyHexLen is the length of the key form that
// meta.deriveKey consumes DIRECTLY, with no hashing: 64 hex characters
// decoding to 32 bytes. This is what `openssl rand -hex 32` emits and what
// deploy/quickstart/install.sh generates.
const canonicalSecretKeyHexLen = 64

// minSecretKeyBytes is the floor below which a key is rejected outright.
// It is a floor on LENGTH, which is not the same as a floor on entropy — see
// the warning path in ValidateSecretKey.
const minSecretKeyBytes = 16

// ValidateSecretKey is the single source of truth for PULSE_SECRET_KEY
// validation. It returns operator-facing warnings (never fatal) and an error
// for conditions that must stop startup.
//
// WHY THIS IS ONE FUNCTION. These checks were copy-pasted into four call sites
// (cmd/pulse/main.go runMigrate and runReconcile, cmd/pulse/serve.go newServer,
// and config.validate) and had already drifted: two of the four never gained
// the "changeme" placeholder check, so a placeholder key that `pulse serve`
// refused was accepted by `pulse reconcile`.
//
// THE WARNING PATH, AND WHY IT IS ONLY A WARNING.
// meta.deriveKey has two branches for a non-empty key:
//
//	64 hex chars   -> hex-decoded and used directly as the AES-256-GCM key
//	anything else  -> SHA-256(key) used as the AES-256-GCM key
//
// SHA-256 is a fast hash. A passphrase that merely clears the 16-byte floor
// ("correcthorsebattery1") has far less entropy than 256 bits, so if the meta
// database is ever exfiltrated, the encryption key is recoverable by dictionary
// attack. That is CodeQL alert #6 (go/weak-sensitive-data-hashing), and on the
// non-hex branch the alert is correct.
//
// It is a warning rather than an error, and deriveKey is deliberately NOT
// changed, because both stricter options break running deployments:
//
//   - Rejecting non-hex keys refuses to boot deployments already using one —
//     and their data is encrypted under exactly that derivation, so refusing to
//     start is refusing to let them read their own credentials.
//   - Switching deriveKey to a slow KDF (Argon2id/PBKDF2) changes the derived
//     key, making every existing credential_enc, webhook_secret_enc and
//     config_enc blob undecryptable. No `pulse rekey` command exists yet to
//     re-encrypt under a new key; ADR-0004 defers it.
//
// So the honest move is: keep every deployment working, and tell the operator
// precisely what their choice costs and how to fix it.
func ValidateSecretKey(key, metaDSN string) (warnings []string, err error) {
	// ":memory:" is the dev/test path — nothing is persisted, so nothing needs
	// encrypting and no key is required.
	if metaDSN == ":memory:" {
		return nil, nil
	}

	if len(key) == 0 {
		return nil, fmt.Errorf("PULSE_SECRET_KEY must be set (min %d bytes); generate with: openssl rand -hex 32", minSecretKeyBytes)
	}
	if len(key) < minSecretKeyBytes {
		return nil, fmt.Errorf("PULSE_SECRET_KEY is too short (%d bytes); minimum is %d bytes; generate with: openssl rand -hex 32", len(key), minSecretKeyBytes)
	}
	// Substring match, not equality: .env.example ships
	// "changeme-generate-with-openssl-rand-hex-32", and a half-hearted edit that
	// leaves "changeme" embedded anywhere is still a placeholder.
	if strings.Contains(strings.ToLower(key), "changeme") {
		return nil, fmt.Errorf("PULSE_SECRET_KEY appears to be a placeholder value; generate a real key with: openssl rand -hex 32")
	}

	// Mirror meta.deriveKey's hex test EXACTLY — length 64 AND hex-decodable to
	// 32 bytes. Approximating it here (e.g. checking only the length, or only
	// that the characters look hexish) would tell an operator they are on the
	// direct-key path when deriveKey is actually hashing their key.
	if len(key) == canonicalSecretKeyHexLen {
		if b, decErr := hex.DecodeString(key); decErr == nil && len(b) == 32 {
			return nil, nil
		}
	}

	warnings = append(warnings, fmt.Sprintf(
		"PULSE_SECRET_KEY is not a 64-character hex string, so it is converted to an "+
			"encryption key with a single SHA-256 pass rather than used directly. SHA-256 is "+
			"fast, so a guessable passphrase yields a guessable key if your meta database is "+
			"ever copied. Length alone is not entropy. Prefer: openssl rand -hex 32. "+
			"(Changing this key makes existing encrypted credentials unreadable, so rotate it "+
			"deliberately, not casually.)"))
	return warnings, nil
}
