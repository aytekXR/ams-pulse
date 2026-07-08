// normalize_realcapture_test.go — fixture-replay regression suite.
//
// These tests exercise the FULL pipeline:
//
//	json.Unmarshal(realCaptureBytes, &[]amsclient.BroadcastDTO{}) → NormalizeBroadcast() → assert pinned outputs
//
// Unlike the synthetic struct-literal tests in normalize_test.go, the replay
// tests catch field-name changes in the DTO struct, tolerance of null/absent
// fields in real JSON, and any future edits to the /1000 bitrate constant.
//
// Fixture files live in server/pkg/amsclient/testdata/ (the repo convention for
// collector-package tests; see restpoller/standalone_node_stats_test.go:29-43).
// The path relative to server/internal/collector/ is ../../pkg/amsclient/testdata/.
//
// FIXTURE PROVENANCE:
//   - broadcasts_real_test123_v303.json  — real AMS 3.0.3 Enterprise capture, 2026-06-21,
//     sanitized (IP → TEST-NET-3). Single-stream snapshot: bitrate=624016 bps.
//   - broadcasts_real_liveapp.json       — same server, different poll moment: 16 entries,
//     test123 has bitrate=622312 bps (distinct from the v303 fixture — both are real).
//   - system_status.json                 — GET /rest/v2/system-status, AMS 3.0.3 Enterprise.
//   - webrtc_client_stats_real_empty.json — GET /test123/webrtc-client-stats: [] (no viewers
//     at capture time; real-capture path for WebRTC is the empty-array case).
//   - webrtc_client_stats.json           — SYNTHETIC fixture (2 peers); used here because
//     the real capture is an empty array and no real non-empty WebRTC capture was taken.
//
// D-029 / D-031 semantics pinned:
//   - bps→kbps: bitrateKbps = b.BitRate / 1000.0 (line 79 of normalize.go)
//   - FPS-redistribution: when AMS 3.0.3 omits currentFPS, fps key is ABSENT from ingest_stats
//   - terminated_unexpectedly: maps to EventStreamPublishEnd with Data["reason"]=b.Status
//   - WebRTC single-track: avgNonZero averages only non-zero tracks
package collector

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// loadCaptureFixture reads a JSON file from server/pkg/amsclient/testdata/.
// Path is relative to the server/internal/collector/ package directory.
// Missing fixture → t.Fatal (never t.Skip — a skipped test is a false green, D-028).
func loadCaptureFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := "../../pkg/amsclient/testdata/" + name
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("loadCaptureFixture: fixture %q not found at %s: %v", name, p, err)
	}
	return data
}

// ─── Test 1: single-stream real capture, AMS 3.0.3 ──────────────────────────

// TestReplay_RealAMS303_Test123_Broadcasting replays the real AMS 3.0.3 capture
// for test123 (bitrate=624016 bps) through the full decode→normalize pipeline and
// pins all hardcoded expected values derived from the fixture bytes.
//
// D-029v bps→kbps sentinel: bitrate 624016 bps → 624.016 kbps.
// This non-round value is distinctive enough to catch any wrong divisor.
//
// D-031 FPS-redistribution sentinel: AMS 3.0.3 REST omits currentFPS entirely;
// it decodes to 0 in Go. normalize.go:122-124 must NOT emit the fps key —
// downstream ComputeHealthScore redistributes the FPS weight rather than scoring
// a phantom 0 fps (which would pin every stream to "Warning").
func TestReplay_RealAMS303_Test123_Broadcasting(t *testing.T) {
	raw := loadCaptureFixture(t, "broadcasts_real_test123_v303.json")

	var dtos []amsclient.BroadcastDTO
	if err := json.Unmarshal(raw, &dtos); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("fixture decode: want 1 DTO, got %d", len(dtos))
	}
	dto := dtos[0]

	// Verify raw decode before normalizing — ensures JSON tags are correct.
	if dto.StreamID != "test123" {
		t.Fatalf("decode: StreamID=%q, want %q", dto.StreamID, "test123")
	}
	if dto.BitRate != 624016 {
		t.Fatalf("decode: BitRate=%.0f, want 624016", dto.BitRate)
	}
	if dto.Speed != 0.991 {
		t.Errorf("decode: Speed=%.3f, want 0.991", dto.Speed)
	}
	if dto.CurrentFPS != 0 {
		t.Errorf("decode: CurrentFPS=%d, want 0 (AMS 3.0.3 REST omits currentFPS → zero)", dto.CurrentFPS)
	}
	if dto.Status != "broadcasting" {
		t.Errorf("decode: Status=%q, want broadcasting", dto.Status)
	}
	if dto.Type != "streamSource" {
		t.Errorf("decode: Type=%q, want streamSource", dto.Type)
	}
	if dto.PublishType != "" {
		t.Errorf("decode: PublishType=%q, want empty (null in JSON)", dto.PublishType)
	}

	// prevStatus="" simulates first poll — this stream has never been seen before.
	events := NormalizeBroadcast(dto, "node-ams303", "", NoopGeoResolver{}, NoopUAParser{})

	// First poll, broadcasting: expect publish_start + stream_stats + ingest_stats.
	if len(events) != 3 {
		t.Fatalf("NormalizeBroadcast: want 3 events (publish_start+stream_stats+ingest_stats), got %d: %v", len(events), events)
	}

	var publishStart, streamStats, ingestStats *domain.ServerEvent
	for i := range events {
		switch events[i].Type {
		case domain.EventStreamPublishStart:
			publishStart = &events[i]
		case domain.EventStreamStats:
			streamStats = &events[i]
		case domain.EventIngestStats:
			ingestStats = &events[i]
		}
	}
	if publishStart == nil {
		t.Fatal("no stream_publish_start event")
	}
	if streamStats == nil {
		t.Fatal("no stream_stats event")
	}
	if ingestStats == nil {
		t.Fatal("no ingest_stats event")
	}

	// ── publish_start assertions ──────────────────────────────────────────────
	pt, ok := publishStart.Data["publish_type"].(string)
	if !ok {
		t.Fatalf("publish_start.Data[publish_type]: type=%T, want string", publishStart.Data["publish_type"])
	}
	// type=streamSource + publishType=null/"" → normalizePublishType falls through to "other".
	if pt != "other" {
		t.Errorf("publish_type=%q, want %q (streamSource + null publishType → other)", pt, "other")
	}
	if publishStart.App != "live" {
		// AppName is absent in the raw JSON (only set by ListBroadcasts path, not Unmarshal);
		// the app="" → "live" fallback at normalize.go:47-49 must fire.
		t.Errorf("publish_start.App=%q, want %q (AppName absent → fallback to live)", publishStart.App, "live")
	}
	if publishStart.StreamID != "test123" {
		t.Errorf("publish_start.StreamID=%q, want %q", publishStart.StreamID, "test123")
	}

	// ── stream_stats assertions ───────────────────────────────────────────────
	// D-029v bps→kbps: bitrate=624016 bps → 624.016 kbps.
	// Hardcoded sentinel: 624016 / 1000.0 = 624.016 (non-round, mutation-detectable).
	const wantBitrateKbps = 624.016
	gotBitrateKbps, ok := streamStats.Data["bitrate_kbps"].(float64)
	if !ok {
		t.Fatalf("stream_stats.Data[bitrate_kbps]: type=%T, want float64", streamStats.Data["bitrate_kbps"])
	}
	if math.Abs(gotBitrateKbps-wantBitrateKbps) > 0.001 {
		t.Errorf("stream_stats.bitrate_kbps=%.6f, want %.3f (bps→kbps via /1000.0)", gotBitrateKbps, wantBitrateKbps)
	}

	gotSpeed, ok := streamStats.Data["speed_read_kbps"].(float64)
	if !ok {
		t.Fatalf("stream_stats.Data[speed_read_kbps]: type=%T, want float64", streamStats.Data["speed_read_kbps"])
	}
	// AMS "speed" is a realtime ratio (≈1.0), stored verbatim. Not converted to kbps.
	if math.Abs(gotSpeed-0.991) > 0.0001 {
		t.Errorf("stream_stats.speed_read_kbps=%.3f, want 0.991 (verbatim AMS ratio)", gotSpeed)
	}

	gotViewers, ok := streamStats.Data["viewer_count"].(int)
	if !ok {
		t.Fatalf("stream_stats.Data[viewer_count]: type=%T, want int", streamStats.Data["viewer_count"])
	}
	if gotViewers != 0 {
		t.Errorf("stream_stats.viewer_count=%d, want 0 (all viewer counts are 0 in capture)", gotViewers)
	}

	// ── ingest_stats assertions ───────────────────────────────────────────────
	gotIngestBitrate, ok := ingestStats.Data["bitrate_kbps"].(float64)
	if !ok {
		t.Fatalf("ingest_stats.Data[bitrate_kbps]: type=%T, want float64", ingestStats.Data["bitrate_kbps"])
	}
	if math.Abs(gotIngestBitrate-wantBitrateKbps) > 0.001 {
		t.Errorf("ingest_stats.bitrate_kbps=%.6f, want %.3f", gotIngestBitrate, wantBitrateKbps)
	}

	gotLoss, ok := ingestStats.Data["packet_loss_pct"].(float64)
	if !ok {
		t.Fatalf("ingest_stats.Data[packet_loss_pct]: type=%T, want float64", ingestStats.Data["packet_loss_pct"])
	}
	if gotLoss != 0.0 {
		t.Errorf("ingest_stats.packet_loss_pct=%.4f, want 0.0 (packetLostRatio=0.0 in capture)", gotLoss)
	}

	gotJitter, ok := ingestStats.Data["jitter_ms"].(float64)
	if !ok {
		t.Fatalf("ingest_stats.Data[jitter_ms]: type=%T, want float64", ingestStats.Data["jitter_ms"])
	}
	if gotJitter != 0.0 {
		t.Errorf("ingest_stats.jitter_ms=%.4f, want 0.0 (jitterMs=0 in capture, already ms)", gotJitter)
	}

	gotRtt, ok := ingestStats.Data["rtt_ms"].(float64)
	if !ok {
		t.Fatalf("ingest_stats.Data[rtt_ms]: type=%T, want float64", ingestStats.Data["rtt_ms"])
	}
	if gotRtt != 0.0 {
		t.Errorf("ingest_stats.rtt_ms=%.4f, want 0.0 (rttMs=0 in capture, already ms)", gotRtt)
	}

	// D-031 FPS-redistribution: AMS 3.0.3 omits currentFPS → decodes to 0 →
	// normalize.go:122-124 must NOT emit fps key.
	if _, fpPresent := ingestStats.Data["fps"]; fpPresent {
		t.Errorf("ingest_stats must NOT carry fps key when AMS 3.0.3 omits currentFPS; got %v", ingestStats.Data["fps"])
	}

	t.Logf("PASS TestReplay_RealAMS303_Test123_Broadcasting: bitrate_kbps=%.3f speed=%.3f viewer_count=%d fps_key_absent=true",
		gotBitrateKbps, gotSpeed, gotViewers)
}

// ─── Test 2: full 16-entry live-app list, two distinct bitrate values ────────

// TestReplay_RealLiveApp_FullList replays the 16-entry real AMS 3.0.3 capture.
// The test123 entry has bitrate=622312 bps → 622.312 kbps.
// This is DISTINCT from the 624.016 kbps in broadcasts_real_test123_v303.json
// because the two captures were taken at different poll moments — two independent
// wire measurements of the same stream at different times.
//
// The 15 finished streams with prevStatus="" emit 0 events each (never seen live).
func TestReplay_RealLiveApp_FullList(t *testing.T) {
	raw := loadCaptureFixture(t, "broadcasts_real_liveapp.json")

	var dtos []amsclient.BroadcastDTO
	if err := json.Unmarshal(raw, &dtos); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(dtos) != 16 {
		t.Fatalf("fixture decode: want 16 DTOs, got %d", len(dtos))
	}

	// All 15 finished streams with prevStatus="" must emit 0 events.
	var finishedCount int
	for _, dto := range dtos {
		if dto.Status == "finished" {
			finishedCount++
			events := NormalizeBroadcast(dto, "node-ams303", "", NoopGeoResolver{}, NoopUAParser{})
			if len(events) != 0 {
				t.Errorf("finished stream %q (prevStatus=): want 0 events, got %d", dto.StreamID, len(events))
			}
		}
	}
	if finishedCount != 15 {
		t.Errorf("expected 15 finished streams in fixture, got %d", finishedCount)
	}

	// The one broadcasting stream (test123, bitrate=622312 bps) with prevStatus=""
	// must emit 3 events (publish_start + stream_stats + ingest_stats).
	var broadcastingCount int
	for _, dto := range dtos {
		if dto.Status != "broadcasting" {
			continue
		}
		broadcastingCount++

		if dto.StreamID != "test123" {
			t.Errorf("expected broadcasting stream to be test123, got %q", dto.StreamID)
		}
		if dto.BitRate != 622312 {
			t.Errorf("decode: BitRate=%.0f, want 622312 (this fixture, different poll from v303)", dto.BitRate)
		}

		events := NormalizeBroadcast(dto, "node-ams303", "", NoopGeoResolver{}, NoopUAParser{})
		if len(events) != 3 {
			t.Fatalf("broadcasting test123 (prevStatus=): want 3 events, got %d", len(events))
		}

		var streamStats *domain.ServerEvent
		for i := range events {
			if events[i].Type == domain.EventStreamStats {
				streamStats = &events[i]
			}
		}
		if streamStats == nil {
			t.Fatal("no stream_stats event for broadcasting test123")
		}

		// D-029v: bitrate=622312 bps → 622.312 kbps.
		// DISTINCT from 624.016 kbps (the v303 fixture) — proves fixtures are independent.
		const wantKbps = 622.312
		gotKbps, ok := streamStats.Data["bitrate_kbps"].(float64)
		if !ok {
			t.Fatalf("stream_stats.Data[bitrate_kbps]: type=%T", streamStats.Data["bitrate_kbps"])
		}
		if math.Abs(gotKbps-wantKbps) > 0.001 {
			t.Errorf("stream_stats.bitrate_kbps=%.6f, want %.3f (bitrate=622312 bps /1000 → 622.312 kbps)", gotKbps, wantKbps)
		}

		t.Logf("PASS liveapp test123: bitrate_kbps=%.3f (different from v303 fixture 624.016)", gotKbps)
	}
	if broadcastingCount != 1 {
		t.Errorf("expected 1 broadcasting stream in fixture, got %d", broadcastingCount)
	}
}

// ─── Test 3: finished stream prevStatus behavior ──────────────────────────────

// TestReplay_RealLiveApp_FinishedStreamPrevStatus exercises the
// terminated_unexpectedly/finished mapping (D-029v) via a real finished stream.
//
// First entry in broadcasts_real_liveapp.json: streamId=0c3869f8-..., status=finished.
// prevStatus="" → 0 events (never seen live, so no end event).
// prevStatus="broadcasting" → 1 event: stream_publish_end with reason="finished".
func TestReplay_RealLiveApp_FinishedStreamPrevStatus(t *testing.T) {
	raw := loadCaptureFixture(t, "broadcasts_real_liveapp.json")

	var dtos []amsclient.BroadcastDTO
	if err := json.Unmarshal(raw, &dtos); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(dtos) == 0 {
		t.Fatal("fixture is empty")
	}

	// First entry must be a finished stream.
	dto := dtos[0]
	if dto.Status != "finished" {
		t.Fatalf("expected first fixture entry to have status=finished, got %q", dto.Status)
	}
	if dto.StreamID != "0c3869f8-41ef-45a2-b2e5-4fd8ecd9d6a0" {
		t.Fatalf("unexpected first-entry streamId=%q", dto.StreamID)
	}

	// Case A: prevStatus="" — stream was never seen live; no end event.
	eventsA := NormalizeBroadcast(dto, "node-ams303", "", NoopGeoResolver{}, NoopUAParser{})
	if len(eventsA) != 0 {
		t.Errorf("finished stream (prevStatus=): want 0 events, got %d", len(eventsA))
	}

	// Case B: prevStatus="broadcasting" — stream was live; emit publish_end.
	eventsB := NormalizeBroadcast(dto, "node-ams303", "broadcasting", NoopGeoResolver{}, NoopUAParser{})
	if len(eventsB) != 1 {
		t.Fatalf("finished stream (prevStatus=broadcasting): want 1 event, got %d", len(eventsB))
	}
	ev := eventsB[0]
	if ev.Type != domain.EventStreamPublishEnd {
		t.Errorf("event type=%q, want stream_publish_end", ev.Type)
	}
	reason, ok := ev.Data["reason"].(string)
	if !ok {
		t.Fatalf("publish_end.Data[reason]: type=%T, want string", ev.Data["reason"])
	}
	if reason != "finished" {
		t.Errorf("publish_end.reason=%q, want %q", reason, "finished")
	}

	t.Logf("PASS TestReplay_RealLiveApp_FinishedStreamPrevStatus: caseA=0events caseB=publish_end(reason=finished)")
}

// ─── Test 4: real system-status fixture through NormalizeSystemStats ──────────

// TestReplay_RealSystemStatus_NormalizeFields loads the real AMS 3.0.3 system-status
// capture and asserts the exact field values emitted by NormalizeSystemStats.
//
// Real AMS 3.x shape: ONLY {osName, osArch, javaVersion, processorCount}.
// cpu_pct, mem_pct, disk_pct keys must be ABSENT (honest reporting — these fields
// do not exist in the real AMS 3.x system-status response).
func TestReplay_RealSystemStatus_NormalizeFields(t *testing.T) {
	raw := loadCaptureFixture(t, "system_status.json")

	var stats map[string]any
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("json.Unmarshal system_status.json: %v", err)
	}

	ev := NormalizeSystemStats(stats, "node-ams303", "3.0.3")

	if ev.Type != domain.EventNodeStats {
		t.Errorf("Type=%q, want %q", ev.Type, domain.EventNodeStats)
	}
	if ev.NodeID != "node-ams303" {
		t.Errorf("NodeID=%q, want %q", ev.NodeID, "node-ams303")
	}

	// Real capture values: osName=Linux, osArch=amd64, javaVersion=17, processorCount=8.
	if got, ok := ev.Data["os_name"].(string); !ok || got != "Linux" {
		t.Errorf("os_name=%v, want %q", ev.Data["os_name"], "Linux")
	}
	if got, ok := ev.Data["os_arch"].(string); !ok || got != "amd64" {
		t.Errorf("os_arch=%v, want %q", ev.Data["os_arch"], "amd64")
	}
	if got, ok := ev.Data["java_version"].(string); !ok || got != "17" {
		t.Errorf("java_version=%v, want %q", ev.Data["java_version"], "17")
	}
	// processorCount arrives as float64 from JSON; normalize.go:215-220 converts to int.
	if got, ok := ev.Data["processor_count"].(int); !ok || got != 8 {
		t.Errorf("processor_count=%v (%T), want int(8)", ev.Data["processor_count"], ev.Data["processor_count"])
	}
	if got, ok := ev.Data["version"].(string); !ok || got != "3.0.3" {
		t.Errorf("version=%v, want %q", ev.Data["version"], "3.0.3")
	}

	// CRITICAL: cpu_pct, mem_pct, disk_pct must be ABSENT (not zero-filled).
	for _, key := range []string{"cpu_pct", "mem_pct", "disk_pct", "net_in_mbps", "net_out_mbps"} {
		if v, exists := ev.Data[key]; exists {
			t.Errorf("Data[%q] must be ABSENT (not in real AMS 3.x system-status); got %v", key, v)
		}
	}

	t.Logf("PASS TestReplay_RealSystemStatus_NormalizeFields: os=%s/%s java=%s cores=%d version=%s",
		ev.Data["os_name"], ev.Data["os_arch"], ev.Data["java_version"],
		ev.Data["processor_count"], ev.Data["version"])
}

// ─── Test 5: real WebRTC capture — empty array, no panic ─────────────────────

// TestReplay_RealWebRTC_EmptyArray exercises the real AMS 3.0.3 WebRTC capture
// for test123. At capture time there were no WebRTC viewers, so the response is
// an empty JSON array []. This test verifies:
//  1. The decode succeeds (no error, no panic).
//  2. The result is a zero-length slice (no phantom viewer events).
//
// NOTE: Because the real capture is always empty, the non-trivial WebRTC stats
// (single-track averaging, unit conversions) can only be tested via the synthetic
// webrtc_client_stats.json fixture — see TestReplay_WebRTC_SyntheticFixture below.
func TestReplay_RealWebRTC_EmptyArray(t *testing.T) {
	raw := loadCaptureFixture(t, "webrtc_client_stats_real_empty.json")

	var peers []amsclient.WebRTCClientStatsDTO
	if err := json.Unmarshal(raw, &peers); err != nil {
		t.Fatalf("json.Unmarshal webrtc_client_stats_real_empty.json: %v", err)
	}
	if len(peers) != 0 {
		t.Fatalf("real WebRTC capture is an empty array; got %d entries", len(peers))
	}

	// Zero peers → no NormalizeWebRTCStats calls → no events (trivial but documented).
	t.Log("PASS TestReplay_RealWebRTC_EmptyArray: empty array decoded without error, len=0")
}

// ─── Test 6: synthetic WebRTC fixture — single-track averaging via JSON decode ─

// TestReplay_WebRTC_SyntheticFixture loads webrtc_client_stats.json (synthetic,
// 2 peers) through json.Unmarshal → NormalizeWebRTCStats and pins all hardcoded
// expected values. This exercises the decode→normalize pipeline for WebRTC even
// though no real non-empty capture exists.
//
// peer-abc123 has BOTH video and audio tracks non-zero.
// peer-def456 has all fields absent/zero (decodes to all-zero DTO).
//
// D-029v WebRTC unit conversions pinned:
//   - rtt: AMS reports in seconds → ×1000 = ms
//   - jitter: AMS reports in seconds → ×1000 = ms
//   - packet_loss: AMS reports 0..1 fraction → ×100 = percent
//
// D-031 avgNonZero sentinel: both tracks non-zero → average; the single-track
// (one track zero) guarantee is separately covered by TestNormalizeWebRTCStats_SingleTrackNotHalved.
func TestReplay_WebRTC_SyntheticFixture(t *testing.T) {
	raw := loadCaptureFixture(t, "webrtc_client_stats.json")

	var peers []amsclient.WebRTCClientStatsDTO
	if err := json.Unmarshal(raw, &peers); err != nil {
		t.Fatalf("json.Unmarshal webrtc_client_stats.json: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("fixture decode: want 2 peers, got %d", len(peers))
	}

	// ── peer-abc123: both video+audio tracks non-zero ─────────────────────────
	p0 := peers[0]
	if p0.StatID != "peer-abc123" {
		t.Fatalf("peers[0].StatID=%q, want peer-abc123", p0.StatID)
	}
	// Verify raw decode of key fields before normalizing.
	if math.Abs(p0.VideoRoundTripTime-0.045) > 1e-9 {
		t.Errorf("peers[0].VideoRoundTripTime=%.6f, want 0.045", p0.VideoRoundTripTime)
	}
	if math.Abs(p0.AudioRoundTripTime-0.043) > 1e-9 {
		t.Errorf("peers[0].AudioRoundTripTime=%.6f, want 0.043", p0.AudioRoundTripTime)
	}

	ev0 := NormalizeWebRTCStats(p0, "LiveApp", "test123", "node-ams303")
	if ev0.Type != domain.EventWebRTCClientStats {
		t.Errorf("ev0.Type=%q, want %q", ev0.Type, domain.EventWebRTCClientStats)
	}

	// rtt: avgNonZero(0.045, 0.043) = (0.045+0.043)/2 = 0.044 → ×1000 = 44.0 ms
	const wantRtt0 = 44.0
	if got := ev0.Data["rtt_ms"].(float64); math.Abs(got-wantRtt0) > 0.001 {
		t.Errorf("peers[0] rtt_ms=%.4f, want %.1f", got, wantRtt0)
	}

	// jitter: avgNonZero(0.002, 0.001) = 0.0015 → ×1000 = 1.5 ms
	const wantJitter0 = 1.5
	if got := ev0.Data["jitter_ms"].(float64); math.Abs(got-wantJitter0) > 0.001 {
		t.Errorf("peers[0] jitter_ms=%.4f, want %.1f", got, wantJitter0)
	}

	// loss: avgNonZero(0.005, 0.003) = 0.004 → ×100 = 0.4%
	const wantLoss0 = 0.4
	if got := ev0.Data["packet_loss_pct"].(float64); math.Abs(got-wantLoss0) > 0.001 {
		t.Errorf("peers[0] packet_loss_pct=%.4f, want %.1f", got, wantLoss0)
	}

	// ── peer-def456: all fields absent → zero ─────────────────────────────────
	p1 := peers[1]
	if p1.StatID != "peer-def456" {
		t.Fatalf("peers[1].StatID=%q, want peer-def456", p1.StatID)
	}

	ev1 := NormalizeWebRTCStats(p1, "LiveApp", "test123", "node-ams303")
	if got := ev1.Data["rtt_ms"].(float64); got != 0.0 {
		t.Errorf("peers[1] rtt_ms=%.4f, want 0.0 (all fields absent → zero)", got)
	}
	if got := ev1.Data["jitter_ms"].(float64); got != 0.0 {
		t.Errorf("peers[1] jitter_ms=%.4f, want 0.0", got)
	}
	if got := ev1.Data["packet_loss_pct"].(float64); got != 0.0 {
		t.Errorf("peers[1] packet_loss_pct=%.4f, want 0.0", got)
	}

	t.Logf("PASS TestReplay_WebRTC_SyntheticFixture: peer0 rtt=%.1f jitter=%.1f loss=%.1f | peer1 all-zero",
		ev0.Data["rtt_ms"], ev0.Data["jitter_ms"], ev0.Data["packet_loss_pct"])
}

// ─── Test 7a: terminated_unexpectedly mapping ─────────────────────────────────

// TestReplay_TerminatedUnexpectedly_Mapping pins the terminated_unexpectedly case
// in normalize.go:138. This status is a real AMS 3.0.3 condition for crashed/dropped
// ingests; the case clause must include it alongside "finished" and "ended".
//
// No real capture has this status (all captures were taken while the stream was
// live or had cleanly finished). A synthetic DTO is used to exercise the branch
// directly — the mapping semantics are constant regardless of wire source.
//
// Mutation sentinel: removing "terminated_unexpectedly" from the case clause must
// cause this test to fail (0 events instead of 1).
func TestReplay_TerminatedUnexpectedly_Mapping(t *testing.T) {
	dto := amsclient.BroadcastDTO{
		StreamID: "stream-crash",
		Status:   "terminated_unexpectedly",
		AppName:  "LiveApp",
	}

	// Case A: prevStatus="" — never seen live; no end event.
	eventsA := NormalizeBroadcast(dto, "node-ams303", "", NoopGeoResolver{}, NoopUAParser{})
	if len(eventsA) != 0 {
		t.Errorf("terminated_unexpectedly (prevStatus=): want 0 events, got %d", len(eventsA))
	}

	// Case B: prevStatus="broadcasting" — was live, now crashed; emit publish_end.
	eventsB := NormalizeBroadcast(dto, "node-ams303", "broadcasting", NoopGeoResolver{}, NoopUAParser{})
	if len(eventsB) != 1 {
		t.Fatalf("terminated_unexpectedly (prevStatus=broadcasting): want 1 event, got %d; "+
			"if 0: the case clause is missing terminated_unexpectedly", len(eventsB))
	}
	ev := eventsB[0]
	if ev.Type != domain.EventStreamPublishEnd {
		t.Errorf("event type=%q, want stream_publish_end", ev.Type)
	}
	reason, ok := ev.Data["reason"].(string)
	if !ok {
		t.Fatalf("publish_end.Data[reason]: type=%T, want string", ev.Data["reason"])
	}
	// Reason must be verbatim "terminated_unexpectedly" — not "finished" or anything else.
	// This distinguishes crashed vs normal end for downstream dashboards.
	if reason != "terminated_unexpectedly" {
		t.Errorf("publish_end.reason=%q, want %q", reason, "terminated_unexpectedly")
	}

	t.Logf("PASS TestReplay_TerminatedUnexpectedly_Mapping: caseA=0events caseB=publish_end(reason=terminated_unexpectedly)")
}

// ─── Test 7: app-name fallback exercised via raw unmarshal ───────────────────

// TestReplay_AppNameFallback_LiveFallback verifies that when AppName is absent
// in the JSON (as it always is from raw unmarshal — AppName is only populated
// by the ListBroadcasts caller at client.go:450-453), normalize.go:47-49 falls
// back to "live" for all emitted events.
//
// This exercises the app="" → "live" branch that is otherwise invisible when
// tests supply AppName directly.
func TestReplay_AppNameFallback_LiveFallback(t *testing.T) {
	raw := loadCaptureFixture(t, "broadcasts_real_test123_v303.json")

	var dtos []amsclient.BroadcastDTO
	if err := json.Unmarshal(raw, &dtos); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("want 1 DTO, got %d", len(dtos))
	}

	dto := dtos[0]
	// Confirm AppName is indeed absent from the raw JSON (decodes to "").
	// If the fixture somehow has AppName populated, the fallback branch is not
	// exercised — this is a fixture integrity failure, not a skip condition (D-028).
	if dto.AppName != "" {
		t.Fatalf("AppName=%q in fixture; expected empty from raw unmarshal (fixture may have changed)", dto.AppName)
	}

	events := NormalizeBroadcast(dto, "node-ams303", "", NoopGeoResolver{}, NoopUAParser{})
	for _, ev := range events {
		if ev.App != "live" {
			t.Errorf("event type=%q: App=%q, want %q (AppName absent → fallback to live)", ev.Type, ev.App, "live")
		}
	}

	t.Log("PASS TestReplay_AppNameFallback_LiveFallback: all events carry App=live when AppName absent in JSON")
}
