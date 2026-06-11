// Package reports generates usage and billing reports (F6): viewer-minutes,
// peak concurrency, egress GB and recording storage per app/stream/tenant,
// with scheduled CSV (Phase 2) and white-label PDF (Phase 3) exports.
//
// Tenant mapping: stream-name pattern or beacon metadata tag.
// Egress methodology: delivered-bytes events where available, else
// bitrate × watch-time model — the method used is disclosed on every report
// (PRD F6 technical notes). Statements must reconcile with raw events within 1%.
package reports

// Generator produces usage statements and runs export schedules.
type Generator struct {
	// TODO(BE-02, Phase 2)
}
