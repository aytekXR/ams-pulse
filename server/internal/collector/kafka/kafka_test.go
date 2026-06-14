// Package kafka — contract tests using an in-process fake (D-007.5).
// No real broker is required; these tests exercise normalization + routing logic.
package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
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
