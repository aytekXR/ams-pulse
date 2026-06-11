// Package collector ingests all data sources and emits normalized domain
// events to the store. It is stateless: restart-safe, no local persistence
// beyond in-flight batching (PRD §7.10).
//
// Sources (each in its own subpackage, all implementing Source):
//
//	restpoller — polls AMS REST v2 at a configurable interval (the universal
//	             fallback; works on every AMS version — PRD Appendix A.5)
//	logtail    — tails ant-media-server-analytics.log (JSON, v2.10+)
//	kafka      — consumes the native AMS Kafka producer feed when enabled
//	webhook    — receives AMS publish/unpublish/recording webhooks
//	beacon     — public HTTPS ingest endpoint for the player QoE SDK
//
// Sources are deduplicating-by-design: REST polling and webhooks may report the
// same lifecycle event; the collector normalizes and dedupes before storage.
package collector

import "context"

// Source is one ingest pipeline producing normalized ServerEvents/BeaconEvents.
type Source interface {
	// Name identifies the source in logs and self-metrics.
	Name() string
	// Run blocks, emitting events until ctx is cancelled.
	Run(ctx context.Context) error
}

// Collector supervises all configured sources with per-source restart/backoff.
type Collector struct {
	// TODO(BE-01)
}
