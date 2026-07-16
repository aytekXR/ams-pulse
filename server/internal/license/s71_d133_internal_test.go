package license

// s71_d133_internal_test.go — white-box test for the [23] defense-in-depth layer.
// activate() now rejects an unknown tier, so the public API can no longer put one
// into a Manager; this constructs one directly to pin that CheckProbes /
// CheckBeaconIngest use POSITIVE tier membership and block it, rather than the old
// `t == TierFree` gate that granted access to any non-"free" string.

import "testing"

func TestChecks_UnknownTier_BlockedByPositiveMembership(t *testing.T) {
	unknown := &Manager{tier: Tier("enterprise_lite")}
	if err := unknown.CheckProbes(); err == nil {
		t.Error("CheckProbes must block an unknown tier (positive membership), got nil")
	}
	if err := unknown.CheckBeaconIngest(); err == nil {
		t.Error("CheckBeaconIngest must block an unknown tier (positive membership), got nil")
	}

	// The three paid tiers must still pass both gates.
	for _, paid := range []Tier{TierPro, TierBusiness, TierEnterprise} {
		mp := &Manager{tier: paid}
		if err := mp.CheckProbes(); err != nil {
			t.Errorf("CheckProbes(%q) = %v, want nil", paid, err)
		}
		if err := mp.CheckBeaconIngest(); err != nil {
			t.Errorf("CheckBeaconIngest(%q) = %v, want nil", paid, err)
		}
	}

	// Free is still blocked (unchanged behaviour).
	free := &Manager{tier: TierFree}
	if err := free.CheckProbes(); err == nil {
		t.Error("CheckProbes(free) must block, got nil")
	}
	if err := free.CheckBeaconIngest(); err == nil {
		t.Error("CheckBeaconIngest(free) must block, got nil")
	}
}
