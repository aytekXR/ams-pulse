// Tests for normalize.go — pinning correct AMS v2 field scales.
package collector

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// TestVD05_ViewerCountAccuracy_NonTautological verifies that NormalizeBroadcast
// produces the correct viewer_count sum by using *asymmetric* per-protocol
// counts that cannot be conflated. VD-05 noted the old budget test in
// run-budget-tests.sh was tautological (grep for the addition string, not a
// runtime check). This test checks the runtime output directly.
//
// Truth: hls=50, webrtc=30, rtmp=10, dash=5 → total=95
// The per-protocol map must also be correctly populated (no field is silently
// dropped or double-counted). Using values that are all distinct so a wrong
// sum (e.g. missing dash, or summing hls twice) produces a wrong answer.
func TestVD05_ViewerCountAccuracy_NonTautological(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID:          "s1",
		AppName:           "live",
		Status:            "broadcasting",
		HlsViewerCount:    50,
		WebRTCViewerCount: 30,
		RTMPViewerCount:   10,
		DashViewerCount:   5,
	}
	events := NormalizeBroadcast(dto, "node-x", "broadcasting", NoopGeoResolver{}, NoopUAParser{})

	var statsEv *domain.ServerEvent
	for i := range events {
		if events[i].Type == domain.EventStreamStats {
			statsEv = &events[i]
			break
		}
	}
	if statsEv == nil {
		t.Fatal("VD-05: NormalizeBroadcast emitted no EventStreamStats; cannot check viewer_count")
	}

	// viewer_count must be the runtime sum, not an inlined grep target.
	const wantTotal = 95 // 50+30+10+5
	got, ok := statsEv.Data["viewer_count"].(int)
	if !ok {
		t.Fatalf("VD-05: viewer_count type = %T, want int", statsEv.Data["viewer_count"])
	}
	if got != wantTotal {
		t.Errorf("VD-05: viewer_count = %d, want %d (hls=50 webrtc=30 rtmp=10 dash=5)", got, wantTotal)
	}

	// Verify the per-protocol breakdown is also correct.
	byProto, ok := statsEv.Data["viewer_count_by_protocol"].(map[string]any)
	if !ok {
		t.Fatalf("VD-05: viewer_count_by_protocol missing or wrong type")
	}
	checks := map[string]int{"hls": 50, "webrtc": 30, "rtmp": 10, "dash": 5}
	for proto, wantV := range checks {
		if v, ok2 := byProto[proto].(int); !ok2 || v != wantV {
			t.Errorf("VD-05: viewer_count_by_protocol[%s] = %v, want %d", proto, byProto[proto], wantV)
		}
	}
	t.Logf("PASS VD-05: viewer_count=%d (hls=%v webrtc=%v rtmp=%v dash=%v)",
		got, byProto["hls"], byProto["webrtc"], byProto["rtmp"], byProto["dash"])
}

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

// ─── FIX 1 — VD-40: node version propagated through Data map ─────────────────

// TestNormalizeClusterNode_VersionInData verifies that NormalizeClusterNode
// writes ClusterNodeDTO.Version into Data["version"] so that
// aggregator.onNodeStats can read it and populate LiveNodeStats.Version.
// Before the fix, Data["version"] was always "" because the field was decoded
// but never written into the event.
func TestNormalizeClusterNode_VersionInData(t *testing.T) {
	dto := amsclient.ClusterNodeDTO{
		NodeID:  "node-42",
		Version: "2.10.3",
	}
	ev := NormalizeClusterNode(dto)

	got, ok := ev.Data["version"].(string)
	if !ok {
		t.Fatalf("VD-40: Data[\"version\"] not a string, got %T %v", ev.Data["version"], ev.Data["version"])
	}
	if got != "2.10.3" {
		t.Errorf("VD-40: Data[\"version\"] = %q, want %q", got, "2.10.3")
	}
	t.Logf("PASS VD-40: Data[\"version\"] = %q", got)
}

// ─── FIX 2 — v2.10 speed-only bitrate fallback ───────────────────────────────

// TestNormalizeBroadcast_SpeedFallback verifies that when BitRate==0 and
// Speed>0 (AMS v2.10 behavior), stream_stats uses Speed as bitrate_kbps and
// ingest_stats is still emitted (condition was `b.BitRate > 0` before fix).
func TestNormalizeBroadcast_SpeedFallback(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "speed-stream",
		AppName:  "live",
		Status:   "broadcasting",
		BitRate:  0,
		Speed:    2000.0,
	}

	events := NormalizeBroadcast(dto, "node-1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})

	var statsEv, ingestEv *domain.ServerEvent
	for i := range events {
		switch events[i].Type {
		case domain.EventStreamStats:
			statsEv = &events[i]
		case domain.EventIngestStats:
			ingestEv = &events[i]
		}
	}

	if statsEv == nil {
		t.Fatal("FIX2: no stream_stats event emitted for speed-only DTO")
	}
	gotBitrate, ok := statsEv.Data["bitrate_kbps"].(float64)
	if !ok {
		t.Fatalf("FIX2: stream_stats bitrate_kbps type = %T, want float64", statsEv.Data["bitrate_kbps"])
	}
	if gotBitrate != 2000.0 {
		t.Errorf("FIX2: stream_stats bitrate_kbps = %.0f, want 2000 (Speed fallback)", gotBitrate)
	}

	if ingestEv == nil {
		t.Error("FIX2: no ingest_stats event emitted — emit condition must use effectiveBitrate not b.BitRate")
	} else {
		gotIngestBitrate, ok2 := ingestEv.Data["bitrate_kbps"].(float64)
		if !ok2 {
			t.Fatalf("FIX2: ingest_stats bitrate_kbps type = %T, want float64", ingestEv.Data["bitrate_kbps"])
		}
		if gotIngestBitrate != 2000.0 {
			t.Errorf("FIX2: ingest_stats bitrate_kbps = %.0f, want 2000", gotIngestBitrate)
		}
	}
	t.Logf("PASS FIX2: stream_stats bitrate=%.0f ingest_stats emitted=%v", gotBitrate, ingestEv != nil)
}

// ─── FIX 3 — empty StreamID guard ────────────────────────────────────────────

// TestNormalizeBroadcast_EmptyStreamID verifies that NormalizeBroadcast returns
// no events when StreamID is "". An empty key would silently corrupt the
// aggregator's live stream map by writing all such streams under key nodeID+"/".
func TestNormalizeBroadcast_EmptyStreamID(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "",
		AppName:  "live",
		Status:   "broadcasting",
		BitRate:  1000.0,
	}

	events := NormalizeBroadcast(dto, "node-1", "", NoopGeoResolver{}, NoopUAParser{})
	if len(events) != 0 {
		t.Errorf("FIX3: expected 0 events for empty StreamID, got %d", len(events))
	}
	t.Log("PASS FIX3: no events emitted for empty StreamID")
}

// TestNormalizeBroadcast_CreatedStatus verifies that a DTO with status "created"
// (stream registered but not yet live) emits no events.
func TestNormalizeBroadcast_CreatedStatus(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "s-created",
		AppName:  "live",
		Status:   "created",
	}
	events := NormalizeBroadcast(dto, "node-1", "", NoopGeoResolver{}, NoopUAParser{})
	if len(events) != 0 {
		t.Errorf("expected 0 events for status=created, got %d", len(events))
	}
}

// TestNormalizeBroadcast_FinishedAfterBroadcasting verifies that status
// "finished" when prevStatus was "broadcasting" emits a stream_publish_end.
func TestNormalizeBroadcast_FinishedAfterBroadcasting(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "s-end",
		AppName:  "live",
		Status:   "finished",
	}
	events := NormalizeBroadcast(dto, "node-1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})
	if len(events) != 1 {
		t.Fatalf("expected 1 stream_publish_end event, got %d", len(events))
	}
	if events[0].Type != domain.EventStreamPublishEnd {
		t.Errorf("event type = %q, want stream_publish_end", events[0].Type)
	}
}

// TestNormalizeBroadcast_EndedAfterBroadcasting verifies that status
// "ended" when prevStatus was "broadcasting" also emits stream_publish_end.
func TestNormalizeBroadcast_EndedAfterBroadcasting(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "s-ended",
		AppName:  "live",
		Status:   "ended",
	}
	events := NormalizeBroadcast(dto, "node-1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})
	if len(events) != 1 {
		t.Fatalf("expected 1 stream_publish_end event, got %d", len(events))
	}
	if events[0].Type != domain.EventStreamPublishEnd {
		t.Errorf("event type = %q, want stream_publish_end", events[0].Type)
	}
}

// ─── WebRTC averaging: zero/absent fields ────────────────────────────────────

// TestNormalizeWebRTCStats_ZeroFields verifies that NormalizeWebRTCStats
// correctly averages to zero when all peer-stat fields are zero/absent.
func TestNormalizeWebRTCStats_ZeroFields(t *testing.T) {
	dto := amsclient.WebRTCClientStatsDTO{
		StatID:               "client-1",
		VideoRoundTripTime:   0,
		AudioRoundTripTime:   0,
		VideoJitter:          0,
		AudioJitter:          0,
		VideoPacketLostRatio: 0,
		AudioPacketLostRatio: 0,
	}
	ev := NormalizeWebRTCStats(dto, "live", "s1", "node-1")
	if ev.Data["rtt_ms"].(float64) != 0 {
		t.Errorf("rtt_ms = %v, want 0", ev.Data["rtt_ms"])
	}
	if ev.Data["jitter_ms"].(float64) != 0 {
		t.Errorf("jitter_ms = %v, want 0", ev.Data["jitter_ms"])
	}
	if ev.Data["packet_loss_pct"].(float64) != 0 {
		t.Errorf("packet_loss_pct = %v, want 0", ev.Data["packet_loss_pct"])
	}
}

// TestNormalizeWebRTCStats_Averaging verifies the (video+audio)/2 averaging
// for rtt, jitter, and packet_loss when fields are non-zero.
func TestNormalizeWebRTCStats_Averaging(t *testing.T) {
	dto := amsclient.WebRTCClientStatsDTO{
		StatID:               "client-2",
		VideoRoundTripTime:   0.02,  // 20ms in seconds
		AudioRoundTripTime:   0.04,  // 40ms in seconds → avg = 30ms → *1000 = 30
		VideoJitter:          0.005, // 5ms in seconds
		AudioJitter:          0.015, // 15ms in seconds → avg = 10ms → *1000 = 10
		VideoPacketLostRatio: 0.01,  // 1%
		AudioPacketLostRatio: 0.03,  // 3% → avg = 2% → *100 = 2
	}
	ev := NormalizeWebRTCStats(dto, "live", "s1", "node-1")

	if got := ev.Data["rtt_ms"].(float64); got != 30.0 {
		t.Errorf("rtt_ms = %.1f, want 30.0", got)
	}
	if got := ev.Data["jitter_ms"].(float64); got != 10.0 {
		t.Errorf("jitter_ms = %.1f, want 10.0", got)
	}
	if got := ev.Data["packet_loss_pct"].(float64); got != 2.0 {
		t.Errorf("packet_loss_pct = %.1f, want 2.0", got)
	}
	t.Logf("PASS WebRTC averaging: rtt=%.0f jitter=%.0f loss=%.0f",
		ev.Data["rtt_ms"], ev.Data["jitter_ms"], ev.Data["packet_loss_pct"])
}
