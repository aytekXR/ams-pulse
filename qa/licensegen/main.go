// Command licensegen mints an ephemeral test Pulse license for CI.
//
// Usage:
//
//	go run . [-tier free|pro|business|enterprise] >> "$GITHUB_ENV"
//
// It generates a fresh ed25519 key pair at runtime, signs a JSON claims blob,
// and prints exactly two GITHUB_ENV-compatible lines to stdout:
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
	flag.Parse()

	tier := *tierFlag

	claims, err := tierClaims(tier)
	if err != nil {
		return err
	}

	// Generate a fresh ed25519 key pair — nothing is persisted.
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("generate key pair: %w", err)
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

	// Diagnostics to stderr (claims JSON is not secret — the key is ephemeral).
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
