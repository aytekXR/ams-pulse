// Package license_test — regression guard for the tier node-limit ladder.
//
// Ensures MaxNodes is strictly non-decreasing across tiers (Free ≤ Pro ≤
// Business ≤ Enterprise) so an inversion can never ship again. -1 (unlimited)
// is treated as ∞ for comparison purposes.
package license_test

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/license"
)

// asOrderedNodes maps MaxNodes to a comparable integer: -1 → maxInt so that
// unlimited (Enterprise) always sorts higher than any finite limit.
func asOrderedNodes(n int) int {
	if n < 0 {
		return int(^uint(0) >> 1) // maxInt
	}
	return n
}

// TestTierNodeLadder_NonDecreasing guards that the default MaxNodes for each
// tier is strictly non-decreasing from Free → Pro → Business → Enterprise.
// Expected ladder: Free 1 / Pro 10 / Business 50 / Enterprise unlimited (-1).
func TestTierNodeLadder_NonDecreasing(t *testing.T) {
	tiers := []license.Tier{
		license.TierFree,
		license.TierPro,
		license.TierBusiness,
		license.TierEnterprise,
	}

	prev := -1
	prevName := ""
	for _, tier := range tiers {
		raw := license.TierDefaultMaxNodes(tier)
		cur := asOrderedNodes(raw)
		if prevName != "" && cur < prev {
			t.Errorf(
				"tier ladder inversion: %s MaxNodes (%d ordered=%d) < %s MaxNodes (ordered=%d) — higher tier has FEWER nodes",
				tier, raw, cur, prevName, prev,
			)
		}
		prev = cur
		prevName = string(tier)
	}
}

// TestTierNodeLadder_ExactValues pins the persona-consistent ladder values so
// any future edit to the tier defaults produces a compile-time diff:
// Free 1 / Pro 10 / Business 50 / Enterprise unlimited (-1).
func TestTierNodeLadder_ExactValues(t *testing.T) {
	cases := []struct {
		tier    license.Tier
		want    int
		comment string
	}{
		{license.TierFree, 1, "single-node Free"},
		{license.TierPro, 10, "Pro buyers run 1-10 nodes"},
		{license.TierBusiness, 50, "Business buyers run 5-50 nodes (QA fixture max_nodes=50)"},
		{license.TierEnterprise, -1, "unlimited (-1)"},
	}
	for _, c := range cases {
		got := license.TierDefaultMaxNodes(c.tier)
		if got != c.want {
			t.Errorf("TierDefaultMaxNodes(%q) = %d, want %d (%s)",
				c.tier, got, c.want, c.comment)
		}
	}
}
