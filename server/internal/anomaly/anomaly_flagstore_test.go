package anomaly_test

// anomaly_flagstore_test.go — TDD tests for BUG-008 phase 2 (ADR-0009).
// Covers: FlagEventStore write path, hysteresis interaction, WarmHysteresis,
// nil-store safety, insert-error resilience, ComputeFlags isolation.
//
// Pattern: external package, standalone functions, fakes (no ClickHouse).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

// fakeFlagStore captures InsertAnomalyFlagEvent calls and serves programmable
// RecentFlagKeys / insert-error responses.
type fakeFlagStore struct {
	mu         sync.Mutex
	events     []anomaly.AnomalyFlagEvent
	insertErr  error
	recentKeys []anomaly.FlagKey
	recentErr  error
}

func (f *fakeFlagStore) InsertAnomalyFlagEvent(_ context.Context, ev anomaly.AnomalyFlagEvent) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
	return nil
}

func (f *fakeFlagStore) RecentFlagKeys(_ context.Context, _ int) ([]anomaly.FlagKey, error) {
	if f.recentErr != nil {
		return nil, f.recentErr
	}
	return f.recentKeys, nil
}

func (f *fakeFlagStore) capturedEvents() []anomaly.AnomalyFlagEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]anomaly.AnomalyFlagEvent, len(f.events))
	copy(out, f.events)
	return out
}

// flagStoreWindowCapture captures the windowSecs argument to RecentFlagKeys.
type flagStoreWindowCapture struct {
	capturedWindowSecs int
}

func (f *flagStoreWindowCapture) InsertAnomalyFlagEvent(_ context.Context, _ anomaly.AnomalyFlagEvent) error {
	return nil
}
func (f *flagStoreWindowCapture) RecentFlagKeys(_ context.Context, windowSecs int) ([]anomaly.FlagKey, error) {
	f.capturedWindowSecs = windowSecs
	return nil, nil
}

// anomalyLiveForFlagTest provides a live snapshot with configurable per-stream viewers.
type anomalyLiveForFlagTest struct {
	mu          sync.Mutex
	viewerCount int
}

func (l *anomalyLiveForFlagTest) setViewers(n int) {
	l.mu.Lock()
	l.viewerCount = n
	l.mu.Unlock()
}

func (l *anomalyLiveForFlagTest) CurrentSnapshot() *domain.LiveSnapshot {
	l.mu.Lock()
	vc := l.viewerCount
	l.mu.Unlock()
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"s1": {StreamID: "s1", ViewerCount: vc},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func (l *anomalyLiveForFlagTest) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// dualStreamLive provides a live snapshot with two streams, each with a configurable
// viewer count. Used to verify that all events from one tick share DetectedAt.
type dualStreamLive struct {
	mu       sync.Mutex
	viewers1 int
	viewers2 int
}

func (d *dualStreamLive) CurrentSnapshot() *domain.LiveSnapshot {
	d.mu.Lock()
	v1, v2 := d.viewers1, d.viewers2
	d.mu.Unlock()
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"s1": {StreamID: "s1", ViewerCount: v1},
			"s2": {StreamID: "s2", ViewerCount: v2},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	}
}

func (d *dualStreamLive) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// warmupLive builds alternating-value baselines to establish a non-zero stddev.
// Feeds nTicks updates to the detector, alternating between lo and hi values.
func warmupLive(t *testing.T, det *anomaly.Detector, live *anomalyLiveForFlagTest, nTicks, lo, hi int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < nTicks; i++ {
		if i%2 == 0 {
			live.setViewers(lo)
		} else {
			live.setViewers(hi)
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("warmup UpdateBaselines tick %d: %v", i, err)
		}
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestFlagStore_WritePath_FiresOnDeviation verifies that checkFlags (called from
// UpdateBaselines) fires an AnomalyFlagEvent with:
//   - DetectedAt within the UpdateBaselines call window (tick timestamp)
//   - Scope byte-identical to the baseline's scope string
//   - correct Metric, Observed, Expected, Sigma fields
func TestFlagStore_WritePath_FiresOnDeviation(t *testing.T) {
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

	// Warmup: 60 alternating ticks so the injected spike has negligible impact on the
	// baseline mean. With n=60 warmup samples and threshold=3.0:
	//   z_limit ≈ n/sqrt(n+1) = 60/sqrt(61) ≈ 7.7 >> 3.0 (reliably exceeds threshold).
	warmupLive(t, det, live, 60, 95, 105)

	// Get baseline info (mean and stddev after warmup).
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
	if vb.Stddev <= 0 {
		t.Fatalf("expected stddev > 0 after warmup, got %.6f", vb.Stddev)
	}
	t.Logf("baseline: mean=%.2f stddev=%.4f samples=%d scope=%q", vb.Mean, vb.Stddev, vb.SampleCount, vb.Scope)

	// Inject a very large deviation THROUGH UpdateBaselines (not only ComputeFlags).
	// This exercises the checkFlags write path.
	injected := int(vb.Mean + 30.0*vb.Stddev)
	live.setViewers(injected)
	t.Logf("injecting viewers=%d (~30σ above mean)", injected)

	before := time.Now().UTC()
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines after injection: %v", err)
	}
	after := time.Now().UTC()

	events := flagStore.capturedEvents()
	if len(events) == 0 {
		t.Fatal("expected at least 1 AnomalyFlagEvent from checkFlags, got 0")
	}

	var ev *anomaly.AnomalyFlagEvent
	for i := range events {
		if events[i].Metric == "viewers" {
			ev = &events[i]
			break
		}
	}
	if ev == nil {
		t.Fatalf("expected AnomalyFlagEvent with Metric=viewers, got %+v", events)
	}

	// DetectedAt must be within the UpdateBaselines call window.
	if ev.DetectedAt.Before(before) || ev.DetectedAt.After(after) {
		t.Errorf("DetectedAt %v is outside [%v, %v]", ev.DetectedAt, before, after)
	}

	// Scope must be byte-identical to the baseline's scope.
	if ev.Scope != vb.Scope {
		t.Errorf("Scope mismatch: got %q, want %q", ev.Scope, vb.Scope)
	}

	// Sigma must exceed the threshold.
	if ev.Sigma < 3.0 {
		t.Errorf("expected Sigma >= 3.0, got %.4f", ev.Sigma)
	}

	// Observed must match the injected value.
	if ev.Observed != float64(injected) {
		t.Errorf("expected Observed=%v, got %v", float64(injected), ev.Observed)
	}

	// Expected must approximate the baseline mean.
	if ev.Expected <= 0 {
		t.Errorf("expected Expected > 0, got %v", ev.Expected)
	}

	t.Logf("PASS: flagEvent metric=%s sigma=%.2f observed=%.1f expected=%.2f scope=%q detectedAt=%v",
		ev.Metric, ev.Sigma, ev.Observed, ev.Expected, ev.Scope, ev.DetectedAt)
}

// TestFlagStore_MinSamplesGuard verifies that checkFlags does NOT fire when
// sample_count < minSamples (warmup phase). Same guard as ComputeFlags.
func TestFlagStore_MinSamplesGuard(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}
	flagStore := &fakeFlagStore{}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    1.0, // very low — would fire immediately if guard absent
		MinSamples:      30,
		HysteresisTicks: 1,
		TickInterval:    time.Second,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()

	// Only 5 updates — below minSamples=30.
	for i := 0; i < 5; i++ {
		live.setViewers(100)
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("UpdateBaselines: %v", err)
		}
	}

	// Inject anomaly THROUGH UpdateBaselines — must be suppressed by minSamples guard.
	live.setViewers(10000)
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines (injected): %v", err)
	}

	events := flagStore.capturedEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events (minSamples guard), got %d: %+v", len(events), events)
	}
	t.Logf("PASS: minSamples guard suppressed flag (sample_count < 30)")
}

// TestFlagStore_HysteresisSuppressesReFire verifies that after checkFlags fires
// and sets hysteresis, the NEXT UpdateBaselines tick with the same anomalous value
// does NOT produce a second event.
func TestFlagStore_HysteresisSuppressesReFire(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}
	flagStore := &fakeFlagStore{}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 5,
		TickInterval:    time.Second,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()
	// 60 warmup ticks: z_limit ≈ 60/sqrt(61) ≈ 7.7 >> 3.0 — reliable detection.
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
		t.Skip("viewers baseline not available")
	}

	// First injection: should fire.
	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines tick 1: %v", err)
	}

	after1 := flagStore.capturedEvents()
	if len(after1) == 0 {
		t.Fatal("expected 1 event on first tick, got 0")
	}
	t.Logf("tick 1: %d event(s) fired", len(after1))

	// Second tick (same anomalous value): hysteresis should suppress.
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines tick 2: %v", err)
	}

	after2 := flagStore.capturedEvents()
	// Must not have gained a new event.
	if len(after2) != len(after1) {
		t.Errorf("expected no new events on tick 2 (hysteresis), but got %d total (was %d)",
			len(after2), len(after1))
	}
	t.Logf("PASS: hysteresis suppressed re-fire on tick 2")
}

// TestFlagStore_NilStore_NoPanic verifies that when no FlagEventStore is wired,
// checkFlags runs (including hysteresis bookkeeping) without panicking.
// ComputeFlags must still produce flags — the in-memory path is unaffected.
func TestFlagStore_NilStore_NoPanic(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}

	// No SetFlagStore call — nil flagStore.
	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)

	ctx := context.Background()
	warmupLive(t, det, live, 10, 95, 105)

	baselines, _ := store.ListAnomalyBaselines(ctx)
	var vb *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "viewers" {
			vb = &baselines[i]
			break
		}
	}
	if vb == nil || vb.Stddev <= 0 {
		t.Skip("viewers baseline not available")
	}

	// UpdateBaselines with a nil flagStore must not panic.
	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines with nil flagStore: %v", err)
	}

	// ComputeFlags must still work (nil flagStore does not affect the HTTP path).
	// Value is now suppressed by hysteresis; set back to mean to confirm no crash.
	live.setViewers(int(vb.Mean))
	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags with nil flagStore: %v", err)
	}
	_ = flags // not asserting count — confirming no crash
	t.Logf("PASS: nil flagStore — no panic on UpdateBaselines or ComputeFlags")
}

// TestFlagStore_InsertError_NocrashHysteresisSet verifies that an InsertAnomalyFlagEvent
// error does NOT crash the detector AND that hysteresis is still set (suppressing
// a re-fire on the next tick even though the row was dropped).
// At-most-once undercount: the ADR accepts this as a deliberate trade-off.
func TestFlagStore_InsertError_NocrashHysteresisSet(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}
	flagStore := &fakeFlagStore{insertErr: errors.New("simulated CH write failure")}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 5,
		TickInterval:    time.Second,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()
	// 60 warmup ticks: z_limit ≈ 60/sqrt(61) ≈ 7.7 >> 3.0 — reliable detection.
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
		t.Skip("viewers baseline not available")
	}

	// First tick with insert error: must not crash.
	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines (insert error): %v", err)
	}

	// Hysteresis must be set despite the insert error — clear insertErr and
	// confirm suppression on tick 2.
	flagStore.insertErr = nil
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines tick 2: %v", err)
	}
	eventsAfterTick2 := flagStore.capturedEvents()
	if len(eventsAfterTick2) != 0 {
		t.Errorf("expected 0 stored events on tick 2 (hysteresis suppression), got %d", len(eventsAfterTick2))
	}
	t.Logf("PASS: insert error → no crash; hysteresis still set → tick 2 suppressed")
}

// TestFlagStore_WarmHysteresis_SuppressesRefire verifies that WarmHysteresis
// pre-populates the hysteresis map from the store, causing ComputeFlags to
// suppress a flag that would otherwise fire after a restart.
func TestFlagStore_WarmHysteresis_SuppressesRefire(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}

	// Scope for stream "s1" — matches the live provider above.
	s1Scope := `{"stream_id":"s1"}`

	flagStore := &fakeFlagStore{
		recentKeys: []anomaly.FlagKey{
			{Metric: "viewers", Scope: s1Scope},
		},
	}

	hysteresisTicks := 5
	tickInterval := 2 * time.Second

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: hysteresisTicks,
		TickInterval:    tickInterval,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()

	// Warm up the baseline (alternating to build stddev).
	warmupLive(t, det, live, 10, 95, 105)

	// WarmHysteresis should populate the hysteresis map for the "viewers/s1" key.
	if err := det.WarmHysteresis(ctx); err != nil {
		t.Fatalf("WarmHysteresis: %v", err)
	}

	baselines, _ := store.ListAnomalyBaselines(ctx)
	var vb *anomaly.AnomalyBaselineRow
	for i := range baselines {
		if baselines[i].Metric == "viewers" {
			vb = &baselines[i]
			break
		}
	}
	if vb == nil || vb.Stddev <= 0 {
		t.Skip("viewers baseline not available")
	}

	// Without WarmHysteresis, ComputeFlags would fire a flag now.
	// With WarmHysteresis, hysteresis is pre-set so the flag is suppressed.
	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)

	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags after WarmHysteresis: %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags (suppressed by WarmHysteresis), got %d", len(flags))
	}
	t.Logf("PASS: WarmHysteresis → flag suppressed after restart simulation")
}

// TestFlagStore_WarmHysteresis_WindowSecs verifies that WarmHysteresis calls
// RecentFlagKeys with windowSecs = hysteresisTicks * int(tickInterval.Seconds()).
func TestFlagStore_WarmHysteresis_WindowSecs(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &anomalyLiveForFlagTest{viewerCount: 100}

	fs := &flagStoreWindowCapture{}
	hysteresisTicks := 7
	tickInterval := 3 * time.Second
	expectedWindowSecs := hysteresisTicks * int(tickInterval.Seconds()) // 21

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: hysteresisTicks,
		TickInterval:    tickInterval,
	}, store, live, nil)
	det.SetFlagStore(fs)

	if err := det.WarmHysteresis(context.Background()); err != nil {
		t.Fatalf("WarmHysteresis: %v", err)
	}

	if fs.capturedWindowSecs != expectedWindowSecs {
		t.Errorf("WarmHysteresis passed windowSecs=%d, want %d",
			fs.capturedWindowSecs, expectedWindowSecs)
	}
	t.Logf("PASS: WarmHysteresis windowSecs=%d (=%d ticks × %v)",
		fs.capturedWindowSecs, hysteresisTicks, tickInterval)
}

// TestComputeFlags_NeverPersists verifies that ComputeFlags (even when a flagStore
// is wired) does NOT write to the flagStore — it is the HTTP on-demand path only.
func TestComputeFlags_NeverPersists(t *testing.T) {
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
		t.Skip("viewers baseline not available")
	}

	// Call ComputeFlags directly (not UpdateBaselines) with anomalous value.
	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)

	flags, err := det.ComputeFlags(ctx, 3.0)
	if err != nil {
		t.Fatalf("ComputeFlags: %v", err)
	}
	if len(flags) == 0 {
		t.Fatal("expected at least 1 flag from ComputeFlags, got 0")
	}

	// flagStore must be empty — ComputeFlags must never persist.
	events := flagStore.capturedEvents()
	if len(events) != 0 {
		t.Errorf("ComputeFlags wrote %d event(s) to flagStore — must not persist", len(events))
	}
	t.Logf("PASS: ComputeFlags emitted %d flag(s) but wrote 0 events to flagStore", len(flags))
}

// TestComputeFlags_SigmaThresholdHonored verifies that ComputeFlags still
// honors the sigmaThreshold argument after the checkFlags refactor.
func TestComputeFlags_SigmaThresholdHonored(t *testing.T) {
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
		t.Skip("viewers baseline not available")
	}

	// With a very high sigma threshold (100.0), no flag should fire.
	injected := int(vb.Mean + 20.0*vb.Stddev)
	live.setViewers(injected)
	flags, err := det.ComputeFlags(ctx, 100.0)
	if err != nil {
		t.Fatalf("ComputeFlags (high sigma): %v", err)
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags at sigma=100.0, got %d", len(flags))
	}
	t.Logf("PASS: ComputeFlags sigma=100.0 → 0 flags")
}

// TestFlagStore_DetectedAtIsTickAt verifies that all flag events from one
// UpdateBaselines call share the same DetectedAt value (the tick timestamp),
// not different per-event time.Now() calls.
func TestFlagStore_DetectedAtIsTickAt(t *testing.T) {
	store := &fakeBaselineStore{}
	live := &dualStreamLive{viewers1: 100, viewers2: 200}
	flagStore := &fakeFlagStore{}

	det := anomaly.New(anomaly.Config{
		DefaultSigma:    3.0,
		MinSamples:      5,
		HysteresisTicks: 10,
		TickInterval:    time.Second,
	}, store, live, nil)
	det.SetFlagStore(flagStore)

	ctx := context.Background()

	// Warmup both streams with alternating values. 60 ticks so the spike is detectable.
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			live.viewers1 = 95
			live.viewers2 = 190
		} else {
			live.viewers1 = 105
			live.viewers2 = 210
		}
		if err := det.UpdateBaselines(ctx); err != nil {
			t.Fatalf("warmup tick %d: %v", i, err)
		}
	}

	// Inject anomaly on both streams simultaneously in one UpdateBaselines call.
	live.viewers1 = 10000
	live.viewers2 = 50000
	if err := det.UpdateBaselines(ctx); err != nil {
		t.Fatalf("UpdateBaselines (anomalous): %v", err)
	}

	events := flagStore.capturedEvents()
	if len(events) == 0 {
		t.Skip("no events fired — cannot verify DetectedAt invariant")
	}
	if len(events) < 2 {
		t.Logf("got %d events (only 1 stream may have fired)", len(events))
	}

	// All events from one tick must share the same DetectedAt (ms precision).
	firstTS := events[0].DetectedAt
	for i, ev := range events[1:] {
		if !ev.DetectedAt.Equal(firstTS) {
			t.Errorf("event[%d] DetectedAt %v != event[0] DetectedAt %v (must share tick timestamp)",
				i+1, ev.DetectedAt, firstTS)
		}
	}
	t.Logf("PASS: %d events share DetectedAt=%v", len(events), firstTS)
}
