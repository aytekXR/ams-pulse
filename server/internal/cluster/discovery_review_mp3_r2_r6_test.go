// discovery_review_mp3_r2_r6_test.go — REVIEW-MP3 round-3 regression pins.
//
//	R2: cluster.Discovery must NOT fabricate the alias-only metrics (disk/net/jvm/
//	    version) that real AMS 3.x never sends. Post-N2 both Discovery and the
//	    restpoller key their node_stats to the SAME real node ID, so Discovery's
//	    unconditional zeros overwrote the poller's clean event every poll —
//	    flapping the aggregator's presence flags, rendering "Disk 0%" as a
//	    measurement, and feeding zeros into the Welford anomaly baselines.
//	R6: AMS's own liveness fields (status / lastUpdateTime) must mark a node down.
//	    AMS keeps a dead member LISTED with a frozen lastUpdateTime, so the
//	    vanish-based staleness sweep never fires for it and the node reported "ok"
//	    with last-known stats forever.
package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/collector"
	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
)

// recordingSink captures the full events, unlike captureSink which only counts.
type recordingSink struct {
	mu     sync.Mutex
	events []domain.ServerEvent
}

func (r *recordingSink) WriteServerEvent(ev domain.ServerEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}
func (r *recordingSink) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (r *recordingSink) WriteViewerSession(_ domain.ViewerSession) {}

func (r *recordingSink) nodeStats() []domain.ServerEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.ServerEvent
	for _, ev := range r.events {
		if ev.Type == domain.EventNodeStats {
			out = append(out, ev)
		}
	}
	return out
}

var _ domain.EventSink = (*recordingSink)(nil)

// pollOnce runs exactly one discovery poll cycle against the given nodes and
// returns the emitted node_stats events plus the resulting snapshot.
func pollOnce(t *testing.T, nodes []amsclient.ClusterNodeDTO) (*recordingSink, *Discovery) {
	t.Helper()
	mock := &mockClusterClient{}
	mock.setNodes(nodes)
	sink := &recordingSink{}
	d := New(Config{PollInterval: 30 * time.Second, NodeID: "local"}, mock, sink, nil)
	d.poll(context.Background())
	return sink, d
}

// ─── R2: no fabricated metrics ──────────────────────────────────────────────

// TestR2_RealAMSNode_NoFabricatedMetrics pins the exact real-AMS-3.x wire shape:
// only id/ip/cpu/memory/status/lastUpdateTime are sent. Every alias-only field
// must be ABSENT from the emitted event, not present as zero.
func TestR2_RealAMSNode_NoFabricatedMetrics(t *testing.T) {
	sink, _ := pollOnce(t, []amsclient.ClusterNodeDTO{{
		ID:             "cluster-node-a",
		IP:             "10.0.0.1",
		CPU:            "15.3",
		Memory:         "40.2%",
		Status:         "Running",
		LastUpdateTime: time.Now().UnixMilli(),
	}})

	events := sink.nodeStats()
	if len(events) != 1 {
		t.Fatalf("want 1 node_stats event, got %d", len(events))
	}
	data := events[0].Data

	// The two real wire metrics must be present and correct.
	if v, ok := data["cpu_pct"].(float64); !ok || v != 15.3 {
		t.Errorf("cpu_pct = %v (ok=%v), want 15.3", data["cpu_pct"], ok)
	}
	if v, ok := data["mem_pct"].(float64); !ok || v != 40.2 {
		t.Errorf("mem_pct = %v (ok=%v), want 40.2", data["mem_pct"], ok)
	}

	// The alias-only fields must be ABSENT. A present zero is the bug: the
	// aggregator sets *Reported=true for any present key, so "Disk 0%" renders
	// as a real measurement and poisons the anomaly baseline.
	for _, key := range []string{"disk_pct", "net_in_mbps", "net_out_mbps", "jvm_heap_used_mb", "version"} {
		if v, present := data[key]; present {
			t.Errorf("FAIL(R2): %q present with value %v — real AMS 3.x never sends it; "+
				"emitting it fabricates a measurement", key, v)
		}
	}
}

// TestR2_AliasNodeStillEmitsPresentFields guards the other direction: a mock/alias
// profile that DOES carry the fields must still have them emitted, so the R2 fix
// cannot be satisfied by simply dropping the fields altogether.
func TestR2_AliasNodeStillEmitsPresentFields(t *testing.T) {
	sink, _ := pollOnce(t, []amsclient.ClusterNodeDTO{{
		NodeID:           "alias-node",
		IP:               "10.0.0.2",
		CPUUsage:         12.0,
		MemoryUsage:      30.0,
		DiskUsage:        55.0,
		NetworkInputBps:  2_000_000,
		NetworkOutputBps: 4_000_000,
		JvmMemoryUsage:   512,
		Version:          "3.0.3",
	}})

	events := sink.nodeStats()
	if len(events) != 1 {
		t.Fatalf("want 1 node_stats event, got %d", len(events))
	}
	data := events[0].Data

	want := map[string]any{
		"disk_pct":         55.0,
		"net_in_mbps":      2.0,
		"net_out_mbps":     4.0,
		"jvm_heap_used_mb": 512.0,
		"version":          "3.0.3",
	}
	for key, expect := range want {
		got, present := data[key]
		if !present {
			t.Errorf("%q absent — alias profiles must still report it", key)
			continue
		}
		if got != expect {
			t.Errorf("%q = %v, want %v", key, got, expect)
		}
	}
}

// TestR2_MatchesNormalizeClusterNode is the anti-drift pin: Discovery and the
// restpoller emit under the SAME node key, so their emission RULES must agree
// exactly. If either side changes independently, one overwrites the other.
func TestR2_MatchesNormalizeClusterNode(t *testing.T) {
	for name, dto := range map[string]amsclient.ClusterNodeDTO{
		"real-ams-wire": {ID: "n1", IP: "10.0.0.1", CPU: "15.3", Memory: "40.2%", Status: "Running"},
		"alias-mock":    {NodeID: "n2", IP: "10.0.0.2", CPUUsage: 9, MemoryUsage: 11, DiskUsage: 3, Version: "3.0.3"},
		"partial":       {ID: "n3", IP: "10.0.0.3", CPU: "50", Memory: "60", JvmMemoryUsage: 128},
	} {
		t.Run(name, func(t *testing.T) {
			sink, _ := pollOnce(t, []amsclient.ClusterNodeDTO{dto})
			events := sink.nodeStats()
			if len(events) != 1 {
				t.Fatalf("want 1 event, got %d", len(events))
			}
			discoveryKeys := events[0].Data

			// The restpoller's emission for the identical DTO.
			normalizeKeys := collector.NormalizeClusterNode(dto).Data

			for k, v := range normalizeKeys {
				dv, present := discoveryKeys[k]
				if !present {
					t.Errorf("key %q emitted by NormalizeClusterNode but not by Discovery", k)
					continue
				}
				if dv != v {
					t.Errorf("key %q: Discovery=%v NormalizeClusterNode=%v — values must agree", k, dv, v)
				}
			}
			for k := range discoveryKeys {
				if _, present := normalizeKeys[k]; !present {
					t.Errorf("key %q emitted by Discovery but not by NormalizeClusterNode — "+
						"Discovery would overwrite the poller's event with a field it invented", k)
				}
			}
		})
	}
}

// ─── R6: AMS liveness signals ───────────────────────────────────────────────

// TestR6_NonRunningStatusMarksNodeDown pins that AMS's own status field wins over
// the cpu/mem load heuristic.
func TestR6_NonRunningStatusMarksNodeDown(t *testing.T) {
	for _, status := range []string{"Not Running", "SHUTTING_DOWN", "dead"} {
		t.Run(status, func(t *testing.T) {
			_, d := pollOnce(t, []amsclient.ClusterNodeDTO{{
				ID:             "dead-node",
				IP:             "10.0.0.9",
				CPU:            "5",
				Memory:         "10%",
				Status:         status,
				LastUpdateTime: time.Now().UnixMilli(),
			}})

			nodes := d.Snapshot()
			if len(nodes) != 1 {
				t.Fatalf("want 1 node, got %d", len(nodes))
			}
			if nodes[0].Status != "down" {
				t.Errorf("FAIL(R6): AMS status %q → Pulse status %q, want \"down\"; "+
					"a dead-but-listed node reporting healthy stats is the exact defect", status, nodes[0].Status)
			}
		})
	}
}

// TestR6_RunningStatusStaysOK guards against over-triggering: the healthy value
// AMS actually sends must not be read as down.
func TestR6_RunningStatusStaysOK(t *testing.T) {
	for _, status := range []string{"Running", "running", "RUNNING"} {
		_, d := pollOnce(t, []amsclient.ClusterNodeDTO{{
			ID: "live-node", IP: "10.0.0.1", CPU: "5", Memory: "10%",
			Status: status, LastUpdateTime: time.Now().UnixMilli(),
		}})
		if got := d.Snapshot()[0].Status; got != "ok" {
			t.Errorf("AMS status %q → %q, want \"ok\"", status, got)
		}
	}
}

// TestR6_FrozenLastUpdateTimeMarksNodeDown pins the second liveness signal: AMS
// still lists the member and still claims "Running", but has stopped updating it.
func TestR6_FrozenLastUpdateTimeMarksNodeDown(t *testing.T) {
	// StaleTimeout defaults to 3 × PollInterval = 90s at the 30s interval used here.
	frozen := time.Now().Add(-5 * time.Minute).UnixMilli()

	_, d := pollOnce(t, []amsclient.ClusterNodeDTO{{
		ID:             "frozen-node",
		IP:             "10.0.0.8",
		CPU:            "5",
		Memory:         "10%",
		Status:         "Running", // AMS never revised it
		LastUpdateTime: frozen,
	}})

	nodes := d.Snapshot()
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0].Status != "down" {
		t.Errorf("FAIL(R6): lastUpdateTime frozen 5m ago → status %q, want \"down\"", nodes[0].Status)
	}
}

// TestR6_AbsentLivenessFieldsFallBackToLoadHeuristic pins that alias/mock profiles
// (which send neither field) keep their previous behavior.
func TestR6_AbsentLivenessFieldsFallBackToLoadHeuristic(t *testing.T) {
	_, d := pollOnce(t, []amsclient.ClusterNodeDTO{
		{NodeID: "healthy", IP: "10.0.0.1", CPUUsage: 10, MemoryUsage: 20},
		{NodeID: "hot", IP: "10.0.0.2", CPUUsage: 95, MemoryUsage: 20},
	})

	got := map[string]string{}
	for _, n := range d.Snapshot() {
		got[n.NodeID] = n.Status
	}
	if got["healthy"] != "ok" {
		t.Errorf("healthy alias node = %q, want \"ok\"", got["healthy"])
	}
	if got["hot"] != "degraded" {
		t.Errorf("hot alias node = %q, want \"degraded\" (load heuristic must survive)", got["hot"])
	}
}

// TestR6_DownEdgeStopsSuppressingOriginViewers is the consequence pin: a dead edge
// must stop suppressing the origin's viewer counts, which is what made this a data
// defect rather than a cosmetic one.
func TestR6_DownEdgeStopsSuppressingOriginViewers(t *testing.T) {
	mock := &mockClusterClient{}
	sink := &recordingSink{}
	d := New(Config{PollInterval: 30 * time.Second, NodeID: "local"}, mock, sink, nil)

	// A live edge serving a stream suppresses origin viewer counts.
	mock.setNodes([]amsclient.ClusterNodeDTO{{
		ID: "edge-1", IP: "10.0.0.5", Role: "edge", CPU: "10", Memory: "20%",
		Status: "Running", LastUpdateTime: time.Now().UnixMilli(), ActiveStreamCount: 3,
	}})
	d.poll(context.Background())
	if !d.IsEdgeStream("any") {
		t.Fatal("precondition: a live edge with active streams should mark edge-served")
	}

	// AMS now reports the same edge as not running — still listed, so the
	// vanish-based sweep cannot help.
	mock.setNodes([]amsclient.ClusterNodeDTO{{
		ID: "edge-1", IP: "10.0.0.5", Role: "edge", CPU: "10", Memory: "20%",
		Status: "Not Running", LastUpdateTime: time.Now().UnixMilli(), ActiveStreamCount: 3,
	}})
	d.poll(context.Background())

	if d.IsEdgeStream("any") {
		t.Error("FAIL(R6): a dead edge still suppresses origin viewer counts — " +
			"viewers vanish from the dashboard for the whole outage")
	}
}
