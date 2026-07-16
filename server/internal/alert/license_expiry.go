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

// perpetualLicenseDays is the sentinel "days until expiry" reported for a perpetual /
// no-key licence. Deliberately bounded and float32-safe (~100 years): the OpenAPI alert
// value field is format:float, and math.MaxFloat64 (~1.8e308) overflowed float32 clients
// to +Inf and rendered as "1.8e+308" in the history UI (D-129 review). ok=false is what
// actually prevents firing — this value is purely the informational number persisted /
// delivered on the resolve, so a large-but-readable "effectively never" is all it needs.
const perpetualLicenseDays = 36500

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
// "license". A perpetual licence (ok=false) yields a single ok=false result so a
// previously-firing alert resolves; it can never itself fire.
func (e *Evaluator) evalLicenseExpiry(rule meta.AlertRuleRow, _ domain.AlertScope, checker LicenseExpiryChecker) []evalResult {
	days, ok := checker.DaysUntilExpiry()
	if !ok {
		// D-129: a perpetual / no-key licence has "nothing to warn about", but we must
		// still emit a result so processEvaluation can RESOLVE a previously-firing
		// near-expiry alert. The state machine has no stale-state sweep — an absent
		// groupKey stays firing forever (returning nil here left the alert stuck).
		// ok=false is terminal: perpetual can only resolve, never fire, whatever the
		// operator. The value is a bounded, float32-safe sentinel (see the const).
		return []evalResult{{groupKey: "license", value: perpetualLicenseDays, ok: false}}
	}
	return []evalResult{{
		groupKey: "license",
		value:    days,
		ok:       compare(days, rule.Operator, rule.Threshold),
	}}
}
