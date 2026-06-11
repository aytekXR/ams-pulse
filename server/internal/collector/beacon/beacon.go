// Package beacon is the public HTTPS ingest endpoint for the player QoE SDK
// (POST /ingest/beacon). The only internet-facing surface of Pulse; treat as
// hostile input.
//
// Contract: contracts/events/beacon-event.schema.json
// Requirements (PRD F3): token auth, rate limiting, payload size caps,
// schema validation, configurable sampling for very large audiences.
package beacon

// TODO(BE-02, Phase 2): Handler implementing collector.Source semantics for
// pushed events; session stitching by session_id.
