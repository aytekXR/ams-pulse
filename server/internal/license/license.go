// Package license validates the Pulse license key against the vendor licensing
// service, with a signed offline-license file path for air-gapped Enterprise
// installs (PRD §7.10). It exposes tier entitlements (node count, retention,
// features) that query, alerting and reports consult.
//
// Design constraints: fail-open for reads of already-collected data, fail-closed
// for tier-gated features; the Free tier must work with no key at all
// (1 node, 7-day retention, email alerts).
package license

// Manager resolves and caches the active license and its entitlements.
type Manager struct {
	// TODO(BE-02)
}
