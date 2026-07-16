// S47 (D-109) — anomaly sigma-boundary consistency.
//
// The detect path (anomaly.go: `if z < sigmaThreshold { continue }`) flags a
// z-score exactly AT the threshold (inclusive, i.e. z >= sigma). The eval/fire
// path in wave3.go used a strict `z > effectiveSigma`, so a z landing exactly on
// the threshold fired on detect but was silently suppressed on eval — two answers
// for one input. wave3.go now uses `>=` to match detect.
//
// This test drives the eval path (TickOnce) with a z engineered to equal sigma
// exactly and asserts it fires. Mutation: revert wave3.go to `z > effectiveSigma`
// → z == sigma no longer fires → this test goes RED. It sits exactly between the
// existing FiresAboveSigma (z >> sigma) and NoFireBelowSigma (z < sigma) tests.
package alert_test

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
)

func TestEvalAnomalyMetric_FiresAtExactSigmaBoundary(t *testing.T) {
	store := openTestStore(t)
	live := newFakeLive()
	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	// mean=10, stddev=1 → effStddev = max(1.0, max(0.05*10=0.5, 1e-9)) = 1.0.
	// observed=15 → z = (15-10)/1.0 = 5.0, which is EXACTLY sigma=5.0.
	reader := &alert.FakeAnomalyBaselineReader{
		Row: streamBaseline("viewers", "s1", 10.0, 1.0, 30),
	}
	ev.SetAnomalyBaselineReader(reader)

	ctx := context.Background()
	makeAnomalyRule(ctx, t, store, "fires-at-boundary", "viewer_count", 5.0, 5)

	live.setSnap(snapWithStream("s1", 15))
	clock.Advance(3601 * time.Second)
	ev.TickOnce(ctx)

	hist, err := store.ListAlertHistory(ctx, "", "", 0, 0, 10, "")
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("z=5.0 exactly at sigma=5.0 must fire (>= boundary), got 0 history — eval path still uses strict >")
	}
	if hist[0].State != "firing" {
		t.Errorf("expected state=firing at the sigma boundary, got %q", hist[0].State)
	}
	if hist[0].Value != 15.0 {
		t.Errorf("expected observed Value=15, got %g", hist[0].Value)
	}
}
