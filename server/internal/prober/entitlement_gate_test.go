// S46 (D-108) — the probe runner must stop probing at runtime when the tenant's
// tier no longer includes synthetic probes.
//
// The HTTP CRUD handlers gate CheckProbes() (403 on Free), but the background
// Runner kept executing enabled probes regardless — a downgraded tenant went on
// probing (S37 "enforced, not decorative" class). Config.EntitlementGate, wired
// to license.Manager.CheckProbes in serve.go, is checked before every execution.
//
// This drives the real Run loop against a WORKING HLS origin so the only reason a
// gated probe produces no result is the gate — not a probe failure. The ungated
// arm is the positive control (a result DOES appear), so removing the gate check
// in executeProbe turns the gated arm RED.
package prober_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/prober"
)

func TestRunner_EntitlementGate(t *testing.T) {
	newSource := func(url string) *fakeSource {
		return &fakeSource{probes: []domain.ProbeConfig{{
			ID: "p1", Name: "gated-probe", URL: url,
			Protocol: "hls", IntervalS: 60, TimeoutS: 5, Enabled: true,
		}}}
	}

	run := func(t *testing.T, gate func() error) (results, recorded int) {
		t.Helper()
		srv := buildHLSOrigin(t, 50_000, 6.0) // working origin
		source := newSource(srv.URL + "/playlist.m3u8")
		store := &fakeStore{}
		clock := NewFakeClock(time.Now())
		r := prober.New(prober.Config{
			Workers: 2, MaxJitterFraction: 0, EntitlementGate: gate,
		}, source, store, nil, clock)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		go func() { _ = r.Run(ctx) }()

		// jitter=0 → the first probe fires immediately. A non-gated probe against
		// a working origin produces a result within ~1s; wait up to 1.5s.
		got := waitForResults(store, 1, 1500*time.Millisecond)
		cancel()
		return len(got), len(source.Results())
	}

	t.Run("downgraded tier -> no probes execute", func(t *testing.T) {
		results, recorded := run(t, func() error { return fmt.Errorf("synthetic probes require Pro tier") })
		if results != 0 {
			t.Errorf("gate failed: %d probe results written despite downgrade (want 0)", results)
		}
		if recorded != 0 {
			t.Errorf("gate failed: RecordResult called %d times despite downgrade (want 0)", recorded)
		}
	})

	t.Run("licensed tier (nil gate) -> probes execute (positive control)", func(t *testing.T) {
		results, _ := run(t, nil)
		if results == 0 {
			t.Error("positive control failed: no probe result with a nil gate — the harness would make the gated arm vacuous")
		}
	})
}
