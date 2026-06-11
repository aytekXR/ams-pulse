// Package domain holds Pulse's core types, shared by collector, store, query,
// alerting and reports. Types here mirror the contracts in /contracts/events —
// when a contract changes, this package changes first and the compiler finds
// the rest.
//
// Rule: no AMS-specific field names in this package. AMS shapes are translated
// at the collector boundary (pkg/amsclient + internal/collector) into these
// normalized types. That boundary is the Phase 3 multi-server portability play.
package domain

// ServerEvent is the normalized event from any server-side source.
// Contract: contracts/events/ams-server-event.schema.json
type ServerEvent struct {
	// TODO(BE-01)
}

// BeaconEvent is a viewer-side QoE event batch.
// Contract: contracts/events/beacon-event.schema.json
type BeaconEvent struct {
	// TODO(BE-01)
}

// ViewerSession is a stitched per-viewer playback session.
type ViewerSession struct {
	// TODO(BE-01)
}

// AlertRule is a user-defined alerting rule (metric, condition, window, scope).
type AlertRule struct {
	// TODO(BE-02)
}

// Notification is an alert delivery payload.
// Contract: contracts/events/alert-notification.schema.json
type Notification struct {
	// TODO(BE-02)
}
