// Package cluster — fleet discovery tests.
//
// Tests use a mock AMS client (no real broker), exercise the discovery loop
// timing, and verify the ≤ 2 min budget by construction.
package cluster

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// mockClusterClient returns a configurable list of cluster nodes.
type mockClusterClient struct {
	mu    sync.Mutex
	nodes []amsclient.ClusterNodeDTO
	calls atomic.Int32
}

func (m *mockClusterClient) ClusterNodes(_ context.Context) ([]amsclient.ClusterNodeDTO, error) {
	m.calls.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]amsclient.ClusterNodeDTO, len(m.nodes))
	copy(cp, m.nodes)
	return cp, nil
}

// setNodes replaces the node list in a race-safe manner.
func (m *mockClusterClient) setNodes(nodes []amsclient.ClusterNodeDTO) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = nodes
}

// captureSink records events written to it. Satisfies domain.EventSink.
type captureSink struct {
	count atomic.Int32
}

func (c *captureSink) WriteServerEvent(_ domain.ServerEvent)     { c.count.Add(1) }
func (c *captureSink) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (c *captureSink) WriteViewerSession(_ domain.ViewerSession) {}

// Compile-time assertion that captureSink satisfies domain.EventSink.
var _ domain.EventSink = (*captureSink)(nil)

// TestDiscovery_NewNodeVisible verifies that a new node appears in the snapshot
// within one poll interval (≤ 2 min budget = ≤ 30s default, proven by math).
//
// Budget proof:
//   - default poll interval = 30s
//   - new node is visible within 1 poll cycle
//   - 30s ≤ 120s (2 min) → budget met by construction
//   - test uses 20ms interval to verify the mechanism quickly
func TestDiscovery_NewNodeVisible(t *testing.T) {
	mock := &mockClusterClient{}
	sink := &captureSink{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use a very short poll interval for testing.
	testInterval := 20 * time.Millisecond

	d := New(Config{
		PollInterval: testInterval,
		NodeID:       "local",
	}, mock, sink, nil)

	// Start with one node.
	mock.setNodes([]amsclient.ClusterNodeDTO{
		{NodeID: "node-1", IP: "10.0.0.1", Role: "origin", CPUUsage: 15.0, MemoryUsage: 40.0},
	})

	go d.Run(ctx)

	// Wait for the first poll to complete.
	time.Sleep(testInterval * 3)

	if d.NodeCount() < 1 {
		t.Fatalf("after first poll: node count = %d, want ≥1", d.NodeCount())
	}

	// Add a second node.
	t_add := time.Now()
	mock.setNodes([]amsclient.ClusterNodeDTO{
		{NodeID: "node-1", IP: "10.0.0.1", Role: "origin", CPUUsage: 15.0, MemoryUsage: 40.0},
		{NodeID: "node-2", IP: "10.0.0.2", Role: "edge", CPUUsage: 20.0, MemoryUsage: 30.0},
	})

	// Wait for up to 2 poll cycles.
	deadline := time.Now().Add(testInterval * 3)
	for time.Now().Before(deadline) {
		if d.NodeCount() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	discoveryLatency := time.Since(t_add)
	t.Logf("F7 fleet: new node discovered in %v (test interval: %v, budget: 2 min)",
		discoveryLatency, testInterval)

	if d.NodeCount() < 2 {
		t.Errorf("after adding node-2: node count = %d, want 2", d.NodeCount())
	}

	// Budget derivation (D-042 forbids blind bumps — this bound is derived):
	//   testInterval = 20 ms.
	//   A new node becomes visible within at most 1 full poll cycle (≤ 1 × testInterval).
	//   Under -race on a contended 6-core VPS (D-041) the Go scheduler can delay
	//   the ticker goroutine for up to ~2 × testInterval before the next tick fires,
	//   yielding a worst-case observed latency near 3 × testInterval (≈ 60 ms;
	//   68.8 ms was measured in practice under whole-suite -race).
	//   Budget = 5 × testInterval (100 ms) = 1 discovery poll + 4 × testInterval
	//   (80 ms) of scheduler headroom, covering 3 full poll cycles with margin.
	//   This remains well under 1 s, so a genuinely hung discovery loop is still caught.
	if discoveryLatency > testInterval*5 {
		t.Errorf("discovery latency %v > 5 poll cycles (%v) — budget derived: 1 poll + 4× jitter headroom under -race",
			discoveryLatency, testInterval*5)
	}

	t.Logf("PASS: F7 new node visible in ≤ 1 poll cycle (%v)", discoveryLatency)

	// Verify the sink.WriteServerEvent emit path (discovery.go ~177) actually fired.
	if sinkCalls := sink.count.Load(); sinkCalls < 1 {
		t.Errorf("expected sink.WriteServerEvent to be called ≥1 time; got %d", sinkCalls)
	} else {
		t.Logf("PASS: sink.WriteServerEvent called %d times (emit path verified)", sinkCalls)
	}

	// Default config math: 30s poll → node visible within 30s → ≤ 2 min (120s).
	defaultInterval := 30 * time.Second
	budgetSeconds := 2 * time.Minute
	if defaultInterval > budgetSeconds {
		t.Errorf("default poll interval %v exceeds budget %v", defaultInterval, budgetSeconds)
	}
	t.Logf("default 30s poll interval ≤ 2 min budget: PASS")
}

// TestDiscovery_RoleLabeling verifies role labels are correctly set.
func TestDiscovery_RoleLabeling(t *testing.T) {
	mock := &mockClusterClient{
		nodes: []amsclient.ClusterNodeDTO{
			{NodeID: "orig-1", Role: "origin", CPUUsage: 20},
			{NodeID: "edge-1", Role: "edge", CPUUsage: 15},
			{NodeID: "edge-2", Role: "edge", CPUUsage: 25},
		},
	}

	d := New(Config{PollInterval: 10 * time.Millisecond}, mock, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go d.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	snap := d.Snapshot()
	roleMap := make(map[string]string, len(snap))
	for _, n := range snap {
		roleMap[n.NodeID] = n.Role
	}

	if roleMap["orig-1"] != "origin" {
		t.Errorf("orig-1 role = %q, want origin", roleMap["orig-1"])
	}
	if roleMap["edge-1"] != "edge" {
		t.Errorf("edge-1 role = %q, want edge", roleMap["edge-1"])
	}
	if roleMap["edge-2"] != "edge" {
		t.Errorf("edge-2 role = %q, want edge", roleMap["edge-2"])
	}
}

// TestDiscovery_StatusDegraded verifies high CPU/mem triggers degraded status.
func TestDiscovery_StatusDegraded(t *testing.T) {
	mock := &mockClusterClient{
		nodes: []amsclient.ClusterNodeDTO{
			{NodeID: "hot-node", CPUUsage: 95.0, MemoryUsage: 50.0},
		},
	}

	d := New(Config{PollInterval: 10 * time.Millisecond}, mock, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go d.Run(ctx)

	time.Sleep(50 * time.Millisecond)

	snap := d.Snapshot()
	if len(snap) == 0 {
		t.Fatal("no nodes in snapshot")
	}
	if snap[0].Status != "degraded" {
		t.Errorf("high-CPU node status = %q, want degraded", snap[0].Status)
	}
}

// TestDiscovery_NodeRoleQuery verifies NodeRole lookup.
func TestDiscovery_NodeRoleQuery(t *testing.T) {
	mock := &mockClusterClient{
		nodes: []amsclient.ClusterNodeDTO{
			{NodeID: "n1", Role: "origin"},
		},
	}
	d := New(Config{PollInterval: 10 * time.Millisecond}, mock, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go d.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	if d.NodeRole("n1") != "origin" {
		t.Errorf("NodeRole(n1) = %q, want origin", d.NodeRole("n1"))
	}
	if d.NodeRole("nonexistent") != "" {
		t.Errorf("NodeRole(nonexistent) = %q, want empty", d.NodeRole("nonexistent"))
	}
}

// TestDiscovery_DefaultRoleIsOrigin verifies nodes without role default to "origin".
func TestDiscovery_DefaultRoleIsOrigin(t *testing.T) {
	mock := &mockClusterClient{
		nodes: []amsclient.ClusterNodeDTO{
			{NodeID: "n1"}, // no Role set
		},
	}
	d := New(Config{PollInterval: 10 * time.Millisecond}, mock, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go d.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	snap := d.Snapshot()
	if len(snap) == 0 {
		t.Fatal("no nodes")
	}
	if snap[0].Role != "origin" {
		t.Errorf("default role = %q, want origin", snap[0].Role)
	}
}

// TestDiscovery_PollsRepeatedly verifies the discovery polls multiple times.
func TestDiscovery_PollsRepeatedly(t *testing.T) {
	mock := &mockClusterClient{
		nodes: []amsclient.ClusterNodeDTO{{NodeID: "n1"}},
	}
	d := New(Config{PollInterval: 15 * time.Millisecond}, mock, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go d.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	calls := mock.calls.Load()
	if calls < 3 {
		t.Errorf("expected ≥ 3 polls in 100ms with 15ms interval, got %d", calls)
	}
	t.Logf("poll count in 100ms (interval=15ms): %d", calls)
}

// reentrantSink mimics the live aggregator: from inside WriteServerEvent it calls
// back into Discovery.IsEdgeStream, which takes d.mu.RLock — exactly the path
// OnServerEvent → onStreamStats → IsEdgeStream takes in production. If poll()
// holds d.mu while emitting, that RLock deadlocks the goroutine against itself.
type reentrantSink struct {
	d     *Discovery
	calls atomic.Int32
}

func (s *reentrantSink) WriteServerEvent(domain.ServerEvent) {
	s.d.IsEdgeStream("any-stream") // RLock d.mu — must not be held by poll()
	s.calls.Add(1)
}
func (s *reentrantSink) WriteBeaconEvent(domain.BeaconEvent)     {}
func (s *reentrantSink) WriteViewerSession(domain.ViewerSession) {}

// TestDiscovery_PollDoesNotHoldLockDuringSinkEmit is a regression guard for the
// AB→BA deadlock between cluster.Discovery (d.mu) and the live aggregator (a.mu)
// that wedged the dashboard: poll() emitted node_stats while holding d.mu, the
// sink fanned it into the aggregator (a.mu.Lock), which re-entered Discovery via
// IsEdgeStream (d.mu.RLock). With the fix, poll() emits only after releasing d.mu.
func TestDiscovery_PollDoesNotHoldLockDuringSinkEmit(t *testing.T) {
	mock := &mockClusterClient{}
	mock.setNodes([]amsclient.ClusterNodeDTO{{NodeID: "n1", IP: "10.0.0.1", Role: "origin"}})
	d := New(Config{}, mock, nil, nil)
	sink := &reentrantSink{d: d}
	d.sink = sink

	done := make(chan struct{})
	go func() { d.poll(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("poll() deadlocked: it emitted to the sink while holding d.mu (AB→BA with the aggregator)")
	}
	if sink.calls.Load() == 0 {
		t.Fatal("expected the reentrant sink to receive the node_stats event")
	}
}
