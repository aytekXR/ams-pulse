// Package api is the HTTP layer: REST routes per contracts/openapi/pulse-api.yaml,
// WebSocket push for the live dashboard (F1), the Prometheus /metrics endpoint
// (F8, gauges/counters only, low cardinality), /healthz, the beacon ingest
// route (delegating to collector/beacon), and static serving of the built web UI.
//
// Auth: bearer tokens (meta store) for the API; separate ingest tokens for
// beacons. No business logic here — handlers call internal/query,
// internal/alert, internal/reports.
package api

// Server hosts all HTTP surfaces of a Pulse node.
type Server struct {
	// TODO(BE-02): router, middleware (auth, rate limit, logging), websocket hub.
}
