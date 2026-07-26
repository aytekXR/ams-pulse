// review_mp3_r1_test.go — REVIEW-MP3 round-3 regression pin R1.
//
// On a CLUSTER, the failure-streak event must be keyed to the REAL node IDs, not
// to the configured cfg.NodeID.
//
// Why the pre-existing streak tests could not catch this: they run a
// single-identity fixture where the configured ID and the emitted ID are the same
// string, so an event addressed to cfg.NodeID always matched. Post-N2, cluster
// success events carry AMS's real node IDs while the failure path still stamped
// cfg.NodeID (default "standalone"). The aggregator's D-087 contract drops
// api_unreachable events for unknown keys ("failure events create nothing"), so
// ConsecAPIErrors stayed pinned at 0 for every real node and the node_degraded
// ladder — rung 3 of the AMS early-warning design — never fired for the entire
// duration of an AMS API outage. That is precisely the outage it exists to catch.
package restpoller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/domain"
)

// TestR1_ClusterFailureStreak_UsesRealNodeIDs is the discriminating pin.
func TestR1_ClusterFailureStreak_UsesRealNodeIDs(t *testing.T) {
	var failNow atomic.Bool
	const nodeA, nodeB = "ams-node-alpha", "ams-node-beta"

	srv := clusterAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if failNow.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
			{"id": nodeA, "ip": "10.0.0.1", "cpu": "20.0", "memory": "40.0%", "status": "Running"},
			{"id": nodeB, "ip": "10.0.0.2", "cpu": "25.0", "memory": "45.0%", "status": "Running"},
		})
	})
	defer srv.Close()

	// makePoller configures NodeID "streak-test-node" — an identity NO cluster
	// node carries. That mismatch is the whole point of this test.
	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Phase 1: let the cluster path succeed so the real node IDs are learned.
	success := waitNodeStats(sink, 2, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return !u
	}, 3*time.Second)
	if len(success) < 2 {
		t.Fatalf("precondition: want 2 cluster success events, got %d", len(success))
	}

	// Phase 2: AMS starts failing.
	clearEvents(sink)
	failNow.Store(true)

	failures := waitNodeStats(sink, 2, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return u
	}, 3*time.Second)
	if len(failures) < 2 {
		t.Fatalf("want >=2 failure-streak events (one per cluster node), got %d", len(failures))
	}

	got := map[string]bool{}
	for _, ev := range failures {
		got[ev.NodeID] = true
	}

	for _, want := range []string{nodeA, nodeB} {
		if !got[want] {
			t.Errorf("FAIL(R1): no failure-streak event addressed to real cluster node %q; "+
				"got node IDs %v. The aggregator drops api_unreachable events for unknown "+
				"keys, so this node's ConsecAPIErrors never advances and node_degraded "+
				"cannot fire during the outage.", want, keysOf(got))
		}
	}
	if got["streak-test-node"] {
		t.Errorf("FAIL(R1): failure-streak event addressed to the CONFIGURED node ID "+
			"%q — no cluster node carries that identity post-N2, so the event is a "+
			"silent no-op at the aggregator", "streak-test-node")
	}
}

// TestR1_StandaloneFailureStreak_StillUsesConfiguredID guards the other
// direction: on the standalone path AMS supplies no node ID, so cfg.NodeID
// remains correct and must not regress to something else.
func TestR1_StandaloneFailureStreak_StillUsesConfiguredID(t *testing.T) {
	srv := standaloneAMSServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	events := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return u
	}, 3*time.Second)
	if len(events) == 0 {
		t.Fatal("no failure-streak event from standalone failure")
	}
	if events[0].NodeID != "streak-test-node" {
		t.Errorf("standalone failure-streak NodeID = %q, want the configured %q",
			events[0].NodeID, "streak-test-node")
	}
}

// TestR1_EmptyClusterDropsStaleNodeIDs pins the cleanup half: once AMS answers
// on the standalone path, the remembered cluster IDs are forgotten, so a later
// failure is not addressed to nodes that no longer exist.
//
// The transition is driven through the EMPTY-node-list route (nodes==0, err==nil
// → standalone SystemStats fallback) rather than by flipping cluster-mode-status,
// because the probe owns the mode cache by design (REVIEW-MP3 N1) and only
// re-probes periodically — a cache flip would make this test measure the cache
// TTL instead of the cleanup behavior.
func TestR1_EmptyClusterDropsStaleNodeIDs(t *testing.T) {
	const (
		phaseCluster    = 0 // cluster serves one node
		phaseStandalone = 1 // cluster list empty; system-status healthy
		phaseFailing    = 2 // cluster list empty; system-status failing
	)
	var phase atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{"applications": []any{}}) //nolint:errcheck
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"success": true}) //nolint:errcheck
		case r.URL.Path == "/rest/v2/system-status":
			if phase.Load() == phaseFailing {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"osName": "Linux"}) //nolint:errcheck
		case r.URL.Path == "/rest/v2/version":
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/"):
			w.Header().Set("Content-Type", "application/json")
			if phase.Load() == phaseCluster {
				json.NewEncoder(w).Encode([]map[string]any{ //nolint:errcheck
					{"id": "gone-node", "ip": "10.0.0.1", "cpu": "20.0", "memory": "40.0%", "status": "Running"},
				})
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, sink := makePoller(t, srv.URL)
	_, cancel := runPoller(p)
	defer cancel()

	// Phase 1: learn the cluster node.
	if got := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		return ev.NodeID == "gone-node"
	}, 3*time.Second); len(got) == 0 {
		t.Fatal("precondition: cluster node never discovered")
	}

	// Phase 2: the cluster empties out and AMS answers as standalone. This is the
	// poll that must forget the remembered IDs.
	clearEvents(sink)
	phase.Store(phaseStandalone)
	if got := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return !u && ev.NodeID == "streak-test-node"
	}, 3*time.Second); len(got) == 0 {
		t.Fatal("precondition: standalone success event never observed")
	}

	// Phase 3: AMS starts failing.
	clearEvents(sink)
	phase.Store(phaseFailing)

	failures := waitNodeStats(sink, 1, func(ev domain.ServerEvent) bool {
		u, _ := ev.Data["api_unreachable"].(bool)
		return u
	}, 3*time.Second)
	if len(failures) == 0 {
		t.Fatal("no failure-streak event after the cluster emptied out")
	}
	for _, ev := range failures {
		if ev.NodeID == "gone-node" {
			t.Errorf("failure-streak still addressed to %q after the cluster emptied — "+
				"the remembered ID set was never cleared", ev.NodeID)
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
