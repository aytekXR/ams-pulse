package anomaly_test

// s70_d132_flag_test.go — black-box integration test for D-132 [16] through the
// public API. Reuses the fakes in anomaly_flagstore_test.go (fakeBaselineStore,
// anomalyLiveForFlagTest, fakeFlagStore, warmupLive).

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
)

// TestFlagStore_ComputeFlagsDoesNotSuppressTickPersistence reproduces the [16]
// race end-to-end: an HTTP GET /anomalies poll (ComputeFlags) lands while an
// anomaly is active, then the next UpdateBaselines tick runs. The tick path is
// the sole writer of the persistent flag-event audit trail, so it MUST still
// persist exactly one event — ComputeFlags must not have armed the shared
// cooldown. Pre-fix, ComputeFlags armed it and the tick skipped the write,
// silently dropping the anomaly from the ClickHouse audit trail.
func TestFlagStore_ComputeFlagsDoesNotSuppressTickPersistence(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}
	flagStore := &fakeFlagStore{}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()
	// 60 alternating warmup ticks: z_limit ≈ 60/sqrt(61) ≈ 7.7 >> 3.0.
	warmupLive(t, det, live, 60, 95, 105)

	baselines, _ := store.ListAnomalyBaselines(ctx)
	var vb *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "viewers" {
			vb = &baselines[i]
			break
		}
	}
	if vb == nil || vb.Stddev <= 0 {
		// 60 alternating warmup ticks reliably build a viewers baseline with stddev>0;
		// its absence means the setup is broken and the [16] property went unverified —
		// fail loudly rather than skip (D-086 verify-catch pattern).
		t.Fatalf("viewers baseline not built after warmup (vb=%v) — setup broken, [16] unverified", vb)
	}

	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)

	// HTTP read path lands first — reports the anomaly but must not arm cooldown.
	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) == 0 {
		t.Fatal("expected ComputeFlags to report the active anomaly")
	}
	if got := len(flagStore.capturedEvents()); got != 0 {
		t.Fatalf("ComputeFlags persisted %d event(s) — it must never write", got)
	}

	// Next tick must still persist the flag event.
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines after ComputeFlags: %v", err)
	}

	var got int
	for _, ev := range flagStore.capturedEvents() {
		if ev.Metric == "viewers" {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("expected exactly 1 persisted viewers flag event after ComputeFlags+tick, got %d "+
			"(ComputeFlags armed the cooldown → tick skipped the audit-trail write)", got)
	}
	t.Logf("PASS: ComputeFlags reported %d flag(s), tick persisted exactly 1 event", len(flags))
}
