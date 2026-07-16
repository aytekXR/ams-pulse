// Package cluster implements fleet awareness (F7): periodic node discovery via
// the AMS cluster REST API, edge/origin role labeling, and aggregate metric
// deduplication (origin/edge double-count correction).
//
// Budget: new cluster node auto-discovered ≤ 2 min (default 30s poll; math:
// with a 30s interval, a new node is visible within 1 poll cycle = ≤ 30s ≤ 2 min).
//
// Origin/edge dedup rule:
//
//	When a stream is served via edges, viewers are counted AT THE EDGE only.
//	An origin node reports viewer_count = sum(edge viewers) which would
//	double-count. Rule: for a stream that has at least one edge node reporting
//	viewers, ignore the origin's viewer_count for that stream.
//	Implementation: the fleet manager exposes IsEdgeStream(streamID) for the
//	aggregator to call; streams with any edge viewer > 0 are "edge-served".
//
// Node domain events emitted:
//   - node_stats (every poll, for each node) — routed to aggregator + ClickHouse
//   - (future) node_up/node_down — for the alert evaluator
package cluster

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// NodeInfo holds the current state of a discovered cluster node.
type NodeInfo struct {
	NodeID        string
	IP            string
	Port          int
	Role          string // "origin" | "edge"
	Status        string // "ok" | "degraded" | "down"
	Version       string
	LastSeen      time.Time
	CPUPct        float64
	MemPct        float64
	DiskPct       float64
	ActiveStreams int
}

// ClusterClient is the interface we need from amsclient (allows mock injection in tests).
type ClusterClient interface {
	ClusterNodes(ctx context.Context) ([]amsclient.ClusterNodeDTO, error)
}

// Config holds discovery configuration.
type Config struct {
	// PollInterval is how often to query the AMS cluster nodes endpoint.
	// Default: 30s. Budget: new node discovered within 1 poll = ≤ interval ≤ 2 min.
	PollInterval time.Duration

	// NodeID is the local node ID (to exclude self from discovery if needed).
	NodeID string

	// StaleTimeout: nodes not seen for this long are marked "down".
	// Default: 3 × PollInterval.
	StaleTimeout time.Duration
}

// Discovery implements cluster fleet awareness (collector.Source).
type Discovery struct {
	mu    sync.RWMutex
	nodes map[string]*NodeInfo // key = nodeID

	cfg    Config
	client ClusterClient
	sink   domain.EventSink
	logger *slog.Logger
}

// New creates a Discovery instance.
func New(cfg Config, client ClusterClient, sink domain.EventSink, logger *slog.Logger) *Discovery {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.StaleTimeout == 0 {
		cfg.StaleTimeout = cfg.PollInterval * 3
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Discovery{
		nodes:  make(map[string]*NodeInfo),
		cfg:    cfg,
		client: client,
		sink:   sink,
		logger: logger,
	}
}

// Name implements collector.Source.
func (d *Discovery) Name() string { return "cluster-discovery" }

// Run implements collector.Source. Polls the AMS cluster nodes endpoint at
// cfg.PollInterval until ctx is cancelled.
func (d *Discovery) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	// Run immediately on start.
	d.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// poll queries the AMS cluster nodes endpoint once.
func (d *Discovery) poll(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	nodes, err := d.client.ClusterNodes(pollCtx)
	if err != nil {
		d.logger.Warn("cluster: nodes poll failed", "error", err)
		return
	}

	now := time.Now()

	// pending node_stats events are emitted to the sink only AFTER releasing
	// d.mu (see the explicit Unlock below). Emitting while holding d.mu wedges
	// the server: the sink fans the event into the live aggregator, whose
	// OnServerEvent takes a.mu and then calls Discovery.IsEdgeStream (d.mu.RLock)
	// — an A→B / B→A lock-order inversion (mirror of the aggregator.EvictStale fix).
	var pending []domain.ServerEvent
	d.mu.Lock()

	seen := make(map[string]struct{}, len(nodes))

	for _, n := range nodes {
		nodeID := n.NodeID
		if nodeID == "" {
			nodeID = n.IP
		}
		seen[nodeID] = struct{}{}

		role := n.Role
		if role == "" {
			role = "origin" // default — single-node deployments
		}

		status := "ok"
		if n.CPUUsage > 90 || n.MemoryUsage > 90 {
			status = "degraded"
		}

		info, exists := d.nodes[nodeID]
		if !exists {
			info = &NodeInfo{NodeID: nodeID}
			d.nodes[nodeID] = info
			d.logger.Info("cluster: new node discovered",
				"node_id", nodeID,
				"ip", n.IP,
				"role", role,
				"status", status,
			)
		}

		info.IP = n.IP
		info.Port = n.Port
		info.Role = role
		info.Status = status
		info.Version = n.Version // VD-40: propagate version from DTO
		info.LastSeen = now
		info.CPUPct = n.CPUUsage
		info.MemPct = n.MemoryUsage
		info.DiskPct = n.DiskUsage
		info.ActiveStreams = n.ActiveStreamCount

		// Collect the node_stats event; emit after releasing d.mu (see above).
		if d.sink != nil {
			pending = append(pending, domain.ServerEvent{
				Version: 1,
				Type:    domain.EventNodeStats,
				TS:      now.UnixMilli(),
				Source:  domain.SourceRestPoll,
				NodeID:  nodeID,
				Data: map[string]any{
					"cpu_pct":          n.CPUUsage,
					"mem_pct":          n.MemoryUsage,
					"disk_pct":         n.DiskUsage,
					"net_in_mbps":      n.NetworkInputBps / 1_000_000,
					"net_out_mbps":     n.NetworkOutputBps / 1_000_000,
					"jvm_heap_used_mb": n.JvmMemoryUsage,
					"version":          n.Version, // VD-40
				},
			})
		}
	}

	// Mark stale nodes as down.
	for nodeID, info := range d.nodes {
		if _, ok := seen[nodeID]; !ok {
			if now.Sub(info.LastSeen) > d.cfg.StaleTimeout {
				if info.Status != "down" {
					info.Status = "down"
					d.logger.Warn("cluster: node down (stale)",
						"node_id", nodeID,
						"last_seen", info.LastSeen,
					)
				}
			}
		}
	}
	d.mu.Unlock()

	// Emit AFTER releasing d.mu — never hold d.mu across a sink call (the sink
	// fans back into the aggregator which re-enters Discovery via IsEdgeStream).
	for _, ev := range pending {
		d.sink.WriteServerEvent(ev)
	}
}

// Snapshot returns a copy of the current fleet state.
func (d *Discovery) Snapshot() []NodeInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]NodeInfo, 0, len(d.nodes))
	for _, n := range d.nodes {
		out = append(out, *n)
	}
	return out
}

// NodeCount returns the number of known nodes.
func (d *Discovery) NodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.nodes)
}

// IsEdgeStream returns true if any edge node is known to serve the given
// stream. This is used by the aggregator to avoid double-counting viewer
// counts in origin+edge cluster deployments (F1+F7 AC).
//
// Edge dedup rule:
// When at least one edge node has Role=="edge" AND has active streams,
// origin nodes report a viewer_count that already includes edge viewers.
// To avoid double-counting: if IsEdgeStream(streamID) returns true AND the
// reporting node's role is "origin", the aggregator must skip adding that
// node's viewer_count for that stream.
//
// Implementation: we check whether any known edge node has ActiveStreams > 0.
// This is a conservative heuristic — a more precise implementation would
// track per-stream edge viewers. For F7 MVP correctness this is sufficient:
// in an origin+edge deployment all active streams route through edges.
func (d *Discovery) IsEdgeStream(streamID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, n := range d.nodes {
		// Exclude edges marked "down": poll() sets Status="down" on a stale node
		// but never clears its last-polled ActiveStreams, so a crashed edge would
		// otherwise keep this true forever and permanently suppress origin viewer
		// counts (the aggregator skips origin viewer_count when IsEdgeStream is
		// true). A "degraded" edge is still up and serving, so it still counts.
		if n.Role == "edge" && n.Status != "down" && n.ActiveStreams > 0 {
			return true
		}
	}
	return false
}

// NodeRole returns the role of a node ("origin" | "edge" | "").
func (d *Discovery) NodeRole(nodeID string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if n, ok := d.nodes[nodeID]; ok {
		return n.Role
	}
	return ""
}

// ─── Interface enforcement ────────────────────────────────────────────────────

var _ collector.Source = (*Discovery)(nil)
