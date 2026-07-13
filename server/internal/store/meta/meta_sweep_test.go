package meta_test

// meta_sweep_test.go — D-088 TDD: DeleteZeroMeanNodeBaselines zero-mean sweep.
//
// Covers:
//   - Exactly the zero-mean cpu_pct row is deleted when called with
//     metrics=["cpu_pct","mem_pct","disk_pct"]. Other rows (nonzero cpu_pct,
//     zero-mean viewers, real ams_api_latency_ms) are left intact.
//   - Idempotent: a second call deletes 0 rows (no rows remaining to delete).
//   - Empty metrics slice is a no-op (count=0, no error).
//
// RED: store.DeleteZeroMeanNodeBaselines undefined until meta/anomaly.go adds it.

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
)

// seedBaseline is a helper to insert an anomaly_baselines row for sweep tests.
func seedBaseline(t *testing.T, ctx context.Context, s interface {
	UpsertAnomalyBaseline(context.Context, anomaly.AnomalyBaselineRow) error
}, metric, scope string, mean, stddev float64, sampleCount int) {
	t.Helper()
	if err := s.UpsertAnomalyBaseline(ctx, anomaly.AnomalyBaselineRow{
		Metric:      metric,
		Scope:       scope,
		WindowS:     3600,
		Mean:        mean,
		Stddev:      stddev,
		SampleCount: sampleCount,
		LastUpdated: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("UpsertAnomalyBaseline(%s): %v", metric, err)
	}
}

// TestDeleteZeroMeanNodeBaselines_DeletesOnlyTargetRows verifies the selective delete:
//
//	Row 1: cpu_pct, mean=0, stddev=0, samples=500 (poisoned by standalone node) → DELETED
//	Row 2: cpu_pct, mean=50, stddev=5 (real cluster reading)                    → kept
//	Row 3: viewers, mean=0, stddev=0  (zero but metric not in list)              → kept
//	Row 4: ams_api_latency_ms, mean=20, stddev=2 (real, not in list)            → kept
//
// Expected: exactly 1 row deleted (count=1), others intact.
func TestDeleteZeroMeanNodeBaselines_DeletesOnlyTargetRows(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	nodeScope1 := `{"node_id":"standalone-poison"}`
	nodeScope2 := `{"node_id":"cluster-good"}`
	streamScope := `{"stream_id":"s1"}`
	nodeScope3 := `{"node_id":"cluster-latency"}`

	// Row 1: poisoned zero-mean cpu_pct (standalone node accumulated ~500 samples).
	seedBaseline(t, ctx, s, "cpu_pct", nodeScope1, 0.0, 0.0, 500)
	// Row 2: real nonzero cpu_pct row (cluster node, must be kept).
	seedBaseline(t, ctx, s, "cpu_pct", nodeScope2, 50.0, 5.0, 200)
	// Row 3: zero-mean viewers row (not in the sweep list — must be kept).
	seedBaseline(t, ctx, s, "viewers", streamScope, 0.0, 0.0, 300)
	// Row 4: real ams_api_latency_ms row (not in the sweep list — must be kept).
	seedBaseline(t, ctx, s, "ams_api_latency_ms", nodeScope3, 20.0, 2.0, 100)

	sweepMetrics := []string{"cpu_pct", "mem_pct", "disk_pct"}
	n, err := s.DeleteZeroMeanNodeBaselines(ctx, sweepMetrics)
	if err != nil {
		t.Fatalf("DeleteZeroMeanNodeBaselines: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteZeroMeanNodeBaselines: expected 1 row deleted, got %d "+
			"(only the poisoned cpu_pct row with mean=0,stddev=0 must be deleted)", n)
	}

	// Verify rows 2,3,4 are intact.
	remaining, err := s.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines after sweep: %v", err)
	}
	if len(remaining) != 3 {
		t.Errorf("expected 3 rows after sweep, got %d: %+v", len(remaining), remaining)
	}

	for _, r := range remaining {
		if r.Metric == "cpu_pct" && r.Scope == nodeScope1 {
			t.Errorf("poisoned cpu_pct row (mean=0,stddev=0) was NOT deleted — " +
				"DeleteZeroMeanNodeBaselines must delete rows where metric IN list AND mean=0 AND stddev=0")
		}
	}

	// Verify the nonzero cpu_pct row survived.
	foundGoodCPU := false
	for _, r := range remaining {
		if r.Metric == "cpu_pct" && r.Scope == nodeScope2 {
			foundGoodCPU = true
		}
	}
	if !foundGoodCPU {
		t.Error("the nonzero cpu_pct row (mean=50) must NOT be deleted — sweep must only delete mean=0 AND stddev=0")
	}

	t.Logf("PASS: DeleteZeroMeanNodeBaselines deleted 1 poisoned row, kept 3 others")
}

// TestDeleteZeroMeanNodeBaselines_Idempotent verifies that calling the sweep a
// second time after the first successfully deleted the target rows returns 0.
func TestDeleteZeroMeanNodeBaselines_Idempotent(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	nodeScope := `{"node_id":"idempotent-node"}`
	seedBaseline(t, ctx, s, "cpu_pct", nodeScope, 0.0, 0.0, 800)

	sweepMetrics := []string{"cpu_pct", "mem_pct", "disk_pct"}

	n1, err := s.DeleteZeroMeanNodeBaselines(ctx, sweepMetrics)
	if err != nil {
		t.Fatalf("first DeleteZeroMeanNodeBaselines: %v", err)
	}
	if n1 != 1 {
		t.Fatalf("first sweep: expected 1 deleted, got %d", n1)
	}

	n2, err := s.DeleteZeroMeanNodeBaselines(ctx, sweepMetrics)
	if err != nil {
		t.Fatalf("second DeleteZeroMeanNodeBaselines: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second sweep (idempotent): expected 0 deleted, got %d", n2)
	}
	t.Logf("PASS: second sweep deletes 0 rows (idempotent)")
}

// TestDeleteZeroMeanNodeBaselines_EmptyMetrics verifies that an empty metrics
// slice is a safe no-op (no error, no deletions).
func TestDeleteZeroMeanNodeBaselines_EmptyMetrics(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Seed a row to confirm it is not affected.
	seedBaseline(t, ctx, s, "cpu_pct", `{"node_id":"safe"}`, 0.0, 0.0, 10)

	n, err := s.DeleteZeroMeanNodeBaselines(ctx, nil)
	if err != nil {
		t.Fatalf("DeleteZeroMeanNodeBaselines(nil): %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted for nil metrics, got %d", n)
	}

	n2, err := s.DeleteZeroMeanNodeBaselines(ctx, []string{})
	if err != nil {
		t.Fatalf("DeleteZeroMeanNodeBaselines([]): %v", err)
	}
	if n2 != 0 {
		t.Errorf("expected 0 deleted for empty metrics slice, got %d", n2)
	}

	// Original row must still be there.
	rows, _ := s.ListAnomalyBaselines(ctx)
	if len(rows) != 1 {
		t.Errorf("expected 1 row after no-op sweep, got %d", len(rows))
	}
	t.Logf("PASS: empty metrics slice is a safe no-op")
}

// TestDeleteZeroMeanNodeBaselines_HighSampleCount verifies that the sweep does
// NOT filter on sample_count — poisoned rows commonly have n=500..8000 samples
// because standalone nodes reported zeros for hundreds of ticks before detection.
func TestDeleteZeroMeanNodeBaselines_HighSampleCount(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	nodeScope := `{"node_id":"high-n-poison"}`
	// High sample count (live system might have n=8813) — must still be deleted.
	seedBaseline(t, ctx, s, "cpu_pct", nodeScope, 0.0, 0.0, 8813)

	n, err := s.DeleteZeroMeanNodeBaselines(ctx, []string{"cpu_pct", "mem_pct", "disk_pct"})
	if err != nil {
		t.Fatalf("DeleteZeroMeanNodeBaselines: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted even with high sample_count=8813, got %d "+
			"(the NO-sample_count-clause requirement — poisoned rows have many samples)", n)
	}
	t.Logf("PASS: high-sample-count poisoned row deleted (no sample_count clause)")
}
