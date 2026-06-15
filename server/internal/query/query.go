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
	live               domain.LiveProvider
	conn               clickhouse.Conn
	lic                *license.Manager
	probeResultQuerier ProbeResultQuerier // optional; wired via SetProbeResultQuerier
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

	// rollup_audience_1h / rollup_audience_1d (AggregatingMergeTree) column names:
	//   bucket (DateTime for 1h, Date for 1d), views, uniq_viewers,
	//   watch_time_s AggregateFunction(sum,UInt64),
	//   peak_concurrency AggregateFunction(max,UInt32).
	// Use *Merge() functions to finalize the aggregate states.
	// toInt64(toUnixTimestamp(bucket)) gives epoch-seconds; multiply by 1000 for ms.
	q := fmt.Sprintf(`
		SELECT
			toInt64(toUnixTimestamp(bucket)) * 1000 AS ts,
			countMerge(views)                        AS views,
			uniqMerge(uniq_viewers)                  AS uniques,
			toInt64(sumMerge(watch_time_s))          AS watch_time_s,
			toInt64(maxMerge(peak_concurrency))      AS peak_concurrency
		FROM %s
		WHERE %s
		GROUP BY bucket
		ORDER BY bucket`, table, where)

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
			Version:  n.Version, // VD-40: propagate from LiveNodeStats
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

// ─── F10 Probe results ────────────────────────────────────────────────────────

// ProbeResultQuerier is the interface the query service uses to read probe results.
// Implemented by *store/clickhouse.Store.
type ProbeResultQuerier interface {
	QueryProbeResults(ctx context.Context, probeID string, from, to time.Time, limit int) ([]domain.ProbeResult, error)
}

// SetProbeResultQuerier wires the probe result reader (from the ClickHouse store)
// into the query service. Call after New, before use.
func (s *Service) SetProbeResultQuerier(q ProbeResultQuerier) {
	s.probeResultQuerier = q
}

// QueryProbeResults fetches probe results for a given probeID via the ClickHouse
// store. Returns nil, nil when no querier is wired (ClickHouse not available).
func (s *Service) QueryProbeResults(ctx context.Context, probeID string, from, to time.Time, limit int) ([]domain.ProbeResult, error) {
	if s.probeResultQuerier == nil {
		return nil, nil
	}
	return s.probeResultQuerier.QueryProbeResults(ctx, probeID, from, to, limit)
}

// ─── VD-06: Geo breakdown ────────────────────────────────────────────────────

// GeoParams holds filters for the geo breakdown query.
type GeoParams struct {
	From    time.Time
	To      time.Time
	App     string
	Stream  string
	Tenant  string
	Region  bool // if true, GROUP BY geo_region as well
}

// GeoRow is one row in the geo breakdown result.
type GeoRow struct {
	Country    string  `json:"country"`
	Region     *string `json:"region,omitempty"`
	Views      int64   `json:"views"`
	Uniques    int64   `json:"uniques"`
	WatchTimeS int64   `json:"watch_time_s"`
}

// GeoBreakdown returns viewer counts grouped by geo_country (and optionally
// geo_region) from viewer_sessions. Falls back to empty when ClickHouse is not
// configured.
func (s *Service) GeoBreakdown(ctx context.Context, p GeoParams) ([]GeoRow, error) {
	if s.conn == nil {
		return []GeoRow{}, nil
	}

	groupBy := "geo_country"
	selectRegion := ""
	if p.Region {
		groupBy = "geo_country, geo_region"
		selectRegion = ", geo_region"
	}

	where, args := buildSessionTimeWhere(p.From, p.To)
	if p.App != "" {
		where += " AND app = ?"
		args = append(args, p.App)
	}
	if p.Stream != "" {
		where += " AND stream_id = ?"
		args = append(args, p.Stream)
	}
	if p.Tenant != "" {
		where += " AND tenant = ?"
		args = append(args, p.Tenant)
	}

	q := fmt.Sprintf(`
		SELECT
			geo_country%s,
			toInt64(count())            AS views,
			toInt64(uniq(session_id))   AS uniques,
			toInt64(sum(watch_time_s))  AS watch_time_s
		FROM viewer_sessions FINAL
		WHERE %s
		GROUP BY %s
		ORDER BY views DESC`,
		selectRegion, where, groupBy)

	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("geo breakdown query: %w", err)
	}
	defer rows.Close()

	var result []GeoRow
	for rows.Next() {
		var row GeoRow
		if p.Region {
			var region string
			if err := rows.Scan(&row.Country, &region, &row.Views, &row.Uniques, &row.WatchTimeS); err != nil {
				return nil, err
			}
			row.Region = &region
		} else {
			if err := rows.Scan(&row.Country, &row.Views, &row.Uniques, &row.WatchTimeS); err != nil {
				return nil, err
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []GeoRow{}
	}
	return result, nil
}

// ─── VD-06: Device breakdown ─────────────────────────────────────────────────

// DeviceParams holds filters for the device breakdown query.
type DeviceParams struct {
	From   time.Time
	To     time.Time
	App    string
	Stream string
	Tenant string
}

// DeviceRow is one row in the device breakdown result.
type DeviceRow struct {
	Device     string `json:"device"`
	OS         string `json:"os"`
	Browser    string `json:"browser"`
	Protocol   string `json:"protocol"`
	Views      int64  `json:"views"`
	Uniques    int64  `json:"uniques"`
	WatchTimeS int64  `json:"watch_time_s"`
}

// DeviceBreakdown returns viewer counts grouped by client_device, client_os,
// client_browser, and protocol from viewer_sessions. Falls back to empty when
// ClickHouse is not configured.
func (s *Service) DeviceBreakdown(ctx context.Context, p DeviceParams) ([]DeviceRow, error) {
	if s.conn == nil {
		return []DeviceRow{}, nil
	}

	where, args := buildSessionTimeWhere(p.From, p.To)
	if p.App != "" {
		where += " AND app = ?"
		args = append(args, p.App)
	}
	if p.Stream != "" {
		where += " AND stream_id = ?"
		args = append(args, p.Stream)
	}
	if p.Tenant != "" {
		where += " AND tenant = ?"
		args = append(args, p.Tenant)
	}

	q := fmt.Sprintf(`
		SELECT
			client_device,
			client_os,
			client_browser,
			protocol,
			toInt64(count())            AS views,
			toInt64(uniq(session_id))   AS uniques,
			toInt64(sum(watch_time_s))  AS watch_time_s
		FROM viewer_sessions FINAL
		WHERE %s
		GROUP BY client_device, client_os, client_browser, protocol
		ORDER BY views DESC`, where)

	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("device breakdown query: %w", err)
	}
	defer rows.Close()

	var result []DeviceRow
	for rows.Next() {
		var row DeviceRow
		if err := rows.Scan(&row.Device, &row.OS, &row.Browser, &row.Protocol,
			&row.Views, &row.Uniques, &row.WatchTimeS); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []DeviceRow{}
	}
	return result, nil
}

// ─── VD-11: QoE summary from rollup_qoe_1h ───────────────────────────────────

// QoeParams holds filters for the QoE summary query.
type QoeParams struct {
	From     time.Time
	To       time.Time
	App      string
	Stream   string
	Tenant   string
	Country  string
	Device   string
	Interval string // "hour" | "day"
}

// QoeTotals holds the aggregated QoE metrics.
type QoeTotals struct {
	StartupP50Ms  float64 `json:"startup_p50_ms"`
	StartupP95Ms  float64 `json:"startup_p95_ms"`
	RebufferRatio float64 `json:"rebuffer_ratio"`
	ErrorRate     float64 `json:"error_rate"`
}

// BitrateBucket is one point in the bitrate timeline.
type BitrateBucket struct {
	TS              int64   `json:"ts"`
	BitrateKbpsP50  float64 `json:"bitrate_kbps_p50"`
	BitrateKbpsP95  float64 `json:"bitrate_kbps_p95,omitempty"`
}

// QoeSummaryResult is the response for GET /qoe/summary.
type QoeSummaryResult struct {
	Totals          QoeTotals       `json:"totals"`
	BitrateTimeline []BitrateBucket `json:"bitrate_timeline"`
}

// QoeSummary queries rollup_qoe_1h (or rollup_qoe_1d for daily interval) and
// returns startup latency percentiles, rebuffer ratio, error rate and bitrate
// timeline. Falls back to empty result when ClickHouse is not configured.
func (s *Service) QoeSummary(ctx context.Context, p QoeParams) (*QoeSummaryResult, error) {
	empty := &QoeSummaryResult{
		Totals:          QoeTotals{},
		BitrateTimeline: []BitrateBucket{},
	}
	if s.conn == nil {
		return empty, nil
	}

	table := "rollup_qoe_1h"
	if p.Interval == "day" {
		table = "rollup_qoe_1d"
	}

	where, args := buildTimeWhere(p.From, p.To)
	if p.App != "" {
		where += " AND app = ?"
		args = append(args, p.App)
	}
	if p.Stream != "" {
		where += " AND stream_id = ?"
		args = append(args, p.Stream)
	}
	if p.Tenant != "" {
		where += " AND tenant = ?"
		args = append(args, p.Tenant)
	}
	if p.Country != "" {
		where += " AND geo_country = ?"
		args = append(args, p.Country)
	}
	if p.Device != "" {
		where += " AND client_device = ?"
		args = append(args, p.Device)
	}

	// ── Totals row ──────────────────────────────────────────────────────────
	// quantilesStateMerge returns an array; index 0 = p50, index 1 = p95.
	// rebuffer_ratio = sum(rebuffer_total_ms) / sum(watch_time_ms), capped at 1.
	// error_rate = sum(error_count) / sum(session_count).
	totalsQ := fmt.Sprintf(`
		SELECT
			quantilesMerge(0.5, 0.95)(startup_ms_state)[1]  AS startup_p50,
			quantilesMerge(0.5, 0.95)(startup_ms_state)[2]  AS startup_p95,
			sumMerge(rebuffer_total_ms)                      AS reb_ms,
			sumMerge(watch_time_ms)                          AS watch_ms,
			sumMerge(error_count)                            AS errs,
			countMerge(session_count)                        AS sessions
		FROM %s
		WHERE %s`, table, where)

	trow := s.conn.QueryRow(ctx, totalsQ, args...)
	var startupP50, startupP95 float64
	var rebMs, watchMs, errs, sessions uint64
	if err := trow.Scan(&startupP50, &startupP95, &rebMs, &watchMs, &errs, &sessions); err != nil {
		// No data — return empty.
		return empty, nil
	}

	var rebRatio, errRate float64
	if watchMs > 0 {
		r := float64(rebMs) / float64(watchMs)
		if r > 1.0 {
			r = 1.0
		}
		rebRatio = r
	}
	if sessions > 0 {
		errRate = float64(errs) / float64(sessions)
	}

	// ── Bitrate timeline ─────────────────────────────────────────────────────
	timelineQ := fmt.Sprintf(`
		SELECT
			toInt64(toUnixTimestamp(bucket)) * 1000       AS ts,
			quantilesMerge(0.5, 0.95)(bitrate_kbps_state)[1] AS bitrate_p50,
			quantilesMerge(0.5, 0.95)(bitrate_kbps_state)[2] AS bitrate_p95
		FROM %s
		WHERE %s
		GROUP BY bucket
		ORDER BY bucket`, table, where)

	trows, err := s.conn.Query(ctx, timelineQ, args...)
	if err != nil {
		// Timeline failure is non-fatal; return totals without timeline.
		return &QoeSummaryResult{
			Totals:          QoeTotals{StartupP50Ms: startupP50, StartupP95Ms: startupP95, RebufferRatio: rebRatio, ErrorRate: errRate},
			BitrateTimeline: []BitrateBucket{},
		}, nil
	}
	defer trows.Close()

	var timeline []BitrateBucket
	for trows.Next() {
		var b BitrateBucket
		if err := trows.Scan(&b.TS, &b.BitrateKbpsP50, &b.BitrateKbpsP95); err != nil {
			continue
		}
		timeline = append(timeline, b)
	}
	if timeline == nil {
		timeline = []BitrateBucket{}
	}

	return &QoeSummaryResult{
		Totals: QoeTotals{
			StartupP50Ms:  startupP50,
			StartupP95Ms:  startupP95,
			RebufferRatio: rebRatio,
			ErrorRate:     errRate,
		},
		BitrateTimeline: timeline,
	}, nil
}

// ─── VD-21: Ingest timeseries ────────────────────────────────────────────────

// IngestBucket is one timeseries point for GET /qoe/ingest.
// Maps to the IngestBucket schema in contracts/openapi/pulse-api.yaml.
type IngestBucket struct {
	TS               int64   `json:"ts"`                          // Unix epoch ms
	BitrateKbps      float64 `json:"bitrate_kbps"`
	FPS              float64 `json:"fps"`
	KeyframeIntervalS float64 `json:"keyframe_interval_s,omitempty"`
	PacketLossPct    float64 `json:"packet_loss_pct,omitempty"`
	JitterMS         float64 `json:"jitter_ms,omitempty"`
}

// DropEvent is one ingest drop event for GET /qoe/ingest.
// Maps to the DropEvent schema in contracts/openapi/pulse-api.yaml.
type DropEvent struct {
	TS     int64  `json:"ts"`     // Unix epoch ms
	Reason string `json:"reason"` // bitrate_drop, fps_drop, packet_loss_spike, jitter_spike, disconnect
}

// IngestTimeseriesParams holds filters for the ingest timeseries query.
type IngestTimeseriesParams struct {
	StreamID string
	App      string
	NodeID   string
	From     time.Time
	To       time.Time
	// BucketSeconds controls the time bucket width (default 60s).
	BucketSeconds int
}

// IngestTimeseriesResult is the per-stream result returned by IngestTimeseries.
type IngestTimeseriesResult struct {
	Timeseries []IngestBucket `json:"timeseries"`
	DropEvents []DropEvent    `json:"drop_events"`
}

// IngestTimeseries queries server_events for ingest_stats rows and returns
// per-minute bucketed timeseries + detected drop events.
// Falls back to an empty result when ClickHouse is not configured.
func (s *Service) IngestTimeseries(ctx context.Context, p IngestTimeseriesParams) (*IngestTimeseriesResult, error) {
	empty := &IngestTimeseriesResult{
		Timeseries: []IngestBucket{},
		DropEvents: []DropEvent{},
	}
	if s.conn == nil {
		return empty, nil
	}

	bucketSec := p.BucketSeconds
	if bucketSec <= 0 {
		bucketSec = 60
	}

	// Build WHERE clause for server_events.
	// Filter to ingest_stats events (fps > 0 OR bitrate_kbps > 0).
	where := "event_type = 'ingest_stats'"
	var args []any
	if p.StreamID != "" {
		where += " AND stream_id = ?"
		args = append(args, p.StreamID)
	}
	if p.App != "" {
		where += " AND app = ?"
		args = append(args, p.App)
	}
	if p.NodeID != "" {
		where += " AND node_id = ?"
		args = append(args, p.NodeID)
	}
	if !p.From.IsZero() {
		where += " AND ts >= ?"
		args = append(args, p.From)
	}
	if !p.To.IsZero() {
		where += " AND ts <= ?"
		args = append(args, p.To)
	}

	q := fmt.Sprintf(`
		SELECT
			toInt64(toUnixTimestamp(toStartOfInterval(ts, toIntervalSecond(%d))) * 1000) AS bucket_ms,
			avg(bitrate_kbps)         AS avg_bitrate,
			avg(fps)                  AS avg_fps,
			avg(keyframe_interval_s)  AS avg_kf,
			avg(packet_loss_pct)      AS avg_loss,
			avg(jitter_ms)            AS avg_jitter
		FROM server_events
		WHERE %s
		GROUP BY bucket_ms
		ORDER BY bucket_ms`, bucketSec, where)

	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		// Non-fatal: return empty on ClickHouse error.
		return empty, nil
	}
	defer rows.Close()

	var timeseries []IngestBucket
	for rows.Next() {
		var b IngestBucket
		if err := rows.Scan(&b.TS, &b.BitrateKbps, &b.FPS, &b.KeyframeIntervalS,
			&b.PacketLossPct, &b.JitterMS); err != nil {
			continue
		}
		timeseries = append(timeseries, b)
	}
	if timeseries == nil {
		timeseries = []IngestBucket{}
	}

	// Detect drop events from the timeseries buckets.
	// Heuristics:
	//   bitrate_drop   — bitrate falls to < 20% of the preceding bucket
	//   fps_drop       — fps falls to < 20% of the preceding bucket
	//   packet_loss_spike — packet_loss_pct > 5%
	//   jitter_spike   — jitter_ms > 50ms
	var dropEvents []DropEvent
	for i, b := range timeseries {
		if i > 0 {
			prev := timeseries[i-1]
			if prev.BitrateKbps > 0 && b.BitrateKbps < prev.BitrateKbps*0.20 {
				dropEvents = append(dropEvents, DropEvent{TS: b.TS, Reason: "bitrate_drop"})
			} else if prev.FPS > 0 && b.FPS < prev.FPS*0.20 {
				dropEvents = append(dropEvents, DropEvent{TS: b.TS, Reason: "fps_drop"})
			}
		}
		if b.PacketLossPct > 5.0 {
			dropEvents = append(dropEvents, DropEvent{TS: b.TS, Reason: "packet_loss_spike"})
		}
		if b.JitterMS > 50.0 {
			dropEvents = append(dropEvents, DropEvent{TS: b.TS, Reason: "jitter_spike"})
		}
	}
	if dropEvents == nil {
		dropEvents = []DropEvent{}
	}

	return &IngestTimeseriesResult{
		Timeseries: timeseries,
		DropEvents: dropEvents,
	}, nil
}

// ─── SQL helpers ─────────────────────────────────────────────────────────────

// buildSessionTimeWhere generates a WHERE clause for viewer_sessions based on
// started_at. Uses epoch-second DateTime64 column.
func buildSessionTimeWhere(from, to time.Time) (string, []any) {
	if from.IsZero() && to.IsZero() {
		return "1=1", nil
	}
	if from.IsZero() {
		return "started_at <= ?", []any{to}
	}
	if to.IsZero() {
		return "started_at >= ?", []any{from}
	}
	return "started_at >= ? AND started_at <= ?", []any{from, to}
}

func buildTimeWhere(from, to time.Time) (string, []any) {
	if from.IsZero() && to.IsZero() {
		return "1=1", nil
	}
	if from.IsZero() {
		return "bucket <= ?", []any{to}
	}
	if to.IsZero() {
		return "bucket >= ?", []any{from}
	}
	return "bucket >= ? AND bucket <= ?", []any{from, to}
}
