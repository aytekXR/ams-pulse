// Package restpoller polls AMS REST API v2 endpoints (broadcasts,
// broadcast-statistics, cluster nodes) and emits normalized events.
// This is the universal-fallback source: it must work against every supported
// AMS version with no server-side configuration (PRD Appendix A.5).
//
// F1 acceptance dependency: poll interval default must surface a new stream on
// the dashboard within 10 seconds of publish.
package restpoller

// TODO(BE-01): Poller implementing collector.Source over pkg/amsclient.
