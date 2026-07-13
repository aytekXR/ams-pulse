// aggregator_presence_flags_test.go — D-088 TDD: presence flags for cpu/mem/disk.
//
// Pins (both RED before domain.LiveNodeStats gains CPUPCTReported/MemPCTReported/DiskPCTReported):
//
//	(a) Keys present  → flags true  (cluster AMS path).
//	(b) Keys absent   → flags false (standalone AMS 3.x path; normalize.go omits them).
//
// The flags drive the anomaly detector's UpdateBaselines and ComputeFlags guards
// so standalone nodes never feed zero-value observations into the Welford baseline.
package aggregator

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// TestAggregator_OnNodeStats_KeysPresent_FlagsTrue verifies that CPUPCTReported,
// MemPCTReported, and DiskPCTReported are all true when the corresponding keys
// are present in the event Data (cluster AMS path where normalize.go emits all three).
func TestAggregator_OnNodeStats_KeysPresent_FlagsTrue(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "node-cluster",
		Data: map[string]any{
			"cpu_pct":  45.0,
			"mem_pct":  60.0,
			"disk_pct": 30.0,
		},
	})

	snap := agg.CurrentSnapshot()
	n, ok := snap.Nodes["node-cluster"]
	if !ok {
		t.Fatal("node-cluster not found in snapshot after normal-path event")
	}
	if !n.CPUPCTReported {
		t.Error("CPUPCTReported must be true when cpu_pct key is present in event Data")
	}
	if !n.MemPCTReported {
		t.Error("MemPCTReported must be true when mem_pct key is present in event Data")
	}
	if !n.DiskPCTReported {
		t.Error("DiskPCTReported must be true when disk_pct key is present in event Data")
	}
	t.Logf("PASS: all three reported flags are true when keys are present")
}

// TestAggregator_OnNodeStats_KeysAbsent_FlagsFalse verifies that CPUPCTReported,
// MemPCTReported, and DiskPCTReported remain false when the corresponding keys are
// absent from the event Data. This matches the standalone AMS 3.x path where
// normalize.go honestly omits cpu_pct/mem_pct/disk_pct (they are not available
// in /rest/v2/system-status). Sending zero would poison the Welford baseline.
func TestAggregator_OnNodeStats_KeysAbsent_FlagsFalse(t *testing.T) {
	agg := New(3*time.Minute, nil, nil)

	// Standalone AMS path: only network metrics and version are present.
	agg.OnServerEvent(domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  "node-standalone",
		Data: map[string]any{
			"net_in_mbps":  1.5,
			"net_out_mbps": 2.0,
			"version":      "3.0.3",
			// cpu_pct, mem_pct, disk_pct intentionally absent.
		},
	})

	snap := agg.CurrentSnapshot()
	n, ok := snap.Nodes["node-standalone"]
	if !ok {
		t.Fatal("node-standalone not found in snapshot after standalone-path event")
	}
	if n.CPUPCTReported {
		t.Error("CPUPCTReported must be false when cpu_pct key is absent from event Data")
	}
	if n.MemPCTReported {
		t.Error("MemPCTReported must be false when mem_pct key is absent from event Data")
	}
	if n.DiskPCTReported {
		t.Error("DiskPCTReported must be false when disk_pct key is absent from event Data")
	}
	t.Logf("PASS: all three reported flags are false when keys are absent")
}
