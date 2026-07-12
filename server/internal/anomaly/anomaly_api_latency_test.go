package anomaly_test

// anomaly_api_latency_test.go — TDD tests for ams_api_latency_ms metric (D-087).
//
// Covers:
//   - UpdateBaselines obs loop: baseline forms for nodes with APILatencyMS > 0.
//   - ComputeFlags liveValues map: ComputeFlags returns a flag for a latency spike.
//   - Presence guard: a node with APILatencyMS==0 (no measurement) must not
//     contribute a baseline row — feeding 0 would poison the Welford toward zero
//     and make normal latency look anomalous.
//   - Spike persisted: 30 warmup ticks at ~20ms, then 500ms injection →
//     AnomalyFlagEvent with Metric=="ams_api_latency_ms" written to fakeFlagStore.
//
// RED: all tests below fail before anomaly.go adds ams_api_latency_ms to
//
//	UpdateBaselines and ComputeFlags.
//
// GREEN: pass once those paths are implemented (D-087 A2 deliverable 1a+1b).

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

// fakeNodeLatencyLive provides a live snapshot with a single node whose
// APILatencyMS is configurable. Used to test the ams_api_latency_ms metric.
type fakeNodeLatencyLive struct {
	nodeID       string
	apiLatencyMS float64
}

func (f *fakeNodeLatencyLive) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			f.nodeID: {
				NodeID:       f.nodeID,
				CPUPCT:       10.0,
				MemPCT:       20.0,
				APILatencyMS: f.apiLatencyMS,
			},
		},
	}
}

func (f *fakeNodeLatencyLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestAnomaly_APILatencyMS_Baselines verifies that UpdateBaselines writes a
// baseline row keyed by metric="ams_api_latency_ms" with the correct node scope.
//
// RED: fails before anomaly.go UpdateBaselines observes APILatencyMS.
// GREEN: passes once the node-section obs loop is extended (D-087 deliverable 1a).
func TestAnomaly_APILatencyMS_Baselines(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeNodeLatencyLive{nodeID: "node1", apiLatencyMS: 20.5}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed 5 observations with APILatencyMS set.
	for i := 0; i < 5; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	// Assert: baseline row with metric="ams_api_latency_ms" must now exist.
	baselines, err := store.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}

	var found *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "ams_api_latency_ms" {
			found = &baselines[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected baseline row with metric=ams_api_latency_ms, got none " +
			"(UpdateBaselines must observe APILatencyMS when > 0)")
	}
	if found.SampleCount < 5 {
		t.Errorf("expected SampleCount>=5, got %d", found.SampleCount)
	}
	// Scope must be node-scoped: {"node_id":"node1"}.
	wantScope := `{"node_id":"node1"}`
	if found.Scope != wantScope {
		t.Errorf("expected scope=%q, got %q", wantScope, found.Scope)
	}
	// Mean should be ~20.5 (all observations identical).
	if found.Mean < 20.0 || found.Mean > 21.0 {
		t.Errorf("expected Mean ~20.5, got %.4f", found.Mean)
	}
	t.Logf("ams_api_latency_ms baseline: mean=%.4f stddev=%.4f samples=%d scope=%s",
		found.Mean, found.Stddev, found.SampleCount, found.Scope)
	t.Logf("PASS: ams_api_latency_ms baseline created with node scope")
}

// TestAnomaly_APILatencyMS_Spike_Flags verifies that a latency spike (30 warmup
// ticks at ~20ms, then 500ms injection) fires an AnomalyFlagEvent with
// Metric=="ams_api_latency_ms" and the correct node scope, persisted via
// the D-086 fakeFlagStore path (UpdateBaselines → checkFlags → InsertAnomalyFlagEvent).
//
// RED: fails before anomaly.go UpdateBaselines observes APILatencyMS.
// GREEN: passes once both the obs loop and checkFlags see the new metric (D-087 1a).
func TestAnomaly_APILatencyMS_Spike_Flags(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeNodeLatencyLive{nodeID: "node1", apiLatencyMS: 20.0}
	flagStore := &fakeFlagStore{}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()

	// Warmup: 30 ticks alternating 18 ms / 22 ms to build a non-zero stddev
	// baseline around ~20 ms. 30 warmup samples >> MinSamples=5 so flagging
	// is enabled, and the stddev is stable.
	for i := 0; i < 30; i++ {
		if i%2 == 0 {
			live.apiLatencyMS = 18.0
		} else {
			live.apiLatencyMS = 22.0
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("warmup UpdateBaselines tick %d: %v", i, err)
		}
	}

	// Confirm baseline was built.
	baselines, _ := store.ListAnomalyBaselines(ctx)
	var lb *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "ams_api_latency_ms" {
			lb = &baselines[i]
			break
		}
	}
	if lb == nil {
		t.Fatal("ams_api_latency_ms baseline not found after warmup; " +
			"UpdateBaselines must observe APILatencyMS (D-087 deliverable 1a)")
	}
	if lb.Stddev <= 0 {
		t.Fatalf("expected stddev > 0 after alternating warmup, got %.6f", lb.Stddev)
	}
	t.Logf("warmup baseline: mean=%.4f stddev=%.4f samples=%d",
		lb.Mean, lb.Stddev, lb.SampleCount)

	// Inject spike: 500 ms (far above ~20 ms mean with stddev ~2 ms).
	// Expected z-score ≈ (500 - 20) / 2 = 240 >> sigma=3.0 — must fire.
	live.apiLatencyMS = 500.0
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines (spike): %v", err)
	}

	events := flagStore.capturedEvents()
	var found *anomaly.AnomalyFlagEvent
	for i := range events {
		if events[i].Metric == "ams_api_latency_ms" {
			found = &events[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected AnomalyFlagEvent with Metric=ams_api_latency_ms after 500ms spike, "+
			"got 0 events (total events captured: %d)", len(events))
	}

	wantScope := `{"node_id":"node1"}`
	if found.Scope != wantScope {
		t.Errorf("Scope: got %q, want %q", found.Scope, wantScope)
	}
	if found.Sigma < 3.0 {
		t.Errorf("Sigma: got %.2f, expected >= 3.0", found.Sigma)
	}
	if found.Observed != 500.0 {
		t.Errorf("Observed: got %.1f, want 500.0", found.Observed)
	}
	t.Logf("PASS: ams_api_latency_ms spike flagged: sigma=%.2f observed=%.1f expected=%.2f scope=%q",
		found.Sigma, found.Observed, found.Expected, found.Scope)
}

// TestAnomaly_APILatencyMS_NoMeasurement_NoBaseline verifies that a node with
// APILatencyMS==0 (meaning the last stats call failed — key-absent semantics per
// D-075/D-087 contract) does NOT contribute an ams_api_latency_ms observation.
//
// Feeding 0 would poison the Welford baseline toward zero, making normal latency
// look anomalous. The presence guard (skip when APILatencyMS==0) prevents this.
//
// This test DOES discriminate the guard (proven by the S25 verify mutation
// M5): removing the presence guard feeds 0.0 observations, a zero-mean
// baseline row appears for ams_api_latency_ms, and the no-baseline assertion
// below fails with "expected NO baseline row ... found one: mean=0.0000".
func TestAnomaly_APILatencyMS_NoMeasurement_NoBaseline(t *testing.T) {
	store := &fakeBaselineStore{}
	// Node with APILatencyMS == 0: no measurement (last call failed or not yet polled).
	live := &fakeNodeLatencyLive{nodeID: "node1", apiLatencyMS: 0}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Feed 10 observations — all with APILatencyMS==0 (no measurement).
	for i := 0; i < 10; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	baselines, err := store.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}

	for _, b := range baselines {
		if b.Metric == "ams_api_latency_ms" {
			t.Errorf("expected NO baseline row for ams_api_latency_ms when APILatencyMS==0, "+
				"but found one: mean=%.4f samples=%d scope=%q — "+
				"presence guard must skip when APILatencyMS==0", b.Mean, b.SampleCount, b.Scope)
		}
	}
	t.Logf("PASS: node with APILatencyMS==0 contributes no ams_api_latency_ms baseline")
}

// TestAnomaly_APILatencyMS_ComputeFlags verifies that ComputeFlags (the HTTP
// on-demand path) also returns a flag for a latency spike on the ams_api_latency_ms
// metric when the baseline is warmed up and the observed value exceeds sigma.
//
// This exercises deliverable 1b: ComputeFlags liveValues map must include
// "ams_api_latency_ms:"+nodeScope with the same presence guard as UpdateBaselines.
//
// RED: fails before anomaly.go ComputeFlags liveValues includes ams_api_latency_ms.
// GREEN: passes once ComputeFlags is extended (D-087 deliverable 1b).
func TestAnomaly_APILatencyMS_ComputeFlags(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &fakeNodeLatencyLive{nodeID: "node2", apiLatencyMS: 20.0}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	// Warmup: alternating 18/22 ms to build stddev.
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			live.apiLatencyMS = 18.0
		} else {
			live.apiLatencyMS = 22.0
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("warmup UpdateBaselines tick %d: %v", i, err)
		}
	}

	// Confirm baseline.
	baselines, _ := store.ListAnomalyBaselines(ctx)
	var lb *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "ams_api_latency_ms" {
			lb = &baselines[i]
			break
		}
	}
	if lb == nil {
		t.Fatal("ams_api_latency_ms baseline not found after warmup; " +
			"UpdateBaselines must observe APILatencyMS (D-087 deliverable 1a)")
	}
	if lb.Stddev <= 0 {
		t.Fatalf("expected stddev > 0 after alternating warmup, got %.6f", lb.Stddev)
	}
	t.Logf("warmup baseline: mean=%.4f stddev=%.4f samples=%d",
		lb.Mean, lb.Stddev, lb.SampleCount)

	// Inject spike via ComputeFlags (HTTP path, no flagStore write).
	live.apiLatencyMS = 500.0
	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}

	var found *anomaly.AnomalyFlag
	for i := range flags {
		if flags[i].Metric == "ams_api_latency_ms" {
			found = &flags[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("ComputeFlags: expected AnomalyFlag with Metric=ams_api_latency_ms "+
			"after 500ms spike, got %d total flags — "+
			"ComputeFlags liveValues must include ams_api_latency_ms (D-087 deliverable 1b)",
			len(flags))
	}
	if found.Scope.NodeID != "node2" {
		t.Errorf("Scope.NodeID: got %q, want node2", found.Scope.NodeID)
	}
	if found.Sigma < 3.0 {
		t.Errorf("Sigma: got %.2f, expected >= 3.0", found.Sigma)
	}
	t.Logf("PASS: ComputeFlags returned ams_api_latency_ms flag: sigma=%.2f scope.NodeID=%s",
		found.Sigma, found.Scope.NodeID)
}
