// Command licensegen mints a Pulse license key for CI or production use.
//
// Usage:
//
//	go run . [-tier free|pro|business|enterprise] [-privkey <path>] \
//	         [-expires <days>] [-expires-minutes <minutes>] >> "$GITHUB_ENV"
//
// Without -privkey it generates a fresh ed25519 key pair at runtime (CI mode).
// With -privkey it loads the hex-encoded 64-byte private key from <path> and
// derives the matching public key from it (production minting mode).
//
// Without -expires or -expires-minutes the license is perpetual (no expires_at claim).
// With -expires <days> (positive integer) it sets expires_at to
// time.Now().UTC() + days*24h, expressed as Unix epoch milliseconds.
// With -expires-minutes <minutes> (positive integer, mutually exclusive with -expires)
// it sets expires_at to time.Now().UTC() + minutes*1min — for live trial-flow demos.
//
// It signs a JSON claims blob and prints exactly two GITHUB_ENV-compatible
// lines to stdout:
//
//	PULSE_LICENSE_KEY=<base64(claimsJSON)>.<base64(ed25519sig)>
//	PULSE_LICENSE_PUBKEY=<hex-encoded-public-key>
//
// Diagnostics (including the claims JSON) are written to stderr only; stdout is
// the env-file payload.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// tierClaims returns the JSON-serialisable claims map for a given tier.
// Claim semantics mirror license.go:
//   - max_nodes / retention_days omitted (nil ptr) → unlimited in the parser.
//   - data_api drives CheckDataAPI; any tier except free sets it to true.
//   - CheckBeaconIngest only fails for the "free" tier string, so all non-free
//     tiers automatically pass regardless of other fields.
func tierClaims(tier string) (map[string]any, error) {
	switch tier {
	case "free":
		maxNodes := 1
		retDays := 7
		return map[string]any{
			"tier":           "free",
			"max_nodes":      maxNodes,
			"retention_days": retDays,
			"data_api":       false,
			"white_label":    false,
		}, nil
	case "pro":
		maxNodes := 10
		retDays := 90
		return map[string]any{
			"tier":           "pro",
			"max_nodes":      maxNodes,
			"retention_days": retDays,
			"data_api":       true,
			"white_label":    false,
		}, nil
	case "business":
		maxNodes := 5
		retDays := 396 // 13 months per tier table
		return map[string]any{
			"tier":           "business",
			"max_nodes":      maxNodes,
			"retention_days": retDays,
			"data_api":       true,
			"white_label":    false,
		}, nil
	case "enterprise":
		// 0 in claims → unlimited in buildEntitlements.
		return map[string]any{
			"tier":        "enterprise",
			"data_api":    true,
			"white_label": true,
		}, nil
	default:
		return nil, fmt.Errorf("unknown tier %q; valid values: free, pro, business, enterprise", tier)
	}
}

func run() error {
	tierFlag := flag.String("tier", "pro", "license tier: free|pro|business|enterprise")
	privkeyFlag := flag.String("privkey", "", "path to a file containing the hex-encoded ed25519 private key (128 hex chars = 64 bytes seed||pub)")
	expiresFlag := flag.Int("expires", 0, "license validity in days (positive integer); omit for perpetual")
	expiresMinutesFlag := flag.Int("expires-minutes", 0, "license validity in minutes (positive integer); mutually exclusive with -expires; for live trial-flow demos")
	flag.Parse()

	tier := *tierFlag

	// Detect whether -expires / -expires-minutes were explicitly supplied.
	// flag.Visit only iterates flags that were actually set; this is the only
	// reliable way to distinguish "not passed" from "passed with default value".
	var expiresExplicit, expiresMinutesExplicit bool
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "expires":
			expiresExplicit = true
		case "expires-minutes":
			expiresMinutesExplicit = true
		}
	})

	if expiresExplicit && expiresMinutesExplicit {
		return fmt.Errorf("-expires and -expires-minutes are mutually exclusive")
	}
	if expiresExplicit && *expiresFlag <= 0 {
		return fmt.Errorf("-expires must be a positive integer, got %d", *expiresFlag)
	}
	if expiresMinutesExplicit && *expiresMinutesFlag <= 0 {
		return fmt.Errorf("-expires-minutes must be a positive integer, got %d", *expiresMinutesFlag)
	}

	claims, err := tierClaims(tier)
	if err != nil {
		return err
	}

	// Set expires_at only when an expiry flag was explicitly provided.
	switch {
	case expiresExplicit:
		expiresAt := time.Now().UTC().Add(time.Duration(*expiresFlag) * 24 * time.Hour).UnixMilli()
		claims["expires_at"] = expiresAt
	case expiresMinutesExplicit:
		expiresAt := time.Now().UTC().Add(time.Duration(*expiresMinutesFlag) * time.Minute).UnixMilli()
		claims["expires_at"] = expiresAt
		fmt.Fprintf(os.Stderr, "licensegen: expires-minutes=%d (demo/test mode)\n", *expiresMinutesFlag)
	}

	var pubKey ed25519.PublicKey
	var privKey ed25519.PrivateKey

	if *privkeyFlag != "" {
		// Load hex-encoded private key from file; derive the public key from it.
		raw, readErr := os.ReadFile(*privkeyFlag)
		if readErr != nil {
			return fmt.Errorf("privkey: cannot read file %q: %w", *privkeyFlag, readErr)
		}
		privKeyBytes, hexErr := hex.DecodeString(strings.TrimSpace(string(raw)))
		if hexErr != nil {
			return fmt.Errorf("privkey: invalid hex in file %q: %w", *privkeyFlag, hexErr)
		}
		if len(privKeyBytes) != ed25519.PrivateKeySize {
			return fmt.Errorf("privkey: wrong key length in file %q: got %d bytes, want %d (ed25519.PrivateKeySize)",
				*privkeyFlag, len(privKeyBytes), ed25519.PrivateKeySize)
		}
		privKey = ed25519.PrivateKey(privKeyBytes)
		pubKey = privKey.Public().(ed25519.PublicKey)
	} else {
		// Generate a fresh ed25519 key pair — nothing is persisted (CI mode).
		var genErr error
		pubKey, privKey, genErr = ed25519.GenerateKey(nil)
		if genErr != nil {
			return fmt.Errorf("generate key pair: %w", genErr)
		}
	}

	// Marshal claims to canonical JSON.
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("marshal claims: %w", err)
	}

	// Sign the raw claims bytes.
	sig := ed25519.Sign(privKey, claimsJSON)

	// Encode per license.go activate(): StdEncoding base64.
	claimsB64 := base64.StdEncoding.EncodeToString(claimsJSON)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	licenseKey := claimsB64 + "." + sigB64
	pubKeyHex := hex.EncodeToString(pubKey)

	// Diagnostics to stderr (claims JSON is not secret — the key is ephemeral in CI).
	fmt.Fprintf(os.Stderr, "licensegen: tier=%s claims=%s\n", tier, claimsJSON)

	// Emit exactly two lines to stdout for GITHUB_ENV consumption.
	fmt.Printf("PULSE_LICENSE_KEY=%s\n", licenseKey)
	fmt.Printf("PULSE_LICENSE_PUBKEY=%s\n", pubKeyHex)

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "licensegen:", err)
		os.Exit(1)
	}
}
