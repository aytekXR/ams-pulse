// Package alert — license_expiry metric (S39, WO / D-101).
//
// Out-of-band warning that the Pulse licence key is near expiry, delivered through
// the operator's configured alert channels rather than only a dashboard banner —
// a customer who never opens the UI still gets warned before a downgrade.
//
// This mirrors the cert_expiry mechanism in wave2.go exactly: a non-ClickHouse
// scalar ("days until expiry") is injected via an interface, dispatched by the
// evaluator's metric switch, and evaluated against the rule's operator/threshold.
// A rule is { metric: "license_expiry", operator: "lt", threshold: 14 } → fire when
// the licence expires in fewer than 14 days.
package alert

import (
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// LicenseExpiryChecker reports how many days remain until the licence key expires.
// ok=false means the licence has no expiry set at all (a perpetual key, or the
// free/no-key fallback) — there is nothing to warn about, so such rules are skipped.
// Defined here (not in the license package) so the evaluator does not import license;
// the concrete adapter over *license.Manager is wired in cmd/pulse/serve.go.
type LicenseExpiryChecker interface {
	DaysUntilExpiry() (days float64, ok bool)
}

// FakeLicenseChecker returns fixed values for testing.
type FakeLicenseChecker struct {
	Days      float64
	HasExpiry bool
}

// DaysUntilExpiry returns the pre-configured value for tests.
func (f FakeLicenseChecker) DaysUntilExpiry() (float64, bool) { return f.Days, f.HasExpiry }

// evalLicenseExpiry evaluates a license_expiry rule against the injected checker.
// The licence is global (no per-stream scope), so it yields a single result keyed
// "license". A perpetual licence (ok=false) yields no result — the rule cannot fire.
func (e *Evaluator) evalLicenseExpiry(rule meta.AlertRuleRow, _ domain.AlertScope, checker LicenseExpiryChecker) []evalResult {
	days, ok := checker.DaysUntilExpiry()
	if !ok {
		return nil // perpetual / no-key licence — nothing to warn about
	}
	return []evalResult{{
		groupKey: "license",
		value:    days,
		ok:       compare(days, rule.Operator, rule.Threshold),
	}}
}
