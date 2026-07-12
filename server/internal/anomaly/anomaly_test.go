package anomaly_test

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

type fakeBaselineStore struct {
	rows []anomaly.AnomalyBaselineRow
}

func (f *fakeBaselineStore) ListAnomalyBaselines(_ context.Context) ([]anomaly.AnomalyBaselineRow, error) {
	out := make([]anomaly.AnomalyBaselineRow, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

func (f *fakeBaselineStore) UpsertAnomalyBaseline(_ context.Context, row anomaly.AnomalyBaselineRow) error {
	for i, r := range f.rows {
		if r.Metric == row.Metric && r.Scope == row.Scope && r.WindowS == row.WindowS {
			f.rows[i] = row
			return nil
		}
	}
	f.rows = append(f.rows, row)
	return nil
}

// fakeStreamLive provides a synthetic live snapshot for testing.
type fakeStreamLive struct {
	viewerCount int
}

func (f *fakeStreamLive) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"stream1": {
				StreamID:    "stream1",
				ViewerCount: f.viewerCount,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func (f *fakeStreamLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestAnomaly_SteadyStream_NoFlag verifies that a stable metric stream
// at the baseline mean produces no flags.
func TestAnomaly_SteadyStream_NoFlag(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeStreamLive{viewerCount: 100}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.5,
		MinSamples:      5, // low for test
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed 10 steady observations at value=100.
	for i := 0; i < 10; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines: %v", err)
		}
	}

	// Verify a below-threshold wobble (±0.5σ) does not flag.
	live.viewerCount = 101 // tiny wobble

	flags, err := det.ComputeFlags(ctx, 3.5)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags for steady stream with minor wobble, got %d", len(flags))
	}
	t.Logf("PASS: steady stream → 0 flags (wobble of +1 viewer; σ<threshold)")
}

// variableStreamLive provides a live snapshot with a controllable viewer count.
// Feed alternating values during warm-up to build a non-zero stddev baseline,
// then inject a large deviation to test flagging.
type variableStreamLive struct {
	viewerCount int
}

func (f *variableStreamLive) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"stream1": {
				StreamID:    "stream1",
				ViewerCount: f.viewerCount,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func (f *variableStreamLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// TestAnomaly_Injection_OneFlag verifies that an injected N-sigma deviation
// produces exactly one flag (not a storm — hysteresis works).
// Uses alternating values during warm-up so stddev > 0.
func TestAnomaly_Injection_OneFlag(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &variableStreamLive{viewerCount: 100}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0, // lower threshold for test
		MinSamples:      5,
		HysteresisTicks: 3, // small for test
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed 10 alternating observations (95, 105, 95, 105, ...) to build stddev≈5.
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			live.viewerCount = 95
		} else {
			live.viewerCount = 105
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	// Check that baseline was established with non-zero stddev.
	baselines, _ := store.ListAnomalyBaselines(ctx)
	if len(baselines) == 0 {
		t.Fatal("no baselines established after 10 updates")
	}
	var viewerBaseline *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "viewers" {
			viewerBaseline = &baselines[i]
			break
		}
	}
	if viewerBaseline == nil {
		t.Fatal("viewers baseline not found")
	}
	t.Logf("baseline after 10 ticks: mean=%.2f stddev=%.4f samples=%d",
		viewerBaseline.Mean, viewerBaseline.Stddev, viewerBaseline.SampleCount)

	if viewerBaseline.Stddev <= 0 {
		t.Fatalf("expected stddev > 0 after alternating inputs, got %.4f", viewerBaseline.Stddev)
	}

	// Inject a value 20× the standard deviation above the mean.
	// This guarantees |Z| >> 3.0 and must produce a flag.
	injectedValue := int(viewerBaseline.Mean + 20.0*viewerBaseline.Stddev)
	live.viewerCount = injectedValue
	t.Logf("injecting viewer count=%d (mean=%.1f, +20σ=%.1f)",
		injectedValue, viewerBaseline.Mean, viewerBaseline.Mean+20.0*viewerBaseline.Stddev)

	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags after injection: %v", err)
	}

	if len(flags) == 0 {
		t.Errorf("expected at least 1 flag after 20σ deviation, got 0 (mean=%.2f, stddev=%.4f)",
			viewerBaseline.Mean, viewerBaseline.Stddev)
		return
	}
	if len(flags) != 1 {
		t.Errorf("expected exactly 1 flag from deviation, got %d", len(flags))
	}
	if flags[0].Metric != "viewers" {
		t.Errorf("expected metric=viewers, got %q", flags[0].Metric)
	}
	if flags[0].Sigma < 3.0 {
		t.Errorf("expected sigma >= 3.0, got %.2f", flags[0].Sigma)
	}
	t.Logf("PASS: injected deviation → 1 flag (sigma=%.2f observed=%.1f expected=%.2f)",
		flags[0].Sigma, flags[0].Observed, flags[0].Expected)

	// Calling ComputeFlags again immediately should produce 0 flags (hysteresis).
	flags2, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags (2nd call): %v", err)
	}
	if len(flags2) != 0 {
		t.Errorf("hysteresis failed: expected 0 flags on re-check, got %d", len(flags2))
	}
	t.Logf("PASS: hysteresis → 0 flags on re-check immediately after first flag")
}

// TestAnomaly_BelowThreshold_NoFlag verifies that a below-threshold wobble
// (even after warmup) produces no flags.
func TestAnomaly_BelowThreshold_NoFlag(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeStreamLive{viewerCount: 100}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.5,
		MinSamples:      5,
		HysteresisTicks: 3,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed steady observations but with slight noise (+/- 2).
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			live.viewerCount = 102
		} else {
			live.viewerCount = 98
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines: %v", err)
		}
	}

	// Now set to mean level (100) — well within 1 sigma.
	live.viewerCount = 100
	flags, err := det.ComputeFlags(ctx, 3.5)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags for in-band value, got %d", len(flags))
	}
	t.Logf("PASS: below-threshold value → 0 flags")
}

// TestAnomaly_MinSamples_Gate verifies that flags are suppressed until
// minSamples is reached.
func TestAnomaly_MinSamples_Gate(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeStreamLive{viewerCount: 100}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    1.0, // very low threshold
		MinSamples:      30,  // requires 30 samples
		HysteresisTicks: 1,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Only 5 updates — below minSamples=30.
	for i := 0; i < 5; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines: %v", err)
		}
	}

	// Even with a large deviation, no flags because samples < minSamples.
	live.viewerCount = 1000
	flags, err := det.ComputeFlags(ctx, 1.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags with only 5 samples (minSamples=30), got %d", len(flags))
	}
	t.Logf("PASS: minSamples gate → 0 flags when sample_count < minSamples")
}

// TestAnomaly_ConstantBaseline_LargeDeviation_Flags verifies that a constant
// baseline (all observations identical → stddev=0) still flags when a large
// deviation is observed. This closes the zero-stddev blind spot (GAP-3-004).
func TestAnomaly_ConstantBaseline_LargeDeviation_Flags(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeStreamLive{viewerCount: 200}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    4.0,
		MinSamples:      anomaly.MinSamples, // 30
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed MinSamples identical observations so stddev=0.
	for i := 0; i < anomaly.MinSamples; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	// Confirm the baseline is constant (stddev=0).
	baselines, _ := store.ListAnomalyBaselines(ctx)
	var vb *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "viewers" {
			vb = &baselines[i]
			break
		}
	}
	if vb == nil {
		t.Fatal("viewers baseline not found after warmup")
	}
	if vb.Stddev != 0 {
		t.Fatalf("expected stddev=0 for constant baseline, got %.6f", vb.Stddev)
	}
	t.Logf("baseline: mean=%.1f stddev=%.6f samples=%d", vb.Mean, vb.Stddev, vb.SampleCount)

	// Observe a large deviation (1000 vs mean=200): should flag despite stddev=0.
	// With StddevRelEpsilon=0.05, effStddev = 0.05*200 = 10; z = |1000-200|/10 = 80 >> σ=4.
	live.viewerCount = 1000
	flags, err := det.ComputeFlags(ctx, 4.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) != 1 {
		t.Errorf("expected exactly 1 flag for large deviation on constant baseline, got %d", len(flags))
	} else {
		t.Logf("PASS: constant baseline + large deviation → 1 flag (sigma=%.2f)", flags[0].Sigma)
	}
}

// TestAnomaly_ConstantBaseline_SmallDeviation_NoFlag verifies that a small
// deviation within the relative floor (StddevRelEpsilon) does NOT flag on a
// constant baseline. This ensures the epsilon floor does not cause spurious alerts.
func TestAnomaly_ConstantBaseline_SmallDeviation_NoFlag(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeStreamLive{viewerCount: 200}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    4.0,
		MinSamples:      anomaly.MinSamples, // 30
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed MinSamples identical observations so stddev=0.
	for i := 0; i < anomaly.MinSamples; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	// Observe 205 vs mean=200: deviation=5, 2.5% of mean.
	// effStddev = max(0, 0.05*200, 1e-9) = 10; z = 5/10 = 0.5 << σ=4.0 → no flag.
	live.viewerCount = 205
	flags, err := det.ComputeFlags(ctx, 4.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags for small deviation within relative floor, got %d", len(flags))
	} else {
		t.Logf("PASS: constant baseline + small deviation (2.5%% of mean) → 0 flags")
	}
}

// fakeIngestBitrateLive provides a live snapshot with a stream having a
// configurable IngestBitrate. Used by TestAnomaly_IngestBitrate_Baselines.
type fakeIngestBitrateLive struct {
	ingestBitrate float64
}

func (f *fakeIngestBitrateLive) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"stream1": {
				StreamID:      "stream1",
				IngestBitrate: f.ingestBitrate,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func (f *fakeIngestBitrateLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// fakeDiskPctLive provides a live snapshot with a node having a configurable DiskPCT.
// Used by TestAnomaly_DiskPct_Baselines.
type fakeDiskPctLive struct {
	diskPCT float64
}

func (f *fakeDiskPctLive) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			"node1": {
				NodeID:  "node1",
				DiskPCT: f.diskPCT,
			},
		},
	}
}

func (f *fakeDiskPctLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// TestAnomaly_IngestBitrate_Baselines verifies that UpdateBaselines writes a
// baseline row keyed by metric="ingest_bitrate_kbps" with the correct stream scope,
// and that ComputeFlags emits a flag when the observed value deviates significantly.
//
// RED: fails before anomaly.go UpdateBaselines/ComputeFlags observe IngestBitrate.
// GREEN: passes once those paths are added (D-074 WO-D).
func TestAnomaly_IngestBitrate_Baselines(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeIngestBitrateLive{ingestBitrate: 5000.0}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed MinSamples steady observations so the baseline warms up.
	for i := 0; i < 5; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	// Assert: a baseline row with metric="ingest_bitrate_kbps" must now exist.
	baselines, err := store.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}
	var found *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "ingest_bitrate_kbps" {
			found = &baselines[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected baseline row with metric=ingest_bitrate_kbps, got none (UpdateBaselines must observe IngestBitrate)")
	}
	if found.SampleCount < 5 {
		t.Errorf("expected SampleCount>=5, got %d", found.SampleCount)
	}
	// Scope must be stream-scoped: {"stream_id":"stream1"}
	if found.Scope != `{"stream_id":"stream1"}` {
		t.Errorf("expected scope={\"stream_id\":\"stream1\"}, got %q", found.Scope)
	}
	t.Logf("ingest_bitrate_kbps baseline: mean=%.2f stddev=%.4f samples=%d scope=%s",
		found.Mean, found.Stddev, found.SampleCount, found.Scope)

	// Build a non-zero stddev by alternating the bitrate.
	for i := 0; i < 5; i++ {
		if i%2 == 0 {
			live.ingestBitrate = 4500.0
		} else {
			live.ingestBitrate = 5500.0
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines (vary) iter %d: %v", i, err)
		}
	}

	// Inject a very large spike — ComputeFlags must return a flag for ingest_bitrate_kbps.
	// With effStddev >= 0.05*mean ≈ 250, a value of 50000 gives z=(50000-5000)/250=180>>3.0.
	live.ingestBitrate = 50000.0
	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	var bitrateFlag *anomaly.AnomalyFlag
	for i := range flags {
		if flags[i].Metric == "ingest_bitrate_kbps" {
			bitrateFlag = &flags[i]
			break
		}
	}
	if bitrateFlag == nil {
		t.Fatal("expected AnomalyFlag with metric=ingest_bitrate_kbps after large ingest_bitrate deviation, got none")
	}
	if bitrateFlag.Scope.StreamID != "stream1" {
		t.Errorf("expected flag Scope.StreamID=stream1, got %q", bitrateFlag.Scope.StreamID)
	}
	t.Logf("PASS: ingest_bitrate_kbps anomaly flag: sigma=%.2f observed=%.1f expected=%.2f",
		bitrateFlag.Sigma, bitrateFlag.Observed, bitrateFlag.Expected)
}

// TestAnomaly_DiskPct_Baselines verifies that UpdateBaselines writes a baseline
// row keyed by metric="disk_pct" with the correct node scope, and that
// ComputeFlags emits a flag when the observed value deviates significantly.
//
// RED: fails before anomaly.go UpdateBaselines/ComputeFlags observe DiskPCT.
// GREEN: passes once those paths are added (D-074 WO-D).
func TestAnomaly_DiskPct_Baselines(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeDiskPctLive{diskPCT: 45.0}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed MinSamples steady observations so the baseline warms up.
	for i := 0; i < 5; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	// Assert: a baseline row with metric="disk_pct" must now exist.
	baselines, err := store.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}
	var found *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "disk_pct" {
			found = &baselines[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected baseline row with metric=disk_pct, got none (UpdateBaselines must observe DiskPCT)")
	}
	if found.SampleCount < 5 {
		t.Errorf("expected SampleCount>=5, got %d", found.SampleCount)
	}
	// Scope must be node-scoped: {"node_id":"node1"}
	if found.Scope != `{"node_id":"node1"}` {
		t.Errorf("expected scope={\"node_id\":\"node1\"}, got %q", found.Scope)
	}
	t.Logf("disk_pct baseline: mean=%.2f stddev=%.4f samples=%d scope=%s",
		found.Mean, found.Stddev, found.SampleCount, found.Scope)

	// Build a non-zero stddev by alternating the disk usage.
	for i := 0; i < 5; i++ {
		if i%2 == 0 {
			live.diskPCT = 43.0
		} else {
			live.diskPCT = 47.0
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines (vary) iter %d: %v", i, err)
		}
	}

	// Inject a near-full-disk spike. With effStddev >= 0.05*45 ≈ 2.25,
	// z = (99-45)/2.25 = 24 >> 3.0 → must flag.
	live.diskPCT = 99.0
	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	var diskFlag *anomaly.AnomalyFlag
	for i := range flags {
		if flags[i].Metric == "disk_pct" {
			diskFlag = &flags[i]
			break
		}
	}
	if diskFlag == nil {
		t.Fatal("expected AnomalyFlag with metric=disk_pct after large disk_pct deviation, got none")
	}
	if diskFlag.Scope.NodeID != "node1" {
		t.Errorf("expected flag Scope.NodeID=node1, got %q", diskFlag.Scope.NodeID)
	}
	t.Logf("PASS: disk_pct anomaly flag: sigma=%.2f observed=%.1f expected=%.2f",
		diskFlag.Sigma, diskFlag.Observed, diskFlag.Expected)
}

// TestAnomaly_FalseAlarmRate documents the modeled false-alarm rate.
// This is a documentation/calibration test, not a stochastic simulation.
//
// Model: renewal-process with hysteresis suppression.
// After a false alarm fires for (metric, scope), the next HysteresisTicks
// checks are suppressed. The effective rate in steady state is:
//
//	lambda_effective = lambda_raw / (1 + lambda_raw × HysteresisTicks)
//
// where lambda_raw = ticks/week × P(|Z| >= sigma) per metric.
func TestAnomaly_FalseAlarmRate_ModeledTarget(t *testing.T) {
	// Parameters.
	sigma := anomaly.DefaultSigma              // 4.0
	minSamples := anomaly.MinSamples           // 30
	hysteresisTicks := anomaly.HysteresisTicks // 10
	tickIntervalS := 60                        // seconds

	// Observations per node per week at 60 s tick.
	secondsPerWeek := 7 * 24 * 3600
	ticksPerWeek := secondsPerWeek / tickIntervalS // 10,080
	// metricsPerNode is a CONSERVATIVE per-node metric budget (D-074 WO-D, updated D-087).
	// Truly node-scoped metrics are cpu_pct, mem_pct, disk_pct, ams_api_latency_ms (4);
	// viewers is stream-scoped (UpdateBaselines iterates snap.Streams) but is counted
	// here as a 5th as-if-node-scoped metric so the modeled rate stays an upper bound
	// for the common 1-node/1-stream deployment. ingest_bitrate_kbps is likewise
	// stream-scoped and excluded — it scales with stream count, not node count.
	// True node-only rate at 4 metrics ≈ 0.3442/node-week; this 5-metric bound
	// ≈ 0.4322/node-week — both < the PRD 1.0 target. D-087 adds ams_api_latency_ms
	// as the 4th truly-node-scoped metric (0.08644×5 = 0.4322 < 1.0 PASS).
	metricsPerNode := 5

	// Gaussian tail probability for |Z| >= sigma (two-tailed).
	// P(|Z| >= 4.0) ≈ 6.33e-5 (standard normal, two-tailed).
	// Source: Abramowitz & Stegun 26.2.17 approximation.
	tailProb := 6.33e-5

	// Raw exceedance rate per metric per node per week (before hysteresis).
	lambdaRaw := float64(ticksPerWeek) * tailProb // ≈ 0.638/week

	// Effective rate with renewal-process hysteresis model:
	//   lambda_eff = lambda_raw / (1 + lambda_raw × cooldown_ticks)
	lambdaEff := lambdaRaw / (1.0 + lambdaRaw*float64(hysteresisTicks))

	// Total across all tracked metrics.
	totalPerNodeWeek := lambdaEff * float64(metricsPerNode)

	t.Logf("Anomaly sensitivity calibration (renewal-process model):")
	t.Logf("  sigma=%.1f, minSamples=%d, hysteresisTicks=%d, tickInterval=%ds",
		sigma, minSamples, hysteresisTicks, tickIntervalS)
	t.Logf("  ticks/week: %d | metrics/node: %d", ticksPerWeek, metricsPerNode)
	t.Logf("  tail P(|Z|>=%.1f) ≈ %.2e", sigma, tailProb)
	t.Logf("  lambda_raw per metric/week: %.4f", lambdaRaw)
	t.Logf("  lambda_effective per metric/week: %.4f (hysteresis applied)", lambdaEff)
	t.Logf("  modeled false alarms/node/week: %.4f (across %d metrics)",
		totalPerNodeWeek, metricsPerNode)

	target := 1.0 // <1 false alarm/node/week per PRD F9
	if totalPerNodeWeek >= target {
		t.Errorf("modeled false-alarm rate %.4f/node-week exceeds PRD target <%.1f",
			totalPerNodeWeek, target)
	}
	t.Logf("PASS: modeled false-alarm rate %.4f/node-week < PRD target %.1f/node-week",
		totalPerNodeWeek, target)
}
