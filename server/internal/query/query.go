// Package query is the read-side service behind the REST API: it translates
// API requests (live overview, audience analytics, QoE summaries, usage
// aggregates) into store reads over rollups, applying tier entitlements
// (retention windows per license tier) before answering.
//
// Contract: contracts/openapi/pulse-api.yaml — this package implements its
// data-shaping; HTTP concerns live in internal/api.
package query

import (
	"context"
	"fmt"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
)

// Service answers metric queries for the API layer.
type Service struct {
	live    domain.LiveProvider
	conn    clickhouse.Conn
	lic     *license.Manager
}

// New creates a Service.
func New(live domain.LiveProvider, conn clickhouse.Conn, lic *license.Manager) *Service {
	return &Service{live: live, conn: conn, lic: lic}
}

// ─── Live queries (from in-memory aggregates) ─────────────────────────────────

// LiveOverview returns the live dashboard overview from in-memory aggregates.
// Filter parameters may be empty to return all data.
func (s *Service) LiveOverview(ctx context.Context, app, nodeID, tenant string) (*LiveOverviewResult, error) {
	snap := s.live.CurrentSnapshot()
	if snap == nil {
		return &LiveOverviewResult{
			TS:              time.Now().UnixMilli(),
			TotalViewers:    0,
			TotalPublishers: 0,
			ProtocolMix:     ProtocolMix{},
			Apps:            []AppOverview{},
			Nodes:           []NodeHealth{},
		}, nil
	}

	// Aggregate app-level data.
	appMap := map[string]*AppOverview{}
	for _, stream := range snap.Streams {
		if app != "" && stream.App != app {
			continue
		}
		if nodeID != "" && stream.NodeID != nodeID {
			continue
		}
		ao := appMap[stream.App]
		if ao == nil {
			appMap[stream.App] = &AppOverview{App: stream.App}
			ao = appMap[stream.App]
		}
		ao.Viewers += stream.ViewerCount
		ao.Streams++
		if stream.Active {
			ao.Publishers++
		}
	}
	var apps []AppOverview
	for _, ao := range appMap {
		apps = append(apps, *ao)
	}

	// Protocol mix across all streams.
	var mix ProtocolMix
	totalViewers := 0
	totalPublishers := 0
	for _, stream := range snap.Streams {
		if app != "" && stream.App != app {
			continue
		}
		if nodeID != "" && stream.NodeID != nodeID {
			continue
		}
		totalViewers += stream.ViewerCount
		if stream.Active {
			totalPublishers++
		}
		mix.WebRTC += stream.ViewersByProto.WebRTC
		mix.HLS += stream.ViewersByProto.HLS
		mix.RTMP += stream.ViewersByProto.RTMP
		mix.DASH += stream.ViewersByProto.DASH
		mix.Other += stream.ViewersByProto.Other
	}

	// Node health.
	var nodes []NodeHealth
	for nid, n := range snap.Nodes {
		if nodeID != "" && nid != nodeID {
			continue
		}
		nh := NodeHealth{
			NodeID:  nid,
			Role:    "standalone",
			Status:  "up",
			LastSeen: n.UpdatedAt.UnixMilli(),
			CPUPCT:  n.CPUPCT,
			MemPCT:  n.MemPCT,
		}
		if n.CPUPCT > 90 || n.MemPCT > 90 {
			nh.Status = "degraded"
		}
		nodes = append(nodes, nh)
	}

	return &LiveOverviewResult{
		TS:              snap.UpdatedAt.UnixMilli(),
		TotalViewers:    totalViewers,
		TotalPublishers: totalPublishers,
		ProtocolMix:     mix,
		Apps:            apps,
		Nodes:           nodes,
	}, nil
}

// LiveStreams returns a paginated list of active streams.
func (s *Service) LiveStreams(ctx context.Context, app, nodeID, tenant string, limit int, cursor string) (*LiveStreamListResult, error) {
	snap := s.live.CurrentSnapshot()
	if snap == nil {
		return &LiveStreamListResult{Items: []LiveStreamItem{}, Meta: PaginatedMeta{}}, nil
	}

	var items []LiveStreamItem
	for sid, stream := range snap.Streams {
		if app != "" && stream.App != app {
			continue
		}
		if nodeID != "" && stream.NodeID != nodeID {
			continue
		}

		pubState := "idle"
		if stream.Active {
			pubState = "publishing"
		}

		healthScore := 100.0
		switch stream.Health {
		case domain.StreamHealthWarning:
			healthScore = 50.0
		case domain.StreamHealthCritical:
			healthScore = 20.0
		case domain.StreamHealthOffline:
			healthScore = 0.0
		}

		items = append(items, LiveStreamItem{
			StreamID:       sid,
			App:            stream.App,
			NodeID:         stream.NodeID,
			Viewers:        stream.ViewerCount,
			PublisherState: pubState,
			HealthScore:    healthScore,
			ProtocolMix: ProtocolMix{
				WebRTC: stream.ViewersByProto.WebRTC,
				HLS:    stream.ViewersByProto.HLS,
				RTMP:   stream.ViewersByProto.RTMP,
				DASH:   stream.ViewersByProto.DASH,
				Other:  stream.ViewersByProto.Other,
			},
			BitrateKbps: stream.IngestBitrate,
			StartedAt:   stream.StartedAt.UnixMilli(),
		})
	}

	// Pagination.
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	// Simple offset cursor.
	start := 0
	_ = cursor // wave 1: ignore cursor, return first page
	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	var nextCursor *string
	if end < len(items) {
		c := fmt.Sprintf("%d", end)
		nextCursor = &c
	}

	return &LiveStreamListResult{
		Items: items[start:end],
		Meta:  PaginatedMeta{NextCursor: nextCursor},
	}, nil
}

// ─── Historical queries (from ClickHouse rollups) ─────────────────────────────

// AudienceAnalytics returns audience timeseries and totals from rollup tables.
func (s *Service) AudienceAnalytics(ctx context.Context, p AudienceParams) (*AudienceResult, error) {
	// No ClickHouse connection — return empty result (test/dev environment).
	if s.conn == nil {
		return &AudienceResult{Totals: AudienceTotals{}, Timeseries: []AudienceBucket{}}, nil
	}

	// Apply retention check.
	effectiveFrom, effectiveTo := s.applyRetention(p.From, p.To)

	table := "rollup_audience_1h"
	if p.Interval == "day" {
		table = "rollup_audience_1d"
	}

	where, args := buildTimeWhere(effectiveFrom, effectiveTo)
	if p.App != "" {
		where += " AND app = ?"
		args = append(args, p.App)
	}
	if p.Stream != "" {
		where += " AND stream_id = ?"
		args = append(args, p.Stream)
	}

	q := fmt.Sprintf(`
		SELECT
			toUnixTimestamp64Milli(bucket_ts) AS ts,
			sumMerge(views_state)             AS views,
			uniqMerge(uniques_state)          AS uniques,
			sumMerge(watch_s_state)           AS watch_time_s,
			maxMerge(peak_viewers_state)      AS peak_concurrency
		FROM %s
		WHERE %s
		GROUP BY bucket_ts
		ORDER BY bucket_ts`, table, where)

	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("audience query: %w", err)
	}
	defer rows.Close()

	var buckets []AudienceBucket
	var totals AudienceTotals
	for rows.Next() {
		var b AudienceBucket
		if err := rows.Scan(&b.TS, &b.Views, &b.Uniques, &b.WatchTimeS, &b.PeakConcurrency); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
		totals.Views += b.Views
		totals.Uniques += b.Uniques
		totals.WatchTimeS += b.WatchTimeS
		if b.PeakConcurrency > totals.PeakConcurrency {
			totals.PeakConcurrency = b.PeakConcurrency
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &AudienceResult{Totals: totals, Timeseries: buckets}, nil
}

// applyRetention caps the time range to the license retention period.
func (s *Service) applyRetention(from, to time.Time) (time.Time, time.Time) {
	if s.lic == nil {
		return from, to
	}
	retDays := s.lic.CheckRetention(36500) // request max
	minFrom := time.Now().AddDate(0, 0, -retDays)
	if from.Before(minFrom) {
		from = minFrom
	}
	if to.IsZero() || to.After(time.Now()) {
		to = time.Now()
	}
	return from, to
}

// ─── Fleet (from live snapshot) ───────────────────────────────────────────────

// FleetNodes returns known cluster nodes from the live snapshot.
func (s *Service) FleetNodes(ctx context.Context, limit int, cursor string) (*FleetNodeListResult, error) {
	snap := s.live.CurrentSnapshot()
	if snap == nil {
		return &FleetNodeListResult{Items: []FleetNode{}, Meta: PaginatedMeta{}}, nil
	}

	var nodes []FleetNode
	for nid, n := range snap.Nodes {
		fn := FleetNode{
			NodeID:   nid,
			Role:     "standalone",
			Status:   "up",
			LastSeen: n.UpdatedAt.UnixMilli(),
			CPUPCT:   n.CPUPCT,
			MemPCT:   n.MemPCT,
		}
		if n.CPUPCT > 90 {
			fn.Status = "degraded"
		}
		nodes = append(nodes, fn)
	}

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	end := limit
	if end > len(nodes) {
		end = len(nodes)
	}
	return &FleetNodeListResult{Items: nodes[:end], Meta: PaginatedMeta{}}, nil
}

// ─── Result types ─────────────────────────────────────────────────────────────

// LiveOverviewResult is the response shape for GET /live/overview.
type LiveOverviewResult struct {
	TS              int64         `json:"ts"`
	TotalViewers    int           `json:"total_viewers"`
	TotalPublishers int           `json:"total_publishers"`
	ProtocolMix     ProtocolMix   `json:"protocol_mix"`
	Apps            []AppOverview `json:"apps"`
	Nodes           []NodeHealth  `json:"nodes"`
}

// ProtocolMix is viewer counts per delivery protocol.
type ProtocolMix struct {
	WebRTC int `json:"webrtc"`
	HLS    int `json:"hls"`
	RTMP   int `json:"rtmp"`
	DASH   int `json:"dash"`
	Other  int `json:"other"`
}

// AppOverview is per-app summary in LiveOverview.
type AppOverview struct {
	App        string `json:"app"`
	Viewers    int    `json:"viewers"`
	Publishers int    `json:"publishers"`
	Streams    int    `json:"streams"`
}

// NodeHealth is per-node health in LiveOverview.
type NodeHealth struct {
	NodeID  string  `json:"node_id"`
	Role    string  `json:"role"`
	Status  string  `json:"status"`
	LastSeen int64  `json:"last_seen"`
	CPUPCT  float64 `json:"cpu_pct,omitempty"`
	MemPCT  float64 `json:"mem_pct,omitempty"`
	Version string  `json:"version,omitempty"`
}

// LiveStreamItem is one stream in GET /live/streams.
type LiveStreamItem struct {
	StreamID       string      `json:"stream_id"`
	App            string      `json:"app"`
	NodeID         string      `json:"node_id,omitempty"`
	Viewers        int         `json:"viewers"`
	PublisherState string      `json:"publisher_state"`
	HealthScore    float64     `json:"health_score"`
	ProtocolMix    ProtocolMix `json:"protocol_mix,omitempty"`
	BitrateKbps    float64     `json:"bitrate_kbps,omitempty"`
	StartedAt      int64       `json:"started_at,omitempty"`
}

// LiveStreamListResult is the response for GET /live/streams.
type LiveStreamListResult struct {
	Items []LiveStreamItem `json:"items"`
	Meta  PaginatedMeta    `json:"meta"`
}

// PaginatedMeta is the pagination envelope.
type PaginatedMeta struct {
	NextCursor *string `json:"next_cursor"`
	Total      *int    `json:"total,omitempty"`
}

// AudienceParams holds the filter for audience queries.
type AudienceParams struct {
	From     time.Time
	To       time.Time
	App      string
	Stream   string
	Node     string
	Tenant   string
	Interval string // "hour" | "day"
}

// AudienceTotals is the aggregate across the query window.
type AudienceTotals struct {
	Views           int64 `json:"views"`
	Uniques         int64 `json:"uniques"`
	WatchTimeS      int64 `json:"watch_time_s"`
	PeakConcurrency int64 `json:"peak_concurrency"`
}

// AudienceBucket is one timeseries bucket.
type AudienceBucket struct {
	TS              int64 `json:"ts"`
	Views           int64 `json:"views"`
	Uniques         int64 `json:"uniques"`
	WatchTimeS      int64 `json:"watch_time_s"`
	PeakConcurrency int64 `json:"peak_concurrency"`
}

// AudienceResult is the response for GET /analytics/audience.
type AudienceResult struct {
	Totals     AudienceTotals   `json:"totals"`
	Timeseries []AudienceBucket `json:"timeseries"`
}

// FleetNode is one node entry in GET /fleet/nodes.
type FleetNode struct {
	NodeID   string  `json:"node_id"`
	Role     string  `json:"role"`
	Status   string  `json:"status"`
	LastSeen int64   `json:"last_seen"`
	Version  string  `json:"version,omitempty"`
	CPUPCT   float64 `json:"cpu_pct,omitempty"`
	MemPCT   float64 `json:"mem_pct,omitempty"`
	NetIn    float64 `json:"net_in_mbps,omitempty"`
	NetOut   float64 `json:"net_out_mbps,omitempty"`
}

// FleetNodeListResult is the response for GET /fleet/nodes.
type FleetNodeListResult struct {
	Items []FleetNode   `json:"items"`
	Meta  PaginatedMeta `json:"meta"`
}

// ─── SQL helpers ─────────────────────────────────────────────────────────────

func buildTimeWhere(from, to time.Time) (string, []any) {
	if from.IsZero() && to.IsZero() {
		return "1=1", nil
	}
	if from.IsZero() {
		return "bucket_ts <= ?", []any{to}
	}
	if to.IsZero() {
		return "bucket_ts >= ?", []any{from}
	}
	return "bucket_ts >= ? AND bucket_ts <= ?", []any{from, to}
}
