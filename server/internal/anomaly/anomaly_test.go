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
	metricsPerNode := 3

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
