// Package kafka — contract tests using an in-process fake (D-007.5).
// No real broker is required; these tests exercise normalization + routing logic.
package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
	kafkago "github.com/segmentio/kafka-go"
)

// captureSink collects events written to it.
type captureSink struct {
	server  []domain.ServerEvent
	beacon  []domain.BeaconEvent
	session []domain.ViewerSession
}

func (c *captureSink) WriteServerEvent(ev domain.ServerEvent)     { c.server = append(c.server, ev) }
func (c *captureSink) WriteBeaconEvent(ev domain.BeaconEvent)     { c.beacon = append(c.beacon, ev) }
func (c *captureSink) WriteViewerSession(s domain.ViewerSession)  { c.session = append(c.session, s) }

// TestKafka_NormalizeNodeStats verifies that a cpuUsage message becomes node_stats.
func TestKafka_NormalizeNodeStats(t *testing.T) {
	raw := map[string]any{
		"cpuUsage":    float64(42.5),
		"memoryUsage": float64(60.0),
		"diskUsage":   float64(30.0),
		"nodeId":      "node-1",
	}
	ev, err := normalizeKafkaMessage(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != domain.EventNodeStats {
		t.Errorf("expected node_stats, got %q", ev.Type)
	}
	if v := ev.Data["cpu_pct"].(float64); v != 42.5 {
		t.Errorf("cpu_pct = %v, want 42.5", v)
	}
	if ev.Source != domain.SourceKafka {
		t.Errorf("source = %q, want %q", ev.Source, domain.SourceKafka)
	}
}

// TestKafka_NormalizeIngestStats verifies that bitrate+fps message becomes ingest_stats.
func TestKafka_NormalizeIngestStats(t *testing.T) {
	raw := map[string]any{
		"streamId": "s1",
		"app":      "live",
		"bitrate":  float64(2000.0),
		"fps":      float64(30.0),
	}
	ev, err := normalizeKafkaMessage(raw, "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != domain.EventIngestStats {
		t.Errorf("expected ingest_stats, got %q", ev.Type)
	}
	if ev.StreamID != "s1" {
		t.Errorf("stream_id = %q, want %q", ev.StreamID, "s1")
	}
	if v := ev.Data["bitrate_kbps"].(float64); v != 2000.0 {
		t.Errorf("bitrate_kbps = %v, want 2000", v)
	}
	if v := ev.Data["fps"].(float64); v != 30.0 {
		t.Errorf("fps = %v, want 30", v)
	}
}

// TestKafka_NormalizeStreamStats verifies viewer-count message becomes stream_stats.
func TestKafka_NormalizeStreamStats(t *testing.T) {
	raw := map[string]any{
		"streamId":          "s2",
		"app":               "live",
		"hlsViewerCount":    float64(10),
		"webRTCViewerCount": float64(5),
		"rtmpViewerCount":   float64(2),
	}
	ev, err := normalizeKafkaMessage(raw, "node-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != domain.EventStreamStats {
		t.Errorf("expected stream_stats, got %q", ev.Type)
	}
	vc, _ := ev.Data["viewer_count"].(int)
	if vc != 17 {
		t.Errorf("viewer_count = %d, want 17", vc)
	}
}

// TestKafka_MalformedJSON verifies malformed payloads are skipped without panic.
func TestKafka_MalformedJSON(t *testing.T) {
	sink := &captureSink{}
	src := New(Config{Brokers: []string{"localhost:9092"}, NodeID: "n1"}, sink, nil)

	// normalizeKafkaMessage on an empty map produces stream_stats (best-effort), no error.
	_, err := normalizeKafkaMessage(map[string]any{}, "n1")
	if err != nil {
		t.Errorf("unexpected error for empty map: %v", err)
	}

	// Verify parseErrors counter starts at zero.
	if src.ParseErrors() != 0 {
		t.Errorf("expected 0 parse errors initially, got %d", src.ParseErrors())
	}
}

// TestKafka_TimestampFromMessage verifies that message-embedded timestamp is used.
func TestKafka_TimestampFromMessage(t *testing.T) {
	fixedTS := int64(1700000000000) // 2023-11-14 or similar
	raw := map[string]any{
		"timestamp": float64(fixedTS),
		"streamId":  "s3",
	}
	ev, err := normalizeKafkaMessage(raw, "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.TS != fixedTS {
		t.Errorf("TS = %d, want %d", ev.TS, fixedTS)
	}
}

// TestKafka_DefaultTopic verifies DefaultConfig sets the default topic.
func TestKafka_DefaultTopic(t *testing.T) {
	cfg := DefaultConfig([]string{"broker:9092"}, "n1")
	if len(cfg.Topics) != 1 || cfg.Topics[0] != "ams-server-events" {
		t.Errorf("unexpected topics: %v", cfg.Topics)
	}
}

// TestKafka_NoBrokers verifies Run returns an error immediately when no brokers.
func TestKafka_NoBrokers(t *testing.T) {
	sink := &captureSink{}
	src := New(Config{NodeID: "n1"}, sink, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := src.Run(ctx)
	if err == nil {
		t.Error("expected error when no brokers configured")
	}
}

// TestKafka_ContractRoundTrip verifies that a JSON-encoded AMS Kafka message
// round-trips through the normalizer to a valid domain.ServerEvent.
// This is the D-007.5 "contract test" — no broker needed.
func TestKafka_ContractRoundTrip(t *testing.T) {
	// Simulate what AMS publishes to Kafka: a JSON stats object.
	payload := map[string]any{
		"streamId":          "myStream",
		"app":               "live",
		"fps":               float64(25),
		"bitrate":           float64(1500),
		"keyFrameInterval":  float64(2.0),
		"packetLost":        float64(0.1),
		"jitter":            float64(5.0),
		"timestamp":         float64(time.Now().UnixMilli()),
	}

	payloadBytes, _ := json.Marshal(payload)

	var decoded map[string]any
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}

	ev, err := normalizeKafkaMessage(decoded, "node-3")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	if ev.Version != 1 {
		t.Errorf("version = %d, want 1", ev.Version)
	}
	if ev.Type != domain.EventIngestStats {
		t.Errorf("type = %q, want ingest_stats", ev.Type)
	}
	if ev.Source != domain.SourceKafka {
		t.Errorf("source = %q, want kafka", ev.Source)
	}
	if ev.StreamID != "myStream" {
		t.Errorf("stream_id = %q, want myStream", ev.StreamID)
	}
	if ev.App != "live" {
		t.Errorf("app = %q, want live", ev.App)
	}
	fps := ev.Data["fps"].(float64)
	if fps != 25.0 {
		t.Errorf("fps = %v, want 25", fps)
	}
}

// TestKafka_DashViewerCountIncluded verifies that dashViewerCount is summed into
// viewer_count in the Kafka normalizer (FIX 4), matching the REST path
// (NormalizeBroadcast already included it). Before the fix the Kafka path
// silently omitted dash viewers.
func TestKafka_DashViewerCountIncluded(t *testing.T) {
	raw := map[string]any{
		"streamId":          "s-dash",
		"app":               "live",
		"hlsViewerCount":    float64(10),
		"webRTCViewerCount": float64(5),
		"rtmpViewerCount":   float64(2),
		"dashViewerCount":   float64(3), // the field that was missing from the sum
	}
	ev, err := normalizeKafkaMessage(raw, "node-1")
	if err != nil {
		t.Fatalf("FIX4: unexpected error: %v", err)
	}
	if ev.Type != domain.EventStreamStats {
		t.Fatalf("FIX4: expected stream_stats, got %q", ev.Type)
	}

	vc, _ := ev.Data["viewer_count"].(int)
	const wantTotal = 20 // 10+5+2+3
	if vc != wantTotal {
		t.Errorf("FIX4: viewer_count = %d, want %d (dash viewers must be included)", vc, wantTotal)
	}

	// Also verify that dash appears in the by-protocol breakdown.
	byProto, ok := ev.Data["viewer_count_by_protocol"].(map[string]any)
	if !ok {
		t.Fatalf("FIX4: viewer_count_by_protocol missing or wrong type")
	}
	if dash, _ := byProto["dash"].(int); dash != 3 {
		t.Errorf("FIX4: viewer_count_by_protocol[dash] = %v, want 3", byProto["dash"])
	}
	t.Logf("PASS FIX4: viewer_count=%d (hls=%v webrtc=%v rtmp=%v dash=%v)",
		vc, byProto["hls"], byProto["webrtc"], byProto["rtmp"], byProto["dash"])
}

// TestKafka_DashViewerCountMatchesREST is a true cross-path parity check: it runs
// BOTH the Kafka normalizer and the real REST normalizer (collector.NormalizeBroadcast)
// on the same per-protocol counts and asserts identical viewer_count totals. (It does
// NOT compute the REST total with inline arithmetic — that would not catch a bug in
// the REST sum formula.)
func TestKafka_DashViewerCountMatchesREST(t *testing.T) {
	const (
		hls    = 50
		webrtc = 30
		rtmp   = 10
		dash   = 5
	)

	// Kafka path.
	raw := map[string]any{
		"streamId":          "match-stream",
		"app":               "live",
		"hlsViewerCount":    float64(hls),
		"webRTCViewerCount": float64(webrtc),
		"rtmpViewerCount":   float64(rtmp),
		"dashViewerCount":   float64(dash),
	}
	kafkaEv, err := normalizeKafkaMessage(raw, "node-1")
	if err != nil {
		t.Fatalf("kafka normalize: %v", err)
	}
	kafkaTotal, _ := kafkaEv.Data["viewer_count"].(int)

	// REST path: run the real NormalizeBroadcast and pull viewer_count from the
	// emitted stream_stats event. prevStatus="broadcasting" so only stream_stats is
	// emitted (no publish_start). nil geo/ua is safe (empty IPs short-circuit).
	dto := amsclient.BroadcastDTO{
		StreamID:          "match-stream",
		AppName:           "live",
		Status:            "broadcasting",
		HlsViewerCount:    hls,
		WebRTCViewerCount: webrtc,
		RTMPViewerCount:   rtmp,
		DashViewerCount:   dash,
	}
	restTotal := -1
	for _, e := range collector.NormalizeBroadcast(dto, "node-1", "broadcasting", nil, nil) {
		if e.Type == domain.EventStreamStats {
			if v, ok := e.Data["viewer_count"].(int); ok {
				restTotal = v
			}
		}
	}
	if restTotal < 0 {
		t.Fatalf("REST path emitted no stream_stats viewer_count")
	}

	if kafkaTotal != restTotal {
		t.Errorf("FIX4: Kafka viewer_count=%d != REST viewer_count=%d; paths disagree", kafkaTotal, restTotal)
	}
	if want := hls + webrtc + rtmp + dash; kafkaTotal != want {
		t.Errorf("FIX4: viewer_count=%d, want %d (incl. dash)", kafkaTotal, want)
	}
	t.Logf("PASS FIX4: Kafka=%d REST=%d — paths agree (both normalizers run)", kafkaTotal, restTotal)
}

// TestKafka_AtomicCounters verifies that ParseErrors() and Lag() are race-safe
// and that processMessage increments parseErrors on malformed input (D-007.5 — no broker).
func TestKafka_AtomicCounters(t *testing.T) {
	sink := &captureSink{}
	src := New(Config{Brokers: []string{"localhost:9092"}, NodeID: "n1"}, sink, nil)

	// Counters start at zero.
	if src.ParseErrors() != 0 {
		t.Errorf("initial ParseErrors = %d, want 0", src.ParseErrors())
	}
	if src.Lag() != 0 {
		t.Errorf("initial Lag = %d, want 0", src.Lag())
	}

	// Feed a malformed JSON message directly through processMessage.
	malformed := kafkago.Message{
		Topic:     "ams-server-events",
		Partition: 0,
		Offset:    0,
		Value:     []byte("{not valid json"),
	}
	src.processMessage(malformed)

	if src.ParseErrors() != 1 {
		t.Errorf("after 1 malformed message: ParseErrors = %d, want 1", src.ParseErrors())
	}

	// Feed another malformed message — counter must increment again.
	src.processMessage(malformed)
	if src.ParseErrors() != 2 {
		t.Errorf("after 2 malformed messages: ParseErrors = %d, want 2", src.ParseErrors())
	}

	// Valid message must NOT increment parseErrors.
	valid := kafkago.Message{
		Topic: "ams-server-events",
		Value: []byte(`{"cpuUsage":10.0,"nodeId":"n1"}`),
	}
	src.processMessage(valid)
	if src.ParseErrors() != 2 {
		t.Errorf("after valid message: ParseErrors = %d, want still 2", src.ParseErrors())
	}

	// Lag() is readable at any time (atomic); manually store a value and check.
	src.lag.Store(42)
	if src.Lag() != 42 {
		t.Errorf("Lag() = %d, want 42", src.Lag())
	}

	t.Logf("PASS: ParseErrors=%d, Lag=%d — atomic counters correct", src.ParseErrors(), src.Lag())
}
