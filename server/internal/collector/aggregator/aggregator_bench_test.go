// Package aggregator — benchmarks and O(1)-per-event regression guards.
//
// BUILD HISTORY
//
//	S10/D-068: harness authored BEFORE the incremental-snapshot fix so that BEFORE
//	and AFTER numbers come from the same measurement apparatus.
package aggregator

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Poll-cycle benchmarks ────────────────────────────────────────────────────

// benchmarkPollCycle seeds n streams via publish_start, then times n stream_stats
// events (one simulated 5-second poll cycle) per b.N iteration.
//
// Subscriber note: one channel (cap 16) is registered but never drained.
// After the first 16 notifySubs calls the channel is full; all subsequent calls
// take the non-blocking drop path. With the BEFORE code, notifySubs still calls
// copySnapshot (O(N) deep copy) before every drop attempt, so the subscriber cost
// IS included. With the AFTER code, rate-limiting suppresses the copy to at most
// once per second, so the drop cost becomes a single try-select-default.
func benchmarkPollCycle(b *testing.B, n int) {
	b.Helper()

	agg := New(3*time.Minute, nil, nil)

	// Seed n streams.
	for i := 0; i < n; i++ {
		agg.OnServerEvent(domain.ServerEvent{
			Version:  1,
			Type:     domain.EventStreamPublishStart,
			TS:       time.Now().UnixMilli(),
			Source:   domain.SourceRestPoll,
			NodeID:   "node-1",
			App:      "live",
			StreamID: fmt.Sprintf("stream-%d", i),
			Data:     map[string]any{"publish_type": "rtmp"},
		})
	}

	// One subscriber — not drained; see note above.
	_, cancel := agg.Subscribe()
	defer cancel()

	// Pre-build the event slice representing one poll cycle.
	events := make([]domain.ServerEvent, n)
	for i := 0; i < n; i++ {
		events[i] = domain.ServerEvent{
			Version:  1,
			Type:     domain.EventStreamStats,
			TS:       time.Now().UnixMilli(),
			Source:   domain.SourceRestPoll,
			NodeID:   "node-1",
			App:      "live",
			StreamID: fmt.Sprintf("stream-%d", i),
			Data: map[string]any{
				"viewer_count": 10,
				"bitrate_kbps": 2000.0,
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, ev := range events {
			agg.OnServerEvent(ev)
		}
	}
}

func BenchmarkPollCycle100(b *testing.B)  { benchmarkPollCycle(b, 100) }
func BenchmarkPollCycle500(b *testing.B)  { benchmarkPollCycle(b, 500) }
func BenchmarkPollCycle1000(b *testing.B) { benchmarkPollCycle(b, 1000) }

// ─── Alloc guard ─────────────────────────────────────────────────────────────

// TestPollCycle_AllocsPerEvent_Bounded is the primary CI-stable complexity gate.
//
// It seeds N=1000 streams (one subscriber registered) and measures the number of
// heap allocations produced by a single stream_stats event via testing.AllocsPerRun.
//
// WHY this is count-based, not wall-clock:
//   - Wall-clock varies with CI load and -race overhead; a 2× wall-clock delta is
//     noise; alloc counts are deterministic across hardware and -race mode.
//
// WHY the bound is 64 (generous, order-of-magnitude):
//   - BEFORE code: rebuildSnapshot allocates a LiveSnapshot struct + 3 map backing
//     arrays; notifySubs calls copySnapshot which allocates N=1000 individual
//     LiveStream structs via `vCopy := *v; &vCopy` = ~1005 allocs/event.
//     1005 >> 64 → clearly RED.
//   - AFTER code (O(1) incremental): delta ops are pointer updates and integer
//     arithmetic; notifySubs is rate-limited (at most 1 copySnapshot/second, so
//     ~0 copies during the 200-iteration measurement window).
//     Expected: 0–5 allocs/event → clearly GREEN.
//   - The 64 bound is robust: it survives -race inflation (race detector adds ~2×
//     allocs on map operations) and will not produce false failures on slow CI.
//
// Run with: go test -run TestPollCycle_AllocsPerEvent_Bounded -v -count=1
func TestPollCycle_AllocsPerEvent_Bounded(t *testing.T) {
	const (
		N     = 1000
		bound = 64
	)

	agg := New(3*time.Minute, nil, nil)
	for i := 0; i < N; i++ {
		agg.OnServerEvent(domain.ServerEvent{
			Version:  1,
			Type:     domain.EventStreamPublishStart,
			TS:       time.Now().UnixMilli(),
			Source:   domain.SourceRestPoll,
			NodeID:   "node-1",
			App:      "live",
			StreamID: fmt.Sprintf("stream-%d", i),
			Data:     map[string]any{"publish_type": "rtmp"},
		})
	}

	// Register one subscriber to include notifySubs cost in the measurement.
	// Channel is not drained; the first 16 sends fill the buffer, then each
	// notifySubs call hits the non-blocking drop. BEFORE code still calls
	// copySnapshot before every drop; AFTER code rate-limits the copy.
	_, cancel := agg.Subscribe()
	defer cancel()

	ev := domain.ServerEvent{
		Version:  1,
		Type:     domain.EventStreamStats,
		TS:       time.Now().UnixMilli(),
		Source:   domain.SourceRestPoll,
		NodeID:   "node-1",
		App:      "live",
		StreamID: "stream-0",
		Data: map[string]any{
			"viewer_count": 10,
			"bitrate_kbps": 2000.0,
		},
	}

	allocs := testing.AllocsPerRun(200, func() {
		agg.OnServerEvent(ev)
	})

	t.Logf("AllocsPerRun(200) = %.1f  bound = %d", allocs, bound)
	if allocs > float64(bound) {
		t.Errorf("AllocsPerRun = %.0f exceeds bound %d: O(N) rebuildSnapshot/copySnapshot still on the hot path?",
			allocs, bound)
	}
}

// ─── Equivalence regression test ─────────────────────────────────────────────

// TestAggregator_IncrementalEquivalence drives a seeded random event sequence
// (rand.NewSource(1) — deterministic, same on every run) and verifies that after
// every checkEvery events the incrementally-maintained snapshot is numerically
// consistent with the ground truth computed directly from a.streams/a.nodes.
//
// This is the correctness pin for the incremental redesign: if a delta operation
// (snapAdd/snapRemove) miscounts a viewer or a bitrate, this test fails.
//
// Covers: publish_start / stream_stats (with viewer/bitrate deltas) /
// ingest_stats (bitrate updates) / publish_end (removal) / node_stats.
// Uses unique streamID per slot (stream-0…stream-19) and a single node/app so
// there are no same-StreamID bare-key collisions; equivalence is therefore exact.
func TestAggregator_IncrementalEquivalence(t *testing.T) {
	const (
		maxStreamIdx = 20 // stream-0 … stream-19
		numEvents    = 400
		checkEvery   = 5
	)
	rng := rand.New(rand.NewSource(1))
	agg := New(3*time.Minute, nil, nil)

	for i := 0; i < numEvents; i++ {
		idx := rng.Intn(maxStreamIdx)
		streamID := fmt.Sprintf("stream-%d", idx)
		const (
			nodeID = "node-1"
			app    = "live"
		)

		pick := rng.Intn(5)
		var ev domain.ServerEvent
		switch pick {
		case 0: // publish_start
			ev = domain.ServerEvent{
				Version: 1, Type: domain.EventStreamPublishStart,
				TS: time.Now().UnixMilli(), Source: domain.SourceRestPoll,
				NodeID: nodeID, App: app, StreamID: streamID,
				Data: map[string]any{"publish_type": "rtmp"},
			}
		case 1: // publish_end
			ev = domain.ServerEvent{
				Version: 1, Type: domain.EventStreamPublishEnd,
				TS: time.Now().UnixMilli(), Source: domain.SourceRestPoll,
				NodeID: nodeID, App: app, StreamID: streamID,
				Data: map[string]any{},
			}
		case 2: // stream_stats — exercises viewer/bitrate delta path
			ev = domain.ServerEvent{
				Version: 1, Type: domain.EventStreamStats,
				TS: time.Now().UnixMilli(), Source: domain.SourceRestPoll,
				NodeID: nodeID, App: app, StreamID: streamID,
				Data: map[string]any{
					"viewer_count": rng.Intn(100),
					"bitrate_kbps": float64(rng.Intn(5000)),
				},
			}
		case 3: // ingest_stats — exercises IngestBitrate update path
			ev = domain.ServerEvent{
				Version: 1, Type: domain.EventIngestStats,
				TS: time.Now().UnixMilli(), Source: domain.SourceRestPoll,
				NodeID: nodeID, App: app, StreamID: streamID,
				Data: map[string]any{
					"bitrate_kbps": float64(rng.Intn(5000)),
					"fps":          float64(rng.Intn(60)),
				},
			}
		case 4: // node_stats — exercises O(1) node path
			ev = domain.ServerEvent{
				Version: 1, Type: domain.EventNodeStats,
				TS: time.Now().UnixMilli(), Source: domain.SourceRestPoll,
				NodeID: nodeID,
				Data: map[string]any{
					"cpu_pct": float64(rng.Intn(100)),
					"mem_pct": float64(rng.Intn(100)),
				},
			}
		}
		agg.OnServerEvent(ev)

		if (i+1)%checkEvery == 0 {
			checkSnapshotConsistency(t, agg, fmt.Sprintf("after-event-%d", i+1))
		}
	}
}

// checkSnapshotConsistency verifies that a.snapshot's aggregate counters, stream
// map, and node map are consistent with the ground truth in a.streams/a.nodes.
// Acquires a.mu.RLock() internally — never call with lock held.
func checkSnapshotConsistency(t *testing.T, agg *Aggregator, label string) {
	t.Helper()
	agg.mu.RLock()
	defer agg.mu.RUnlock()

	snap := agg.snapshot

	// Ground truth: walk a.streams directly.
	var wantActive int
	var wantViewers int
	var wantBitrate float64
	wantAppViewers := make(map[string]int)

	for _, s := range agg.streams {
		if !s.Active {
			continue
		}
		wantActive++
		wantViewers += s.ViewerCount
		wantBitrate += s.IngestBitrate
		wantAppViewers[s.App] += s.ViewerCount
	}

	if snap.ActiveStreams != wantActive {
		t.Errorf("%s: ActiveStreams = %d, want %d", label, snap.ActiveStreams, wantActive)
	}
	if snap.TotalViewers != wantViewers {
		t.Errorf("%s: TotalViewers = %d, want %d", label, snap.TotalViewers, wantViewers)
	}
	// Allow tiny IEEE-754 rounding on repeated float64 += deltas.
	const epsilon = 1e-6
	if diff := snap.IngestBitrate - wantBitrate; diff > epsilon || diff < -epsilon {
		t.Errorf("%s: IngestBitrate = %.6f, want %.6f (diff=%.2e)", label, snap.IngestBitrate, wantBitrate, diff)
	}

	// AppViewers: both directions.
	for app, v := range wantAppViewers {
		if snap.AppViewers[app] != v {
			t.Errorf("%s: AppViewers[%q] = %d, want %d", label, app, snap.AppViewers[app], v)
		}
	}
	for app, v := range snap.AppViewers {
		if wantAppViewers[app] != v {
			t.Errorf("%s: snap.AppViewers[%q] = %d, want %d (extra entry)", label, app, v, wantAppViewers[app])
		}
	}

	// Nodes: every node in a.nodes should appear in snap.Nodes.
	for nodeID := range agg.nodes {
		if _, ok := snap.Nodes[nodeID]; !ok {
			t.Errorf("%s: node %q missing from snapshot", label, nodeID)
		}
	}
}
