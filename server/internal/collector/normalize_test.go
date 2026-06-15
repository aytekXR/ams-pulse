// Tests for normalize.go — pinning correct AMS v2 field scales.
package collector

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// TestNormalizeBroadcast_EmitsIngestStats verifies that NormalizeBroadcast
// emits an EventIngestStats event when BroadcastDTO has FPS or bitrate (VD-22).
// Before the fix, REST-only deployments always had fps=0 and FPS=0 in ClickHouse
// because EventIngestStats was only produced by Kafka and log-tail sources.
func TestNormalizeBroadcast_EmitsIngestStats(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID:   "test-stream",
		AppName:    "live",
		Status:     "broadcasting",
		BitRate:    2500.0,
		CurrentFPS: 30,
	}

	events := NormalizeBroadcast(dto, "node-1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})

	var ingestStats []domain.ServerEvent
	for _, ev := range events {
		if ev.Type == domain.EventIngestStats {
			ingestStats = append(ingestStats, ev)
		}
	}

	if len(ingestStats) == 0 {
		t.Fatalf("NormalizeBroadcast emitted no EventIngestStats events (VD-22); want 1")
	}
	ev := ingestStats[0]
	if fps, ok := ev.Data["fps"].(float64); !ok || fps != 30.0 {
		t.Errorf("ingest_stats fps = %v, want 30.0 (VD-22)", ev.Data["fps"])
	}
	if bps, ok := ev.Data["bitrate_kbps"].(float64); !ok || bps != 2500.0 {
		t.Errorf("ingest_stats bitrate_kbps = %v, want 2500.0 (VD-22)", ev.Data["bitrate_kbps"])
	}
	t.Logf("PASS VD-22: EventIngestStats emitted with fps=%.0f bitrate=%.0f",
		ev.Data["fps"], ev.Data["bitrate_kbps"])
}

// TestNormalizeBroadcast_NoIngestStatsWhenZero verifies that EventIngestStats
// is NOT emitted when both FPS and BitRate are zero (no useful ingest data).
func TestNormalizeBroadcast_NoIngestStatsWhenZero(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID:   "test-stream",
		AppName:    "live",
		Status:     "broadcasting",
		BitRate:    0,
		CurrentFPS: 0,
	}

	events := NormalizeBroadcast(dto, "node-1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})

	for _, ev := range events {
		if ev.Type == domain.EventIngestStats {
			t.Errorf("unexpected EventIngestStats when FPS=0 and BitRate=0 (VD-22)")
		}
	}
}

// TestNormalizeClusterNode_CPUScale pins that cpuUsage=15.0 (AMS REST v2
// 0-100 percentage) maps to cpu_pct=15.0, NOT 1500.0.
// Regression guard for D-W1-001.
func TestNormalizeClusterNode_CPUScale(t *testing.T) {
	dto := amsclient.ClusterNodeDTO{
		NodeID:     "node-1",
		CPUUsage:   15.0,
		MemoryUsage: 40.0,
		DiskUsage:  55.0,
	}
	ev := NormalizeClusterNode(dto)

	data := ev.Data
	cpuPct, ok := data["cpu_pct"].(float64)
	if !ok {
		t.Fatalf("cpu_pct not a float64: %T %v", data["cpu_pct"], data["cpu_pct"])
	}
	if cpuPct != 15.0 {
		t.Errorf("cpu_pct = %.1f, want 15.0 (D-W1-001: AMS v2 cpuUsage is already 0-100, must not multiply by 100)", cpuPct)
	}

	memPct, ok := data["mem_pct"].(float64)
	if !ok {
		t.Fatalf("mem_pct not a float64: %T %v", data["mem_pct"], data["mem_pct"])
	}
	if memPct != 40.0 {
		t.Errorf("mem_pct = %.1f, want 40.0", memPct)
	}

	diskPct, ok := data["disk_pct"].(float64)
	if !ok {
		t.Fatalf("disk_pct not a float64: %T %v", data["disk_pct"], data["disk_pct"])
	}
	if diskPct != 55.0 {
		t.Errorf("disk_pct = %.1f, want 55.0", diskPct)
	}
}

// TestNormalizeClusterNode_NodeIDFallback verifies that when NodeID is empty
// the IP field is used as the node identifier.
func TestNormalizeClusterNode_NodeIDFallback(t *testing.T) {
	dto := amsclient.ClusterNodeDTO{
		NodeID: "",
		IP:     "10.0.0.1",
	}
	ev := NormalizeClusterNode(dto)
	if ev.NodeID != "10.0.0.1" {
		t.Errorf("NodeID = %q, want %q", ev.NodeID, "10.0.0.1")
	}
}

// TestNormalizeClusterNode_BoundaryValues ensures values at the 0 and 100
// boundaries are passed through unchanged.
func TestNormalizeClusterNode_BoundaryValues(t *testing.T) {
	cases := []struct {
		name    string
		cpu     float64
		mem     float64
		disk    float64
	}{
		{"zero", 0.0, 0.0, 0.0},
		{"max", 100.0, 100.0, 100.0},
		{"mid", 50.0, 75.0, 25.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dto := amsclient.ClusterNodeDTO{
				NodeID:      "n",
				CPUUsage:    tc.cpu,
				MemoryUsage: tc.mem,
				DiskUsage:   tc.disk,
			}
			ev := NormalizeClusterNode(dto)
			if got := ev.Data["cpu_pct"].(float64); got != tc.cpu {
				t.Errorf("cpu_pct = %.1f, want %.1f", got, tc.cpu)
			}
			if got := ev.Data["mem_pct"].(float64); got != tc.mem {
				t.Errorf("mem_pct = %.1f, want %.1f", got, tc.mem)
			}
			if got := ev.Data["disk_pct"].(float64); got != tc.disk {
				t.Errorf("disk_pct = %.1f, want %.1f", got, tc.disk)
			}
		})
	}
}
