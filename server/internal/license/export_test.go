// export_test.go — test-only hooks exported for use in *_test.go files of the
// license package. Not compiled in production builds (file ends in _test.go).
package license

import (
	"crypto/ed25519"
	"io"
	"time"
)

// SetNow replaces the package-level clock function used in expiry checks.
// Call t.Cleanup(func() { license.SetNow(time.Now) }) to restore after the test.
func SetNow(f func() time.Time) { now = f }

// SetGenerateKey replaces the package-level ed25519 key generator, so a test can
// force the dev-mode fallback in New to fail. Restore via
// t.Cleanup(func() { license.SetGenerateKey(ed25519.GenerateKey) }).
func SetGenerateKey(f func(io.Reader) (ed25519.PublicKey, ed25519.PrivateKey, error)) {
	generateKey = f
}

// TierDefaultMaxNodes returns the default MaxNodes entitlement for the given
// tier as defined by the tier entitlement vars in license.go. -1 means unlimited.
// Exported for tier_ladder_test.go which cannot access unexported package vars.
func TierDefaultMaxNodes(t Tier) int {
	switch t {
	case TierFree:
		return freeTierEntitlements.MaxNodes
	case TierPro:
		return proTierEntitlements.MaxNodes
	case TierBusiness:
		return businessTierEntitlements.MaxNodes
	case TierEnterprise:
		return enterpriseTierEntitlements.MaxNodes
	default:
		return 0
	}
}
