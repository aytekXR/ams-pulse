package anomaly_test

// anomaly_presence_flags_test.go — D-088 TDD: presence-flag guards for cpu/mem/disk.
//
// Covers:
//   - UpdateBaselines: node with CPUPCTReported=false (standalone path) → NO baseline
//     rows for cpu_pct, mem_pct, disk_pct.
//   - Anti-heuristic pin (cluster-zero): node with CPUPCTReported=true, CPUPCT=0 →
//     baseline IS observed (a value>0 guard would fail this; flag-based guard passes).
//   - ComputeFlags: node with CPUPCTReported=false → cpu_pct key absent from
//     liveValues even when a stale zero-mean baseline is seeded (prevents false flag).
//   - Run() wiring: sweep (DeleteZeroMeanNodeBaselines) is called on detector startup
//     when the store implements BaselineSweeper.
//
// RED:
//   - Compile errors for all tests that reference CPUPCTReported/MemPCTReported/
//     DiskPCTReported before domain.LiveNodeStats gains those fields.
//   - Behavioral failures for sweep test before Run() gains the sweep call.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

// nodeFlagLive provides a live snapshot with a single node whose presence flags
// and metric values are configurable. Used to test the cpu/mem/disk presence guards.
type nodeFlagLive struct {
	mu   sync.Mutex
	node *domain.LiveNodeStats
}

func newNodeFlagLive(n *domain.LiveNodeStats) *nodeFlagLive {
	return &nodeFlagLive{node: n}
}

func (f *nodeFlagLive) CurrentSnapshot() *domain.LiveSnapshot {
	f.mu.Lock()
	n := f.node
	f.mu.Unlock()
	if n == nil {
		return nil
	}
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes: map[string]*domain.LiveNodeStats{
			n.NodeID: n,
		},
	}
}

func (f *nodeFlagLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// sweepSpyStore extends fakeBaselineStore (anomaly_test.go) with a spy
// implementation of DeleteZeroMeanNodeBaselines. The detector's Run() will
// type-assert the store to anomaly.BaselineSweeper; if the assertion succeeds
// the method is called and called is set to true.
type sweepSpyStore struct {
	fakeBaselineStore
	mu      sync.Mutex
	called  bool
	metrics []string
}

func (s *sweepSpyStore) DeleteZeroMeanNodeBaselines(_ context.Context, metrics []string) (int64, error) {
	s.mu.Lock()
	s.called = true
	s.metrics = append(s.metrics, metrics...)
	s.mu.Unlock()
	return 0, nil
}

func (s *sweepSpyStore) wasCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

// nilLive returns a nil snapshot (UpdateBaselines is a no-op for nil snapshots).
// Used by the Run() sweep pin where we only want to verify the sweep call.
type nilLive struct{}

func (n *nilLive) CurrentSnapshot() *domain.LiveSnapshot { return nil }
func (n *nilLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestUpdateBaselines_SkipsUnreportedCPUMemDisk verifies that UpdateBaselines
// does NOT create baseline rows for cpu_pct, mem_pct, or disk_pct when the node
// has CPUPCTReported=false (the standalone AMS 3.x path).
//
// Feeding zero-value observations would poison the Welford mean toward zero and
// make any real reading look like an anomaly after the node transitions to cluster mode.
//
// RED: compile error (CPUPCTReported field missing) until domain.LiveNodeStats
// gains the presence fields; then behavioral failure until UpdateBaselines adds the guards.
func TestUpdateBaselines_SkipsUnreportedCPUMemDisk(t *testing.T) {
	store := &fakeBaselineStore{}
	// Standalone node: all flags false (zero value), values also zero.
	live := newNodeFlagLive(&domain.LiveNodeStats{
		NodeID:          "standalone-1",
		CPUPCT:          0.0,
		MemPCT:          0.0,
		DiskPCT:         0.0,
		CPUPCTReported:  false,
		MemPCTReported:  false,
		DiskPCTReported: false,
	})

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    4.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

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
		switch b.Metric {
		case "cpu_pct":
			t.Errorf("expected NO cpu_pct baseline when CPUPCTReported=false, "+
				"got one: mean=%.4f samples=%d scope=%q — "+
				"UpdateBaselines must guard on CPUPCTReported", b.Mean, b.SampleCount, b.Scope)
		case "mem_pct":
			t.Errorf("expected NO mem_pct baseline when MemPCTReported=false, "+
				"got one: mean=%.4f samples=%d scope=%q", b.Mean, b.SampleCount, b.Scope)
		case "disk_pct":
			t.Errorf("expected NO disk_pct baseline when DiskPCTReported=false, "+
				"got one: mean=%.4f samples=%d scope=%q", b.Mean, b.SampleCount, b.Scope)
		}
	}
	t.Logf("PASS: standalone node (all flags false) contributes no cpu/mem/disk baselines")
}

// TestUpdateBaselines_ClusterZero_BaselineObserved is the anti-heuristic pin:
// a cluster node with CPUPCTReported=true but CPUPCT=0.0 MUST produce a baseline.
// disk_pct=0 is a valid cluster reading (discovery.go:191-195 emits all three keys
// unconditionally). A value>0 guard would silently discard these observations.
//
// RED: compile error (CPUPCTReported field missing) before implementation.
// GREEN: the flag-based guard passes (flag=true → observe, regardless of value).
func TestUpdateBaselines_ClusterZero_BaselineObserved(t *testing.T) {
	store := &fakeBaselineStore{}
	// Cluster node: flags true, but CPU/mem/disk happen to read 0.0 this tick.
	live := newNodeFlagLive(&domain.LiveNodeStats{
		NodeID:          "cluster-1",
		CPUPCT:          0.0,
		MemPCT:          0.0,
		DiskPCT:         0.0,
		CPUPCTReported:  true,
		MemPCTReported:  true,
		DiskPCTReported: true,
	})

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    4.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines iter %d: %v", i, err)
		}
	}

	baselines, err := store.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}

	want := map[string]bool{"cpu_pct": false, "mem_pct": false, "disk_pct": false}
	for _, b := range baselines {
		if _, ok := want[b.Metric]; ok {
			want[b.Metric] = true
			if b.SampleCount < 5 {
				t.Errorf("metric %q: expected SampleCount>=5, got %d", b.Metric, b.SampleCount)
			}
		}
	}
	for metric, found := range want {
		if !found {
			t.Errorf("metric %q: expected baseline to be observed when flag=true even if value=0 "+
				"(a value>0 guard is wrong — disk_pct=0 is valid on cluster nodes)", metric)
		}
	}
	t.Logf("PASS: cluster node with flags=true, values=0 produces baselines (anti-heuristic pin)")
}

// TestComputeFlags_Unreported_KeyAbsent verifies that ComputeFlags does NOT add
// cpu_pct to liveValues when CPUPCTReported=false, even when a stale zero-mean
// baseline exists with enough samples to trigger detection.
//
// Without the guard, the current code unconditionally adds n.CPUPCT (=0) to
// liveValues. With a baseline of mean=80/stddev=1, z=|0-80|/4=20 >> sigma=4 →
// false alarm. The guard makes the key absent → detectFlagsLocked skips it.
//
// RED: compile error (CPUPCTReported field) before implementation; then
// behavioral failure (false flag fires) after field added but before guard.
func TestComputeFlags_Unreported_KeyAbsent(t *testing.T) {
	store := &fakeBaselineStore{}
	// Seed a stale zero-mean cpu_pct baseline — would produce a false alarm if
	// CPUPCT=0 (unreported) is used as the observed value.
	nodeScope := `{"node_id":"unreported-1"}`
	store.rows = append(store.rows, anomaly.AnomalyBaselineRow{
		ID:          "test-baseline-1",
		Metric:      "cpu_pct",
		Scope:       nodeScope,
		WindowS:     3600,
		Mean:        80.0,
		Stddev:      1.0,
		SampleCount: 100, // >= MinSamples
		LastUpdated: time.Now().UnixMilli(),
	})

	// Node with CPUPCTReported=false (standalone path) and CPUPCT=0.
	live := newNodeFlagLive(&domain.LiveNodeStats{
		NodeID:         "unreported-1",
		CPUPCT:         0.0,
		CPUPCTReported: false, // key was absent from the event
	})

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    4.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()

	flags, err := det.ComputeFlags(ctx, 4.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}

	for _, f := range flags {
		if f.Metric == "cpu_pct" {
			t.Errorf("ComputeFlags: got cpu_pct flag (sigma=%.2f) for node with "+
				"CPUPCTReported=false — liveValues must NOT include cpu_pct when "+
				"the key was absent from the event (prevents false alarm against "+
				"stale zero-mean baseline)", f.Sigma)
		}
	}
	t.Logf("PASS: ComputeFlags does not flag cpu_pct when CPUPCTReported=false")
}

// TestDetector_Run_SweepCalled verifies that Detector.Run calls
// DeleteZeroMeanNodeBaselines on the store immediately after WarmHysteresis,
// before the first tick. The sweep evicts zero-mean baselines that were accumulated
// by standalone nodes before D-088 presence flags were deployed.
//
// RED: sweep is never called because Run() has no sweep code (called=false after Run).
// GREEN: Run() type-asserts store to BaselineSweeper and calls DeleteZeroMeanNodeBaselines.
func TestDetector_Run_SweepCalled(t *testing.T) {
	spy := &sweepSpyStore{}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    4.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Hour, // long interval so tick never fires during test
	}, spy, &nilLive{}, nil)

	// Cancel the context immediately so Run() exits after startup (WarmHysteresis
	// + sweep) but before the first tick.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	det.Run(ctx) // returns promptly because ctx is already done

	if !spy.wasCalled() {
		t.Error("Detector.Run must call DeleteZeroMeanNodeBaselines on detector startup " +
			"(store implements BaselineSweeper) — sweep was not called")
	}
	if spy.wasCalled() {
		// Verify the right metrics were swept.
		spy.mu.Lock()
		got := spy.metrics
		spy.mu.Unlock()

		wantSet := map[string]bool{"cpu_pct": false, "mem_pct": false, "disk_pct": false}
		for _, m := range got {
			wantSet[m] = true
		}
		for metric, seen := range wantSet {
			if !seen {
				t.Errorf("sweep metrics: expected %q to be in the sweep list, got: %v", metric, got)
			}
		}
	}
	t.Logf("PASS: Run() calls sweep on startup; metrics swept: %v", spy.metrics)
}
