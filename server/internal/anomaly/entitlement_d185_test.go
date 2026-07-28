package anomaly_test

// entitlement_d185_test.go — D-185: runtime tier gate on the anomaly baseline loop.
//
// D-108 established the rule for the prober and round 10's M-03 applied it to
// alert channels: an HTTP CRUD gate does not reach a background goroutine, so a
// tenant that downgrades keeps having the paid work done for them until the
// process restarts. Round 11 then asserted the class was "now uniform across
// prober / reports / alerts" — true of the three it enumerated, false of the set.
// The anomaly detector was the fourth such loop and the only one still ungated:
// its API is gated (wave3.go CheckAnomalies) but Run() ticked regardless,
// recomputing and WRITING baselines for a tier with no right to them.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/anomaly"
)

// countingStore records how many baseline writes actually happen.
type countingStore struct {
	mu     sync.Mutex
	writes int
	rows   []anomaly.AnomalyBaselineRow
}

func (c *countingStore) ListAnomalyBaselines(_ context.Context) ([]anomaly.AnomalyBaselineRow, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]anomaly.AnomalyBaselineRow, len(c.rows))
	copy(out, c.rows)
	return out, nil
}

func (c *countingStore) UpsertAnomalyBaseline(_ context.Context, row anomaly.AnomalyBaselineRow) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	c.rows = append(c.rows, row)
	return nil
}

func (c *countingStore) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

// runTicks starts the detector and lets it tick a few times.
func runTicks(t *testing.T, gate func() error, store *countingStore) {
	t.Helper()
	det := anomaly.New(anomaly.Config{
		TickInterval:    10 * time.Millisecond,
		EntitlementGate: gate,
	}, store, &fakeStreamLive{viewerCount: 42}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		det.Run(ctx)
		close(done)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("detector did not stop on context cancel")
	}
}

// TestRun_DowngradedTier_WritesNoBaselines is the fix: a gate that refuses must
// stop the background work, not merely the API that reads its output.
func TestRun_DowngradedTier_WritesNoBaselines(t *testing.T) {
	store := &countingStore{}
	runTicks(t, func() error { return errors.New("anomaly detection requires the Enterprise tier") }, store)

	if n := store.writeCount(); n != 0 {
		t.Fatalf("downgraded tier still had %d baselines computed and written for it", n)
	}
}

// TestRun_EntitledTier_StillWritesBaselines is the negative control. A gate that
// silences the loop for an ENTITLED tenant would be a worse defect than the one
// being fixed — the product would quietly stop detecting anomalies.
func TestRun_EntitledTier_StillWritesBaselines(t *testing.T) {
	store := &countingStore{}
	runTicks(t, func() error { return nil }, store)

	if n := store.writeCount(); n == 0 {
		t.Fatal("entitled tier wrote no baselines — the gate is refusing when it should allow")
	}
}

// TestRun_NilGate_IsBackwardCompatible pins the nil default: tests and any
// deployment that does not wire a licence must behave exactly as before.
func TestRun_NilGate_IsBackwardCompatible(t *testing.T) {
	store := &countingStore{}
	runTicks(t, nil, store)

	if n := store.writeCount(); n == 0 {
		t.Fatal("nil gate must allow — a nil licence hook cannot be allowed to disable the feature")
	}
}
