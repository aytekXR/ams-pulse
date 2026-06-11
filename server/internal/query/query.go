// Package query is the read-side service behind the REST API: it translates
// API requests (live overview, audience analytics, QoE summaries, usage
// aggregates) into store reads over rollups, applying tier entitlements
// (retention windows per license tier) before answering.
//
// Contract: contracts/openapi/pulse-api.yaml — this package implements its
// data-shaping; HTTP concerns live in internal/api.
package query

// Service answers metric queries for the API layer.
type Service struct {
	// TODO(BE-02)
}
