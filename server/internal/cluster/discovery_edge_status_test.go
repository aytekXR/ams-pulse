// discovery_edge_status_test.go — S52 regression test (D-114) for S48 finding [5].
//
// poll() marks a stale node Status="down" but never clears its last-polled
// ActiveStreams. IsEdgeStream must therefore exclude "down" edges — otherwise a
// crashed edge keeps IsEdgeStream true forever, and the aggregator permanently
// skips origin viewer_count (VD-03 dedup), freezing origin viewer totals at 0.
//
// Internal test (package cluster): seeds d.nodes directly so the predicate is
// exercised deterministically, without depending on poll-loop timing.
package cluster

import "testing"

func TestIsEdgeStream_ExcludesDownEdge(t *testing.T) {
	cases := []struct {
		name  string
		nodes []*NodeInfo
		want  bool
	}{
		{
			name: "down edge with stale ActiveStreams does NOT count (the fix)",
			nodes: []*NodeInfo{
				{NodeID: "edge-1", Role: "edge", Status: "down", ActiveStreams: 5},
				{NodeID: "origin-1", Role: "origin", Status: "ok", ActiveStreams: 3},
			},
			want: false,
		},
		{
			name: "healthy edge with active streams counts (positive control)",
			nodes: []*NodeInfo{
				{NodeID: "edge-1", Role: "edge", Status: "ok", ActiveStreams: 5},
				{NodeID: "origin-1", Role: "origin", Status: "ok", ActiveStreams: 3},
			},
			want: true,
		},
		{
			// Pins the predicate to `!= "down"`, NOT `== "ok"`: a degraded edge is
			// still up and serving edge viewers, so origin still double-counts and
			// must still be skipped.
			name: "degraded edge with active streams still counts",
			nodes: []*NodeInfo{
				{NodeID: "edge-1", Role: "edge", Status: "degraded", ActiveStreams: 2},
			},
			want: true,
		},
		{
			name: "healthy edge with zero active streams does not count",
			nodes: []*NodeInfo{
				{NodeID: "edge-1", Role: "edge", Status: "ok", ActiveStreams: 0},
			},
			want: false,
		},
		{
			name: "only a down edge remains after origin keeps serving",
			nodes: []*NodeInfo{
				{NodeID: "edge-1", Role: "edge", Status: "down", ActiveStreams: 9},
				{NodeID: "origin-1", Role: "origin", Status: "ok", ActiveStreams: 4},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := New(Config{NodeID: "local"}, &mockClusterClient{}, &captureSink{}, nil)
			d.mu.Lock()
			for _, n := range tc.nodes {
				d.nodes[n.NodeID] = n
			}
			d.mu.Unlock()

			if got := d.IsEdgeStream("any-stream"); got != tc.want {
				t.Errorf("IsEdgeStream = %v, want %v (a down edge must not keep origin viewer counts suppressed)", got, tc.want)
			}
		})
	}
}
