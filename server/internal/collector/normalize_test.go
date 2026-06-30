// Tests for normalize.go — pinning correct AMS v2 field scales.
package collector

import (
	"math"
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
		BitRate:    2_500_000.0, // AMS reports bps; 2.5 Mbps → 2500 kbps after /1000
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
		NodeID:      "node-1",
		CPUUsage:    15.0,
		MemoryUsage: 40.0,
		DiskUsage:   55.0,
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
		name string
		cpu  float64
		mem  float64
		disk float64
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

// ─── D-029v — AMS "speed" is a realtime RATIO, never a bitrate ────────────────

// TestNormalizeBroadcast_SpeedIsNotBitrate is the regression guard for the
// removed v2.10 "speed fallback". Real AMS 3.0.3 wire data proved "speed" is a
// dimensionless realtime ratio (e.g. 0.991 for a 624 kbps stream, 1.236 for a
// 1381 kbps stream) — NOT a kbps bitrate. The old code set bitrate_kbps = Speed
// when BitRate==0, emitting ~1 "kbps" of garbage. After the fix, Speed must
// never leak into bitrate_kbps, and with BitRate==0 (and no fps) there is no
// useful ingest data so ingest_stats is NOT emitted.
func TestNormalizeBroadcast_SpeedIsNotBitrate(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "speed-stream",
		AppName:  "live",
		Status:   "broadcasting",
		BitRate:  0,
		Speed:    0.991, // realtime ratio — must NOT become a bitrate
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
		t.Fatal("D-029v: no stream_stats event emitted for broadcasting DTO")
	}
	if got := statsEv.Data["bitrate_kbps"].(float64); got != 0.0 {
		t.Errorf("D-029v: stream_stats bitrate_kbps = %v, want 0 (Speed must NOT be used as bitrate)", got)
	}
	if got := statsEv.Data["speed_read_kbps"].(float64); got != 0.991 {
		t.Errorf("D-029v: speed_read_kbps = %v, want 0.991 (speed preserved verbatim)", got)
	}
	if ingestEv != nil {
		t.Errorf("D-029v: ingest_stats must NOT be emitted when BitRate==0 and fps absent, got %+v", ingestEv.Data)
	}
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

// ─── D-029v — real AMS 3.0.3 wire regressions (live-capture-driven) ───────────

// TestNormalizeBroadcast_BitrateBpsToKbps is the headline regression for the
// 1000× bitrate bug. The live test123 stream reported bitrate=624016, which is
// BITS/sec (≈624 kbps, cross-checked against receivedBytes/duration). The old
// code stored it raw into bitrate_kbps. Both stream_stats and ingest_stats must
// now carry ~624, not 624016.
func TestNormalizeBroadcast_BitrateBpsToKbps(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "test123",
		AppName:  "LiveApp",
		Status:   "broadcasting",
		Type:     "streamSource",
		BitRate:  624016, // bps, from the real test.antmedia.io capture
		Speed:    0.991,
		// no CurrentFPS — AMS 3.0.3 REST omits it
	}
	events := NormalizeBroadcast(dto, "test-antmedia", "broadcasting", NoopGeoResolver{}, NoopUAParser{})

	var statsEv, ingestEv *domain.ServerEvent
	for i := range events {
		switch events[i].Type {
		case domain.EventStreamStats:
			statsEv = &events[i]
		case domain.EventIngestStats:
			ingestEv = &events[i]
		}
	}
	if statsEv == nil || ingestEv == nil {
		t.Fatalf("want both stream_stats and ingest_stats events; got stats=%v ingest=%v", statsEv != nil, ingestEv != nil)
	}
	const wantKbps = 624.016
	if got := statsEv.Data["bitrate_kbps"].(float64); math.Abs(got-wantKbps) > 0.001 {
		t.Errorf("stream_stats bitrate_kbps = %v, want ≈%.3f (bps→kbps; NOT 624016)", got, wantKbps)
	}
	if got := ingestEv.Data["bitrate_kbps"].(float64); math.Abs(got-wantKbps) > 0.001 {
		t.Errorf("ingest_stats bitrate_kbps = %v, want ≈%.3f", got, wantKbps)
	}
	// AMS 3.0.3 omits currentFPS → no "fps" key (so the scorer redistributes weight).
	if _, ok := ingestEv.Data["fps"]; ok {
		t.Errorf("ingest_stats must NOT carry an fps key when AMS omits currentFPS; got %v", ingestEv.Data["fps"])
	}
}

// TestNormalizeBroadcast_IngestQoEFieldsWired verifies the previously-dropped
// ingest-side QoE fields now flow through with correct units: packetLostRatio is
// a 0..1 fraction (×100 → percent), jitterMs/rttMs are already milliseconds.
func TestNormalizeBroadcast_IngestQoEFieldsWired(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID:        "qoe-stream",
		AppName:         "LiveApp",
		Status:          "broadcasting",
		BitRate:         1_000_000, // 1000 kbps
		PacketLostRatio: 0.05,      // fraction → 5.0%
		JitterMs:        12.0,      // ms
		RttMs:           8.0,       // ms
	}
	events := NormalizeBroadcast(dto, "n1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})

	var ingestEv *domain.ServerEvent
	for i := range events {
		if events[i].Type == domain.EventIngestStats {
			ingestEv = &events[i]
		}
	}
	if ingestEv == nil {
		t.Fatal("no ingest_stats event emitted")
	}
	if got := ingestEv.Data["packet_loss_pct"].(float64); got != 5.0 {
		t.Errorf("packet_loss_pct = %v, want 5.0 (0.05 ratio ×100)", got)
	}
	if got := ingestEv.Data["jitter_ms"].(float64); got != 12.0 {
		t.Errorf("jitter_ms = %v, want 12.0 (AMS jitterMs is already ms)", got)
	}
	if got := ingestEv.Data["rtt_ms"].(float64); got != 8.0 {
		t.Errorf("rtt_ms = %v, want 8.0 (AMS rttMs is already ms)", got)
	}
}

// TestNormalizeBroadcast_TerminatedUnexpectedly verifies that a crashed ingest
// (real AMS 3.0.3 status, curl-verified in the meet app) emits publish_end with
// the actual status as the reason — instead of staying "live" until stale
// eviction (~3 min).
func TestNormalizeBroadcast_TerminatedUnexpectedly(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "Tahir_J1KlsQ2QOf",
		AppName:  "meet",
		Status:   "terminated_unexpectedly",
	}
	events := NormalizeBroadcast(dto, "n1", "broadcasting", NoopGeoResolver{}, NoopUAParser{})
	if len(events) != 1 || events[0].Type != domain.EventStreamPublishEnd {
		t.Fatalf("want 1 stream_publish_end, got %d events (%v)", len(events), events)
	}
	if got := events[0].Data["reason"].(string); got != "terminated_unexpectedly" {
		t.Errorf("publish_end reason = %q, want %q", got, "terminated_unexpectedly")
	}
}

// TestNormalizeBroadcast_TerminatedUnexpectedlyNotLive verifies the end event is
// NOT emitted when we never saw the stream live (prevStatus != broadcasting) —
// avoids phantom end events for streams that were already dead on first poll.
func TestNormalizeBroadcast_TerminatedUnexpectedlyNotLive(t *testing.T) {
	dto := amsclient.BroadcastDTO{StreamID: "x", AppName: "meet", Status: "terminated_unexpectedly"}
	if events := NormalizeBroadcast(dto, "n1", "", NoopGeoResolver{}, NoopUAParser{}); len(events) != 0 {
		t.Errorf("want 0 events when never seen live, got %d", len(events))
	}
}

// TestNormalizeWebRTCStats_SingleTrackNotHalved verifies that an audio-only or
// video-only viewer (the absent track reports 0) is not halved by a constant /2.
func TestNormalizeWebRTCStats_SingleTrackNotHalved(t *testing.T) {
	// Video-only viewer: audio fields are 0.
	dto := amsclient.WebRTCClientStatsDTO{
		StatID:               "video-only",
		VideoRoundTripTime:   0.040, // 40 ms
		VideoJitter:          0.020, // 20 ms
		VideoPacketLostRatio: 0.04,  // 4%
	}
	ev := NormalizeWebRTCStats(dto, "live", "s1", "node-1")
	if got := ev.Data["rtt_ms"].(float64); got != 40.0 {
		t.Errorf("rtt_ms = %v, want 40.0 (single track must not be halved)", got)
	}
	if got := ev.Data["jitter_ms"].(float64); got != 20.0 {
		t.Errorf("jitter_ms = %v, want 20.0", got)
	}
	if got := ev.Data["packet_loss_pct"].(float64); got != 4.0 {
		t.Errorf("packet_loss_pct = %v, want 4.0", got)
	}
}

// ─── NormalizeSystemStats (real AMS 3.x shape) ───────────────────────────────
//
// Real AMS 3.x GET /rest/v2/system-status returns ONLY:
//   {osName, osArch, javaVersion, processorCount}
// There are NO cpu/mem/disk/network metrics. The old tests asserted fake-parsed
// cpuUsage/jvmMemoryUsage/systemMemoryInfo/fileSystemInfo fields that don't exist
// in production — those tests are replaced here with the honest real-shape tests.

// TestNormalizeSystemStats_RealShape verifies that NormalizeSystemStats correctly
// maps the real AMS 3.x system-status fields {osName, osArch, javaVersion, processorCount}
// and a separately-fetched version string, and does NOT emit cpu_pct/mem_pct/disk_pct.
func TestNormalizeSystemStats_RealShape(t *testing.T) {
	stats := map[string]any{
		"osName":         "Linux",
		"osArch":         "amd64",
		"javaVersion":    "17",
		"processorCount": float64(8), // JSON numbers decode as float64
	}

	ev := NormalizeSystemStats(stats, "node-standalone", "3.0.3")

	if ev.Type != domain.EventNodeStats {
		t.Errorf("Type = %q, want %q", ev.Type, domain.EventNodeStats)
	}
	if ev.NodeID != "node-standalone" {
		t.Errorf("NodeID = %q, want %q", ev.NodeID, "node-standalone")
	}

	// Real identity fields must be present.
	if got, ok := ev.Data["os_name"].(string); !ok || got != "Linux" {
		t.Errorf("os_name = %v, want %q", ev.Data["os_name"], "Linux")
	}
	if got, ok := ev.Data["os_arch"].(string); !ok || got != "amd64" {
		t.Errorf("os_arch = %v, want %q", ev.Data["os_arch"], "amd64")
	}
	if got, ok := ev.Data["java_version"].(string); !ok || got != "17" {
		t.Errorf("java_version = %v, want %q", ev.Data["java_version"], "17")
	}
	if got, ok := ev.Data["processor_count"].(int); !ok || got != 8 {
		t.Errorf("processor_count = %v (%T), want 8", ev.Data["processor_count"], ev.Data["processor_count"])
	}
	if got, ok := ev.Data["version"].(string); !ok || got != "3.0.3" {
		t.Errorf("version = %v, want %q", ev.Data["version"], "3.0.3")
	}

	// CRITICAL: cpu_pct / mem_pct / disk_pct / net_* must be ABSENT.
	// AMS 3.x system-status does not carry these metrics — emitting zeros is dishonest.
	for _, key := range []string{"cpu_pct", "mem_pct", "disk_pct", "net_in_mbps", "net_out_mbps", "jvm_heap_used_mb"} {
		if v, exists := ev.Data[key]; exists {
			t.Errorf("HONEST: Data[%q] must be absent (unavailable from AMS 3.x system-status); got %v", key, v)
		}
	}

	t.Logf("PASS NormalizeSystemStats real shape: os=%s/%s java=%s cores=%v version=%s",
		ev.Data["os_name"], ev.Data["os_arch"], ev.Data["java_version"],
		ev.Data["processor_count"], ev.Data["version"])
}

// TestNormalizeSystemStats_EmptyMap verifies that an empty stats map does not
// panic and returns a valid (empty-Data) EventNodeStats event.
func TestNormalizeSystemStats_EmptyMap(t *testing.T) {
	ev := NormalizeSystemStats(map[string]any{}, "standalone", "")
	if ev.Type != domain.EventNodeStats {
		t.Errorf("Type = %q, want %q", ev.Type, domain.EventNodeStats)
	}
	if ev.NodeID != "standalone" {
		t.Errorf("NodeID = %q, want %q", ev.NodeID, "standalone")
	}
	// With empty input and empty version, Data must be empty (no fabricated zeros).
	if len(ev.Data) != 0 {
		t.Errorf("empty map + empty version: Data must be empty, got %v", ev.Data)
	}
	t.Log("PASS: empty map produces empty-Data EventNodeStats without panic")
}

// TestNormalizeSystemStats_WrongTypes verifies that wrong/unexpected types in the
// stats map do not cause a panic; fields with wrong types are silently skipped.
func TestNormalizeSystemStats_WrongTypes(t *testing.T) {
	stats := map[string]any{
		"osName":         12345,   // wrong type — expect string
		"osArch":         nil,     // nil — skip
		"processorCount": "eight", // wrong type — expect float64/int
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NormalizeSystemStats panicked on wrong types: %v", r)
		}
	}()

	ev := NormalizeSystemStats(stats, "node-bad", "")
	if ev.Type != domain.EventNodeStats {
		t.Errorf("Type = %q, want EventNodeStats", ev.Type)
	}
	// Wrong-typed fields must be absent (not zero-filled).
	if _, ok := ev.Data["os_name"]; ok {
		t.Errorf("os_name must be absent when wrong type, got %v", ev.Data["os_name"])
	}
	if _, ok := ev.Data["processor_count"]; ok {
		t.Errorf("processor_count must be absent when wrong type, got %v", ev.Data["processor_count"])
	}
	t.Log("PASS: wrong types do not panic and are silently skipped")
}

// TestNormalizeSystemStats_VersionParam verifies that the version string from
// the separate /rest/v2/version call is correctly written to Data["version"].
func TestNormalizeSystemStats_VersionParam(t *testing.T) {
	ev := NormalizeSystemStats(map[string]any{"osName": "Linux"}, "n1", "3.0.3")
	if got, ok := ev.Data["version"].(string); !ok || got != "3.0.3" {
		t.Errorf("version = %v, want %q", ev.Data["version"], "3.0.3")
	}
	// Empty version → absent key.
	ev2 := NormalizeSystemStats(map[string]any{"osName": "Linux"}, "n1", "")
	if _, ok := ev2.Data["version"]; ok {
		t.Errorf("version key must be absent when version param is empty")
	}
}
