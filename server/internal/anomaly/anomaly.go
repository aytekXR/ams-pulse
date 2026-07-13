// Package anomaly implements F9 anomaly detection for Pulse.
//
// # Design
//
// Rolling per-(metric, scope, window_s) statistics are maintained in the
// anomaly_baselines meta table using Welford's online algorithm, which computes
// mean and variance in a single pass without storing history.
//
// # Sensitivity calibration — <1 false alarm per node-week (PRD F9)
//
// A Gaussian distribution's tail probability for |Z| >= sigma (two-tailed):
//
//	sigma=3.0 → P ≈ 2.70e-3 per observation
//	sigma=3.5 → P ≈ 4.65e-4 per observation
//	sigma=4.0 → P ≈ 6.33e-5 per observation
//
// Baseline update tick = 60 s → 10,080 ticks/node/week.
// Tracking 5 node-scoped metrics/node (viewers, cpu_pct, mem_pct, disk_pct,
// ams_api_latency_ms) = 50,400 raw obs/node-week.
// ingest_bitrate_kbps is stream-scoped and scales with stream count.
//
// With hysteresis (renewal-process model): after a false alarm, the next
// HysteresisTicks checks are suppressed. Effective false-alarm rate:
//
//	lambda_effective = lambda_raw / (1 + lambda_raw × HysteresisTicks)
//
// where lambda_raw = ticks/week × P (per metric).
//
//	sigma=4.0 per metric: lambda_raw = 10,080 × 6.33e-5 = 0.638/week
//	lambda_effective = 0.638 / (1 + 0.638 × 10) = 0.638 / 7.38 ≈ 0.086/week
//	across 4 node-only metrics (cpu_pct, mem_pct, disk_pct, ams_api_latency_ms):
//	  0.086 × 4 = 0.3442/node-week < 1.0 ← PASSES PRD target
//	conservative budget (+ viewers as 5th as-if-node-scoped):
//	  0.086 × 5 = 0.4322/node-week < 1.0 ← PASSES PRD target
//
// DefaultSigma=4.0 satisfies the PRD <1 false alarm/node-week at the default
// tick rate with hysteresis cooldown of 10 ticks.
//
// Summary of default calibration:
//
//	DefaultSigma    = 4.0   (configurable; min_sigma query param default per spec = 2.0)
//	MinSamples      = 30    (require 30 observations before flagging)
//	HysteresisTicks = 10    (suppress re-firing for 10 ticks after a flag)
//	TickInterval    = 60 s  (baseline update period)
//
// D-074 historical note: stale '3 metrics' text (viewers, error_rate, rebuffer_ratio)
// dates from pre-exclusion; rebuffer_ratio/error_rate are excluded by the wave3 sparsity
// gate (wave3_test.go:709 pin). Current budget: 5 metrics (D-087, see above).
package anomaly

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// DefaultSigma is the default number of standard deviations that triggers a flag.
// See package-level doc for calibration math.
// At σ=4.0 with MinSamples=30 and HysteresisTicks=10:
//
//	P(|Z|>=4.0) ≈ 6.33e-5 per observation.
//	raw flags/node/week (5 metrics, 10080 ticks) = 10080 × 6.33e-5 × 5 = 3.19
//	with hysteresis renewal process: lambda_flags = lambda / (1 + lambda × cooldown)
//	lambda = 10080 × 6.33e-5 = 0.638/week/metric
//	lambda_flags = 0.638 / (1 + 0.638 × 10) ≈ 0.638/7.38 ≈ 0.086/week/metric
//	across 5 metrics: 0.086 × 5 ≈ 0.43/node/week < 1.0. PASSES PRD target.
//	(ingest_bitrate_kbps is stream-scoped; excluded from per-node budget.
//	 ams_api_latency_ms added D-087 as the 4th truly-node-scoped metric.)
const DefaultSigma = 4.0

// MinSamples is the minimum number of samples required before anomaly flagging.
// Prevents false alarms during baseline warm-up.
const MinSamples = 30

// HysteresisTicks is the number of ticks to suppress re-firing after a flag.
// At 60 s/tick this is 10 × 60 = 600 s cooldown.
const HysteresisTicks = 10

// StddevRelEpsilon is the minimum effective stddev expressed as a fraction of
// |mean|. A value of 0.05 means the effective stddev is at least 5% of the
// absolute mean, implementing a coefficient-of-variation floor.
// This prevents constant-baseline metrics (stddev=0) from never flagging: a
// deviation must be less than 5% of the mean to avoid detection (at default σ=4.0).
const StddevRelEpsilon = 0.05

// StddevAbsEpsilon is a tiny absolute floor added only to avoid divide-by-zero
// when both mean and stddev are 0. At this floor a 1-unit deviation from a
// zero-mean, zero-stddev baseline produces a very large z-score and correctly flags.
const StddevAbsEpsilon = 1e-9

// BaselineStore is the interface to the meta store anomaly_baselines table.
type BaselineStore interface {
	// ListBaselines returns all anomaly_baselines rows.
	ListAnomalyBaselines(ctx context.Context) ([]AnomalyBaselineRow, error)

	// UpsertBaseline inserts or updates a baseline row keyed by (metric, scope, window_s).
	UpsertAnomalyBaseline(ctx context.Context, row AnomalyBaselineRow) error
}

// BaselineSweeper is an optional capability of BaselineStore implementations.
// When d.store satisfies this interface, Detector.Run sweeps stale zero-mean
// baselines on startup (before the first tick) to evict rows poisoned by
// standalone AMS nodes that never report cpu/mem/disk (D-088).
// The sweep is a Detector-startup operation; it is not a schema migration.
type BaselineSweeper interface {
	// DeleteZeroMeanNodeBaselines deletes baseline rows where metric is one of
	// the given metrics and both mean=0 and stddev=0. Returns the count deleted.
	DeleteZeroMeanNodeBaselines(ctx context.Context, metrics []string) (int64, error)
}

// AnomalyFlagEvent is a persisted record of one anomaly detection event.
// Written by Detector.UpdateBaselines (tick path) and queried by GET /anomalies
// with ?from/?to (ADR-0009 BUG-008 phase 2).
type AnomalyFlagEvent struct {
	ID         string
	Metric     string
	NodeID     string
	App        string
	StreamID   string
	Scope      string // raw JSON bytes from scopeJSON() — byte-identical to hysteresisKey.scope
	Observed   float64
	Expected   float64
	Sigma      float64   // z-score = |observed − expected| / effStddev
	DetectedAt time.Time // tick time (UTC); shared by all events of one tick
}

// FlagKey identifies a unique (metric, scope) pair in the hysteresis map.
// Used by FlagEventStore.RecentFlagKeys for WarmHysteresis startup warmup.
type FlagKey struct {
	Metric string
	Scope  string
}

// FlagEventStore persists detected anomaly flag events and supports startup
// warmup of the in-memory hysteresis map.
// Implemented by *clickhouse.Store; nil = no persistence (safe, no-op).
type FlagEventStore interface {
	// InsertAnomalyFlagEvent writes one flag event to the persistent store.
	// Called synchronously from Detector.UpdateBaselines after lock release.
	InsertAnomalyFlagEvent(ctx context.Context, event AnomalyFlagEvent) error

	// RecentFlagKeys returns distinct (metric, scope) pairs that had a flag event
	// within the last windowSecs seconds. Used by WarmHysteresis on startup to
	// pre-populate the in-memory hysteresis map (ADR-0009 §5).
	RecentFlagKeys(ctx context.Context, windowSecs int) ([]FlagKey, error)
}

// AnomalyBaselineRow mirrors the anomaly_baselines meta table.
type AnomalyBaselineRow struct {
	ID          string
	Metric      string
	Scope       string // JSON: {node_id, app, stream_id}
	WindowS     int
	Mean        float64
	Stddev      float64
	SampleCount int
	LastUpdated int64 // Unix epoch ms
}

// AnomalyFlag is a computed flag (returned by GET /anomalies).
// It matches the AnomalyFlag schema in contracts/openapi/pulse-api.yaml.
type AnomalyFlag struct {
	ID       string            `json:"id"`
	Metric   string            `json:"metric"`
	Scope    domain.AlertScope `json:"scope"`
	Observed float64           `json:"observed"`
	Expected float64           `json:"expected"`
	Sigma    float64           `json:"sigma"`
	TS       int64             `json:"ts"` // Unix epoch ms
}

// hysteresisKey is a composite key for the hysteresis map.
type hysteresisKey struct {
	metric string
	scope  string
}

// Detector maintains rolling baselines and computes anomaly flags on demand.
type Detector struct {
	mu              sync.Mutex
	store           BaselineStore
	live            domain.LiveProvider
	defaultSigma    float64
	minSamples      int
	hysteresisTicks int
	tickInterval    time.Duration

	// hysteresis tracks how many ticks remain before re-firing is allowed.
	hysteresis map[hysteresisKey]int

	// flagStore persists detected flag events (nil = disabled, no-op).
	// Injected via SetFlagStore; nil by default so existing tests are ClickHouse-free.
	flagStore FlagEventStore

	logger *slog.Logger
}

// Config holds Detector configuration.
type Config struct {
	// DefaultSigma is the default sigma threshold. 0 → DefaultSigma (3.5).
	DefaultSigma float64
	// MinSamples is the minimum sample count before flagging. 0 → MinSamples (30).
	MinSamples int
	// HysteresisTicks is the cooldown tick count. 0 → HysteresisTicks (10).
	HysteresisTicks int
	// TickInterval is how often UpdateBaselines is called. 0 → 60 s.
	TickInterval time.Duration
}

// New creates a Detector.
func New(cfg Config, store BaselineStore, live domain.LiveProvider, logger *slog.Logger) *Detector {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.DefaultSigma == 0 {
		cfg.DefaultSigma = DefaultSigma
	}
	if cfg.MinSamples == 0 {
		cfg.MinSamples = MinSamples
	}
	if cfg.HysteresisTicks == 0 {
		cfg.HysteresisTicks = HysteresisTicks
	}
	if cfg.TickInterval == 0 {
		cfg.TickInterval = 60 * time.Second
	}
	return &Detector{
		store:           store,
		live:            live,
		defaultSigma:    cfg.DefaultSigma,
		minSamples:      cfg.MinSamples,
		hysteresisTicks: cfg.HysteresisTicks,
		tickInterval:    cfg.TickInterval,
		hysteresis:      make(map[hysteresisKey]int),
		logger:          logger,
	}
}

// SetFlagStore wires an optional persistent store for anomaly flag events.
// Call after New, before Run. When nil (default), flag events are never persisted
// and all existing behaviour is unchanged (existing tests remain ClickHouse-free).
func (d *Detector) SetFlagStore(s FlagEventStore) {
	d.flagStore = s
}

// WarmHysteresis pre-populates the in-memory hysteresis map from the store on
// startup, preventing duplicate flag events after a process restart (ADR-0009 §5).
//
// windowSecs = hysteresisTicks × tickInterval.Seconds() (default: 10 × 60 = 600 s).
// For each (metric, scope) pair returned by the store, hysteresis is set to
// hysteresisTicks so the next tick is suppressed for the full cooldown window.
//
// No-op when flagStore is nil. Store errors are returned to the caller (Run logs
// and continues); the detector remains safe to use with an empty hysteresis map.
func (d *Detector) WarmHysteresis(ctx context.Context) error {
	if d.flagStore == nil {
		return nil
	}
	warmupSecs := d.hysteresisTicks * int(d.tickInterval.Seconds())
	keys, err := d.flagStore.RecentFlagKeys(ctx, warmupSecs)
	if err != nil {
		return err
	}
	d.mu.Lock()
	for _, k := range keys {
		d.hysteresis[hysteresisKey{metric: k.Metric, scope: k.Scope}] = d.hysteresisTicks
	}
	d.mu.Unlock()
	return nil
}

// Run starts the baseline update tick loop. Blocks until ctx is cancelled.
// WarmHysteresis is called once before the first tick to restore cooldown state
// across process restarts when a flagStore is configured.
// When d.store implements BaselineSweeper, DeleteZeroMeanNodeBaselines is called
// once after WarmHysteresis to evict baselines poisoned by standalone AMS nodes
// that never report cpu_pct/mem_pct/disk_pct (D-088 startup sweep).
func (d *Detector) Run(ctx context.Context) {
	ticker := time.NewTicker(d.tickInterval)
	defer ticker.Stop()
	if err := d.WarmHysteresis(ctx); err != nil {
		d.logger.Warn("anomaly: WarmHysteresis failed", "error", err)
	}
	if sweeper, ok := d.store.(BaselineSweeper); ok {
		n, err := sweeper.DeleteZeroMeanNodeBaselines(ctx, []string{"cpu_pct", "mem_pct", "disk_pct"})
		if err != nil {
			d.logger.Warn("anomaly: zero-mean baseline sweep failed", "error", err)
		} else if n > 0 {
			d.logger.Info("anomaly: purged zero-mean baselines on startup", "count", n)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.UpdateBaselines(ctx); err != nil {
				d.logger.Warn("anomaly: baseline update failed", "error", err)
			}
		}
	}
}

// UpdateBaselines reads the current live snapshot and updates the rolling
// Welford statistics for each observed metric × scope combination.
//
// Metrics tracked per scope (node_id × stream):
//   - viewers (viewer_count per stream, via scope=stream_id)
//   - node-level: cpu_pct and mem_pct (scope=node_id)
//
// The window_s is fixed to 3600 (1 hour) for now, as a rolling "current hour"
// perspective. Multiple windows are a Phase 3 extension.
//
// After the Welford loop, checkFlags is called with the UPDATED baseline rows
// (not the stale pre-Welford slice) and the current live values to detect and
// persist new anomaly flag events (ADR-0009 §4).
func (d *Detector) UpdateBaselines(ctx context.Context) error {
	snap := d.live.CurrentSnapshot()
	if snap == nil {
		return nil
	}

	// Capture tick time BEFORE the Welford loop. All flag events of this tick share
	// DetectedAt=tickAt. The existing 'now' int64 below stays as-is (different purpose).
	tickAt := time.Now().UTC()

	// Update hysteresis counters (decrement each tick).
	d.mu.Lock()
	for k, rem := range d.hysteresis {
		if rem <= 1 {
			delete(d.hysteresis, k)
		} else {
			d.hysteresis[k] = rem - 1
		}
	}
	d.mu.Unlock()

	now := time.Now().UnixMilli()
	const windowS = 3600 // 1-hour rolling window

	// Collect observations: per-stream viewer counts + ingest bitrate; per-node CPU/mem/disk.
	type observation struct {
		metric string
		scope  AnomalyBaselineRow
		value  float64
	}
	var obs []observation

	for streamID, s := range snap.Streams {
		scope := scopeJSON("", "", streamID)
		obs = append(obs, observation{
			metric: "viewers",
			scope:  AnomalyBaselineRow{Metric: "viewers", Scope: scope, WindowS: windowS},
			value:  float64(s.ViewerCount),
		})
		// ingest_bitrate_kbps: Detector key == rule metric name (no alias).
		obs = append(obs, observation{
			metric: "ingest_bitrate_kbps",
			scope:  AnomalyBaselineRow{Metric: "ingest_bitrate_kbps", Scope: scope, WindowS: windowS},
			value:  s.IngestBitrate,
		})
	}

	for nodeID, n := range snap.Nodes {
		nodeScope := scopeJSON(nodeID, "", "")
		// D-088: guard on presence flags — standalone AMS 3.x omits cpu/mem/disk
		// (normalize.go:241). Feeding zero-value observations would poison the Welford
		// baseline toward zero, causing false alarms when the node later reports real data.
		// cluster path sets all three flags; standalone path leaves them false.
		if n.CPUPCTReported {
			obs = append(obs, observation{
				metric: "cpu_pct",
				scope:  AnomalyBaselineRow{Metric: "cpu_pct", Scope: nodeScope, WindowS: windowS},
				value:  n.CPUPCT,
			})
		}
		if n.MemPCTReported {
			obs = append(obs, observation{
				metric: "mem_pct",
				scope:  AnomalyBaselineRow{Metric: "mem_pct", Scope: nodeScope, WindowS: windowS},
				value:  n.MemPCT,
			})
		}
		// disk_pct: node-scoped, Detector key == rule metric name (no alias).
		if n.DiskPCTReported {
			obs = append(obs, observation{
				metric: "disk_pct",
				scope:  AnomalyBaselineRow{Metric: "disk_pct", Scope: nodeScope, WindowS: windowS},
				value:  n.DiskPCT,
			})
		}
		// ams_api_latency_ms: Pulse-measured poller RTT (D-087 rung-1 feed).
		// Key-absent semantics (D-075): skip when APILatencyMS==0, which signals
		// either that the last stats call FAILED or the node has never been polled.
		// Feeding 0 would poison the Welford baseline toward zero and make normal
		// latency look anomalous. An honest sub-ms RTT rounds to a small positive
		// float64 (e.g. 0.1 ms), never exactly 0 — so 0 is a safe sentinel.
		if n.APILatencyMS > 0 {
			obs = append(obs, observation{
				metric: "ams_api_latency_ms",
				scope:  AnomalyBaselineRow{Metric: "ams_api_latency_ms", Scope: nodeScope, WindowS: windowS},
				value:  n.APILatencyMS,
			})
		}
	}

	// Load current baselines to get existing M2 (needed for Welford).
	existing, err := d.store.ListAnomalyBaselines(ctx)
	if err != nil {
		return err
	}
	// Index by (metric, scope, window_s). Both pre-existing and newly created rows
	// are tracked here so the post-Welford snapshot contains UPDATED values.
	baselineIdx := make(map[string]*AnomalyBaselineRow, len(existing))
	for i := range existing {
		key := baselineKey(existing[i].Metric, existing[i].Scope, existing[i].WindowS)
		baselineIdx[key] = &existing[i]
	}

	for _, o := range obs {
		key := baselineKey(o.scope.Metric, o.scope.Scope, o.scope.WindowS)
		row, ok := baselineIdx[key]
		if !ok {
			row = &AnomalyBaselineRow{
				ID:      uuid.New().String(),
				Metric:  o.scope.Metric,
				Scope:   o.scope.Scope,
				WindowS: o.scope.WindowS,
			}
			// Track new rows in the index so checkFlags sees updated values.
			baselineIdx[key] = row
		}

		// Welford online update:
		// count += 1
		// delta  = x - mean
		// mean  += delta / count
		// delta2 = x - mean   (new mean)
		// M2    += delta * delta2
		// variance = M2 / (count - 1) if count >= 2 else 0
		// stddev   = sqrt(variance)
		//
		// We store mean and stddev directly; M2 is re-derived as stddev² × (n-1).
		n := row.SampleCount + 1
		delta := o.value - row.Mean
		newMean := row.Mean + delta/float64(n)
		delta2 := o.value - newMean

		// M2_prev = stddev_prev² × (n-1)
		var m2 float64
		if row.SampleCount >= 2 {
			m2 = row.Stddev * row.Stddev * float64(row.SampleCount-1)
		}
		m2 += delta * delta2

		var newStddev float64
		if n >= 2 {
			variance := m2 / float64(n-1)
			if variance > 0 {
				newStddev = math.Sqrt(variance)
			}
		}

		row.SampleCount = n
		row.Mean = newMean
		row.Stddev = newStddev
		row.LastUpdated = now

		if err := d.store.UpsertAnomalyBaseline(ctx, *row); err != nil {
			d.logger.Warn("anomaly: upsert baseline failed", "metric", o.metric, "error", err)
		}
	}

	// Build liveValues from the observation slice (key = metric + ":" + scope).
	liveValues := make(map[string]float64, len(obs))
	for _, o := range obs {
		liveValues[o.metric+":"+o.scope.Scope] = o.value
	}

	// Materialise UPDATED baseline rows from the index (NOT the stale pre-Welford
	// 'existing' slice — stale mean/stddev would produce wrong z-scores).
	updatedBaselines := make([]AnomalyBaselineRow, 0, len(baselineIdx))
	for _, row := range baselineIdx {
		updatedBaselines = append(updatedBaselines, *row)
	}

	// Detect flags and persist via flagStore (runs unconditionally; only the
	// Insert is gated on flagStore != nil inside checkFlags).
	d.checkFlags(ctx, tickAt, updatedBaselines, liveValues)

	return nil
}

// detectedFlag is the output of detectFlagsLocked: one detection result whose
// caller translates into either an AnomalyFlag (ComputeFlags) or an
// AnomalyFlagEvent (checkFlags). Hysteresis has already been set for each entry.
type detectedFlag struct {
	baseline AnomalyBaselineRow
	observed float64
	zScore   float64
}

// detectFlagsLocked runs the z-score detection pass and updates the in-memory
// hysteresis map. MUST be called with d.mu held. Returns one detectedFlag per
// new flag; re-fires within the cooldown window are suppressed.
//
// Replicates the minSamples guard and effStddev epsilon logic exactly so both
// ComputeFlags and checkFlags produce consistent results.
func (d *Detector) detectFlagsLocked(baselines []AnomalyBaselineRow, liveValues map[string]float64, sigmaThreshold float64) []detectedFlag {
	var out []detectedFlag
	for _, b := range baselines {
		if b.SampleCount < d.minSamples {
			continue // not enough history (warmup guard — matches ComputeFlags :384)
		}
		liveKey := b.Metric + ":" + b.Scope
		observed, ok := liveValues[liveKey]
		if !ok {
			continue // metric not currently observed (stream offline etc.)
		}

		// Effective stddev with epsilon floor (matches ComputeFlags :399):
		//   - StddevRelEpsilon*|mean| prevents flagging on tiny relative deviations.
		//   - StddevAbsEpsilon avoids divide-by-zero when mean=0 and stddev=0.
		effStddev := math.Max(b.Stddev, math.Max(StddevRelEpsilon*math.Abs(b.Mean), StddevAbsEpsilon))
		z := math.Abs(observed-b.Mean) / effStddev
		if z < sigmaThreshold {
			continue
		}

		// Check and set hysteresis (matches ComputeFlags :407/:412).
		hk := hysteresisKey{metric: b.Metric, scope: b.Scope}
		if rem := d.hysteresis[hk]; rem > 0 {
			continue // still in cooldown
		}
		d.hysteresis[hk] = d.hysteresisTicks

		out = append(out, detectedFlag{baseline: b, observed: observed, zScore: z})
	}
	return out
}

// checkFlags detects anomaly flags from the UPDATED baselines and live values
// produced by one UpdateBaselines tick, and persists each new flag event via
// flagStore.
//
// Runs unconditionally (hysteresis bookkeeping is always performed). Only the
// InsertAnomalyFlagEvent call is gated on flagStore != nil.
//
// The lock is held only during detection; CH inserts run after lock release to
// avoid holding d.mu across a network call (ComputeFlags also contends on d.mu).
// Insert failures are logged and dropped — hysteresis is already set so an
// at-most-once undercount is acceptable (no phantom re-fires on the next tick).
func (d *Detector) checkFlags(ctx context.Context, tickAt time.Time, baselines []AnomalyBaselineRow, liveValues map[string]float64) {
	d.mu.Lock()
	detected := d.detectFlagsLocked(baselines, liveValues, d.defaultSigma)
	d.mu.Unlock()

	if d.flagStore == nil {
		return
	}

	for _, df := range detected {
		scope := parseScopeJSON(df.baseline.Scope)
		ev := AnomalyFlagEvent{
			ID:         uuid.New().String(),
			Metric:     df.baseline.Metric,
			NodeID:     scope.NodeID,
			App:        scope.App,
			StreamID:   scope.StreamID,
			Scope:      df.baseline.Scope, // raw JSON bytes, byte-identical to hysteresisKey.scope
			Observed:   df.observed,
			Expected:   df.baseline.Mean,
			Sigma:      df.zScore,
			DetectedAt: tickAt,
		}
		if err := d.flagStore.InsertAnomalyFlagEvent(ctx, ev); err != nil {
			// Log and drop: hysteresis is already set; at-most-once undercount acceptable.
			d.logger.Warn("anomaly: insert flag event failed, dropping (at-most-once undercount acceptable)",
				"metric", ev.Metric, "scope", ev.Scope, "error", err)
		}
	}
}

// ComputeFlags computes AnomalyFlag entries on-read by comparing current
// live values against stored baselines.
//
// Only baselines with sample_count >= minSamples are flagged.
// A flag is emitted when |observed - mean| / stddev >= sigmaThreshold.
// Hysteresis suppresses re-firing within HysteresisTicks ticks.
//
// ComputeFlags NEVER writes to flagStore — it is the HTTP on-demand path only.
// Persistent flag events are written exclusively by checkFlags (called from
// UpdateBaselines on the single-goroutine tick path — ADR-0009 §4).
func (d *Detector) ComputeFlags(ctx context.Context, sigmaThreshold float64) ([]AnomalyFlag, error) {
	if sigmaThreshold <= 0 {
		sigmaThreshold = d.defaultSigma
	}

	snap := d.live.CurrentSnapshot()
	if snap == nil {
		return nil, nil
	}

	baselines, err := d.store.ListAnomalyBaselines(ctx)
	if err != nil {
		return nil, err
	}

	// Build live values map.
	liveValues := make(map[string]float64) // key = "metric:scopeJSON"
	for streamID, s := range snap.Streams {
		ss := scopeJSON("", "", streamID)
		liveValues["viewers:"+ss] = float64(s.ViewerCount)
		liveValues["ingest_bitrate_kbps:"+ss] = s.IngestBitrate
	}
	for nodeID, n := range snap.Nodes {
		ns := scopeJSON(nodeID, "", "")
		// D-088: presence guards mirror UpdateBaselines — key absent from liveValues
		// when the metric was not reported by the node. detectFlagsLocked skips any
		// baseline whose liveKey is absent, so stale zero-mean baselines cannot fire
		// false alarms against unreported (zero) live values.
		if n.CPUPCTReported {
			liveValues["cpu_pct:"+ns] = n.CPUPCT
		}
		if n.MemPCTReported {
			liveValues["mem_pct:"+ns] = n.MemPCT
		}
		if n.DiskPCTReported {
			liveValues["disk_pct:"+ns] = n.DiskPCT
		}
		// ams_api_latency_ms: same presence guard as UpdateBaselines (D-087 1b).
		// Skip when APILatencyMS==0 (no measurement); avoid populating with a
		// zero that would produce a spurious flag against the real baseline.
		if n.APILatencyMS > 0 {
			liveValues["ams_api_latency_ms:"+ns] = n.APILatencyMS
		}
	}

	// TS is captured once per ComputeFlags call (not per detected flag).
	// This is the ephemeral HTTP-response timestamp; detected_at in persisted
	// events uses tickAt from UpdateBaselines (ADR-0009 Consequences §1).
	now := time.Now().UnixMilli()

	d.mu.Lock()
	detected := d.detectFlagsLocked(baselines, liveValues, sigmaThreshold)
	d.mu.Unlock()

	flags := make([]AnomalyFlag, 0, len(detected))
	for _, df := range detected {
		flags = append(flags, AnomalyFlag{
			ID:       uuid.New().String(),
			Metric:   df.baseline.Metric,
			Scope:    parseScopeJSON(df.baseline.Scope),
			Observed: df.observed,
			Expected: df.baseline.Mean,
			Sigma:    df.zScore,
			TS:       now,
		})
	}
	return flags, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// scopeJSON builds the canonical JSON scope string for a baseline key.
// Only non-empty fields are included.
func scopeJSON(nodeID, app, streamID string) string {
	// Build minimal JSON without full encoding overhead.
	var parts []string
	if nodeID != "" {
		parts = append(parts, `"node_id":"`+nodeID+`"`)
	}
	if app != "" {
		parts = append(parts, `"app":"`+app+`"`)
	}
	if streamID != "" {
		parts = append(parts, `"stream_id":"`+streamID+`"`)
	}
	if len(parts) == 0 {
		return "{}"
	}
	result := "{"
	for i, p := range parts {
		if i > 0 {
			result += ","
		}
		result += p
	}
	result += "}"
	return result
}

// baselineKey returns the unique index key for a baseline.
func baselineKey(metric, scope string, windowS int) string {
	return metric + ":" + scope + ":" + itoa(windowS)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := 20
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// parseScopeJSON parses a scope JSON string into an AlertScope.
// Minimal parser for the canonical format produced by scopeJSON above.
func parseScopeJSON(s string) domain.AlertScope {
	var scope domain.AlertScope
	if s == "" || s == "{}" {
		return scope
	}
	// Simple extraction without full JSON unmarshal.
	if v := extractJSONString(s, "node_id"); v != "" {
		scope.NodeID = v
	}
	if v := extractJSONString(s, "app"); v != "" {
		scope.App = v
	}
	if v := extractJSONString(s, "stream_id"); v != "" {
		scope.StreamID = v
	}
	return scope
}

// extractJSONString extracts the string value for a key from a simple flat JSON object.
func extractJSONString(obj, key string) string {
	needle := `"` + key + `":"`
	idx := 0
	for i := 0; i <= len(obj)-len(needle); i++ {
		if obj[i:i+len(needle)] == needle {
			idx = i + len(needle)
			end := idx
			for end < len(obj) && obj[end] != '"' {
				end++
			}
			return obj[idx:end]
		}
	}
	return ""
}
