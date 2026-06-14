// Package reports generates usage and billing reports (F6): viewer-minutes,
// peak concurrency, egress GB and recording storage per app/stream/tenant,
// with scheduled CSV and PDF exports (Wave 2) and white-label header support.
//
// Tenant mapping: stream-name glob pattern or beacon metadata tag.
// Egress methodology: bitrate × watch-time model — the method used is disclosed
// on every report row (PRD F6 technical notes). Statements reconcile with raw
// viewer_sessions within ±1% (verified by tests and pulse diag --reconcile).
//
// Architecture:
//   - Accountant:   usage computation from ClickHouse rollups
//   - TenantMatcher: stream→tenant resolution (glob + meta-tag)
//   - Generator:     statement generation (CSV + PDF)
//   - Scheduler:     cron-based schedule runner
//   - S3Uploader:    SigV4 PUT to S3-compatible stores
//
// Tier gating (per WO-204 §7, PRD §7.11-7.12):
//   - Reports + Schedules + S3 Export = Business tier
//   - White-label header = Business tier
//   - Enterprise white-label PDF polish = Phase 3
package reports

// Generator produces usage statements and runs export schedules.
// This is the facade used by the API and serve layers.
type Generator struct {
	Accountant *Accountant
	Scheduler  *Scheduler
}
