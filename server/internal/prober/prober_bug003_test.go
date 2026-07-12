package prober_test

// BUG-003 regression pin + ancillary tests.
//
// Root cause: Run's refresh loop calls spawnProbe for every probe on every tick
// unconditionally.  spawnProbe cancels the running goroutine and spawns a new
// one; the new goroutine calls r.clock.After(jitter) with jitter==0
// (MaxJitterFraction=0 in production), so it fires immediately — right on top
// of the old goroutine's own periodic fire.
//
// Fix contract:
//  1. Config.RefreshInterval wires the 60 s default through r.clock.After so
//     FakeClock can drive the refresh loop deterministically.
//  2. spawnProbe compares the incoming ProbeConfig to the stored one; if
//     unchanged it skips cancel+respawn entirely.
//  3. A genuinely changed config still triggers a respawn.
//  4. A removed probe is still cancelled and deleted (unchanged behaviour).
//  5. N24: a freshly-added probe's first check still fires immediately
//     (After(0) with MaxJitterFraction==0).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/prober"
)

// ─── mutableFakeSource ────────────────────────────────────────────────────────

// mutableFakeSource is a ProbeConfigSource whose probe list can be swapped
// mid-test to simulate probe create/update/delete events.
type mutableFakeSource struct {
	mu     sync.Mutex
	probes []domain.ProbeConfig
}

func (m *mutableFakeSource) ListEnabled(_ context.Context) ([]domain.ProbeConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]domain.ProbeConfig, len(m.probes))
	copy(cp, m.probes)
	return cp, nil
}

func (m *mutableFakeSource) RecordResult(_ context.Context, _ domain.ProbeResult) error {
	return nil
}

func (m *mutableFakeSource) SetProbes(probes []domain.ProbeConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probes = make([]domain.ProbeConfig, len(probes))
	copy(m.probes, probes)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestBUG003_NoRespawnOnUnchangedConfig is the regression pin for BUG-003.
//
// Setup:
//   - ONE probe, interval_s=30, MaxJitterFraction=0, RefreshInterval=100s
//   - FakeClock drives both probe timers and the refresh ticker
//   - Advance in steps: T=30 (fire #2), T=60 (fire #3), T=90 (fire #4)
//   - Advance 10 s more to T=100 (refresh fires, config unchanged → NO respawn)
//   - 500 ms real-time wait for any spurious extra fire to land
//   - Assert exactly 4 results
//
// Why this test CANNOT false-pass:
//  1. prober.Config.RefreshInterval did not exist before the fix → the
//     file fails to compile on today's code → truly RED.
//  2. After the fix, if the respawn guard is missing (RefreshInterval added
//     but no equality check), G2 spawns and After(0) fires immediately →
//     InsertProbeResult is called a fifth time → "exactly 4" fails.
//  3. The 500 ms real-time sleep after the refresh advance is more than
//     sufficient for a localhost HLS probe (< 50 ms) to complete and write
//     its result before the assertion.
func TestBUG003_NoRespawnOnUnchangedConfig(t *testing.T) {
	srv := buildHLSOrigin(t, 1_000, 2.0)

	probe1 := domain.ProbeConfig{
		ID:        "bug003-pin",
		Name:      "bug003",
		URL:       srv.URL + "/playlist.m3u8",
		Protocol:  "hls",
		IntervalS: 30,
		TimeoutS:  5,
		Enabled:   true,
	}
	source := &mutableFakeSource{probes: []domain.ProbeConfig{probe1}}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())

	r := prober.New(prober.Config{
		Workers:           2,
		MaxJitterFraction: 0,
		RefreshInterval:   100 * time.Second, // NOTE: field must exist in Config
	}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { _ = r.Run(ctx) }()

	// T=0 fire: After(0) fires immediately (MaxJitterFraction==0).
	got := waitForResults(store, 1, 5*time.Second)
	if len(got) < 1 {
		t.Fatal("BUG-003: fire #1 (T=0) did not happen")
	}

	// T=30: probe interval fire #2.
	time.Sleep(20 * time.Millisecond) // let scheduler goroutine register After(30s)
	clock.Advance(30 * time.Second)
	got = waitForResults(store, 2, 5*time.Second)
	if len(got) < 2 {
		t.Fatalf("BUG-003: fire #2 (T=30s) did not happen; got %d results", len(got))
	}

	// T=60: probe interval fire #3.
	time.Sleep(20 * time.Millisecond)
	clock.Advance(30 * time.Second)
	got = waitForResults(store, 3, 5*time.Second)
	if len(got) < 3 {
		t.Fatalf("BUG-003: fire #3 (T=60s) did not happen; got %d results", len(got))
	}

	// T=90: probe interval fire #4.
	time.Sleep(20 * time.Millisecond)
	clock.Advance(30 * time.Second)
	got = waitForResults(store, 4, 5*time.Second)
	if len(got) < 4 {
		t.Fatalf("BUG-003: fire #4 (T=90s) did not happen; got %d results", len(got))
	}

	// T=100: refresh fires (RefreshInterval=100s). Config is UNCHANGED.
	// With fix  → no respawn → count stays 4.
	// Without equality guard → G2 spawns → After(0) fires → count becomes 5.
	time.Sleep(20 * time.Millisecond) // let scheduler goroutine register After(30s) for T=120
	clock.Advance(10 * time.Second)   // total virtual time = 100 s → refreshCh fires
	// Allow ample real time for any spurious G2 fire to complete and land in the store.
	time.Sleep(500 * time.Millisecond)

	got = store.Results()
	if len(got) != 4 {
		t.Errorf("BUG-003 REGRESSION: expected exactly 4 probe fires in 100 virtual seconds "+
			"(1 immediate + 3 interval; refresh at T=100 must NOT respawn an unchanged probe), "+
			"got %d — extra rows indicate unconditional-respawn bug", len(got))
	}

	// Belt-and-suspenders: consecutive results must differ by >= 1 ms of virtual
	// time.  Duplicate fires at the same fake-clock instant share an identical TS,
	// so diff == 0 < 1 ms catches the duplication.
	for i := 1; i < len(got); i++ {
		diff := got[i].TS.Sub(got[i-1].TS)
		if diff < 0 {
			diff = -diff
		}
		if diff < time.Millisecond {
			t.Errorf("BUG-003: results[%d] and results[%d] have TS within 1 ms (diff=%v): "+
				"duplicate fires at the same clock instant", i-1, i, diff)
		}
	}

	cancel()
	t.Logf("PASS BUG-003 pin: %d fires in 100 virtual seconds (expected 4)", len(got))
}

// TestBUG003_ChangedConfigRespawns verifies that a probe whose config changed
// IS respawned on the next refresh: the new goroutine fires immediately and
// probes the new URL (Success=false because new URL → HTTP 500).
// interval_s=3600 ensures G1's periodic timer never fires during the test,
// eliminating any race between G1's own fire and G2's immediate first fire.
func TestBUG003_ChangedConfigRespawns(t *testing.T) {
	srvOK := buildHLSOrigin(t, 1_000, 2.0)

	// Fail server: always HTTP 500.
	srvFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srvFail.Close)

	// interval_s=3600 ensures G1's own periodic timer never fires during this
	// test (it would fire at T=3600s, far past our 10s refresh advance), so
	// result #2 can only come from G2's immediate After(0) after the respawn.
	probe := domain.ProbeConfig{
		ID:        "changed-probe",
		Name:      "changed",
		URL:       srvOK.URL + "/playlist.m3u8",
		Protocol:  "hls",
		IntervalS: 3600,
		TimeoutS:  5,
		Enabled:   true,
	}
	source := &mutableFakeSource{probes: []domain.ProbeConfig{probe}}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{
		Workers:           2,
		MaxJitterFraction: 0,
		RefreshInterval:   10 * time.Second, // short so refresh is the only driver
	}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { _ = r.Run(ctx) }()

	// Wait for the initial success fire (After(0) with MaxJitterFraction==0).
	got := waitForResults(store, 1, 5*time.Second)
	if len(got) < 1 {
		t.Fatal("expected first result before config change")
	}
	if !got[0].Success {
		t.Fatalf("expected first result Success=true; got error_code=%q", got[0].ErrorCode)
	}

	// Swap the probe URL to the fail server. Changed field → respawn expected.
	changed := probe
	changed.URL = srvFail.URL + "/fail.m3u8"
	source.SetProbes([]domain.ProbeConfig{changed})

	// Advance 10s → refreshCh fires. Config changed → G1 cancelled, G2 spawned.
	// G1's own timer fires at T=3600s — not ready → no race.
	// G2's After(0) fires immediately → probes srvFail → HTTP 500 → Success=false.
	time.Sleep(20 * time.Millisecond) // let G1 scheduler reach its After(3600s)
	clock.Advance(10 * time.Second)
	got = waitForResults(store, 2, 5*time.Second)
	if len(got) < 2 {
		t.Fatal("expected second result after config change + refresh")
	}
	if got[1].Success {
		t.Errorf("expected second result Success=false (new URL → 500 server); got Success=true")
	}

	cancel()
	t.Logf("PASS: changed probe config triggers respawn; new URL probed (Success=%v)", got[1].Success)
}

// TestBUG003_RemovedProbeStops verifies that a probe absent from the source's
// next ListEnabled response has its goroutine cancelled and stops firing.
func TestBUG003_RemovedProbeStops(t *testing.T) {
	srv := buildHLSOrigin(t, 1_000, 2.0)
	probe := domain.ProbeConfig{
		ID:        "remove-probe",
		Name:      "remove",
		URL:       srv.URL + "/playlist.m3u8",
		Protocol:  "hls",
		IntervalS: 30,
		TimeoutS:  5,
		Enabled:   true,
	}
	source := &mutableFakeSource{probes: []domain.ProbeConfig{probe}}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{
		Workers:           2,
		MaxJitterFraction: 0,
		RefreshInterval:   60 * time.Second,
	}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { _ = r.Run(ctx) }()

	// Wait for the first fire.
	got := waitForResults(store, 1, 5*time.Second)
	if len(got) < 1 {
		t.Fatal("expected at least one result before removal")
	}

	// Remove the probe.
	source.SetProbes(nil)

	// Trigger refresh → probe removed → G1.cancel() called.
	time.Sleep(20 * time.Millisecond)
	clock.Advance(60 * time.Second)

	// Wait for any in-flight execution to land.
	time.Sleep(300 * time.Millisecond)
	countAfter := len(store.Results())

	// Advance another full interval; no new fires expected.
	clock.Advance(30 * time.Second)
	time.Sleep(300 * time.Millisecond)

	finalCount := len(store.Results())
	// Allow at most 1 extra in-flight result at the moment of cancellation, but
	// no further fires after the goroutine is cancelled.
	if finalCount > countAfter+1 {
		t.Errorf("probe should have stopped after removal; count grew from %d to %d "+
			"after a full extra interval with no probe in the source", countAfter, finalCount)
	}

	cancel()
	t.Logf("PASS: removed probe stopped (count after removal: %d, final: %d)", countAfter, finalCount)
}

// TestBUG003_N24_FirstFireImmediate verifies the N24 constraint: a probe with
// MaxJitterFraction==0 fires its first check without any clock.Advance call.
// This test ensures the fix did NOT introduce a startup delay for new probes.
func TestBUG003_N24_FirstFireImmediate(t *testing.T) {
	srv := buildHLSOrigin(t, 1_000, 2.0)
	probe := domain.ProbeConfig{
		ID:        "n24-probe",
		Name:      "n24",
		URL:       srv.URL + "/playlist.m3u8",
		Protocol:  "hls",
		IntervalS: 60,
		TimeoutS:  5,
		Enabled:   true,
	}
	source := &mutableFakeSource{probes: []domain.ProbeConfig{probe}}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{
		Workers:           2,
		MaxJitterFraction: 0,
		RefreshInterval:   60 * time.Second,
	}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = r.Run(ctx) }()

	// N24: first result must arrive without ANY clock.Advance call.
	// After(0) fires immediately under FakeClock (and under realClock);
	// the 5 s real-time budget covers CI latency while proving no startup delay
	// was introduced by the fix.
	got := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(got) == 0 {
		t.Fatal("N24 FAIL: first probe did not fire without clock.Advance — fix introduced a startup delay")
	}
	t.Logf("PASS N24: first probe fired immediately (no clock.Advance needed)")
}
