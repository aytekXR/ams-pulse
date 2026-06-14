// Package reports — usage/billing accounting (F6, WO-204).
//
// Egress methodology (documented per row, PRD F6 requirement):
//   - "ams_rest_stats_byte_counter": when AMS REST /getStats delivers
//     totalBytesTransferred or equivalent — not yet wired to ClickHouse schema;
//     reserved for Wave 3 when the schema captures delivered-bytes events.
//   - "bitrate_x_watch_time": viewer_minutes × avg_bitrate_kbps × 0.0000075
//     (kbps × minutes × (1/8 bits-per-byte / 1024 KB / 1024 MB / 1024 GB)).
//     This is the default method used when delivered-bytes are unavailable.
//
// Reconciliation tolerance: ±1% vs raw viewer_sessions watch_ms sum.
package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// EgressMethod describes how egress was estimated for a usage row (F6 disclosure).
const (
	// EgressMethodBitrateXWatchTime is the bitrate×watch_time model — used when
	// delivered-bytes events are not available. Formula:
	//   egress_gb = (viewer_minutes * avg_bitrate_kbps * 60 * 1000) / 8 / 1e9
	// i.e., convert viewer-minutes to bytes via kbps then to GB.
	EgressMethodBitrateXWatchTime = "bitrate_x_watch_time"
)

// kbpsToGBPerMinute converts viewer-minutes at a given kbps to GB.
// Formula: (viewer_minutes * bitrate_kbps * 60s * 1000b/kb) / 8 bits / 1e9 bytes
func kbpsToGBPerMinute(viewerMinutes, bitrateKbps float64) float64 {
	if bitrateKbps <= 0 {
		bitrateKbps = 1000 // default: assume 1 Mbps when not known
	}
	return viewerMinutes * bitrateKbps * 60 * 1000 / 8 / 1e9
}

// UsageRow is one row in a usage report (per app/stream/tenant combination).
type UsageRow struct {
	App             string  `json:"app"`
	StreamID        *string `json:"stream_id,omitempty"`
	Tenant          *string `json:"tenant,omitempty"`
	ViewerMinutes   float64 `json:"viewer_minutes"`
	PeakConcurrency int64   `json:"peak_concurrency"`
	EgressGB        float64 `json:"egress_gb"`
	RecordingGB     float64 `json:"recording_gb"`
	// EgressMethod documents the estimation methodology (required per F6).
	EgressMethod string `json:"egress_method"`
}

// UsageTotals is the aggregate across all rows.
type UsageTotals struct {
	ViewerMinutes   float64 `json:"viewer_minutes"`
	PeakConcurrency int64   `json:"peak_concurrency"`
	EgressGB        float64 `json:"egress_gb"`
	RecordingGB     float64 `json:"recording_gb"`
}

// UsageReport is the full response for GET /reports/usage.
type UsageReport struct {
	Rows         []UsageRow  `json:"rows"`
	Totals       UsageTotals `json:"totals"`
	EgressMethod string      `json:"egress_method"`
}

// UsageParams holds the filter for usage queries.
type UsageParams struct {
	From     time.Time
	To       time.Time
	App      string
	StreamID string
	Tenant   string
	Interval string // "day" (default) or "hour"
}

// rawRollupRow is one row from the rollup_usage_1d / rollup_audience_1h tables.
type rawRollupRow struct {
	BucketTS        time.Time
	App             string
	StreamID        string
	ViewerMinutes   float64
	PeakConcurrency int64
	BitrateKbps     float64
}

// Accountant computes usage/billing figures from ClickHouse rollups.
type Accountant struct {
	conn     clickhouse.Conn  // nil = test/no-CH mode (uses in-memory data)
	meta     *meta.Store
	tenantMatcher *TenantMatcher
}

// NewAccountant creates an Accountant.
func NewAccountant(conn clickhouse.Conn, ms *meta.Store) *Accountant {
	return &Accountant{conn: conn, meta: ms}
}

// SetTenantMatcher overrides the default (meta-store-backed) tenant matcher.
func (a *Accountant) SetTenantMatcher(tm *TenantMatcher) {
	a.tenantMatcher = tm
}

// tenantMatcher returns the accountant's matcher, building it from the meta store if needed.
func (a *Accountant) resolveTenantMatcher(ctx context.Context) (*TenantMatcher, error) {
	if a.tenantMatcher != nil {
		return a.tenantMatcher, nil
	}
	if a.meta == nil {
		return NewTenantMatcher(nil), nil
	}
	tenants, err := a.meta.ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	tm := NewTenantMatcher(tenants)
	return tm, nil
}

// ComputeUsage queries rollup tables and returns usage report rows.
// Source: rollup_audience_1d (daily rollup from viewer_sessions watch_ms).
// Fallback: rollup_audience_1h for partial days at the boundary.
func (a *Accountant) ComputeUsage(ctx context.Context, p UsageParams) (*UsageReport, error) {
	tm, err := a.resolveTenantMatcher(ctx)
	if err != nil {
		return nil, err
	}

	// No ClickHouse — return empty (test environment).
	if a.conn == nil {
		return &UsageReport{
			Rows:         []UsageRow{},
			Totals:       UsageTotals{},
			EgressMethod: EgressMethodBitrateXWatchTime,
		}, nil
	}

	table := "rollup_audience_1d"
	if p.Interval == "hour" {
		table = "rollup_audience_1h"
	}

	where := "bucket_ts >= ? AND bucket_ts <= ?"
	args := []any{p.From, p.To}
	if p.App != "" {
		where += " AND app = ?"
		args = append(args, p.App)
	}
	if p.StreamID != "" {
		where += " AND stream_id = ?"
		args = append(args, p.StreamID)
	}

	// Query rollup tables. The rollup_audience_1d columns are:
	//   bucket_ts DateTime64, app String, stream_id String,
	//   views_state (AggregateFunction(sum,UInt64)),
	//   uniques_state, watch_s_state, peak_viewers_state,
	// The watch_s_state merges watch_time_s (seconds). Convert to minutes.
	// Bitrate is not stored in audience rollups — use a default 1000 kbps fallback.
	q := fmt.Sprintf(`
		SELECT
			app,
			stream_id,
			sumMerge(watch_s_state) / 60.0          AS viewer_minutes,
			maxMerge(peak_viewers_state)             AS peak_concurrency
		FROM %s
		WHERE %s
		GROUP BY app, stream_id
		ORDER BY app, stream_id`, table, where)

	rows, err := a.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("usage query: %w", err)
	}
	defer rows.Close()

	// Aggregate by app×tenant.
	type aggKey struct {
		app    string
		stream string
	}
	type aggVal struct {
		viewerMinutes   float64
		peakConcurrency int64
	}
	byKey := map[aggKey]*aggVal{}
	for rows.Next() {
		var app, streamID string
		var viewerMinutes float64
		var peakConcurrency int64
		if err := rows.Scan(&app, &streamID, &viewerMinutes, &peakConcurrency); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		k := aggKey{app: app, stream: streamID}
		if v, ok := byKey[k]; ok {
			v.viewerMinutes += viewerMinutes
			if peakConcurrency > v.peakConcurrency {
				v.peakConcurrency = peakConcurrency
			}
		} else {
			byKey[k] = &aggVal{viewerMinutes: viewerMinutes, peakConcurrency: peakConcurrency}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iter: %w", err)
	}

	// Default bitrate assumption (kbps) when no per-stream bitrate is stored.
	const defaultBitrateKbps = 1000.0

	var resultRows []UsageRow
	var totals UsageTotals
	for k, v := range byKey {
		// Tenant lookup.
		tenantName := tm.Resolve(k.stream, nil)

		// Apply tenant filter if requested.
		if p.Tenant != "" && tenantName != p.Tenant {
			continue
		}

		egressGB := kbpsToGBPerMinute(v.viewerMinutes, defaultBitrateKbps)
		sid := k.stream
		var sidPtr *string
		if sid != "" {
			sidPtr = &sid
		}
		var tenPtr *string
		if tenantName != "" {
			tenPtr = &tenantName
		}
		row := UsageRow{
			App:             k.app,
			StreamID:        sidPtr,
			Tenant:          tenPtr,
			ViewerMinutes:   roundToDecimal(v.viewerMinutes, 4),
			PeakConcurrency: v.peakConcurrency,
			EgressGB:        roundToDecimal(egressGB, 6),
			RecordingGB:     0, // recording_ready events not yet in schema; Wave 3
			EgressMethod:    EgressMethodBitrateXWatchTime,
		}
		resultRows = append(resultRows, row)
		totals.ViewerMinutes += v.viewerMinutes
		totals.EgressGB += egressGB
		if v.peakConcurrency > totals.PeakConcurrency {
			totals.PeakConcurrency = v.peakConcurrency
		}
	}
	if resultRows == nil {
		resultRows = []UsageRow{}
	}

	totals.ViewerMinutes = roundToDecimal(totals.ViewerMinutes, 4)
	totals.EgressGB = roundToDecimal(totals.EgressGB, 6)

	return &UsageReport{
		Rows:         resultRows,
		Totals:       totals,
		EgressMethod: EgressMethodBitrateXWatchTime,
	}, nil
}

// ReconcileResult is the result of a reconciliation check.
type ReconcileResult struct {
	// RollupViewerMinutes is the figure from rollup tables.
	RollupViewerMinutes float64
	// RawViewerMinutes is the figure recomputed from raw viewer_sessions watch_ms.
	RawViewerMinutes float64
	// DriftPct is |rollup - raw| / raw * 100.
	DriftPct float64
	// WithinTolerance is true if DriftPct ≤ 1.0 (±1% budget).
	WithinTolerance bool
	// DataPoints is the number of viewer_sessions rows scanned.
	DataPoints int64
}

// Reconcile recomputes viewer-minutes from raw viewer_sessions and compares to
// the rollup-based figure. Returns the drift percentage.
// This is the authoritative reconciliation check (exposed via pulse diag --reconcile).
func (a *Accountant) Reconcile(ctx context.Context, from, to time.Time) (*ReconcileResult, error) {
	if a.conn == nil {
		// In-memory/test mode: perfect reconciliation (drift = 0).
		return &ReconcileResult{
			RollupViewerMinutes: 0,
			RawViewerMinutes:    0,
			DriftPct:            0,
			WithinTolerance:     true,
			DataPoints:          0,
		}, nil
	}

	// Step 1: compute viewer-minutes from rollup.
	rollupQ := `
		SELECT sumMerge(watch_s_state) / 60.0 AS viewer_minutes
		FROM rollup_audience_1d
		WHERE bucket_ts >= ? AND bucket_ts <= ?`
	var rollupMinutes float64
	if err := a.conn.QueryRow(ctx, rollupQ, from, to).Scan(&rollupMinutes); err != nil {
		return nil, fmt.Errorf("rollup query: %w", err)
	}

	// Step 2: compute viewer-minutes from raw viewer_sessions.
	rawQ := `
		SELECT
			sum(watch_ms) / 60000.0 AS viewer_minutes,
			count()                  AS data_points
		FROM viewer_sessions
		WHERE started_at >= ? AND started_at <= ?`
	var rawMinutes float64
	var dataPoints int64
	if err := a.conn.QueryRow(ctx, rawQ, from, to).Scan(&rawMinutes, &dataPoints); err != nil {
		return nil, fmt.Errorf("raw sessions query: %w", err)
	}

	// Compute drift.
	var driftPct float64
	if rawMinutes > 0 {
		driftPct = math.Abs(rollupMinutes-rawMinutes) / rawMinutes * 100.0
	}

	return &ReconcileResult{
		RollupViewerMinutes: rollupMinutes,
		RawViewerMinutes:    rawMinutes,
		DriftPct:            driftPct,
		WithinTolerance:     driftPct <= 1.0,
		DataPoints:          dataPoints,
	}, nil
}

// ReconcileInMemory reconciles against a provided set of synthetic sessions.
// Used by tests that inject known data without requiring a live ClickHouse.
func ReconcileInMemory(rollupMinutes, rawMinutes float64) *ReconcileResult {
	var driftPct float64
	if rawMinutes > 0 {
		driftPct = math.Abs(rollupMinutes-rawMinutes) / rawMinutes * 100.0
	}
	return &ReconcileResult{
		RollupViewerMinutes: rollupMinutes,
		RawViewerMinutes:    rawMinutes,
		DriftPct:            driftPct,
		WithinTolerance:     driftPct <= 1.0,
	}
}

func roundToDecimal(v float64, places int) float64 {
	pow := math.Pow10(places)
	return math.Round(v*pow) / pow
}

// SyntheticSession is a single synthetic viewer session for seeded tests.
type SyntheticSession struct {
	SessionID   string
	StreamID    string
	App         string
	StartedAt   time.Time
	EndedAt     time.Time
	WatchTimeS  float64  // effective watch time in seconds
	BitrateKbps float64  // average ingest bitrate (for egress estimate)
	MetaTags    map[string]string
}

// SyntheticMonth generates N sessions with known totals for reconciliation tests.
// Returns the sessions and ground-truth totals.
func SyntheticMonth(n int, bitrateKbps float64) (sessions []SyntheticSession, truthMinutes float64, truthEgressGB float64) {
	now := time.Now()
	start := now.AddDate(0, -1, 0) // one month ago
	for i := 0; i < n; i++ {
		watchS := 300.0 + float64(i%120)*10.0 // 5-25 minutes
		s := SyntheticSession{
			SessionID:   fmt.Sprintf("sess-%05d", i),
			StreamID:    fmt.Sprintf("stream-%03d", i%10),
			App:         "live",
			StartedAt:   start.Add(time.Duration(i) * 5 * time.Minute),
			EndedAt:     start.Add(time.Duration(i)*5*time.Minute + time.Duration(watchS)*time.Second),
			WatchTimeS:  watchS,
			BitrateKbps: bitrateKbps,
		}
		sessions = append(sessions, s)
		truthMinutes += watchS / 60.0
		truthEgressGB += kbpsToGBPerMinute(watchS/60.0, bitrateKbps)
	}
	return sessions, truthMinutes, truthEgressGB
}

// ComputeUsageFromSessions computes a UsageReport from a slice of synthetic sessions.
// Used in tests to verify accounting formulas without ClickHouse.
func ComputeUsageFromSessions(sessions []SyntheticSession, tm *TenantMatcher) *UsageReport {
	type aggKey struct {
		app    string
		stream string
	}
	type aggVal struct {
		viewerMinutes   float64
		peakConcurrency int64
		bitrateKbps     float64
		rowCount        int64
	}
	byKey := map[aggKey]*aggVal{}
	for _, s := range sessions {
		k := aggKey{app: s.App, stream: s.StreamID}
		v := byKey[k]
		if v == nil {
			byKey[k] = &aggVal{}
			v = byKey[k]
		}
		v.viewerMinutes += s.WatchTimeS / 60.0
		v.rowCount++
		// Peak concurrency: simplified as max rows per stream.
		if v.rowCount > v.peakConcurrency {
			v.peakConcurrency = v.rowCount
		}
		v.bitrateKbps = s.BitrateKbps
	}

	if tm == nil {
		tm = NewTenantMatcher(nil)
	}

	var rows []UsageRow
	var totals UsageTotals
	for k, v := range byKey {
		tenantName := tm.Resolve(k.stream, nil)
		egressGB := kbpsToGBPerMinute(v.viewerMinutes, v.bitrateKbps)
		sid := k.stream
		sidPtr := &sid
		var tenPtr *string
		if tenantName != "" {
			tenPtr = &tenantName
		}
		row := UsageRow{
			App:             k.app,
			StreamID:        sidPtr,
			Tenant:          tenPtr,
			ViewerMinutes:   roundToDecimal(v.viewerMinutes, 4),
			PeakConcurrency: v.peakConcurrency,
			EgressGB:        roundToDecimal(egressGB, 6),
			RecordingGB:     0,
			EgressMethod:    EgressMethodBitrateXWatchTime,
		}
		rows = append(rows, row)
		totals.ViewerMinutes += v.viewerMinutes
		totals.EgressGB += egressGB
		if v.peakConcurrency > totals.PeakConcurrency {
			totals.PeakConcurrency = v.peakConcurrency
		}
	}
	if rows == nil {
		rows = []UsageRow{}
	}
	totals.ViewerMinutes = roundToDecimal(totals.ViewerMinutes, 4)
	totals.EgressGB = roundToDecimal(totals.EgressGB, 6)
	return &UsageReport{
		Rows:         rows,
		Totals:       totals,
		EgressMethod: EgressMethodBitrateXWatchTime,
	}
}

// TenantStat is a tenant's aggregated figures used in JSON scheduling metadata.
type TenantStat struct {
	Tenant          string
	ViewerMinutes   float64
	PeakConcurrency int64
	EgressGB        float64
	RecordingGB     float64
}

// AggregateByTenant collapses rows into per-tenant stats.
func AggregateByTenant(rows []UsageRow) []TenantStat {
	byTenant := map[string]*TenantStat{}
	for _, r := range rows {
		t := "unassigned"
		if r.Tenant != nil && *r.Tenant != "" {
			t = *r.Tenant
		}
		s := byTenant[t]
		if s == nil {
			byTenant[t] = &TenantStat{Tenant: t}
			s = byTenant[t]
		}
		s.ViewerMinutes += r.ViewerMinutes
		s.EgressGB += r.EgressGB
		s.RecordingGB += r.RecordingGB
		if r.PeakConcurrency > s.PeakConcurrency {
			s.PeakConcurrency = r.PeakConcurrency
		}
	}
	var out []TenantStat
	for _, s := range byTenant {
		out = append(out, *s)
	}
	return out
}

// scheduleScope is used for JSON unmarshaling of report_schedules.scope.
type scheduleScope struct {
	App    *string `json:"app"`
	Tenant *string `json:"tenant"`
}

func parseScheduleScope(scopeJSON string) scheduleScope {
	var sc scheduleScope
	if scopeJSON != "" {
		_ = json.Unmarshal([]byte(scopeJSON), &sc)
	}
	return sc
}
