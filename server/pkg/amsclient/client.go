// Package amsclient is a typed, read-only client for the Ant Media Server REST
// API v2 (broadcasts, broadcast-statistics, WebRTC client stats, cluster nodes,
// applications). It is the ONLY package allowed to speak AMS wire formats for
// REST; raw responses are translated to domain types at this boundary.
//
// In pkg/ (not internal/) deliberately: it is a candidate for open-sourcing
// alongside the beacon SDK as community surface area (PRD §7.12 GTM).
//
// Compatibility: AMS API/log formats vary across versions (PRD §7.13 risk).
// This package carries a version matrix and is exercised by the
// ams-version-matrix CI workflow against released AMS containers.
package amsclient

// Client talks to one AMS node's REST API v2.
type Client struct {
	// TODO(BE-01): base URL, credentials, http.Client with sane timeouts.
}

// TODO(BE-01): New(...), ListBroadcasts, BroadcastStatistics, WebRTCClientStats,
// ClusterNodes, Applications — all context-aware, all read-only.
