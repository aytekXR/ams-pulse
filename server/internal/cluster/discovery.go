// Package cluster implements fleet awareness (F7): periodic node discovery via
// the AMS cluster REST API, edge/origin role labeling, and aggregate metric
// deduplication (origin/edge double-count correction).
//
// Acceptance: new nodes appear without manual config within 2 minutes; one
// Pulse instance watches a whole cluster.
package cluster

// Discovery refreshes the known-node list and labels roles.
type Discovery struct {
	// TODO(BE-01, Phase 2)
}
