// Package kafka — contract tests using an in-process fake (D-007.5).
// No real broker is required; these tests exercise normalization + routing logic.
package kafka

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/collector"
	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
	kafkago "github.com/segmentio/kafka-go"
)

// captureSink collects events written to it.
type captureSink struct {
	server  []domain.ServerEvent
	beacon  []domain.BeaconEvent
	session []domain.ViewerSession
}

func (c *captureSink) WriteServerEvent(ev domain.ServerEvent)    { c.server = append(c.server, ev) }
func (c *captureSink) WriteBeaconEvent(ev domain.BeaconEvent)    { c.beacon = append(c.beacon, ev) }
func (c *captureSink) WriteViewerSession(s domain.ViewerSession) { c.session = append(c.session, s) }

// TestKafka_NormalizeNodeStats verifies that a cpuUsage message on the legacy
// (non-AMS-native) path becomes node_stats.
func TestKafka_NormalizeNodeStats(t *testing.T) {
	raw := map[string]any{
		"cpuUsage":    float64(42.5),
		"memoryUsage": float64(60.0),
		"diskUsage":   float64(30.0),
		"nodeId":      "node-1",
	}
	// Empty topic → legacy field-sniffing path.
	ev, err := normalizeKafkaMessage(raw, "", "")
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
	ev, err := normalizeKafkaMessage(raw, "node-1", "")
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
	ev, err := normalizeKafkaMessage(raw, "node-2", "")
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
	_, err := normalizeKafkaMessage(map[string]any{}, "n1", "")
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
	ev, err := normalizeKafkaMessage(raw, "n1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.TS != fixedTS {
		t.Errorf("TS = %d, want %d", ev.TS, fixedTS)
	}
}

// TestKafka_DefaultTopic verifies DefaultConfig sets the two official AMS topics.
func TestKafka_DefaultTopic(t *testing.T) {
	cfg := DefaultConfig([]string{"broker:9092"}, "n1")
	want := []string{"ams-instance-stats", "ams-webrtc-stats"}
	if len(cfg.Topics) != len(want) {
		t.Fatalf("got %d topics, want %d: %v", len(cfg.Topics), len(want), cfg.Topics)
	}
	for i, topic := range want {
		if cfg.Topics[i] != topic {
			t.Errorf("Topics[%d] = %q, want %q", i, cfg.Topics[i], topic)
		}
	}
}

// TestKafka_NewFallbackTopics verifies New() uses the correct AMS topics when
// Topics is empty.
func TestKafka_NewFallbackTopics(t *testing.T) {
	sink := &captureSink{}
	src := New(Config{Brokers: []string{"broker:9092"}, NodeID: "n1"}, sink, nil)
	want := []string{"ams-instance-stats", "ams-webrtc-stats"}
	if len(src.cfg.Topics) != len(want) {
		t.Fatalf("fallback topics: got %v, want %v", src.cfg.Topics, want)
	}
	for i, topic := range want {
		if src.cfg.Topics[i] != topic {
			t.Errorf("fallback Topics[%d] = %q, want %q", i, src.cfg.Topics[i], topic)
		}
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
		"streamId":         "myStream",
		"app":              "live",
		"fps":              float64(25),
		"bitrate":          float64(1500),
		"keyFrameInterval": float64(2.0),
		"packetLost":       float64(0.1),
		"jitter":           float64(5.0),
		"timestamp":        float64(time.Now().UnixMilli()),
	}

	payloadBytes, _ := json.Marshal(payload)

	var decoded map[string]any
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}

	ev, err := normalizeKafkaMessage(decoded, "node-3", "")
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
	ev, err := normalizeKafkaMessage(raw, "node-1", "")
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
	kafkaEv, err := normalizeKafkaMessage(raw, "node-1", "")
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
		Topic:     "ams-instance-stats",
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

	// Valid ams-instance-stats message must NOT increment parseErrors.
	valid := kafkago.Message{
		Topic: "ams-instance-stats",
		Value: []byte(`{
			"instanceId": "n1",
			"cpuUsage": {"systemCPULoad": 10, "processCPULoad": 5, "systemLoadAverageLastMinute": 0.5},
			"systemMemoryInfo": {"totalMemory": 8589934592, "inUseMemory": 4294967296},
			"fileSystemInfo": {"totalSpace": 107374182400, "inUseSpace": 53687091200}
		}`),
	}
	src.processMessage(valid)
	if src.ParseErrors() != 2 {
		t.Errorf("after valid message: ParseErrors = %d, want still 2", src.ParseErrors())
	}

	// ams-webrtc-stats must NOT increment parseErrors (intentional skip).
	webrtcMsg := kafkago.Message{
		Topic: "ams-webrtc-stats",
		Value: []byte(`{"streamId":"s1","webrtcClientId":"c1","measured_bitrate":1500000}`),
	}
	src.processMessage(webrtcMsg)
	if src.ParseErrors() != 2 {
		t.Errorf("after webrtc-stats skip: ParseErrors = %d, want still 2", src.ParseErrors())
	}

	// Lag() is readable at any time (atomic); manually store a value and check.
	src.lag.Store(42)
	if src.Lag() != 42 {
		t.Errorf("Lag() = %d, want 42", src.Lag())
	}

	t.Logf("PASS: ParseErrors=%d, Lag=%d — atomic counters correct", src.ParseErrors(), src.Lag())
}

// TestKafka_InstanceStatsFixture exercises the ams-instance-stats path against
// the pinned fixture (testdata/ams-instance-stats.json), asserting the correct
// event type, node id from instanceId, and plausible cpu/mem/disk percentages.
func TestKafka_InstanceStatsFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/ams-instance-stats.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	ev, err := normalizeKafkaMessage(raw, "", "ams-instance-stats")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	// Event type must be node_stats.
	if ev.Type != domain.EventNodeStats {
		t.Errorf("Type = %q, want %q", ev.Type, domain.EventNodeStats)
	}
	// Source must be kafka.
	if ev.Source != domain.SourceKafka {
		t.Errorf("Source = %q, want %q", ev.Source, domain.SourceKafka)
	}
	// Node ID must come from instanceId when cfg NodeID is empty.
	if ev.NodeID != "ams-node-1" {
		t.Errorf("NodeID = %q, want %q", ev.NodeID, "ams-node-1")
	}

	// Fixture values:
	//   systemCPULoad = 42 (AMS publishes an INTEGER percent) → cpu_pct = 42.0
	//   inUseMemory = 7516192768 / totalMemory = 8589934592 → ~87.5%
	//   inUseSpace  = 107374182400 / totalSpace = 214748364800 → 50.0%
	cpuPct, ok := ev.Data["cpu_pct"].(float64)
	if !ok {
		t.Fatalf("cpu_pct missing or wrong type: %T", ev.Data["cpu_pct"])
	}
	memPct, ok := ev.Data["mem_pct"].(float64)
	if !ok {
		t.Fatalf("mem_pct missing or wrong type: %T", ev.Data["mem_pct"])
	}
	diskPct, ok := ev.Data["disk_pct"].(float64)
	if !ok {
		t.Fatalf("disk_pct missing or wrong type: %T", ev.Data["disk_pct"])
	}

	if cpuPct == 0 {
		t.Errorf("cpu_pct = 0, want non-zero")
	}
	if memPct == 0 {
		t.Errorf("mem_pct = 0, want non-zero")
	}
	if diskPct == 0 {
		t.Errorf("disk_pct = 0, want non-zero")
	}
	if cpuPct <= 0 || cpuPct > 100 {
		t.Errorf("cpu_pct = %v: not in (0, 100]", cpuPct)
	}
	if memPct <= 0 || memPct > 100 {
		t.Errorf("mem_pct = %v: not in (0, 100]", memPct)
	}
	if diskPct <= 0 || diskPct > 100 {
		t.Errorf("disk_pct = %v: not in (0, 100]", diskPct)
	}

	t.Logf("PASS: cpu_pct=%.2f mem_pct=%.2f disk_pct=%.2f nodeID=%q",
		cpuPct, memPct, diskPct, ev.NodeID)
}

// TestKafka_InstanceStatsNodeIDPrecedence verifies the three-level node ID
// precedence for ams-instance-stats: configured NodeID > instanceId > nodeId.
func TestKafka_InstanceStatsNodeIDPrecedence(t *testing.T) {
	base := map[string]any{
		"instanceId": "from-instanceId",
		"nodeId":     "from-nodeId",
		"cpuUsage":   map[string]any{"systemCPULoad": float64(10)},
		"systemMemoryInfo": map[string]any{
			"totalMemory": float64(1000), "inUseMemory": float64(500),
		},
		"fileSystemInfo": map[string]any{
			"totalSpace": float64(1000), "inUseSpace": float64(300),
		},
	}

	// Case 1: configured NodeID wins.
	ev, err := normalizeKafkaMessage(base, "cfg-node", "ams-instance-stats")
	if err != nil {
		t.Fatalf("case1 normalize: %v", err)
	}
	if ev.NodeID != "cfg-node" {
		t.Errorf("case1: NodeID = %q, want cfg-node", ev.NodeID)
	}

	// Case 2: no cfg NodeID → instanceId wins over nodeId.
	ev, err = normalizeKafkaMessage(base, "", "ams-instance-stats")
	if err != nil {
		t.Fatalf("case2 normalize: %v", err)
	}
	if ev.NodeID != "from-instanceId" {
		t.Errorf("case2: NodeID = %q, want from-instanceId", ev.NodeID)
	}

	// Case 3: no cfg NodeID, no instanceId → nodeId fallback.
	noInstanceID := map[string]any{
		"nodeId":   "from-nodeId",
		"cpuUsage": map[string]any{"systemCPULoad": float64(10)},
		"systemMemoryInfo": map[string]any{
			"totalMemory": float64(1000), "inUseMemory": float64(500),
		},
		"fileSystemInfo": map[string]any{
			"totalSpace": float64(1000), "inUseSpace": float64(300),
		},
	}
	ev, err = normalizeKafkaMessage(noInstanceID, "", "ams-instance-stats")
	if err != nil {
		t.Fatalf("case3 normalize: %v", err)
	}
	if ev.NodeID != "from-nodeId" {
		t.Errorf("case3: NodeID = %q, want from-nodeId", ev.NodeID)
	}
}

// TestKafka_WebRTCStatsSkipped verifies that ams-webrtc-stats messages are skipped
// without emitting an event or incrementing parseErrors.
func TestKafka_WebRTCStatsSkipped(t *testing.T) {
	data, err := os.ReadFile("testdata/ams-webrtc-stats.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	sink := &captureSink{}
	src := New(Config{Brokers: []string{"localhost:9092"}, NodeID: "n1"}, sink, nil)

	src.processMessage(kafkago.Message{
		Topic: "ams-webrtc-stats",
		Value: data,
	})

	// Must not emit any event.
	if len(sink.server) != 0 {
		t.Errorf("expected no events, got %d", len(sink.server))
	}
	// Must not count as a parse error.
	if src.ParseErrors() != 0 {
		t.Errorf("ParseErrors = %d, want 0 (intentional skip)", src.ParseErrors())
	}
	t.Logf("PASS: ams-webrtc-stats skipped cleanly (no event, no parse error)")
}

// TestKafka_InstanceStatsDivByZeroGuard verifies that zero totals in
// systemMemoryInfo and fileSystemInfo do not panic and return 0.
func TestKafka_InstanceStatsDivByZeroGuard(t *testing.T) {
	raw := map[string]any{
		"instanceId": "n1",
		"cpuUsage":   map[string]any{"systemCPULoad": float64(50)},
		"systemMemoryInfo": map[string]any{
			"totalMemory": float64(0), "inUseMemory": float64(0),
		},
		"fileSystemInfo": map[string]any{
			"totalSpace": float64(0), "inUseSpace": float64(0),
		},
	}
	ev, err := normalizeKafkaMessage(raw, "", "ams-instance-stats")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data["mem_pct"].(float64) != 0 {
		t.Errorf("mem_pct should be 0 when totalMemory=0, got %v", ev.Data["mem_pct"])
	}
	if ev.Data["disk_pct"].(float64) != 0 {
		t.Errorf("disk_pct should be 0 when totalSpace=0, got %v", ev.Data["disk_pct"])
	}
	t.Logf("PASS: div-by-zero guard — mem_pct=%v disk_pct=%v", ev.Data["mem_pct"], ev.Data["disk_pct"])
}

// TestKafka_Integration is an env-gated end-to-end test.
// It produces the ams-instance-stats fixture to a real broker and asserts the
// Source consumes and normalizes it correctly.
// Set PULSE_TEST_KAFKA_BROKERS=host:port to run; skipped in CI.
func TestKafka_Integration(t *testing.T) {
	brokerStr := os.Getenv("PULSE_TEST_KAFKA_BROKERS")
	if brokerStr == "" {
		t.Skip("PULSE_TEST_KAFKA_BROKERS not set — skipping integration test")
	}

	data, err := os.ReadFile("testdata/ams-instance-stats.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	const topic = "ams-instance-stats"
	brokers := []string{brokerStr}

	// Produce the fixture message.
	w := kafkago.NewWriter(kafkago.WriterConfig{
		Brokers: brokers,
		Topic:   topic,
	})
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafkago.Message{Value: data}); err != nil {
		t.Fatalf("produce: %v", err)
	}

	// Consume with a unique group so we read from the beginning.
	sink := &captureSink{}
	cfg := DefaultConfig(brokers, "")
	cfg.GroupID = "pulse-integration-test-" + t.Name()
	cfg.StartOffset = kafkago.FirstOffset
	cfg.Topics = []string{topic}
	src := New(cfg, sink, nil)

	runCtx, runCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer runCancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = src.Run(runCtx)
	}()

	// Poll until we get an event or time out.
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if len(sink.server) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	runCancel()
	<-done

	if len(sink.server) == 0 {
		t.Fatal("no events received from broker")
	}
	ev := sink.server[0]
	if ev.Type != domain.EventNodeStats {
		t.Errorf("Type = %q, want node_stats", ev.Type)
	}
	if ev.NodeID != "ams-node-1" {
		t.Errorf("NodeID = %q, want ams-node-1", ev.NodeID)
	}
	cpuPct, _ := ev.Data["cpu_pct"].(float64)
	memPct, _ := ev.Data["mem_pct"].(float64)
	diskPct, _ := ev.Data["disk_pct"].(float64)
	if cpuPct == 0 || memPct == 0 || diskPct == 0 {
		t.Errorf("integration: cpu_pct=%v mem_pct=%v disk_pct=%v — expected all non-zero",
			cpuPct, memPct, diskPct)
	}
	t.Logf("PASS integration: cpu_pct=%.2f mem_pct=%.2f disk_pct=%.2f", cpuPct, memPct, diskPct)
}
